package daemon

import (
	"os"
	"runtime"
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
	d.sendReports(now.Add(reportQuiet))
	setAgent(agent, proto.Idle)
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
	if rp == nil || !strings.Contains(strings.Join(rp.lines, "\n"), a.ID+` "alpha" closed`) {
		t.Errorf("closing a child: %+v", rp)
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
