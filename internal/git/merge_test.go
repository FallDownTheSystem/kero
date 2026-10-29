package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// conflicted is a repository whose main and feature both changed a.txt,
// and feature deleted d.txt, which main changed.
func conflicted(t *testing.T) Repo {
	t.Helper()
	r := opsRepo(t, map[string]string{"a.txt": "a\n", "d.txt": "d\n"})
	gitIn(t, r.Root, "switch", "-q", "-c", "feature")
	write(t, filepath.Join(r.Root, "a.txt"), "feature\n")
	gitIn(t, r.Root, "rm", "-q", "d.txt")
	gitIn(t, r.Root, "commit", "-q", "-am", "feature")
	gitIn(t, r.Root, "switch", "-q", "main")
	write(t, filepath.Join(r.Root, "a.txt"), "main\n")
	write(t, filepath.Join(r.Root, "d.txt"), "main d\n")
	gitIn(t, r.Root, "commit", "-q", "-am", "main")
	return r
}

func TestMergeResolve(t *testing.T) {
	ctx := context.Background()
	r := conflicted(t)
	if _, err := r.Merge(ctx, "--abort", false); err == nil {
		t.Error("Merge took an option as a revision")
	}
	// A change not committed, of a file the merge changes too, is stashed
	// for it and back as its commit ends it.
	gitIn(t, r.Root, "switch", "-q", "feature")
	write(t, filepath.Join(r.Root, "e.txt"), "e\n")
	gitIn(t, r.Root, "add", "e.txt")
	gitIn(t, r.Root, "commit", "-q", "-m", "e")
	gitIn(t, r.Root, "switch", "-q", "main")
	write(t, filepath.Join(r.Root, "e.txt"), "e\n")
	gitIn(t, r.Root, "add", "e.txt")
	gitIn(t, r.Root, "commit", "-q", "-m", "e on main")
	write(t, filepath.Join(r.Root, "e.txt"), "e changed\n")
	if _, err := r.Merge(ctx, "feature", false); err == nil {
		t.Fatal("the merge had no conflict")
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "e.txt")); string(b) != "e\n" {
		t.Errorf("e.txt during the merge %q, not stashed", b)
	}
	if ours, theirs := r.ConflictSides(ctx); ours != "main" || theirs != "feature" {
		t.Errorf("merge's sides %q, %q", ours, theirs)
	}
	if err := r.ResolveSide(ctx, "a.txt", SideTheirs); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(b) != "feature\n" {
		t.Errorf("a.txt resolved as %q", b)
	}
	// Theirs deleted d.txt: taking theirs deletes it.
	if err := r.ResolveSide(ctx, "d.txt", SideTheirs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "d.txt")); !os.IsNotExist(err) {
		t.Error("d.txt, deleted by theirs, stays")
	}
	if err := r.ResolveSide(ctx, "a.txt", SideOurs); err == nil {
		t.Error("resolved a file twice")
	}
	if _, unstaged, _ := r.Index(ctx); len(unstaged) != 0 {
		t.Errorf("conflicts left: %s", paths(unstaged))
	}
	if _, err := r.Continue(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.State != "" {
		t.Errorf("state %q after the merge", st.State)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "e.txt")); string(b) != "e changed\n" {
		t.Errorf("e.txt after the merge %q", b)
	}
	if list, _ := r.Stashes(ctx); len(list) != 0 {
		t.Errorf("the merge left stashes %+v", list)
	}
}

func TestRebaseSides(t *testing.T) {
	ctx := context.Background()
	r := conflicted(t)
	gitIn(t, r.Root, "switch", "-q", "feature")
	// Changes not committed are stashed for the rebase, and back once it
	// is aborted.
	write(t, filepath.Join(r.Root, "e.txt"), "e\n")
	gitIn(t, r.Root, "add", "e.txt")
	gitIn(t, r.Root, "commit", "-q", "-m", "e")
	write(t, filepath.Join(r.Root, "e.txt"), "e changed\n")
	if _, err := r.Rebase(ctx, "main", false); err == nil {
		t.Fatal("rebased over changes not committed")
	}
	if _, err := r.Rebase(ctx, "main", true); err == nil {
		t.Fatal("the rebase had no conflict")
	}
	// In a rebase, HEAD is the branch rebased onto.
	if ours, theirs := r.ConflictSides(ctx); ours != "main" || theirs != "feature" {
		t.Errorf("rebase's sides %q, %q", ours, theirs)
	}
	// The merge editor's versions: the index's stages of the conflict.
	if tx, err := r.Texts(ctx, SideOursStage, SideTheirsStage, "a.txt", ""); err != nil || tx.Old != "main\n" || tx.New != "feature\n" {
		t.Errorf("a.txt's stages %q, %q (%v)", tx.Old, tx.New, err)
	}
	if err := r.ResolveSide(ctx, "a.txt", SideOurs); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(b) != "main\n" {
		t.Errorf("a.txt resolved as %q", b)
	}
	if _, err := r.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(b) != "feature\n" {
		t.Errorf("a.txt after the abort %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "e.txt")); string(b) != "e changed\n" {
		t.Errorf("the change stashed for the rebase is %q after it", b)
	}
	if list, _ := r.Stashes(ctx); len(list) != 0 {
		t.Errorf("the rebase left stashes %+v", list)
	}
}

// A rebase whose commits don't conflict takes the changes not committed
// along.
func TestRebaseAutostash(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	gitIn(t, r.Root, "switch", "-q", "-c", "feature")
	write(t, filepath.Join(r.Root, "f.txt"), "f\n")
	gitIn(t, r.Root, "add", "f.txt")
	gitIn(t, r.Root, "commit", "-q", "-m", "f")
	gitIn(t, r.Root, "switch", "-q", "main")
	write(t, filepath.Join(r.Root, "m.txt"), "m\n")
	gitIn(t, r.Root, "add", "m.txt")
	gitIn(t, r.Root, "commit", "-q", "-m", "m")
	gitIn(t, r.Root, "switch", "-q", "feature")
	write(t, filepath.Join(r.Root, "a.txt"), "a changed\n")
	if _, err := r.Rebase(ctx, "main", true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(b) != "a changed\n" {
		t.Errorf("a.txt after the rebase %q", b)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "m.txt")); err != nil {
		t.Error("the rebase isn't onto main")
	}
}

// A remote's branch checked out is its local branch: brought up to the
// remote's when behind it, as it is when ahead, and refused when each has
// commits of its own, rather than reset.
func TestSwitchAt(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	rev := func(ref string) string { return strings.TrimSpace(gitIn(t, r.Root, "rev-parse", ref)) }
	gitIn(t, r.Root, "remote", "add", "origin", t.TempDir())
	first := rev("HEAD")
	write(t, filepath.Join(r.Root, "a.txt"), "b\n")
	gitIn(t, r.Root, "commit", "-q", "-am", "b")
	second := rev("HEAD")

	// Behind: x comes up to origin/x, following it.
	gitIn(t, r.Root, "update-ref", "refs/remotes/origin/x", second)
	gitIn(t, r.Root, "branch", "--no-track", "x", first)
	if _, err := r.Switch(ctx, "origin/x", true, second); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.Branch != "x" || rev("x") != second || st.Upstream != "origin/x" {
		t.Errorf("behind: on %q at %s, following %q", st.Branch, rev("x"), st.Upstream)
	}

	// Ahead: x as it is.
	gitIn(t, r.Root, "switch", "-q", "main")
	gitIn(t, r.Root, "update-ref", "refs/remotes/origin/x", first)
	if _, err := r.Switch(ctx, "origin/x", true, first); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.Branch != "x" || rev("x") != second {
		t.Errorf("ahead: on %q at %s", st.Branch, rev("x"))
	}

	// Each its own: refused, x kept.
	gitIn(t, r.Root, "switch", "-q", "main")
	gitIn(t, r.Root, "switch", "-q", "--detach", first)
	write(t, filepath.Join(r.Root, "a.txt"), "c\n")
	gitIn(t, r.Root, "commit", "-q", "-am", "c")
	gitIn(t, r.Root, "update-ref", "refs/remotes/origin/x", rev("HEAD"))
	gitIn(t, r.Root, "switch", "-q", "main")
	if _, err := r.Switch(ctx, "origin/x", true, rev("origin/x")); err == nil {
		t.Error("switched to x, which has commits origin/x lacks and lacks some of its")
	}
	if rev("x") != second {
		t.Error("the refused switch moved x")
	}
}
