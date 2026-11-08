package main

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// treeApp is the app with an orchestrator, "boss", whose two children
// kerod keeps apart from it, after an ordinary session.
func treeApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	row := func(id, name string) proto.Session {
		return proto.Session{ID: id, Project: "p1", Name: name, Panes: []proto.Pane{{ID: id + "-p", Shell: "fish", Dir: "/tmp", Status: proto.Idle, Progress: -1}},
			Focus: id + "-p", Pane: proto.Pane{ID: id + "-p", Status: proto.Idle, Progress: -1, Dir: "/tmp"}}
	}
	boss := row("o", "boss")
	boss.Orchestrator, boss.Goal = true, "Ship the login page"
	k1, k2 := row("k1", "worker one"), row("k2", "worker two")
	k1.Parent, k2.Parent = "o", "o"
	k2.Status, k2.Panes[0].Status, k2.Message = proto.Waiting, proto.Waiting, "Which database?"
	// kerod's order: a child before its orchestrator, another after a
	// session of none.
	a.st.Sessions = append(a.st.Sessions, k1, boss, row("x", "solo"), k2)
	return a
}

func rowY(t *testing.T, tt *ui.Tester, text string) float32 {
	t.Helper()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("no %q; texts %q", text, tt.Texts())
	}
	return r.Y
}

// An orchestrator's children show right under it, each after a ↳; its
// chevron folds them, which Kero remembers, the row then counting those
// that need the user; the session shown stays in view.
func TestOrchestratorSidebar(t *testing.T) {
	for _, mode := range []string{"detailed", "compact"} {
		t.Run(mode, func(t *testing.T) {
			a := treeApp(t)
			a.set.SidebarRows = mode
			a.session("o").Since = time.Now().UnixMilli()
			tt := ui.NewTester(a.view, 1100, 640)
			tt.Frame()
			tt.Advance(time.Second)
			tt.Frame()
			boss, one, two, solo := rowY(t, tt, "boss"), rowY(t, tt, "worker one"), rowY(t, tt, "worker two"), rowY(t, tt, "solo")
			if !(boss < one && one < two && two < solo) {
				t.Errorf("rows at boss %v, one %v, two %v, solo %v: not the tree", boss, one, two, solo)
			}
			if n := countText(tt.Texts(), "↳"); n != 2 {
				t.Errorf("%d ↳ marks, not 2", n)
			}
			shot(t, tt, "orchestrator-"+mode)
			if got := a.visibleOrder(); !slices.Equal(got, []string{"s1", "o", "k1", "k2", "x"}) {
				t.Errorf("order %v", got)
			}

			fold, ok := tt.Find(locales.SidebarHideChildren())
			if !ok {
				t.Fatal("no chevron to fold the children")
			}
			// A detailed row has the mark at the end of its second line,
			// under the time; a compact one, on its one line.
			name, _ := tt.Find("boss")
			now, ok := tt.Find(locales.AgoNow())
			if !ok {
				t.Fatalf("no time; texts %q", tt.Texts())
			}
			below := fold.Y >= name.Y+name.H
			if below != (mode == "detailed") {
				t.Errorf("chevron at y %v, the name's line %v–%v", fold.Y, name.Y, name.Y+name.H)
			}
			if mode == "detailed" && fold.X+fold.W < now.X+now.W {
				t.Errorf("chevron ends at x %v, left of the time's end %v", fold.X+fold.W, now.X+now.W)
			}
			tt.ClickAt(fold.X+fold.W/2, fold.Y+fold.H/2)
			tt.Frame()
			tt.Frame()
			if !a.prefs.Folded["o"] {
				t.Fatal("folding is not remembered")
			}
			if _, ok := tt.Find("worker one"); ok {
				t.Error("folded, a child shows")
			}
			// The waiting child is counted on the folded row.
			if _, ok := tt.Find(locales.SidebarShowChildren()); !ok {
				t.Fatal("no chevron to unfold")
			}
			if _, ok := tt.Find("1"); !ok {
				t.Errorf("the folded row doesn't count the child that needs you; texts %q", tt.Texts())
			}
			shot(t, tt, "orchestrator-folded-"+mode)
			if got := a.visibleOrder(); !slices.Equal(got, []string{"s1", "o", "x"}) {
				t.Errorf("folded order %v", got)
			}
			a.selectSession("k2")
			tt.Frame()
			if _, ok := tt.Find("worker two"); !ok {
				t.Error("the child shown left the sidebar as its orchestrator folded")
			}
		})
	}
}

// A child's waiting notifies the user only with notify-children; the
// orchestrator's own always does, as any session's.
func TestOrchestratorChildrenMuted(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run(fmt.Sprint(on), func(t *testing.T) {
			a := treeApp(t)
			a.set.NotifyChildren, a.set.NotifyWaiting = on, true
			var shown []string
			old := showNote
			showNote = func(o mygo.NotificationOptions, _ func()) { shown = append(shown, o.ID) }
			t.Cleanup(func() { showNote = old })
			a.notifySessions(true)
			for _, id := range []string{"k1", "o"} {
				s := a.session(id)
				s.Status = proto.Waiting
			}
			want, badge := []string{"session/o"}, 1
			if on {
				// k2, waiting from the start, counts too.
				want, badge = []string{"session/k1", "session/o"}, 3
			}
			if n := a.notifySessions(false); n != badge {
				t.Errorf("the badge counts %d, not %d", n, badge)
			}
			slices.Sort(shown)
			if !slices.Equal(shown, want) {
				t.Errorf("notified %v, want %v", shown, want)
			}
		})
	}
}

// Closing an orchestrator asks what of its children: Close All closes
// them, Keep Sessions leaves them.
func TestCloseOrchestrator(t *testing.T) {
	for _, all := range []bool{true, false} {
		t.Run(fmt.Sprint(all), func(t *testing.T) {
			a := treeApp(t)
			a.closeSession("o", true)
			if a.confirm == nil || a.confirm.alt == "" {
				t.Fatal("closing an orchestrator didn't ask of its children")
			}
			if all {
				a.confirm.run()
			} else {
				a.confirm.altRun()
			}
			if a.session("o") != nil {
				t.Error("the orchestrator stays")
			}
			kept := a.session("k1") != nil && a.session("k2") != nil
			if kept == all {
				t.Errorf("close all %v: children kept %v", all, kept)
			}
			if a.session("x") == nil {
				t.Error("closed a session of none")
			}
		})
	}
}

// The New Session dialog's Orchestrator starts the agent chosen as an
// orchestrator of its goal, in no worktree.
func TestNewOrchestratorParams(t *testing.T) {
	w := &newSessionAsk{kind: kindOrchestrator, agent: "claude --yolo", goal: "  Ship it  ", worktree: true}
	p, ok := w.params("")
	if !ok || !p.Orchestrator || p.Goal != "Ship it" || !slices.Equal(p.Command, []string{"claude", "--yolo"}) || p.Worktree != "" {
		t.Errorf("params %+v, %v", p, ok)
	}
}

// The dialog's Orchestrator has a Goal, and no worktree option.
func TestNewOrchestratorDialog(t *testing.T) {
	a := testApp(t)
	a.newAsk = &newSessionAsk{open: true, host: a.local(), p: proto.CreateParams{Project: "p1"}, dir: "/tmp", kind: kindOrchestrator, agent: "claude", gitErr: "none"}
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find(locales.NewSessionGoal()); !ok {
		t.Fatalf("no Goal; texts %q", tt.Texts())
	}
	if tt.HasText(locales.NewSessionWorktree()) {
		t.Error("an orchestrator offers a worktree")
	}
	shot(t, tt, "orchestrator-dialog")
}
