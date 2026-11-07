package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kero/internal/proto"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// A new worktree's branch starts from the remote's copy of its base,
// fetched fresh, and tracks nothing; from the local base when the fetch
// fails, or when the local base has commits the remote's lacks.
func TestWorktreeFromOrigin(t *testing.T) {
	d := testDaemon(t)
	up := gitRepo(t)
	repo := filepath.Join(t.TempDir(), "clone")
	runGit(t, up, "clone", "-q", up, repo)
	commit := func(dir, msg string) {
		os.WriteFile(filepath.Join(dir, msg+".txt"), []byte(msg+"\n"), 0o644)
		runGit(t, dir, "add", "-A")
		runGit(t, dir, "commit", "-q", "-m", msg)
	}
	local := gitOut(t, repo, "rev-parse", "main")
	commit(up, "second")
	ahead := gitOut(t, up, "rev-parse", "main")

	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "fresh"})
	if err != nil {
		t.Fatal(err)
	}
	if wt := s.Worktree; wt.Base != "origin/main" || gitOut(t, wt.Path, "rev-parse", "HEAD") != ahead {
		t.Errorf("started from %q at %s, not origin's %s", wt.Base, gitOut(t, wt.Path, "rev-parse", "HEAD"), ahead)
	}
	if out, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "fresh@{upstream}").Output(); err == nil {
		t.Errorf("the branch tracks %s", out)
	}
	if got := gitOut(t, repo, "rev-parse", "main"); got != local {
		t.Errorf("local main moved to %s", got)
	}

	// Offline, it starts from the local branch.
	commit(up, "third")
	runGit(t, repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
	s, err = d.create(proto.CreateParams{Dir: repo, Worktree: "offline"})
	if err != nil {
		t.Fatal(err)
	}
	if wt := s.Worktree; wt.Base != "main" || gitOut(t, wt.Path, "rev-parse", "HEAD") != local {
		t.Errorf("offline, started from %q", wt.Base)
	}

	// Local commits the remote's branch lacks aren't left out.
	runGit(t, repo, "remote", "set-url", "origin", up)
	commit(repo, "mine")
	s, err = d.create(proto.CreateParams{Dir: repo, Worktree: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if wt := s.Worktree; wt.Base != "main" || gitOut(t, wt.Path, "rev-parse", "HEAD") != gitOut(t, repo, "rev-parse", "main") {
		t.Errorf("with local commits, started from %q", wt.Base)
	}
}
