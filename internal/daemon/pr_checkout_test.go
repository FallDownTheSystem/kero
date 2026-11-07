package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kero/internal/proto"
)

// A pull request checks out in the pane's worktree on its own branch,
// following the remote's, but not over uncommitted changes; and where a
// worktree has it already, as git checks a branch out in one worktree
// alone, or a review checked it out in one, it is there instead.
func TestPRCheckout(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, repo, "init", "-q", "--bare", bare)
	runGit(t, repo, "remote", "add", "origin", bare)
	runGit(t, repo, "push", "-q", "origin", "main")
	runGit(t, repo, "switch", "-q", "-c", "fix")
	os.WriteFile(filepath.Join(repo, "fix.txt"), []byte("fix\n"), 0o644)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "fix")
	runGit(t, repo, "push", "-q", "origin", "fix")
	runGit(t, repo, "switch", "-q", "main")
	runGit(t, repo, "branch", "-q", "-D", "fix")
	branchOf := func(dir string) string {
		out, _ := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
		return strings.TrimSpace(string(out))
	}

	main, err := d.create(proto.CreateParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	pr := proto.PRCheckoutParams{Pane: main.Panes[0].ID, Remote: "origin", Number: 7, Head: "fix", Here: true}

	os.WriteFile(filepath.Join(repo, "wip.txt"), []byte("wip\n"), 0o644)
	if _, err := d.prCheckout(pr); err == nil || !strings.Contains(err.Error(), "1 changed files") || branchOf(repo) != "main" {
		t.Fatalf("checked out over changes: %v, on %s", err, branchOf(repo))
	}
	os.Remove(filepath.Join(repo, "wip.txt"))
	got, err := d.prCheckout(pr)
	if err != nil || got.Branch != "fix" || got.Worktree != "" || branchOf(repo) != "fix" {
		t.Fatalf("%+v, %v, on %s", got, err, branchOf(repo))
	}
	if out, _ := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "fix@{upstream}").Output(); strings.TrimSpace(string(out)) != "origin/fix" {
		t.Errorf("fix follows %q, not origin/fix", out)
	}

	// A new worktree of it is the main checkout, which has its branch.
	pr.Here = false
	if got, err := d.prCheckout(pr); err != nil || !samePath(got.Worktree, repo) {
		t.Fatalf("a new worktree: %+v, %v", got, err)
	}

	// A review's worktree, its branch pushing to the pull request's.
	runGit(t, repo, "switch", "-q", "main")
	fetched, err := d.prFetch(proto.PRFetchParams{Pane: main.Panes[0].ID, Remote: "origin", Number: 7, Head: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := d.create(proto.CreateParams{Dir: repo, Worktree: fetched.Branch, Base: fetched.Base, PushRemote: fetched.PushRemote, PushBranch: fetched.PushBranch})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := d.prCheckout(pr); err != nil || !samePath(got.Worktree, review.Worktree.Path) {
		t.Fatalf("a new worktree with a review's: %+v, %v", got, err)
	}
	// Here, it checks out anew, the review's on a branch of its own...
	pr.Here = true
	if got, err := d.prCheckout(pr); err != nil || got.Worktree != "" || branchOf(repo) != "fix" {
		t.Fatalf("here with a review's worktree: %+v, %v, on %s", got, err, branchOf(repo))
	}
	// ...but in the review's, the main checkout has the branch.
	pr.Pane = review.Panes[0].ID
	if got, err := d.prCheckout(pr); err != nil || !samePath(got.Worktree, repo) || branchOf(review.Worktree.Path) != fetched.Branch {
		t.Fatalf("here, the branch elsewhere: %+v, %v", got, err)
	}
}
