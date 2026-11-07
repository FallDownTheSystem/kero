package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/settings"
)

// A project's worktree cleanup is the rules for all until the user picks
// another: Custom starts its own as those, which then change apart from
// them; Off keeps its worktrees; Inherit forgets its own.
func TestProjectWorktreeCleanup(t *testing.T) {
	a := testApp(t)
	a.ss.draft = map[string]string{}
	a.ss.page = "git"
	a.set.WorktreeCleanup = settings.WorktreeCleanup{On: true, InactiveDays: 14, Merged: true}
	tt := ui.NewTester(a.settingsView, 820, 2600)
	tt.Frame()
	shot(t, tt, "worktree-cleanup-on")
	key := settings.CleanupKey("/tmp")
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatalf("%s: %v; texts %q", s, err, tt.Texts())
		}
		tt.Frame()
		tt.Frame()
	}
	click("Custom")
	if w, ok := a.set.ProjectCleanup[key]; !ok || w != a.set.WorktreeCleanup {
		t.Fatalf("custom: %+v, %v; want %+v", w, ok, a.set.WorktreeCleanup)
	}
	tt.Advance(time.Second) // the toggles' knobs move
	tt.Frame()
	shot(t, tt, "worktree-cleanup-custom")
	a.set.ProjectCleanup[key] = settings.WorktreeCleanup{On: true, Closed: true}
	tt.Frame()
	if a.ss.cleanup.rules != a.set.ProjectCleanup[key] {
		t.Errorf("the controls keep %+v over the settings' %+v", a.ss.cleanup.rules, a.set.ProjectCleanup[key])
	}
	if a.set.WorktreeCleanup.Closed {
		t.Error("the project's rule changed all projects'")
	}
	// The page's first Off is pull requests' notifications: this one is
	// between Inherit and Custom.
	in, _ := tt.Find("Inherit")
	cu, _ := tt.Find("Custom")
	tt.ClickAt((in.X+in.W+cu.X)/2, in.Y+in.H/2)
	tt.Frame()
	tt.Frame()
	shot(t, tt, "worktree-cleanup-off")
	if w, ok := a.set.ProjectCleanup[key]; !ok || w.On {
		t.Fatalf("off: %+v, %v", w, ok)
	}
	click("Inherit")
	if _, ok := a.set.ProjectCleanup[key]; ok {
		t.Fatal("inherit keeps the project's own")
	}
}
