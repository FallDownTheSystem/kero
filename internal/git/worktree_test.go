package git

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// A session's worktree starts on a new branch from its base; what
// removing it would lose counts its changes and the commits nothing else
// has, and removing it deletes its branch only once the base has it.
func TestWorktree(t *testing.T) {
	ctx := context.Background()
	r := repo(t, map[string]string{"a.txt": "a\n"})
	if r.Main() != r.Root {
		t.Fatalf("main %q, root %q", r.Main(), r.Root)
	}
	if err := CheckBranch(ctx, r.Root, "fix/login"); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"", "-x", "a..b", "a b", "x~1"} {
		if CheckBranch(ctx, r.Root, bad) == nil {
			t.Errorf("%q named a branch", bad)
		}
	}
	path := filepath.Join(r.Root, ".kero", "worktrees", "fix-login")
	if err := r.AddWorktree(ctx, path, "fix/login", "main"); err != nil {
		t.Fatal(err)
	}
	if err := r.Exclude("/.kero/"); err != nil {
		t.Fatal(err)
	}
	r.Exclude("/.kero/")
	if b, _ := os.ReadFile(filepath.Join(r.Common, "info", "exclude")); strings.Count(string(b), "/.kero/") != 1 {
		t.Errorf("exclude:\n%s", b)
	}
	if st, _ := r.Status(ctx); st.Files != 0 {
		t.Errorf("the main checkout shows %d files changed: .kero isn't ignored", st.Files)
	}
	wt, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if wt.Main() != r.Root {
		t.Errorf("the worktree's main checkout %q", wt.Main())
	}
	cos, err := r.Checkouts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := run(ctx, r.Root, nil, "rev-parse", "HEAD")
	head := strings.TrimSpace(string(out))
	if len(cos) != 2 || !strings.EqualFold(filepath.Clean(cos[0].Path), filepath.Clean(r.Root)) || !strings.EqualFold(filepath.Clean(cos[1].Path), filepath.Clean(path)) || cos[1].Head != head {
		t.Errorf("checkouts %+v, head %s", cos, head)
	}
	state := func() WorktreeState {
		t.Helper()
		st, err := wt.State(ctx, "fix/login", "main")
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	if st := state(); st != (WorktreeState{Merged: true}) {
		t.Errorf("fresh: %+v", st)
	}
	write(t, filepath.Join(path, "b.txt"), "b\n")
	if st := state(); st.Files != 1 || st.Unpushed != 0 {
		t.Errorf("a new file: %+v", st)
	}
	gitIn(t, path, "add", "-A")
	gitIn(t, path, "commit", "-q", "-m", "b")
	if st := state(); st.Files != 0 || st.Unpushed != 1 || st.Merged {
		t.Errorf("a commit: %+v", st)
	}

	// Not merged: the worktree goes, its branch stays.
	if err := RemoveWorktree(ctx, r.Root, path, "fix/login", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the worktree is still there: %v", err)
	}
	if !r.HasBranch("fix/login") {
		t.Fatal("deleted a branch with commits main doesn't have")
	}

	// Merged: the branch goes too.
	gitIn(t, r.Root, "merge", "-q", "--ff-only", "fix/login")
	if err := r.AddWorktree(ctx, path, "fix/login", ""); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(ctx, r.Root, path, "fix/login", "main"); err != nil {
		t.Fatal(err)
	}
	if r.HasBranch("fix/login") {
		t.Error("kept a merged branch")
	}
}

// A new worktree gets the ignored files .worktreeinclude lists, as
// .gitignore would match them: never a tracked one, nor one git doesn't
// ignore, nor one in node_modules or in another worktree, nor one in a
// wholly ignored folder that no pattern names a path in.
func TestIncluded(t *testing.T) {
	ctx := context.Background()
	r := repo(t, map[string]string{
		".gitignore":       ".env\n.claude/settings.local.json\nnode_modules/\nsecrets/\ncache/\nother/\n*.log\nnotes.txt\n",
		".worktreeinclude": "# local setup\n.env\n.claude/settings.local.json\n/secrets/\ncache/keep.txt\n/other/**\n*.log\n!debug.log\nplain.txt\n/.kero/**\n",
		".claude/agent.md": "tracked\n",
		"src/main.go":      "package main\n",
	})
	write(t, filepath.Join(r.Root, "tracked.log"), "tracked\n")
	gitIn(t, r.Root, "add", "-f", "tracked.log")
	gitIn(t, r.Root, "commit", "-q", "-m", "log")
	for _, f := range []string{
		".env", "src/.env", "node_modules/pkg/.env", ".claude/settings.local.json",
		"secrets/a/key", "cache/keep.txt", "cache/big.bin", "other/top.txt",
		"app.log", "debug.log", "notes.txt", "plain.txt",
	} {
		write(t, filepath.Join(r.Root, f), f+"\n")
	}
	if err := r.AddWorktree(ctx, filepath.Join(r.Root, "other", "wt"), "other", "main"); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(r.Root, "other", "wt", "x.log"), "x\n")
	if err := r.AddWorktree(ctx, filepath.Join(r.Root, ".kero", "worktrees", "w"), "w", "main"); err != nil {
		t.Fatal(err)
	}
	r.Exclude("/.kero/")
	write(t, filepath.Join(r.Root, ".kero", "worktrees", "w", ".env"), "w\n")
	write(t, filepath.Join(r.Root, ".kero", "worktrees", "stale", "a.log"), "w\n")

	got, err := r.Included(ctx, ".kero/worktrees")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want := []string{".claude/settings.local.json", ".env", "app.log", "cache/keep.txt", "other/top.txt", "secrets/a/key", "src/.env"}
	if !slices.Equal(got, want) {
		t.Errorf("included %q, want %q", got, want)
	}

	// The copy keeps paths and modes, and a file the worktree has.
	os.Chmod(filepath.Join(r.Root, "secrets", "a", "key"), 0o600)
	dst := filepath.Join(r.Root, ".kero", "worktrees", "w")
	n, err := r.CopyIncluded(ctx, dst, ".kero/worktrees")
	if err != nil || n != len(want)-1 {
		t.Fatalf("copied %d, %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, ".env")); string(b) != "w\n" {
		t.Errorf("the worktree's .env became %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "secrets", "a", "key")); string(b) != "secrets/a/key\n" {
		t.Errorf("secrets/a/key: %q", b)
	}
	if fi, err := os.Stat(filepath.Join(dst, "secrets", "a", "key")); err != nil || runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("key's mode: %v, %v", fi, err)
	}
}
