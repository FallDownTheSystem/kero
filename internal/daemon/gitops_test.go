package daemon

import (
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
)

// The Changes view stages a change of a file, commits it, and finds the
// commit in the History view, through kerod.
func TestGitOps(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	p := testPane(t)
	d := p.d
	d.log = log.New(io.Discard, "", 0)
	d.quit = make(chan struct{})
	d.panes = map[string]*pane{"p": p}
	d.git = newGitFollow(d)

	repo := realPath(t, t.TempDir())
	gitIn(t, repo, "init", "-q")
	for _, kv := range [][2]string{{"user.name", "t"}, {"user.email", "t@t"}, {"commit.gpgsign", "false"}, {"core.autocrlf", "false"}} {
		gitIn(t, repo, "config", kv[0], kv[1])
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\ntwo\n"), 0o644)
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-q", "-m", "first")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("ONE\ntwo\nthree\n"), 0o644)
	p.info.Dir = repo

	ix, err := d.gitIndex(proto.GitParams{Pane: "p"})
	if err != nil || len(ix.Staged) != 0 || len(ix.Unstaged) != 1 || ix.Root != repo {
		t.Fatalf("index %+v, %v", ix, err)
	}
	txt, err := d.gitText(proto.GitParams{Pane: "p", Base: "unstaged", Path: "a.txt"})
	if err != nil || txt.Old != "one\ntwo\n" || txt.New != "ONE\ntwo\nthree\n" || txt.Index != git.Hash([]byte("one\ntwo\n")) {
		t.Fatalf("unstaged text %+v, %v", txt, err)
	}
	if same, _ := d.gitText(proto.GitParams{Pane: "p", Base: "unstaged", Path: "a.txt", Known: txt.Index + txt.Hash}); !same.Same {
		t.Error("a known text sent again")
	}

	// The first change staged, as the view's Stage of it writes the index.
	res, err := d.gitOp(proto.GitOpParams{Pane: "p", Op: "setIndex", Path: "a.txt", Known: txt.Index, Text: "ONE\ntwo\n"})
	if err != nil || res.Error != "" {
		t.Fatalf("setIndex %+v, %v", res, err)
	}
	staged, err := d.gitText(proto.GitParams{Pane: "p", Base: "staged", Path: "a.txt"})
	if err != nil || staged.Old != "one\ntwo\n" || staged.New != "ONE\ntwo\n" {
		t.Fatalf("staged text %+v, %v", staged, err)
	}
	if res, _ := d.gitOp(proto.GitOpParams{Pane: "p", Op: "setIndex", Path: "a.txt", Known: txt.Index, Text: "x\n"}); res.Error == "" {
		t.Error("wrote over an index that changed")
	}
	if res, _ := d.gitOp(proto.GitOpParams{Pane: "p", Op: "commit"}); res.Error == "" {
		t.Error("committed with no message")
	}
	if res, _ := d.gitOp(proto.GitOpParams{Pane: "p", Op: "commit", Message: "Capitalize one"}); res.Error != "" {
		t.Fatalf("commit: %+v", res)
	}
	if res, _ := d.gitOp(proto.GitOpParams{Pane: "p", Op: "nope"}); res.Error == "" {
		t.Error("an unknown operation went through")
	}

	lg, err := d.gitLog(proto.GitLogParams{Pane: "p", Max: 1})
	if err != nil || len(lg.Commits) != 1 || lg.Commits[0].Subject != "Capitalize one" || !lg.More || len(lg.Unpushed) != 2 {
		t.Fatalf("log %+v, %v", lg, err)
	}
	if found, _ := d.gitLog(proto.GitLogParams{Pane: "p", Query: "capitalize"}); len(found.Commits) != 1 {
		t.Errorf("found %+v", found)
	}
	if found, _ := d.gitLog(proto.GitLogParams{Pane: "p", Query: lg.Commits[0].Hash[:8]}); len(found.Commits) != 1 {
		t.Errorf("found by hash %+v", found)
	}
	ci, err := d.gitShow(proto.GitShowParams{Pane: "p", Rev: lg.Commits[0].Hash})
	if err != nil || len(ci.Files) != 1 || ci.Files[0].To != ci.Hash || ci.Files[0].From != ci.Parents[0] {
		t.Fatalf("show %+v, %v", ci, err)
	}
	ft, err := d.gitText(proto.GitParams{Pane: "p", Base: "rev", Path: "a.txt", From: ci.Files[0].From, To: ci.Files[0].To})
	if err != nil || ft.Old != "one\ntwo\n" || ft.New != "ONE\ntwo\n" {
		t.Errorf("commit's file %+v, %v", ft, err)
	}
	if _, err := d.gitText(proto.GitParams{Pane: "p", Base: "rev", Path: "a.txt", From: "--output=x", To: ci.Hash}); err == nil {
		t.Error("took an option for a side")
	}

	// A revert while an agent works goes through only as the user says.
	p.mu.Lock()
	p.info.Agent = "claude"
	p.setStatus(proto.Working)
	p.mu.Unlock()
	cur, _ := d.gitText(proto.GitParams{Pane: "p", Base: "unstaged", Path: "a.txt"})
	w := proto.GitWriteParams{Pane: "p", Base: "unstaged", Path: "a.txt", Hash: cur.Hash, Text: "ONE\ntwo\n"}
	if _, err := d.gitRevert(w); err == nil {
		t.Error("reverted under a working agent")
	}
	w.Anyway = true
	if _, err := d.gitRevert(w); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(b) != "ONE\ntwo\n" {
		t.Errorf("reverted to %q", b)
	}
}

func TestGitRuns(t *testing.T) {
	ctx, progress, done := follow("op1", time.Minute)
	progress("Receiving objects: 50%")
	if got := gitProgress("op1"); got.Line != "Receiving objects: 50%" {
		t.Errorf("progress %q", got.Line)
	}
	if !gitCancel("op1") || ctx.Err() == nil {
		t.Error("not canceled")
	}
	done()
	if gitCancel("op1") || gitProgress("op1").Line != "" {
		t.Error("the run outlived its end")
	}
	if _, p, end := follow("", time.Minute); p != nil {
		t.Error("an operation without an id is followed")
	} else {
		end()
	}
}

// The History view's worktrees: each with what removing it would lose,
// read against the default branch, and removed with its branch through
// kerod; a stash of chosen files takes them alone.
func TestGitWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	p := testPane(t)
	d := p.d
	d.log = log.New(io.Discard, "", 0)
	d.quit = make(chan struct{})
	d.panes = map[string]*pane{"p": p}
	d.git = newGitFollow(d)
	repo := realPath(t, t.TempDir())
	gitIn(t, repo, "init", "-q", "-b", "main")
	for _, kv := range [][2]string{{"user.name", "t"}, {"user.email", "t@t"}, {"commit.gpgsign", "false"}, {"core.autocrlf", "false"}} {
		gitIn(t, repo, "config", kv[0], kv[1])
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644)
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-q", "-m", "first")
	p.info.Dir = repo
	wt := realPath(t, t.TempDir())
	clean, ahead := filepath.Join(wt, "clean"), filepath.Join(wt, "ahead")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "clean", clean)
	gitIn(t, repo, "worktree", "add", "-q", "-b", "ahead", ahead)
	os.WriteFile(filepath.Join(ahead, "b.txt"), []byte("b\n"), 0o644)
	gitIn(t, ahead, "add", "-A")
	gitIn(t, ahead, "commit", "-q", "-m", "ahead")
	os.WriteFile(filepath.Join(ahead, "c.txt"), []byte("c\n"), 0o644)

	res, err := d.gitWorktrees(proto.GitWorktreesParams{Pane: "p", State: true})
	if err != nil || len(res.Worktrees) != 3 || res.Default != "main" {
		t.Fatalf("worktrees %+v, %v", res, err)
	}
	by := map[string]proto.GitWorktree{}
	for _, w := range res.Worktrees {
		by[w.Branch] = w
	}
	if w := by["main"]; !w.Main || !w.State {
		t.Errorf("main %+v", w)
	}
	if w := by["clean"]; w.Main || !w.State || w.Files != 0 || w.Unpushed != 0 || !w.Merged {
		t.Errorf("clean %+v", w)
	}
	if w := by["ahead"]; w.Files != 1 || w.Unpushed != 1 || w.Merged {
		t.Errorf("ahead %+v", w)
	}
	if res, _ := d.gitOp(proto.GitOpParams{Pane: "p", Op: "worktreeRemove", Path: clean, Name: "clean"}); res.Error != "" {
		t.Fatalf("remove: %+v", res)
	}
	if res, _ := d.gitWorktrees(proto.GitWorktreesParams{Pane: "p"}); len(res.Worktrees) != 2 || res.Worktrees[1].State {
		t.Errorf("after removing, without state %+v", res)
	}

	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("A\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "n.txt"), []byte("n\n"), 0o644)
	if res, _ := d.gitOp(proto.GitOpParams{Pane: "p", Op: "stash", Paths: []string{"n.txt"}, IncludeUntracked: true, Message: "just n"}); res.Error != "" {
		t.Fatalf("stash: %+v", res)
	}
	if out, _ := exec.Command("git", "-C", repo, "status", "--porcelain").Output(); string(out) != " M a.txt\n" {
		t.Errorf("after stashing n.txt: %q", out)
	}
}
