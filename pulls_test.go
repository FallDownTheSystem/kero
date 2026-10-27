package main

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
	"kero/internal/settings"
)

// pullsCalls makes the local host a kerod that answers the Review view's
// calls as handle does, else as one with no pull requests.
func pullsCalls(t *testing.T, a *App, handle func(method string, raw json.RawMessage) (any, error)) {
	t.Helper()
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if v, err := handle(method, raw); v != nil || err != nil {
			return v, err
		}
		if method == "pr.list" {
			return demoPulls(), nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
}

func demoPulls() proto.PRList {
	return proto.PRList{
		Repos:  []proto.GitHubRepo{{Remote: "origin", Slug: "me/demo", Owner: "me", URL: "https://github.com/me/demo"}},
		Remote: "origin",
		PRs: []proto.PullRequest{{Number: 7, Title: "Fix the parser", Body: "Fixes KERO-12.", URL: "https://github.com/me/demo/pull/7",
			Author: "ann", Head: "fix", Base: "main", Checks: "success", Review: "APPROVED"}},
	}
}

// frameUntil frames the tester until ok, or fails.
func frameUntil(t *testing.T, tt *ui.Tester, what string, ok func() bool) {
	t.Helper()
	for range 200 {
		tt.Frame()
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %s; texts %q", what, tt.Texts())
}

func noBrowser(t *testing.T) <-chan string {
	opened := make(chan string, 4)
	old := openURL
	openURL = func(u string) { opened <- u }
	t.Cleanup(func() { openURL = old })
	return opened
}

// A pull request's Review, from its menu, fetches its branch and starts the
// repository's review agent in a session of a new worktree of it, which is
// sent the repository's prompt, filled in, once the agent is ready.
func TestPullsReview(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	a.set.Reviews = map[string]settings.ReviewSetup{"me/demo": {Command: "codex --full-auto", Prompt: "/code-review {ticket} #{number} of {branch} by {author}"}}
	fetches := make(chan proto.PRFetchParams, 2)
	creates := make(chan proto.CreateParams, 2)
	sends := make(chan proto.SendParams, 4)
	starts := make(chan proto.PRReviewParams, 2)
	var mu sync.Mutex
	var drafts []proto.PRReviewDraft
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.drafts":
			mu.Lock()
			defer mu.Unlock()
			return append([]proto.PRReviewDraft{}, drafts...), nil
		case "pr.review":
			var p proto.PRReviewParams
			json.Unmarshal(raw, &p)
			if p.Op == "start" {
				starts <- p
			}
			return proto.PRReviewDraft{Repo: "me/demo", Number: 7, Comments: []proto.PRDraftComment{}}, nil
		case "pr.fetch":
			var p proto.PRFetchParams
			json.Unmarshal(raw, &p)
			fetches <- p
			return proto.PRFetched{Branch: "review/pr-7", Base: "origin/fix", PushRemote: "origin", PushBranch: "fix"}, nil
		case "create":
			var p proto.CreateParams
			json.Unmarshal(raw, &p)
			creates <- p
			return proto.Session{ID: "s2", Project: "p1", Name: "#7", Panes: []proto.Pane{{ID: "r1", Dir: "/tmp/demo-fix", Status: proto.Working, Progress: -1}}}, nil
		case "send":
			var p proto.SendParams
			json.Unmarshal(raw, &p)
			sends <- p
			return true, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "pull request", func() bool { return tt.HasText("Fix the parser") })
	for _, want := range []string{"#7", "Approved"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	shot(t, tt, "pulls")

	if err := tt.RightClick("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Review in New Worktree"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	w := a.prReview
	if w == nil || w.mode != reviewWorktree {
		t.Fatalf("review asked %+v", w)
	}
	if want := "/code-review KERO-12 #7 of fix by ann"; w.prompt != want {
		t.Errorf("prompt %q, want %q", w.prompt, want)
	}
	tt.Frame()
	shot(t, tt, "pulls-review")
	if err := tt.Click("Start Review"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	select {
	case p := <-fetches:
		if p.Number != 7 || p.Head != "fix" || p.Remote != "origin" || p.CrossRepo {
			t.Errorf("fetched %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no pr.fetch")
	}
	var cp proto.CreateParams
	for range 300 {
		tt.Frame()
		select {
		case cp = <-creates:
		default:
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
	if cp.Worktree != "review/pr-7" || cp.Base != "origin/fix" || cp.PushRemote != "origin" || cp.PushBranch != "fix" || cp.Dir != "/tmp/demo" || len(cp.Command) != 2 || cp.Command[0] != "codex" {
		t.Fatalf("created %+v", cp)
	}
	frameUntil(t, tt, "launch", func() bool { return len(a.launches) == 1 })
	select {
	case p := <-starts:
		if p.Agent != "r1" || p.Number != 7 || p.Remote != "origin" {
			t.Errorf("draft started %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no draft review started for the agent")
	}
	// The review works in the background: the user stays on the list.
	if a.selected != "s1" || a.side.pulls.page != nil {
		t.Errorf("went to session %q, page %+v", a.selected, a.side.pulls.page)
	}
	if _, ok := tt.Find("Reviewing"); ok {
		t.Errorf("a pull request with no draft shows a draft; texts %q", tt.Texts())
	}
	// As the agent writes its draft, kerod says so, and the list marks the
	// pull request: Reviewing while the agent works, with its comments.
	mu.Lock()
	drafts = []proto.PRReviewDraft{{Repo: "me/demo", Remote: "origin", Number: 7, Agent: "r1",
		Comments: []proto.PRDraftComment{{ID: 1, Path: "a.go", Line: 1, Body: "x", By: "agent"}, {ID: 2, Path: "a.go", Line: 2, Body: "y", By: "agent"}}}}
	mu.Unlock()
	a.event(a.local(), a.local().cl, proto.Message{Event: "review", Review: &proto.PRReviewRef{Slug: "Me/Demo", Number: 7}})
	frameUntil(t, tt, "the draft's pill", func() bool { _, ok := tt.Find("Reviewing"); return ok })
	shot(t, tt, "pulls-reviewing")

	// Not while the agent works.
	tt.Advance(3 * time.Second)
	tt.Frame()
	if len(sends) > 0 || len(a.launches) != 1 {
		t.Fatal("the prompt went to a busy pane")
	}
	p := a.paneByID("r1")
	p.Agent, p.Status, p.Activity = "codex", proto.Idle, time.Now().UnixMilli()
	tt.Advance(2 * time.Second)
	tt.Frame()
	select {
	case s := <-sends:
		if s.ID != "r1" || !s.Paste || s.Data != "/code-review KERO-12 #7 of fix by ann" {
			t.Errorf("sent %+v", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the prompt wasn't sent")
	}
	if len(a.launches) != 0 {
		t.Error("the launch stays")
	}
	// The agent idle, the review is ready.
	frameUntil(t, tt, "Review Ready", func() bool { _, ok := tt.Find("Review Ready"); return ok })
	shot(t, tt, "pulls-ready")

	// Its comments all deleted, the pull request is in review no more.
	mu.Lock()
	drafts[0].Comments = nil
	mu.Unlock()
	a.event(a.local(), a.local().cl, proto.Message{Event: "review", Review: &proto.PRReviewRef{Slug: "me/demo", Number: 7}})
	frameUntil(t, tt, "Review back", func() bool { _, ok := tt.Find("Review Ready"); return !ok })
	if _, ok := tt.Find("Review"); !ok {
		t.Errorf("no Review; texts %q", tt.Texts())
	}
}

// The review setup keeps a repository's command and prompt by its slug,
// and the default's own words as none, which follows Kero's default.
func TestPullsReviewSetup(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 720)
	a.openReviewSetup("Me/Demo")
	tt.Frame()
	if !tt.HasText("Review Setup for Me/Demo") {
		t.Fatalf("no setup; texts %q", tt.Texts())
	}
	a.prSetup.command = "codex"
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if r := a.set.Reviews["me/demo"]; r != (settings.ReviewSetup{Command: "codex"}) {
		t.Errorf("kept %+v", r)
	}
	a.openReviewSetup("me/demo")
	a.prSetup.command, a.prSetup.prompt = "", DefaultReviewPrompt
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := a.set.Reviews["me/demo"]; ok {
		t.Errorf("a setup of defaults stays: %+v", a.set.Reviews)
	}
}

// New Pull Request shows what the branch brings, its one commit's subject
// as the title; Create pushes the commit the push remote hasn't, then
// makes the pull request and opens it; Show on GitHub opens GitHub's page
// that makes it.
func TestPRForm(t *testing.T) {
	a := testApp(t)
	g, _ := gitDemo(a)
	g.Branch = "feature"
	opened := noBrowser(t)
	ops := make(chan proto.GitOpParams, 2)
	creates := make(chan proto.PRCreateParams, 2)
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.draft":
			d := proto.PRDraft{Repos: demoPulls().Repos, Remote: "origin", Base: "main", Head: "feature", HeadOwner: "me", PushRemote: "origin", MergeBase: "mb",
				Commits:  []proto.GitCommit{{Hash: "abc1234567", Author: "me", Subject: "Add the parser"}},
				Files:    []proto.GitFile{{Path: "parse.go", Status: "added", Added: 3}},
				Unpushed: 1, Uncommitted: 2}
			return d, nil
		case "git.branches":
			return proto.GitBranches{Branches: []proto.GitBranch{{Name: "feature", Current: true}, {Name: "origin/main", Remote: true}, {Name: "origin/dev", Remote: true}}}, nil
		case "git.file":
			return proto.GitText{New: "package parse\n", Hash: "h"}, nil
		case "git.op":
			var p proto.GitOpParams
			json.Unmarshal(raw, &p)
			ops <- p
			return proto.GitOpResult{}, nil
		case "pr.create":
			var p proto.PRCreateParams
			json.Unmarshal(raw, &p)
			creates <- p
			if p.Show {
				return proto.PRCreated{URL: "https://github.com/me/demo/compare/main...feature"}, nil
			}
			return proto.PRCreated{URL: "https://github.com/me/demo/pull/8"}, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("New Pull Request"); err != nil {
		t.Fatal(err)
	}
	f := a.side.pulls.form
	frameUntil(t, tt, "draft", func() bool { return f.draft != nil })
	if f.title != "Add the parser" {
		t.Errorf("title %q", f.title)
	}
	if len(f.branches) != 2 || f.branches[0].name != "main" {
		t.Errorf("branches %v", f.branches)
	}
	for _, want := range []string{"origin/main", "1 commit", "1 file changed", "parse.go", "1 commit isn't on origin", "2 uncommitted files"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	if err := tt.Click("parse.go"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "diff", func() bool { sf := f.files["parse.go"]; return sf != nil && sf.d != nil })
	tt.Frame()
	shot(t, tt, "pr-form")

	if err := tt.Click("More Ways to Create"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Show on GitHub"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	if p := nextOp(t, ops); p.Op != "push" {
		t.Errorf("op %+v", p)
	}
	frameUntil(t, tt, "compare page", func() bool { return len(opened) > 0 })
	if u := <-opened; u != "https://github.com/me/demo/compare/main...feature" {
		t.Errorf("opened %q", u)
	}
	if p := <-creates; !p.Show || p.Title != "Add the parser" || p.Base != "main" || p.Remote != "origin" {
		t.Errorf("show %+v", p)
	}
	if a.side.pulls.form != f {
		t.Fatal("Show on GitHub closed the form")
	}

	f.body = "Parses."
	if err := tt.Click("Create Pull Request"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "pull request", func() bool { return len(opened) > 0 })
	if len(ops) > 0 {
		t.Error("pushed again what was pushed")
	}
	if u := <-opened; u != "https://github.com/me/demo/pull/8" {
		t.Errorf("opened %q", u)
	}
	if p := <-creates; p.Show || p.Draft || p.Title != "Add the parser" || p.Body != "Parses." {
		t.Errorf("created %+v", p)
	}
	tt.Frame()
	if a.side.pulls.form != nil {
		t.Error("the form stays after its pull request is made")
	}
}

// A click on a pull request shows its page: its state, its description,
// how each reviewer's review stands, its checks and its conversation; its
// Files tab, its diffs with their threads of comments under their lines.
// Its files open from the worktree that has it checked out, and only then.
func TestPRPage(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	view := proto.PRView{
		PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", Additions: 2, Deletions: 1, Labels: []string{"bug"},
		Requested: []string{"bob"},
		Reviews: []proto.PRReview{
			{Author: "cid", State: "CHANGES_REQUESTED", Body: "Needs a test.", Submitted: 100},
			{Author: "cid", State: "COMMENTED", Body: "", Submitted: 200},
			{Author: "dee", State: "APPROVED", Submitted: 300},
		},
		Comments:  []proto.PRComment{{Author: "eve", Body: "Looks **good**.\n\n<!-- bot -->\n<details><summary>Bot notes</summary>\n<ul><li>hidden &amp; shown</li></ul></details>", Created: 150}},
		CheckRuns: []proto.PRCheck{{Name: "build", Workflow: "CI", State: "success"}, {Name: "lint", State: "failure", URL: "https://ci/lint"}},
		LineComments: []proto.PRLineComment{
			{ID: 1, Path: "parse.go", Body: "Off by one?", Author: "cid", Side: "RIGHT", Line: 2},
			{ID: 2, ReplyTo: 1, Path: "parse.go", Body: "Fixed.", Author: "ann", Side: "RIGHT", Line: 2},
			{ID: 3, Path: "parse.go", Body: "Old note", Author: "cid", Side: "RIGHT", OriginalLine: 9},
		},
		MergeBase: "mb",
		Files:     []proto.GitFile{{Path: "parse.go", Status: "modified", Added: 2, Removed: 1, From: "mb", To: "h1"}},
	}
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "git.file":
			return proto.GitText{Old: "package parse\n\nfunc a() {}\n", New: "package parse\n\nfunc b() {}\nfunc c() {}\n", Hash: "h"}, nil
		case "pr.review":
			return json.RawMessage("null"), nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 1200)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	g := a.side.pulls.page
	if g == nil {
		t.Fatal("no page")
	}
	frameUntil(t, tt, "page", func() bool { return g.res != nil })
	tt.Frame()
	shot(t, tt, "pr-page")
	for _, want := range []string{"ann wants to merge fix into main", "bug", "Reviewers", "Awaiting review", "Changes requested", "Approved", "Needs a test.", "eve", "Bot notes", "Old note", "outdated · parse.go:9", "parse.go:2", "Off by one?", "Fixed."} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText("bot") {
		t.Error("an HTML comment shows")
	}
	if tt.HasText("hidden & shown") {
		t.Error("a <details> shows folded")
	}
	if err := tt.Click("Bot notes"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("hidden & shown") {
		t.Errorf("an opened <details> hides its text; texts %q", tt.Texts())
	}
	if rs := prReviewersOf(&view); len(rs) != 3 || rs[0] != (prReviewer{login: "cid", state: "CHANGES_REQUESTED"}) || rs[2] != (prReviewer{login: "bob", pending: true}) {
		t.Errorf("reviewers %+v", rs)
	}

	if err := tt.Click("Files (1)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("parse.go"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	frameUntil(t, tt, "diff", func() bool { sf := g.files["parse.go"]; return sf != nil && sf.d != nil })
	tt.Frame()
	tt.Frame()
	shot(t, tt, "pr-page-files")
	for _, want := range []string{"Off by one?", "Fixed."} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText("Old note") {
		t.Error("an outdated comment shows on the diff")
	}
	if err := tt.RightClick("parse.go"); err != nil {
		t.Fatal(err)
	}
	if m := tt.Menu(); m != nil {
		t.Errorf("a file of a pull request not checked out has a menu %q", m)
	}
	tt.CloseMenu()

	g.res.Checkout = "/tmp/demo-fix"
	tt.Frame()
	if err := tt.RightClick("parse.go"); err != nil {
		t.Fatal(err)
	}
	if m := tt.Menu(); !slices.Contains(m, "Open in Pane") {
		t.Errorf("menu %q", m)
	}
	tt.CloseMenu()
}

// A pull request's draft review shows on its diffs, the agent's comments
// marked; deleting one deletes it in kerod's draft; Post Review, once the
// user confirms it, posts it with the verdict chosen.
func TestPRDraftReview(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	view := proto.PRView{PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", MergeBase: "mb", Viewer: "me",
		Files: []proto.GitFile{{Path: "parse.go", Status: "modified", Added: 2, Removed: 1, From: "mb", To: "h1"}}}
	draft := proto.PRReviewDraft{Repo: "me/demo", Remote: "origin", Number: 7, Head: "h1", MergeBase: "mb", Agent: "zz", Summary: "One bug.",
		Comments: []proto.PRDraftComment{{ID: 1, Path: "parse.go", Side: "RIGHT", Line: 3, Body: "This drops a token.", By: "agent"}}}
	ops := make(chan proto.PRReviewParams, 8)
	var mu sync.Mutex
	posted := false
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "git.file":
			return proto.GitText{Old: "package parse\n\nfunc a() {}\n", New: "package parse\n\nfunc b() {}\nfunc c() {}\n", Hash: "h"}, nil
		case "pr.review":
			var p proto.PRReviewParams
			json.Unmarshal(raw, &p)
			if p.Op != "get" {
				ops <- p
			}
			mu.Lock()
			defer mu.Unlock()
			switch p.Op {
			case "delete":
				draft.Comments = nil
			case "post":
				d := draft
				d.Posted = "https://github.com/me/demo/pull/7#review"
				posted = true
				return d, nil
			}
			if posted {
				return json.RawMessage("null"), nil
			}
			return draft, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	g := a.side.pulls.page
	frameUntil(t, tt, "draft", func() bool { return g.res != nil && g.review.draft != nil })
	if !tt.HasText("Your review, 1 comment") || g.review.summary != "One bug." {
		t.Errorf("summary %q; texts %q", g.review.summary, tt.Texts())
	}
	g.tab = prFiles
	tt.Frame()
	if err := tt.Click("parse.go"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "diff", func() bool { sf := g.files["parse.go"]; return sf != nil && sf.d != nil })
	tt.Frame()
	tt.Frame()
	shot(t, tt, "pr-draft")
	for _, want := range []string{"This drops a token.", "Agent", "parse.go:3"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	// The viewer isn't the author: every verdict is there.
	if !tt.HasText("Request Changes") {
		t.Error("no Request Changes")
	}

	if err := tt.Click("Delete Comment"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-ops:
		if p.Op != "delete" || p.ID != 1 || p.Number != 7 {
			t.Errorf("deleted %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the comment wasn't deleted in kerod")
	}
	frameUntil(t, tt, "deleted", func() bool { return len(g.review.draft.Comments) == 0 })

	g.review.event = "REQUEST_CHANGES"
	a.confirmPost(g)
	if a.confirm == nil || !strings.Contains(a.confirm.title, "me/demo#7") {
		t.Fatalf("confirm %+v", a.confirm)
	}
	a.confirm.run()
	select {
	case p := <-ops:
		if p.Op != "post" || p.Event != "REQUEST_CHANGES" {
			t.Errorf("posted %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not posted")
	}
	frameUntil(t, tt, "posted", func() bool { return !g.review.posting && g.review.draft == nil })
	// The bar stays, for another review, with nothing of the one posted.
	if !tt.HasText("Your review, no comments yet") || g.review.summary != "" {
		t.Errorf("summary %q; texts %q", g.review.summary, tt.Texts())
	}
}

// Next Comment goes through the comments on a pull request's diffs, the
// draft review's and the threads posted, in the order of the files, then
// of the lines: each on the Files tab, its file unfolded, in view.
func TestPRCommentJump(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	var long []string
	for i := range 120 {
		long = append(long, "line "+strconv.Itoa(i))
	}
	oldLong := strings.Join(long, "\n") + "\n"
	// Every line changed: nothing folds, the comments far apart.
	for i := range long {
		long[i] += " changed"
	}
	newLong := strings.Join(long, "\n") + "\n"
	view := proto.PRView{PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", MergeBase: "mb", Viewer: "me",
		Files: []proto.GitFile{
			{Path: "a.go", Status: "modified", Added: 2, Removed: 1, From: "mb", To: "h1"},
			{Path: "b.go", Status: "modified", Added: 120, Removed: 120, From: "mb", To: "h1"},
		},
		LineComments: []proto.PRLineComment{{ID: 5, Path: "a.go", Body: "Posted thread.", Author: "cid", Side: "RIGHT", Line: 3}},
	}
	// The draft's comments come first in the draft, but are on the second
	// file, the one far down first: near its top, then far down.
	draft := proto.PRReviewDraft{Repo: "me/demo", Remote: "origin", Number: 7, Head: "h1", MergeBase: "mb", Agent: "zz",
		Comments: []proto.PRDraftComment{
			{ID: 1, Path: "b.go", Side: "RIGHT", Line: 101, Body: "Agent's note far down.", By: "agent"},
			{ID: 2, Path: "b.go", Side: "RIGHT", Line: 11, Body: "Agent's note near the top.", By: "agent"},
		}}
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "git.file":
			var p proto.GitParams
			json.Unmarshal(raw, &p)
			if p.Path == "b.go" {
				return proto.GitText{Old: oldLong, New: newLong, Hash: "hb"}, nil
			}
			return proto.GitText{Old: "package a\n\nfunc a() {}\n", New: "package a\n\nfunc b() {}\nfunc c() {}\n", Hash: "ha"}, nil
		case "pr.review":
			return draft, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	g := a.side.pulls.page
	frameUntil(t, tt, "draft", func() bool { return g.res != nil && g.review.draft != nil })
	tt.Frame()
	if _, ok := tt.Find("3"); !ok || g.tab != prConversation {
		t.Errorf("tab %q; texts %q", g.tab, tt.Texts())
	}
	inView := func(text string) bool {
		r, ok := tt.Find(text)
		return ok && r.Y >= 0 && r.Y+r.H <= 720
	}

	if err := tt.Click("Next Comment"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "the thread on a.go", func() bool { return inView("Posted thread.") })
	if _, ok := tt.Find("1/3"); g.tab != prFiles || !ok || g.at.thread != 5 {
		t.Errorf("tab %q, at %+v; texts %q", g.tab, g.at, tt.Texts())
	}
	if !a.fileCollapsed(prFileKey(g.res, "b.go")) {
		t.Error("b.go unfolded before its comment was jumped to")
	}

	if err := tt.Click("Next Comment"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "the draft's first comment on b.go, in view", func() bool { return inView("Agent's note near the top.") })
	for range 3 {
		tt.Frame()
	}
	if g.at.draft != 2 || inView("Agent's note far down.") {
		t.Errorf("at %+v, the next comment in view already", g.at)
	}
	// The next in the same file, its diff shown already.
	if err := tt.Click("Next Comment"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "the draft's comment on b.go, in view", func() bool { return inView("Agent's note far down.") })
	for range 3 {
		tt.Frame()
	}
	if !inView("Agent's note far down.") || g.at.draft != 1 || g.rv.at != 1 {
		t.Errorf("at %+v, rv.at %d", g.at, g.rv.at)
	}
	// It is there: the user scrolls freely again.
	if g.reveal != 0 {
		t.Errorf("still revealing, %d frames left", g.reveal)
	}
	shot(t, tt, "pr-jump")
	// And back up, in the same file.
	if err := tt.Click("Previous Comment"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "the comment above, in view", func() bool { return inView("Agent's note near the top.") })
	if err := tt.Click("Next Comment"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "the comment below again", func() bool { return inView("Agent's note far down.") })

	// Round, both ways.
	if err := tt.Click("Next Comment"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if g.at.thread != 5 {
		t.Errorf("after the last, at %+v", g.at)
	}
	if err := tt.Click("Previous Comment"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if g.at.draft != 1 {
		t.Errorf("before the first, at %+v", g.at)
	}
}

// Polling learns a repository's pull requests first, and notifies of
// none; then of each new one, but, of all, the user's own; once for each,
// and none after a restart, which the prefs remember them through.
func TestPullsPoll(t *testing.T) {
	a := testApp(t)
	a.st.Projects[0].Dir = "/tmp/demo"
	a.set.PRNotify = "all"
	var notes []string
	old := showNote
	showNote = func(o mygo.NotificationOptions, click func()) { notes = append(notes, o.Title+": "+o.Body) }
	t.Cleanup(func() { showNote = old })
	var mu sync.Mutex
	prs := []proto.PullRequest{{Number: 7, Title: "Fix", Author: "ann"}}
	asked := make(chan proto.PRListParams, 8)
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		if method != "pr.list" {
			return nil, nil
		}
		var p proto.PRListParams
		json.Unmarshal(raw, &p)
		asked <- p
		mu.Lock()
		defer mu.Unlock()
		l := demoPulls()
		l.PRs, l.Viewer = slices.Clone(prs), "me"
		return l, nil
	})
	run := func() []proto.PullRequest {
		t.Helper()
		var res proto.PRList
		if err := a.local().cl.Call("pr.list", proto.PRListParams{Dir: "/tmp/demo", Viewer: true}, &res); err != nil {
			t.Fatal(err)
		}
		<-asked
		return a.pullsPolled([]polled{{a.local(), "p1", res}}, true)
	}
	if f := run(); len(f) != 0 {
		t.Errorf("the first poll notified of %+v", f)
	}
	mu.Lock()
	prs = append(prs, proto.PullRequest{Number: 9, Title: "Mine", Author: "me"}, proto.PullRequest{Number: 8, Title: "New", Author: "bob"})
	mu.Unlock()
	if f := run(); len(f) != 1 || f[0].Number != 8 {
		t.Errorf("notified of %+v", f)
	}
	if len(notes) != 1 || notes[0] != "New pull request in me/demo: #8 New · bob" {
		t.Errorf("notifications %q", notes)
	}
	if f := run(); len(f) != 0 {
		t.Errorf("notified again of %+v", f)
	}
	if got := newApp().prefs.PRSeen[a.local().name+"|me/demo"]; !slices.Equal(got, []int{7, 9, 8}) {
		t.Errorf("remembered %v", got)
	}

	// Polling asks each project's folder, as the settings say, and only
	// once it is time.
	a.poll.last = time.Time{}
	a.pollPulls()
	select {
	case p := <-asked:
		if p.Dir != "/tmp/demo" || p.Filter != "" || !p.Viewer {
			t.Errorf("asked %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not polled")
	}
	a.poll.busy = false
	a.pollPulls()
	select {
	case p := <-asked:
		t.Errorf("polled again at once: %+v", p)
	case <-time.After(200 * time.Millisecond):
	}
	a.set.PRNotify, a.poll.last = "off", time.Time{}
	a.pollPulls()
	select {
	case p := <-asked:
		t.Errorf("polled while off: %+v", p)
	case <-time.After(200 * time.Millisecond):
	}
}

// The base picker lists every branch of the remote, its default first,
// then the most recent; typing filters them, and Return takes the branch
// the arrows are on as the base.
func TestPRBasePicker(t *testing.T) {
	a := testApp(t)
	g, _ := gitDemo(a)
	g.Branch = "feature"
	noBrowser(t)
	drafts := make(chan proto.PRDraftParams, 4)
	branches := []proto.GitBranch{{Name: "feature", Current: true}, {Name: "origin/HEAD", Remote: true}}
	for i := range 150 {
		branches = append(branches, proto.GitBranch{Name: "origin/feat-" + strconv.Itoa(i), Remote: true, Time: int64(1000 + i)})
	}
	branches = append(branches, proto.GitBranch{Name: "origin/main", Remote: true, Time: 1})
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.draft":
			var p proto.PRDraftParams
			json.Unmarshal(raw, &p)
			drafts <- p
			base := p.Base
			if base == "" {
				base = "main"
			}
			repos := demoPulls().Repos
			repos[0].Default = "main"
			return proto.PRDraft{Repos: repos, Remote: "origin", Base: base, Head: "feature", PushRemote: "origin", MergeBase: "mb",
				Commits: []proto.GitCommit{{Hash: "abc1234567", Subject: "Add the parser"}}}, nil
		case "git.branches":
			return proto.GitBranches{Branches: branches}, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("New Pull Request"); err != nil {
		t.Fatal(err)
	}
	f := a.side.pulls.form
	frameUntil(t, tt, "draft", func() bool { return f.draft != nil })
	<-drafts
	if len(f.branches) != 151 || f.branches[0].name != "main" || f.branches[1].name != "feat-149" || f.branches[150].name != "feat-0" {
		t.Fatalf("%d branches, first %v, last %v", len(f.branches), f.branches[:2], f.branches[len(f.branches)-1])
	}

	if err := tt.Click("The branch that the pull request goes into"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !f.picker.open {
		t.Fatal("the picker didn't open")
	}
	tt.Type("feat-1")
	tt.Frame()
	shot(t, tt, "pr-base-picker")
	// The matches, the most recent first: feat-149, feat-148, …
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if f.picker.open {
		t.Error("the picker stays open")
	}
	select {
	case p := <-drafts:
		if p.Base != "feat-148" || p.Remote != "origin" {
			t.Errorf("asked %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no draft of the base chosen")
	}
}

// draftServer answers pr.review as kerod would, roughly: the draft that
// start makes, and comments and a summary kept in it; it reports each op
// but get.
func draftServer(ops chan<- proto.PRReviewParams) func(raw json.RawMessage) (any, error) {
	var mu sync.Mutex
	var dr *proto.PRReviewDraft
	return func(raw json.RawMessage) (any, error) {
		var p proto.PRReviewParams
		json.Unmarshal(raw, &p)
		mu.Lock()
		defer mu.Unlock()
		if p.Op != "get" {
			ops <- p
		}
		switch p.Op {
		case "start":
			if dr == nil {
				dr = &proto.PRReviewDraft{Repo: "me/demo", Remote: "origin", Number: 7, Head: "h1", MergeBase: "mb", Comments: []proto.PRDraftComment{}}
			}
		case "summary":
			dr.Summary = p.Summary
		case "comment":
			c := p.Comment
			c.ID = len(dr.Comments) + 1
			dr.Comments = append(dr.Comments, c)
		case "post":
			d := *dr
			dr = nil
			d.Posted = "https://github.com/me/demo/pull/7#review"
			return d, nil
		}
		if dr == nil {
			return json.RawMessage("null"), nil
		}
		return *dr, nil
	}
}

// A pull request is approved with no comment on its lines: the summary
// and the verdict alone, the draft they post from started as it posts.
func TestPRReviewWithoutComments(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	view := proto.PRView{PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", MergeBase: "mb", Viewer: "me",
		Files: []proto.GitFile{{Path: "parse.go", Status: "modified", Added: 2, Removed: 1, From: "mb", To: "h1"}}}
	ops := make(chan proto.PRReviewParams, 8)
	review := draftServer(ops)
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "pr.review":
			return review(raw)
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	g := a.side.pulls.page
	frameUntil(t, tt, "the review's bar", func() bool { return g.res != nil && tt.HasText("Your review, no comments yet") })
	if g.review.draft != nil {
		t.Fatal("a draft before any was written")
	}
	shot(t, tt, "pr-review-bar")
	if err := tt.Click("Approve"); err != nil {
		t.Fatal(err)
	}
	// Typed, then posted while the field has it.
	g.review.summary = "LGTM, thanks."
	tt.Frame()
	if err := tt.Click("Post Review"); err != nil {
		t.Fatal(err)
	}
	if a.confirm == nil {
		t.Fatalf("no confirmation; texts %q", tt.Texts())
	}
	a.confirm.run()
	var got []string
	for len(got) < 3 {
		select {
		case p := <-ops:
			got = append(got, p.Op+":"+p.Summary+p.Event)
		case <-time.After(3 * time.Second):
			t.Fatalf("ops %q", got)
		}
		tt.Frame()
	}
	if want := []string{"start:", "summary:LGTM, thanks.", "post:APPROVE"}; !slices.Equal(got, want) {
		t.Errorf("ops %q, want %q", got, want)
	}
	frameUntil(t, tt, "posted", func() bool { return !g.review.posting })
}

// A comment on a file, from its header, shows above its diff, and goes to
// the draft as the file's; one posted on a file shows there too, not as
// outdated; the jump goes to them before the file's lines.
func TestPRFileComments(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	view := proto.PRView{PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", MergeBase: "mb", Viewer: "me",
		Files: []proto.GitFile{{Path: "parse.go", Status: "modified", Added: 2, Removed: 1, From: "mb", To: "h1"}},
		LineComments: []proto.PRLineComment{
			{ID: 4, Path: "parse.go", Body: "On a line.", Author: "cid", Side: "RIGHT", Line: 3},
			{ID: 5, Path: "parse.go", Body: "Rename this file?", Author: "dee", File: true},
		}}
	ops := make(chan proto.PRReviewParams, 8)
	review := draftServer(ops)
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "git.file":
			return proto.GitText{Old: "package parse\n\nfunc a() {}\n", New: "package parse\n\nfunc b() {}\nfunc c() {}\n", Hash: "h"}, nil
		case "pr.review":
			return review(raw)
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 720)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	if err := tt.Click("Fix the parser"); err != nil {
		t.Fatal(err)
	}
	g := a.side.pulls.page
	frameUntil(t, tt, "page", func() bool { return g.res != nil })
	tt.Frame()
	// The conversation shows it as the file's, not as outdated.
	if !tt.HasText("Rename this file?") || !tt.HasText("parse.go, the whole file") || tt.HasText("outdated") {
		t.Errorf("the file's thread in the conversation; texts %q", tt.Texts())
	}
	g.tab = prFiles
	tt.Frame()
	if err := tt.Click("Comment on File"); err != nil {
		t.Fatal(err)
	}
	frameUntil(t, tt, "the diff", func() bool { sf := g.files["parse.go"]; return sf != nil && sf.d != nil })
	tt.Frame()
	tt.Type("Split the parser out.")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	var cm proto.PRDraftComment
	for cm.Path == "" {
		select {
		case p := <-ops:
			if p.Op == "comment" {
				cm = p.Comment
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("not kept; texts %q", tt.Texts())
		}
		tt.Frame()
	}
	if !cm.File || cm.Path != "parse.go" || cm.Line != 0 || cm.Body != "Split the parser out." {
		t.Errorf("kept %+v", cm)
	}
	frameUntil(t, tt, "the comment from kerod", func() bool { return g.review.draft != nil && len(g.review.draft.Comments) == 1 })
	tt.Frame()
	shot(t, tt, "pr-file-comments")
	// Above the diff: the file's comments, then its lines'.
	posted, _ := tt.Find("Rename this file?")
	mine, ok := tt.Find("Split the parser out.")
	line, _ := tt.Find("On a line.")
	if !ok || !tt.HasText("parse.go, the whole file") || posted.Y > line.Y || mine.Y > line.Y {
		t.Errorf("posted %v, mine %v, on a line %v; texts %q", posted, mine, line, tt.Texts())
	}
	if spots := prSpots(g); len(spots) != 3 || spots[2].thread != 4 {
		t.Errorf("spots %+v", spots)
	}
}

// Resolve Conversation resolves the thread on GitHub and folds it at once;
// when GitHub refuses, it shows as it was.
func TestPRResolveThread(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	noBrowser(t)
	view := proto.PRView{PullRequest: demoPulls().PRs[0], Repo: demoPulls().Repos[0], State: "OPEN", HeadOID: "h1", MergeBase: "mb",
		LineComments: []proto.PRLineComment{
			{ID: 1, Path: "parse.go", Body: "Off by one?", Author: "cid", Side: "RIGHT", Line: 2, Thread: "T1"},
			{ID: 2, ReplyTo: 1, Path: "parse.go", Body: "Fixed.", Author: "ann", Side: "RIGHT", Line: 2},
		}}
	calls := make(chan proto.PRThreadParams, 4)
	var mu sync.Mutex
	refuse := false
	pullsCalls(t, a, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "pr.view":
			return view, nil
		case "pr.review":
			return json.RawMessage("null"), nil
		case "pr.thread":
			var p proto.PRThreadParams
			json.Unmarshal(raw, &p)
			calls <- p
			mu.Lock()
			defer mu.Unlock()
			if refuse {
				return nil, errors.New("not allowed")
			}
			return true, nil
		}
		return nil, nil
	})
	a.prefs.Side = sideReview
	tt := ui.NewTester(a.view, 1280, 900)
	frameUntil(t, tt, "list", func() bool { return tt.HasText("Fix the parser") })
	tt.Click("Fix the parser")
	g := a.side.pulls.page
	frameUntil(t, tt, "thread", func() bool { return g.res != nil && tt.HasText("Resolve Conversation") })
	if err := tt.Click("Resolve Conversation"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-calls:
		if p.Thread != "T1" || !p.Resolve || p.Number != 7 {
			t.Errorf("resolved %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not resolved on GitHub")
	}
	tt.Frame()
	if tt.HasText("Fixed.") || !tt.HasText("Resolved") {
		t.Errorf("not folded; texts %q", tt.Texts())
	}
	// Opened, then made unresolved, which GitHub refuses.
	tt.Click("Off by one?")
	tt.Frame()
	mu.Lock()
	refuse = true
	mu.Unlock()
	if err := tt.Click("Unresolve"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	<-calls
	frameUntil(t, tt, "resolved again", func() bool { return g.res.LineComments[0].Resolved })
}
