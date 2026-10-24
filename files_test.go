package main

import (
	"bytes"
	"errors"
	"slices"
	"testing"

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

	tr.outline.Open.Add("/tmp/demo/src")
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
// the window's controls their room.
func TestSideHeaderNarrow(t *testing.T) {
	a := testApp(t)
	a.prefs.Side = sideFiles
	a.prefs.SideWidth = minSide
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	if r, ok := tt.Find("Changes"); !ok || r.W > 36 {
		t.Errorf("a narrow panel's Changes tab is %v wide", r.W)
	}
	a.prefs.SideWidth = defaultSide
	tt.Frame()
	if r, ok := tt.Find("Changes"); !ok || r.W < 60 {
		t.Errorf("a wide panel's Changes tab is %v wide", r.W)
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
	if items := tt.Menu(); !slices.Contains(items, "Split Right") || !slices.Contains(items, "Toggle Files") {
		t.Errorf("the folded controls' menu: %q", items)
	}
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
