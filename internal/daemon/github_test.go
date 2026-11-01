package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kero/internal/git"
	"kero/internal/github"
	"kero/internal/proto"
)

// A pull request of a branch brings the commits and the files since it
// left the base, as last fetched, not what isn't committed; the branch's
// unpushed commits are counted, and a fork's branch goes into the parent
// as owner:branch. Without gh, the draft says so, and GitHub's page that
// makes the pull request is still there.
func TestPRDraft(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git")
	}
	// gh is left out of reach: the test asks GitHub nothing.
	t.Setenv("PATH", filepath.Dir(git))
	runGit(t, repo, "remote", "add", "origin", "git@github.com:acme/web.git")
	runGit(t, repo, "remote", "add", "upstream", "https://github.com/parent/web")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runGit(t, repo, "update-ref", "refs/remotes/upstream/main", "HEAD")
	runGit(t, repo, "checkout", "-q", "-b", "feature")
	for _, name := range []string{"b.txt", "c.txt"} {
		os.WriteFile(filepath.Join(repo, name), []byte(name+"\n"), 0o644)
		runGit(t, repo, "add", name)
		runGit(t, repo, "commit", "-q", "-m", "add "+name)
	}
	os.WriteFile(filepath.Join(repo, "draft.txt"), []byte("not yet\n"), 0o644)
	s, err := d.create(proto.CreateParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	defer d.removeSession(s.ID)

	dr, err := d.prDraft(proto.PRDraftParams{Pane: s.ID})
	if err != nil {
		t.Fatal(err)
	}
	if dr.Remote != "origin" || dr.Base != "main" || dr.Head != "feature" || dr.HeadOwner != "acme" || dr.PushRemote != "origin" {
		t.Errorf("draft %+v", dr)
	}
	if len(dr.Repos) != 2 || dr.Repos[0].Slug != "acme/web" || dr.Repos[1].Slug != "parent/web" {
		t.Errorf("repos %+v", dr.Repos)
	}
	if len(dr.Commits) != 2 || dr.Commits[0].Subject != "add c.txt" {
		t.Errorf("commits %+v", dr.Commits)
	}
	var paths []string
	for _, f := range dr.Files {
		paths = append(paths, f.Path)
		if f.From != dr.MergeBase || f.To == "" {
			t.Errorf("%s between %q and %q", f.Path, f.From, f.To)
		}
	}
	if strings.Join(paths, ",") != "b.txt,c.txt" {
		t.Errorf("files %q: what isn't committed is no part of it", paths)
	}
	if dr.Unpushed != 2 || dr.Uncommitted != 1 {
		t.Errorf("unpushed %d, uncommitted %d", dr.Unpushed, dr.Uncommitted)
	}
	if !strings.Contains(dr.Error, "gh") {
		t.Errorf("without gh, the draft's error is %q", dr.Error)
	}

	made, err := d.prCreate(proto.PRCreateParams{Pane: s.ID, Remote: "upstream", Base: "main", Title: "Add b & c", Show: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(made.URL, "https://github.com/parent/web/compare/main...acme:feature?") || !strings.Contains(made.URL, "title=Add+b+%26+c") {
		t.Errorf("the page is %s", made.URL)
	}
	if _, err := d.prCreate(proto.PRCreateParams{Pane: s.ID, Base: "main", Title: "x"}); err == nil || !strings.Contains(err.Error(), "gh") {
		t.Errorf("created without gh: %v", err)
	}
	if _, err := d.prDraft(proto.PRDraftParams{Pane: s.ID, Base: "nope"}); err == nil || !strings.Contains(err.Error(), "fetch") {
		t.Errorf("a base never fetched: %v", err)
	}
}

// A pull request's changes are fetched with its base: of a fork's, from
// GitHub's refs/pull/N/head, as pr-N; of the repository's own branch, by
// its name; both from where the branch left the base, which moved on.
func TestPRChanges(t *testing.T) {
	d := testDaemon(t)
	up := gitRepo(t)
	runGit(t, up, "checkout", "-q", "-b", "fix")
	os.WriteFile(filepath.Join(up, "fix.txt"), []byte("fix\n"), 0o644)
	runGit(t, up, "add", "fix.txt")
	runGit(t, up, "commit", "-q", "-m", "fix")
	runGit(t, up, "update-ref", "refs/pull/5/head", "fix")
	runGit(t, up, "checkout", "-q", "main")
	os.WriteFile(filepath.Join(up, "later.txt"), []byte("later\n"), 0o644)
	runGit(t, up, "add", "later.txt")
	runGit(t, up, "commit", "-q", "-m", "later")
	out, err := exec.Command("git", "-C", up, "rev-parse", "fix").Output()
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(string(out))

	work, _ := filepath.EvalSymlinks(t.TempDir())
	runGit(t, work, "init", "-q")
	runGit(t, work, "remote", "add", "origin", up)
	ctx := context.Background()
	r, err := git.Open(ctx, work)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		cross bool
		ref   string
	}{{true, "refs/remotes/origin/pr-5"}, {false, "refs/remotes/origin/fix"}} {
		var res proto.PRView
		dt := github.Detail{PR: github.PR{Number: 5, Head: "fix", Base: "main", CrossRepo: c.cross}, HeadOID: head}
		if err := d.prChanges(ctx, r, "origin", dt, &res); err != nil {
			t.Fatal(err)
		}
		if len(res.Files) != 1 || res.Files[0].Path != "fix.txt" || res.Files[0].To != head || res.Files[0].From != res.MergeBase || res.MergeBase == "" {
			t.Errorf("cross %v: files %+v, merge base %q", c.cross, res.Files, res.MergeBase)
		}
		if err := exec.Command("git", "-C", work, "rev-parse", "--verify", c.ref).Run(); err != nil {
			t.Errorf("no %s", c.ref)
		}
	}
}
