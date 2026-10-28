package main

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/git"
	"kero/internal/gitgraph"
	"kero/internal/proto"
)

// gitCalls makes the local host a kerod that tells the git.op and git.show
// calls it takes, and answers git.index with ix, when set, and git.op as
// done; no other call.
func gitCalls(t *testing.T, a *App, ix *proto.GitIndex) (<-chan proto.GitOpParams, <-chan string) {
	t.Helper()
	ops, shows := make(chan proto.GitOpParams, 8), make(chan string, 8)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.index":
			if ix != nil {
				return ix, nil
			}
		case "git.op":
			var p proto.GitOpParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			ops <- p
			return proto.GitOpResult{}, nil
		case "git.show":
			var p proto.GitShowParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			shows <- p.Rev
		}
		return nil, errors.New("no git in a test")
	})
	a.local().cl = cl
	return ops, shows
}

func nextOp(t *testing.T, ops <-chan proto.GitOpParams) proto.GitOpParams {
	t.Helper()
	select {
	case p := <-ops:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("no git.op came")
		return proto.GitOpParams{}
	}
}

// gitDemo puts the session's panes in a worktree of /tmp/demo, a.go of
// which is staged and b.go and new.txt not.
func gitDemo(a *App) (*proto.GitInfo, *proto.GitIndex) {
	s := &a.st.Sessions[0]
	g := &proto.GitInfo{Root: "/tmp/demo", Branch: "main", Default: "main", Head: "c3", Upstream: "origin/main", Files: 3, Added: 6, Removed: 1, Staged: 1}
	s.Panes[0].Git, s.Panes[1].Git = g, g
	ix := &proto.GitIndex{Root: "/tmp/demo",
		Staged: []proto.GitFile{{Path: "a.go", Status: "modified", Added: 2, Removed: 1}},
		Unstaged: []proto.GitFile{
			{Path: "b.go", Status: "modified", Added: 3},
			{Path: "new.txt", Status: "untracked", Added: 1},
		}}
	return g, ix
}

// In Branch, a change's Revert writes the file back with the base's lines
// in its place; there is no Keep, as nothing of a branch waits for review.
func TestBranchRevert(t *testing.T) {
	a := testApp(t)
	g, _ := gitDemo(a)
	g.Branch = "feature"
	writes := make(chan proto.GitWriteParams, 4)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.file":
			return proto.GitText{Old: "package b\n\nfunc a() {}\n", New: "package b\n\nfunc b() {}\n", Hash: "h"}, nil
		case "git.revert":
			var p proto.GitWriteParams
			json.Unmarshal(raw, &p)
			writes <- p
			return "h2", nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
	s := &a.st.Sessions[0]
	a.prefs.Side = sideChanges
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseBranch)
	ch.base, ch.scope, ch.stamp, ch.fetched = git.BaseBranch, scope, gitStamp(g)+"|0", time.Now()
	ch.res = &proto.GitChanges{Root: g.Root, BaseName: "main", Files: []proto.GitFile{{Path: "b.go", Status: "modified", Added: 1, Removed: 1}}}
	key := fileKey(scope, "", "b.go")
	ch.collapsed = map[string]bool{key: false}

	tt := ui.NewTester(a.view, 1280, 720)
	for range 100 {
		tt.Frame()
		if sf := ch.files[key]; sf != nil && sf.d != nil && len(sf.d.Hunks()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The change's bar shows under the pointer, down the diff below the
	// file's header.
	hdr, ok := tt.Find("b.go")
	if !ok {
		t.Fatalf("no b.go; texts %q", tt.Texts())
	}
	for y := hdr.Y + hdr.H; y < hdr.Y+200 && !tt.HasText("Revert"); y += 6 {
		tt.Move(hdr.X+40, y)
		tt.Frame()
	}
	if tt.HasText("Keep") {
		t.Error("a branch's change has Keep")
	}
	if err := tt.Click("Revert"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	select {
	case p := <-writes:
		if p.Base != git.BaseBranch || p.Path != "b.go" || p.Hash != "h" || p.Text != "package b\n\nfunc a() {}\n" {
			t.Errorf("wrote %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Revert wrote nothing")
	}
}

// The tree shows a section's files in their folders, and a file's or a
// folder's menu adds it to .gitignore: at once while git tracks nothing
// of it, else as the user says, who may stop tracking it too.
func TestChangesTreeIgnore(t *testing.T) {
	a := testApp(t)
	g, ix := gitDemo(a)
	ix.Unstaged = []proto.GitFile{{Path: "src/b.go", Status: "modified", Added: 3}, {Path: "src/gen/new.txt", Status: "untracked", Added: 1}}
	ops := make(chan proto.GitOpParams, 8)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.index":
			return ix, nil
		case "git.op":
			var p proto.GitOpParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			ops <- p
			if p.Op == "tracked" && slices.Equal(p.Paths, []string{"src/gen"}) {
				return proto.GitOpResult{Output: "src/gen/old.txt"}, nil
			}
			return proto.GitOpResult{}, nil
		}
		return nil, errors.New("no git in a test")
	})
	a.local().cl = cl
	s := &a.st.Sessions[0]
	a.prefs.Side, a.prefs.ChangesTree = sideChanges, true
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseHead)
	ch.base, ch.scope, ch.stamp, ch.fetched = git.BaseHead, scope, gitStamp(g)+"|0", time.Now()
	ch.res = &proto.GitChanges{Root: g.Root, Files: unionFiles(ix.Staged, ix.Unstaged)}
	ch.indexes = map[string]*proto.GitIndex{scope: ix}

	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	gen, ok := tt.Find("gen")
	src, _ := tt.Find("src")
	file, _ := tt.Find("new.txt")
	if !ok || !tt.HasText("src") || !(src.Y < gen.Y && gen.Y < file.Y && src.X < gen.X && gen.X < file.X) {
		t.Fatalf("no tree of src, gen and new.txt: src %v, gen %v, new.txt %v; texts %q", src, gen, file, tt.Texts())
	}
	shot(t, tt, "git-tree")
	// The next operation, as the one before it comes back.
	next := func() proto.GitOpParams {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			tt.Frame()
			select {
			case p := <-ops:
				return p
			case <-time.After(10 * time.Millisecond):
			}
		}
		t.Fatal("no git.op came")
		return proto.GitOpParams{}
	}

	if err := tt.RightClick("new.txt"); err != nil {
		t.Fatal(err)
	}
	if items := tt.Menu(); !slices.Contains(items, "Open in Editor") || !slices.Contains(items, "Open in Pane") {
		t.Errorf("a changed file's menu opens it not: %q", items)
	}
	if err := tt.ChooseMenuItem("Add to .gitignore"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	if p := next(); p.Op != "tracked" || !slices.Equal(p.Paths, []string{"src/gen/new.txt"}) {
		t.Errorf("first %+v", p)
	}
	if p := next(); p.Op != "ignore" || !slices.Equal(p.Paths, []string{"src/gen/new.txt"}) || p.Untrack {
		t.Errorf("then %+v", p)
	}
	for range 3 {
		tt.Frame()
	}

	if err := tt.RightClick("gen"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Add Folder to .gitignore"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	if p := next(); p.Op != "tracked" || !slices.Equal(p.Paths, []string{"src/gen"}) {
		t.Errorf("folder first %+v", p)
	}
	for range 3 {
		tt.Frame()
	}
	if a.confirm == nil || a.confirm.title != "Stop tracking gen?" || !tt.HasText("Ignore and Stop Tracking") {
		t.Fatalf("no question about tracked files: %+v", a.confirm)
	}
	a.confirm.run()
	if p := next(); p.Op != "ignore" || !slices.Equal(p.Paths, []string{"src/gen/"}) || !p.Untrack {
		t.Errorf("folder then %+v", p)
	}

	// A folder's click folds what it holds.
	a.confirm = nil
	for range 3 {
		tt.Frame()
	}
	if err := tt.Click("gen"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("new.txt") {
		t.Error("folded, gen shows new.txt")
	}
}

// Uncommitted lists what's staged apart from what isn't, each file with
// its buttons, and commits what's staged with the box's message.
func TestUncommittedStaging(t *testing.T) {
	a := testApp(t)
	g, ix := gitDemo(a)
	ops, _ := gitCalls(t, a, ix)
	s := &a.st.Sessions[0]
	a.prefs.Side = sideChanges
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseHead)
	ch.base, ch.scope, ch.stamp, ch.fetched = git.BaseHead, scope, gitStamp(g)+"|0", time.Now()
	ch.res = &proto.GitChanges{Root: g.Root, Files: unionFiles(ix.Staged, ix.Unstaged)}
	ch.indexes = map[string]*proto.GitIndex{scope: ix}
	ch.collapsed = map[string]bool{}
	for _, sec := range []string{baseStaged, baseUnstaged} {
		for _, f := range ch.res.Files {
			ch.collapsed[fileKey(scope, sec, f.Path)] = true
		}
	}

	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	for _, want := range []string{"Staged Changes", "a.go", "b.go", "new.txt", "Commit 1 File", "main"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	shot(t, tt, "git-uncommitted")
	staged, _ := tt.Find("Staged Changes")
	b, _ := tt.Find("b.go")
	if b.Y < staged.Y {
		t.Error("b.go, not staged, lists above the staged section")
	}

	// Each section unfolds and folds its own files, all at once: the
	// staged one's first.
	unfolded := func(sec string) (n int) {
		for _, f := range ch.res.Files {
			if !a.fileCollapsed(fileKey(scope, sec, f.Path)) {
				n++
			}
		}
		return n
	}
	if err := tt.Click("Expand All Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if unfolded(baseStaged) != len(ix.Staged) || unfolded(baseUnstaged) != 0 || !tt.HasText("Collapse All Files") {
		t.Errorf("Expand All Files unfolded %d staged, %d not", unfolded(baseStaged), unfolded(baseUnstaged))
	}
	if err := tt.Click("Collapse All Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if unfolded(baseStaged) != 0 || tt.HasText("Collapse All Files") {
		t.Errorf("Collapse All Files left %d staged unfolded", unfolded(baseStaged))
	}

	// A file's own button stages it alone.
	if err := tt.Click("Stage"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if p := nextOp(t, ops); p.Op != "stage" || !slices.Equal(p.Paths, []string{"b.go"}) {
		t.Errorf("Stage sent %+v", p)
	}
	if !a.fileCollapsed(fileKey(scope, baseStaged, "b.go")) {
		t.Error("b.go, folded, unfolds as it's staged")
	}
	a.side.git.op = nil

	// Discarding asks first, by what it does to the file: an untracked one
	// goes.
	tools := []string{}
	for _, l := range tt.Texts() {
		if l == "Discard" {
			tools = append(tools, l)
		}
	}
	if len(tools) != 2 {
		t.Errorf("%d Discard buttons, want one for each file not staged", len(tools))
	}
	a.discard(s.Panes[0].ID, g.Root, ix.Unstaged[1:], "Delete new.txt?", "", "Delete File")
	tt.Frame()
	if !tt.HasText("Delete new.txt?") {
		t.Fatalf("no confirmation; texts %q", tt.Texts())
	}
	a.confirm.run()
	if p := nextOp(t, ops); p.Op != "discard" || len(p.Paths) != 0 || !slices.Equal(p.Untracked, []string{"new.txt"}) {
		t.Errorf("Discard of new.txt sent %+v", p)
	}
	a.confirm = nil
	a.side.git.op = nil

	// No message, no commit.
	tt.Frame()
	if err := tt.Click("Commit 1 File"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	select {
	case p := <-ops:
		t.Errorf("Commit with no message sent %+v", p)
	default:
	}
	if err := tt.Click("Commit message"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("Fix the login")
	tt.Frame()
	if err := tt.Click("Commit 1 File"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if p := nextOp(t, ops); p.Op != "commit" || p.Message != "Fix the login" || p.Amend {
		t.Errorf("Commit sent %+v", p)
	}
	// The commit under way shows in the bar, once it takes a moment, and
	// moves nothing below it: one that runs still, as this kerod's ended.
	tt.Frame()
	before, _ := tt.Find("Staged Changes")
	a.side.git.op = &gitRun{pane: s.Panes[0].ID, root: g.Root, op: "commit", label: "Commit", started: time.Now().Add(-time.Second)}
	tt.Frame()
	if r, ok := tt.Find("Commit"); !ok || r.Y > before.Y {
		t.Errorf("the bar shows no commit under way at %+v; texts %q", r, tt.Texts())
	}
	if after, _ := tt.Find("Staged Changes"); after.Y != before.Y {
		t.Errorf("the list moved from %v to %v as the commit ran", before.Y, after.Y)
	}
	a.side.git.op = nil
}

// A merge's conflicts resolve by a side's version, named by its branch, or
// by the agent there, which gets them as a prompt; a branch merges and
// rebases from its commit's menu.
func TestConflicts(t *testing.T) {
	a := testApp(t)
	ops, sent := make(chan proto.GitOpParams, 8), make(chan string, 8)
	var ix *proto.GitIndex
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.index":
			return ix, nil
		case "git.op":
			var p proto.GitOpParams
			json.Unmarshal(raw, &p)
			ops <- p
			return proto.GitOpResult{}, nil
		case "send":
			var p proto.SendParams
			json.Unmarshal(raw, &p)
			sent <- p.ID + ":" + p.Data
			return true, nil
		}
		return nil, errors.New("no git in a test")
	})
	a.local().cl = cl
	g, _ := gitDemo(a)
	g.State = "merge"
	s := &a.st.Sessions[0]
	s.Panes[1].Agent, s.Panes[1].Status = "claude", proto.Idle
	// A file's pane, whose editor takes the keyboard where the tester has
	// no terminal to.
	s.Panes[0].File = "/tmp/a.txt"
	f := a.editorOf(&s.Panes[0])
	f.read(f.stamp, false, false, nil, []byte("a\n"), nil)
	ix = &proto.GitIndex{Root: g.Root, Ours: "main", Theirs: "feature",
		Unstaged: []proto.GitFile{{Path: "a.go", Status: "conflict"}}}
	a.prefs.Side = sideChanges
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseHead)
	ch.base, ch.scope, ch.stamp, ch.fetched = git.BaseHead, scope, gitStamp(g)+"|0", time.Now()
	ch.res = &proto.GitChanges{Root: g.Root, Files: ix.Unstaged}
	ch.indexes = map[string]*proto.GitIndex{scope: ix}
	ch.collapsed = map[string]bool{fileKey(scope, baseUnstaged, "a.go"): true}

	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	if err := tt.Click("Resolve Conflict"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	for _, want := range []string{"Use main's Version", "Use feature's Version", "Resolve with Agent"} {
		if !slices.ContainsFunc(tt.Menu(), func(s string) bool { return strings.Contains(s, want) }) {
			t.Errorf("the conflict's menu has no %q: %q", want, tt.Menu())
		}
	}
	if err := tt.ChooseMenuItem("Use feature's Version"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.confirm == nil {
		t.Fatal("a side's version took the file without asking")
	}
	tt.Frame()
	if err := tt.Click("Use feature's Version"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if p := nextOp(t, ops); p.Op != "resolve" || p.Path != "a.go" || p.Mode != git.SideTheirs {
		t.Errorf("Use feature's Version sent %+v", p)
	}
	// Answered, the dialog leaves the keyboard in the Explorer it was
	// asked from, not in the terminal.
	if a.confirm != nil || !a.side.focused {
		t.Errorf("after the answer: dialog %v, Explorer focused %v", a.confirm != nil, a.side.focused)
	}
	a.side.git.op = nil

	// The agent gets the conflicts in its pane, then Enter.
	tt.Frame()
	if err := tt.Click("Resolve with Agent"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	for _, want := range []string{"a.go", "\r"} {
		select {
		case got := <-sent:
			if !strings.HasPrefix(got, s.Panes[1].ID+":") || !strings.Contains(got, want) {
				t.Errorf("sent %q, want %q to the agent's pane", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("nothing sent for %q", want)
		}
	}

	// Merging another branch into this one, from its commit.
	g.State = ""
	a.mergeInto(&s.Panes[0], "feature", "feature", true)
	if p := nextOp(t, ops); p.Op != "merge" || p.Rev != "feature" || p.Mode != "no-ff" {
		t.Errorf("merge sent %+v", p)
	}
	a.side.git.op = nil
	a.rebaseOnto(&s.Panes[0], "feature", "feature")
	if a.confirm == nil {
		t.Fatal("a rebase ran without asking")
	}
	// Over the changes not committed, which it stashes and puts back.
	if a.confirm.action != "Stash and Rebase" {
		t.Errorf("the rebase over changes asks %q", a.confirm.action)
	}
	a.confirm.run()
	if p := nextOp(t, ops); p.Op != "rebase" || p.Rev != "feature" || !p.Autostash {
		t.Errorf("rebase sent %+v", p)
	}
}

// An editor pane of a file in conflict shows it in the merge editor: its
// versions are the index's stages, a checkbox takes one's lines, and Save
// and Mark Resolved writes the file, then stages it. Marked resolved, it
// is the editor pane it was.
func TestMergePane(t *testing.T) {
	a := testApp(t)
	const text = "a\n<<<<<<< HEAD\nmain\n=======\nfeature\n>>>>>>> 1a2b3c4 (Use feature)\nz\n"
	ops, writes := make(chan proto.GitOpParams, 8), make(chan string, 8)
	conflict := true
	var files []proto.GitParams
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "fs.stat":
			return proto.FSStat{Path: "/tmp/demo/a.go", Exists: true, Size: int64(len(text))}, nil
		case "fs.read":
			return proto.FSRead{Data: []byte(text)}, nil
		case "fs.write":
			var p proto.FSWriteParams
			json.Unmarshal(raw, &p)
			writes <- string(p.Data)
			return proto.FSWrite{Hash: git.Hash(p.Data)}, nil
		case "git.index":
			ix := proto.GitIndex{Root: "/tmp/demo", Ours: "main", Theirs: "feature"}
			if conflict {
				ix.Unstaged = []proto.GitFile{{Path: "a.go", Status: "conflict"}}
			}
			return ix, nil
		case "git.file":
			var p proto.GitParams
			json.Unmarshal(raw, &p)
			files = append(files, p)
			return proto.GitText{Old: "a\nmain\nz\n", New: "a\nfeature\nz\n"}, nil
		case "git.op":
			var p proto.GitOpParams
			json.Unmarshal(raw, &p)
			ops <- p
			return proto.GitOpResult{}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
	g, _ := gitDemo(a)
	g.State = "rebase"
	s := &a.st.Sessions[0]
	s.Panes[0].File = "/tmp/demo/a.go"
	s.Panes[0].Git = nil

	tt := ui.NewTester(a.view, 1280, 720)
	shows := func(text string) bool {
		t.Helper()
		for range 300 {
			tt.Frame()
			if tt.HasText(text) {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}
	if !shows("Use feature's lines") {
		t.Fatalf("no merge editor; texts %q", tt.Texts())
	}
	if len(files) == 0 || files[0].Pane != s.Panes[1].ID || files[0].Path != "a.go" || files[0].From != git.SideOursStage || files[0].To != git.SideTheirsStage {
		t.Errorf("the versions were read by %+v", files)
	}
	shot(t, tt, "git-merge")
	for _, want := range []string{"main", "feature", "1 conflict left"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Use feature's lines"); err != nil {
		t.Fatal(err)
	}
	if !shows("All conflicts resolved") {
		t.Fatalf("not resolved; texts %q", tt.Texts())
	}
	if err := tt.Click("Save and Mark Resolved"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	select {
	case got := <-writes:
		if got != "a\nfeature\nz\n" {
			t.Errorf("saved %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing saved")
	}
	// The save's answer stages the file, in a frame.
	var staged *proto.GitOpParams
	for i := 0; staged == nil && i < 300; i++ {
		tt.Frame()
		select {
		case p := <-ops:
			staged = &p
		case <-time.After(10 * time.Millisecond):
		}
	}
	if staged == nil || staged.Op != "stage" || !slices.Equal(staged.Paths, []string{"a.go"}) {
		t.Fatalf("Mark Resolved sent %+v", staged)
	}

	// Staged, the file is in conflict no more.
	conflict = false
	g.Staged++
	for i := 0; i < 300 && tt.HasText("Use feature's lines"); i++ {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	if tt.HasText("Use feature's lines") || !tt.HasText("Wrap Lines") {
		t.Errorf("the merge editor stays once resolved; texts %q", tt.Texts())
	}
}

// The interactive rebase lists the branch's commits, the newest on top:
// a key or a row's menu sets a commit's action, Reword opens a panel for
// its new message, a drag moves a commit, and Rebase sends the commits,
// the oldest first, as they are.
func TestInteractiveRebase(t *testing.T) {
	a := testApp(t)
	ops := make(chan proto.GitOpParams, 8)
	commits := []proto.GitCommit{
		{Hash: "aaaaaaa1", Author: "Ann", Subject: "one", Time: 1700000000},
		{Hash: "bbbbbbb2", Author: "Ann", Subject: "two", Time: 1700000100},
		{Hash: "ccccccc3", Author: "Bob", Subject: "three", Time: 1700000200},
	}
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.rebasePlan":
			return proto.GitRebasePlan{Branch: "feature", Onto: proto.GitCommit{Hash: "0000000f", Subject: "base"}, Commits: commits}, nil
		case "git.show":
			var p proto.GitShowParams
			json.Unmarshal(raw, &p)
			for _, c := range commits {
				if c.Hash == p.Rev {
					return proto.GitCommitInfo{GitCommit: c, Message: c.Subject + "\n\nThe body.\n",
						Files: []proto.GitFile{{Path: c.Subject + ".txt", Status: "added", Added: 1}}}, nil
				}
			}
		case "git.op":
			var p proto.GitOpParams
			json.Unmarshal(raw, &p)
			ops <- p
			return proto.GitOpResult{}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
	gitDemo(a)
	s := &a.st.Sessions[0]
	a.openRebase(&s.Panes[0], "main")

	tt := ui.NewTester(a.view, 1280, 800)
	shows := func(text string) bool {
		t.Helper()
		for range 300 {
			tt.Frame()
			if tt.HasText(text) {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}
	if !shows("three") || !tt.HasText("Rebase feature onto 0000000f base") {
		t.Fatalf("no commits; texts %q", tt.Texts())
	}
	one, _ := tt.Find("one")
	three, _ := tt.Find("three")
	if three.Y > one.Y {
		t.Error("the oldest commit is on top")
	}
	shows("three.txt") // the newest's, chosen first
	shot(t, tt, "git-rebase")

	// A key sets the chosen commit's action: Reword opens its panel, with
	// its message.
	if err := tt.Click("two"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyR)
	if !shows("Description") {
		t.Fatalf("no Reword panel; texts %q", tt.Texts())
	}
	rv := a.rebase
	if rv.subject != "two" || rv.body != "The body." {
		t.Errorf("the panel has %q, %q", rv.subject, rv.body)
	}
	rv.subject = "two, reworded"
	if err := tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Description") || !tt.HasText("two, reworded") {
		t.Errorf("the panel stays, or the row has its old subject; texts %q", tt.Texts())
	}

	// The newest dragged below two, then fixed up into one, below it.
	if err := tt.Click("three"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	r3, _ := tt.Find("three")
	x, y := r3.X+r3.W/2, r3.Y+r3.H/2
	tt.Press(x, y)
	for dy := float32(4); dy <= rebaseRowH+4; dy += 4 {
		tt.Move(x, y+dy)
		tt.Frame()
	}
	tt.Release(x, y+rebaseRowH+4)
	tt.Frame()
	if after, _ := tt.Find("three"); after.Y <= r3.Y {
		t.Fatalf("the drag left three at %v, from %v", after.Y, r3.Y)
	}
	tt.Key(0, ui.KeyF)
	tt.Frame()

	// One dropped, three has nothing kept below it to fix up into: no
	// Rebase, and why.
	if err := tt.Click("one"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyD)
	tt.Frame()
	if !tt.HasText("Fixup needs a kept commit below it") || slices.Contains(tt.Texts(), "Rebase") {
		t.Errorf("a fixup onto a dropped commit can rebase; texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyP)
	tt.Frame()

	if err := tt.Click("Rebase"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	p := nextOp(t, ops)
	want := []proto.GitRebaseStep{
		{Action: git.RebasePick, Hash: "aaaaaaa1"},
		{Action: git.RebaseFixup, Hash: "ccccccc3"},
		{Action: git.RebaseReword, Hash: "bbbbbbb2", Message: "two, reworded\n\nThe body.\n"},
	}
	if p.Op != "rebaseInteractive" || p.Rev != "main" || !p.Autostash || !slices.Equal(p.Steps, want) {
		t.Errorf("Rebase sent %+v, want steps %+v", p, want)
	}
	tt.Frame()
	if a.rebase != nil {
		t.Error("the rebase's window stays")
	}
}

// While an agent works in the worktree, an operation that changes its
// files asks first, and one that waits runs once the agent is done.
func TestGitGuard(t *testing.T) {
	a := testApp(t)
	gitDemo(a)
	s := &a.st.Sessions[0]
	s.Panes[1].Agent, s.Panes[1].Status = "claude", proto.Working
	ran := 0
	a.guarded("a", "/tmp/demo", "Pull", func() { ran++ })
	if ran != 0 || a.confirm == nil {
		t.Fatal("Pull ran over the agent's work without asking")
	}
	a.confirm.run()
	a.confirm = nil
	if ran != 0 || len(a.side.git.queued) != 1 {
		t.Fatalf("When It's Done ran it at once, or queued %d", len(a.side.git.queued))
	}
	a.drainGit()
	if len(a.side.git.queued) != 1 {
		t.Error("the queue ran while the agent works")
	}
	s.Panes[1].Status = proto.Idle
	a.drainGit()
	for _, fn := range a.later {
		fn()
	}
	a.later = nil
	if ran != 1 || len(a.side.git.queued) != 0 {
		t.Errorf("once the agent is done, it ran %d times, %d wait", ran, len(a.side.git.queued))
	}

	// Elsewhere, no agent: no question.
	s.Panes[1].Git = &proto.GitInfo{Root: "/tmp/other"}
	s.Panes[1].Status = proto.Working
	a.guarded("a", "/tmp/demo", "Pull", func() { ran++ })
	if ran != 2 {
		t.Error("an agent in another worktree held the operation back")
	}
}

// History draws the worktree's commits as a graph, under the changes not
// committed, and opens a commit below its row.
func TestHistoryGraph(t *testing.T) {
	a := testApp(t)
	ops, shows := gitCalls(t, a, nil)
	g, _ := gitDemo(a)
	a.prefs.Side = sideHistory
	hs := &a.side.git.hist
	hs.key = g.Root + "|0|0||"
	hs.stamp = gitStamp(g) + "|0"
	hs.stashAt = hs.stamp
	hs.remotes = []string{"origin"}
	now := time.Now().Unix()
	hs.commits = []proto.GitCommit{
		{Hash: "c3", Parents: []string{"c2", "s1"}, Author: "Ann", Time: now, Refs: "HEAD -> main, origin/main", Subject: "Merge branch 'side'"},
		{Hash: "s1", Parents: []string{"c1"}, Author: "Bo", Time: now - 3600, Refs: "origin/side", Subject: "Side work"},
		{Hash: "c2", Parents: []string{"c1"}, Author: "Ann", Time: now - 7200, Subject: "Main work"},
		{Hash: "c1", Author: "Ann", Time: now - 86400, Subject: "First commit"},
	}
	for _, cm := range hs.commits {
		hs.graph = append(hs.graph, hs.layout.Add(gitgraph.Commit{Hash: cm.Hash, Parents: cm.Parents}))
	}
	if hs.graph[1].Width < 2 {
		t.Errorf("the side branch shares its lane: %+v", hs.graph[1])
	}

	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	for _, want := range []string{"Uncommitted", "Merge branch 'side'", "Side work", "Main work", "First commit", "origin/main"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	shot(t, tt, "git-history")
	un, _ := tt.Find("Uncommitted")
	merge, _ := tt.Find("Merge branch 'side'")
	if un.Y > merge.Y {
		t.Error("the changes not committed list below the commits")
	}

	// A branch and its remote's at the same commit show as one chip, the
	// branch's name.
	if _, ok := tt.Find("main"); !ok {
		t.Errorf("main doesn't show; texts %q", tt.Texts())
	}
	if r, ok := tt.Find("origin/main"); ok && r.W > 0 {
		t.Errorf("origin/main shows apart from main at %v", r)
	}
	if err := tt.RightClick("Merge branch 'side'"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if menu := tt.Menu(); !slices.ContainsFunc(menu, func(s string) bool { return strings.Contains(s, "Copy Branch Name") }) {
		t.Errorf("the merge's menu has no Copy Branch Name: %q", menu)
	}
	if err := tt.ChooseMenuItem("Copy Branch Name", "origin/main"); err != nil {
		t.Error(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != "origin/main" {
		t.Errorf("Copy Branch Name copied %q", got)
	}
	tt.CloseMenu()

	// Checking out a commit a branch is at checks the branch out: a
	// remote's as its local branch, which is to be at that commit.
	if err := tt.RightClick("Side work"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("Check Out origin/side"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if a.confirm == nil || a.confirm.altRun == nil {
		t.Fatal("no question of the changes not committed")
	}
	a.confirm.altRun()
	a.confirm = nil
	if p := nextOp(t, ops); p.Op != "switch" || p.Name != "origin/side" || !p.Remote || p.Rev != "s1" {
		t.Errorf("Check Out origin/side sent %+v", p)
	}
	a.side.git.op = nil
	tt.CloseMenu()
	tt.Frame()

	if err := tt.Click("Main work"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	select {
	case rev := <-shows:
		if rev != "c2" {
			t.Errorf("opening Main work showed %q", rev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("opening a commit asked for none")
	}
	if hs.opened["c2"] == nil {
		t.Error("the commit isn't open")
	}

	// The scope's choice lands after the frame that drew it: it must
	// still change what the list is of.
	if err := tt.Click("Branch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !hs.branchOnly() || !strings.HasPrefix(hs.key, g.Root+"|1|") {
		t.Errorf("Branch left the scope %q, the key %q", hs.scope, hs.key)
	}
}

// An open commit's subject that its row cuts wraps, all of it shown.
func TestHistoryOpenSubjectWraps(t *testing.T) {
	a := testApp(t)
	gitCalls(t, a, nil)
	g, _ := gitDemo(a)
	a.prefs.Side = sideHistory
	hs := &a.side.git.hist
	hs.key = g.Root + "|0|0||"
	hs.stamp = gitStamp(g) + "|0"
	hs.stashAt = hs.stamp
	subject := "Keep the focus in the Explorer as git actions and their confirmations close, not in the terminal"
	hs.commits = []proto.GitCommit{{Hash: "c1", Author: "Ann", Time: time.Now().Unix(), Subject: subject}}
	hs.graph = append(hs.graph, hs.layout.Add(gitgraph.Commit{Hash: "c1"}))

	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	closed, ok := tt.Find(subject)
	if !ok {
		t.Fatalf("no subject; texts %q", tt.Texts())
	}
	if err := tt.Click(subject); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	open, _ := tt.Find(subject)
	if hs.opened["c1"] == nil || open.H < 2*closed.H {
		t.Errorf("the open subject is %v high, %v closed", open.H, closed.H)
	}
}

// A folder of the tree stages, unstages and discards the files in it
// alone, a renamed file's old path with it.
func TestChangesTreeFolderTools(t *testing.T) {
	a := testApp(t)
	g, ix := gitDemo(a)
	ix.Staged = []proto.GitFile{{Path: "lib/r.go", OldPath: "old/r.go", Status: "renamed"}, {Path: "top.go", Status: "modified"}}
	ix.Unstaged = []proto.GitFile{{Path: "src/b.go", Status: "modified", Added: 3}, {Path: "src/gen/new.txt", Status: "untracked", Added: 1},
		{Path: "src/gen/c.go", Status: "conflict"}, {Path: "other.go", Status: "modified"}}
	ops, _ := gitCalls(t, a, ix)
	s := &a.st.Sessions[0]
	a.prefs.Side, a.prefs.ChangesTree = sideChanges, true
	ch := &a.side.changes
	scope := changesScope(&s.Panes[0], git.BaseHead)
	ch.base, ch.scope, ch.stamp, ch.fetched = git.BaseHead, scope, gitStamp(g)+"|0", time.Now()
	ch.res = &proto.GitChanges{Root: g.Root, Files: unionFiles(ix.Staged, ix.Unstaged)}
	ch.indexes = map[string]*proto.GitIndex{scope: ix}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	shot(t, tt, "git-tree-folder-tools")
	// A folder's buttons are in line with its section title's.
	at := func(button, row string) {
		t.Helper()
		b, ok := tt.Find(button)
		r, ok2 := tt.Find(row)
		if !ok || !ok2 {
			t.Fatalf("no %q or %q; texts %q", button, row, tt.Texts())
		}
		tt.ClickAt(b.X+b.W/2, r.Y+r.H/2)
		tt.Frame()
	}

	at("Unstage All Changes", "lib")
	if p := nextOp(t, ops); p.Op != "unstage" || !slices.Equal(p.Paths, []string{"lib/r.go", "old/r.go"}) {
		t.Errorf("lib unstaged %+v", p)
	}
	// Staging leaves the conflict to its own button, as Stage All does.
	at("Stage All Changes", "gen")
	if p := nextOp(t, ops); p.Op != "stage" || !slices.Equal(p.Paths, []string{"src/gen/new.txt"}) {
		t.Errorf("gen staged %+v", p)
	}
	at("Discard All Changes", "src")
	if a.confirm == nil || a.confirm.title != "Discard changes to src/?" {
		t.Fatalf("no question before discarding src: %+v", a.confirm)
	}
	a.confirm.run()
	if p := nextOp(t, ops); p.Op != "discard" || !slices.Equal(p.Paths, []string{"src/b.go"}) || !slices.Equal(p.Untracked, []string{"src/gen/new.txt"}) {
		t.Errorf("src discarded %+v", p)
	}
}
