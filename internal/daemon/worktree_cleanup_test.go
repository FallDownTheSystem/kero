package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kero/internal/proto"
	"kero/internal/settings"
)

// The cleanup removes kero's worktrees its rules say are done with, but
// never one a session works in, one with changes or files of its own, a
// new one, or one kero didn't make; a branch with commits of its own
// stays; a project turned off keeps its worktrees.
func TestWorktreeCleanup(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env\nnode_modules/\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".env"), []byte("KEY=main\n"), 0o644)
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-q", "-m", "ignore")
	kero := filepath.Join(repo, ".kero", "worktrees")
	add := func(name string, age time.Duration) string {
		path := filepath.Join(kero, name)
		runGit(t, repo, "worktree", "add", "-q", "-b", name, path)
		then := time.Now().Add(-age)
		os.Chtimes(filepath.Join(repo, ".git", "worktrees", name, "logs", "HEAD"), then, then)
		return path
	}
	const day = 24 * time.Hour
	plain := add("plain", 2*day)
	committed := add("committed", 2*day)
	runGit(t, committed, "commit", "-q", "--allow-empty", "-m", "work")
	os.Chtimes(filepath.Join(repo, ".git", "worktrees", "committed", "logs", "HEAD"), time.Now().Add(-2*day), time.Now().Add(-2*day))
	old := add("old", 40*day)
	runGit(t, old, "commit", "-q", "--allow-empty", "-m", "old work")
	os.Chtimes(filepath.Join(repo, ".git", "worktrees", "old", "logs", "HEAD"), time.Now().Add(-40*day), time.Now().Add(-40*day))
	dirty := add("dirty", 40*day)
	os.WriteFile(filepath.Join(dirty, "new.txt"), []byte("x"), 0o644)
	env := add("env", 40*day)
	os.WriteFile(filepath.Join(env, ".env"), []byte("KEY=mine\n"), 0o644)
	copied := add("copied", 40*day)
	os.WriteFile(filepath.Join(copied, ".env"), []byte("KEY=main\n"), 0o644)
	os.MkdirAll(filepath.Join(copied, "node_modules", "x"), 0o755)
	os.WriteFile(filepath.Join(copied, "node_modules", "x", "i.js"), []byte("1"), 0o644)
	fresh := add("fresh", 0)
	busy := add("busy", 40*day)
	s, err := d.create(proto.CreateParams{Dir: busy})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-outside")
	runGit(t, repo, "worktree", "add", "-q", "-b", "outside", outside)
	t.Cleanup(func() { os.RemoveAll(outside) })
	os.Chtimes(filepath.Join(repo, ".git", "worktrees", filepath.Base(outside), "logs", "HEAD"), time.Now().Add(-40*day), time.Now().Add(-40*day))

	set := settings.Default()
	set.WorktreeCleanup = settings.WorktreeCleanup{InactiveDays: 30, Unchanged: true}
	if err := settings.Save(set); err != nil {
		t.Fatal(err)
	}
	if res := d.cleanWorktrees(); len(res.Removed) != 0 {
		t.Fatalf("removed while turned off: %+v", res)
	}
	exists := func(path string) bool { _, err := os.Stat(path); return err == nil }
	hasBranch := func(name string) bool {
		return exec.Command("git", "-C", repo, "rev-parse", "--verify", "-q", "refs/heads/"+name).Run() == nil
	}

	// A project turned off keeps them, though the rules are on.
	set.WorktreeCleanup.On = true
	set.ProjectCleanup = map[string]settings.WorktreeCleanup{settings.CleanupKey(repo): {}}
	settings.Save(set)
	if res := d.cleanWorktrees(); len(res.Removed) != 0 {
		t.Fatalf("removed in a project turned off: %+v", res)
	}

	set.ProjectCleanup = nil
	settings.Save(set)
	res := d.cleanWorktrees()
	reasons := map[string]string{}
	for _, w := range res.Removed {
		reasons[w.Branch] = w.Reason
	}
	want := map[string]string{"plain": "unchanged", "old": "inactive", "copied": "unchanged"}
	if len(reasons) != len(want) || len(res.Failed) != 0 {
		t.Errorf("removed %v, failed %v; want %v", reasons, res.Failed, want)
	}
	for b, r := range want {
		if reasons[b] != r {
			t.Errorf("%s: removed for %q, want %q", b, reasons[b], r)
		}
	}
	for _, p := range []string{plain, old, copied} {
		if exists(p) {
			t.Errorf("%s stays", p)
		}
	}
	for _, p := range []string{committed, dirty, env, fresh, busy, outside} {
		if !exists(p) {
			t.Errorf("%s was removed", p)
		}
	}
	if hasBranch("plain") || !hasBranch("old") {
		t.Errorf("branches: plain %v (merged: goes), old %v (its commit: stays)", hasBranch("plain"), hasBranch("old"))
	}

	// Once no session is left in one, closed removes it, but what holds
	// anything of its own.
	set.WorktreeCleanup = settings.WorktreeCleanup{On: true, Closed: true}
	settings.Save(set)
	d.removeSession(s.ID)
	res = d.cleanWorktrees()
	if !exists(busy) {
		t.Fatal("removed a worktree a session just left")
	}
	seen := d.loadCleanupSeen()
	seen[busy] = time.Now().Add(-time.Hour).UnixMilli()
	d.saveCleanupSeen(seen)
	res.Removed = append(res.Removed, d.cleanWorktrees().Removed...)
	if len(res.Removed) != 2 || exists(committed) || exists(busy) || !exists(dirty) || !exists(env) || !exists(fresh) {
		t.Errorf("closed removed %+v", res.Removed)
	}
}

// kero worktree list tells, of each worktree, the sessions in it (of the
// deepest worktree that holds a pane, not the main checkout holding them
// all), whether kero made it, what removing it would lose, and what the
// cleanup makes of it, removing nothing itself.
func TestWorktreeList(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	kero := filepath.Join(repo, ".kero", "worktrees")
	add := func(path, branch string) {
		runGit(t, repo, "worktree", "add", "-q", "-b", branch, path)
		then := time.Now().Add(-48 * time.Hour)
		os.Chtimes(filepath.Join(repo, ".git", "worktrees", filepath.Base(path), "logs", "HEAD"), then, then)
	}
	done, busy := filepath.Join(kero, "done"), filepath.Join(kero, "busy")
	add(done, "done")
	add(busy, "busy")
	os.WriteFile(filepath.Join(busy, "new.txt"), []byte("x"), 0o644)
	outside := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-mine")
	add(outside, "mine")
	t.Cleanup(func() { os.RemoveAll(outside) })
	s, err := d.create(proto.CreateParams{Dir: busy})
	if err != nil {
		t.Fatal(err)
	}
	set := settings.Default()
	set.WorktreeCleanup = settings.WorktreeCleanup{On: true, Closed: true}
	if err := settings.Save(set); err != nil {
		t.Fatal(err)
	}

	res, err := d.worktreeList(proto.WorktreeListParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cleanup != "on" || len(res.Worktrees) != 4 {
		t.Fatalf("list %+v", res)
	}
	by := map[string]proto.WorktreeInfo{}
	for _, w := range res.Worktrees {
		by[filepath.Base(w.Path)] = w
	}
	if w := by[filepath.Base(repo)]; !w.Main || len(w.Sessions) != 0 || w.Kero {
		t.Errorf("main checkout %+v", w)
	}
	if w := by["busy"]; !w.Kero || len(w.Sessions) != 1 || w.Sessions[0] != s.ID || w.Files != 1 || w.Keeps != keepInUse || w.Removes != "" {
		t.Errorf("busy %+v", w)
	}
	if w := by["done"]; !w.Kero || len(w.Sessions) != 0 || w.Removes != "closed" {
		t.Errorf("done %+v", w)
	}
	if w := by[filepath.Base(outside)]; w.Kero || w.Keeps != keepNotKero || w.Removes != "" {
		t.Errorf("outside %+v", w)
	}
	if _, err := os.Stat(done); err != nil {
		t.Errorf("listing removed %s: %v", done, err)
	}
}
