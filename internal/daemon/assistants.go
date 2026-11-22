package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kero/internal/assistant"
	"kero/internal/proto"
	"kero/internal/settings"
)

// The user's assistants (proto.Assistant): kerod keeps their definitions,
// their checks' times and their last reports in assistants.json, and is
// the only one to write it: the app and the CLI change them through its
// methods (assistant.*). Each has at most one session, of no project, in
// its folder (internal/assistant), whose panes get $KERO_ASSISTANT, its
// id: no secret, and it grants nothing. Its checks come to it through
// agent.send, as Kero's every message to an agent does, labelled "your
// schedule", once its agent is at its prompt (assistantLoop). Its session
// leads children as an orchestrator's does (leads), in any project.

const (
	assistantTick = 20 * time.Second
	// assistantFirst is how long after kerod starts the schedule first
	// looks: past startHold, so that restored agents and their listeners
	// are back.
	assistantFirst = 20 * time.Second
	// assistantNoAgent is how long a session with checks due may run no
	// agent before a fresh one replaces it, at most every
	// assistantReplace.
	assistantNoAgent = 2 * time.Minute
	assistantReplace = 30 * time.Minute
	// assistantReports is how many reports an assistant keeps, each of at
	// most assistantReportMax characters; assistantClip how much of the
	// last the state carries.
	assistantReports   = 10
	assistantReportMax = 4000
	assistantClip      = 200
	assistantMaxChecks = 8
	assistantMaxTracks = 8000
	// assistantLabel is whom a check is from, as its agent reads it: "Kero:
	// message from your schedule:".
	assistantLabel = "your schedule"
	// waitingRecord is the record a report waiting for the user declares
	// on its agent's pane, as kero report blocked would.
	waitingRecord = "kero/assistant"
	assistantEnv  = "KERO_ASSISTANT"
)

// assistants are the definitions and what the schedule keeps of them; its
// own lock, taken after d.mu, never while holding it the other way round.
type assistants struct {
	mu   sync.Mutex
	list []*savedAssistant
	// starting are the sessions on their way, by assistant: a second
	// start waits for the first's (startAssistant).
	starting map[string]*assistantStart
	// forced are the checks run asks for, by assistant ("" for all);
	// noAgent when an assistant with checks due was first seen to run no
	// agent; replaced when its session was last replaced for it;
	// problem why its due checks don't run.
	forced   map[string][]string
	noAgent  map[string]time.Time
	replaced map[string]time.Time
	problem  map[string]string
	wake     chan struct{}
}

// savedAssistant is an assistant as assistants.json keeps it: its
// definition, and its reports, the last last.
type savedAssistant struct {
	proto.Assistant
	Reports []proto.AssistantReport `json:"reports,omitempty"`
}

type savedAssistants struct {
	Version    int               `json:"version"`
	Assistants []*savedAssistant `json:"assistants"`
}

// assistantStart is a session on its way, which other starts wait for.
type assistantStart struct {
	done chan struct{}
	s    proto.Session
	err  error
}

func (as *assistants) initLocked() {
	if as.starting == nil {
		as.starting, as.forced = map[string]*assistantStart{}, map[string][]string{}
		as.noAgent, as.replaced, as.problem = map[string]time.Time{}, map[string]time.Time{}, map[string]string{}
		as.wake = make(chan struct{}, 1)
	}
}

func (d *Daemon) assistantsPath() string { return filepath.Join(d.dir, "assistants.json") }

// loadAssistants reads assistants.json, before restore, which gives the
// assistants their sessions back.
func (d *Daemon) loadAssistants() {
	as := &d.asst
	as.mu.Lock()
	defer as.mu.Unlock()
	as.initLocked()
	b, err := os.ReadFile(d.assistantsPath())
	if err != nil {
		return
	}
	var saved savedAssistants
	if err := json.Unmarshal(b, &saved); err != nil {
		d.log.Printf("assistants.json: %v", err)
		return
	}
	as.list = slices.DeleteFunc(saved.Assistants, func(a *savedAssistant) bool { return a == nil || a.ID == "" })
}

// saveAssistantsLocked writes assistants.json; as.mu is held.
func (d *Daemon) saveAssistantsLocked() {
	data, err := json.MarshalIndent(savedAssistants{Version: 1, Assistants: d.asst.list}, "", "  ")
	if err == nil {
		err = assistant.WriteFile(d.assistantsPath(), data, 0o600)
	}
	if err != nil {
		d.log.Printf("saving assistants.json: %v", err)
	}
}

// findLocked finds an assistant by id, a unique prefix of it, or its
// name, of any case; as.mu is held.
func (as *assistants) findLocked(ref string) (*savedAssistant, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("which assistant?")
	}
	for _, a := range as.list {
		if a.ID == ref || strings.EqualFold(a.Name, ref) {
			return a, nil
		}
	}
	var found []*savedAssistant
	for _, a := range as.list {
		if strings.HasPrefix(a.ID, strings.ToLower(ref)) {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, fmt.Errorf("no assistant %q", ref)
	}
	return nil, fmt.Errorf("%q names several assistants", ref)
}

// assistantDef is assistant id's definition, a copy; ok false for none.
func (d *Daemon) assistantDef(id string) (proto.Assistant, bool) {
	as := &d.asst
	as.mu.Lock()
	defer as.mu.Unlock()
	for _, a := range as.list {
		if a.ID == id {
			return copyAssistant(a.Assistant), true
		}
	}
	return proto.Assistant{}, false
}

func copyAssistant(a proto.Assistant) proto.Assistant {
	a.Checks = slices.Clone(a.Checks)
	for i := range a.Checks {
		a.Checks[i].Days = slices.Clone(a.Checks[i].Days)
	}
	return a
}

// assistantsLocked are the assistants as the state shows them, with
// their sessions, of sessions by assistant; d.mu is held.
func (d *Daemon) assistantsLocked(now time.Time) []proto.Assistant {
	sessions := map[string]string{}
	for _, s := range d.sessions {
		if s.assistant != "" {
			sessions[s.assistant] = s.id
		}
	}
	as := &d.asst
	as.mu.Lock()
	defer as.mu.Unlock()
	as.initLocked()
	out := make([]proto.Assistant, 0, len(as.list))
	for _, a := range as.list {
		info := copyAssistant(a.Assistant)
		info.Dir, info.Session, info.Problem = assistant.Dir(a.ID), sessions[a.ID], as.problem[a.ID]
		if n := len(a.Reports); n > 0 {
			r := a.Reports[n-1]
			if rs := []rune(r.Text); len(rs) > assistantClip {
				r.Text = string(rs[:assistantClip]) + "…"
			}
			info.LastReport = &r
		}
		if !a.Paused {
			for _, c := range a.Checks {
				if t := assistant.Next(c, now); !t.IsZero() && (info.NextCheck == 0 || t.UnixMilli() < info.NextCheck) {
					info.NextCheck = t.UnixMilli()
				}
			}
		}
		out = append(out, info)
	}
	return out
}

// assistantEnvOf is the environment a pane of session sid adds: the
// assistant's id, for an assistant's.
func (d *Daemon) assistantEnvOf(sid string) []string {
	if id, ok := d.assistantSIDs.Load(sid); ok {
		return []string{assistantEnv + "=" + id.(string)}
	}
	return nil
}

// leads tells a session that may have children: an orchestrator, or an
// assistant's.
func leads(s *session) bool { return s != nil && (s.orchestrator || s.assistant != "") }

// assistantSessionLocked is assistant id's session, nil for none; d.mu is
// held.
func (d *Daemon) assistantSessionLocked(id string) *session {
	for _, s := range d.sessions {
		if s.assistant == id {
			return s
		}
	}
	return nil
}

// applyEdit puts what e sets into a; now is when its checks changed.
func applyEdit(a *proto.Assistant, e proto.AssistantEdit, now int64) error {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	set(&a.Name, e.Name)
	set(&a.Color, e.Color)
	set(&a.Avatar, e.Avatar)
	set(&a.Group, e.Group)
	set(&a.Role, e.Role)
	set(&a.Agent, e.Agent)
	set(&a.AgentArgs, e.AgentArgs)
	if e.Tracks != nil {
		a.Tracks = strings.TrimSpace(*e.Tracks)
	}
	if e.Paused != nil {
		a.Paused = *e.Paused
	}
	switch {
	case a.Name == "":
		return errors.New("an assistant needs a name")
	case strings.TrimSpace(a.Role) == "":
		return errors.New("an assistant needs a role: a line of what it does")
	case len(a.Tracks) > assistantMaxTracks:
		return fmt.Errorf("what it keeps track of is %d characters long, more than %d", len(a.Tracks), assistantMaxTracks)
	case !assistant.ColorValid(a.Color):
		return fmt.Errorf("no color %q: %s, or #rrggbb", a.Color, strings.Join(settings.HostColors, ", "))
	case strings.HasPrefix(a.Avatar, "file:") && strings.ContainsAny(strings.TrimPrefix(a.Avatar, "file:"), `/\`):
		return errors.New("an avatar's image is a file of the assistant's folder")
	}
	if e.Checks != nil {
		if len(*e.Checks) > assistantMaxChecks {
			return fmt.Errorf("an assistant has at most %d checks", assistantMaxChecks)
		}
		old := map[string]proto.AssistantCheck{}
		for _, c := range a.Checks {
			old[c.ID] = c
		}
		var checks []proto.AssistantCheck
		used := map[string]bool{}
		for _, c := range *e.Checks {
			c.Kind, c.Time, c.Text = strings.ToLower(strings.TrimSpace(c.Kind)), strings.TrimSpace(c.Time), strings.TrimSpace(c.Text)
			if c.Kind != proto.CheckWeekly {
				c.Days = nil
			}
			if c.Kind == proto.CheckStart {
				c.Time = ""
			}
			if err := assistant.CheckValid(c); err != nil {
				return err
			}
			slices.Sort(c.Days)
			c.Days = slices.Compact(c.Days)
			if o, ok := old[c.ID]; ok && c.ID != "" && !used[c.ID] {
				// Its times stay unless when it runs changed.
				c.Last, c.Created = o.Last, o.Created
				if o.Kind != c.Kind || o.Time != c.Time || !slices.Equal(o.Days, c.Days) {
					c.Created = now
				}
			} else {
				c.ID, c.Created, c.Last = "", now, 0
			}
			if c.ID != "" {
				used[c.ID] = true
			}
			checks = append(checks, c)
		}
		for i := range checks {
			if checks[i].ID != "" {
				continue
			}
			for n := 1; ; n++ {
				if id := fmt.Sprintf("c%d", n); !used[id] {
					checks[i].ID, used[id] = id, true
					break
				}
			}
		}
		a.Checks = checks
	}
	return nil
}

// avatarExts are the images an avatar may be, and avatarMax how large.
var avatarExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

const avatarMax = 2 << 20

// copyAvatar copies the image at src into assistant id's folder, as its
// avatar, in place of the one there was; it returns its Avatar.
func copyAvatar(id, src string) (string, error) {
	ext := strings.ToLower(filepath.Ext(src))
	if !slices.Contains(avatarExts, ext) {
		return "", fmt.Errorf("an avatar is a PNG, JPEG, GIF or WebP image, not %s", filepath.Base(src))
	}
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, avatarMax+1))
	if err != nil {
		return "", err
	}
	if len(data) > avatarMax {
		return "", fmt.Errorf("%s is larger than 2 MB", filepath.Base(src))
	}
	dir := assistant.Dir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, e := range avatarExts {
		if e != ext {
			os.Remove(filepath.Join(dir, "avatar"+e))
		}
	}
	if err := assistant.WriteFile(filepath.Join(dir, "avatar"+ext), data, 0o644); err != nil {
		return "", err
	}
	return "file:avatar" + ext, nil
}

// createAssistant makes an assistant, its folder, and with e.Start its
// session.
func (d *Daemon) createAssistant(e proto.AssistantEdit) (proto.Assistant, error) {
	now := time.Now().UnixMilli()
	a := proto.Assistant{Created: now}
	if err := applyEdit(&a, e, now); err != nil {
		return a, err
	}
	as := &d.asst
	as.mu.Lock()
	as.initLocked()
	for _, o := range as.list {
		if strings.EqualFold(o.Name, a.Name) {
			as.mu.Unlock()
			return a, fmt.Errorf("there is an assistant named %s already", o.Name)
		}
	}
	a.ID = assistant.NewID(a.Name, func(id string) bool {
		if slices.ContainsFunc(as.list, func(o *savedAssistant) bool { return strings.EqualFold(o.ID, id) }) {
			return true
		}
		// A folder left by one removed holds its notes: a new one doesn't
		// take them over.
		_, err := os.Stat(assistant.Dir(id))
		return err == nil
	})
	if e.Image != "" {
		avatar, err := copyAvatar(a.ID, e.Image)
		if err != nil {
			as.mu.Unlock()
			return a, err
		}
		a.Avatar = avatar
	}
	if _, err := assistant.Prepare(a); err != nil {
		as.mu.Unlock()
		return a, err
	}
	as.list = append(as.list, &savedAssistant{Assistant: a})
	d.saveAssistantsLocked()
	as.mu.Unlock()
	d.log.Printf("assistant %s created", a.ID)
	d.changed()
	if e.Start {
		if _, err := d.startAssistant(a.ID, false); err != nil {
			d.log.Printf("assistant %s: starting its session: %v", a.ID, err)
		}
	}
	return d.assistantInfo(a.ID)
}

// assistantInfo is assistant id as the state shows it.
func (d *Daemon) assistantInfo(id string) (proto.Assistant, error) {
	d.mu.Lock()
	list := d.assistantsLocked(time.Now())
	d.mu.Unlock()
	for _, a := range list {
		if a.ID == id {
			return a, nil
		}
	}
	return proto.Assistant{}, fmt.Errorf("no assistant %q", id)
}

// updateAssistant changes an assistant as e asks: its folder's
// instructions follow, and an assistant running hears of them; its name
// is its session's too.
func (d *Daemon) updateAssistant(e proto.AssistantEdit) (proto.Assistant, error) {
	as := &d.asst
	as.mu.Lock()
	as.initLocked()
	sa, err := as.findLocked(e.ID)
	if err != nil {
		as.mu.Unlock()
		return proto.Assistant{}, err
	}
	a := copyAssistant(sa.Assistant)
	if err := applyEdit(&a, e, time.Now().UnixMilli()); err != nil {
		as.mu.Unlock()
		return a, err
	}
	for _, o := range as.list {
		if o != sa && strings.EqualFold(o.Name, a.Name) {
			as.mu.Unlock()
			return a, fmt.Errorf("there is an assistant named %s already", o.Name)
		}
	}
	if e.Image != "" {
		avatar, err := copyAvatar(a.ID, e.Image)
		if err != nil {
			as.mu.Unlock()
			return a, err
		}
		a.Avatar = avatar
	}
	changed, err := assistant.Prepare(a)
	if err != nil {
		as.mu.Unlock()
		return a, err
	}
	sa.Assistant = a
	if a.Paused {
		delete(as.problem, a.ID)
	}
	d.saveAssistantsLocked()
	as.mu.Unlock()
	d.mu.Lock()
	s := d.assistantSessionLocked(a.ID)
	sid := ""
	if s != nil {
		s.name, sid = a.Name, s.id
	}
	d.mu.Unlock()
	d.changed()
	if changed && sid != "" {
		if _, err := d.agentSend(proto.AgentSendParams{To: sid, Label: "Kero", Mode: "push",
			Text: "Your instructions (CLAUDE.md and AGENTS.md in this folder) changed: read them again."}); err != nil {
			d.log.Printf("assistant %s: telling of its instructions: %v", a.ID, err)
		}
	}
	return d.assistantInfo(a.ID)
}

// removeAssistant closes an assistant's session and forgets it; with
// p.Folder, its folder goes too, which Windows lets go only once no
// program runs in it: it tries for a while, and says where it stays.
func (d *Daemon) removeAssistant(p proto.AssistantParams) error {
	as := &d.asst
	as.mu.Lock()
	sa, err := as.findLocked(p.ID)
	if err != nil {
		as.mu.Unlock()
		return err
	}
	id := sa.ID
	as.list = slices.DeleteFunc(as.list, func(o *savedAssistant) bool { return o == sa })
	delete(as.forced, id)
	delete(as.problem, id)
	delete(as.noAgent, id)
	d.saveAssistantsLocked()
	as.mu.Unlock()
	d.mu.Lock()
	var sids []string
	for _, s := range d.sessions {
		if s.assistant == id {
			sids = append(sids, s.id)
		}
	}
	d.mu.Unlock()
	var panes []*pane
	for _, sid := range sids {
		ps, _ := d.closePanes(sid)
		panes = append(panes, ps...)
	}
	d.changed()
	d.log.Printf("assistant %s removed", id)
	if !p.Folder {
		return nil
	}
	dir := assistant.Dir(id)
	for i := 0; i < 20; i++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("the assistant is removed, but its folder stays, as a program still has it open: %s (%v)", dir, err)
}

// startAssistant starts assistant id's session, or with fresh a new one
// in place of the one it has; without fresh, the one it has is the one
// it gets. Starts at once, as the schedule's and a click's, make one
// session: the second waits for the first's.
func (d *Daemon) startAssistant(ref string, fresh bool) (proto.Session, error) {
	as := &d.asst
	as.mu.Lock()
	as.initLocked()
	sa, err := as.findLocked(ref)
	if err != nil {
		as.mu.Unlock()
		return proto.Session{}, err
	}
	id := sa.ID
	if st := as.starting[id]; st != nil {
		as.mu.Unlock()
		<-st.done
		if !fresh {
			return st.s, st.err
		}
		return d.startAssistant(id, true)
	}
	if !fresh {
		as.mu.Unlock()
		d.mu.Lock()
		if s := d.assistantSessionLocked(id); s != nil {
			info := d.sessionInfo(s)
			d.mu.Unlock()
			return info, nil
		}
		d.mu.Unlock()
		as.mu.Lock()
		if st := as.starting[id]; st != nil {
			as.mu.Unlock()
			<-st.done
			return st.s, st.err
		}
	}
	st := &assistantStart{done: make(chan struct{})}
	as.starting[id] = st
	a := copyAssistant(sa.Assistant)
	as.mu.Unlock()
	st.s, st.err = d.createAssistantSession(proto.CreateParams{Assistant: id, Command: assistant.Command(a, settings.Load()), Cols: 100, Rows: 30}, a)
	as.mu.Lock()
	delete(as.starting, id)
	as.mu.Unlock()
	close(st.done)
	return st.s, st.err
}

// createAssistantSession starts an assistant's session in its folder, of
// no project, named after it; the one it had closes once this one
// started, as Ama's does.
func (d *Daemon) createAssistantSession(p proto.CreateParams, a proto.Assistant) (proto.Session, error) {
	if _, err := assistant.Prepare(a); err != nil {
		return proto.Session{}, err
	}
	dir := assistant.Dir(a.ID)
	s := &session{id: newID(), name: a.Name, created: time.Now().UnixMilli(), assistant: a.ID}
	d.assistantSIDs.Store(s.id, a.ID)
	p.Worktree, p.URL, p.File, p.Preview, p.Review, p.Project, p.Orchestrator = "", "", "", false, nil, "", false
	pn, err := d.startPane(s.id, p, dir)
	if err != nil {
		d.assistantSIDs.Delete(s.id)
		return proto.Session{}, err
	}
	s.panes, s.focus = []*pane{pn}, pn.info.ID
	d.mu.Lock()
	var old []string
	for _, o := range d.sessions {
		if o.assistant == a.ID {
			old = append(old, o.id)
		}
	}
	d.sessions = append(d.sessions, s)
	d.panes[pn.info.ID] = pn
	d.mu.Unlock()
	for _, id := range old {
		d.removeSession(id)
	}
	d.mu.Lock()
	info := d.sessionInfo(s)
	d.mu.Unlock()
	d.changed()
	d.log.Printf("assistant %s: session %s created in %s", a.ID, s.id, dir)
	return info, nil
}

// createAssistantP is create of p.Assistant's session, as a conversation
// of its folder resumes.
func (d *Daemon) createAssistantP(p proto.CreateParams) (proto.Session, error) {
	a, ok := d.assistantDef(p.Assistant)
	if !ok {
		return proto.Session{}, fmt.Errorf("no assistant %q", p.Assistant)
	}
	return d.createAssistantSession(p, a)
}

// runAssistant runs an assistant's checks now, or one: they go to it as a
// due check does, once its agent is at its prompt.
func (d *Daemon) runAssistant(p proto.AssistantParams) (proto.AgentSendResult, error) {
	as := &d.asst
	as.mu.Lock()
	as.initLocked()
	sa, err := as.findLocked(p.ID)
	if err != nil {
		as.mu.Unlock()
		return proto.AgentSendResult{}, err
	}
	if p.Check != "" && !slices.ContainsFunc(sa.Checks, func(c proto.AssistantCheck) bool { return c.ID == p.Check }) {
		as.mu.Unlock()
		return proto.AgentSendResult{}, fmt.Errorf("%s has no check %q", sa.Name, p.Check)
	}
	if p.Check == "" {
		as.forced[sa.ID] = []string{""}
	} else if !slices.Contains(as.forced[sa.ID], p.Check) {
		as.forced[sa.ID] = append(as.forced[sa.ID], p.Check)
	}
	id := sa.ID
	as.mu.Unlock()
	select {
	case as.wake <- struct{}{}:
	default:
	}
	return proto.AgentSendResult{State: "queued", To: id}, nil
}

// assistantReport keeps an assistant's report to the user: of
// p.Assistant, else of the assistant whose session p.From is of. A report
// waiting for the user declares its agent's pane blocked with its
// question, as kero report blocked would; any other report takes that
// back.
func (d *Daemon) assistantReport(p proto.AssistantReportParams) (proto.AssistantReport, error) {
	text := strings.TrimSpace(p.Text)
	if text == "" {
		return proto.AssistantReport{}, errors.New("an empty report")
	}
	id := p.Assistant
	if id == "" {
		d.mu.Lock()
		s := d.findSessionLocked(p.From)
		if s != nil {
			id = s.assistant
		}
		d.mu.Unlock()
		if id == "" {
			return proto.AssistantReport{}, errors.New("whose report? Run it in an assistant's session, or give --assistant")
		}
	}
	if rs := []rune(text); len(rs) > assistantReportMax {
		text = string(rs[:assistantReportMax]) + "\n[… cut: a report keeps " + fmt.Sprint(assistantReportMax) + " characters]"
	}
	r := proto.AssistantReport{At: time.Now().UnixMilli(), Text: text, Waiting: proto.ReportWaits(text)}
	as := &d.asst
	as.mu.Lock()
	sa, err := as.findLocked(id)
	if err != nil {
		as.mu.Unlock()
		return r, err
	}
	id = sa.ID
	// The checks it followed: those last sent, since its last report.
	since := int64(0)
	if n := len(sa.Reports); n > 0 {
		since = sa.Reports[n-1].At
	}
	for _, c := range sa.Checks {
		if c.Last > since {
			r.Checks = append(r.Checks, c.ID)
		}
	}
	sa.Reports = append(sa.Reports, r)
	if n := len(sa.Reports); n > assistantReports {
		sa.Reports = slices.Clone(sa.Reports[n-assistantReports:])
	}
	d.saveAssistantsLocked()
	as.mu.Unlock()
	d.declareWaiting(id, r)
	d.changed()
	return r, nil
}

// declareWaiting declares the agent's pane of assistant id's session
// blocked with r's question, or takes that back for a report that waits
// for nothing.
func (d *Daemon) declareWaiting(id string, r proto.AssistantReport) {
	d.mu.Lock()
	s := d.assistantSessionLocked(id)
	var pn *pane
	if s != nil {
		if pn = prSessionLocked(s).agent; pn == nil {
			pn = focusedLocked(s)
		}
	}
	d.mu.Unlock()
	if pn == nil {
		return
	}
	rec := proto.Record{ID: waitingRecord, State: "clear", Progress: -1, Updated: r.At}
	if r.Waiting {
		q := strings.TrimSpace(strings.TrimLeft(r.Text, " \t\r\n*#_>"))
		q = strings.TrimSpace(q[len(proto.WaitingPrefix):])
		q, _, _ = strings.Cut(q, "\n")
		rec.State, rec.Kind, rec.Msg = "blocked", "question", neutralize(strings.TrimSpace(strings.Trim(q, "*_")))
	}
	pn.mu.Lock()
	if rec.State == "blocked" || pn.records[waitingRecord].State != "" {
		pn.applyReport(rec)
	}
	pn.mu.Unlock()
}

// assistantReports are an assistant's last reports, the latest first.
func (d *Daemon) assistantReports(p proto.AssistantParams) ([]proto.AssistantReport, error) {
	as := &d.asst
	as.mu.Lock()
	defer as.mu.Unlock()
	sa, err := as.findLocked(p.ID)
	if err != nil {
		return nil, err
	}
	out := slices.Clone(sa.Reports)
	slices.Reverse(out)
	if p.Limit > 0 && len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

// assistantLoop runs the assistants' checks as they come due, and those
// run asks for.
func (d *Daemon) assistantLoop() {
	as := &d.asst
	as.mu.Lock()
	as.initLocked()
	wake := as.wake
	as.mu.Unlock()
	t := time.NewTicker(assistantTick)
	defer t.Stop()
	for {
		select {
		case now := <-t.C:
			d.runDueChecks(now)
		case <-wake:
			d.runDueChecks(time.Now())
		case <-d.quit:
			return
		}
	}
}

// dueChecks are an assistant's checks to send now: those due, and those
// run asks for; forced tells run asked, with none of its checks due, as
// one with no checks.
type dueChecks struct {
	a      proto.Assistant
	checks []proto.AssistantCheck
	forced bool
}

// runDueChecks sends each assistant whose agent is at its prompt the
// checks due to it, as one message.
func (d *Daemon) runDueChecks(now time.Time) {
	if now.Sub(d.started) < assistantFirst {
		return
	}
	as := &d.asst
	as.mu.Lock()
	as.initLocked()
	var due []dueChecks
	for _, sa := range as.list {
		f := as.forced[sa.ID]
		dc := dueChecks{a: copyAssistant(sa.Assistant), forced: len(f) > 0}
		for _, c := range sa.Checks {
			if slices.Contains(f, "") || slices.Contains(f, c.ID) || !sa.Paused && assistant.Due(c, d.started, now) {
				dc.checks = append(dc.checks, c)
			}
		}
		if len(dc.checks) > 0 || dc.forced {
			due = append(due, dc)
		} else {
			delete(as.problem, sa.ID)
			delete(as.noAgent, sa.ID)
		}
	}
	as.mu.Unlock()
	changed := false
	for _, dc := range due {
		if d.sendChecks(dc, now) {
			changed = true
		}
	}
	if changed {
		d.changed()
	}
}

// sendChecks sends an assistant its due checks once its agent is at its
// prompt, starting its session first when it has none; it tells whether
// what the state shows changed.
func (d *Daemon) sendChecks(dc dueChecks, now time.Time) bool {
	id := dc.a.ID
	d.mu.Lock()
	s := d.assistantSessionLocked(id)
	sid := ""
	if s != nil {
		sid = s.id
	}
	d.mu.Unlock()
	if sid == "" {
		// Its session starts; its agent gets the checks at a later look,
		// once it is at its prompt.
		if _, err := d.startAssistant(id, false); err != nil {
			return d.assistantProblem(id, "its session would not start: "+err.Error())
		}
		return false
	}
	to, err := d.agentTarget(sid, "")
	if err != nil || to == "" {
		as := &d.asst
		as.mu.Lock()
		since, seen := as.noAgent[id]
		if !seen {
			as.noAgent[id] = now
			since = now
		}
		replace := now.Sub(since) >= assistantNoAgent && now.Sub(as.replaced[id]) >= assistantReplace
		if replace {
			as.replaced[id] = now
			delete(as.noAgent, id)
		}
		as.mu.Unlock()
		if replace {
			d.log.Printf("assistant %s: no agent runs in session %s; a fresh one replaces it", id, sid)
			if _, err := d.startAssistant(id, true); err != nil {
				return d.assistantProblem(id, "its session would not start: "+err.Error())
			}
		}
		if now.Sub(since) >= assistantNoAgent {
			return d.assistantProblem(id, "no agent runs in its session: start it again from its menu (New Chat)")
		}
		return false
	}
	d.mu.Lock()
	pn := d.findPaneLocked(to)
	d.mu.Unlock()
	if pn == nil {
		return false
	}
	pn.mu.Lock()
	ready := pn.agentReady()
	pn.mu.Unlock()
	if !ready {
		// Busy, or asking the user: its checks wait for it.
		return false
	}
	text := checksMessage(dc, d.started, now)
	if _, err := d.agentSend(proto.AgentSendParams{To: to, Label: assistantLabel, Text: text, Mode: "push"}); err != nil {
		return d.assistantProblem(id, "its checks could not reach its agent: "+err.Error())
	}
	as := &d.asst
	as.mu.Lock()
	defer as.mu.Unlock()
	delete(as.forced, id)
	delete(as.noAgent, id)
	delete(as.problem, id)
	for _, sa := range as.list {
		if sa.ID != id {
			continue
		}
		for i := range sa.Checks {
			if slices.ContainsFunc(dc.checks, func(c proto.AssistantCheck) bool { return c.ID == sa.Checks[i].ID }) {
				sa.Checks[i].Last = now.UnixMilli()
			}
		}
	}
	d.saveAssistantsLocked()
	d.log.Printf("assistant %s: %d checks sent", id, len(dc.checks))
	return true
}

// assistantProblem tells why an assistant's checks don't run, logged
// once; it tells whether that changed.
func (d *Daemon) assistantProblem(id, problem string) bool {
	as := &d.asst
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.problem[id] == problem {
		return false
	}
	as.problem[id] = problem
	d.log.Printf("assistant %s: %s", id, problem)
	return true
}

// checksMessage is the message of an assistant's due checks, as its agent
// reads it after "Kero: message from your schedule:".
func checksMessage(dc dueChecks, started, now time.Time) string {
	var b strings.Builder
	b.WriteString("Checks due now:\n")
	if len(dc.checks) == 0 {
		b.WriteString("- Run now, as the user asked: Do your usual check.\n")
	}
	for _, c := range dc.checks {
		when := assistant.CheckWhen(c)
		if c.Kind != proto.CheckStart {
			if at := assistant.DueAt(c, now); !at.IsZero() && assistant.Due(c, started, now) {
				when += ", due " + dayName(at, now) + " " + at.Format("15:04")
				switch {
				case at.Before(started):
					when += ", late because Kero was not running"
				case now.Sub(at) > 30*time.Minute:
					when += ", late because you were busy"
				}
			} else {
				when += ", run now as the user asked"
			}
		}
		fmt.Fprintf(&b, "- %s: %s\n", when, assistant.CheckText(c))
	}
	b.WriteString("\nDo them as your instructions say, then report with kero assistants report.")
	return b.String()
}

// dayName names the day of t from now's: "today", "yesterday", else its
// weekday, else its date.
func dayName(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	days := int(time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 7:
		return t.Weekday().String()
	}
	return t.Format("Jan 2")
}
