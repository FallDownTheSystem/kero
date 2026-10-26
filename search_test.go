package main

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

// The Search view searches the folder as the user stops typing, as the
// toggles ask, shows each file's matches under it, and a match opens its
// file at its line.
func TestSearchView(t *testing.T) {
	a := testApp(t)
	asked := make(chan proto.FSSearchParams, 8)
	splits := make(chan proto.CreateParams, 4)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "fs.search":
			var p proto.FSSearchParams
			json.Unmarshal(raw, &p)
			asked <- p
			return proto.FSSearch{Files: []proto.FSSearchFile{
				{Path: "src/a.go", Matches: []proto.FSMatch{{Line: 3, Text: "func Hello() {}", Start: 5, End: 10}, {Line: 9, Text: "// hello", Start: 3, End: 8}}},
				{Path: "README.md", Matches: []proto.FSMatch{{Line: 1, Text: "Say hello", Start: 4, End: 9}}},
			}}, nil
		case "split":
			var p proto.CreateParams
			json.Unmarshal(raw, &p)
			splits <- p
			return proto.Pane{}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
	var card ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		card = a.paneRef("card-b").Bounds(c)
	}, 1280, 720)
	tt.Frame()
	a.run("search.toggle", nil)
	tt.Frame()
	tt.Frame()
	tt.Type("hello")
	tt.Frame()
	if a.prefs.Side != sideSearch || a.side.search.query != "hello" {
		t.Fatalf("the view shows %q, its field took %q", a.prefs.Side, a.side.search.query)
	}
	select {
	case p := <-asked:
		t.Fatalf("searched %+v as the user typed", p)
	default:
	}
	tt.Advance(searchDelay)
	tt.Frame()
	next := func() proto.FSSearchParams {
		t.Helper()
		select {
		case p := <-asked:
			return p
		case <-time.After(2 * time.Second):
			t.Fatal("no search")
		}
		return proto.FSSearchParams{}
	}
	if p := next(); p.Query != "hello" || p.Dir != "/tmp" || p.Case || p.Word || p.Regex {
		t.Errorf("searched %+v", p)
	}
	for range 100 {
		tt.Frame()
		if tt.HasText("README.md") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !tt.HasText("3 results in 2 files") {
		t.Errorf("texts %q", tt.Texts())
	}
	shot(t, tt, "search")

	if err := tt.Click("Match Case"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if p := next(); !p.Case || p.Query != "hello" {
		t.Errorf("Match Case searched %+v", p)
	}

	// The file folds, and a match opens it at its line.
	if err := tt.Click("README.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Say hello") {
		t.Error("README.md did not fold")
	}
	r, ok := tt.Find("// hello")
	if !ok {
		t.Fatalf("no match; texts %q", tt.Texts())
	}
	tt.ClickAt(r.X+4, r.Y+4)
	tt.Frame()
	tt.Frame()
	select {
	case p := <-splits:
		if p.File != "/tmp/src/a.go" {
			t.Errorf("opened %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the match opened nothing")
	}
	if pos, ok := a.gotos[gotoKey(a.local(), "/tmp/src/a.go")]; !ok || pos.Line != 8 {
		t.Errorf("the caret goes to %+v", pos)
	}
	opened := func(how, beside string, line int) {
		t.Helper()
		select {
		case p := <-splits:
			if p.File != "/tmp/src/a.go" || p.Split != beside || p.Direction != "right" {
				t.Errorf("%s opened %+v", how, p)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s opened nothing", how)
		}
		if pos, ok := a.gotos[gotoKey(a.local(), "/tmp/src/a.go")]; !ok || pos.Line != line-1 {
			t.Errorf("%s puts the caret at %+v, want line %d", how, pos, line)
		}
	}

	// A match's menu opens its file at its line too.
	if err := tt.RightClick("func Hello() {}"); err != nil {
		t.Fatal(err)
	}
	if items := tt.Menu(); !slices.Equal(items, []string{"Open in Editor", "Open in Pane"}) {
		t.Errorf("a match's menu has %q", items)
	}
	if err := tt.ChooseMenuItem("Open in Pane"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	opened("Open in Pane", a.focusOf(&a.st.Sessions[0]), 3)

	// And a match dragged onto a pane opens there, at its line. No pane
	// opened to take the caret Open in Pane left.
	clear(a.gotos)
	r, ok = tt.Find("func Hello() {}")
	if !ok {
		t.Fatalf("no match; texts %q", tt.Texts())
	}
	fx, fy := r.X+4, r.Y+r.H/2
	tt.Advance(time.Second)
	tt.Press(fx, fy)
	tt.Frame()
	tt.Move(fx+12, fy)
	tt.Frame()
	x, y := card.X+card.W-20, card.Y+card.H/2
	tt.Move(x, y)
	tt.Frame()
	tt.Release(x, y)
	tt.Frame()
	opened("a drag", "b", 3)
}
