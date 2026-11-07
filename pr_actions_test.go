package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/github"
	"kero/internal/proto"
	"kero/internal/settings"
)

// A repository's default merge method holds while GitHub allows it; else
// the first allowed of squash, merge and rebase.
func TestDefaultMerge(t *testing.T) {
	a := testApp(t)
	a.set.Reviews = map[string]settings.ReviewSetup{"me/demo": {MergeMethod: github.MethodRebase}}
	for _, tc := range []struct {
		slug    string
		allowed []string
		want    string
	}{
		{"Me/Demo", []string{github.MethodMerge, github.MethodRebase}, github.MethodRebase},
		{"me/demo", []string{github.MethodMerge, github.MethodSquash}, github.MethodSquash},
		{"me/demo", nil, github.MethodRebase},
		{"me/other", []string{github.MethodMerge, github.MethodRebase}, github.MethodMerge},
		{"me/other", nil, github.MethodSquash},
	} {
		if got := a.defaultMerge(tc.slug, tc.allowed); got != tc.want {
			t.Errorf("defaultMerge(%s, %q) = %s, want %s", tc.slug, tc.allowed, got, tc.want)
		}
	}
}

// A pull request that waits on its checks merges with the repository's
// default method, or the one chosen, only at the head the page showed;
// Enable Auto-Merge has GitHub merge it once its checks pass.
func TestPRMerge(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	a.set.Reviews = map[string]settings.ReviewSetup{"me/demo": {MergeMethod: github.MethodSquash}}
	view := proto.PRView{PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", MergeBase: "mb"}
	st := proto.PRActions{State: "OPEN", Head: "fix", HeadOID: "h1", Base: "main", HeadExists: true, Mergeable: "MERGEABLE", MergeState: "BLOCKED",
		Methods: []string{github.MethodMerge, github.MethodSquash}, AutoMergeAllowed: true, CanWrite: true, CanUpdate: true, CanUpdateBranch: true, CanDeleteBranch: true}
	acts := make(chan proto.PRActionParams, 8)
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "pr.review":
			return json.RawMessage("null"), nil
		case "pr.action":
			var p proto.PRActionParams
			json.Unmarshal(raw, &p)
			if p.Op != proto.PRActState {
				acts <- p
			}
			s := st
			return proto.PRActionResult{State: &s}, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 900)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	tt.Click("Fix the parser")
	frameUntil(t, tt, "merge button", func() bool { return tt.HasText("Squash and Merge") })
	if !tt.HasText("Waiting on required reviews or checks.") {
		t.Errorf("no merge state; texts %q", tt.Texts())
	}
	tt.Click("Squash and Merge")
	frameUntil(t, tt, "merge dialog", func() bool { return tt.HasText("Merge #7") })
	if err := tt.Click("Merge Commit"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "method chosen", func() bool { return tt.HasText("Make this the repository's default") })
	if err := tt.Click("Enable Auto-Merge"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-acts:
		if p.Op != proto.PRActAutoMerge || p.Method != github.MethodMerge || p.Head != "h1" || p.Number != 7 {
			t.Errorf("merged with %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not merged")
	}
	// Not made the default, as the box was left unticked.
	if m := a.set.Reviews["me/demo"].MergeMethod; m != github.MethodSquash {
		t.Errorf("default %q, want squash", m)
	}
}
