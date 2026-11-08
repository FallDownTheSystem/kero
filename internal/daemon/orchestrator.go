package daemon

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"kero/internal/commitmsg"
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
// be, which a report may have told; worked, that it worked since.
type kid struct {
	parent, name string
	status, told proto.Status
	since        time.Time
	worked       bool
}

// report is what waits to go to an orchestrator: lines, the first come at
// first, the last at last.
type report struct {
	lines       []string
	first, last time.Time
}

func (r *reports) initLocked() {
	if r.kids == nil {
		r.kids, r.out, r.quiet = map[string]*kid{}, map[string]*report{}, map[string]bool{}
	}
}

// addLocked puts a line in the report to orchestrator parent; r.mu is
// held.
func (r *reports) addLocked(parent, line string, now time.Time) {
	rp := r.out[parent]
	if rp == nil {
		rp = &report{first: now}
		r.out[parent] = rp
	}
	rp.lines = append(rp.lines, line)
	rp.last = now
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

// childRoom tells whether orchestrator o may start another child, as the
// orchestrator-max-children setting says; d.mu is held.
func (d *Daemon) childRoomLocked(o *session, most int) error {
	if most <= 0 {
		return nil
	}
	if n := d.runningLocked(o); n >= most {
		return fmt.Errorf("orchestrator %s has %d child sessions running, as many as orchestrator-max-children allows: "+
			"wait for one to finish and close it (kero close <id>) first", o.id, n)
	}
	return nil
}

// orchestratorCommand is the command an orchestrator starts: its agent,
// command or the default-agent setting's, given the role prompt.
func orchestratorCommand(command []string, goal string) []string {
	set := settings.Load()
	if len(command) == 0 {
		command = commitmsg.Words(set.DefaultAgent)
	}
	return orchestrator.AgentArgs(command, orchestrator.Prompt(set.OrchestratorPrompt, goal, set.DefaultAgent))
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
	d.log.Printf("session %s: orchestrator %v", s.id, s.orchestrator)
	text := ""
	switch {
	case s.orchestrator && !was:
		set := settings.Load()
		text = orchestrator.Prompt(set.OrchestratorPrompt, info.Goal, set.DefaultAgent)
	case s.orchestrator && info.Goal != goal && info.Goal != "":
		text = "Kero: your goal as an orchestrator is now: " + info.Goal
	}
	if text != "" {
		if _, err := d.agentSend(proto.AgentSendParams{To: s.id, Text: text, Mode: "push"}); err != nil {
			d.log.Printf("session %s: the role prompt: %v", s.id, err)
		}
	}
	return info, nil
}

// quietClose marks a child its orchestrator closes, as p.By tells, which
// it needs no report of.
func (d *Daemon) quietClose(p proto.CloseParams) {
	if p.By == "" {
		return
	}
	d.mu.Lock()
	s := d.findSessionLocked(p.ID)
	var by *session
	if pn := d.findPaneLocked(p.By); pn != nil {
		by = d.sessionOfLocked(pn)
	} else {
		by = d.findSessionLocked(p.By)
	}
	quiet := s != nil && by != nil && d.parentLocked(s) == by.id
	id := ""
	if s != nil {
		id = s.id
	}
	d.mu.Unlock()
	if !quiet {
		return
	}
	r := &d.reports
	r.mu.Lock()
	r.initLocked()
	r.quiet[id] = true
	r.mu.Unlock()
}

// reportToParent puts a child's message to its orchestrator in the next
// report (agent.send's ToParent).
func (d *Daemon) reportToParent(p proto.AgentSendParams) (proto.AgentSendResult, error) {
	d.mu.Lock()
	var s *session
	if pn := d.findPaneLocked(p.From); pn != nil {
		s = d.sessionOfLocked(pn)
	} else {
		s = d.findSessionLocked(p.From)
	}
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
	d.mu.Unlock()
	if parent == "" {
		return proto.AgentSendResult{}, fmt.Errorf("session %s is no orchestrator's child", id)
	}
	r := &d.reports
	r.mu.Lock()
	r.initLocked()
	r.addLocked(parent, fmt.Sprintf("- session %s %q reports:\n%s", id, name, indent(strings.TrimSpace(p.Text))), time.Now())
	r.mu.Unlock()
	return proto.AgentSendResult{State: "queued"}, nil
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
// its agent's pane, else of its lead.
type childNow struct {
	parent, name, msg, kind string
	status                  proto.Status
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
		if a := prSessionLocked(s).agent; a != nil {
			if ap := info.PaneByID(a.info.ID); ap != nil {
				p = *ap
			}
		}
		seen[s.id] = childNow{parent: parent, name: info.DisplayName(proto.NameLead), msg: p.Message, kind: p.Kind, status: p.Status}
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
		quiet := r.quiet[id]
		delete(r.quiet, id)
		if quiet {
			continue
		}
		if orchestrators[k.parent] && !open[id] {
			r.addLocked(k.parent, fmt.Sprintf("- session %s %q closed.", id, k.name), now)
		}
	}
	for id, c := range seen {
		k := r.kids[id]
		if k == nil || k.parent != c.parent {
			// New to its orchestrator: what it is in now is where it
			// starts, no news.
			r.kids[id] = &kid{parent: c.parent, name: c.name, status: c.status, told: c.status, since: now, worked: c.status == proto.Working}
			continue
		}
		k.name = c.name
		if c.status == proto.Working {
			k.worked = true
		}
		if c.status != k.status {
			k.status, k.since = c.status, now
			continue
		}
		if c.status == k.told || now.Sub(k.since) < reportSettle {
			continue
		}
		k.told = c.status
		line := ""
		switch c.status {
		case proto.Waiting:
			what := "waits for input"
			if c.kind != "" {
				what += " (" + c.kind + ")"
			}
			line = fmt.Sprintf("- session %s %q %s", id, c.name, what)
		case proto.Idle:
			if k.worked {
				line = fmt.Sprintf("- session %s %q finished its turn: it is idle", id, c.name)
			}
		case proto.Failed:
			line = fmt.Sprintf("- session %s %q failed", id, c.name)
		case proto.Exited:
			line = fmt.Sprintf("- session %s %q exited: its program ended", id, c.name)
		}
		if c.status != proto.Working {
			k.worked = false
		}
		if line == "" {
			continue
		}
		if msg := strings.Join(strings.Fields(c.msg), " "); msg != "" && c.status != proto.Idle {
			line += ": " + msg
		} else {
			line += "."
		}
		r.addLocked(c.parent, line, now)
	}
}

// reportFooter ends a report: what the orchestrator may do of it.
const reportFooter = "kero read <id> shows a session's screen, kero prompt <id> \"<text>\" answers it, kero status shows them all."

// sendReports sends each orchestrator the report due to it, as one
// message.
func (d *Daemon) sendReports(now time.Time) {
	r := &d.reports
	r.mu.Lock()
	r.initLocked()
	due := map[string]string{}
	for id, rp := range r.out {
		if now.Sub(rp.last) >= reportQuiet || now.Sub(rp.first) >= reportMax {
			due[id] = strings.Join(rp.lines, "\n") + "\n\n" + reportFooter
			delete(r.out, id)
		}
	}
	r.mu.Unlock()
	ids := make([]string, 0, len(due))
	for id := range due {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if _, err := d.agentSend(proto.AgentSendParams{To: id, Label: reportLabel, Text: due[id], Mode: "push"}); err != nil {
			d.log.Printf("report to orchestrator %s: %v", id, err)
		}
	}
}
