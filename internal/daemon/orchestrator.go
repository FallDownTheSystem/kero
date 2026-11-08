package daemon

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kero/internal/commitmsg"
	"kero/internal/convsearch"
	"kero/internal/orchestrator"
	"kero/internal/proto"
	"kero/internal/settings"
)

// Orchestrators and their children (proto.Session's Orchestrator and
// Parent): kerod keeps the links, one level of them, and reports to an
// orchestrator's agent what its children do: each going to wait for
// input, to idle after working, failing, exiting or closing, and the
// summaries they send it (kero report --to-parent). A state counts once
// it has held reportSettle, so that a flicker is no news; what comes close
// together goes as one message, reportQuiet after the last of it, or
// reportMax after the first, pushed into the agent's prompt as every
// message to an agent is (agent.send, deliver.go), which waits for the
// agent to be at its prompt.

const (
	reportTick   = 500 * time.Millisecond
	reportSettle = 2 * time.Second
	reportQuiet  = 4 * time.Second
	reportMax    = 30 * time.Second
	// reportKeep is how many lines a report waiting for its agent keeps,
	// the latest.
	reportKeep = 200
	// reportLabel is whom a report is from, as its agent reads it: "Kero:
	// message from your child sessions:".
	reportLabel = "your child sessions"
)

// reports are the children's states as last seen, and what waits to go
// to each orchestrator; its own lock, never held while taking d.mu.
type reports struct {
	mu   sync.Mutex
	kids map[string]*kid    // by child's id
	out  map[string]*report // by orchestrator's id
	// quiet are the children whose orchestrator closes them itself, which
	// it needs no word of.
	quiet map[string]bool
}

// kid is a child as the reports last saw it: its orchestrator and name;
// the status it is in since since; told, the status it was last held to
// be, which a report may have told; worked, that it worked since;
// reported, that it sent its orchestrator a report while it worked, and
// reportAt, how long its transcript was then, -1 for none; replyAt,
// where its last reply a report told of ends in its transcript, and
// replyText, that reply. turn is the end of a turn its agent told of
// (pane.turn), yet to be told, and hooked that its agent tells them;
// ended is when the last turn told of ended, whose work is no news.
type kid struct {
	parent, name string
	status, told proto.Status
	since        time.Time
	worked       bool
	reported     bool
	reportAt     int64
	replyAt      int64
	replyText    string
	turn         *turnEnd
	hooked       bool
	ended        time.Time
}

// turnEnd is the end of a turn as a child's agent told it, at at.
type turnEnd struct {
	proto.TurnParams
	at time.Time
}

// replyWait is how long after a child went idle its idle line waits for
// the reply to reach its transcript, which an agent may write after its
// screen shows the turn's end.
const replyWait = 8 * time.Second

// report is what waits to go to an orchestrator: lines, the first come at
// first, the last at last. It waits in kerod, not in the mailbox, until
// the orchestrator's agent is at its prompt, so that a child closed
// meanwhile can take its stale state with it (dropLocked).
type report struct {
	lines       []reportLine
	first, last time.Time
	failed      bool // it could not go, and waits for its agent
}

// reportLine is a line of a report; kid is the child whose state it
// tells, "" for what stays true once the child is closed: a summary it
// wrote, or that it closed.
type reportLine struct {
	kid, text string
}

// text is the report's lines, as its agent reads them.
func (rp *report) text() string {
	lines := make([]string, len(rp.lines))
	for i, l := range rp.lines {
		lines[i] = l.text
	}
	return strings.Join(lines, "\n")
}

func (r *reports) initLocked() {
	if r.kids == nil {
		r.kids, r.out, r.quiet = map[string]*kid{}, map[string]*report{}, map[string]bool{}
	}
}

// addLocked puts a line in the report to orchestrator parent, of child
// kid's state, or kid ""; r.mu is held.
func (r *reports) addLocked(parent, kid, line string, now time.Time) {
	rp := r.out[parent]
	if rp == nil {
		rp = &report{first: now}
		r.out[parent] = rp
	}
	rp.lines = append(rp.lines, reportLine{kid: kid, text: line})
	rp.last = now
}

// dropLocked takes out of the report to orchestrator parent the state
// lines of child kid, which closed: its last state, idle or waiting,
// would have the orchestrator act on a session that is no more. Its
// summaries stay. r.mu is held.
func (r *reports) dropLocked(parent, kid string) {
	rp := r.out[parent]
	if rp == nil {
		return
	}
	rp.lines = slices.DeleteFunc(rp.lines, func(l reportLine) bool { return l.kid == kid })
	if len(rp.lines) == 0 {
		delete(r.out, parent)
	}
}

// sessionByIDLocked is the session of id, nil for none; d.mu is held.
func (d *Daemon) sessionByIDLocked(id string) *session {
	for _, s := range d.sessions {
		if s.id == id {
			return s
		}
	}
	return nil
}

// parentLocked is the orchestrator s is a child of, "" for none: a parent
// that closed, or is an orchestrator no more, leaves it a top-level
// session. d.mu is held.
func (d *Daemon) parentLocked(s *session) string {
	if s.parent == "" || s.orchestrator {
		return ""
	}
	if o := d.sessionByIDLocked(s.parent); o != nil && o.orchestrator {
		return o.id
	}
	return ""
}

// childrenLocked are orchestrator o's children, in the sidebar's order;
// d.mu is held.
func (d *Daemon) childrenLocked(o *session) []*session {
	var kids []*session
	for _, s := range d.sessions {
		if s != o && d.parentLocked(s) == o.id {
			kids = append(kids, s)
		}
	}
	return kids
}

// unlinkLocked leaves the children of s, which closes, top-level
// sessions; d.mu is held.
func (d *Daemon) unlinkLocked(s *session) {
	for _, o := range d.sessions {
		if o.parent == s.id {
			o.parent = ""
		}
	}
}

// newParentLocked is the orchestrator a session p starts is to be a child
// of, nil for none: Parent's, else that of By, the session that asks, as
// an agent's kero new. A child's session is another child of its
// orchestrator, as there is one level alone; Parent of a session that is
// neither is an error, By's is none. d.mu is held.
func (d *Daemon) newParentLocked(p proto.CreateParams) (*session, error) {
	if p.Parent == proto.NoParent {
		return nil, nil
	}
	if p.Orchestrator {
		if p.Parent != "" {
			return nil, errors.New("an orchestrator is no other's child: there is one level of them")
		}
		return nil, nil
	}
	ref, explicit := p.Parent, p.Parent != ""
	if !explicit {
		ref = p.By
	}
	if ref == "" {
		return nil, nil
	}
	s := d.findSessionLocked(ref)
	switch {
	case s == nil && explicit:
		return nil, fmt.Errorf("no session %q", ref)
	case s == nil:
		return nil, nil
	case s.orchestrator:
		return s, nil
	}
	if id := d.parentLocked(s); id != "" {
		return d.sessionByIDLocked(id), nil
	}
	if explicit {
		return nil, fmt.Errorf("session %s is no orchestrator: make it one with kero orchestrator %s", s.id, s.id)
	}
	return nil, nil
}

// runningLocked counts o's children that run: those whose programs have
// not all exited. d.mu is held.
func (d *Daemon) runningLocked(o *session) int {
	n := 0
	for _, s := range d.childrenLocked(o) {
		for _, p := range s.panes {
			p.mu.Lock()
			up := p.info.Status != proto.Exited
			p.mu.Unlock()
			if up {
				n++
				break
			}
		}
	}
	return n
}

// childRoomLocked tells whether orchestrator o may start another child,
// as the orchestrator-max-children setting says, counting those starting
// still, which several kero new at once are; d.mu is held.
func (d *Daemon) childRoomLocked(o *session, most int) error {
	if most <= 0 {
		return nil
	}
	if n := d.runningLocked(o) + o.starting; n >= most {
		return fmt.Errorf("orchestrator %s has %d child sessions running, as many as orchestrator-max-children allows: "+
			"wait for one to finish and close it (kero close <id>) first", o.id, n)
	}
	return nil
}

// orchestratorCommand is the command an orchestrator starts: its agent,
// command or the default-agent setting's, with orchestrator-agent-args,
// given the role prompt.
func orchestratorCommand(command []string, goal string) []string {
	set := settings.Load()
	if len(command) == 0 {
		command = commitmsg.Words(set.DefaultAgent)
	}
	command = withArgs(command, set.OrchestratorAgentArgs)
	return orchestrator.AgentArgs(command, orchestrator.Prompt(set.OrchestratorPrompt, goal, set.DefaultAgent))
}

// roleArgs are the arguments the settings give the agent of Ama's session,
// or of an orchestrator, after its command, as it starts or resumes.
func roleArgs(set settings.Settings, ama, orchestrator bool) string {
	switch {
	case ama:
		return set.AmaAgentArgs
	case orchestrator:
		return set.OrchestratorAgentArgs
	}
	return ""
}

// withArgs puts args after an agent's command: as words, or as written
// after a line the shell runs (oneLine, as a resume command of the
// user's), which takes them as the shell reads them.
func withArgs(command []string, args string) []string {
	args = strings.TrimSpace(args)
	if args == "" || len(command) == 0 {
		return command
	}
	if oneLine(command) {
		return []string{command[0] + " " + args}
	}
	return append(slices.Clone(command), commitmsg.Words(args)...)
}

// setOrchestrator makes a session an orchestrator, or an ordinary session
// again, whose children become top-level sessions (orchestrator). A
// session made one has its agent given the role prompt; one whose goal
// changes, the new goal.
func (d *Daemon) setOrchestrator(p proto.OrchestratorParams) (proto.Session, error) {
	d.mu.Lock()
	s := d.findSessionLocked(p.ID)
	if s == nil {
		d.mu.Unlock()
		return proto.Session{}, fmt.Errorf("no session %q", p.ID)
	}
	if parent := d.parentLocked(s); !p.Off && parent != "" {
		d.mu.Unlock()
		return proto.Session{}, fmt.Errorf("session %s is a child of orchestrator %s, and there is one level of them: "+
			"start it as a top-level session (kero new --parent none) to make it one", s.id, parent)
	}
	was, goal := s.orchestrator, s.goal
	s.orchestrator = !p.Off
	if p.Goal != nil {
		s.goal = strings.TrimSpace(*p.Goal)
	}
	if p.Off {
		s.goal = ""
		d.unlinkLocked(s)
	} else {
		s.parent = ""
	}
	info := d.sessionInfo(s)
	d.mu.Unlock()
	d.changed()
	d.log.Printf("session %s: orchestrator %v", info.ID, info.Orchestrator)
	text := ""
	switch {
	case info.Orchestrator && !was:
		set := settings.Load()
		text = orchestrator.Prompt(set.OrchestratorPrompt, info.Goal, set.DefaultAgent)
	case info.Orchestrator && info.Goal != goal && info.Goal != "":
		text = "Kero: your goal as an orchestrator is now: " + info.Goal
	}
	if text != "" {
		if _, err := d.agentSend(proto.AgentSendParams{To: info.ID, Text: text, Mode: "push"}); err != nil {
			d.log.Printf("session %s: the role prompt: %v", info.ID, err)
		}
	}
	return info, nil
}

// quietClose marks a child its orchestrator closes, as p.By tells, which
// it needs no report of; undo takes the mark back, for a close that
// failed.
func (d *Daemon) quietClose(p proto.CloseParams) (undo func()) {
	undo = func() {}
	if p.By == "" {
		return
	}
	d.mu.Lock()
	s, by := d.findSessionLocked(p.ID), d.findSessionLocked(p.By)
	quiet := s != nil && by != nil && d.parentLocked(s) == by.id
	d.mu.Unlock()
	if !quiet {
		return
	}
	r := &d.reports
	r.mu.Lock()
	r.initLocked()
	r.quiet[s.id] = true
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		delete(r.quiet, s.id)
		r.mu.Unlock()
	}
}

// reportToParent puts a child's message to its orchestrator in the next
// report (agent.send's ToParent).
func (d *Daemon) reportToParent(p proto.AgentSendParams) (proto.AgentSendResult, error) {
	d.mu.Lock()
	s := d.findSessionLocked(p.From)
	if s == nil {
		d.mu.Unlock()
		if p.From == "" {
			return proto.AgentSendResult{}, errors.New("agent.send: to whose parent? From is no session's pane")
		}
		return proto.AgentSendResult{}, fmt.Errorf("no session or pane %q", p.From)
	}
	parent := d.parentLocked(s)
	info := d.sessionInfo(s)
	name, id := info.DisplayName(proto.NameLead), s.id
	var as *proto.AgentSession
	if a := prSessionLocked(s).agent; a != nil {
		a.mu.Lock()
		as = a.info.AgentSession
		a.mu.Unlock()
	}
	d.mu.Unlock()
	if parent == "" {
		return proto.AgentSendResult{}, fmt.Errorf("session %s is no orchestrator's child", id)
	}
	at := int64(-1)
	if as != nil {
		at = convsearch.TranscriptSize(proto.Home(), as.Agent, as.ID)
	}
	r := &d.reports
	r.mu.Lock()
	r.initLocked()
	r.addLocked(parent, "", fmt.Sprintf("- session %s %s reports:\n%s", id, proto.Quoted(name), indent(strings.TrimSpace(p.Text))), time.Now())
	// A report at the turn's end tells it: its idle line would add
	// nothing, unless the child went on after it (idleLine).
	if k := r.kids[id]; k != nil && k.status == proto.Working {
		k.reported, k.reportAt = true, at
	}
	r.mu.Unlock()
	return proto.AgentSendResult{State: "queued"}, nil
}

// turnEnded notes that pane pn's agent ended its turn, as its hook tells
// at its end, so that a child's report tells the turn's end when it came
// and with the reply it came with, not as its screen and transcript let
// it be guessed. A turn end just told from the screen is that turn's.
func (d *Daemon) turnEnded(pn *pane, p proto.TurnParams, now time.Time) {
	d.mu.Lock()
	sid := pn.sid
	d.mu.Unlock()
	r := &d.reports
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()
	r.endTurnLocked(sid, p, now)
}

// endTurnLocked notes that child id's agent ended its turn; r.mu is held.
func (r *reports) endTurnLocked(id string, p proto.TurnParams, now time.Time) {
	k := r.kids[id]
	if k == nil {
		return
	}
	k.hooked = true
	if !k.worked && now.Before(k.ended.Add(replyWait)) {
		return
	}
	k.turn = &turnEnd{TurnParams: p, at: now}
}

// indent puts a message's lines under its report's line.
func indent(text string) string {
	return "  " + strings.ReplaceAll(text, "\n", "\n  ")
}

// reportLoop follows the children, and sends the reports as they are
// due.
func (d *Daemon) reportLoop() {
	t := time.NewTicker(reportTick)
	defer t.Stop()
	for {
		select {
		case now := <-t.C:
			d.followChildren(now)
			d.sendReports(now)
		case <-d.quit:
			return
		}
	}
}

// childNow is a child as it is now: its orchestrator, name and status, of
// its agent's pane, else of its lead, and that pane.
type childNow struct {
	parent, name, msg, kind string
	status                  proto.Status
	pane                    *pane
}

// followChildren notes what the children did since the last look, which
// reports tell once it has held reportSettle.
func (d *Daemon) followChildren(now time.Time) {
	d.mu.Lock()
	seen := map[string]childNow{}
	// Which sessions are, and which of them orchestrators, for those the
	// children that are no more were of.
	open, orchestrators := map[string]bool{}, map[string]bool{}
	for _, s := range d.sessions {
		open[s.id], orchestrators[s.id] = true, s.orchestrator
		parent := d.parentLocked(s)
		if parent == "" {
			continue
		}
		info := d.sessionInfo(s)
		p := info.Pane
		a := prSessionLocked(s).agent
		if a != nil {
			if ap := info.PaneByID(a.info.ID); ap != nil {
				p = *ap
			}
		} else {
			a = d.findPaneLocked(p.ID)
		}
		seen[s.id] = childNow{parent: parent, name: info.DisplayName(proto.NameLead), msg: p.Message, kind: p.Kind, status: p.Status, pane: a}
	}
	d.mu.Unlock()

	r := &d.reports
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()
	for id, k := range r.kids {
		if _, ok := seen[id]; ok {
			continue
		}
		// Closed, or no child any more: an orchestrator that closed it, or
		// that is gone, needs no word of it.
		delete(r.kids, id)
		if !open[id] {
			r.dropLocked(k.parent, id)
		}
		quiet := r.quiet[id]
		delete(r.quiet, id)
		if quiet {
			continue
		}
		if orchestrators[k.parent] && !open[id] {
			r.addLocked(k.parent, "", fmt.Sprintf("- session %s %s closed.", id, proto.Quoted(k.name)), now)
		}
	}
	// A child closed before the reports first saw it leaves its mark.
	for id := range r.quiet {
		if r.kids[id] == nil && !open[id] {
			delete(r.quiet, id)
		}
	}
	for id, c := range seen {
		r.followLocked(id, c, now)
	}
}

// followLocked notes what child id did since the last look, as it is now,
// c; r.mu is held.
func (r *reports) followLocked(id string, c childNow, now time.Time) {
	k := r.kids[id]
	if k == nil || k.parent != c.parent {
		// New to its orchestrator: what it is in now is where it starts,
		// no news.
		r.kids[id] = &kid{parent: c.parent, name: c.name, status: c.status, told: c.status, since: now, worked: c.status == proto.Working}
		return
	}
	k.name = c.name
	changed := c.status != k.status
	if changed {
		k.status, k.since = c.status, now
	}
	// What it comes to after working is news, as a second question after
	// the first was answered; but not the work of a turn whose end its
	// agent told, which a screen behind it may still show.
	if c.status == proto.Working && k.since.After(k.ended) {
		k.worked, k.told = true, proto.Working
	}
	r.turnLocked(id, c, k, now)
	if changed || c.status == k.told || now.Sub(k.since) < reportSettle {
		return
	}
	k.told = c.status
	line := ""
	switch c.status {
	case proto.Waiting:
		what := "waits for input"
		if c.kind != "" {
			what += " (" + c.kind + ")"
		}
		line = fmt.Sprintf("- session %s %s %s", id, proto.Quoted(c.name), what)
	case proto.Idle:
		if !k.worked {
			break
		}
		// Of an agent that tells its turns' ends the end comes from it;
		// the screen tells only one it did not, as a turn interrupted.
		if k.turn != nil || k.hooked && now.Sub(k.since) < replyWait {
			k.told = proto.Working
			return
		}
		retry := false
		line, retry = idleLine(id, c, k, nil)
		if retry && now.Sub(k.since) < replyWait {
			// Look again next time, as it worked still.
			k.told = proto.Working
			return
		}
		k.ended = now
	case proto.Failed:
		line = fmt.Sprintf("- session %s %s failed", id, proto.Quoted(c.name))
	case proto.Exited:
		line = fmt.Sprintf("- session %s %s exited: its program ended", id, proto.Quoted(c.name))
	}
	if c.status != proto.Working {
		k.worked, k.reported = false, false
	}
	if line == "" {
		return
	}
	if c.status != proto.Idle {
		if msg := strings.Join(strings.Fields(c.msg), " "); msg != "" {
			line += ": " + msg
		} else {
			line += "."
		}
	}
	r.addLocked(c.parent, id, line, now)
}

// turnLocked tells the end of child id's turn its agent told of, once it
// held reportSettle; r.mu is held.
func (r *reports) turnLocked(id string, c childNow, k *kid, now time.Time) {
	t := k.turn
	switch {
	case t == nil:
		return
	case c.status == proto.Working && k.since.After(t.at):
		// It works again: a stop hook of its agent's went on with the
		// turn, whose end is yet to come.
		k.turn = nil
		return
	case now.Sub(t.at) < reportSettle:
		return
	}
	k.turn = nil
	line, _ := idleLine(id, c, k, t)
	k.ended, k.worked, k.reported = t.at, false, false
	if t.End == proto.TurnFailed {
		k.told = proto.Failed
	}
	if line != "" {
		r.addLocked(c.parent, id, line, now)
	}
}

// idleLine is the line of child id that finished its turn, "" for none:
// with the reply it ended the turn with, which the orchestrator acts on,
// and the work it left running in the background, which tells it is not
// done. A turn with no new reply, as one that only compacted the
// conversation or ran a command, is no news, nor is one whose report
// told its end, unless work still runs; retry tells that the reply may
// not be in the transcript yet. Of an agent whose transcript Kero cannot
// read, the line tells the turn's end alone. t is the turn's end as the
// agent told it, with its reply and background work, which the
// transcript and screen may lag; nil for one the screen tells. r.mu is
// held.
func idleLine(id string, c childNow, k *kid, t *turnEnd) (line string, retry bool) {
	name := proto.Quoted(c.name)
	switch {
	case t != nil && t.End == proto.TurnFailed:
		if e := proto.OneLine(t.Error); e != "" {
			return fmt.Sprintf("- session %s %s failed: %s", id, name, e), false
		}
		return fmt.Sprintf("- session %s %s failed.", id, name), false
	case t != nil && t.End == proto.TurnInterrupted:
		return fmt.Sprintf("- session %s %s stopped: its turn was interrupted, and it is idle.", id, name), false
	}
	head := fmt.Sprintf("- session %s %s finished its turn: it is idle", id, name)
	bg := ""
	switch {
	case t != nil && t.HasBackground:
		bg = t.Background
	case c.pane != nil:
		bg = backgroundWork(c.pane.readText(false, 0))
	}
	if bg != "" {
		head += ", but " + bg + " still running"
	}
	var as *proto.AgentSession
	if c.pane != nil {
		c.pane.mu.Lock()
		as = c.pane.info.AgentSession
		c.pane.mu.Unlock()
	}
	var reply convsearch.Reply
	err := convsearch.ErrNoTranscript
	switch {
	case t != nil && t.HasReply:
		reply, err = convsearch.Reply{Text: t.Reply, At: k.replyAt}, nil
	case as != nil:
		reply, err = convsearch.LastReply(proto.Home(), as.Agent, as.ID)
	}
	switch {
	case err != nil:
		if k.reported && bg == "" {
			return "", false
		}
		return head + ".", false
	case t != nil && strings.TrimSpace(reply.Text) == "":
		// Its turn ended with no words, as a compaction's.
		if bg != "" && !k.reported {
			return head + ".", false
		}
		return "", false
	// The transcript's last reply, after a turn that ended unlike its
	// agent tells (interrupted), may be the one its hook told.
	case t == nil && (reply.Text == "" || reply.At == k.replyAt || k.hooked && reply.Text == k.replyText):
		return "", true
	}
	k.replyAt, k.replyText = reply.At, reply.Text
	// The report told the turn's end when nothing but its own tool call's
	// result came after it.
	if k.reported && k.reportAt >= 0 && as != nil {
		if n, err := convsearch.TurnsAfter(proto.Home(), as.Agent, as.ID, k.reportAt); err != nil || n > 1 {
			k.reported = false
		}
	}
	switch {
	case k.reported && bg == "":
		return "", false
	case k.reported:
		return head + ".", false
	}
	return head + ". Its reply:\n" + indent(clipReply(reply.Text, id)), false
}

// clipHalf is how much of a long reply a report keeps of its start, and
// of its end: the start says what was done, the end asks what the child
// waits for.
const clipHalf = 1000

// clipReply is a reply as a report carries it: whole, else its start and
// its end, with what was cut between, and where the whole of it is.
func clipReply(text, id string) string {
	rs := []rune(text)
	if len(rs) <= clipHalf*2+clipHalf/2 {
		return text
	}
	head, tail := string(rs[:clipHalf]), string(rs[len(rs)-clipHalf:])
	// At a line's end, when one is near.
	if i := strings.LastIndexByte(head, '\n'); i >= len(head)*3/4 {
		head = head[:i]
	}
	if i := strings.IndexByte(tail, '\n'); i >= 0 && i < len(tail)/4 {
		tail = tail[i+1:]
	}
	cut := len(rs) - utf8.RuneCountInString(head) - utf8.RuneCountInString(tail)
	return fmt.Sprintf("%s\n[… %d characters cut: kero read %s --reply prints the whole reply …]\n%s",
		strings.TrimRight(head, " \n"), cut, id, strings.TrimLeft(tail, " \n"))
}

// backgroundRe finds what an agent's screen says still runs after its
// turn, as Claude Code's "1 shell, 1 monitor still running" or "2 MCP
// tasks still running".
var backgroundRe = regexp.MustCompile(`(\d+ [A-Za-z]+(?: [A-Za-z]+)?(?:(?:,| and) \d+ [A-Za-z]+(?: [A-Za-z]+)?)*) still running`)

// backgroundWork is what screen, a pane's, says still runs, from its
// last lines, where an agent tells it under its prompt; "" for nothing.
func backgroundWork(screen string) string {
	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	lines = lines[max(0, len(lines)-12):]
	for i := len(lines) - 1; i >= 0; i-- {
		if m := backgroundRe.FindStringSubmatch(lines[i]); m != nil {
			return m[1]
		}
	}
	return ""
}

// reportFooter ends a report: what the orchestrator may do of it.
const reportFooter = "kero read <id> shows a session's screen (with --reply, its whole last reply), kero prompt <id> \"<text>\" answers it, kero status shows them all. " +
	"If nothing here needs you, reply just OK: no summary."

// reportReady tells whether orchestrator id's agent is at its prompt,
// where a report is typed at once; true too for one with no agent Kero
// knows, which agent.send tells of.
func (d *Daemon) reportReady(id string) bool {
	to, err := d.agentTarget(id, "")
	if err != nil || to == "" {
		return true
	}
	d.mu.Lock()
	pn := d.findPaneLocked(to)
	d.mu.Unlock()
	if pn == nil {
		return true
	}
	pn.mu.Lock()
	defer pn.mu.Unlock()
	return pn.agentReady()
}

// sendReports sends each orchestrator the report due to it, as one
// message.
func (d *Daemon) sendReports(now time.Time) {
	r := &d.reports
	r.mu.Lock()
	r.initLocked()
	var ids []string
	for id, rp := range r.out {
		if now.Sub(rp.last) >= reportQuiet || now.Sub(rp.first) >= reportMax {
			ids = append(ids, id)
		}
	}
	r.mu.Unlock()
	slices.Sort(ids)
	for _, id := range ids {
		// A busy orchestrator's report stays here till its agent is at
		// its prompt, where what closes meanwhile still changes it.
		if !d.reportReady(id) {
			continue
		}
		r.mu.Lock()
		rp := r.out[id]
		delete(r.out, id)
		r.mu.Unlock()
		if rp == nil {
			continue
		}
		text := rp.text() + "\n\n" + reportFooter
		_, err := d.agentSend(proto.AgentSendParams{To: id, Label: reportLabel, Text: text, Mode: "push"})
		if err == nil {
			continue
		}
		// Its agent isn't there yet, as just after kerod restarted: the
		// report waits for it, with what comes meanwhile, while the
		// orchestrator is one, and goes with its orchestrator.
		d.mu.Lock()
		o := d.sessionByIDLocked(id)
		keep := o != nil && o.orchestrator
		d.mu.Unlock()
		if !keep {
			continue
		}
		if !rp.failed {
			d.log.Printf("report to orchestrator %s: %v; kept for its agent", id, err)
		}
		r.mu.Lock()
		if later := r.out[id]; later != nil {
			rp.lines = append(rp.lines, later.lines...)
		}
		if n := len(rp.lines); n > reportKeep {
			rp.lines = rp.lines[n-reportKeep:]
		}
		rp.last, rp.failed = now, true
		r.out[id] = rp
		r.mu.Unlock()
	}
}
