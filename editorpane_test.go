package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/proto"
)

// diskHost makes the local host a kerod with one file, whose text and
// time of writing the test changes, and tells the pane.preview calls it
// takes.
type diskHost struct {
	mu       sync.Mutex
	text     string
	mod      int64
	previews chan proto.PreviewParams
}

func (d *diskHost) write(text string, mod int64) {
	d.mu.Lock()
	d.text, d.mod = text, mod
	d.mu.Unlock()
}

func newDiskHost(t *testing.T, a *App, text string) *diskHost {
	t.Helper()
	d := &diskHost{text: text, mod: 1, previews: make(chan proto.PreviewParams, 8)}
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		switch method {
		case "fs.stat":
			var p proto.FSStatParams
			json.Unmarshal(raw, &p)
			return proto.FSStat{Path: p.Path, Exists: true, Size: int64(len(d.text)), Mod: d.mod}, nil
		case "fs.read":
			return proto.FSRead{Data: []byte(d.text)}, nil
		case "pane.preview":
			var p proto.PreviewParams
			json.Unmarshal(raw, &p)
			d.previews <- p
		}
		return true, nil
	})
	a.local().cl = cl
	return d
}

// until runs the app's updates until ok, or fails.
func until(t *testing.T, a *App, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatalf("never %s", what)
		}
		time.Sleep(5 * time.Millisecond)
		a.runEarly()
	}
}

// An editor pane previews its Markdown file as kerod keeps it, as kero
// split --preview and kero do pane.preview set it; its button, and the
// menu's Toggle Markdown Preview, tell kerod in turn.
func TestEditorPanePreview(t *testing.T) {
	a := testApp(t)
	d := newDiskHost(t, a, "# The Plan\n\nSome **bold** text.\n")
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/plan.md", Dir: "/tmp", Preview: true, Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	f := a.edits["b"]
	until(t, a, "read the plan", func() bool { return f.st != nil })
	tt.Frame()
	if !f.preview || !tt.HasText("The Plan") || tt.HasText("# The Plan") || tt.HasText("Wrap Lines") {
		t.Fatalf("a pane kerod previews shows its text; texts %q", tt.Texts())
	}

	if err := tt.Click("Preview Markdown"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := <-d.previews; got != (proto.PreviewParams{ID: "b", Preview: false}) {
		t.Errorf("the button told kerod %+v", got)
	}
	if f.preview || s.PaneByID("b").Preview || !tt.HasText("Wrap Lines") {
		t.Error("the preview stayed as its button was clicked")
	}

	// kero do pane.preview on, as kerod's state tells it.
	s.Panes[1].Preview = true
	tt.Frame()
	if !f.preview {
		t.Error("the pane ignored kerod's preview")
	}

	a.setFocus(s, "b")
	a.run("pane.preview", nil)
	tt.Frame()
	if got := <-d.previews; got.Preview || f.preview {
		t.Errorf("Toggle Markdown Preview left the preview on, telling %+v", got)
	}
}

// An editor pane reads its file again as it changes on disk, though no
// worktree's status tells of it, unless it has edits, which stay.
func TestEditorPaneFollowsDisk(t *testing.T) {
	a := testApp(t)
	d := newDiskHost(t, a, "# One\n")
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/plan.md", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	f := a.edits["b"]
	until(t, a, "read the file", func() bool { return f.st != nil && !f.busy })

	d.write("# Two, longer\n", 2)
	f.checked = time.Time{}
	tt.Frame()
	until(t, a, "read the file again", func() bool { return f.st.Buffer.String() == "# Two, longer\n" })
	if f.st.Buffer.Modified() {
		t.Error("the text read again shows as edits")
	}

	until(t, a, "end the check", func() bool { return !f.checking && !f.busy })
	f.st.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "mine "})
	d.write("# Three\n", 3)
	f.checked = time.Time{}
	tt.Frame()
	time.Sleep(50 * time.Millisecond)
	a.runEarly()
	if got := f.st.Buffer.String(); got != "mine # Two, longer\n" || f.checking {
		t.Errorf("the disk's change went over the edits: %q", got)
	}
}

// A browser pane of a local file reloads its page as the file changes on
// disk: the first look, and a page of no file, tell of no change.
func TestBrowserPageFollowsDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brief.html")
	if err := os.WriteFile(path, []byte("<p>one"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", Dir: "/tmp", URL: proto.FileURL(path), Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	e := a.webs["b"]
	if e == nil || e.disk.path != path {
		t.Fatalf("the pane looks at no file: %+v", e)
	}
	if e.diskChanged() {
		t.Error("an unchanged file changed")
	}
	if err := os.WriteFile(path, []byte("<p>two!"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	os.Chtimes(path, later, later)
	if !e.diskChanged() {
		t.Error("a file written again did not change")
	}
	if e.diskChanged() {
		t.Error("the change told twice")
	}
	e.url = "https://kero.sh"
	if e.diskChanged() {
		t.Error("a site's page changed on disk")
	}
}
