package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/proto"
)

// The Files view lists the worktree of the pane the user is in, opens a
// folder's files as it opens, and a file chosen below the tree, in an
// editor: its edits show unsaved until a save, and stay as another file
// opens.
func TestFilesView(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	a.prefs.Side = sideFiles

	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tr := a.side.files.shown
	if tr == nil || tr.root != "/tmp/demo" {
		t.Fatalf("the view shows %+v, not the worktree", tr)
	}
	// What kerod would answer.
	tr.listed("/tmp/demo", tr.stamp, proto.FSList{Entries: []proto.FSEntry{
		{Name: ".git", Dir: true}, {Name: "src", Dir: true}, {Name: "README.md"}, {Name: "main.go"},
	}}, nil)
	tt.Frame()
	for _, want := range []string{"Changes", "Files", "src", "README.md", "main.go"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText(".git") {
		t.Error("the tree lists .git")
	}

	// A click on a folder chooses it and opens it.
	if err := tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tr.outline.Open.Has("/tmp/demo/src") || tr.chosen < 0 || tr.outline.Item(tr.chosen) != "/tmp/demo/src" {
		t.Fatalf("a click on src didn't open and choose it: open %v, chosen %d", tr.outline.Open.Has("/tmp/demo/src"), tr.chosen)
	}
	tr.listed("/tmp/demo/src", tr.stamp, proto.FSList{Entries: []proto.FSEntry{{Name: "util.go"}}}, nil)
	tt.Frame()
	if !tt.HasText("util.go") {
		t.Errorf("an open folder's files don't show; texts %q", tt.Texts())
	}
	shot(t, tt, "files-tree")

	// A file chosen opens below the tree, as its text comes.
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	f := tr.files["/tmp/demo/main.go"]
	if f == nil || tr.shown != f.path {
		t.Fatalf("main.go didn't open: %v", tr.shown)
	}
	f.read(tr.stamp, false, false, nil, []byte("package main\r\n\r\nfunc main() {}\r\n"), nil)
	tt.Frame()
	if f.st == nil || f.modified() || tt.HasText("Unsaved") {
		t.Fatal("main.go opened as edited, or not as text")
	}
	if l := f.st.Buffer.Language(); l == nil || l.Name != "Go" {
		t.Errorf("main.go is highlighted as %v", l)
	}
	shot(t, tt, "files-open")

	// An edit shows, by the file and in the tree, until it is saved; a save
	// keeps the file's line endings.
	f.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// Main.\n"})
	tt.Frame()
	if !tt.HasText("Unsaved") || !tt.HasText("●") {
		t.Errorf("an edit doesn't show unsaved; texts %q", tt.Texts())
	}
	var buf bytes.Buffer
	f.st.Buffer.WriteTo(&buf)
	if !bytes.HasPrefix(buf.Bytes(), []byte("// Main.\r\npackage main\r\n")) {
		t.Errorf("a save would write %q", buf.Bytes())
	}
	f.saved(f.st.Buffer.Version(), proto.FSWrite{Hash: "h"}, nil)
	tt.Frame()
	if f.modified() || tt.HasText("Unsaved") || f.hash != "h" {
		t.Error("a save left the file unsaved")
	}

	// Another program changed it since: the save is refused, and the user
	// reloads it or overwrites it.
	f.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "x"})
	f.saved(f.st.Buffer.Version(), proto.FSWrite{Changed: true}, nil)
	tt.Frame()
	if !f.modified() || !tt.HasText("The file changed on disk since you opened it.") || !tt.HasText("Overwrite") {
		t.Errorf("a refused save isn't told; texts %q", tt.Texts())
	}
	// Saved while typing on: what was typed since is not saved.
	v := f.st.Buffer.Version()
	f.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "y"})
	f.saved(v, proto.FSWrite{Hash: "h2"}, nil)
	if !f.modified() || f.changed {
		t.Error("a save marked text typed after it saved")
	}

	// The edited file stays as another opens; one unedited goes.
	if err := tt.Click("README.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	readme := tr.files["/tmp/demo/README.md"]
	if readme == nil || tr.files["/tmp/demo/main.go"] != f {
		t.Fatalf("files open: %v", tr.files)
	}
	readme.read(tr.stamp, false, false, nil, []byte{0x89, 'P', 'N', 'G', 0, 0}, nil)
	tt.Frame()
	if !tt.HasText("Binary file") {
		t.Errorf("a binary file isn't told; texts %q", tt.Texts())
	}
	if err := tt.Click("util.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tr.files["/tmp/demo/README.md"] != nil {
		t.Error("a file without edits stayed open")
	}
	if tr.files["/tmp/demo/main.go"] != f {
		t.Error("an edited file went")
	}
	tr.files["/tmp/demo/src/util.go"].read(tr.stamp, false, false, nil, nil, errors.New("permission denied"))
	tt.Frame()
	if !tt.HasText("Can't read the file: permission denied") {
		t.Errorf("a read that failed isn't told; texts %q", tt.Texts())
	}

	// A save with the host away says so.
	tr.shown = f.path
	tt.Frame()
	a.saveFile(tr.pane, f, false)
	if a.pendingToast == "" {
		t.Error("a save with no kerod to write it said nothing")
	}

	// Its menu copies its path from the worktree.
	if err := tt.RightClick("util.go"); err != nil {
		t.Fatal(err)
	}
	if items := tt.Menu(); !slices.Contains(items, "Copy Relative Path") || !slices.Contains(items, "Open in Editor") || !slices.Contains(items, "Open in Pane") {
		t.Fatalf("menu %q", items)
	}
	if err := tt.ChooseMenuItem("Copy Relative Path"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != "src/util.go" {
		t.Errorf("copied %q", got)
	}
}

// An editor pane shows its file in the editor, which wraps its lines as
// the user asks; closing it with edits not saved asks first.
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
	if f == nil || f.path != "/tmp/main.go" {
		t.Fatalf("the pane's file is %+v", f)
	}
	f.read("", false, false, nil, []byte("package main\n"), nil)
	tt.Frame()
	if f.st == nil {
		t.Fatal("no editor")
	}
	if err := tt.Click("Wrap Lines"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.prefs.Wrap || !f.st.SoftWrap {
		t.Error("Wrap Lines left the lines unwrapped")
	}
	shot(t, tt, "editor-pane")

	f.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "// x\n"})
	a.closePane(s, "b", true)
	if a.confirm == nil || s.PaneByID("b") == nil {
		t.Fatal("closed an editor with edits unasked")
	}
	a.confirm.run()
	if s := a.session("s1"); s.PaneByID("b") != nil || a.edits["b"] != nil {
		t.Error("the editor stayed as the user closed it")
	}
}

// New File, from the tree's toolbar or a folder's menu, names a file in
// the folder chosen; once kerod makes it, it is chosen in the tree, and
// opens below it.
func TestFilesNew(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	a.prefs.Side = sideFiles
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tr := a.side.files.shown
	tr.listed("/tmp/demo", tr.stamp, proto.FSList{Entries: []proto.FSEntry{{Name: "src", Dir: true}, {Name: "main.go"}}}, nil)
	tt.Frame()

	if err := tt.RightClick("src"); err != nil {
		t.Fatal(err)
	}
	items := tt.Menu()
	for _, want := range []string{"New File…", "New Folder…", "New Terminal Here", "Open in Editor", "Copy Relative Path"} {
		if !slices.Contains(items, want) {
			t.Errorf("a folder's menu has no %q: %q", want, items)
		}
	}
	if slices.Contains(items, "Open in Pane") {
		t.Errorf("a folder's menu opens it in a pane: %q", items)
	}
	if err := tt.ChooseMenuItem("New File…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	n := tr.adding
	if n == nil || n.in != "/tmp/demo/src" || n.dir {
		t.Fatalf("New File names %+v", n)
	}
	if !tr.outline.Open.Has("/tmp/demo/src") {
		t.Error("the folder a file is new in stayed closed")
	}

	// The field has the keyboard; Enter makes the file, through kerod.
	tt.Type("util/new.go")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if n.name != "util/new.go" || n.err == "" {
		t.Errorf("typed %q; made a file with no kerod to make it: %q", n.name, n.err)
	}
	a.added(tr, n, tr.join(n.in, n.name), nil)
	if tr.adding != nil {
		t.Error("the name's field stayed")
	}
	if tr.shown != "/tmp/demo/src/util/new.go" || tr.reveal != tr.shown || !tr.outline.Open.Has("/tmp/demo/src/util") {
		t.Errorf("the new file shows %q, reveals %q", tr.shown, tr.reveal)
	}

	// The toolbar's, with nothing chosen, names one in the tree's folder.
	tr.chosen = -1
	tt.Frame()
	if err := tt.Click("New Folder…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if tr.adding == nil || tr.adding.in != "/tmp/demo" || !tr.adding.dir {
		t.Errorf("the toolbar's New Folder names %+v", tr.adding)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tr.adding != nil {
		t.Error("Cancel left the field")
	}
}

// The toolbar's filter shows what kerod finds in all of the folder, in the
// folders it is in; Down goes to the first file found, and Esc shows it
// in the whole tree.
func TestFilesFilter(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	a.prefs.Side = sideFiles
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tr := a.side.files.shown
	tr.listed("/tmp/demo", tr.stamp, proto.FSList{Entries: []proto.FSEntry{{Name: "src", Dir: true}, {Name: "main.go"}}}, nil)
	tt.Frame()

	if err := tt.Click("Filter"); err != nil {
		t.Fatal(err)
	}
	tt.Type("util")
	tt.Frame()
	if tr.find.query != "util" || !tr.filtering() {
		t.Fatalf("typed %q in the filter", tr.find.query)
	}
	tr.found("util", tr.stamp, proto.FSFind{Entries: []proto.FSFound{{Path: "src/util", Dir: true}, {Path: "src/util/util.go"}, {Path: "lib/util.c"}}}, nil)
	tt.Frame()
	for _, want := range []string{"src", "util", "util.go", "lib", "util.c"} {
		if !tt.HasText(want) {
			t.Errorf("the filter shows no %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText("main.go") {
		t.Error("the filter shows main.go")
	}
	shot(t, tt, "files-filter")

	tt.Key(0, ui.KeyDown)
	tt.Frame()
	f := &tr.find
	if f.chosen < 0 || f.outline.Item(f.chosen) != "/tmp/demo/lib/util.c" || tr.shown != "/tmp/demo/lib/util.c" {
		t.Fatalf("Down chose row %d, shows %q", f.chosen, tr.shown)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tr.filtering() || f.paths != nil {
		t.Fatalf("Esc left the filter at %q", f.query)
	}
	if tr.reveal != "" && tr.reveal != "/tmp/demo/lib/util.c" || !tr.outline.Open.Has("/tmp/demo/lib") {
		t.Errorf("the file found isn't revealed in the tree: %q", tr.reveal)
	}

	if err := tt.Click("Filter"); err != nil {
		t.Fatal(err)
	}
	tt.Type("zzz")
	tt.Frame()
	tr.found("zzz", tr.stamp, proto.FSFind{}, nil)
	tt.Frame()
	if !tt.HasText("Nothing matches “zzz”") {
		t.Errorf("no match shows %q", tt.Texts())
	}
	if err := tt.Click("Clear Filter"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tr.filtering() || !tt.HasText("main.go") {
		t.Errorf("Clear Filter left %q", f.query)
	}
}

// A path the CLI names is the tree's, as the tree writes its paths.
func TestFilesWithin(t *testing.T) {
	unix := &fileTree{root: "/tmp/demo"}
	win := &fileTree{root: "C:/src/kero"}
	for _, c := range []struct {
		t    *fileTree
		in   string
		want string
		ok   bool
	}{
		{unix, "/tmp/demo/src/a.go", "/tmp/demo/src/a.go", true},
		{unix, "/tmp/demo", "/tmp/demo", true},
		{unix, "/tmp/demos/a.go", "", false},
		{unix, "/etc/passwd", "", false},
		{unix, "src/a.go", "/tmp/demo/src/a.go", true},
		{win, `C:\src\kero\files.go`, `C:/src/kero\files.go`, true},
		{win, `c:\SRC\kero\internal\cli`, `C:/src/kero\internal\cli`, true},
		{win, `D:\src\kero\files.go`, "", false},
	} {
		got, ok := c.t.within(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("within(%q) of %s = %q, %v; want %q, %v", c.in, c.t.root, got, ok, c.want, c.ok)
		}
	}
}

// An editor pane of a file not there yet opens empty, as a new file, to be
// made by its save.
func TestEditorNewFile(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/plan.md", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	f := a.edits["b"]
	if f == nil || !f.fresh {
		t.Fatalf("the pane's file is %+v", f)
	}
	f.absent = true
	f.read("", false, false, nil, nil, nil)
	tt.Frame()
	if f.st == nil || f.err != "" || f.hash != "" || !tt.HasText("New file") {
		t.Fatalf("a new file opened as %+v; texts %q", f, tt.Texts())
	}
	f.saved(f.st.Buffer.Version(), proto.FSWrite{Hash: "h"}, nil)
	tt.Frame()
	if f.absent || tt.HasText("New file") {
		t.Error("a saved new file still shows as new")
	}
}

// In a narrow panel, the header's tabs show their icons alone, and leave
// the window's controls their room: the shown one keeps its label longest.
func TestSideHeaderNarrow(t *testing.T) {
	a := testApp(t)
	a.prefs.Side = sideFiles
	a.prefs.SideWidth = minSide
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	for _, tab := range []string{"Changes", "Files"} {
		if r, ok := tt.Find(tab); !ok || r.W > 36 {
			t.Errorf("a narrow panel's %s tab is %v wide", tab, r.W)
		}
	}
	a.prefs.SideWidth = defaultSide
	tt.Frame()
	if r, ok := tt.Find("Changes"); !ok || r.W > 36 {
		t.Errorf("the default panel's Changes tab is %v wide", r.W)
	}
	if r, ok := tt.Find("Files"); !ok || r.W < 50 {
		t.Errorf("the default panel's shown Files tab is %v wide", r.W)
	}
	a.prefs.SideWidth = compactSide
	tt.Frame()
	if r, ok := tt.Find("Changes"); !ok || r.W < 60 {
		t.Errorf("a wide panel's Changes tab is %v wide", r.W)
	}
}

// A narrow pane's header folds its controls into a menu, and its git chip
// shows its counts alone, or nothing, rather than run over its buttons.
func TestPaneHeaderNarrow(t *testing.T) {
	a := testApp(t)
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

// A new file's pane, not yet typed in, offers to open a file instead; one
// typed in, or saved, doesn't.
func TestNewFilePaneOpenFile(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/untitled.txt", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	f := &openFile{path: "/tmp/untitled.txt", fresh: true, absent: true, st: &editor.State{Buffer: editor.NewBuffer("")}}
	a.edits = map[string]*openFile{"b": f}
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

// Without previews, a file chosen in the tree stays closed until its
// menu's Preview opens it below the tree.
func TestFilesNoPreview(t *testing.T) {
	a := testApp(t)
	a.st.Sessions[0].Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	a.prefs.Side = sideFiles
	a.set.FilePreview = false
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tr := a.side.files.shown
	tr.listed("/tmp/demo", tr.stamp, proto.FSList{Entries: []proto.FSEntry{{Name: "main.go"}}}, nil)
	tt.Frame()
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tr.shown != "" {
		t.Fatalf("a file chosen opened below the tree: %q", tr.shown)
	}
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Preview"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if tr.shown != "/tmp/demo/main.go" {
		t.Errorf("Preview opened %q", tr.shown)
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
			return proto.FSList{Entries: []proto.FSEntry{{Name: "main.go"}}}, nil
		case "split":
			var p proto.CreateParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			calls <- "split " + p.Split + " " + p.Direction + " " + p.File
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

// filesBeside opens the Files view of /tmp/demo, beside pane a and an
// editor pane b of its README.md.
func filesBeside(t *testing.T, a *App) {
	t.Helper()
	s := &a.st.Sessions[0]
	s.Panes[0].Git = &proto.GitInfo{Root: "/tmp/demo", Branch: "main"}
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/demo/README.md", Dir: "/tmp/demo", Status: proto.Idle, Progress: -1}
	a.prefs.Side = sideFiles
}

// With files-pane = replace, a file's menu opens it in a pane of its own
// too, beside Open in Pane, which takes the place of the editor pane's
// file.
func TestFilesOpenInNewPane(t *testing.T) {
	a := testApp(t)
	filesBeside(t, a)
	calls := paneCalls(t, a)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tr := a.side.files.shown
	tr.listed("/tmp/demo", tr.stamp, proto.FSList{Entries: []proto.FSEntry{{Name: "main.go"}}}, nil)
	tt.Frame()
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if items := tt.Menu(); slices.Contains(items, "Open in New Pane") {
		t.Errorf("with files-pane = new, the menu has Open in New Pane: %q", items)
	}
	tt.CloseMenu()

	a.set.FilePane = "replace"
	tt.Frame()
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open in New Pane"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if got := nextCall(t, calls); got != "split a right /tmp/demo/main.go" {
		t.Errorf("Open in New Pane called %q", got)
	}
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open in Pane"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := nextCall(t, calls); got != "file b /tmp/demo/main.go" {
		t.Errorf("Open in Pane called %q", got)
	}
}

// A row's menu opens from all of the row that shows hovered: its edges
// above and below its name too, the list row's padding.
func TestFilesRowEdges(t *testing.T) {
	a := testApp(t)
	filesBeside(t, a)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tr := a.side.files.shown
	tr.listed("/tmp/demo", tr.stamp, proto.FSList{Entries: []proto.FSEntry{{Name: "main.go"}}}, nil)
	tt.Frame()
	r, ok := tt.Find("main.go")
	if !ok {
		t.Fatalf("no main.go; texts %q", tt.Texts())
	}
	for _, y := range []float32{r.Y - 2, r.Y + r.H + 2} {
		tt.RightClickAt(r.X+r.W/2, y)
		if items := tt.Menu(); !slices.Contains(items, "Open in Pane") {
			t.Errorf("a right-click at %v, beside main.go at %v to %v, opened %q", y, r.Y, r.Y+r.H, items)
		}
		tt.CloseMenu()
		tt.Frame()
	}
}

// A file dragged from the tree onto a pane opens beside it, on the side it
// is dropped at, or, in the middle of an editor pane, in its place: a
// terminal's middle is no place for it.
func TestFilesDragToPane(t *testing.T) {
	a := testApp(t)
	filesBeside(t, a)
	calls := paneCalls(t, a)
	cards := map[string]ui.Rect{}
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		for _, id := range []string{"a", "b"} {
			cards[id] = a.paneRef("card-" + id).Bounds(c)
		}
	}, 1280, 720)
	tt.Frame()
	tr := a.side.files.shown
	tr.listed("/tmp/demo", tr.stamp, proto.FSList{Entries: []proto.FSEntry{{Name: "main.go"}}}, nil)
	tt.Frame()
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
		tt.Move(fx+12, fy)
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
	if tr.shown != "" && tr.shown != "/tmp/demo/main.go" {
		t.Errorf("a drag opened %q below the tree", tr.shown)
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

// One button shows and hides the side panel, with the view it showed last;
// another widens it to all the room the panes leave, and back to its width.
func TestPanelButtons(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	closed, _ := tt.Find(a.tip("Show Explorer", "changes.toggle"))
	if err := tt.Click(a.tip("Show Explorer", "changes.toggle")); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	// The toggle stays where it was, now in the panel's header.
	if open, _ := tt.Find(a.tip("Hide Explorer", "changes.toggle")); open.X != closed.X || open.Y != closed.Y {
		t.Errorf("the panel's toggle moved from %v to %v as it opened", closed, open)
	}
	if a.prefs.Side != sideChanges {
		t.Fatalf("the panel shows %q, want Changes first", a.prefs.Side)
	}
	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(a.tip("Hide Explorer", "files.toggle")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.prefs.Side != "" || a.sideView() != sideFiles {
		t.Fatalf("hidden, the panel shows %q and comes back with %q", a.prefs.Side, a.sideView())
	}
	a.toggleSide(a.sideView())
	tt.Frame()
	before, _ := tt.Find("Files")
	if err := tt.Click("Widen Explorer"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	wide, _ := tt.Find("Files")
	if wide.X >= before.X-100 {
		t.Errorf("the widened panel's tab is at %v, from %v", wide.X, before.X)
	}
	if err := tt.Click("Restore Explorer Width"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if back, _ := tt.Find("Files"); back.X != before.X || a.prefs.SideWidth != 0 {
		t.Errorf("restored, the tab is at %v, from %v; width %v", back.X, before.X, a.prefs.SideWidth)
	}
}

// With the sessions at the window's right, the side panel takes its left.
func TestSessionsRight(t *testing.T) {
	a := testApp(t)
	a.set.SessionsSide = "right"
	a.prefs.Side = sideChanges
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tab, ok1 := tt.Find("Changes")
	name, ok2 := tt.Find("demo")
	if !ok1 || !ok2 {
		t.Fatalf("no panel or sidebar; texts %q", tt.Texts())
	}
	if tab.X > 300 || name.X < 900 {
		t.Errorf("the panel's tab at %v, the session's name at %v", tab.X, name.X)
	}
	shot(t, tt, "sessions-right")
}

// The view's tab shows it, and the panel's other view with the other's.
func TestFilesTab(t *testing.T) {
	a := testApp(t)
	a.prefs.Side = sideChanges
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.prefs.Side != sideFiles {
		t.Errorf("the Files tab shows %q", a.prefs.Side)
	}
	if err := tt.Click("Changes"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.prefs.Side != sideChanges {
		t.Errorf("the Changes tab shows %q", a.prefs.Side)
	}
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

func TestIsBinary(t *testing.T) {
	for in, want := range map[string]bool{"": false, "package main\n": false, "héllo": false, "a\x00b": true, "\xff\xfe": true} {
		if got := isBinary([]byte(in)); got != want {
			t.Errorf("isBinary(%q) = %v", in, got)
		}
	}
}
