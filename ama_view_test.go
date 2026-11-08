package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// Ama's session is no row of the sidebar's and no count of Needs You: the
// frog at the sidebar's bottom tells of it, with a dot while it waits, and
// opens its window, Ama's terminal under New Chat, History and Clear,
// which closes again with Ama still there; the shortcut's action toggles
// it, and selecting Ama's session opens it in place of a pane.
func TestAmaFrogAndWindow(t *testing.T) {
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
			term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}, Font: a.paneFont("am"), Transparent: true})
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
			shot(t, tt, "ama-frog-"+side)
			tt.ClickAt(frog.X+frog.W/2, frog.Y+frog.H/2)
			tt.Frame()
			if !a.ama.open {
				t.Fatal("the frog opened nothing")
			}
			if a.selected != "s1" {
				t.Errorf("opening Ama selected %q", a.selected)
			}

			// Its window.
			// At 150% on the right's, as on a display of that scale.
			win := ui.NewTester(a.amaView, amaW, amaH)
			win.SetDark(side == "right")
			if side == "right" {
				win.SetScale(1.5)
			}
			win.Frame()
			win.Frame()
			closeLabel := a.tip(locales.CommonClose(), amaAction)
			for _, label := range []string{locales.AmaNewChat(), locales.AmaHistory(), closeLabel} {
				if _, ok := win.Find(label); !ok {
					t.Errorf("the window has no %s", label)
				}
			}
			shot(t, win, "ama-window-"+side)

			// Deleting chats asks first, in Ama's window, and the running
			// one stays.
			a.ama.history = []proto.Conversation{
				{Agent: "claude", ID: "c1", Title: "Close old sessions", Session: "ama1"},
				{Agent: "claude", ID: "c2", Title: "Which need me"},
				{Agent: "claude", ID: "c3", Title: "Start a worktree"},
			}
			a.askDeleteAmaChats(a.ama.history[1:])
			win.Frame()
			if _, ok := win.Find(locales.AmaDeleteAllTitle(2)); !ok {
				t.Fatalf("no confirmation in Ama's window; texts %q", win.Texts())
			}
			del, ok := win.Find(locales.EndedDelete())
			if !ok {
				t.Fatal("the confirmation has no Delete")
			}
			win.ClickAt(del.X+del.W/2, del.Y+del.H/2)
			win.Frame()
			if a.ama.confirm != nil || len(a.ama.history) != 1 || a.ama.history[0].ID != "c1" {
				t.Errorf("after Delete: confirm %v, history %+v", a.ama.confirm != nil, a.ama.history)
			}
			if c, ok := win.Find(closeLabel); ok {
				win.ClickAt(c.X+c.W/2, c.Y+c.H/2)
				win.Frame()
			}
			if a.ama.open || a.amaSession() == nil {
				t.Fatal("Close didn't close the window, or Ama went with it")
			}
			a.run(amaAction, nil)
			if !a.ama.open {
				t.Error("the action didn't open Ama")
			}
			a.run(amaAction, nil)
			if a.ama.open {
				t.Error("the action didn't close Ama")
			}
			a.selectSession("ama1")
			tt.Frame()
			if !a.ama.open || a.selected != "s1" {
				t.Errorf("selecting Ama: open %v, selected %q", a.ama.open, a.selected)
			}
		})
	}
}

// Ama's window keeps its place by the main window's corner nearest it, so
// that it stays by that corner as the main window moves or grows, and in
// the work area of a display: the one it is the most on, or the nearest
// across a gap that displays of different scales leave, as small as it
// has to be.
func TestAmaPlace(t *testing.T) {
	main := mygo.Rectangle{X: 100, Y: 100, Width: 1200, Height: 800}
	if got, want := defaultAmaPlace(true).rect(main), (mygo.Rectangle{X: 112, Y: 356, Width: amaW, Height: amaH}); got != want {
		t.Errorf("above the frog at the left: %v, want %v", got, want)
	}
	if got, want := defaultAmaPlace(false).rect(main), (mygo.Rectangle{X: 668, Y: 356, Width: amaW, Height: amaH}); got != want {
		t.Errorf("above the frog at the right: %v, want %v", got, want)
	}

	// By the top right corner: it stays there as the window grows.
	r := mygo.Rectangle{X: 900, Y: 150, Width: 400, Height: 300}
	p := placeOf(main, r)
	if want := (amaPlace{X: -400, Y: 50, Width: 400, Height: 300, Right: true}); p != want {
		t.Fatalf("place %+v, want %+v", p, want)
	}
	if got := p.rect(main); got != r {
		t.Errorf("back from its place: %v, want %v", got, r)
	}
	if got, want := p.rect(mygo.Rectangle{X: 100, Y: 100, Width: 1600, Height: 1000}), (mygo.Rectangle{X: 1300, Y: 150, Width: 400, Height: 300}); got != want {
		t.Errorf("the main window grown: %v, want %v", got, want)
	}
	// By the bottom left one, it moves down as the main window does.
	q := placeOf(main, mygo.Rectangle{X: 50, Y: 700, Width: 400, Height: 300})
	if !q.Bottom || q.Right || q.X != -50 || q.Y != -200 {
		t.Errorf("bottom left: %+v", q)
	}
	// A size under the window's smallest grows to it.
	if got := (amaPlace{Width: 10, Height: 10}).rect(main); got.Width != amaMinW || got.Height != amaMinH {
		t.Errorf("too small: %v", got)
	}

	// The displays: a 100% one, and a 150% one beyond a gap, as MyGo's
	// screen coordinates have it.
	areas := []mygo.Rectangle{{X: 0, Y: 0, Width: 1000, Height: 1000}, {X: 1200, Y: 0, Width: 800, Height: 600}}
	for _, c := range []struct {
		name   string
		in, to mygo.Rectangle
	}{
		{"on a display", mygo.Rectangle{X: 100, Y: 100, Width: 300, Height: 200}, mygo.Rectangle{X: 100, Y: 100, Width: 300, Height: 200}},
		{"over the right edge", mygo.Rectangle{X: 900, Y: 100, Width: 300, Height: 200}, mygo.Rectangle{X: 700, Y: 100, Width: 300, Height: 200}},
		{"mostly in the gap, on the second", mygo.Rectangle{X: 1050, Y: 100, Width: 300, Height: 200}, mygo.Rectangle{X: 1200, Y: 100, Width: 300, Height: 200}},
		{"in the gap, nearer the first", mygo.Rectangle{X: 1010, Y: 100, Width: 150, Height: 100}, mygo.Rectangle{X: 850, Y: 100, Width: 150, Height: 100}},
		{"below the second", mygo.Rectangle{X: 1500, Y: 900, Width: 300, Height: 200}, mygo.Rectangle{X: 1500, Y: 400, Width: 300, Height: 200}},
		{"where a display was", mygo.Rectangle{X: -3000, Y: 50, Width: 620, Height: 460}, mygo.Rectangle{X: 0, Y: 50, Width: 620, Height: 460}},
		{"bigger than the display", mygo.Rectangle{X: 1300, Y: 0, Width: 1000, Height: 700}, mygo.Rectangle{X: 1200, Y: 0, Width: 800, Height: 600}},
	} {
		if got := onScreen(c.in, areas); got != c.to {
			t.Errorf("%s: %v, want %v", c.name, got, c.to)
		}
	}
	if got := onScreen(r, nil); got != r {
		t.Errorf("without displays: %v", got)
	}
}

// Ama's terminal has a text size of its own: a point under the others',
// zooming with them, or ama-font-size; the panes' keep theirs.
func TestAmaFont(t *testing.T) {
	a := testApp(t)
	a.st.Sessions = append(a.st.Sessions, proto.Session{ID: "ama1", Ama: true, Panes: []proto.Pane{{ID: "am"}}, Focus: "am"})
	a.set.FontSize = 15
	if got := a.paneFont("am").Size; got != 14 {
		t.Errorf("automatic: %v, want 14", got)
	}
	if got := a.paneFont("p1").Size; got != 15 {
		t.Errorf("a pane's: %v, want 15", got)
	}
	a.set.AmaFontSize = 11
	if got := a.paneFont("am").Size; got != 11 {
		t.Errorf("ama-font-size 11: %v", got)
	}
	a.set.AmaFontSize, a.set.FontSize = 0, 8
	if got := a.paneFont("am").Size; got != 8 {
		t.Errorf("under the smallest: %v, want 8", got)
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
