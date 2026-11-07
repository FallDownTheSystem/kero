package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

// Check Out, from a pull request's menu and its page, checks it out in the
// pane's worktree, or in a new session's worktree on its branch; one that
// has it already, kerod says, the user goes to, in a session there.
func TestPRCheckoutMenu(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	checkouts := make(chan proto.PRCheckoutParams, 4)
	creates := make(chan proto.CreateParams, 4)
	reuse := ""
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.checkout":
			var p proto.PRCheckoutParams
			json.Unmarshal(raw, &p)
			checkouts <- p
			if reuse != "" {
				return proto.PRCheckedOut{Branch: "fix", Worktree: reuse}, nil
			}
			return proto.PRCheckedOut{Branch: "fix", Base: "origin/fix", PushRemote: "origin", PushBranch: "fix"}, nil
		case "create":
			var p proto.CreateParams
			json.Unmarshal(raw, &p)
			creates <- p
			return proto.Session{ID: "s2", Project: "p1", Name: "#7", Panes: []proto.Pane{{ID: "c1", Dir: p.Dir}}}, nil
		case "pr.view":
			return proto.PRView{PullRequest: demoPulls().PRs[0], State: "OPEN", Files: []proto.GitFile{}}, nil
		case "pr.review":
			return json.RawMessage("null"), nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	checkout := func(item string) proto.PRCheckoutParams {
		t.Helper()
		if err := tt.RightClick("Fix the parser"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(item); err != nil {
			t.Fatalf("%v; menu %q", err, tt.Menu())
		}
		tt.Frame()
		select {
		case p := <-checkouts:
			return p
		case <-time.After(3 * time.Second):
			t.Fatal("no pr.checkout")
		}
		return proto.PRCheckoutParams{}
	}
	created := func() proto.CreateParams {
		t.Helper()
		for range 300 {
			tt.Frame()
			select {
			case p := <-creates:
				// Back to the list, from the session it goes to.
				frameUntil(t, tt, "the new session", func() bool { return a.selected == "s2" })
				a.selectSession("s1")
				frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
				return p
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
		t.Fatal("no session created")
		return proto.CreateParams{}
	}

	if p := checkout("Check Out Here"); !p.Here || p.Number != 7 || p.Head != "fix" || p.Remote != "origin" {
		t.Fatalf("checked out %+v", p)
	}
	frameUntil(t, tt, "the toast", func() bool { return tt.HasText("Checked out #7 on fix") })
	select {
	case p := <-creates:
		t.Fatalf("checking out here made a session %+v", p)
	default:
	}

	if p := checkout("Check Out in New Worktree"); p.Here {
		t.Fatalf("checked out %+v", p)
	}
	if p := created(); p.Worktree != "fix" || p.Base != "origin/fix" || p.PushRemote != "origin" || p.PushBranch != "fix" || p.Dir != "/tmp/demo" {
		t.Fatalf("created %+v", p)
	}

	// A worktree has it already: a session there, which none works in.
	reuse = "/tmp/demo/.kero/worktrees/review-pr-7"
	checkout("Check Out in New Worktree")
	if p := created(); p.Worktree != "" || p.Dir != reuse {
		t.Fatalf("created %+v", p)
	}

	// The page's Check Out has the same places.
	if err := tt.Click("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "page", func() bool { return a.side.pulls.page != nil && a.side.pulls.page.res != nil })
	shot(t, tt, "pr-page-checkout")
	if err := tt.Click("Check Out"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("Check Out Here"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	select {
	case p := <-checkouts:
		if !p.Here || p.Number != 7 {
			t.Errorf("checked out %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no pr.checkout from the page")
	}
}
