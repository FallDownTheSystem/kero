package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/proto"
)

// Go to File lists the project's files, finds them by name before path as
// the user types, and opens the one chosen in an editor pane beside the
// pane the user is in.
func TestGoToFile(t *testing.T) {
	a := testApp(t)
	splits := make(chan proto.CreateParams, 4)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "fs.files":
			var p proto.FSFilesParams
			json.Unmarshal(raw, &p)
			if p.Dir != "/tmp" {
				return nil, errors.New("listed " + p.Dir)
			}
			return proto.FSFiles{Paths: []string{"README.md", "docs/main.md", "src/main.go", "src/domain/mapping.go"}}, nil
		case "split":
			var p proto.CreateParams
			json.Unmarshal(raw, &p)
			splits <- p
			return proto.Pane{}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	a.run("files.goto", nil)
	wait := func(ok func() bool) {
		t.Helper()
		for range 100 {
			tt.Frame()
			if ok() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("waited; texts %q", tt.Texts())
	}
	wait(func() bool { return tt.HasText("mapping.go") })

	tt.Type("main")
	tt.Frame()
	got := a.gotoF.goToFound()
	if len(got) != 3 || got[0] != "src/main.go" || got[1] != "docs/main.md" {
		t.Errorf("main finds %q, want the names first, the shorter path first", got)
	}
	shot(t, tt, "go-to-file")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	select {
	case p := <-splits:
		if p.File != "/tmp/src/main.go" || p.Split != "a" {
			t.Errorf("opened %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no editor pane opened")
	}
	if a.gotoF.open {
		t.Error("Go to File stayed open")
	}

	// A path's slash finds by paths alone.
	a.gotoF = goToFile{paths: []string{"main.go", "src/a/main.go", "domain.go"}, query: "s/m", foundOf: -1}
	if got := a.gotoF.goToFound(); len(got) != 1 || got[0] != "src/a/main.go" {
		t.Errorf("s/m finds %q", got)
	}
}

// openAt puts the caret at the line in the editor pane that shows the file
// already, else in the one it opens, as its text first shows.
func TestOpenAt(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", File: "/tmp/a.go", Dir: "/tmp", Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	f := a.edits["b"]
	f.read("", false, false, nil, []byte("one\ntwo\nthree\nfour\n"), nil)
	tt.Frame()
	a.openAt("a", "/tmp/a.go", 3)
	tt.Frame()
	if got := f.st.CursorPos(); got != (editor.Pos{Line: 2}) || a.focusOf(s) != "b" {
		t.Errorf("caret at %+v, focus %q", got, a.focusOf(s))
	}

	// A file no pane shows opens in one, which puts the caret at its line.
	a.openAt("a", "/tmp/b.go", 2)
	if _, ok := a.gotos[gotoKey(a.local(), "/tmp/b.go")]; !ok {
		t.Fatal("the line was not kept for the pane to open")
	}
	s.Panes = append(s.Panes, proto.Pane{ID: "c", File: "/tmp/b.go", Dir: "/tmp", Status: proto.Idle, Progress: -1})
	var l *proto.Layout
	s.Layout = l.Split("a", "b", "right", 0).Split("b", "c", "down", 0)
	tt.Frame()
	g := a.edits["c"]
	g.read("", false, false, nil, []byte("x\ny\nz\n"), nil)
	tt.Frame()
	if got := g.st.CursorPos(); got != (editor.Pos{Line: 1}) {
		t.Errorf("the new pane's caret at %+v", got)
	}
	if len(a.gotos) != 0 {
		t.Errorf("kept %v", a.gotos)
	}
}
