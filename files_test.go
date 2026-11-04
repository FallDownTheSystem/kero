package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/git"
	"kero/internal/proto"
	"kero/locales"
)

// The right sidebar shows on the tab it showed last, from the button in
// the header at the window's top right, and hides from its own; Toggle
// Changes and Toggle Files show their tab, or hide the sidebar showing it.
func TestRightSidebar(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	shot(t, tt, "right-hidden")
	if err := tt.Click(a.tip("Show Right Sidebar", "rightSidebar.toggle")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.prefs.Side != sideChanges || !tt.HasText("Files") {
		t.Fatalf("the sidebar didn't show on Changes: %q; texts %q", a.prefs.Side, tt.Texts())
	}
	if a.viewOf(s) != nil {
		t.Error("showing the sidebar opened the viewer")
	}
	if tt.HasText("Show Right Sidebar") {
		t.Error("the button that shows the sidebar stayed as it showed")
	}
	shot(t, tt, "right-sidebar")

	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.prefs.Side != sideFiles {
		t.Errorf("a click on Files showed %q", a.prefs.Side)
	}
	if err := tt.Click(a.tip("Hide Right Sidebar", "rightSidebar.toggle")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.prefs.Side != "" {
		t.Errorf("the sidebar stayed: %q", a.prefs.Side)
	}
	a.run("rightSidebar.toggle", nil)
	if a.prefs.Side != sideFiles {
		t.Errorf("the sidebar came back on %q, not the tab it showed last", a.prefs.Side)
	}

	// Toggle Changes shows the tab and the diffs, and again hides both.
	a.run("changes.toggle", nil)
	if v := a.viewOf(s); a.prefs.Side != sideChanges || v == nil || v.kind != sideChanges {
		t.Fatal("Toggle Changes didn't show the changes and their diffs")
	}
	a.run("changes.toggle", nil)
	if a.prefs.Side != "" || a.viewOf(s) != nil {
		t.Error("Toggle Changes again didn't hide the sidebar and the diffs")
	}
	a.run("files.toggle", nil)
	if a.prefs.Side != sideFiles || a.viewOf(s) != nil {
		t.Errorf("Toggle Files showed %q, and the viewer %v", a.prefs.Side, a.viewOf(s))
	}
}

// The Files tab is the worktree's tree, a folder listed as it unfolds,
// without .git, what git ignores faded; a click on a file opens it in the
// viewer, as its text, which edits and follows the file.
func TestFilesTab(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main"}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	a.prefs.Side = sideFiles
	h := a.hostOf(s.ID)
	fs := &a.side.files
	fs.dirs = map[string]*dirList{
		filesKey(h, "/tmp/demo"): {entries: []proto.FSEntry{
			{Name: ".git", Dir: true}, {Name: "internal", Dir: true}, {Name: "node_modules", Dir: true, Ignored: true}, {Name: "main.go"},
		}},
		filesKey(h, "/tmp/demo/internal"): {entries: []proto.FSEntry{{Name: "auth.go"}}},
	}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	for _, want := range []string{"internal", "node_modules", "main.go"} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	for _, not := range []string{".git", "auth.go"} {
		if _, ok := tt.Find(not); ok {
			t.Errorf("%q shows", not)
		}
	}

	if err := tt.Click("internal"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("auth.go"); !ok {
		t.Fatalf("unfolding internal didn't show auth.go; texts %q", tt.Texts())
	}
	if err := tt.Click("auth.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	v := a.viewOf(s)
	if v == nil || v.kind != sideFiles || v.file == nil || v.file.path != "/tmp/demo/internal/auth.go" || v.file.rel != "internal/auth.go" {
		t.Fatalf("a click on auth.go opened %+v", v)
	}
	fv := v.file
	fv.fetched = time.Now()
	showFileText(fv, "package internal\r\n\r\nfunc Auth() {}\r\n")
	tt.Frame()
	tt.Frame()
	if got := fv.st.Buffer.String(); got != "package internal\n\nfunc Auth() {}\n" {
		t.Errorf("the viewer shows %q", got)
	}
	if fv.st.ReadOnly {
		t.Error("the file's text doesn't edit with no agent working")
	}
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "side-files")

	// The file changing changes the text in place.
	st := fv.st
	showFileText(fv, "package internal\n\n// Auth signs in.\nfunc Auth() {}\n")
	if fv.st != st || fv.st.Buffer.String() != "package internal\n\n// Auth signs in.\nfunc Auth() {}\n" {
		t.Error("the file's text didn't change in place")
	}

	// Collapse Folders folds them all.
	if err := tt.Click("Collapse Folders"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("auth.go"); ok && len(fs.open) != 0 {
		t.Error("Collapse Folders left internal unfolded")
	}

	// The keys move in the tree from the row clicked: Up to a folder,
	// Right opens it and goes in, Enter opens a file, Left goes out to
	// its folder and closes it.
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	internal := filesKey(h, "/tmp/demo/internal")
	for _, k := range []ui.Key{ui.KeyUp, ui.KeyUp, ui.KeyRight} {
		tt.Key(0, k)
		tt.Frame()
	}
	if fs.cursor != internal || !fs.open[internal] {
		t.Fatalf("Up, Up, Right: at %q, internal open %v", fs.cursor, fs.open[internal])
	}
	shot(t, tt, "files-keys")
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if v := a.viewOf(s); v == nil || v.file == nil || v.file.path != "/tmp/demo/internal/auth.go" {
		t.Fatalf("Right, Enter on internal's first file opened %+v", v)
	}
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if fs.cursor != internal || fs.open[internal] {
		t.Errorf("Left, Left from auth.go: at %q, internal open %v", fs.cursor, fs.open[internal])
	}
}

// Close takes the viewer while it, or the Files tree a click on a file
// leaves the keyboard in (to move in it), has the keyboard, not the pane
// beside it; else the pane, viewer or not.
func TestCloseInViewer(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main"}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	s.Panes[0].Process, s.Panes[1].Process = "", ""
	a.prefs.Side = sideFiles
	a.side.files.dirs = map[string]*dirList{
		filesKey(a.hostOf(s.ID), "/tmp/demo"): {entries: []proto.FSEntry{{Name: "main.go"}}},
	}
	panes := len(s.Panes)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	open := func() *sideView {
		t.Helper()
		if err := tt.Click("main.go"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Frame()
		v := a.viewOf(s)
		if v == nil || !a.side.files.focused {
			t.Fatalf("a click on main.go left viewer %+v, tree focused %v", v, a.side.files.focused)
		}
		return v
	}

	// With edits it asks first, and pressing it again doesn't reach the pane.
	fv := open().file
	fv.fetched = time.Now()
	showFileText(fv, "package main\n")
	tt.Frame()
	fv.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// Main.\n"})
	tt.Frame()
	a.run("pane.close", nil)
	tt.Frame()
	tt.Frame()
	if a.viewOf(s) == nil || a.confirm == nil {
		t.Fatal("Close with edits didn't ask to save them")
	}
	if a.side.focused {
		t.Fatal("the question left the keyboard in the viewer")
	}
	a.run("pane.close", nil)
	tt.Frame()
	if len(s.Panes) != panes {
		t.Fatalf("Close behind the question closed a pane: %d of %d", len(s.Panes), panes)
	}
	if err := tt.Click("Don't Save"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.viewOf(s) != nil || len(s.Panes) != panes {
		t.Fatalf("Don't Save left viewer %+v, %d panes of %d", a.viewOf(s), len(s.Panes), panes)
	}

	// Without edits it closes at once, the keyboard left in the tree.
	open()
	a.run("pane.close", nil)
	tt.Frame()
	tt.Frame()
	if a.viewOf(s) != nil || a.side.focused || !a.side.files.focused || len(s.Panes) != panes {
		t.Fatalf("Close left viewer %+v, focused %v, tree focused %v, %d panes of %d", a.viewOf(s), a.side.focused, a.side.files.focused, len(s.Panes), panes)
	}

	// kero do means the pane, wherever the keyboard is.
	v := open()
	a.from = a.local()
	a.run("pane.close", nil)
	a.from = nil
	tt.Frame()
	if a.viewOf(s) != v || len(s.Panes) != panes-1 {
		t.Fatalf("kero do left viewer %+v, %d panes of %d", a.viewOf(s), len(s.Panes), panes)
	}

	// With the keyboard out of the viewer, the pane closes and it stays.
	s.Panes = append(s.Panes, proto.Pane{ID: "extra"})
	s.Layout = s.Layout.Split(s.Panes[0].ID, "extra", "right", 0.5)
	tt.Frame()
	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.side.focused || a.side.files.focused {
		t.Fatalf("the viewer kept the keyboard: viewer %v, tree %v", a.side.focused, a.side.files.focused)
	}
	a.run("pane.close", nil)
	tt.Frame()
	if a.viewOf(s) != v || len(s.Panes) != panes-1 {
		t.Fatalf("Close in the pane left viewer %+v, %d panes of %d", a.viewOf(s), len(s.Panes), panes)
	}
}

// Close in the diffs takes the viewer, as in a file, asking first to save
// their edits.
func TestCloseInChanges(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main"}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	s.Panes[0].Process, s.Panes[1].Process = "", ""
	a.prefs.Side = sideChanges
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseHead)
	list := func() {
		ch.base, ch.scope, ch.stamp = git.BaseHead, scope, gitStamp(g)
		ch.res = &proto.GitChanges{Root: "/tmp/demo", Files: []proto.GitFile{{Path: "pkg/a.go", Status: "modified", Added: 1}}}
		ch.fetched = time.Now()
	}
	list()
	panes := len(s.Panes)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()

	// A click on a changed file opens the diffs with the keyboard there.
	list()
	tt.Frame()
	if err := tt.Click("a.go"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	list()
	tt.Frame()
	tt.Frame()
	if v := a.viewOf(s); v == nil || v.kind != sideChanges || !a.side.focused {
		t.Fatalf("a click on a.go left viewer %+v, focused %v", v, a.side.focused)
	}
	sf := ch.files[scope+"|pkg/a.go"]
	if sf == nil {
		t.Fatal("no diff of pkg/a.go")
	}
	a.showText(sf, proto.GitText{Old: "package a\n", New: "package a\n\nvar x = 1\n", Hash: "h1"})
	tt.Frame()

	sf.d.New.Edit(editor.Edit{Start: 0, End: 0, Text: "// A.\n"})
	tt.Frame()
	a.run("pane.close", nil)
	tt.Frame()
	if a.viewOf(s) == nil || a.confirm == nil || len(s.Panes) != panes {
		t.Fatalf("Close with edits left viewer %+v, confirm %+v, %d panes of %d", a.viewOf(s), a.confirm, len(s.Panes), panes)
	}
	if err := tt.Click("Don't Save"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.viewOf(s) != nil || a.side.focused || len(s.Panes) != panes {
		t.Fatalf("Don't Save left viewer %+v, focused %v, %d panes of %d", a.viewOf(s), a.side.focused, len(s.Panes), panes)
	}
}

// The branch's chip in a pane's header opens the diffs with the keyboard
// there too, for Close to take them rather than the pane.
func TestCloseAfterBranchChip(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main", Files: 1, Added: 2}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	s.Panes[0].Process, s.Panes[1].Process = "", ""
	a.prefs.Side = sideFiles
	panes := len(s.Panes)
	tt := ui.NewTester(a.view, 1600, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("+2"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if v := a.viewOf(s); v == nil || v.kind != sideChanges || !a.side.focused {
		t.Fatalf("the chip left viewer %+v, focused %v", v, a.side.focused)
	}
	a.run("pane.close", nil)
	tt.Frame()
	if a.viewOf(s) != nil || len(s.Panes) != panes {
		t.Fatalf("Close left viewer %+v, %d panes of %d", a.viewOf(s), len(s.Panes), panes)
	}
}

// Toggle Changes gives the diffs the keyboard, for Close to take them, and
// gives it back as it hides them; kero do leaves the keyboard where it is.
func TestToggleChangesFocus(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main"}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	s.Panes[0].Process, s.Panes[1].Process = "", ""
	a.prefs.Side = sideFiles
	panes := len(s.Panes)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()

	a.run("changes.toggle", nil)
	tt.Frame()
	tt.Frame()
	if v := a.viewOf(s); v == nil || !a.side.focused {
		t.Fatalf("Toggle Changes left viewer %+v, focused %v", v, a.side.focused)
	}
	a.run("pane.close", nil)
	tt.Frame()
	if a.viewOf(s) != nil || len(s.Panes) != panes {
		t.Fatalf("Close left viewer %+v, %d panes of %d", a.viewOf(s), len(s.Panes), panes)
	}

	// Toggling it away gives the keyboard back.
	a.prefs.Side = sideFiles
	a.run("changes.toggle", nil)
	tt.Frame()
	tt.Frame()
	a.run("changes.toggle", nil)
	tt.Frame()
	tt.Frame()
	if a.viewOf(s) != nil || a.side.focused {
		t.Fatalf("Toggle Changes again left viewer %+v, focused %v", a.viewOf(s), a.side.focused)
	}

	// In an app of its own: no terminal here takes the keyboard back above.
	a = testApp(t)
	s = &a.st.Sessions[0]
	s.Panes[0].Git, s.Panes[1].Git = g, g
	a.prefs.Side = sideFiles
	tt = ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	a.from = a.local()
	a.run("changes.toggle", nil)
	a.from = nil
	tt.Frame()
	tt.Frame()
	if a.viewOf(s) == nil || a.side.focused {
		t.Fatalf("kero do left viewer %+v, focused %v", a.viewOf(s), a.side.focused)
	}
}

// With the viewer beside it, Close takes a browser pane once the user is in
// its address bar, not the viewer they left. (A click in its page needs a
// real web view, which no test's window has.)
func TestCloseInBrowserBesideViewer(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", Dir: "/tmp", URL: "http://localhost:5173", Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Frame()
	fv := &fileView{pane: s.Panes[0].ID, path: "/tmp/a.go", rel: "a.go", fetched: time.Now()}
	a.showFile(fv, []byte("package a\n"))
	a.openView(s, sideFiles, fv)
	a.focusViewer()
	tt.Frame()
	tt.Frame()
	if !a.side.focused {
		t.Fatal("the viewer didn't take the keyboard")
	}
	v := a.viewOf(s)
	if err := tt.Click(locales.BrowserAddress()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.side.focused || a.focusOf(s) != "b" {
		t.Fatalf("after the click: viewer focused %v, pane %q", a.side.focused, a.focusOf(s))
	}
	a.run("pane.close", nil)
	tt.Frame()
	if s.PaneByID("b") != nil || a.viewOf(s) != v {
		t.Fatalf("Close left pane b %v, viewer %+v", s.PaneByID("b") != nil, a.viewOf(s))
	}
}

// The button above a file of Files wraps its long lines, then stops, as
// it does the diffs', keeping wrap-lines.
func TestWrapFile(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	fv := &fileView{pane: s.Panes[0].ID, path: "/tmp/a.go", rel: "a.go", fetched: time.Now()}
	a.showFile(fv, []byte("package a\n\nvar s = \""+strings.Repeat("a long line ", 30)+"\"\n"))
	a.openView(s, sideFiles, fv)
	tt.Frame()

	for _, want := range []bool{true, false} {
		label := locales.ViewerWrap()
		if !want {
			label = locales.ViewerNoWrap()
		}
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.set.WrapLines != want || fv.st.SoftWrap != want {
			t.Fatalf("clicking %q left wrap-lines %v, the file's wrap %v", label, a.set.WrapLines, fv.st.SoftWrap)
		}
		tt.Frame()
		row, long := fv.st.LineTop(1)-fv.st.LineTop(0), fv.st.LineTop(3)-fv.st.LineTop(2)
		if wrapped := long > row; wrapped != want {
			t.Errorf("with wrap-lines %v, the long line is %v high, a row %v", want, long, row)
		}
		shot(t, tt, map[bool]string{true: "file-wrapped", false: "file-unwrapped"}[want])
	}
}

func TestIsBinary(t *testing.T) {
	if isBinary([]byte("hello\nworld")) || !isBinary([]byte("PNG\x00\x01")) || isBinary(nil) {
		t.Error("isBinary is wrong")
	}
}

// The viewer's texts edit as VS Code's and Zed's do: a file of Files and
// the new side of a diff of Changes keep their edits, a dot on the
// viewer's close button saying so, until Save writes them through kerod,
// as the file was read and with its line endings, agent working or not.
// Closing the viewer with edits asks to save them, and a file changed on
// disk meanwhile asks to overwrite it or discard them.
func TestViewerEdits(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main", Files: 1, Added: 1}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	writes := make(chan proto.FSWriteParams, 4)
	cl, stop := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method != "fs.write" {
			return nil, errors.New("unexpected method " + method)
		}
		var p proto.FSWriteParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		writes <- p
		return git.Hash([]byte(p.Text)), nil
	})
	defer stop()
	written := func() (proto.FSWriteParams, bool) {
		select {
		case p := <-writes:
			return p, true
		case <-time.After(time.Second):
			return proto.FSWriteParams{}, false
		}
	}

	// A file of Files, with \r\n line endings, an agent working.
	a.prefs.Side = sideFiles
	s.Panes[1].Status = proto.Working
	fv := &fileView{pane: "a", path: "/tmp/demo/main.go", rel: "main.go", fetched: time.Now(), stamp: gitStamp(g)}
	fv.hash = git.Hash([]byte("package main\r\n"))
	showFileText(fv, "package main\r\n")
	a.openView(s, sideFiles, fv)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	a.local().cl = cl
	if fv.st.ReadOnly {
		t.Fatal("the file is read only")
	}
	if tt.HasText("Unsaved changes") {
		t.Error("the viewer says it has unsaved changes before any edit")
	}
	fv.st.Buffer.Edit(editor.Edit{Start: 12, End: 12, Text: "\n\nfunc main() {}"})
	tt.Frame()
	tt.Advance(5 * time.Second)
	tt.Frame()
	select {
	case p := <-writes:
		t.Fatalf("wrote %q unsaved", p.Text)
	default:
	}
	if !tt.HasText("Unsaved changes") {
		t.Error("the viewer doesn't say it has unsaved changes")
	}
	shot(t, tt, "viewer-unsaved")
	a.paneRef("side-card").Focus() // as the user typing there has it
	tt.Frame()
	read := fv.hash // as the write's answer, run in a frame, changes it
	tt.Key(ui.Cmd, ui.KeyS)
	p, ok := written()
	if !ok {
		t.Fatal("Save didn't write the file")
	}
	if want := (proto.FSWriteParams{Path: "/tmp/demo/main.go", Hash: read, Text: "package main\r\n\r\nfunc main() {}\r\n"}); p != want {
		t.Errorf("wrote %+v, want %+v", p, want)
	}
	s.Panes[1].Status = proto.Idle

	// Closing the viewer with edits asks to save them; Don't Save drops
	// them, for the file as it is.
	fv.edits.saving = false // as the write answered, which no window runs here
	fv.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// Main.\n"})
	a.closeView(s)
	tt.Frame()
	if a.viewOf(s) == nil || !tt.HasText("Do you want to save the changes you made to “main.go”?") {
		t.Fatalf("closing with edits didn't ask to save them; texts %q", tt.Texts())
	}
	shot(t, tt, "viewer-ask-save")
	if err := tt.Click("Don't Save"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.viewOf(s) != nil || fv.st.Buffer.Modified() || !fv.fetched.IsZero() {
		t.Error("Don't Save didn't close the viewer, dropping the edits for the file read anew")
	}

	// The new side of a diff edits, and Save writes the one focused.
	a.local().cl = nil
	a.toggleSide(sideChanges)
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseHead)
	ch.base, ch.scope, ch.stamp = git.BaseHead, scope, gitStamp(g)
	ch.res = &proto.GitChanges{Root: "/tmp/demo", Files: []proto.GitFile{{Path: "pkg/a.go", Status: "modified", Added: 1}}}
	ch.fetched = time.Now()
	tt.Frame()
	sf := ch.files[scope+"|pkg/a.go"]
	if sf == nil {
		t.Fatal("no diff of pkg/a.go")
	}
	a.showText(sf, proto.GitText{Old: "package a\n", New: "package a\n\nvar x = 1\n", Hash: "h1"})
	tt.Frame()
	if !sf.d.Editable {
		t.Fatal("the diff's new side doesn't edit")
	}
	a.local().cl = cl
	sf.d.New.Edit(editor.Edit{Start: 0, End: 0, Text: "// A.\n"})
	ch.focus = sf
	a.paneRef("side-card").Focus()
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyS)
	p, ok = written()
	if !ok {
		t.Fatal("Save didn't write the diff's edit")
	}
	if want := (proto.FSWriteParams{Path: "/tmp/demo/pkg/a.go", Hash: "h1", Text: "// A.\npackage a\n\nvar x = 1\n"}); p != want {
		t.Errorf("wrote %+v, want %+v", p, want)
	}

	// Text read anew doesn't go over edits not saved.
	sf.edits.saving = false
	sf.d.New.Edit(editor.Edit{Start: 0, End: 0, Text: "x"})
	a.showText(sf, proto.GitText{Old: "package a\n", New: "package b\n", Hash: "h2"})
	if got := sf.d.New.String(); got[:1] != "x" || sf.hash == "h2" {
		t.Errorf("reading the file anew went over the user's edit: %q, hash %q", got, sf.hash)
	}
	if n := len(a.allTargets()); n != 1 {
		t.Errorf("%d texts to save as the app quits, want 1", n)
	}

	// Its file changed meanwhile: Discard Changes drops the edits.
	a.local().cl = nil
	a.askOverwrite(a.diffTarget(sf), func(bool) {})
	tt.Frame()
	if !tt.HasText("“a.go” changed on disk") {
		t.Fatalf("no question of the file changed; texts %q", tt.Texts())
	}
	shot(t, tt, "viewer-conflict")
	if err := tt.Click("Discard Changes"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if sf.d.New.Modified() {
		t.Error("Discard Changes kept the edits")
	}
}

// Edits not saved outlast the app, as VS Code's hot exit keeps them: they
// are backed up as the user pauses typing and as the app quits, which
// asks nothing, and come back, not saved still, as their files' texts
// load again, with the viewers that showed them.
func TestUnsavedOutlastQuit(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main", Files: 1, Added: 1}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	a.prefs.Side = sideFiles
	disk := []byte("package main\r\n")
	fv := &fileView{pane: "a", path: "/tmp/demo/main.go", rel: "main.go", fetched: time.Now(), stamp: gitStamp(g)}
	a.showFile(fv, disk)
	a.openView(s, sideFiles, fv)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	fv.st.Buffer.Edit(editor.Edit{Start: 12, End: 12, Text: "\n\nfunc main() {}"})
	tt.Frame()

	// Backed up a moment after the user pauses.
	if _, err := os.Stat(backupPath()); err == nil {
		t.Fatal("backed up as the user typed")
	}
	tt.Advance(2 * time.Second)
	tt.Frame()
	var vb viewerBackup
	if b, err := os.ReadFile(backupPath()); err == nil {
		json.Unmarshal(b, &vb)
	}
	want := unsavedText{Kind: sideFiles, Pane: "a", Path: "/tmp/demo/main.go", Hash: git.Hash(disk), Text: "package main\r\n\r\nfunc main() {}\r\n"}
	if len(vb.Unsaved) != 1 || vb.Unsaved[0] != want {
		t.Fatalf("backed up %+v, want %+v", vb.Unsaved, want)
	}
	if st, _ := os.Stat(backupPath()); st.Mode().Perm() != 0o600 {
		t.Errorf("the backup is %v, readable by others", st.Mode().Perm())
	}

	// Quitting asks nothing.
	a.backupOnQuit()
	if a.confirm != nil {
		t.Fatalf("quitting asked %q", a.confirm.title)
	}

	// The next launch shows the viewer as it was, and the edit as the
	// file loads, not saved, with the hash of the file it was made on.
	b := newApp()
	v := b.side.views[s.ID]
	if v == nil || v.kind != sideFiles || v.file == nil || v.file.path != "/tmp/demo/main.go" || v.file.rel != "main.go" {
		t.Fatalf("the viewer came back as %+v", v)
	}
	now := []byte("package main\r\n\r\n// Changed meanwhile.\r\n")
	b.showFile(v.file, now)
	if got := bufferText(v.file.st.Buffer); got != want.Text || !v.file.st.Buffer.Modified() {
		t.Errorf("the edit came back as %q, modified %v", got, v.file.st.Buffer.Modified())
	}
	if v.file.hash != git.Hash(disk) {
		t.Error("the edit came back without the hash of the file it was made on, saving over the change made meanwhile")
	}
	if len(b.side.backup.unsaved) != 0 {
		t.Error("the edit stayed in the backup as it came back")
	}

	// A diff's too, as its text loads.
	b.side.backup.unsaved = map[string]unsavedText{}
	u := unsavedText{Kind: sideChanges, Pane: "a", Path: "/tmp/demo/pkg/a.go", Hash: "h0", Text: "package a // edited\n"}
	b.side.backup.unsaved[u.key()] = u
	sf := &sideFile{pane: "a", base: git.BaseHead, root: "/tmp/demo", f: proto.GitFile{Path: "pkg/a.go"}}
	b.showText(sf, proto.GitText{Old: "package a\n", New: "package a\n\nvar x = 1\n", Hash: "h1"})
	if got := sf.d.New.String(); got != u.Text || !sf.d.New.Modified() || sf.hash != "h0" {
		t.Errorf("the diff's edit came back as %q, hash %q", got, sf.hash)
	}
}

// The Files tab makes files and folders, renames and deletes them: New
// File in its header puts a field for the name at the top of the root,
// which Return makes, through kerod; a row's menu offers the same in its
// folder, Rename and Delete; a file renamed keeps its viewer, and its
// edits, at its new path; Delete asks first.
func TestFilesTabChanges(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main"}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	a.prefs.Side = sideFiles
	h := a.hostOf(s.ID)
	fs := &a.side.files
	fs.dirs = map[string]*dirList{
		filesKey(h, "/tmp/demo"):          {entries: []proto.FSEntry{{Name: "internal", Dir: true}, {Name: "main.go"}}},
		filesKey(h, "/tmp/demo/internal"): {entries: []proto.FSEntry{{Name: "auth.go"}}},
	}
	calls := make(chan [2]string, 4)
	cl, stop := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method == "fs.list" {
			return nil, errors.New("the tree keeps what it listed")
		}
		if method == "fs.stat" {
			// The file made, opened in the viewer.
			return proto.FSStat{}, nil
		}
		calls <- [2]string{method, string(raw)}
		return true, nil
	})
	defer stop()
	call := func() [2]string {
		select {
		case c := <-calls:
			return c
		case <-time.After(time.Second):
			return [2]string{}
		}
	}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	a.local().cl = cl

	if err := tt.Click("New File…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if fs.edit == nil || fs.edit.op != editNewFile || fs.edit.dir != "/tmp/demo" {
		t.Fatalf("New File put %+v", fs.edit)
	}
	tt.Type("docs/notes.md")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if c := call(); c[0] != "fs.create" || c[1] != `{"path":"/tmp/demo/docs/notes.md"}` {
		t.Errorf("New File called %q", c)
	}
	shot(t, tt, "files-new")

	// A row's menu.
	if err := tt.RightClick("internal"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Contains(got, "New File…") || !slices.Contains(got, "New Folder…") || !slices.Contains(got, "Rename…") || !slices.Contains(got, "Delete…") {
		t.Fatalf("the folder's menu is %q", got)
	}
	if err := tt.ChooseMenuItem("New Folder…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if fs.edit == nil || fs.edit.op != editNewFolder || fs.edit.dir != "/tmp/demo/internal" {
		t.Fatalf("New Folder in internal put %+v", fs.edit)
	}
	tt.Type("handlers")
	tt.Frame()
	shot(t, tt, "files-name")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if fs.edit != nil {
		t.Error("Escape left the field")
	}

	// Rename, in the entry's place.
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Rename…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if fs.edit == nil || fs.edit.op != editRename || fs.edit.name != "main.go" {
		t.Fatalf("Rename put %+v", fs.edit)
	}
	fs.edit.name = "app.go"
	a.commitName()
	if c := call(); c[0] != "fs.rename" || c[1] != `{"from":"/tmp/demo/main.go","to":"/tmp/demo/app.go"}` {
		t.Errorf("Rename called %q", c)
	}
	fs.edit = &nameEdit{op: editRename, pane: "a", dir: "/tmp/demo", path: "/tmp/demo/main.go", oldRel: "main.go", name: "a/b.go"}
	a.commitName()
	if c := call(); c[0] != "" {
		t.Errorf("a name with a slash renamed: %q", c)
	}

	// The viewer follows its file, and the folders theirs.
	fv := &fileView{pane: "a", path: "/tmp/demo/internal/auth.go", rel: "internal/auth.go"}
	showFileText(fv, "package internal\n")
	fv.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// x\n"})
	a.openView(s, sideFiles, fv)
	fs.open[filesKey(h, "/tmp/demo/internal")] = true
	a.renamed(h, "/tmp/demo/internal", "/tmp/demo/pkg", "internal", "pkg")
	if fv.path != "/tmp/demo/pkg/auth.go" || fv.rel != "pkg/auth.go" || !fv.st.Buffer.Modified() {
		t.Errorf("the viewer's file went to %q (%q), modified %v", fv.path, fv.rel, fv.st.Buffer.Modified())
	}
	if !fs.open[filesKey(h, "/tmp/demo/pkg")] || fs.open[filesKey(h, "/tmp/demo/internal")] {
		t.Error("the folder renamed didn't stay unfolded")
	}

	// Delete asks first.
	tt.Frame()
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Delete “main.go”?") || !tt.HasText("Move to Trash") {
		t.Fatalf("Delete didn't ask; texts %q", tt.Texts())
	}
	shot(t, tt, "files-delete")
}

// The app opening with the right sidebar shown shows it at once, not
// sliding in as kerod's state comes; showing it later slides it in.
func TestRightSidebarOpensShown(t *testing.T) {
	a := testApp(t)
	a.prefs.Side = sideChanges
	a.loaded = false // kerod's state not come yet
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	if a.side.barW != 0 {
		t.Fatalf("the sidebar shows with no session, %v wide", a.side.barW)
	}
	a.loaded = true
	tt.Frame()
	if a.side.barW != a.rightWidth() {
		t.Errorf("as the state came, the sidebar was %v wide, not at once %v", a.side.barW, a.rightWidth())
	}

	a.toggleRight()
	tt.Advance(time.Second)
	tt.Frame()
	a.toggleRight()
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	if w := a.side.barW; w <= 0 || w >= a.rightWidth() {
		t.Errorf("showing the sidebar didn't slide it in: %v wide a moment after", w)
	}
}

// The right sidebar's edge takes the pointer a few pixels to either side
// of it, over the cards and over the sidebar, and a drag there resizes it.
func TestRightSidebarEdge(t *testing.T) {
	a := testApp(t)
	a.prefs.Side = sideFiles
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	edge := 1280 - a.side.barW
	for dx := float32(-4); dx <= 4; dx++ {
		tt.Move(edge+dx, 400)
		tt.Frame()
		if c := tt.Cursor(); c != ui.CursorResizeColumn {
			t.Errorf("%v from the edge, the cursor is %v", dx, c)
		}
	}
	w := a.rightWidth()
	tt.Press(edge, 400)
	tt.Move(edge-40, 400)
	tt.Release(edge-40, 400)
	tt.Frame()
	if got := a.rightWidth(); got != w+40 {
		t.Errorf("dragging the edge 40 left made the sidebar %v wide, want %v", got, w+40)
	}
}

// In a window too narrow for the panes', the viewer's and the right
// sidebar's widths, the viewer shares the room left with the panes rather
// than running under the sidebar.
func TestNarrowWindowKeepsViewerBesideSidebar(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	a.prefs.SidebarHidden = true
	a.prefs.Side = sideFiles
	fv := &fileView{pane: "a", path: "/tmp/main.go", rel: "main.go", fetched: time.Now()}
	a.showFile(fv, []byte("package main\n"))
	a.openView(s, sideFiles, fv)
	var card, row ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		card, row = a.paneRef("side-card").Bounds(c), a.paneRef("with-side").Bounds(c)
	}, 560, 600)
	tt.Frame()
	a.toggleRight() // shown anyway, as the window crowds it
	for range 3 {
		tt.Frame()
		tt.Advance(time.Second)
	}
	tt.Frame()
	if a.side.barW == 0 {
		t.Fatal("the sidebar didn't show")
	}
	if edge := float32(560) - a.side.barW; card.X+card.W > edge+0.5 {
		t.Errorf("the viewer runs to %v, under the sidebar from %v", card.X+card.W, edge)
	}
	if card.W <= 0 || card.W > row.W/2+1 {
		t.Errorf("the viewer is %v wide in a row of %v", card.W, row.W)
	}
	shot(t, tt, "narrow")
}

// A window too narrow for the right sidebar beside the cards hides it,
// shown as it is, with the button that shows it, which shows it anyway;
// the window widening shows it again, and narrowing hides it again.
func TestRightSidebarMakesRoom(t *testing.T) {
	a := testApp(t)
	a.prefs.Side = sideFiles
	tt := ui.NewTester(a.view, 1280, 720)
	settle := func() {
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
	}
	settle()
	if a.side.barW == 0 {
		t.Fatal("the sidebar didn't show in a wide window")
	}
	show := a.tip("Show Right Sidebar", "rightSidebar.toggle")

	tt.SetSize(640, 720)
	settle()
	if a.side.barW != 0 || a.prefs.Side != sideFiles {
		t.Fatalf("a narrow window left the sidebar %v wide, shown %q", a.side.barW, a.prefs.Side)
	}
	if err := tt.Click(show); err != nil {
		t.Fatal(err)
	}
	settle()
	if a.side.barW == 0 {
		t.Fatal("Show Right Sidebar didn't show it in a narrow window")
	}
	if tt.HasText("Show Right Sidebar") {
		t.Error("the button stayed with the sidebar shown")
	}

	tt.SetSize(1280, 720)
	settle()
	tt.SetSize(640, 720)
	settle()
	if a.side.barW != 0 {
		t.Error("narrowing the window again didn't hide the sidebar")
	}
	tt.SetSize(1280, 720)
	settle()
	if a.side.barW != a.rightWidth() {
		t.Error("widening the window didn't show the sidebar again")
	}
}

// As the window resizes, the viewer takes its width in the same frame,
// never the last one's, which would shake its edge and its text.
func TestViewerFollowsResize(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	a.prefs.Side = sideFiles
	a.prefs.SideWidth = 600 // wider than the room: the panes' least decides
	fv := &fileView{pane: "a", path: "/tmp/main.go", rel: "main.go", fetched: time.Now()}
	a.showFile(fv, []byte("package main\n"))
	a.openView(s, sideFiles, fv)
	var card, row ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		card, row = a.paneRef("side-card").Bounds(c), a.paneRef("with-side").Bounds(c)
	}, 1300, 720)
	for range 3 {
		tt.Frame()
		tt.Advance(time.Second)
	}
	for _, w := range []int{1320, 1340, 1310, 1290, 1330} {
		tt.SetSize(w, 720)
		tt.Frame()
		if want := row.W - minPanes; math.Abs(float64(card.W-want)) > 0.5 {
			t.Errorf("at %d wide the viewer is %v wide, want %v as the row is %v", w, card.W, want, row.W)
		}
	}
}

// testRead puts text in a file's view as read from its file.
func (fv *fileView) testRead(text string) {
	fv.fetched = time.Now()
	showFileText(fv, text)
}

// An editor pane shows its file in the editor the viewer shows files in,
// with its bar's actions; its title's menu copies its path; the text size
// keys zoom the file while it has the keyboard; and closing it with edits
// asks first.
func TestEditorPane(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/main.go", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	if !tt.HasText("main.go") {
		t.Errorf("no file's name in the header; texts %q", tt.Texts())
	}
	if a.terms["b"] != nil {
		t.Error("an editor pane attached a terminal")
	}
	f := a.edits["b"]
	if f == nil || f.path != "/tmp/main.go" || f.pane != "b" {
		t.Fatalf("the pane's file is %+v", f)
	}
	f.testRead("package main\n")
	tt.Frame()
	if f.st == nil {
		t.Fatal("no editor")
	}
	if err := tt.Click("Wrap Lines"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.set.WrapLines || !f.st.SoftWrap {
		t.Error("Wrap Lines left the lines unwrapped")
	}
	shot(t, tt, "editor-pane")

	// Its title's menu copies its path, whole or from the session's
	// folder, and opens it outside kero.
	for _, c := range []struct{ item, want string }{{"Copy Path", "/tmp/main.go"}, {"Copy Relative Path", "main.go"}} {
		if err := tt.RightClick("main.go"); err != nil {
			t.Fatal(err)
		}
		if items := tt.Menu(); !slices.Contains(items, "Open in Editor") || !slices.Contains(items, "Show in "+fileManager()) {
			t.Fatalf("title's menu %q", items)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := tt.Clipboard(); got != c.want {
			t.Errorf("%s copied %q", c.item, got)
		}
	}

	// The text size keys zoom the file while its pane has the keyboard,
	// and the terminals' text while a terminal has it; the bar's buttons
	// zoom the file too.
	term, size := a.set.FontSize, f.st.Font.Size
	a.setFocus(s, "b")
	a.run("text.bigger", nil)
	tt.Frame()
	if a.prefs.FileZoom != 1 || f.st.Font.Size != size+1 || a.set.FontSize != term {
		t.Errorf("in the file, Bigger made it %v (zoom %d) and the terminals %v", f.st.Font.Size, a.prefs.FileZoom, a.set.FontSize)
	}
	if err := tt.Click("Smaller Text (" + a.keys("text.smaller") + ")"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.prefs.FileZoom != 0 || f.st.Font.Size != size {
		t.Errorf("Smaller Text left the file at %v", f.st.Font.Size)
	}
	a.setFocus(s, "a")
	a.run("text.bigger", nil)
	if a.prefs.FileZoom != 0 || a.set.FontSize != term+1 {
		t.Errorf("in a terminal, Bigger zoomed the file (%d), the terminals to %v", a.prefs.FileZoom, a.set.FontSize)
	}

	// Its edits are kept as the app quits, as the viewer's are.
	f.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// x\n"})
	if ts := a.allTargets(); len(ts) != 1 || ts[0].path != "/tmp/main.go" {
		t.Errorf("texts to keep as the app quits: %+v", ts)
	}
	a.closePane(s, "b", true)
	if a.confirm == nil || s.PaneByID("b") == nil {
		t.Fatal("closed an editor with edits unasked")
	}
	a.confirm.run()
	if s := a.session("s1"); s.PaneByID("b") != nil || a.edits["b"] != nil {
		t.Error("the editor stayed as the user closed it")
	}
}

// A Markdown file previews as it reads, in place of its text, and back;
// another file has no preview.
func TestMarkdownPreview(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/README.md", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	f := a.edits["b"]
	f.testRead("# Title\n\nSome **bold** text.\n")
	tt.Frame()
	if tt.HasText("Title") && !tt.HasText("# Title") {
		t.Error("the text shows rendered before the preview")
	}
	if err := tt.Click("Preview Markdown"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !f.preview || !tt.HasText("Title") || tt.HasText("Wrap Lines") {
		t.Errorf("no preview; texts %q", tt.Texts())
	}
	shot(t, tt, "markdown-preview")

	// The preview has no editor to take the keyboard; the pane takes it
	// still, as the pointer moves into it.
	a.set.FocusFollowsMouse = true
	ar, _ := tt.Find("fish")
	br, _ := tt.Find("Title")
	for _, at := range [][2]float32{{ar.X + 40, 340}, {br.X + 40, br.Y + 40}, {br.X + 60, br.Y + 40}} {
		tt.Move(at[0], at[1])
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	if got := a.focusOf(s); got != "b" || !tt.Focused("Title") {
		t.Errorf("into the preview, focus %q, want b, with the keyboard", got)
	}
	a.set.FocusFollowsMouse = false

	// Without it, a click in the preview focuses the pane.
	tt.ClickAt(ar.X+40, 340)
	tt.Frame()
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("focus %q before the click, want a", got)
	}
	tt.ClickAt(br.X+40, 500) // below the text, in the preview
	tt.Frame()
	if got := a.focusOf(s); got != "b" {
		t.Errorf("clicked in the preview, focus %q, want b", got)
	}

	if err := tt.Click("Preview Markdown"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.preview || !tt.HasText("Wrap Lines") {
		t.Error("the preview stayed")
	}

	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/main.go", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	delete(a.edits, "b")
	tt.Frame()
	a.edits["b"].testRead("package main\n")
	tt.Frame()
	if tt.HasText("Preview Markdown") {
		t.Error("a Go file has a Markdown preview")
	}
}

// An editor pane of a file not there yet opens empty, as a new file, and
// its save makes it, through kerod, then writes it.
func TestEditorNewFile(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/plan.md", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	calls := make(chan string, 4)
	cl, stop := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "fs.create":
			calls <- method + " " + string(raw)
			return nil, nil
		case "fs.write":
			var p proto.FSWriteParams
			json.Unmarshal(raw, &p)
			calls <- method + " " + p.Hash + " " + p.Text
			return git.Hash([]byte(p.Text)), nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	defer stop()
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	f := a.edits["b"]
	if f == nil || !f.fresh {
		t.Fatalf("the pane's file is %+v", f)
	}
	f.absent = true
	f.testRead("")
	tt.Frame()
	if f.st == nil || f.err != "" || f.hash != "" || !tt.HasText("New file") {
		t.Fatalf("a new file opened as %+v; texts %q", f, tt.Texts())
	}
	a.local().cl = cl
	f.st.Buffer.Edit(editor.Edit{Text: "# Plan\n"})
	a.saveTargets([]editTarget{a.fileTarget(f)}, nil)
	next := func() string {
		select {
		case c := <-calls:
			return c
		case <-time.After(2 * time.Second):
			return ""
		}
	}
	if got := next(); got != `fs.create {"path":"/tmp/plan.md"}` {
		t.Errorf("the save called %q first", got)
	}
	if got, want := next(), "fs.write "+git.Hash(nil)+" # Plan\n"; got != want {
		t.Errorf("then %q, want %q", got, want)
	}
	for range 50 {
		tt.Frame()
		if !f.absent {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.absent || f.modified() || tt.HasText("New file") {
		t.Error("a saved new file still shows as new")
	}
}

// A new file's pane, not yet typed in, offers to open a file instead; one
// typed in doesn't.
func TestNewFilePaneOpenFile(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/untitled.txt", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	f := &fileView{pane: "b", path: "/tmp/untitled.txt", fresh: true, absent: true, fetched: time.Now(), st: &editor.State{Buffer: editor.NewBuffer("")}}
	a.edits = map[string]*fileView{"b": f}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	if !tt.HasText("Open File…") {
		t.Errorf("a new file's pane can't open a file; texts %q", tt.Texts())
	}
	shot(t, tt, "new-file-pane")
	f.st.Buffer.Edit(editor.Edit{Text: "hi"})
	tt.Frame()
	if tt.HasText("Open File…") {
		t.Error("a new file typed in offers to open another in its place")
	}
}

// paneCalls makes the local host a kerod that lists one file, main.go, in
// every folder, and tells the splits and pane.file calls it takes, as
// "split <pane> <direction> <file>" and "file <pane> <file>".
func paneCalls(t *testing.T, a *App) <-chan string {
	t.Helper()
	calls := make(chan string, 8)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "fs.list":
			return proto.FSList{Entries: []proto.FSEntry{{Name: "src", Dir: true}, {Name: "main.go"}}}, nil
		case "split":
			var p proto.CreateParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			calls <- "split " + p.Split + " " + p.Direction + " " + p.File + p.Dir
			return nil, errors.New("no panes in a test")
		case "pane.file":
			var p proto.FileParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			calls <- "file " + p.ID + " " + p.File
			return true, nil
		}
		return nil, errors.New("unexpected method " + method)
	})
	a.local().cl = cl
	return calls
}

// nextCall returns the next call paneCalls tells.
func nextCall(t *testing.T, calls <-chan string) string {
	t.Helper()
	select {
	case c := <-calls:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("no call came")
		return ""
	}
}

// filesBeside shows the Files tab of /tmp/demo, beside pane a and an
// editor pane b of its README.md, its root listed, in view.
func filesBeside(t *testing.T, a *App, view func(*ui.Context)) *ui.Tester {
	t.Helper()
	s := &a.st.Sessions[0]
	s.Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/demo/README.md", Dir: "/tmp/demo", Status: proto.Idle, Progress: -1}
	a.prefs.Side = sideFiles
	tt := ui.NewTester(view, 1280, 720)
	for range 50 {
		tt.Frame()
		if tt.HasText("main.go") {
			return tt
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the tree listed nothing; texts %q", tt.Texts())
	return nil
}

// A file's menu opens it in an editor pane of its own, and shows its
// history; a folder's opens a terminal in it, and no pane; both open on
// this machine and copy their paths, from all of the row that shows
// hovered.
func TestFilesMenus(t *testing.T) {
	a := testApp(t)
	calls := paneCalls(t, a)
	tt := filesBeside(t, a, a.view)
	if err := tt.RightClick("src"); err != nil {
		t.Fatal(err)
	}
	items := tt.Menu()
	for _, want := range []string{"New File…", "New Folder…", "New Terminal Here", "Rename…", "Delete…", "Open in Editor", "Copy Path", "Copy Relative Path"} {
		if !slices.Contains(items, want) {
			t.Errorf("a folder's menu has no %q: %q", want, items)
		}
	}
	if slices.Contains(items, "Open in Pane") || slices.Contains(items, "Show History") {
		t.Errorf("a folder's menu opens it in a pane, or shows its history: %q", items)
	}
	if err := tt.ChooseMenuItem("New Terminal Here"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := nextCall(t, calls); got != "split a right /tmp/demo/src" {
		t.Errorf("New Terminal Here called %q", got)
	}

	r, ok := tt.Find("main.go")
	if !ok {
		t.Fatalf("no main.go; texts %q", tt.Texts())
	}
	for _, y := range []float32{r.Y - 2, r.Y + r.H + 2} {
		tt.RightClickAt(r.X+r.W/2, y)
		items := tt.Menu()
		for _, want := range []string{"Open in Default App", "Open in Pane", "Show History", "Open in Editor", "Copy Relative Path", "Rename…"} {
			if !slices.Contains(items, want) {
				t.Errorf("a right-click at %v, beside main.go at %v to %v, has no %q: %q", y, r.Y, r.Y+r.H, want, items)
			}
		}
		if slices.Contains(items, "Open in New Pane") || slices.Contains(items, "Preview") {
			t.Errorf("a file's menu %q", items)
		}
		tt.CloseMenu()
		tt.Frame()
	}
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open in Pane"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := nextCall(t, calls); got != "split a right /tmp/demo/main.go" {
		t.Errorf("Open in Pane called %q", got)
	}
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Copy Relative Path"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != "main.go" {
		t.Errorf("Copy Relative Path copied %q", got)
	}

	// A click opens it in the viewer, in no pane.
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if v := a.viewOf(&a.st.Sessions[0]); v == nil || v.kind != sideFiles || v.file == nil || v.file.path != "/tmp/demo/main.go" {
		t.Errorf("a click opened %+v", v)
	}
	select {
	case c := <-calls:
		t.Errorf("a click called %q", c)
	default:
	}
}

// A file dragged from the tree onto a pane opens beside it, on the side it
// is dropped at, or, in the middle of an editor pane, in its place: a
// terminal's middle is no place for it.
func TestFilesDragToPane(t *testing.T) {
	a := testApp(t)
	calls := paneCalls(t, a)
	cards := map[string]ui.Rect{}
	tt := filesBeside(t, a, func(c *ui.Context) {
		a.view(c)
		for _, id := range []string{"a", "b"} {
			cards[id] = a.paneRef("card-" + id).Bounds(c)
		}
	})
	drop := func(x, y float32, label string, want string) {
		t.Helper()
		r, ok := tt.Find("main.go")
		if !ok {
			t.Fatalf("no main.go; texts %q", tt.Texts())
		}
		fx, fy := r.X+r.W/2, r.Y+r.H/2
		// Pressed again so soon, the file would be double-clicked.
		tt.Advance(time.Second)
		tt.Press(fx, fy)
		tt.Frame()
		tt.Move(fx-12, fy)
		tt.Frame()
		tt.Move(x, y)
		tt.Frame()
		if !tt.HasText(label) {
			t.Errorf("dragged to %v, %v: no %q; texts %q", x, y, label, tt.Texts())
		}
		if label != "Open Here" && tt.HasText("Open Here") {
			t.Errorf("dragged to %v, %v: the file would open in the pane's place", x, y)
		}
		tt.Release(x, y)
		tt.Frame()
		if got := nextCall(t, calls); got != want {
			t.Errorf("dropped at %v, %v: called %q, want %q", x, y, got, want)
		}
	}
	b := cards["b"]
	drop(b.X+b.W/2, b.Y+b.H/2, "Open Here", "file b /tmp/demo/main.go")
	drop(b.X+b.W-20, b.Y+b.H/2, "Split Right", "split b right /tmp/demo/main.go")
	t2 := cards["a"]
	drop(t2.X+t2.W/2, t2.Y+t2.H*0.45, "Split Up", "split a up /tmp/demo/main.go")
	if v := a.viewOf(&a.st.Sessions[0]); v != nil {
		t.Errorf("a drag opened the viewer on %+v", v.file)
	}
}

// The filter shows what kerod finds in all of the folder, in the folders
// it is in; a click on a file found opens it in the viewer; Esc, or Clear
// Filter, shows the whole tree again.
func TestFilesFilter(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	a.prefs.Side = sideFiles
	finds := make(chan proto.FSFindParams, 4)
	cl, stop := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "fs.list":
			return proto.FSList{Entries: []proto.FSEntry{{Name: "src", Dir: true}, {Name: "main.go"}}}, nil
		case "fs.find":
			var p proto.FSFindParams
			json.Unmarshal(raw, &p)
			select {
			case finds <- p:
			default:
			}
			if p.Query == "util" {
				return proto.FSFind{Entries: []proto.FSFound{{Path: "src/util", Dir: true}, {Path: "src/util/util.go"}, {Path: "lib/util.c"}}}, nil
			}
			return proto.FSFind{}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	defer stop()
	a.local().cl = cl
	tt := ui.NewTester(a.view, 1280, 720)
	settle := func() {
		for range 30 {
			tt.Advance(50 * time.Millisecond)
			tt.Frame()
			time.Sleep(5 * time.Millisecond)
		}
	}
	settle()
	if err := tt.Click("Filter"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Type("util")
	settle()
	select {
	case p := <-finds:
		if p.Dir != "/tmp/demo" || p.Query != "util" {
			t.Errorf("found with %+v", p)
		}
	default:
		t.Fatal("the filter asked kerod nothing")
	}
	// The folder found, which holds a file found, shows once, as its.
	for _, want := range []string{"src/util", "util.go", "lib", "util.c"} {
		if !tt.HasText(want) {
			t.Errorf("the filter shows no %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText("main.go") {
		t.Error("the filter shows main.go")
	}
	shot(t, tt, "files-filter")

	// Enter opens the first file found, as the tree shows them.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if v := a.viewOf(s); v == nil || v.file == nil || v.file.path != "/tmp/demo/src/util/util.go" || v.file.rel != "src/util/util.go" {
		t.Errorf("Enter opened %+v", v)
	}
	if err := tt.Click("util.c"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if v := a.viewOf(s); v == nil || v.file == nil || v.file.path != "/tmp/demo/lib/util.c" || v.file.rel != "lib/util.c" {
		t.Errorf("a click on util.c opened %+v", v)
	}
	if err := tt.Click("Filter"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	settle()
	if a.side.files.find.query != "" || !tt.HasText("main.go") {
		t.Fatalf("Esc left the filter at %q", a.side.files.find.query)
	}

	if err := tt.Click("Filter"); err != nil {
		t.Fatal(err)
	}
	tt.Type("zzz")
	settle()
	if !tt.HasText("Nothing matches “zzz”") {
		t.Errorf("no match shows %q", tt.Texts())
	}
	if err := tt.Click("Clear Filter"); err != nil {
		t.Fatal(err)
	}
	settle()
	if a.side.files.find.query != "" || !tt.HasText("main.go") {
		t.Errorf("Clear Filter left %q", a.side.files.find.query)
	}
}

// kero do files.show shows a path in the Files tab: the folders it is in
// open, and a file in the viewer; a relative path is the tab's folder's,
// and one outside it says so.
func TestShowFiles(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	cl, stop := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method == "fs.stat" {
			var p proto.FSStatParams
			json.Unmarshal(raw, &p)
			return proto.FSStat{Path: p.Path, Exists: true, Dir: p.Path == "/tmp/demo/src"}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	defer stop()
	a.local().cl = cl
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	h := a.local()
	wait := func(ok func() bool) bool {
		for range 100 {
			tt.Frame()
			if ok() {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}
	a.run("files.show", map[string]string{"path": "src/util/a.go"})
	if !wait(func() bool { v := a.viewOf(s); return v != nil && v.file != nil }) {
		t.Fatal("files.show opened nothing")
	}
	if v := a.viewOf(s); a.prefs.Side != sideFiles || v.file.path != "/tmp/demo/src/util/a.go" || v.file.rel != "src/util/a.go" {
		t.Errorf("files.show showed %q, the viewer %q (%q)", a.prefs.Side, v.file.path, v.file.rel)
	}
	if fs := &a.side.files; !fs.open[filesKey(h, "/tmp/demo/src")] || !fs.open[filesKey(h, "/tmp/demo/src/util")] {
		t.Errorf("the folders the file is in stayed closed: %v", fs.open)
	}
	a.closeView(s)
	a.run("files.show", map[string]string{"path": "/tmp/demo/src"})
	if !wait(func() bool { return a.side.files.open[filesKey(h, "/tmp/demo/src")] }) || a.viewOf(s) != nil {
		t.Error("files.show of a folder didn't open it, or opened the viewer")
	}
	a.run("files.show", map[string]string{"path": "/etc/passwd"})
	tt.Frame()
	if !tt.HasText("/etc/passwd") {
		t.Errorf("a path outside the tree said nothing; texts %q", tt.Texts())
	}
}

// The viewer's Zoom widens it over the panes, and Show All Panes puts it
// back beside them.
func TestViewerZoom(t *testing.T) {
	const name = "Trade_Reporting_views_documentation_FI_final.md"
	a := testApp(t)
	s := &a.st.Sessions[0]
	fv := &fileView{pane: "a", path: "/tmp/" + name, rel: name, fetched: time.Now()}
	fv.testRead("text\n")
	a.openView(s, sideFiles, fv)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("zsh") {
		t.Fatalf("the panes don't show beside the viewer; texts %q", tt.Texts())
	}
	if err := tt.Click("Zoom Viewer"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if tt.HasText("zsh") || !a.side.wide {
		t.Error("zoomed, the panes show")
	}
	// The file's name shows whole while the viewer has room.
	if r, ok := tt.Find(name); !ok || r.W < 300 {
		t.Errorf("zoomed, the header's name is cut to %v; texts %q", r.W, tt.Texts())
	}
	shot(t, tt, "viewer-zoom")
	if err := tt.Click("Show All Panes"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("zsh") || a.side.wide {
		t.Error("Show All Panes left the viewer zoomed")
	}
}

// With the sessions at the window's right, the right sidebar and the
// viewer take its left.
func TestSessionsRight(t *testing.T) {
	a := testApp(t)
	a.set.SessionsSide = "right"
	a.prefs.Side = sideChanges
	s := &a.st.Sessions[0]
	fv := &fileView{pane: "a", path: "/tmp/notes.txt", rel: "notes.txt", fetched: time.Now()}
	fv.testRead("notes\n")
	a.openView(s, sideFiles, fv)
	var viewer, panes ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		viewer, panes = a.paneRef("side-card").Bounds(c), a.paneRef("card-a").Bounds(c)
	}, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tab, ok1 := tt.Find("Changes")
	name, ok2 := tt.Find("demo")
	if !ok1 || !ok2 {
		t.Fatalf("no sidebar; texts %q", tt.Texts())
	}
	if tab.X > 300 || name.X < 900 {
		t.Errorf("the tab at %v, the session's name at %v", tab.X, name.X)
	}
	if viewer.X > panes.X {
		t.Errorf("the viewer at %v, right of the panes at %v", viewer.X, panes.X)
	}
	shot(t, tt, "sessions-right")
}

func TestHostJoin(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"/tmp/demo", "a.go"}:     "/tmp/demo/a.go",
		{"/", "etc"}:              "/etc",
		{`C:\src\kero`, "a.go"}:   `C:\src\kero\a.go`,
		{`C:\`, "src"}:            `C:\src`,
		{"C:/src", "a.go"}:        `C:/src\a.go`,
		{"/home/me/dir", "x y.z"}: "/home/me/dir/x y.z",
	} {
		if got := hostJoin(in[0], in[1]); got != want {
			t.Errorf("hostJoin(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestCommandWords(t *testing.T) {
	for in, want := range map[string][]string{
		"":                                  nil,
		"  ":                                nil,
		"code":                              {"code"},
		"code -r":                           {"code", "-r"},
		`"C:\Program Files\Zed\zed.exe" -n`: {`C:\Program Files\Zed\zed.exe`, "-n"},
		`subl  --wait`:                      {"subl", "--wait"},
	} {
		if got := commandWords(in); !slices.Equal(got, want) {
			t.Errorf("commandWords(%q) = %q, want %q", in, got, want)
		}
	}
}

// An editor pane's file's buttons are in its header, as the viewer's are,
// and in a pane too narrow for them, in one menu, which still acts.
func TestEditorPaneNarrow(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/some_long_file_name.md", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	a.edits["b"].testRead("# Title\n")
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("Preview Markdown") || tt.HasText("File Actions") {
		t.Errorf("a wide editor pane's buttons fold; texts %q", tt.Texts())
	}
	tt.SetSize(760, 500)
	for range 3 {
		tt.Frame()
	}
	shot(t, tt, "editor-narrow")
	if tt.HasText("Preview Markdown") {
		t.Errorf("a narrow editor pane's buttons don't fold; texts %q", tt.Texts())
	}
	if err := tt.Click("File Actions"); err != nil {
		t.Fatal(err)
	}
	if items := tt.Menu(); !slices.Equal(items, []string{"Preview Markdown", "Smaller Text", "Bigger Text", "Wrap Lines", "Open in Editor"}) {
		t.Errorf("the folded buttons' menu: %q", items)
	}
	if err := tt.ChooseMenuItem("Preview Markdown"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.edits["b"].preview {
		t.Error("Preview Markdown from the menu didn't preview")
	}
	tt.SetSize(1280, 720)
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("Preview Markdown") {
		t.Errorf("widened again, the buttons stay folded; texts %q", tt.Texts())
	}
}

// The Files tab's root shows its whole path where it fits beside the
// buttons, else only its folder's name, as a pane's header fits its folder.
func TestFilesRootFits(t *testing.T) {
	const root = "/tmp/projects/clients/acme/services/billing-api"
	a := testApp(t)
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: root, Branch: "main", Default: "main"}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	a.prefs.Side = sideFiles
	h := a.hostOf(s.ID)
	a.side.files.dirs = map[string]*dirList{filesKey(h, root): {entries: []proto.FSEntry{{Name: "main.go"}}}}
	for _, c := range []struct {
		width     float32
		want, not string
	}{{720, root, ""}, {300, "billing-api", root}} {
		a.prefs.RightWidth = c.width
		tt := ui.NewTester(a.view, 1600, 720)
		for range 3 {
			tt.Frame()
			tt.Advance(time.Second)
		}
		shot(t, tt, "files-root-"+map[bool]string{true: "wide", false: "narrow"}[c.not == ""])
		if !tt.HasText(c.want) || c.not != "" && tt.HasText(c.not) {
			t.Errorf("sidebar %v wide: want %q, not %q; texts %q", c.width, c.want, c.not, tt.Texts())
		}
	}
}
