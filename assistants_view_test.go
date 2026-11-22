package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// assistantApp is an app with three assistants: Jira, waiting on the
// user, in Work; Mail, idle, in Work, with a child in project demo; and
// Branches, of no group, with no session, paused.
func assistantApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	now := time.Now()
	sess := func(id, asst, name string, st proto.Status, msg string) proto.Session {
		p := proto.Pane{ID: id + "-p", Shell: "fish", Dir: "/tmp", Status: st, Message: msg, Progress: -1, Agent: "claude"}
		return proto.Session{ID: id, Name: name, Assistant: asst, Panes: []proto.Pane{p}, Focus: p.ID, Pane: p}
	}
	jira := sess("aj", "jira", "Jira", proto.Waiting, "Close PROJ-41 as a duplicate?")
	mail := sess("am", "mail", "Mail", proto.Idle, "")
	kid := sess("kc", "", "fix flaky login test", proto.Working, "Running the test suite")
	kid.Project, kid.Parent, kid.Pane.Agent = "p1", "am", "claude"
	st := proto.State{Projects: a.st.Projects, Sessions: append(slices.Clone(a.st.Sessions), jira, mail, kid)}
	st.Assistants = []proto.Assistant{
		{ID: "jira", Name: "Jira", Color: "blue", Avatar: "bolt", Group: "Work", Role: "Keeps my Jira tickets moving", Session: "aj",
			Checks: []proto.AssistantCheck{{ID: "c1", Kind: "daily", Time: "09:00"}}, NextCheck: now.Add(3 * time.Hour).UnixMilli()},
		{ID: "mail", Name: "Mail", Color: "pink", Avatar: "mail", Group: "Work", Role: "Reads my inbox", Session: "am",
			LastReport: &proto.AssistantReport{At: now.Add(-time.Hour).UnixMilli(), Text: "Result: Two mails need replies, both drafted."},
			NextCheck:  now.Add(5 * time.Hour).UnixMilli()},
		{ID: "branches", Name: "Branches", Color: "green", Role: "Cleans up merged branches", Paused: true},
	}
	// As the local kerod's state message brings them (setState).
	a.mergeState(a.local(), st)
	return a
}

// The Assistants view lists the assistants by group, those of none
// first, each with its last word and next check, the child of one under
// it; the toggle counts those that need the user, and the session view
// lists none of their sessions.
func TestAssistantsView(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := assistantApp(t)
		tt := ui.NewTester(a.view, 1100, 640)
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(time.Second) // colors ease from the light frame's
		tt.Frame()
		if slices.Contains(a.visibleOrder(), "aj") || slices.Contains(a.visibleOrder(), "am") {
			t.Error("the Sessions view lists an assistant's session")
		}
		views, ok := tt.Find(locales.SidebarViewAssistants())
		if !ok {
			t.Fatalf("no toggle; texts %q", tt.Texts())
		}
		tt.ClickAt(views.X+views.W/2, views.Y+views.H/2)
		tt.Frame()
		tt.Frame()
		if !a.assistantsShown() {
			t.Fatal("the toggle didn't switch views")
		}
		for _, want := range []string{"Jira", "Mail", "Branches", "Work", "Close PROJ-41 as a duplicate?", "Two mails need replies, both drafted.",
			locales.AssistantsPaused(), locales.AssistantsWhileRunning(), "1"} {
			if _, ok := tt.Find(want); !ok {
				t.Errorf("no %q in the view; texts %q", want, tt.Texts())
			}
		}
		if got := a.visibleOrder(); !slices.Equal(got, []string{"aj", "am", "kc"}) {
			t.Errorf("order %v", got)
		}
		if rowY(t, tt, "Branches") > rowY(t, tt, "Work") {
			t.Error("an assistant of no group comes after a group")
		}
		if rowY(t, tt, "fix flaky login test") < rowY(t, tt, "Mail") {
			t.Error("the child isn't under its assistant")
		}
		name := "assistants-view-light"
		if dark {
			name = "assistants-view-dark"
		}
		shot(t, tt, name)
		// Selecting an assistant's session from elsewhere shows its view.
		a.prefs.View = ""
		a.selectSession("am")
		if !a.assistantsShown() {
			t.Error("selecting an assistant's session left the Sessions view")
		}
	}
}

// The dialog: a new assistant starts with a daily check; an edit shows
// the assistant's fields and checks, the kinds as Daily, Weekly, When
// Kero starts.
func TestAssistantForm(t *testing.T) {
	a := assistantApp(t)
	a.openAssistantForm("")
	if f := a.asstForm; f == nil || len(f.checks) != 1 || f.checks[0].kind != proto.CheckDaily {
		t.Fatalf("a new assistant's form: %+v", a.asstForm)
	}
	a.asstForm = nil
	a.st.Assistants[0].Checks = append(a.st.Assistants[0].Checks, proto.AssistantCheck{ID: "c2", Kind: "weekly", Time: "10:00", Days: []int{1, 4}, Text: "List branches merged two weeks ago"})
	a.st.Assistants[0].Tracks = "My open tickets in PROJ, and what moved since yesterday.\nAsk before closing any."
	a.openAssistantForm("jira")
	tt := ui.NewTester(a.view, 1100, 900)
	tt.Frame()
	tt.Frame()
	for _, want := range []string{locales.AssistantFormEdit("Jira"), "Keeps my Jira tickets moving", locales.CheckKindDaily(), locales.CheckKindWeekly(),
		locales.AssistantFormChecksNote(), locales.AssistantFormSave()} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("no %q in the dialog; texts %q", want, tt.Texts())
		}
	}
	if got := a.asstForm.edit(); len(*got.Checks) != 2 || !slices.Equal((*got.Checks)[1].Days, []int{1, 4}) || *got.Name != "Jira" {
		t.Errorf("the dialog's edit: %+v", got)
	}
	shot(t, tt, "assistant-dialog")
	if !slices.Equal(checkKinds, []string{proto.CheckDaily, proto.CheckWeekly, proto.CheckStart}) {
		t.Errorf("kinds %v", checkKinds)
	}
}

// A custom color: the well shows it and opens its picker; a named swatch
// chooses that name again, and the well keeps the custom color.
func TestAssistantFormCustomColor(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := assistantApp(t)
		a.st.Assistants[1].Color = "#b5179e"
		a.openAssistantForm("mail")
		tt := ui.NewTester(a.view, 1100, 900)
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(time.Second) // colors ease from the light frame's
		tt.Frame()
		if info, ok := tt.Access(locales.AssistantFormCustomColor()); !ok || info.Value != "#b5179e" {
			t.Fatalf("the well: %+v %v", info, ok)
		}
		if err := tt.Click(locales.AssistantFormCustomColor()); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Frame()
		name := "assistant-color-light"
		if dark {
			name = "assistant-color-dark"
		}
		shot(t, tt, name)
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if err := tt.Click(colorLabel("teal")); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if f := a.asstForm; f.color != "teal" || f.custom != ui.Hex("#b5179e") {
			t.Errorf("after teal: %q %v", f.color, f.custom)
		}
	}
}

// An assistant notifies once of a report that waits for nothing, as it
// finished a check; once, as needing the user, of one that waits; once of
// a check's turn that ended with no report; and not at all of a turn that
// only answered a child.
func TestAssistantNotifications(t *testing.T) {
	a := assistantApp(t)
	a.set.NotifyDone, a.set.NotifyWaiting = true, true
	var shown []string
	old := showNote
	showNote = func(o mygo.NotificationOptions, _ func()) { shown = append(shown, o.Title+" | "+o.Body) }
	t.Cleanup(func() { showNote = old })
	mail := func() *proto.Assistant { return a.assistantByID("mail") }
	sess := func() *proto.Session { return a.session("am") }
	setStatus := func(st proto.Status, unread bool) {
		s := sess()
		s.Status, s.Pane.Status, s.Unread, s.Panes[0].Status = st, st, unread, st
	}
	a.notifySessions(true)
	step := func(name string, want ...string) {
		t.Helper()
		shown = nil
		a.notifySessions(false)
		if !slices.Equal(shown, want) {
			t.Errorf("%s: notified %q, want %q", name, shown, want)
		}
	}
	done := locales.AssistantNotifyDone("Mail")

	// A check goes to it, it works and reports.
	mail().Checks = []proto.AssistantCheck{{ID: "c1", Kind: "daily", Time: "09:00", Last: time.Now().UnixMilli()}}
	setStatus(proto.Working, false)
	step("working on a check")
	mail().LastReport = &proto.AssistantReport{At: time.Now().UnixMilli() + 1, Text: "**Result:** Three new mails, none urgent.\nNot done: nothing"}
	step("a report", done+" | Three new mails, none urgent.")
	setStatus(proto.Idle, true)
	step("its turn ends after the report")
	step("again")

	// A report that waits notifies as waiting, once.
	setStatus(proto.Working, false)
	step("working")
	mail().LastReport = &proto.AssistantReport{At: time.Now().UnixMilli() + 2, Text: "Waiting on you: Reply to Anna?", Waiting: true}
	setStatus(proto.Waiting, true)
	sess().Message = "Reply to Anna?"
	step("a waiting report", locales.NotifyNeedsYouTitle("Mail")+" | Reply to Anna?")

	// A check whose turn ends with no report.
	setStatus(proto.Idle, false)
	sess().Message = ""
	step("answered")
	mail().Checks[0].Last = time.Now().UnixMilli() + 10
	setStatus(proto.Working, false)
	step("a second check")
	setStatus(proto.Idle, true)
	step("its turn ends with no report", done+" | "+locales.AssistantNotifyNoReport())

	// A turn that only answers a child's report.
	setStatus(proto.Working, false)
	step("a child's report")
	setStatus(proto.Idle, true)
	step("its turn ends")
}

func TestReportResult(t *testing.T) {
	for in, want := range map[string]string{
		"Result: All quiet.\nNot done: nothing":      "All quiet.",
		"**Result:**\n- Merged 3 branches\nWhere: -": "Merged 3 branches",
		"Nothing changed since yesterday.":           "Nothing changed since yesterday.",
	} {
		if got := reportResult(in); got != want {
			t.Errorf("reportResult(%q) = %q, want %q", strings.SplitN(in, "\n", 2)[0], got, want)
		}
	}
}

// The local kerod's state brings the assistants; another host's neither
// brings nor clears them, and its sessions of an assistant, or one whose
// assistant is gone, list as any other session.
func TestAssistantsFromState(t *testing.T) {
	a := testApp(t)
	st := proto.State{Projects: a.st.Projects, Sessions: append(slices.Clone(a.st.Sessions),
		proto.Session{ID: "aj", Name: "Jira", Assistant: "jira", Panes: []proto.Pane{{ID: "aj-p"}}},
		proto.Session{ID: "gone", Name: "Old", Assistant: "old", Panes: []proto.Pane{{ID: "gone-p"}}}),
		Assistants: []proto.Assistant{{ID: "jira", Name: "Jira", Role: "x", Session: "aj"}}}
	a.mergeState(a.local(), st)
	if a.assistantByID("jira") == nil {
		t.Fatal("the local state's assistants weren't taken")
	}
	a.prefs.View = viewAssistants
	if got := a.visibleOrder(); !slices.Contains(got, "aj") {
		t.Errorf("the Assistants view lists %v", got)
	}
	a.prefs.View = ""
	if got := a.visibleOrder(); slices.Contains(got, "aj") || !slices.Contains(got, "gone") {
		t.Errorf("the Sessions view lists %v", got)
	}
	a.syncHosts()
	if a.assistantByID("jira") == nil {
		t.Error("the hosts' change dropped the assistants")
	}
}

// Jump to Attention reaches a waiting assistant; selecting an ordinary
// session leaves the Assistants view, an assistant's child doesn't.
func TestAssistantJumpAndView(t *testing.T) {
	a := assistantApp(t)
	a.selected = "s1"
	a.jumpToAttention()
	if a.selected != "aj" || !a.assistantsShown() {
		t.Fatalf("jumped to %q, view %q", a.selected, a.prefs.View)
	}
	a.selectSession("kc")
	if !a.assistantsShown() {
		t.Error("an assistant's child left the Assistants view")
	}
	a.selectSession("s1")
	if a.assistantsShown() {
		t.Error("an ordinary session stayed in the Assistants view")
	}
}
