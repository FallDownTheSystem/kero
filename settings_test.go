package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/settings"
)

// Every page builds, and shows its settings.
func TestSettingsPages(t *testing.T) {
	a := testApp(t)
	a.ss.draft = map[string]string{}
	tt := ui.NewTester(a.settingsView, 820, 640)
	want := map[string]string{
		"general": "Restore sessions after a restart", "appearance": "Translucent window", "terminal": "Line height",
		"shell": "Scrollback", "notifications": "A session needs me", "keyboard": "Split Right",
		"agents": "Let commands drive this window", "advanced": "Background service",
	}
	for _, p := range settingsPages {
		if p.id == "notifications" {
			continue // asks the system whether it can notify: needs the app
		}
		a.ss.page = p.id
		tt.Frame()
		if !tt.HasText(want[p.id]) {
			t.Errorf("page %s: no %q; texts %q", p.id, want[p.id], tt.Texts())
		}
		shot(t, tt, "settings-"+p.id)
	}
}

// A shortcut is typed into its chip, saved, and takes the keys from the
// action that had them.
func TestRecordShortcut(t *testing.T) {
	a := testApp(t)
	a.ss.draft = map[string]string{}
	a.ss.page = "keyboard"
	tt := ui.NewTester(a.settingsView, 820, 900)
	tt.Frame()
	if err := tt.Click("Shortcut for Split Right"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Super|ui.Shift, ui.KeyD) // Split Down's
	tt.Frame()
	tt.Frame()
	if got := a.set.Shortcuts["pane.split.right"]; got != "CmdOrCtrl+Shift+D" {
		t.Fatalf("Split Right's shortcut %q", got)
	}
	if got, ok := a.set.Shortcuts["pane.split.down"]; !ok || got != "" {
		t.Errorf("Split Down kept its keys: %q", got)
	}
	if !tt.HasText("⇧⌘D was Split Down’s: it has none now.") {
		t.Errorf("no word of the conflict; texts %q", tt.Texts())
	}
	if saved := settings.Load(); saved.Shortcuts["pane.split.right"] != "CmdOrCtrl+Shift+D" {
		t.Errorf("not saved: %v", saved.Shortcuts)
	}
	shot(t, tt, "settings-keyboard-recorded")
}

// A choice made in a segmented control stays, and is saved.
func TestSegmentedChoiceSticks(t *testing.T) {
	a := testApp(t)
	a.ss.page = "appearance"
	tt := ui.NewTester(a.settingsView, 820, 640)
	tt.Frame()
	if err := tt.Click("Light"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	if a.set.Appearance != "light" {
		t.Fatalf("appearance %q after choosing Light", a.set.Appearance)
	}
	if got := settings.Load().Appearance; got != "light" {
		t.Fatalf("saved appearance %q", got)
	}
	if err := tt.Click("System"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	if a.set.Appearance != "system" || settings.Load().Appearance != "system" {
		t.Fatalf("appearance %q, saved %q, after choosing System", a.set.Appearance, settings.Load().Appearance)
	}
}

// Dark pages, and the plain background, for a look ($KERO_SHOTS).
func TestDarkShots(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.settingsView, 820, 640)
	tt.SetDark(true)
	for _, p := range []string{"advanced", "terminal", "appearance"} {
		a.ss.page = p
		tt.Frame()
		shot(t, tt, "dark-"+p)
	}
	a.set.Background = "plain"
	a.applied = a.set.Clone()
	main := ui.NewTester(a.view, 1000, 560)
	main.SetDark(true)
	main.Frame()
	main.Advance(time.Second) // colors ease from the light frame's
	main.Frame()
	shot(t, main, "dark-plain-main")
}

// The segmented control, light and dark, for a look ($KERO_SHOTS).
func TestSegmentedShots(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := testApp(t)
		a.ss.page = "appearance"
		a.set.Appearance = "light"
		a.applied = a.set.Clone()
		tt := ui.NewTester(a.settingsView, 820, 240)
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		name := "segmented-light"
		if dark {
			name = "segmented-dark"
		}
		shot(t, tt, name)
	}
}

// The pop-up button with its menu open, and a focused field, light and
// dark ($KERO_SHOTS).
func TestControlShots(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := testApp(t)
		a.ss.page = "shell"
		tt := ui.NewTester(a.settingsView, 820, 520)
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(time.Second)
		if err := tt.Click("4 MB"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if !tt.HasText("64 MB") {
			t.Fatalf("the scrollback menu did not open; texts %q", tt.Texts())
		}
		name := map[bool]string{false: "light", true: "dark"}[dark]
		shot(t, tt, "controls-menu-"+name)
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if a.set.Scrollback != 16 {
			t.Errorf("chose scrollback %d, want 16", a.set.Scrollback)
		}
		if err := tt.Click("NAME=value"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		shot(t, tt, "controls-field-"+name)
	}
}
