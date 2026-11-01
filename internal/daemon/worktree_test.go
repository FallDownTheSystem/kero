package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kero/internal/git"
	"kero/internal/proto"
)

// gitRepo makes a repository with a first commit on main.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	runGit(t, dir, "init", "-q")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A session in a worktree of its own starts there, on its branch from
// the default branch, in the repository's project; closing it keeps the
// worktree unless asked, and removing one that would lose work needs
// Force.
func TestWorktreeSession(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "fix/login"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(repo, ".kero", "worktrees", "fix-login")
	wt := s.Worktree
	if wt == nil || wt.Path != want || wt.Branch != "fix/login" || wt.Base != "main" || wt.Main != repo || s.Project != "pr" {
		t.Fatalf("session %+v, worktree %+v", s, wt)
	}
	if s.Panes[0].Dir != want {
		t.Errorf("the pane starts in %s", s.Panes[0].Dir)
	}
	if _, err := d.create(proto.CreateParams{Dir: repo, Worktree: "fix/login"}); err == nil {
		t.Error("made a second worktree where the first is")
	}
	if _, err := d.create(proto.CreateParams{Dir: repo, Worktree: "a b"}); err == nil {
		t.Error("made a branch of a bad name")
	}
	if _, err := d.create(proto.CreateParams{Dir: t.TempDir(), Worktree: "x"}); err == nil || !strings.Contains(err.Error(), "no git repository") {
		t.Errorf("outside a repository: %v", err)
	}

	os.WriteFile(filepath.Join(want, "b.txt"), []byte("b\n"), 0o644)
	wc, err := d.checkWorktree(s.ID)
	if err != nil || wc.Files != 1 || wc.Shared || !wc.Merged {
		t.Fatalf("check %+v, %v", wc, err)
	}
	if err := d.closeSession(proto.CloseParams{ID: s.ID, RemoveWorktree: true}); err == nil || !strings.Contains(err.Error(), "1 file changed") {
		t.Fatalf("removed a worktree with changes: %v", err)
	}
	if d.findPane(s.Panes[0].ID) == nil {
		t.Fatal("closed the session it refused")
	}
	if err := d.closeSession(proto.CloseParams{ID: s.ID, RemoveWorktree: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Errorf("the worktree stays: %v", err)
	}

	// Closing alone keeps it; a session working in it shares it.
	s, err = d.create(proto.CreateParams{Dir: repo, Worktree: "docs", Base: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := d.create(proto.CreateParams{Dir: s.Worktree.Path})
	if err != nil {
		t.Fatal(err)
	}
	if wc, _ := d.checkWorktree(s.ID); !wc.Shared {
		t.Error("not shared with a session in it")
	}
	if err := d.closeSession(proto.CloseParams{ID: s.ID, RemoveWorktree: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Worktree.Path); err != nil {
		t.Errorf("removed a worktree another session works in: %v", err)
	}
	d.removeSession(other.ID)
}

func TestWorktreePath(t *testing.T) {
	// An absolute path: on Windows, one with a drive, as \wt is relative to
	// the drive of the directory it is used in.
	abs := func(p string) string {
		if runtime.GOOS == "windows" {
			p = "C:" + p
		}
		return filepath.FromSlash(p)
	}
	main := abs("/src/kero")
	if got := worktreePath(main, "fix/login", ""); got != abs("/src/kero/.kero/worktrees/fix-login") {
		t.Error(got)
	}
	if got := worktreePath(main, "x", abs("/wt")); got != abs("/wt/kero/x") {
		t.Error(got)
	}
	if got := worktreePath(main, "x", "../trees"); got != abs("/src/trees/kero/x") {
		t.Error(got)
	}
}

// A session's worktree gets the main checkout's ignored files that
// .worktreeinclude lists, but none from another worktree, and keeps a
// file it checks out.
func TestWorktreeInclude(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".claude/settings.local.json\n.env\nCLAUDE.md\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".worktreeinclude"), []byte(".claude/settings.local.json\nCLAUDE.md\nb.txt\n"), 0o644)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "include")
	os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	os.WriteFile(filepath.Join(repo, ".claude", "settings.local.json"), []byte(`{"dev":"on"}`), 0o644)
	os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("main's\n"), 0o644)
	runGit(t, repo, "add", "b.txt")
	runGit(t, repo, "commit", "-q", "-m", "b")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("changed\n"), 0o644)

	first, err := d.create(proto.CreateParams{Dir: repo, Worktree: "one"})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(first.Worktree.Path, "CLAUDE.md"), []byte("one's\n"), 0o644)
	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "two"})
	if err != nil {
		t.Fatal(err)
	}
	wt := s.Worktree.Path
	if b, err := os.ReadFile(filepath.Join(wt, ".claude", "settings.local.json")); string(b) != `{"dev":"on"}` {
		t.Errorf("settings.local.json: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".env")); !os.IsNotExist(err) {
		t.Errorf("copied .env, which .worktreeinclude doesn't list: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("copied another worktree's CLAUDE.md: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "b.txt")); string(b) != "main's\n" {
		t.Errorf("b.txt, tracked, became %q", b)
	}
}

// Each review of a pull request checks it out on a branch of its own, as
// git checks a branch out in one worktree alone: a second review, or the
// main checkout on the pull request's branch, doesn't keep it from
// starting. Its commits push to the pull request's branch, and closing it
// deletes its branch once they're there.
func TestReviewWorktrees(t *testing.T) {
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
	runGit(t, repo, "push", "-q", "-u", "origin", "fix")

	here, err := d.create(proto.CreateParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	review := func() proto.Session {
		t.Helper()
		got, err := d.prFetch(proto.PRFetchParams{Pane: here.Panes[0].ID, Remote: "origin", Number: 7, Head: "fix"})
		if err != nil {
			t.Fatal(err)
		}
		s, err := d.create(proto.CreateParams{Dir: repo, Worktree: got.Branch, Base: got.Base, PushRemote: got.PushRemote, PushBranch: got.PushBranch})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	one, two := review(), review()
	if one.Worktree.Branch != "review/pr-7" || two.Worktree.Branch != "review/pr-7-2" || one.Worktree.Base != "origin/fix" {
		t.Fatalf("worktrees %+v and %+v", one.Worktree, two.Worktree)
	}
	if _, err := os.Stat(filepath.Join(two.Worktree.Path, "fix.txt")); err != nil {
		t.Errorf("the review hasn't the pull request's files: %v", err)
	}

	r, err := git.Open(context.Background(), one.Worktree.Path)
	if err != nil {
		t.Fatal(err)
	}
	if remote, name, err := r.PushRemote(context.Background(), "review/pr-7"); remote != "origin" || name != "fix" || err != nil {
		t.Errorf("the review pushes to %s/%s, %v", remote, name, err)
	}
	os.WriteFile(filepath.Join(one.Worktree.Path, "review.txt"), []byte("r\n"), 0o644)
	runGit(t, one.Worktree.Path, "add", "-A")
	runGit(t, one.Worktree.Path, "commit", "-q", "-m", "review fix")
	if wc, _ := d.checkWorktree(one.ID); wc.Unpushed != 1 {
		t.Errorf("%d commits unpushed, not the review's 1", wc.Unpushed)
	}
	args, err := r.PushArgs(context.Background(), "review/pr-7", git.PushOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(context.Background(), args, nil); err != nil {
		t.Fatalf("%q: %v", args, err)
	}
	runGit(t, repo, "fetch", "-q", "origin")
	if err := d.closeSession(proto.CloseParams{ID: one.ID, RemoveWorktree: true}); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command("git", "-C", repo, "branch", "--list", "--format=%(refname:short)", "review/*").Output(); strings.TrimSpace(string(out)) != "review/pr-7-2" {
		t.Errorf("branches left: %q", out)
	}

	// A session asking for a branch another worktree has gets kero's word
	// for it, not git's.
	if _, err := d.create(proto.CreateParams{Dir: repo, Worktree: "fix"}); err == nil || !strings.Contains(err.Error(), "fix is checked out in") {
		t.Errorf("a worktree of the main checkout's branch: %v", err)
	}
}

// A commit message of a repository on GitHub is written with that
// repository's own prompt, which the app sends with the others', else the
// prompt of all of them. The command prints which it was given.
func TestCommitMessageRepoPrompt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("grep")
	}
	d := testDaemon(t)
	repo := gitRepo(t)
	runGit(t, repo, "remote", "add", "origin", "git@github.com:Me/Demo.git")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b\n"), 0o644)
	runGit(t, repo, "add", "b.txt")
	s, err := d.create(proto.CreateParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.removeSession(s.ID) })
	write := func(repos map[string]string) string {
		t.Helper()
		m, err := d.gitMessage(proto.GitMessageParams{Pane: s.Panes[0].ID, Command: "grep -oE 'ALL_REPOS|DEMO_OWN|OTHER_REPO'", Prompt: "ALL_REPOS", Repos: repos})
		if err != nil {
			t.Fatal(err)
		}
		return m.Message
	}
	if m := write(map[string]string{"me/demo": "DEMO_OWN", "me/other": "OTHER_REPO"}); !strings.Contains(m, "DEMO_OWN") || strings.Contains(m, "ALL_REPOS") || strings.Contains(m, "OTHER_REPO") {
		t.Errorf("with demo's own prompt: %q", m)
	}
	if m := write(map[string]string{"me/other": "OTHER_REPO"}); !strings.Contains(m, "ALL_REPOS") || strings.Contains(m, "OTHER_REPO") {
		t.Errorf("with another's prompt alone: %q", m)
	}
}
