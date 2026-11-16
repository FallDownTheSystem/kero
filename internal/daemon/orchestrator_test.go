package daemon

import (
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// quietSession starts a session whose shell runs nothing that writes.
func quietSession(t *testing.T, d *Daemon, p proto.CreateParams) proto.Session {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	if len(p.Command) == 0 {
		p.Command = []string{"sh", "-c", "exec sleep 60"}
	}
	s, err := d.create(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.removeSession(s.ID) })
	return s
}

func sessionOf(d *Daemon, id string) proto.Session {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessionInfo(d.sessionByIDLocked(id))
}

func makeOrchestrator(d *Daemon, id, goal string) {
	d.mu.Lock()
	s := d.sessionByIDLocked(id)
	s.orchestrator, s.goal = true, goal
	d.mu.Unlock()
}

// A session started from an orchestrator, or from one of its children,
// is that orchestrator's child, after its others, in its project; an
// explicit parent must be an orchestrator, and none makes no child. One
// level alone: an orchestrator is no child, nor a child an orchestrator.
// Closing an orchestrator leaves its children top-level sessions.
func TestOrchestratorLinks(t *testing.T) {
	d := testDaemon(t)
	o := quietSession(t, d, proto.CreateParams{})
	other := quietSession(t, d, proto.CreateParams{})
	makeOrchestrator(d, o.ID, "ship it")

	a := quietSession(t, d, proto.CreateParams{By: o.Panes[0].ID})
	if a.Parent != o.ID || a.Project != o.Project {
		t.Fatalf("started from the orchestrator: parent %q project %q", a.Parent, a.Project)
	}
	b := quietSession(t, d, proto.CreateParams{By: a.ID})
	if b.Parent != o.ID {
		t.Fatalf("started from a child: parent %q, not its orchestrator %s", b.Parent, o.ID)
	}
	if c := quietSession(t, d, proto.CreateParams{By: a.ID, Parent: proto.NoParent}); c.Parent != "" {
		t.Errorf("parent none: %q", c.Parent)
	}
	if c := quietSession(t, d, proto.CreateParams{By: other.ID}); c.Parent != "" {
		t.Errorf("started from an ordinary session: parent %q", c.Parent)
	}
	if c := quietSession(t, d, proto.CreateParams{Parent: o.ID}); c.Parent != o.ID {
		t.Errorf("explicit parent: %q", c.Parent)
	}
	if _, err := d.create(proto.CreateParams{Parent: other.ID, Command: []string{"true"}}); err == nil {
		t.Error("a child of a session that is no orchestrator")
	}
	if _, err := d.create(proto.CreateParams{Orchestrator: true, Parent: o.ID, Command: []string{"true"}}); err == nil {
		t.Error("an orchestrator made a child")
	}
	if _, err := d.setOrchestrator(proto.OrchestratorParams{ID: a.ID}); err == nil {
		t.Error("a child made an orchestrator")
	}

	// The children sit right after their orchestrator, whatever came
	// between.
	d.mu.Lock()
	var order []string
	for _, s := range d.sessions {
		order = append(order, s.id)
	}
	d.mu.Unlock()
	i := strings.Index(strings.Join(order, " "), o.ID+" "+a.ID+" "+b.ID)
	if i < 0 {
		t.Errorf("order %v: not %s, %s, %s together", order, o.ID, a.ID, b.ID)
	}

	// Its goal is told, and kept over a restart.
	if s := sessionOf(d, o.ID); !s.Orchestrator || s.Goal != "ship it" {
		t.Errorf("orchestrator %v goal %q", s.Orchestrator, s.Goal)
	}

	if err := d.removeSession(o.ID); err != nil {
		t.Fatal(err)
	}
	if s := sessionOf(d, a.ID); s.Parent != "" {
		t.Errorf("its orchestrator closed, the child keeps parent %q", s.Parent)
	}
}

// An orchestrator made an ordinary session again lets its children go.
func TestOrchestratorOff(t *testing.T) {
	d := testDaemon(t)
	o := quietSession(t, d, proto.CreateParams{})
	makeOrchestrator(d, o.ID, "")
	a := quietSession(t, d, proto.CreateParams{By: o.ID})
	if _, err := d.setOrchestrator(proto.OrchestratorParams{ID: o.ID, Off: true}); err != nil {
		t.Fatal(err)
	}
	if s := sessionOf(d, a.ID); s.Parent != "" {
		t.Errorf("parent %q of an orchestrator no more", s.Parent)
	}
	goal := "again"
	s, err := d.setOrchestrator(proto.OrchestratorParams{ID: o.ID, Goal: &goal})
	if err != nil || !s.Orchestrator || s.Goal != "again" {
		t.Errorf("made one again: %+v, %v", s.Goal, err)
	}
	// The link didn't come back with it.
	if s := sessionOf(d, a.ID); s.Parent != "" {
		t.Errorf("parent %q", s.Parent)
	}
}

// orchestrator-max-children refuses a child beyond it while those there
// run; one that exited leaves room.
func TestOrchestratorMaxChildren(t *testing.T) {
	d := testDaemon(t)
	if err := os.WriteFile(os.Getenv("KERO_CONFIG"), []byte("orchestrator-max-children = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := quietSession(t, d, proto.CreateParams{})
	makeOrchestrator(d, o.ID, "")
	a := quietSession(t, d, proto.CreateParams{By: o.ID})
	if _, err := d.create(proto.CreateParams{By: o.ID, Command: []string{"true"}}); err == nil || !strings.Contains(err.Error(), "orchestrator-max-children") {
		t.Fatalf("a second child: %v", err)
	}
	// Not a child, it may start.
	quietSession(t, d, proto.CreateParams{By: o.ID, Parent: proto.NoParent})
	pn := d.findPane(a.Panes[0].ID)
	pn.mu.Lock()
	pn.info.Status = proto.Exited
	pn.mu.Unlock()
	quietSession(t, d, proto.CreateParams{By: o.ID})
}

// The children's states reach their orchestrator's agent as one report,
// once each held a while: a flicker is no news, nor is a child's state
// as it joins; what comes close together goes as one message, with the
// summaries children send; a child its orchestrator closes isn't told
// of, one closed otherwise is.
func TestOrchestratorReports(t *testing.T) {
	enterWait = 50 * time.Millisecond
	t.Cleanup(func() { enterWait = 2 * time.Second })
	d := testDaemon(t)
	agent, out := agentSession(t, d)
	o := agent.sid
	makeOrchestrator(d, o, "")
	a := quietSession(t, d, proto.CreateParams{By: o, Name: "alpha"})
	b := quietSession(t, d, proto.CreateParams{By: o, Name: "beta"})
	c := quietSession(t, d, proto.CreateParams{By: o, Name: "gamma"})
	pa, pb := d.findPane(a.Panes[0].ID), d.findPane(b.Panes[0].ID)
	setAgent(pa, proto.Working)
	setAgent(pb, proto.Working)

	// alpha's own summary, at the real time, before the steps after it.
	if _, err := d.agentSend(proto.AgentSendParams{From: a.Panes[0].ID, Text: "Done: tests pass.", ToParent: true}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	step := func(dt time.Duration) {
		now = now.Add(dt)
		d.followChildren(now)
	}
	step(0) // they join, working: no news
	// alpha finishes its turn; beta flickers to waiting and back.
	setAgent(pa, proto.Idle)
	setAgent(pb, proto.Waiting)
	step(500 * time.Millisecond)
	step(time.Second) // a second and a half waiting: a flicker still
	setAgent(pb, proto.Working)
	step(time.Second)
	step(3 * time.Second)
	// beta comes to wait for good, with a question.
	pb.mu.Lock()
	pb.info.Status, pb.info.Kind, pb.info.Message = proto.Waiting, "question", "Which database?"
	pb.mu.Unlock()
	step(500 * time.Millisecond)
	step(3 * time.Second)
	// The orchestrator closes gamma itself: no word of it.
	if err := d.closeSession(proto.CloseParams{ID: c.ID, By: agent.info.ID}); err != nil {
		t.Fatal(err)
	}
	step(500 * time.Millisecond)

	d.sendReports(now) // the last line just came: not yet
	d.mail.mu.Lock()
	queued := len(d.mail.out[agent.info.ID])
	d.mail.mu.Unlock()
	if queued != 0 {
		t.Fatal("reported before it went quiet")
	}
	setAgent(agent, proto.Idle)
	d.sendReports(now.Add(reportQuiet))
	d.flushOutbox(time.Now())
	got := typedIn(t, out, "kero read")
	for _, want := range []string{
		"Kero: message from your child sessions:",
		a.ID + ` "alpha" finished its turn`,
		b.ID + ` "beta" waits for input (question): Which database?`,
		"Done: tests pass.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "Kero: message from") != 1 {
		t.Errorf("not one message:\n%s", got)
	}
	if strings.Count(got, `"beta"`) != 1 {
		t.Errorf("told of beta's flicker:\n%s", got)
	}
	if strings.Contains(got, "gamma") {
		t.Errorf("told of the child it closed itself:\n%s", got)
	}

	// Closed by another, a child is told of.
	if err := d.removeSession(a.ID); err != nil {
		t.Fatal(err)
	}
	step(time.Second)
	d.reports.mu.Lock()
	rp := d.reports.out[o]
	d.reports.mu.Unlock()
	if rp == nil || !strings.Contains(rp.text(), a.ID+` "alpha" closed`) {
		t.Errorf("closing a child: %+v", rp)
	}
}

// A child closed while news of it waits for its busy orchestrator takes
// that news with it: its state, as idle, is stale once it's closed, and
// the orchestrator hears only that it closed, or nothing when it closed
// it itself; what the child wrote with kero report --to-parent still goes.
func TestOrchestratorReportsClosedChild(t *testing.T) {
	enterWait = 50 * time.Millisecond
	t.Cleanup(func() { enterWait = 2 * time.Second })
	d := testDaemon(t)
	agent, out := agentSession(t, d) // working: not at its prompt
	o := agent.sid
	makeOrchestrator(d, o, "")
	a := quietSession(t, d, proto.CreateParams{By: o, Name: "alpha"})
	b := quietSession(t, d, proto.CreateParams{By: o, Name: "beta"})
	pa, pb := d.findPane(a.Panes[0].ID), d.findPane(b.Panes[0].ID)
	setAgent(pa, proto.Working)
	setAgent(pb, proto.Working)
	now := time.Now()
	step := func(dt time.Duration) {
		now = now.Add(dt)
		d.followChildren(now)
	}
	step(0)
	if _, err := d.agentSend(proto.AgentSendParams{From: a.Panes[0].ID, Text: "Done: tests pass.", ToParent: true}); err != nil {
		t.Fatal(err)
	}
	// Both finish their turns, and the report falls due while the
	// orchestrator still works.
	setAgent(pa, proto.Idle)
	setAgent(pb, proto.Idle)
	step(0)
	step(3 * time.Second)
	step(reportQuiet)
	d.sendReports(now)
	// The orchestrator closes alpha itself; the user closes beta.
	if err := d.closeSession(proto.CloseParams{ID: a.ID, By: agent.info.ID}); err != nil {
		t.Fatal(err)
	}
	if err := d.removeSession(b.ID); err != nil {
		t.Fatal(err)
	}
	step(time.Second)
	// Its turn over, the orchestrator hears what is left.
	setAgent(agent, proto.Idle)
	d.sendReports(now.Add(reportQuiet))
	d.flushOutbox(time.Now())
	got := typedIn(t, out, "kero read")
	for _, want := range []string{"Done: tests pass.", b.ID + ` "beta" closed`} {
		if !strings.Contains(got, want) {
			t.Errorf("the report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "finished its turn") {
		t.Errorf("told of a closed child's idle:\n%s", got)
	}
}

// A session that is no child has no parent to report to.
func TestReportToParentNoParent(t *testing.T) {
	d := testDaemon(t)
	s := quietSession(t, d, proto.CreateParams{})
	if _, err := d.agentSend(proto.AgentSendParams{From: s.Panes[0].ID, Text: "hi", ToParent: true}); err == nil {
		t.Error("reported to no parent")
	}
}

// A child that waits again after its first question was answered is news
// again, however briefly it worked in between; one that only flickers to
// working and back while nobody answers is not.
func TestOrchestratorReportsAgain(t *testing.T) {
	d := testDaemon(t)
	o := quietSession(t, d, proto.CreateParams{})
	makeOrchestrator(d, o.ID, "")
	a := quietSession(t, d, proto.CreateParams{By: o.ID, Name: "alpha"})
	pa := d.findPane(a.Panes[0].ID)
	setAgent(pa, proto.Working)
	now := time.Now()
	step := func(st proto.Status, dt time.Duration) {
		if st != "" {
			setAgent(pa, st)
		}
		now = now.Add(dt)
		d.followChildren(now)
	}
	waits := func() int {
		d.reports.mu.Lock()
		defer d.reports.mu.Unlock()
		if rp := d.reports.out[o.ID]; rp != nil {
			return strings.Count(rp.text(), "waits for input")
		}
		return 0
	}
	step("", 0)
	step(proto.Waiting, 0)
	step("", 3*time.Second)
	if n := waits(); n != 1 {
		t.Fatalf("%d waits told, not 1", n)
	}
	// Answered: it works a second, and asks again.
	step(proto.Working, 0)
	step("", time.Second)
	step(proto.Waiting, 0)
	step("", 3*time.Second)
	if n := waits(); n != 2 {
		t.Errorf("the second question: %d waits told, not 2", n)
	}
}

// From inside an orchestrator, kero new -p of another project starts no
// child; --parent keeps the child in its orchestrator's project.
func TestOrchestratorOtherProject(t *testing.T) {
	d := testDaemon(t)
	d.projects = append(d.projects, proto.Project{ID: "other", Name: "other", Dir: t.TempDir()})
	o := quietSession(t, d, proto.CreateParams{Project: "pr"})
	makeOrchestrator(d, o.ID, "")
	if s := quietSession(t, d, proto.CreateParams{By: o.ID, Project: "other"}); s.Parent != "" || s.Project != "other" {
		t.Errorf("-p other from the orchestrator: parent %q project %q", s.Parent, s.Project)
	}
	if s := quietSession(t, d, proto.CreateParams{By: o.ID, Project: "pr"}); s.Parent != o.ID {
		t.Errorf("-p of its own project: parent %q", s.Parent)
	}
	if s := quietSession(t, d, proto.CreateParams{Parent: o.ID, Project: "other"}); s.Parent != o.ID || s.Project != "pr" {
		t.Errorf("--parent and -p other: parent %q project %q", s.Parent, s.Project)
	}
}

// Children still starting count towards orchestrator-max-children, as
// several kero new at once are.
func TestOrchestratorMaxChildrenStarting(t *testing.T) {
	d := testDaemon(t)
	if err := os.WriteFile(os.Getenv("KERO_CONFIG"), []byte("orchestrator-max-children = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := quietSession(t, d, proto.CreateParams{})
	makeOrchestrator(d, o.ID, "")
	d.mu.Lock()
	d.sessionByIDLocked(o.ID).starting = 1
	d.mu.Unlock()
	if _, err := d.create(proto.CreateParams{By: o.ID, Command: []string{"true"}}); err == nil {
		t.Fatal("a child past the limit while another starts")
	}
	d.mu.Lock()
	d.sessionByIDLocked(o.ID).starting = 0
	d.mu.Unlock()
	quietSession(t, d, proto.CreateParams{By: o.ID})
	d.mu.Lock()
	n := d.sessionByIDLocked(o.ID).starting
	d.mu.Unlock()
	if n != 0 {
		t.Errorf("starting %d once it started", n)
	}
}

// A report its orchestrator's agent can't take yet is kept for it, with
// what comes meanwhile; one of an orchestrator gone goes with it.
func TestOrchestratorReportKept(t *testing.T) {
	d := testDaemon(t)
	o := quietSession(t, d, proto.CreateParams{}) // runs no agent Kero knows
	makeOrchestrator(d, o.ID, "")
	now := time.Now()
	d.reports.mu.Lock()
	d.reports.initLocked()
	d.reports.addLocked(o.ID, "", "- first", now)
	d.reports.mu.Unlock()
	d.sendReports(now.Add(reportQuiet))
	d.reports.mu.Lock()
	rp := d.reports.out[o.ID]
	d.reports.mu.Unlock()
	if rp == nil || len(rp.lines) != 1 {
		t.Fatalf("not kept: %+v", rp)
	}
	d.removeSession(o.ID)
	d.sendReports(now.Add(2 * reportQuiet))
	d.reports.mu.Lock()
	rp = d.reports.out[o.ID]
	d.reports.mu.Unlock()
	if rp != nil {
		t.Errorf("kept for an orchestrator gone: %+v", rp)
	}
}

// A child's conversation resumed is a child again of the orchestrator it
// was a child of, while that is open; --parent none makes it none's, and
// one resumed from an orchestrator, as kero new, its child.
func TestResumeAsChild(t *testing.T) {
	d := testDaemon(t)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	d.projects = append(d.projects, proto.Project{ID: "other", Name: "other", Dir: t.TempDir()})
	o := quietSession(t, d, proto.CreateParams{Project: "other"})
	makeOrchestrator(d, o.ID, "")
	conv := func(id string, p proto.CreateParams) {
		t.Helper()
		s := quietSession(t, d, p)
		pn := d.findPane(s.Panes[0].ID)
		pn.mu.Lock()
		pn.info.AgentSession = &proto.AgentSession{Agent: "claude", ID: id}
		pn.mu.Unlock()
		d.conversations()
		// Gone, as in a crash none of it came back from.
		if err := d.removeSession(s.ID); err != nil {
			t.Fatal(err)
		}
	}
	resume := func(id string, p proto.ConversationParams) proto.Session {
		t.Helper()
		p.Agent, p.ID = "claude", id
		s, err := d.resumeConversation(p)
		if err != nil {
			t.Fatal(err)
		}
		d.conversations()
		if err := d.removeSession(s.ID); err != nil {
			t.Fatal(err)
		}
		return s
	}
	conv("kid", proto.CreateParams{By: o.ID, Dir: d.projects[0].Dir})
	conv("solo", proto.CreateParams{Dir: d.projects[0].Dir})
	if s := resume("kid", proto.ConversationParams{}); s.Parent != o.ID || s.Project != "other" {
		t.Errorf("resumed a child as parent %q, project %q", s.Parent, s.Project)
	}
	if s := resume("solo", proto.ConversationParams{}); s.Parent != "" {
		t.Errorf("resumed a top-level session as %q's child", s.Parent)
	}
	if s := resume("kid", proto.ConversationParams{Parent: proto.NoParent}); s.Parent != "" {
		t.Errorf("--parent none: a child of %q", s.Parent)
	}
	// Resumed none's above, it is remembered so.
	if s := resume("kid", proto.ConversationParams{}); s.Parent != "" {
		t.Errorf("resumed as %q's child after it was none's", s.Parent)
	}
	if s := resume("solo", proto.ConversationParams{By: o.Panes[0].ID}); s.Parent != o.ID {
		t.Errorf("resumed from the orchestrator as %q's child", s.Parent)
	}
	// Its orchestrator closed, a child resumes top-level.
	d.removeSession(o.ID)
	if s := resume("solo", proto.ConversationParams{}); s.Parent != "" {
		t.Errorf("a child of %q, closed", s.Parent)
	}
}

// kero parent makes an open session a child of an orchestrator, after
// its others and in its project, or of none; one level and
// orchestrator-max-children hold.
func TestSetParent(t *testing.T) {
	d := testDaemon(t)
	d.projects = append(d.projects, proto.Project{ID: "other", Name: "other", Dir: t.TempDir()})
	o := quietSession(t, d, proto.CreateParams{Project: "other"})
	makeOrchestrator(d, o.ID, "")
	k := quietSession(t, d, proto.CreateParams{By: o.ID})
	tail := quietSession(t, d, proto.CreateParams{Project: "other"})
	x := quietSession(t, d, proto.CreateParams{})
	s, err := d.setParent(proto.ParentParams{ID: x.ID, By: k.Panes[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if s.Parent != o.ID || s.Project != "other" {
		t.Errorf("adopted: parent %q, project %q", s.Parent, s.Project)
	}
	d.mu.Lock()
	var order []string
	for _, s := range d.sessions {
		order = append(order, s.id)
	}
	d.mu.Unlock()
	if i := slices.Index(order, x.ID); i < 1 || order[i-1] != k.ID || order[i+1] != tail.ID {
		t.Errorf("order %v: not after %s, its orchestrator's last child", order, k.ID)
	}
	if s, err := d.setParent(proto.ParentParams{ID: x.ID, Parent: proto.NoParent}); err != nil || s.Parent != "" {
		t.Errorf("none: %q, %v", s.Parent, err)
	}
	if _, err := d.setParent(proto.ParentParams{ID: o.ID, By: k.Panes[0].ID}); err == nil {
		t.Error("an orchestrator became a child")
	}
	if _, err := d.setParent(proto.ParentParams{ID: x.ID, Parent: tail.ID}); err == nil {
		t.Error("a child of no orchestrator")
	}
	if err := os.WriteFile(os.Getenv("KERO_CONFIG"), []byte("orchestrator-max-children = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.setParent(proto.ParentParams{ID: x.ID, Parent: o.ID}); err == nil {
		t.Error("past orchestrator-max-children")
	}
}

// Restored after kerod restarts, as after a crash, a child is its
// orchestrator's child still.
func TestRestoreKeepsChild(t *testing.T) {
	d := testDaemon(t)
	o := quietSession(t, d, proto.CreateParams{})
	makeOrchestrator(d, o.ID, "")
	k := quietSession(t, d, proto.CreateParams{By: o.ID})
	d.saveState()
	r := testDaemon(t)
	r.dir = d.dir
	r.restore()
	t.Cleanup(func() {
		for _, id := range []string{o.ID, k.ID} {
			r.removeSession(id)
		}
	})
	r.mu.Lock()
	restored := r.sessionByIDLocked(k.ID) != nil && r.sessionByIDLocked(o.ID) != nil
	r.mu.Unlock()
	if !restored {
		t.Fatal("not restored")
	}
	if s := sessionOf(r, k.ID); s.Parent != o.ID {
		t.Errorf("restored with parent %q, not %s", s.Parent, o.ID)
	}
}
