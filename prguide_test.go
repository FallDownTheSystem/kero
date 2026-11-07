package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

// The Guide tab shows the guide's summary and its stops, one at a time:
// an excerpt of its lines, the change's removed lines before its added
// ones; Next Stop goes to the next; Full Diff shows the file's diff; a
// pull request with no guide offers Write Guide, which asks with the
// guide's prompt.
func TestPRGuideTab(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	view := proto.PRView{PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", MergeBase: "mb",
		Files: []proto.GitFile{{Path: "parse.go", Status: "modified", Added: 2, Removed: 1, From: "mb", To: "h1"},
			{Path: "lex.go", Status: "added", Added: 1, From: "mb", To: "h1"}}}
	guide := &proto.PRGuide{Repo: "me/demo", Remote: "origin", Number: 7, Head: "h1", MergeBase: "mb", Summary: "Splits **a** in two.",
		Stops: []proto.PRGuideStop{
			{ID: 1, Path: "parse.go", Side: "RIGHT", Start: 2, Line: 4, Title: "The split", Note: "b and c replace a."},
			{ID: 2, Path: "lex.go", Title: "A new lexer", Closely: true},
		}}
	var guides []proto.PRGuideParams
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "git.file":
			var p proto.GitParams
			json.Unmarshal(raw, &p)
			if p.Path == "lex.go" {
				return proto.GitText{New: "package parse\n", Hash: "l"}, nil
			}
			return proto.GitText{Old: "package parse\n\nfunc a() {}\n", New: "package parse\n\nfunc b() {}\nfunc c() {}\n", Hash: "h"}, nil
		case "pr.review":
			return json.RawMessage("null"), nil
		case "pr.guide":
			var p proto.PRGuideParams
			json.Unmarshal(raw, &p)
			guides = append(guides, p)
			if guide == nil {
				return json.RawMessage("null"), nil
			}
			return guide, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 900)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	g := a.side.pulls.page
	frameUntil(t, tt, "guide", func() bool { return g.res != nil && g.guide.guide != nil })
	if err := tt.Click("Guide"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	frameUntil(t, tt, "excerpt", func() bool { return tt.HasText("func b() {}") })
	tt.Frame()
	shot(t, tt, "pr-guide")
	texts := strings.Join(tt.Texts(), "\n")
	for _, want := range []string{"Guide, 2 stops", "Stop 1 of 2", "The split", "parse.go:2–4", "b and c replace a.", "func a() {}", "func c() {}", "Look closely", "Next Stop"} {
		if !strings.Contains(texts, want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	// The removed line comes before the lines that replace it.
	if ia, ib := strings.Index(texts, "func a() {}"), strings.Index(texts, "func b() {}"); ia < 0 || ib < ia {
		t.Errorf("removed at %d, added at %d", ia, ib)
	}

	if err := tt.Click("Next Stop"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "next", func() bool { return tt.HasText("Stop 2 of 2") && tt.HasText("package parse") })
	if !tt.HasText("lex.go, the whole file") || !tt.HasText("That's the tour. Your review is below.") {
		t.Errorf("texts %q", tt.Texts())
	}
	if err := tt.Click("Full Diff"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	shot(t, tt, "pr-guide-full")
	if !g.guide.full[2] || !tt.HasText("Excerpt") {
		t.Errorf("full diff not shown; texts %q", tt.Texts())
	}

	// With no guide: Write Guide, which asks with the guide's prompt and
	// starts the guide for the agent.
	guide = nil
	a.fetchGuide(g)
	frameUntil(t, tt, "empty", func() bool { return tt.HasText("Write Guide") })
	shot(t, tt, "pr-guide-empty")
	if err := tt.Click("Write Guide"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	w := a.prReview
	if w == nil || !w.guide || !strings.HasPrefix(w.prompt, "Write a review guide of pull request #7") || !tt.HasText("Guide of #7") {
		t.Fatalf("asked %+v; texts %q", w, tt.Texts())
	}
}
