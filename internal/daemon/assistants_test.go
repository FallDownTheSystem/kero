package daemon

import (
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kero/internal/assistant"
	"kero/internal/proto"
)

func str(s string) *string { return &s }

// newAssistant makes an assistant whose agent is sleep, which writes
// nothing.
func newAssistant(t *testing.T, d *Daemon, name string, checks ...proto.AssistantCheck) proto.Assistant {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sleep")
	}
	e := proto.AssistantEdit{Name: str(name), Role: str("Keeps " + name + " in order"), Agent: str("sleep"), AgentArgs: str("60")}
	if checks != nil {
		e.Checks = &checks
	}
	a, err := d.createAssistant(e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.mu.Lock()
		var ids []string
		for _, s := range d.sessions {
			ids = append(ids, s.id)
		}
		d.mu.Unlock()
		for _, id := range ids {
			d.removeSession(id)
		}
	})
	return a
}

// Two starts at once, the schedule's and a click's, make one session; a
// fresh one replaces it. Its panes have the assistant's id, it is of no
// project and named after the assistant, and the state shows it.
func TestAssistantSession(t *testing.T) {
	d := testDaemon(t)
	a := newAssistant(t, d, "Jira Buddy")
	if a.ID != "jira-buddy" || a.Dir != assistant.Dir("jira-buddy") {
		t.Fatalf("id %q dir %q", a.ID, a.Dir)
	}
	var wg sync.WaitGroup
	got := make([]proto.Session, 4)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := d.startAssistant("Jira buddy", false)
			if err != nil {
				t.Error(err)
			}
			got[i] = s
		}()
	}
	wg.Wait()
	for _, s := range got[1:] {
		if s.ID != got[0].ID {
			t.Fatalf("two starts made two sessions: %s and %s", got[0].ID, s.ID)
		}
	}
	s := got[0]
	if s.Assistant != a.ID || s.Project != "" || s.Name != "Jira Buddy" {
		t.Errorf("session %+v", s)
	}
	if env := d.environment(s.ID, s.Panes[0].ID, "/bin/sh", nil); !slices.Contains(env, "KERO_ASSISTANT=jira-buddy") {
		t.Error("its panes have no $KERO_ASSISTANT")
	}
	if env := d.environment("other", "p", "/bin/sh", []string{"KERO_ASSISTANT=x"}); slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "KERO_ASSISTANT=") }) {
		t.Error("another session's pane has $KERO_ASSISTANT")
	}
	fresh, err := d.startAssistant(a.ID, true)
	if err != nil || fresh.ID == s.ID {
		t.Fatalf("a fresh chat: %v, %s", err, fresh.ID)
	}
	st := d.state()
	n := 0
	for _, o := range st.Sessions {
		if o.Assistant == a.ID {
			n++
		}
	}
	if n != 1 || len(st.Assistants) != 1 || st.Assistants[0].Session != fresh.ID {
		t.Fatalf("%d sessions of the assistant; state %+v", n, st.Assistants)
	}
	// Renamed, its session is too.
	if _, err := d.updateAssistant(proto.AssistantEdit{ID: a.ID, Name: str("Tickets")}); err != nil {
		t.Fatal(err)
	}
	if got := sessionOf(d, fresh.ID).Name; got != "Tickets" {
		t.Errorf("its session is named %q", got)
	}
	if _, err := d.createAssistant(proto.AssistantEdit{Name: str("tickets"), Role: str("x")}); err == nil {
		t.Error("two assistants of one name")
	}
}

// An assistant's session leads children in any project: they keep the
// project they name, stay its children when moved, and need one; it is
// no one's child itself, nor an orchestrator.
func TestAssistantChildren(t *testing.T) {
	d := testDaemon(t)
	newAssistant(t, d, "Branches")
	s, err := d.startAssistant("branches", false)
	if err != nil {
		t.Fatal(err)
	}
	d.projects = append(d.projects, proto.Project{ID: "pr2", Name: "other", Dir: t.TempDir()})
	kid := quietSession(t, d, proto.CreateParams{By: s.Panes[0].ID, Project: "pr2"})
	if kid.Parent != s.ID || kid.Project != "pr2" {
		t.Fatalf("child: parent %q project %q", kid.Parent, kid.Project)
	}
	// A child's kero new makes another of the assistant's.
	kid2 := quietSession(t, d, proto.CreateParams{By: kid.ID, Project: "pr"})
	if kid2.Parent != s.ID || kid2.Project != "pr" {
		t.Fatalf("a child's child: parent %q project %q", kid2.Parent, kid2.Project)
	}
	if _, err := d.create(proto.CreateParams{By: s.Panes[0].ID, Dir: assistant.Dir("branches"), Command: []string{"true"}}); err == nil {
		t.Error("a child in the assistant's folder")
	}
	if err := d.move(proto.MoveParams{ID: kid.ID, Project: "pr", Index: -1}); err != nil {
		t.Fatal(err)
	}
	if got := sessionOf(d, kid.ID); got.Parent != s.ID || got.Project != "pr" {
		t.Errorf("moved: parent %q project %q", got.Parent, got.Project)
	}
	other := quietSession(t, d, proto.CreateParams{Project: "pr2", Parent: proto.NoParent})
	if _, err := d.setParent(proto.ParentParams{ID: other.ID, Parent: s.ID}); err != nil {
		t.Fatal(err)
	}
	if got := sessionOf(d, other.ID); got.Parent != s.ID || got.Project != "pr2" {
		t.Errorf("adopted: parent %q project %q", got.Parent, got.Project)
	}
	if _, err := d.setParent(proto.ParentParams{ID: s.ID, Parent: other.ID}); err == nil {
		t.Error("the assistant's session made a child")
	}
	if _, err := d.setOrchestrator(proto.OrchestratorParams{ID: s.ID}); err == nil {
		t.Error("the assistant's session made an orchestrator")
	}
	// Its children's news goes to it, as to an orchestrator.
	d.followChildren(time.Now())
	d.reports.mu.Lock()
	k := d.reports.kids[kid.ID]
	d.reports.mu.Unlock()
	if k == nil || k.parent != s.ID {
		t.Errorf("the reports don't follow its child: %+v", k)
	}
}

// A report is kept, the last ten; one that starts "Waiting on you:"
// declares the assistant's pane waiting on the question, which the next
// report or the user's answer takes back. kero report --to-parent from
// its session reports so too.
func TestAssistantReport(t *testing.T) {
	d := testDaemon(t)
	newAssistant(t, d, "Mail")
	s, err := d.startAssistant("mail", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.agentSend(proto.AgentSendParams{From: s.Panes[0].ID, ToParent: true,
		Text: "**Waiting on you:** Reply to Anna's offer?\nResult: three new mails."}); err != nil {
		t.Fatal(err)
	}
	p := sessionOf(d, s.ID).Pane
	if p.Status != proto.Waiting || p.Message != "Reply to Anna's offer?" {
		t.Fatalf("after a waiting report: %s %q", p.Status, p.Message)
	}
	st := d.state()
	if r := st.Assistants[0].LastReport; r == nil || !r.Waiting {
		t.Fatalf("last report %+v", r)
	}
	d.findPane(s.Panes[0].ID).acted()
	if p := sessionOf(d, s.ID).Pane; p.Status == proto.Waiting {
		t.Error("the user's answer left it waiting")
	}
	if _, err := d.assistantReport(proto.AssistantReportParams{Assistant: "mail", Text: "**Waiting on you:** Again?"}); err != nil {
		t.Fatal(err)
	}
	for i := range 11 {
		if _, err := d.assistantReport(proto.AssistantReportParams{From: s.ID, Text: strings.Repeat("x", 5000) + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	if p := sessionOf(d, s.ID).Pane; p.Status == proto.Waiting {
		t.Error("a report that waits for nothing left it waiting")
	}
	rs, err := d.assistantReports(proto.AssistantParams{ID: "mail", Limit: 20})
	if err != nil || len(rs) != assistantReports {
		t.Fatalf("%d reports kept, %v", len(rs), err)
	}
	if len([]rune(rs[0].Text)) > assistantReportMax+100 || !strings.HasPrefix(rs[0].Text, "xxx") {
		t.Errorf("the latest report, first, clipped: %d characters", len(rs[0].Text))
	}
	if n := len([]rune(d.state().Assistants[0].LastReport.Text)); n > assistantClip+1 {
		t.Errorf("the state carries %d characters of the last report", n)
	}
	// It goes to disk, and comes back.
	d2 := testDaemon(t)
	d2.dir = d.dir
	d2.loadAssistants()
	if got, _ := d2.assistantReports(proto.AssistantParams{ID: "mail"}); len(got) != assistantReports {
		t.Errorf("%d reports after a restart", len(got))
	}
}

// Editing a check keeps when it last ran unless when it runs changed;
// a new one starts now, as if added, and gets the next free id.
func TestAssistantEditChecks(t *testing.T) {
	d := testDaemon(t)
	a := newAssistant(t, d, "Weekly",
		proto.AssistantCheck{Kind: "daily", Time: "09:00"},
		proto.AssistantCheck{Kind: "weekly", Time: "10:00", Days: []int{1}})
	d.asst.mu.Lock()
	d.asst.list[0].Checks[0].Last, d.asst.list[0].Checks[1].Last = 111, 222
	d.asst.list[0].Checks[0].Created, d.asst.list[0].Checks[1].Created = 1, 1
	d.asst.mu.Unlock()
	checks := []proto.AssistantCheck{
		{ID: "c1", Kind: "daily", Time: "09:00", Text: "new words"},
		{ID: "c2", Kind: "weekly", Time: "11:00", Days: []int{1}},
		{Kind: "start"},
	}
	got, err := d.updateAssistant(proto.AssistantEdit{ID: a.ID, Checks: &checks})
	if err != nil {
		t.Fatal(err)
	}
	if c := got.Checks[0]; c.Last != 111 || c.Created != 1 || c.Text != "new words" {
		t.Errorf("a check whose words changed: %+v", c)
	}
	if c := got.Checks[1]; c.Last != 222 || c.Created == 1 {
		t.Errorf("a check whose time changed: %+v", c)
	}
	if c := got.Checks[2]; c.ID != "c3" || c.Last != 0 {
		t.Errorf("a new check: %+v", c)
	}
	bad := []proto.AssistantCheck{{Kind: "weekly", Time: "10:00"}}
	if _, err := d.updateAssistant(proto.AssistantEdit{ID: a.ID, Checks: &bad}); err == nil {
		t.Error("a weekly check of no day")
	}
}

// The checks' message tells when each was due and why one is late.
func TestChecksMessage(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	now := time.Date(2026, 11, 23, 9, 2, 0, 0, loc) // a Monday
	started := time.Date(2026, 11, 23, 9, 1, 0, 0, loc)
	dc := dueChecks{checks: []proto.AssistantCheck{
		{Kind: "daily", Time: "09:00", Text: "Check my open Jira tickets."},
		{Kind: "weekly", Time: "09:00", Days: []int{0}, Text: "List merged branches."},
		{Kind: "start"},
	}}
	got := checksMessage(dc, started, now)
	for _, want := range []string{
		"- Daily at 09:00, due today 09:00, late because Kero was not running: Check my open Jira tickets.",
		"- Weekly on Sunday at 09:00, due yesterday 09:00, late because Kero was not running: List merged branches.",
		"- When Kero starts: Do your usual check.",
		"report with kero assistants report",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
}

// A check due while kerod was down is due once the next kerod loads
// assistants.json, and once it ran, as its Last says, not again, by that
// kerod or the one after.
func TestAssistantCatchUpAcrossRestart(t *testing.T) {
	d := testDaemon(t)
	newAssistant(t, d, "Morning", proto.AssistantCheck{Kind: "daily", Time: "09:00"})
	twoDays := time.Now().Add(-48 * time.Hour).UnixMilli()
	d.asst.mu.Lock()
	d.asst.list[0].Checks[0].Created, d.asst.list[0].Checks[0].Last = twoDays-1, twoDays
	d.saveAssistantsLocked()
	d.asst.mu.Unlock()

	due := func(d *Daemon) bool {
		d.asst.mu.Lock()
		defer d.asst.mu.Unlock()
		return assistant.Due(d.asst.list[0].Checks[0], d.started, time.Now())
	}
	// kerod was down past this morning's 09:00 and yesterday's: the next
	// one runs the check once.
	d2 := testDaemon(t)
	d2.dir, d2.started = d.dir, time.Now()
	d2.loadAssistants()
	if !due(d2) {
		t.Fatal("a check missed while kerod was down isn't due after it starts")
	}
	// It ran: sendChecks sets its Last and saves.
	d2.asst.mu.Lock()
	d2.asst.list[0].Checks[0].Last = time.Now().UnixMilli()
	d2.saveAssistantsLocked()
	d2.asst.mu.Unlock()
	if due(d2) {
		t.Error("the check is due again in the same run")
	}
	d3 := testDaemon(t)
	d3.dir, d3.started = d.dir, time.Now()
	d3.loadAssistants()
	if due(d3) {
		t.Error("the check is due again after another restart")
	}
}

// An assistant's color is a host color's name or a custom #rrggbb, which
// a save and load of assistants.json keep as it is.
func TestAssistantColor(t *testing.T) {
	d := testDaemon(t)
	a := newAssistant(t, d, "Painter")
	for _, c := range []string{"#b5179e", "teal"} {
		if _, err := d.updateAssistant(proto.AssistantEdit{ID: a.ID, Color: str(c)}); err != nil {
			t.Errorf("color %q: %v", c, err)
		}
	}
	for _, c := range []string{"#12", "#gggggg", "mauve", "#B5179E"} {
		if _, err := d.updateAssistant(proto.AssistantEdit{ID: a.ID, Color: str(c)}); err == nil {
			t.Errorf("color %q was taken", c)
		}
	}
	if _, err := d.updateAssistant(proto.AssistantEdit{ID: a.ID, Color: str("#b5179e")}); err != nil {
		t.Fatal(err)
	}
	d2 := testDaemon(t)
	d2.dir = d.dir
	d2.loadAssistants()
	d2.asst.mu.Lock()
	defer d2.asst.mu.Unlock()
	if len(d2.asst.list) != 1 || d2.asst.list[0].Color != "#b5179e" {
		t.Errorf("after a load: %+v", d2.asst.list)
	}
}
