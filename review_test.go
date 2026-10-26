package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/git"
	"kero/internal/proto"
)

// A comment on lines of a diff shows under them, and in the review's band,
// whose Send to Agent pastes the comments as one prompt in the agent's
// pane, with the files, the lines and their text, then sends it.
func TestReview(t *testing.T) {
	a := testApp(t)
	sent := make(chan proto.SendParams, 8)
	g, ix := gitDemo(a)
	ix.Staged = nil
	ix.Unstaged = []proto.GitFile{{Path: "b.go", Status: "modified", Added: 2, Removed: 1}}
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.index":
			return ix, nil
		case "git.file":
			return proto.GitText{Old: "package b\n\nfunc a() {}\n", New: "package b\n\nfunc b() {}\nfunc c() {}\n", Hash: "h"}, nil
		case "send":
			var p proto.SendParams
			json.Unmarshal(raw, &p)
			sent <- p
			return true, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
	s := &a.st.Sessions[0]
	s.Panes[1].Agent, s.Panes[1].Status = "claude", proto.Idle
	a.prefs.Side = sideChanges
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseHead)
	ch.base, ch.scope, ch.stamp, ch.fetched = git.BaseHead, scope, gitStamp(g)+"|0", time.Now()
	ch.res = &proto.GitChanges{Root: g.Root, Files: ix.Unstaged}
	ch.indexes = map[string]*proto.GitIndex{scope: ix}
	key := fileKey(scope, baseUnstaged, "b.go")
	ch.collapsed = map[string]bool{key: false}

	tt := ui.NewTester(a.view, 1280, 720)
	var sf *sideFile
	for range 100 {
		tt.Frame()
		if sf = ch.files[key]; sf != nil && sf.d != nil && sf.d.OnGutterButton != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sf == nil || sf.d == nil || sf.d.OnGutterButton == nil {
		t.Fatalf("the diff takes no comments; texts %q", tt.Texts())
	}

	// The lines selected around the line the gutter's button is on.
	sf.d.SetSelectedRanges([]editor.DiffLineRange{{Side: editor.DiffNew, Start: 2, End: 3}})
	sf.d.OnGutterButton(editor.DiffLineRange{Side: editor.DiffNew, Start: 3, End: 3})
	tt.Frame()
	tt.Frame()
	// The draft takes the keyboard as it opens, and keeps it as it is
	// typed in: Enter breaks its line, Ctrl+Enter adds it.
	tt.Type("Name these")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Type("after what they do")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	rv := a.reviewOf(a.local(), g.Root, false)
	if rv == nil || len(rv.comments) != 1 {
		t.Fatalf("review %+v; texts %q", rv, tt.Texts())
	}
	cm := rv.comments[0]
	if cm.start != 2 || cm.end != 3 || cm.code != "func b() {}\nfunc c() {}" || cm.text != "Name these\nafter what they do" || rv.draft != nil {
		t.Errorf("comment %+v", cm)
	}
	tt.Frame()
	if !tt.HasText("1 review comment") || !tt.HasText("b.go:3–4") || !tt.HasText("Name these\nafter what they do") {
		t.Errorf("no band, or no comment under its lines; texts %q", tt.Texts())
	}
	// The band is below the files, the comment it lists above it.
	band, _ := tt.Find("1 review comment")
	card, _ := tt.Find("Name these\nafter what they do")
	if band.Y <= card.Y+card.H {
		t.Errorf("the band at %v is above the comment at %v", band, card)
	}
	shot(t, tt, "review")

	// Escape drops a draft, and leaves the panel the keyboard.
	sf.d.OnGutterButton(editor.DiffLineRange{Side: editor.DiffNew, Start: 0, End: 0})
	tt.Frame()
	tt.Frame()
	tt.Type("never mind")
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if rv.draft != nil || len(rv.comments) != 1 || !a.side.focused {
		t.Errorf("Escape left draft %+v, %d comments, the panel focused %v", rv.draft, len(rv.comments), a.side.focused)
	}

	if err := tt.Click("Send to Agent"); err != nil {
		t.Fatal(err)
	}
	next := func() proto.SendParams {
		t.Helper()
		select {
		case p := <-sent:
			return p
		case <-time.After(2 * time.Second):
			t.Fatal("nothing sent")
		}
		return proto.SendParams{}
	}
	p := next()
	for _, want := range []string{"1. b.go, lines 3-4:", "   > func b() {}\n   > func c() {}\n", "   Name these\n   after what they do\n"} {
		if !strings.Contains(p.Data, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, p.Data)
		}
	}
	if p.ID != "b" || !p.Paste {
		t.Errorf("sent to %q, pasted %v", p.ID, p.Paste)
	}
	if p := next(); p.Data != "\r" || p.Paste {
		t.Errorf("then sent %+v", p)
	}
	for range 100 {
		tt.Frame()
		if len(rv.comments) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(rv.comments) != 0 {
		t.Error("the comments sent stayed")
	}
}

// A change's Comment is on the lines selected of it, those added first,
// else on its new lines, else on those it removed.
func TestHunkLines(t *testing.T) {
	h := editor.DiffHunk{OldStart: 4, OldEnd: 6, NewStart: 4, NewEnd: 7}
	for _, c := range []struct {
		sel        []editor.DiffLineRange
		side       editor.DiffSide
		start, end int
	}{
		{nil, editor.DiffNew, 4, 6},
		{[]editor.DiffLineRange{{Side: editor.DiffOld, Start: 5, End: 9}}, editor.DiffOld, 5, 5},
		{[]editor.DiffLineRange{{Side: editor.DiffOld, Start: 4, End: 4}, {Side: editor.DiffNew, Start: 1, End: 5}}, editor.DiffNew, 4, 5},
		{[]editor.DiffLineRange{{Side: editor.DiffNew, Start: 9, End: 9}}, editor.DiffNew, 4, 6},
	} {
		side, start, end, ok := hunkLines(c.sel, h)
		if !ok || side != c.side || start != c.start || end != c.end {
			t.Errorf("%+v: %v %d-%d", c.sel, side, start, end)
		}
	}
	if side, start, end, _ := hunkLines(nil, editor.DiffHunk{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 2}); side != editor.DiffOld || start != 2 || end != 2 {
		t.Errorf("lines removed alone: %v %d-%d", side, start, end)
	}
}
