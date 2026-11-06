package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"kero/internal/actions"
	"kero/internal/proto"

	"kero/locales"
)

// testApp is the app with a session of two panes side by side, a | b,
// and no kerod.
func testApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("KERO_HOME", home)
	// The configuration too: never the user's.
	t.Setenv("KERO_CONFIG", filepath.Join(home, "config"))
	a := newApp()
	t.Cleanup(func() {
		if a.ss.preview != nil {
			a.ss.preview.Close()
		}
	})
	var l *proto.Layout
	a.st = proto.State{
		Projects: []proto.Project{{ID: "p1", Name: "demo", Dir: "/tmp"}},
		Sessions: []proto.Session{{
			ID: "s1", Project: "p1", Name: "demo",
			Panes: []proto.Pane{
				{ID: "a", Shell: "fish", Dir: "/tmp", Status: proto.Idle, Progress: -1},
				{ID: "b", Shell: "zsh", Dir: "/tmp", Status: proto.Idle, Progress: -1},
			},
			Layout: l.Split("a", "b", "right", 0),
			Focus:  "a",
		}},
	}
	a.loaded, a.selected = true, "s1"
	a.local().loaded = true
	return a
}

// shot writes the tester's frame to $KERO_SHOTS, to look at.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("KERO_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func testDropApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	a.local().conn = connReady
	a.terms["a"], a.terms["b"] = &termEntry{}, &termEntry{}
	a.local().cl, _ = projectClient(t, func(method string, _ json.RawMessage) (any, error) {
		return nil, fmt.Errorf("test daemon rejected %s", method)
	})
	return a
}

func TestSidebarReceivesFolderDrops(t *testing.T) {
	a := testDropApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	for _, at := range []ui.Point{{X: 10, Y: 30}, {X: 40, Y: 80}, {X: 40, Y: 120}, {X: 100, Y: 400}, {X: 20, Y: 620}} {
		if !tt.DropFiles(at.X, at.Y, t.TempDir()) {
			t.Errorf("sidebar drop at %v was not received", at)
		}
	}
	if tt.DropFiles(a.prefs.SidebarWidth+3, 400, t.TempDir()) {
		t.Error("sidebar received a drop outside its visible bounds")
	}
	shot(t, tt, "sidebar-folder-drop-light")
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "sidebar-folder-drop-dark")
}

func TestSidebarRejectsUnavailableDrops(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*App)
	}{
		{"hidden", func(a *App) { a.prefs.SidebarHidden = true }},
		{"connecting", func(a *App) { a.local().conn = connConnecting }},
		{"disconnected", func(a *App) { a.local().conn = connFailed }},
		{"no client", func(a *App) { a.local().cl = nil }},
		{"palette", func(a *App) { a.pal.open = true }},
		{"folder picker", func(a *App) { a.picker = folderPicker{open: true, host: a.local()} }},
		{"only remote connected", func(a *App) {
			h := &host{name: "remote", conn: connReady, cl: a.local().cl, loaded: true}
			a.hosts = append(a.hosts, h)
			a.owner = map[string]*host{"s1": h, "p1": h, "a": h, "b": h}
			a.local().conn, a.local().cl = connFailed, nil
		}},
		{"confirmation", func(a *App) { a.confirm = &confirmation{open: true, title: "Confirm", action: "Close"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testDropApp(t)
			tt := ui.NewTester(a.view, 1100, 640)
			tt.Frame()
			tc.set(a)
			tt.Frame()
			if tt.DropFiles(30, 400, t.TempDir()) {
				t.Errorf("sidebar received a drop while %s", tc.name)
			}
		})
	}
}

func TestSidebarLocalDropWithRemoteSelected(t *testing.T) {
	a := testDropApp(t)
	h := &host{name: "remote", conn: connFailed, loaded: true}
	a.hosts = append(a.hosts, h)
	a.owner = map[string]*host{"s1": h, "p1": h, "a": h, "b": h}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	if !tt.DropFiles(30, 400, t.TempDir()) {
		t.Error("local sidebar drop was blocked by the selected remote host")
	}
}

func TestSidebarKeepsTerminalDrops(t *testing.T) {
	a := testDropApp(t)
	term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	a.terms["a"] = &termEntry{term: term}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	if !tt.DropFiles(400, 340, filepath.Join(t.TempDir(), "image.png")) {
		t.Error("terminal did not receive a file drop while the sidebar target was active")
	}
}

// Files from another app open beside a terminal dropped along its left,
// right or top edge, a folder in a terminal; at its middle and bottom the
// terminal takes them, as it pastes their paths at a prompt.
func TestPaneTakesFilesFromOtherApps(t *testing.T) {
	a := testDropApp(t)
	splits := make(chan proto.CreateParams, 4)
	a.local().cl, _ = projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method == "split" {
			var p proto.CreateParams
			json.Unmarshal(raw, &p)
			splits <- p
			return proto.Pane{}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	for _, id := range []string{"a", "b"} {
		term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { term.Close() })
		a.terms[id] = &termEntry{term: term}
	}
	var card ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		card = a.paneRef("card-b").Bounds(c)
	}, 1280, 720)
	tt.Frame()
	file, dir := filepath.Join(t.TempDir(), "notes.md"), t.TempDir()
	if err := os.WriteFile(file, []byte("# notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	split := func(what string) (proto.CreateParams, bool) {
		t.Helper()
		select {
		case p := <-splits:
			return p, true
		case <-time.After(time.Second):
		}
		return proto.CreateParams{}, false
	}

	// Pane a's controls say Split Right too: the preview is another one.
	previews := func() (n int) {
		for _, s := range tt.Texts() {
			if s == "Split Right" || s == "Split Left" || s == "Split Up" {
				n++
			}
		}
		return n - 1
	}
	tt.DragFiles(card.X+card.W-20, card.Y+card.H/2)
	if previews() != 1 {
		t.Errorf("no preview of a split at the right edge; texts %q", tt.Texts())
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	shot(t, tt, "os-drop-right")
	tt.DropFiles(card.X+card.W-20, card.Y+card.H/2, file)
	if p, ok := split("file"); !ok || p.Split != "b" || p.Direction != "right" || p.File != file {
		t.Errorf("file dropped at the right edge split %+v, %v", p, ok)
	}
	tt.DropFiles(card.X+15, card.Y+card.H/2, dir)
	if p, ok := split("folder"); !ok || p.Split != "b" || p.Direction != "left" || p.Dir != dir || p.File != "" {
		t.Errorf("folder dropped at the left edge split %+v, %v", p, ok)
	}
	for _, at := range [][2]float32{{card.X + card.W/2, card.Y + card.H/2}, {card.X + card.W/2, card.Y + card.H - 20}} {
		tt.DragFiles(at[0], at[1])
		if previews() != 0 {
			t.Errorf("a split previewed over the terminal at %v: %q", at, tt.Texts())
		}
		if !tt.DropFiles(at[0], at[1], file) {
			t.Errorf("the terminal did not take the drop at %v", at)
		}
		if p, ok := split("paste"); ok {
			t.Errorf("a drop on the terminal at %v split %+v", at, p)
		}
	}
}

func TestDragPaneToSide(t *testing.T) {
	a := testDropApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	grip, ok := tt.Find("zsh")
	if !ok {
		t.Fatalf("no title of pane b; texts %q", tt.Texts())
	}
	// Pick pane b up by its title, and carry it over a's left edge.
	x, y := grip.X+grip.W/2, grip.Y+grip.H/2
	ar, _ := tt.Find("fish") // pane a's title
	tx, ty := ar.X+10, float32(340)
	tt.Press(x, y)
	for i := 1; i <= 12; i++ {
		f := float32(i) / 12
		tt.Move(x+(tx-x)*f, y+(ty-y)*f)
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("Split Left") {
		t.Errorf("no preview of a split on the left; texts %q", tt.Texts())
	}
	shot(t, tt, "drag-left")

	// Over a's middle: the panes would swap.
	tt.Move(ar.X+120, 340)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("Swap Panes") {
		t.Errorf("no preview of a swap; texts %q", tt.Texts())
	}
	shot(t, tt, "drag-swap")

	tt.Release(ar.X+120, 340)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if got := a.st.Sessions[0].Layout.Panes(); !slices.Equal(got, []string{"b", "a"}) {
		t.Fatalf("after the drop, panes %v, want b then a", got)
	}
	if a.focusOf(&a.st.Sessions[0]) != "b" {
		t.Errorf("the pane moved has not the focus")
	}
	shot(t, tt, "dropped")
}

// The sidebar's animation moves the header's title one way only, in
// whole pixels: a title that steps back and forth shakes.
func TestSidebarAnimationMonotonic(t *testing.T) {
	bar := ui.TitleBar{Left: 78, Height: 60}
	for _, width := range []float32{200, 268, 333, 440} {
		prevX := float32(-1)
		prevEdge := float32(-1)
		for i := 0; i <= 20000; i++ {
			ch := chromeFor(float32(i)/20000, width, bar, false)
			x := ch.sideW + ch.mainPad + ch.headerPad // the title's left, in the window
			edge := ch.sideW + ch.mainPad             // the card's left edge
			for _, v := range []float32{ch.sideW, ch.mainPad, ch.headerPad} {
				if v != float32(int(v)) {
					t.Fatalf("width %v, progress %v: %v is not a whole pixel", width, ch.shown, v)
				}
			}
			if x < prevX {
				t.Fatalf("width %v, step %d: the title moved back, %v to %v", width, i, prevX, x)
			}
			if edge < prevEdge {
				t.Fatalf("width %v, step %d: the card's edge moved back, %v to %v", width, i, prevEdge, edge)
			}
			prevX, prevEdge = x, edge
		}
		if ch := chromeFor(1, width, bar, false); ch.headerPad != 16 || ch.mainPad != 0 || ch.sideW != width {
			t.Errorf("width %v shown: %+v", width, ch)
		}
		if ch := chromeFor(0, width, bar, false); ch.sideW != 0 || ch.mainPad != cardGap {
			t.Errorf("width %v hidden: %+v", width, ch)
		}
	}
}

// With the sidebar at the window's right, the header at the left edge
// keeps its room for the traffic lights and the panel's button as the
// sidebar shows and hides, which only the right's padding follows.
func TestSidebarRightChrome(t *testing.T) {
	bar := ui.TitleBar{Left: 78, Height: 60}
	hidden := chromeFor(0, 268, bar, false)
	for _, p := range []float32{0, 0.3, 1} {
		ch := chromeFor(p, 268, bar, true)
		// One button where the sidebar has two.
		if ch.headerPad != hidden.headerPad-sideButtonsW(1) || ch.buttonsLeft(bar) != hidden.buttonsLeft(bar) {
			t.Errorf("progress %v: %+v, want the left edge as with the sidebar hidden, less a button, %+v", p, ch, hidden)
		}
	}
	if ch := chromeFor(1, 268, bar, true); ch.mainPad != 0 || ch.sideW != 268 || ch.cardsRight() {
		t.Errorf("shown: %+v", ch)
	}
	if ch := chromeFor(0, 268, bar, true); ch.mainPad != cardGap || !ch.cardsRight() {
		t.Errorf("hidden: %+v", ch)
	}
}

// The sidebar's animation never holds still before it lands: a frame
// that does not move, then one that does, reads as a stutter.
func TestSidebarAnimationNoHolds(t *testing.T) {
	ease := sidebarMotion.Ease
	frames := int(sidebarMotion.Duration / (time.Second / 60))
	prev := float32(-1)
	for i := 0; i <= frames; i++ {
		w := chromeFor(ease(float32(i)/float32(frames)), 268, ui.TitleBar{}, false).sideW
		if w == prev && w != 268 {
			t.Errorf("frame %d: the sidebar held at %v before landing", i, w)
		}
		prev = w
	}
}

// With the sidebar hidden, only the header at the window's top left
// keeps clear of the traffic lights: a pane under it starts its title
// where any card does.
func TestHeaderPadBelowTopLeft(t *testing.T) {
	a := testApp(t)
	a.prefs.SidebarHidden = true
	var l *proto.Layout
	a.st.Sessions[0].Layout = l.Split("a", "b", "down", 0)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	top, ok1 := tt.Find("fish")
	below, ok2 := tt.Find("zsh")
	if !ok1 || !ok2 {
		t.Fatalf("no titles; texts %q", tt.Texts())
	}
	if below.Y <= top.Y {
		t.Fatalf("pane b is not under a: %v, %v", top, below)
	}
	if below.X >= top.X {
		t.Errorf("the title under the top left's starts at %v, as far in as the top left's %v", below.X, top.X)
	}
	shot(t, tt, "header-pad-below")
}

// The window moves from the panes' headers and the gaps around them, and
// not from a press on a pane, which focuses it.
func TestWindowDragRegions(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	// Pane b's header, past its title: it moves the window, and leaves
	// the focus where it was.
	b, _ := tt.Find("zsh")
	tt.ClickAt(b.X+200, b.Y+b.H/2)
	tt.Frame()
	if f := a.focusOf(&a.st.Sessions[0]); f != "a" {
		t.Errorf("a press on a header's empty stretch moved the focus to %q", f)
	}
	// Pane b's body: it takes the press, and the focus.
	tt.ClickAt(b.X+200, 400)
	tt.Frame()
	if f := a.focusOf(&a.st.Sessions[0]); f != "b" {
		t.Errorf("a press on pane b left the focus on %q", f)
	}
}

// A selection copied as it ends says so; a copy the user asked for, or
// a program's, does not.
func TestCopyOnSelectToast(t *testing.T) {
	a := testApp(t)
	o := a.termOptions("a", nil)
	o.OnCopy("x", terminal.CopyUser)
	o.OnCopy("x", terminal.CopyProgram)
	if a.pendingToast != "" {
		t.Errorf("toast %q for a copy that shows itself", a.pendingToast)
	}
	o.OnCopy("x", terminal.CopySelect)
	if a.pendingToast != "Copied to clipboard" {
		t.Errorf("toast %q after copying a selection", a.pendingToast)
	}
}

// The sidebar's foot counts what wants a look, each count going to the
// next session of it, and says nothing while nothing does ($KERO_SHOTS).
func TestSidebarFooter(t *testing.T) {
	a := testApp(t)
	// Connected, as ready is: else the app connects, which finds no kerod.
	cl, _ := projectClient(t, func(string, json.RawMessage) (any, error) { return nil, errors.New("no kerod in a test") })
	a.local().conn, a.local().cl = connReady, cl
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	for _, txt := range tt.Texts() {
		if strings.Contains(txt, "running") {
			t.Errorf("the foot still says %q", txt)
		}
	}
	base := a.st.Sessions[0]
	add := func(id, name string, st proto.Status, unread bool) {
		s := base
		s.ID, s.Name, s.Status, s.Unread = id, name, st, unread
		a.st.Sessions = append(a.st.Sessions, s)
	}
	add("s2", "deploy", proto.Waiting, true)
	add("s3", "refactor", proto.Working, false)
	add("s4", "tests", proto.Working, false)
	add("s5", "docs", proto.Idle, true)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "sidebar-footer")
	for label, want := range map[string]string{"1 session needs you": "s2", "2 sessions working": "s3", "1 session done, unseen": "s5"} {
		a.selected = "s1"
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Errorf("no %q in the foot: %v", label, err)
			continue
		}
		tt.Frame()
		if a.selected != want {
			t.Errorf("%q went to %s, want %s", label, a.selected, want)
		}
	}
}

// The foot's Menus button has the menu bar's menus, without the system's
// items, and runs their actions; a project's menu starts a session of it in
// a worktree.
func TestMenusButton(t *testing.T) {
	// The menus' Check for Updates is the updater's, which runApp uses; in
	// a language of its own, as no app runs to ask its language of.
	mygo.Use(updater.New(updater.Options{Language: "en"}))
	a := testApp(t)
	a.menu()
	cl, _ := projectClient(t, func(string, json.RawMessage) (any, error) { return nil, errors.New("no kerod in a test") })
	a.local().conn, a.local().cl = connReady, cl
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if err := tt.Click("Menus"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	menu := tt.Menu()
	for _, want := range []string{"Session", "Edit", "Pane", "View"} {
		if !slices.Contains(menu, want) {
			t.Errorf("the menus have no %q: %q", want, menu)
		}
	}
	if err := tt.ChooseMenuItem("Session", "New Session in Worktree…"); err != nil {
		t.Fatalf("%v; menu %q", err, menu)
	}
	tt.Frame()
	// Of the repository of the session the user is in.
	if a.newAsk == nil || !a.newAsk.worktree || a.newAsk.p.After != a.selected {
		t.Fatalf("New Session in Worktree… asked %+v", a.newAsk)
	}
	a.newAsk = nil
	tt.Frame()

	pr := a.st.Projects[0]
	a.selected = ""
	tt.Frame()
	if err := tt.RightClick(pr.Name); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("New Session in Worktree…"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if w := a.newAsk; w == nil || !w.worktree || w.p.Project != pr.ID || w.dir != pr.Dir {
		t.Errorf("the project's New Session in Worktree… asked %+v", a.newAsk)
	}
}

// A project's New Session opens the dialog on an agent session of the
// default agent, which Return starts in the project; with the worktree
// option, the folder's branch, checked out there, waits for a choice, and
// an existing worktree's row starts the session in it.
func TestNewSessionDialog(t *testing.T) {
	a := testApp(t)
	a.set.Agents, a.set.DefaultAgent = []string{"claude", "cc --yolo"}, "cc --yolo"
	created := make(chan proto.CreateParams, 2)
	n := 0
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.branches":
			return proto.GitBranches{Branches: []proto.GitBranch{
				{Name: "main", Current: true, Worktree: "/tmp"},
				{Name: "feat", Worktree: "/tmp/wt/feat"},
				{Name: "v1", Tag: true},
			}}, nil
		case "git.worktrees":
			return proto.GitWorktrees{Worktrees: []proto.GitWorktree{{Path: "/tmp/wt/feat", Branch: "feat", Sessions: 1}}}, nil
		case "create":
			var p proto.CreateParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			created <- p
			n++
			return proto.Session{ID: fmt.Sprint("new", n), Project: p.Project, Panes: []proto.Pane{{ID: "c", Dir: "/tmp"}}, Focus: "c"}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().conn, a.local().cl = connReady, cl
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	pr := a.st.Projects[0]
	open := func(item string) *newSessionAsk {
		t.Helper()
		if err := tt.RightClick(pr.Name); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		tt.Frame()
		if err := tt.ChooseMenuItem(item); err != nil {
			t.Fatalf("%v; menu %q", err, tt.Menu())
		}
		tt.Frame()
		w := a.newAsk
		if w == nil {
			t.Fatalf("%s opened no dialog", item)
		}
		// The branches come.
		for i := 0; i < 100 && w.branches.list == nil && w.gitErr == ""; i++ {
			time.Sleep(10 * time.Millisecond)
			tt.Frame()
		}
		return w
	}
	wait := func() proto.CreateParams {
		t.Helper()
		select {
		case p := <-created:
			return p
		case <-time.After(2 * time.Second):
			t.Fatal("no session started")
		}
		return proto.CreateParams{}
	}

	w := open("New Session")
	if w.kind != kindAgent || w.agent != "cc --yolo" || w.worktree || !tt.HasText("Agent") {
		t.Fatalf("the dialog opened on %q of %q, worktree %v; texts %q", w.kind, w.agent, w.worktree, tt.Texts())
	}
	if !slices.ContainsFunc(w.branches.list.Branches, func(b proto.GitBranch) bool { return b.Name == "main" }) ||
		slices.ContainsFunc(w.branches.list.Branches, func(b proto.GitBranch) bool { return b.Tag }) {
		t.Errorf("branches %+v", w.branches.list.Branches)
	}
	shot(t, tt, "new-session")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if p := wait(); !slices.Equal(p.Command, []string{"cc", "--yolo"}) || p.Project != pr.ID || p.After != "s1" || p.Worktree != "" {
		t.Errorf("Return started %+v", p)
	}
	if a.newAsk != nil {
		t.Error("the dialog stayed")
	}

	w = open("New Session in Worktree…")
	if w.sel.name != "main" || w.sel.worktree != "/tmp" || !tt.HasText("Open in that worktree") {
		t.Fatalf("chose %+v; texts %q", w.sel, tt.Texts())
	}
	shot(t, tt, "new-session-worktree")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.newAsk == nil {
		t.Fatal("started a session on a branch checked out in the main folder")
	}
	// The branch list, the switcher's, names a new branch; Escape closes
	// it alone.
	if err := tt.Click("main"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !w.branches.open || !tt.HasText("feat") {
		t.Fatalf("no branch list; texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if w.branches.open || a.newAsk == nil {
		t.Fatalf("Escape left the list open %v, the dialog %v", w.branches.open, a.newAsk != nil)
	}
	tt.Click("main")
	tt.Frame()
	tt.Type("topic")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if w.sel.name != "topic" || !w.sel.create || !tt.HasText("Kero creates the branch topic from the default branch") {
		t.Fatalf("chose %+v; texts %q", w.sel, tt.Texts())
	}
	if err := tt.Click("feat"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if p := wait(); p.Dir != "/tmp/wt/feat" || p.Worktree != "" || len(p.Command) == 0 {
		t.Errorf("the worktree's row started %+v", p)
	}
}

// An existing worktree's menu removes it, asking over the dialog as what
// it would lose decides, by the dialog's folder, then lists them anew.
func TestNewSessionRemoveWorktree(t *testing.T) {
	a := testApp(t)
	ops := make(chan proto.GitOpParams, 2)
	var mu sync.Mutex
	gone := false
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.branches":
			return proto.GitBranches{Branches: []proto.GitBranch{{Name: "main", Current: true, Worktree: "/tmp"}}}, nil
		case "git.worktrees":
			var p proto.GitWorktreesParams
			json.Unmarshal(raw, &p)
			mu.Lock()
			defer mu.Unlock()
			if gone {
				return proto.GitWorktrees{Worktrees: []proto.GitWorktree{}}, nil
			}
			wt := proto.GitWorktree{Path: "/tmp/wt/worktree-agent-a0cfdfaef04b22c52", Branch: "worktree-agent-a0cfdfaef04b22c52"}
			if p.State {
				wt.State, wt.Unpushed = true, 1
			}
			return proto.GitWorktrees{Worktrees: []proto.GitWorktree{wt}}, nil
		case "git.op":
			var p proto.GitOpParams
			json.Unmarshal(raw, &p)
			mu.Lock()
			gone = true
			mu.Unlock()
			ops <- p
			return proto.GitOpResult{}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().conn, a.local().cl = connReady, cl
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	a.askNewSession(a.local(), "", "/tmp", true)
	w := a.newAsk
	frameUntil(t, tt, "the worktree's state", func() bool {
		return w.trees != nil && len(w.trees.Worktrees) == 1 && w.trees.Worktrees[0].State
	})
	if err := tt.RightClick("worktree-agent-a0cfdfaef04b22c52"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem("Remove Worktree…"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	cf := a.confirm
	if cf == nil || !strings.Contains(cf.detail, "1 commit") || cf.alt != "" {
		t.Fatalf("asked %+v", cf)
	}
	shot(t, tt, "new-session-remove")
	// Return answers the question, which is over the dialog, not the
	// dialog, which would start a session.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	select {
	case p := <-ops:
		if p.Op != "worktreeRemove" || p.Dir != "/tmp" || p.Pane != "" || p.Path != "/tmp/wt/worktree-agent-a0cfdfaef04b22c52" || p.Force || p.Name != "" {
			t.Errorf("removed %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no removal")
	}
	frameUntil(t, tt, "the list anew", func() bool { return w.trees != nil && len(w.trees.Worktrees) == 0 })
	if a.newAsk != w {
		t.Error("the dialog closed")
	}
}

// A branch checked out in another worktree, as git checks a branch out in
// one alone, starts the session in that worktree, or in a new one on a new
// branch from it, of a name no branch has; another branch is checked out
// as it is, a remote's as its local branch, and one the user names is made
// from the dialog's base.
func TestNewSessionBranch(t *testing.T) {
	list := &proto.GitBranches{Branches: []proto.GitBranch{
		{Name: "main", Current: true, Worktree: "/src/app"},
		{Name: "main-2"},
		{Name: "fix", Worktree: "/src/wt/fix"},
		{Name: "origin/feat", Remote: true},
	}}
	w := &newSessionAsk{kind: kindTerminal, worktree: true, dir: "/src/app", base: "HEAD", p: proto.CreateParams{Project: "p1"}}
	w.branches.list = list
	row := func(name string) branchRow {
		for i := range list.Branches {
			if list.Branches[i].Name == name {
				return branchRow{b: &list.Branches[i]}
			}
		}
		t.Fatalf("no branch %s", name)
		return branchRow{}
	}

	w.pickBranch(row("main"))
	if _, ok := w.params(""); ok {
		t.Error("starts on a branch in use, unasked")
	}
	if w.newBranch != "main-3" {
		t.Errorf("the new branch's name %q, want one no branch has", w.newBranch)
	}
	w.choice = "open"
	if p, ok := w.params(""); !ok || p.Dir != "/src/app" || p.Worktree != "" {
		t.Errorf("open there: %+v %v", p, ok)
	}
	w.choice = "new"
	if p, ok := w.params(""); !ok || p.Dir != "/src/app" || p.Worktree != "main-3" || p.Base != "main" {
		t.Errorf("a new branch: %+v %v", p, ok)
	}
	w.newBranch = "main-2"
	if _, ok := w.params(""); ok {
		t.Error("makes a branch there is")
	}

	w.pickBranch(row("main-2"))
	if p, ok := w.params(""); !ok || p.Worktree != "main-2" || p.Base != "" || w.choice != "" {
		t.Errorf("a free branch: %+v %v", p, ok)
	}
	w.pickBranch(row("origin/feat"))
	if p, ok := w.params(""); !ok || p.Worktree != "feat" || p.Base != "origin/feat" || !w.sel.is(&list.Branches[3]) {
		t.Errorf("a remote's branch: %+v %v", p, ok)
	}
	w.pickBranch(branchRow{create: "topic"})
	if p, ok := w.params(""); !ok || p.Worktree != "topic" || p.Base != "HEAD" || p.Dir != "/src/app" {
		t.Errorf("a branch named: %+v %v", p, ok)
	}
	if p, ok := w.params("/src/wt/fix"); !ok || p.Dir != "/src/wt/fix" || p.Worktree != "" {
		t.Errorf("an existing worktree: %+v %v", p, ok)
	}

	// Without the option, or for a browser, the session goes in the folder.
	w.kind = kindBrowser
	if p, ok := w.params(""); !ok || p.Worktree != "" || p.Dir != "" || p.URL != proto.BlankURL {
		t.Errorf("a browser: %+v %v", p, ok)
	}
}

// The search at the sidebar's foot asks the hosts once typing pauses,
// shows what they found in place of the sessions, opens one to show its
// messages, resumes the one the arrows chose, and goes as Escape clears
// it.
func TestConversationSearch(t *testing.T) {
	a := testApp(t)
	asked := make(chan proto.SearchParams, 4)
	resumed := make(chan proto.ConversationParams, 1)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "conversation.search":
			var p proto.SearchParams
			json.Unmarshal(raw, &p)
			asked <- p
			return proto.SearchResult{Matched: 2, Indexed: 9, Terms: []string{"login"}, Hits: []proto.SearchHit{
				{Conversation: proto.Conversation{Agent: "claude", ID: "c1", Dir: "/src/app", Title: "Fix the login redirect", Updated: time.Now().Add(-time.Hour).UnixMilli()},
					Turns: 7, Score: 3, Snippets: []proto.Snippet{{User: true, Text: "the login page loops", Marks: []int{4, 9}}}},
				{Conversation: proto.Conversation{Agent: "codex", ID: "x2", Dir: "/src/api", Title: "Login API"}, Turns: 2, Score: 1},
			}}, nil
		case "conversation.resume":
			var p proto.ConversationParams
			json.Unmarshal(raw, &p)
			resumed <- p
		}
		return nil, errors.New("no kerod in a test")
	})
	a.local().conn, a.local().cl = connReady, cl
	session := a.st.Sessions[0].DisplayName(a.set.SessionName)
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	shot(t, tt, "search-field")
	if err := tt.Click("Search conversations"); err != nil {
		t.Fatal(err)
	}
	tt.Type("login")
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.Frame()
	select {
	case p := <-asked:
		if p.Query != "login" || !p.Preview {
			t.Errorf("asked %+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no search asked")
	}
	for deadline := time.Now().Add(5 * time.Second); len(a.srch.hits) == 0 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		a.runEarly()
	}
	tt.Frame()
	shot(t, tt, "search-found")
	for _, want := range []string{"Fix the login redirect", "Login API", "2 conversations", "Claude", "Codex", "7"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText(session) {
		t.Errorf("the session %q shows with the search's", session)
	}
	if err := tt.Click("Fix the login redirect"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "search-open")
	if !tt.HasText("the login page loops") || !tt.HasText("Resume") {
		t.Errorf("open, no messages: %q", tt.Texts())
	}
	if err := tt.Click("Search conversations"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	select {
	case p := <-resumed:
		if p.Agent != "codex" || p.ID != "x2" {
			t.Errorf("resumed %+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing resumed")
	}
	tt.Frame()
	if a.searching() || !tt.HasText(session) {
		t.Errorf("after resuming: %q", tt.Texts())
	}
	if err := tt.Click("Search conversations"); err != nil {
		t.Fatal(err)
	}
	tt.Type("x")
	tt.Frame()
	if !a.searching() {
		t.Fatal("not searching")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.searching() || !tt.HasText(session) {
		t.Errorf("after Escape: %q", tt.Texts())
	}
}

// A kerod of a Kero with no search says to restart it, not that nothing
// matched.
func TestConversationSearchOlderKerod(t *testing.T) {
	a := testApp(t)
	cl, _ := projectClient(t, func(method string, _ json.RawMessage) (any, error) {
		return nil, fmt.Errorf("unknown method %q", method)
	})
	a.local().conn, a.local().cl = connReady, cl
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if err := tt.Click("Search conversations"); err != nil {
		t.Fatal(err)
	}
	tt.Type("login")
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.Frame()
	for deadline := time.Now().Add(5 * time.Second); len(a.srch.older) == 0 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		a.runEarly()
	}
	tt.Frame()
	if !tt.HasText(locales.SearchRestart()) || !tt.HasText(locales.CommonRestartService()) || tt.HasText(locales.SearchNoMatches()) {
		t.Errorf("texts %q", tt.Texts())
	}
}

// A session's tooltip shows below the pointer where it rested, as macOS
// shows tooltips, not centered over the row ($KERO_SHOTS).
func TestSessionTooltipAtPointer(t *testing.T) {
	for _, dx := range []float32{20, 160} {
		a := testApp(t)
		a.st.Sessions[0].Process = "claude"
		a.st.Sessions[0].Dir = "/Users/someone/dev/kero"
		tt := ui.NewTester(a.view, 1100, 700)
		tt.Frame()
		r, ok := tt.Find("1") // the project header's count, above the row
		if !ok {
			t.Fatal("no project header")
		}
		x, y := 12+dx, r.Y+r.H/2+34 // the session row's middle
		tt.Move(x, y)
		tt.Advance(2 * time.Second)
		tt.Frame()
		var tip ui.Rect
		found := false
		for _, txt := range tt.Texts() {
			if strings.HasPrefix(txt, "claude · ") {
				tip, found = tt.Find(txt)
			}
		}
		if !found {
			t.Fatalf("dx %g: no tooltip; texts %q", dx, tt.Texts())
		}
		if d := tip.X - x; d < 0 || d > 16 {
			t.Errorf("dx %g: tooltip text at x %g, the pointer at %g", dx, tip.X, x)
		}
		if d := tip.Y - y; d < 12 || d > 36 {
			t.Errorf("dx %g: tooltip text at y %g, the pointer at %g", dx, tip.Y, y)
		}
		shot(t, tt, fmt.Sprintf("session-tooltip-%g", dx))
	}
}

// A kerod of another version of Kero gets a chip to restart it, which asks
// first while a program runs, as the restart ends it.
func TestRestartStaleDaemon(t *testing.T) {
	a := testApp(t)
	a.local().conn, a.local().daemonApp = connReady, "1.9.0"
	a.st.Sessions[0].Process = "claude"
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	if err := tt.Click("Restart Service"); err != nil {
		t.Fatalf("no chip to restart kerod: %v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.confirm == nil || !strings.Contains(a.confirm.detail, "Kero 1.9.0") {
		t.Fatalf("no question before the restart: %+v", a.confirm)
	}

	a.confirm, a.local().daemonApp = nil, mygo.App.Version()
	tt.Frame()
	if tt.HasText("Restart Service") {
		t.Error("a chip to restart a kerod of this version")
	}
}

// idleConn is a Conn that shows only what a test feeds the terminal.
type idleConn struct{ closed chan struct{} }

func (c idleConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, os.ErrClosed
}
func (c idleConn) Write(b []byte) (int, error) { return len(b), nil }
func (c idleConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

// Find searches the pane the user is in, its scrollback too: the bar
// counts the matches as one types, Enter and the buttons go through them,
// and Escape closes it, the highlights going; Find Next opens it again
// with the last query.
func TestFindBar(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads libghostty-vt")
	}
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	var b strings.Builder
	for i := range 200 {
		if i%50 == 7 {
			b.WriteString("error: build failed\r\n")
		} else {
			b.WriteString("ok " + itoa(i) + "\r\n")
		}
	}
	term.Feed([]byte(b.String()))
	a.terms["a"] = &termEntry{term: term}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	a.run("find.open", nil)
	tt.Frame()
	tt.Type("ERROR")
	for range 50 {
		tt.Frame()
		if s := term.SearchStatus(); !s.Searching && s.Selected > 0 {
			break
		}
	}
	tt.Frame()
	if !tt.HasText("1 of 4") {
		t.Fatalf("no count; texts %q", tt.Texts())
	}
	shot(t, tt, "find")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("2 of 4") {
		t.Errorf("after Enter twice and Shift+Enter; texts %q", tt.Texts())
	}
	if err := tt.Click(a.tip(locales.ActionFindNext(), "find.next")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("3 of 4") {
		t.Errorf("after the button for the next; texts %q", tt.Texts())
	}
	shot(t, tt, "find-scrolled")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if a.find.pane != "" || tt.HasText("3 of 4") || term.SearchStatus().Text != "" {
		t.Errorf("Escape left the bar open or the search on: %+v; texts %q", term.SearchStatus(), tt.Texts())
	}
	a.run("find.next", nil)
	tt.Frame()
	tt.Frame()
	if a.find.pane != "a" || a.find.query != "ERROR" || term.SearchStatus().Text != "ERROR" {
		t.Errorf("Find Next did not search again: %+v, %+v", a.find.query, term.SearchStatus())
	}
}

// Command over the path of an image a pane printed, relative to its
// directory, shows the image beside it, until Command is let go of,
// which lets go of the image too.
func TestImagePeek(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads libghostty-vt")
	}
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	dir := t.TempDir()
	a.st.Sessions[0].Panes[0].Dir = dir
	f, err := os.Create(filepath.Join(dir, "shot.png"))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 30, 20))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{230, 120, 40, 255}), image.Point{}, draw.Src)
	png.Encode(f, img)
	f.Close()
	term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}, OpenLink: func(link string) bool { return a.openLink("a", link) }})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("saved shot.png:3 and notes.txt\r\n"))
	a.terms["a"] = &termEntry{term: term}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	b, ok := tt.Find("Terminal")
	if !ok {
		t.Fatalf("no terminal; texts %q", tt.Texts())
	}
	// Along the first row to the path, Command pressed with the pointer
	// still.
find:
	for y := b.Y; y < b.Y+40; y += 4 {
		for x := b.X; x < b.X+b.W/2; x += 3 {
			tt.Move(x, y)
			tt.HoldModifiers(ui.Cmd)
			if link, _, _ := term.HoveredLink(); link != "" {
				break find
			}
		}
	}
	tt.Frame()
	if link, path, _ := term.HoveredLink(); link != "shot.png:3" || !path {
		t.Fatalf("hovered %q, %v; text %q", link, path, term.Text())
	}
	if want := filepath.Join(dir, "shot.png"); a.peek.path != want {
		t.Fatalf("peeks at %q, want %q", a.peek.path, want)
	}
	// The decoded image lands, as the window's update would bring it, and
	// shows in the flyout.
	data, _ := os.ReadFile(a.peek.path)
	a.peek.bm, _ = ui.DecodeBitmap(data)
	tt.Frame()
	ft := ui.NewTester(a.peekView, peekMinW, 20+peekPad+peekCaption)
	ft.Frame()
	if !ft.HasText("shot.png") || !ft.HasText("30 × 20") {
		t.Fatalf("no preview; texts %q", ft.Texts())
	}
	shot(t, ft, "peek")
	tt.HoldModifiers(0)
	tt.Frame()
	if a.peek.bm != nil || a.peek.path != "" {
		t.Errorf("the preview stays once Command is let go of: %+v", a.peek)
	}
}

// A split top to bottom, with one side by side inside it, gives every
// split element a key of its own: MyGo panics on two elements with one key
// under one parent, as when a column split made a Row before its Column.
func TestNestedSplitKeys(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes = append(s.Panes, proto.Pane{ID: "c", Shell: "bash", Dir: "/tmp", Status: proto.Idle, Progress: -1})
	s.Layout = &proto.Layout{Ratio: 0.5, A: &proto.Layout{Pane: "a"},
		B: &proto.Layout{Row: true, Ratio: 0.5, A: &proto.Layout{Pane: "b"}, B: &proto.Layout{Pane: "c"}}}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Frame()
	for _, title := range []string{"fish", "zsh", "bash"} {
		if !tt.HasText(title) {
			t.Errorf("no pane %s; texts %q", title, tt.Texts())
		}
	}
}

// A browser pane shows a toolbar over its page, as its header, with an
// address bar that a blank page asks to type in and Open Location gives
// the keyboard; it attaches no terminal.
func TestBrowserPane(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", Dir: "/tmp", URL: proto.BlankURL, Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Frame()
	for _, text := range []string{locales.BrowserBlank()} {
		if !tt.HasText(text) {
			t.Errorf("no %q; texts %q", text, tt.Texts())
		}
	}
	if a.terms["b"] != nil {
		t.Error("a browser pane attached a terminal")
	}
	e := a.webs["b"]
	if e == nil {
		t.Fatal("no browser for pane b")
	}
	// A blank page's address bar takes the keyboard: what is typed there
	// goes to the page, as a URL.
	tt.Type("localhost:5173")
	tt.Frame()
	if e.address != "localhost:5173" {
		t.Errorf("typed %q into the address bar", e.address)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if e.url != "http://localhost:5173" {
		t.Errorf("went to %q", e.url)
	}

	// Its toolbar is its header: no title over it, page's or other.
	s = &a.st.Sessions[0]
	s.Panes[1].Title = "Vite App"
	tt.Frame()
	if tt.HasText("Vite App") || tt.HasText("browser") || !tt.HasText(locales.BrowserAddress()) {
		t.Errorf("a browser pane shows a title bar; texts %q", tt.Texts())
	}
	// Open Location gives the browser's address bar the keyboard, with
	// what it holds to type over.
	a.setFocus(s, "b")
	a.run("browser.address", nil)
	tt.Frame()
	tt.Type("kero.sh")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if e.url != "https://kero.sh" {
		t.Errorf("Open Location went to %q", e.url)
	}
	shot(t, tt, "browser")
}

// An address typed in the palette opens in a browser, first of what it
// finds; words find the browser's actions, but open nothing.
func TestPaletteOpensAddress(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	a.openPalette()
	tt.Frame()
	tt.Type("localhost:5173")
	tt.Frame()
	// The palette's rows follow its label.
	texts := tt.Texts()
	rows := texts[slices.Index(texts, locales.PaletteLabel())+1:]
	if len(rows) < 2 || rows[0] != locales.PaletteOpenInBrowser() || rows[1] != "http://localhost:5173" {
		t.Fatalf("the address is not the first item; rows %q", rows)
	}
	a.pal.query = ""
	tt.Frame()
	tt.Type("web")
	tt.Frame()
	if tt.HasText(locales.PaletteOpenInBrowser()) || !tt.HasText(locales.ActionNewBrowser()) {
		t.Errorf("words: texts %q", tt.Texts())
	}
}

// With focus-follows-mouse, the pointer going into a pane focuses it, but
// a pane that comes under a pointer standing still does not.
func TestFocusFollowsMouse(t *testing.T) {
	a := testApp(t)
	a.set.FocusFollowsMouse = true
	s := &a.st.Sessions[0]
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	ar, _ := tt.Find("fish") // pane a's title
	br, _ := tt.Find("zsh")  // pane b's
	move := func(x float32) {
		tt.Move(x, 340)
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	move(ar.X + 40)
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("over a, focus %q", got)
	}
	move(br.X + 40)
	if got := a.focusOf(s); got != "b" {
		t.Fatalf("into b, focus %q, want b", got)
	}

	// The keyboard takes the focus back to a: b, under the pointer, keeps
	// it from there, as the pointer moves on in it.
	a.setFocus(s, "a")
	tt.Frame()
	move(br.X + 60)
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("moving in b after the keyboard focused a, focus %q", got)
	}

	// The panes swap places under the pointer standing still, and back.
	var l *proto.Layout
	s.Layout = l.Split("b", "a", "right", 0)
	tt.Advance(time.Second)
	tt.Frame()
	s.Layout = l.Split("a", "b", "right", 0)
	tt.Advance(time.Second)
	tt.Frame()
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("b came under the pointer standing still, and took the focus")
	}

	// Off, the pointer leaves the focus be.
	a.set.FocusFollowsMouse = false
	move(ar.X + 40)
	move(br.X + 40)
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("with the setting off, focus %q", got)
	}
}

// With focus-follows-mouse, the pointer going into the viewer focuses
// it, and going back into a pane focuses the pane.
func TestFocusFollowsMouseExplorer(t *testing.T) {
	a := testApp(t)
	a.set.FocusFollowsMouse = true
	a.prefs.Side = sideFiles
	s := &a.st.Sessions[0]
	// A file's pane, whose editor takes the keyboard where the tester has
	// no terminal to.
	s.Panes[0].File = "/tmp/a.txt"
	f := a.editorOf(&s.Panes[0])
	f.testRead("a\n")
	fv := &fileView{pane: "a", path: "/tmp/b.go", rel: "b.go", fetched: time.Now()}
	fv.testRead("package b\n")
	a.openView(s, sideFiles, fv)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Frame()
	ar, _ := tt.Find("a.txt") // pane a's title
	fr, ok := tt.Find("b.go") // the viewer's
	if !ok {
		t.Fatalf("no viewer; texts %q", tt.Texts())
	}
	move := func(x float32) {
		tt.Move(x, 340)
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	move(ar.X + 40)
	move(fr.X + 20)
	tt.Frame()
	if !a.side.focused {
		t.Fatal("the pointer went into the viewer, which isn't focused")
	}
	move(ar.X + 40)
	tt.Frame()
	if a.side.focused || a.focusOf(s) != "a" {
		t.Fatalf("back into a: viewer focused %v, pane focus %q", a.side.focused, a.focusOf(s))
	}
}

// writes is a Conn that keeps what the terminal sends to the program.
type writes struct {
	idleConn
	got chan []byte
}

func (w writes) Write(b []byte) (int, error) {
	w.got <- append([]byte(nil), b...)
	return len(b), nil
}

// ⌘⌫, ⌘← and ⌘→ send Control+U, Control+A and Control+E, which clear the
// line and go to its start and end, and only on a Mac, where ⌘ is not the
// terminal's Control.
func TestCommandKeysEditLine(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads libghostty-vt")
	}
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	conn := writes{idleConn{make(chan struct{})}, make(chan []byte, 8)}
	term, err := terminal.New(terminal.Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	a.terms["a"] = &termEntry{term: term}
	o := a.termOptions("a", nil)
	for _, k := range []struct {
		key  ui.Key
		want string
	}{{ui.KeyBackspace, "\x15"}, {ui.KeyLeft, "\x01"}, {ui.KeyRight, "\x05"}} {
		if took := o.OnKey(ui.Super, k.key); took != (runtime.GOOS == "darwin") {
			t.Fatalf("⌘+%v taken: %v", k.key, took)
		}
		if runtime.GOOS != "darwin" {
			continue
		}
		select {
		case b := <-conn.got:
			if string(b) != k.want {
				t.Errorf("⌘+%v sent %q, want %q", k.key, b, k.want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("⌘+%v sent nothing", k.key)
		}
	}
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{
		{0, ui.KeyBackspace}, {ui.Super | ui.Shift, ui.KeyBackspace}, {ui.Super, ui.KeyDelete},
		{0, ui.KeyLeft}, {ui.Super | ui.Shift, ui.KeyLeft}, {ui.Super | ui.Alt, ui.KeyRight},
	} {
		if o.OnKey(k.mods, k.key) {
			t.Errorf("%v+%v taken", k.mods, k.key)
		}
	}
}

// A long name in a narrow sidebar ends in an ellipsis: the time beside
// it, and the ⌘ shortcut in its place, show whole.
// A project dragged by its header goes before or after the one it is
// dropped on, by the half the pointer is in, in the sidebar at once and
// in kerod's order.
func TestSidebarDragsProjects(t *testing.T) {
	a := testApp(t)
	a.local().conn = connReady
	moves := make(chan proto.MoveProjectParams, 4)
	a.local().cl, _ = projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method != "project.move" {
			return nil, fmt.Errorf("test daemon rejected %s", method)
		}
		var p proto.MoveProjectParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		moves <- p
		return true, nil
	})
	a.st.Projects = []proto.Project{{ID: "p1", Name: "alpha", Dir: "/a"}, {ID: "p2", Name: "bravo", Dir: "/b"}, {ID: "p3", Name: "charlie", Dir: "/c"}}
	for i, p := range []string{"p2", "p3"} {
		s := a.st.Sessions[0]
		s.ID, s.Project, s.Name = "s"+itoa(i+2), p, "work "+p
		a.st.Sessions = append(a.st.Sessions, s)
	}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	shot(t, tt, "sidebar-projects")
	order := func() string {
		var names []string
		for _, p := range a.st.Projects {
			names = append(names, p.Name)
		}
		return strings.Join(names, ",")
	}
	drag := func(name string, toX, toY float32) {
		t.Helper()
		r, ok := tt.Find(name)
		if !ok {
			t.Fatalf("no %s", name)
		}
		x, y := r.X+r.W/2, r.Y+r.H/2
		tt.Press(x, y)
		tt.Move(x, y+6)
		tt.Move(toX, toY)
		tt.Frame()
		tt.Release(toX, toY)
		tt.Frame()
	}
	alpha, _ := tt.Find("alpha")
	drag("charlie", alpha.X, alpha.Y+2)
	if got := order(); got != "charlie,alpha,bravo" {
		t.Errorf("charlie dropped on alpha's top: %s", got)
	}
	select {
	case p := <-moves:
		if p.ID != "p3" || p.Index != 0 {
			t.Errorf("kerod moved %+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("kerod wasn't told")
	}
	tt.Frame()
	// On the lower half of the last project's block: after it.
	session, _ := tt.Find("work p2")
	drag("charlie", session.X, session.Y+session.H/2)
	if got := order(); got != "alpha,bravo,charlie" {
		t.Errorf("charlie dropped below bravo's session: %s", got)
	}
}

func TestSidebarRowKeepsTime(t *testing.T) {
	widths := func(name string) (ago, key float32) {
		a := testApp(t)
		a.prefs.SidebarWidth = minSidebar
		tt := ui.NewTester(a.view, 1100, 640)
		a.st.Sessions[0].Name = name
		a.st.Sessions[0].Since = tt.Now().Add(-3 * time.Hour).UnixMilli()
		tt.Frame()
		r, ok := tt.Find(locales.AgoHours(3))
		if !ok {
			t.Fatalf("%q: no time shown", name)
		}
		tt.HoldModifiers(ui.Cmd)
		tt.Frame()
		k, ok := tt.Find(actions.Glyphs("CmdOrCtrl+1"))
		if !ok {
			t.Fatalf("%q: no shortcut shown", name)
		}
		return r.W, k.W
	}
	t0, k0 := widths("demo")
	t1, k1 := widths("Investigate why the session list truncates long generated titles")
	if t1 < t0 || k1 < k0 {
		t.Errorf("with a long name, the time is %g wide (want %g) and the shortcut %g (want %g)", t1, t0, k1, k0)
	}
}

// A project's header shows how many sessions it holds, unless
// sidebar-session-count is off.
func TestSidebarSessionCount(t *testing.T) {
	a := testApp(t)
	a.local().conn = connReady
	for _, id := range []string{"s2", "s3"} {
		s := a.st.Sessions[0]
		s.ID = id
		a.st.Sessions = append(a.st.Sessions, s)
	}
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if !slices.Contains(tt.Texts(), "3") {
		t.Errorf("the project's header has no count; texts %q", tt.Texts())
	}
	a.set.SidebarCount = false
	tt.Frame()
	if slices.Contains(tt.Texts(), "3") {
		t.Errorf("with sidebar-session-count off, the project's header still has its count")
	}
}

// A detailed row shows, under the session's name, its status message,
// else its branch and changes, at the whole row's width; a compact one
// keeps to one line ($KERO_SHOTS).
func TestSidebarRows(t *testing.T) {
	a := testApp(t)
	a.local().conn = connReady
	a.st.Sessions[0].Dir = "/Users/someone/dev/demo"
	base := a.st.Sessions[0]
	add := func(id, name string, st proto.Status, msg string, g *proto.GitInfo) {
		s := base
		s.ID, s.Name, s.Status, s.Message, s.Git = id, name, st, msg, g
		a.st.Sessions = append(a.st.Sessions, s)
	}
	add("s2", "migrate", proto.Working, "Migrating the\ndatabase", nil)
	add("s3", "login", proto.Idle, "", &proto.GitInfo{Root: "/tmp/demo", Branch: "fix-login-redirect-after-oauth", Default: "main", Ahead: 2, Files: 3, Added: 40, Removed: 2})
	a.st.Sessions[1].Progress = 40
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	shot(t, tt, "sidebar-rows-detailed")
	texts := strings.Join(tt.Texts(), "\n")
	for _, want := range []string{"Migrating the database", "fix-login-redirect-after-oauth ↑2 · 3 files +40 −2", "/Users/someone/dev/demo"} {
		if !strings.Contains(texts, want) {
			t.Errorf("detailed rows have no %q; texts %q", want, tt.Texts())
		}
	}
	a.set.SidebarRows = "compact"
	tt.Frame()
	shot(t, tt, "sidebar-rows-compact")
	texts = strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, "Migrating the database") {
		t.Errorf("a compact row shows the message")
	}
	if !strings.Contains(texts, "fix-login-redirect-after-oauth") {
		t.Errorf("a compact row lost its branch; texts %q", tt.Texts())
	}
}

// Each row leads with its lead pane's program; a split session's row has
// a count of its panes, which lists them under it, each with its program,
// name, place and status, and a click focuses one ($KERO_SHOTS).
func TestSidebarSplitPanes(t *testing.T) {
	a := testApp(t)
	a.local().conn = connReady
	a.terms["a"], a.terms["b"] = &termEntry{}, &termEntry{}
	s := &a.st.Sessions[0]
	s.Name = "auth refactor"
	s.Panes[0] = proto.Pane{ID: "a", Shell: "zsh", Process: "claude", Agent: "claude", Title: "Refactor the auth flow", Dir: "/Users/someone/dev/demo", Status: proto.Working, Progress: -1,
		Git: &proto.GitInfo{Root: "/Users/someone/dev/demo", Branch: "auth-refactor", Default: "main"}}
	s.Panes[1] = proto.Pane{ID: "b", Shell: "zsh", Process: "codex", Agent: "codex", Title: "Review the tests", Dir: "/Users/someone/dev/demo", Status: proto.Waiting, Unread: true, Progress: -1, Message: "Allow running npm test?"}
	s.Panes = append(s.Panes, proto.Pane{ID: "c", Shell: "zsh", Process: "bun", Runtime: "bun", Dir: "/Users/someone/dev/demo/web", Status: proto.Idle, Progress: -1})
	var l *proto.Layout
	s.Layout = l.Split("a", "b", "right", 0).Split("b", "c", "down", 0)
	s.Pane = s.Panes[proto.Lead(s.Panes, s.Focus)]
	s.Lead = s.Pane.ID
	base := a.st.Sessions[0]
	add := func(id, name string, p proto.Pane) {
		n := base
		n.ID, n.Name, n.Panes, n.Layout, n.Pane, n.Lead, n.Focus = id, name, []proto.Pane{p}, nil, p, p.ID, p.ID
		a.st.Sessions = append(a.st.Sessions, n)
	}
	add("s2", "docs", proto.Pane{ID: "d", Shell: "zsh", Dir: "/Users/someone/dev/demo", Status: proto.Idle, Progress: -1})
	add("s3", "gemini", proto.Pane{ID: "e", Shell: "zsh", Process: "gemini", Agent: "gemini", Dir: "/Users/someone/dev/demo", Status: proto.Working, Progress: -1})
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	// The pane's header shows its name too: the sidebar adds one.
	count := func(s string) int {
		n := 0
		for _, t := range tt.Texts() {
			if t == s {
				n++
			}
		}
		return n
	}
	shown := count("Review the tests")
	if !tt.HasText(locales.SidebarShowPanes()) {
		t.Fatal("a split session's row has no count of its panes")
	}
	if err := tt.Click(locales.SidebarShowPanes()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.selected != "s1" || !a.prefs.Expanded["s1"] {
		t.Fatalf("the count of panes didn't expand the session; expanded %v", a.prefs.Expanded)
	}
	if count("Review the tests") != shown+1 {
		t.Errorf("the expanded session doesn't list its panes; texts %q", tt.Texts())
	}
	for _, want := range []string{"Refactor the auth flow", "auth-refactor", "bun", "web"} {
		if !tt.HasText(want) {
			t.Errorf("the expanded session shows no %q; texts %q", want, tt.Texts())
		}
	}
	tt.Move(600, 400) // no tooltip in the shots
	tt.Frame()
	shot(t, tt, "sidebar-split-panes-light")
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "sidebar-split-panes-dark")
	tt.SetDark(false)
	a.set.SidebarRows = "compact"
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "sidebar-split-panes-compact")
	a.set.SidebarRows = "detailed"
	tt.Frame()

	// A click on a pane's row shows its session, with the pane focused:
	// from another session, whose panes show none of the split one's.
	a.selectSession("s2")
	tt.Frame()
	if err := tt.Click("Review the tests"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f := a.focusOf(a.session("s1")); a.selected != "s1" || f != "b" {
		t.Errorf("clicking the pane's row showed %s focused on %q, want s1 on b", a.selected, f)
	}
	if err := tt.Click(locales.SidebarHidePanes()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.prefs.Expanded["s1"] || count("Review the tests") != shown {
		t.Error("the count of panes didn't collapse the session")
	}
}

// A narrow pane's header folds its controls into a menu, and its git chip
// shows its counts alone, or nothing, rather than run over its buttons.
func TestPaneHeaderNarrow(t *testing.T) {
	a := testApp(t)
	// The sidebar's rows on one line, without the branch the detailed ones
	// show under the session's name.
	a.set.SidebarRows = "compact"
	s := &a.st.Sessions[0]
	for i := range s.Panes {
		s.Panes[i].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "feature-login", Files: 2, Added: 78}
	}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("feature-login") || tt.HasText("More") {
		t.Errorf("a wide header folds, or has no branch; texts %q", tt.Texts())
	}
	tt.SetSize(600, 720)
	tt.Frame()
	tt.Frame()
	if tt.HasText("feature-login") {
		t.Errorf("a narrow header shows the branch; texts %q", tt.Texts())
	}
	if !tt.HasText("More") {
		t.Errorf("a narrow header's controls don't fold; texts %q", tt.Texts())
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if items := tt.Menu(); !slices.Equal(items, []string{"Split Right", "Split Down"}) {
		t.Errorf("the folded controls' menu: %q", items)
	}
}

// A split button's menu offers what goes in the new pane.
func TestSplitMenu(t *testing.T) {
	a := testApp(t)
	// A pane alone shows its buttons only while the pointer is over it.
	s := &a.st.Sessions[0]
	s.Panes, s.Layout = s.Panes[:1], nil
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Move(700, 300)
	tt.Frame()
	if err := tt.RightClick("Split Right"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if items := tt.Menu(); !slices.Equal(items, []string{"Terminal", "Browser", "New File"}) {
		t.Errorf("the split button's menu: %q", items)
	}
	// The menu takes the pointer off the header, whose buttons hide: the
	// item chosen still reaches its button.
	tt.Move(100, 400)
	tt.Frame()
	if err := tt.ChooseMenuItem("Terminal"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.menuPane != "" {
		t.Errorf("the item chosen did not reach the split button, of pane %q", a.menuPane)
	}
}

// A pane's header fits its folder to its room: the path where it fits, else
// the name, else nothing; and the pointer, which shows the header's
// controls, doesn't change which.
func TestPaneFolderFit(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	dir := "/srv/projects/clients/acme/website/frontend"
	s.Panes, s.Layout = s.Panes[:1], nil
	s.Panes[0].Dir = dir
	tt := ui.NewTester(a.view, 1280, 720)
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText(dir) {
		t.Errorf("a wide header lacks the path; texts %q", tt.Texts())
	}
	tt.SetSize(560, 720)
	for range 3 {
		tt.Frame()
	}
	if tt.HasText(dir) || !tt.HasText("frontend") {
		t.Errorf("a narrow header shows %q", tt.Texts())
	}
	tt.Move(400, 30)
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("frontend") {
		t.Errorf("the pointer over the header changes its folder; texts %q", tt.Texts())
	}
	a.set.PaneFolder = "hidden"
	tt.SetSize(1280, 720)
	tt.Frame()
	if tt.HasText(dir) || tt.HasText("frontend") {
		t.Errorf("a hidden folder shows; texts %q", tt.Texts())
	}
}
