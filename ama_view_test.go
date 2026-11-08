package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// Ama's session is no row of the sidebar's and no count of Needs You: the
// frog at the sidebar's bottom tells of it, with a dot while it waits, and
// opens its popover, Ama's terminal under New Chat, History and Clear,
// which closes again with Ama still there; the shortcut's action toggles
// it, and selecting Ama's session opens it in place of a pane.
func TestAmaFrogAndPopover(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads libghostty-vt")
	}
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"left", "right"} {
		t.Run(side, func(t *testing.T) {
			a := testApp(t)
			a.set.SessionsSide = side
			a.st.Sessions = append(a.st.Sessions, proto.Session{
				ID: "ama1", Ama: true, Name: "Ama",
				Pane:  proto.Pane{ID: "am", Status: proto.Waiting, Message: "Close these 3 sessions?", Progress: -1},
				Panes: []proto.Pane{{ID: "am", Status: proto.Waiting, Progress: -1}}, Focus: "am",
			})
			term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			term.Feed([]byte("\x1b[1m> Which sessions need me?\x1b[0m\r\n\r\n  demo · build   waiting   \"Approve the migration?\"\r\n\r\n  Close these 3 sessions? (y/n)\r\n"))
			a.terms["am"] = &termEntry{term: term}
			tt := ui.NewTester(a.view, 1100, 640)
			tt.SetDark(side == "right")
			tt.Frame()
			tt.Frame()
			if slices.Contains(a.visibleOrder(), "ama1") {
				t.Error("Ama's session shows as a sidebar row")
			}
			if _, ok := tt.Find(locales.FooterNeedsYou(1)); ok {
				t.Error("Needs You counts Ama")
			}
			frog, ok := tt.Find(locales.AmaTitle())
			if !ok {
				t.Fatalf("no frog; texts %q", tt.Texts())
			}
			shot(t, tt, "ama-closed-"+side)
			tt.ClickAt(frog.X+frog.W/2, frog.Y+frog.H/2)
			tt.Frame()
			tt.Frame()
			if !a.ama.open {
				t.Fatal("the frog opened nothing")
			}
			for _, label := range []string{locales.AmaNewChat(), locales.AmaHistory(), locales.AmaClear()} {
				if _, ok := tt.Find(label); !ok {
					t.Errorf("the popover has no %s", label)
				}
			}
			if a.selected != "s1" {
				t.Errorf("opening Ama selected %q", a.selected)
			}
			shot(t, tt, "ama-open-"+side)
			tt.ClickAt(frog.X+frog.W/2, frog.Y+frog.H/2)
			tt.Frame()
			if a.ama.open || a.amaSession() == nil {
				t.Fatal("the frog didn't close the popover, or Ama went with it")
			}
			a.run(amaAction, nil)
			tt.Frame()
			if !a.ama.open {
				t.Error("the action didn't open Ama")
			}
			a.run(amaAction, nil)
			a.selectSession("ama1")
			tt.Frame()
			if !a.ama.open || a.selected != "s1" {
				t.Errorf("selecting Ama: open %v, selected %q", a.ama.open, a.selected)
			}
		})
	}
}

// With Remote Control off, the app runs kero do's actions of Ama's alone,
// which kerod marks as it checks their secret, and of this machine's.
func TestAmaDrivesWithoutRemoteControl(t *testing.T) {
	a := testApp(t)
	a.set.RemoteControl = false
	h := a.local()
	a.event(h, h.cl, proto.Message{Event: "action", Action: "sidebar.toggle"})
	if a.prefs.SidebarHidden {
		t.Fatal("an agent's action ran without Remote Control")
	}
	a.event(h, h.cl, proto.Message{Event: "action", Action: "sidebar.toggle", Ama: true})
	if !a.prefs.SidebarHidden {
		t.Fatal("Ama's action didn't run")
	}
	if a.drives(&host{name: "build"}, true) {
		t.Error("another machine's call passed for Ama's")
	}
}
