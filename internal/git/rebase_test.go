package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// subjects are the subjects of HEAD's commits since rev, the oldest first.
func subjects(t *testing.T, r Repo, rev string) []string {
	t.Helper()
	out := strings.TrimSpace(gitIn(t, r.Root, "log", "--reverse", "--format=%s", rev+"..HEAD"))
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// rebaseRepo is a repository whose feature has the commits one to four,
// each adding its own file, on main's first.
func rebaseRepo(t *testing.T) (Repo, []Commit) {
	t.Helper()
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	gitIn(t, r.Root, "switch", "-q", "-c", "feature")
	for _, n := range []string{"one", "two", "three", "four"} {
		write(t, filepath.Join(r.Root, n+".txt"), n+"\n")
		gitIn(t, r.Root, "add", n+".txt")
		gitIn(t, r.Root, "commit", "-q", "-m", n)
	}
	plan, err := r.RebasePlan(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	return r, plan
}

func TestRebaseInteractive(t *testing.T) {
	ctx := context.Background()
	r, plan := rebaseRepo(t)
	if len(plan) != 4 || plan[0].Subject != "one" || plan[3].Subject != "four" {
		t.Fatalf("plan %+v", plan)
	}
	// Four after two, three reworded with a quote in it, one dropped, two
	// fixed up into four.
	steps := []RebaseStep{
		{Action: RebaseDrop, Hash: plan[0].Hash},
		{Action: RebaseReword, Hash: plan[2].Hash, Message: "three, it's new\n\nWith a body.\n"},
		{Action: RebasePick, Hash: plan[3].Hash},
		{Action: RebaseFixup, Hash: plan[1].Hash},
	}
	if _, err := r.RebaseInteractive(ctx, "main", steps, "feature-backup", false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subjects(t, r, "main"), ","); got != "three, it's new,four" {
		t.Errorf("subjects %q", got)
	}
	if body := strings.TrimSpace(gitIn(t, r.Root, "log", "-1", "--format=%b", "HEAD~1")); body != "With a body." {
		t.Errorf("the reworded body %q", body)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "one.txt")); !os.IsNotExist(err) {
		t.Error("the dropped commit's file stays")
	}
	if _, err := os.Stat(filepath.Join(r.Root, "two.txt")); err != nil {
		t.Error("the fixed up commit's file is gone")
	}
	// The backup is where feature was.
	if got := strings.TrimSpace(gitIn(t, r.Root, "rev-parse", "feature-backup")); got != plan[3].Hash {
		t.Errorf("backup at %s, want %s", got, plan[3].Hash)
	}
	if _, err := os.Stat(filepath.Join(r.GitDir, "kero-rebase")); !os.IsNotExist(err) {
		t.Error("the list's files stay after the rebase")
	}
}

// A commit to edit stops the rebase there, and one reworded after it gets
// its message as the rebase goes on.
func TestRebaseInteractiveStops(t *testing.T) {
	ctx := context.Background()
	r, plan := rebaseRepo(t)
	steps := []RebaseStep{
		{Action: RebasePick, Hash: plan[0].Hash},
		{Action: RebaseEdit, Hash: plan[1].Hash},
		{Action: RebaseReword, Hash: plan[2].Hash, Message: "three again"},
		{Action: RebaseSquash, Hash: plan[3].Hash},
	}
	if _, err := r.RebaseInteractive(ctx, "main", steps, "", false); err != nil {
		t.Fatal(err)
	}
	if op := r.operation(); op != "rebase" {
		t.Fatalf("no stop at the commit to edit: %q", op)
	}
	if _, err := r.Continue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subjects(t, r, "main"), ","); got != "one,two,three again" {
		t.Errorf("subjects %q", got)
	}
	// The squash keeps both messages.
	if msg := gitIn(t, r.Root, "log", "-1", "--format=%B"); !strings.Contains(msg, "three again") || !strings.Contains(msg, "four") {
		t.Errorf("squashed message %q", msg)
	}

	for _, bad := range [][]RebaseStep{
		{{Action: RebaseSquash, Hash: plan[0].Hash}},
		{{Action: "exec", Hash: plan[0].Hash}},
		{{Action: RebasePick, Hash: "HEAD; rm -rf"}},
	} {
		if _, err := r.RebaseInteractive(ctx, "main", bad, "", false); err == nil {
			t.Errorf("rebased by %+v", bad)
		}
	}
}

// A range with a merge isn't rebased as a list, which would flatten it.
func TestRebasePlanMerges(t *testing.T) {
	ctx := context.Background()
	r, _ := rebaseRepo(t)
	gitIn(t, r.Root, "switch", "-q", "-c", "side", "main")
	write(t, filepath.Join(r.Root, "s.txt"), "s\n")
	gitIn(t, r.Root, "add", "s.txt")
	gitIn(t, r.Root, "commit", "-q", "-m", "s")
	gitIn(t, r.Root, "switch", "-q", "feature")
	gitIn(t, r.Root, "merge", "-q", "--no-ff", "--no-edit", "side")
	if _, err := r.RebasePlan(ctx, "main"); err != ErrRebaseMerges {
		t.Errorf("plan over a merge: %v", err)
	}
}
