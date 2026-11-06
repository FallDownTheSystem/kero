package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// The agent that reviews a pull request writes its draft from its pane,
// which names no pull request: comments on lines of the diff alone, which
// kerod checks, saying which lines it takes; edits, deletes and the
// summary; discard leaves nothing.
func TestPRReviewDraft(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git")
	}
	t.Setenv("PATH", filepath.Dir(git))
	runGit(t, repo, "remote", "add", "origin", "git@github.com:Acme/web.git")
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line " + string(rune('a'+i%26))
	}
	os.WriteFile(filepath.Join(repo, "big.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	runGit(t, repo, "add", "big.txt")
	runGit(t, repo, "commit", "-q", "-m", "big")
	base := revParse(t, repo, "HEAD")
	runGit(t, repo, "checkout", "-q", "-b", "fix")
	lines[19] = "changed"
	os.WriteFile(filepath.Join(repo, "big.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	runGit(t, repo, "commit", "-q", "-am", "fix")
	head := revParse(t, repo, "HEAD")
	s, err := d.create(proto.CreateParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	defer d.removeSession(s.ID)
	agent := s.Panes[0].ID
	if err := saveReview(&proto.PRReviewDraft{Repo: "Acme/web", Remote: "origin", Number: 5, Head: head, MergeBase: base, Agent: agent, Comments: []proto.PRDraftComment{}}); err != nil {
		t.Fatal(err)
	}

	op := func(p proto.PRReviewParams) (*proto.PRReviewDraft, error) {
		p.Pane = agent
		return d.prReview(p)
	}
	dr, err := op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{Path: "./big.txt", Start: 19, Line: 20, Body: " Why? "}})
	if err != nil {
		t.Fatal(err)
	}
	if len(dr.Comments) != 1 || dr.Comments[0] != (proto.PRDraftComment{ID: 1, Path: "big.txt", Side: "RIGHT", Start: 19, Line: 20, Body: "Why?", By: "agent", Created: dr.Comments[0].Created}) {
		t.Errorf("comments %+v", dr.Comments)
	}
	// The hunk is lines 17 to 23, three around the change.
	if _, err := op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{Path: "big.txt", Line: 30, Body: "x"}}); err == nil || !strings.Contains(err.Error(), "17-23") {
		t.Errorf("a line out of the diff: %v", err)
	}
	if _, err := op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{Path: "a.txt", Line: 1, Body: "x"}}); err == nil || !strings.Contains(err.Error(), "big.txt") {
		t.Errorf("a file not changed: %v", err)
	}
	if _, err := op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{Path: "big.txt", Side: "LEFT", Line: 20, Body: "was"}}); err != nil {
		t.Errorf("the base's line: %v", err)
	}
	// On a file the pull request changes, with no lines, whatever it was
	// given; not on another.
	dr, err = op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{Path: "big.txt", File: true, Side: "LEFT", Line: 7, Body: "All of it."}})
	if err != nil {
		t.Fatal(err)
	}
	if c := dr.Comments[len(dr.Comments)-1]; !c.File || c.Line != 0 || c.Side != "" || c.Body != "All of it." {
		t.Errorf("on the file %+v", c)
	}
	if _, err := op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{Path: "a.txt", File: true, Body: "x"}}); err == nil || !strings.Contains(err.Error(), "big.txt") {
		t.Errorf("a file not changed: %v", err)
	}
	if v := postOf(dr, ""); !v.Comments[len(v.Comments)-1].File {
		t.Errorf("posts %+v", v.Comments)
	}
	if dr, err = op(proto.PRReviewParams{Op: "delete", ID: dr.Comments[len(dr.Comments)-1].ID}); err != nil || len(dr.Comments) != 2 {
		t.Fatalf("deleted %+v, %v", dr, err)
	}
	dr, err = op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{ID: 1, Body: "Why this?"}})
	if err != nil || dr.Comments[0].Body != "Why this?" || dr.Comments[0].Line != 20 {
		t.Errorf("edited: %+v, %v", dr, err)
	}
	if dr, err = op(proto.PRReviewParams{Op: "delete", ID: 2}); err != nil || len(dr.Comments) != 1 {
		t.Errorf("deleted: %+v, %v", dr, err)
	}
	if dr, err = op(proto.PRReviewParams{Op: "summary", Summary: "One question."}); err != nil || dr.Summary != "One question." {
		t.Errorf("summary: %+v, %v", dr, err)
	}
	// The app names the pull request.
	got, err := d.prReview(proto.PRReviewParams{Op: "get", Pane: s.ID, Number: 5})
	if err != nil || got == nil || len(got.Comments) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	// The list marks the repository's drafts, whatever the case of its
	// name, and no other's.
	if err := saveReview(&proto.PRReviewDraft{Repo: "Acme/api", Number: 5, Comments: []proto.PRDraftComment{}}); err != nil {
		t.Fatal(err)
	}
	if drs, err := repoReviews("acme/WEB"); err != nil || len(drs) != 1 || drs[0].Number != 5 || len(drs[0].Comments) != 1 {
		t.Errorf("the repository's drafts %+v, %v", drs, err)
	}
	if _, err := d.prReview(proto.PRReviewParams{Op: "discard", Pane: s.ID, Number: 5}); err != nil {
		t.Fatal(err)
	}
	if drs, err := repoReviews("Acme/web"); err != nil || len(drs) != 0 {
		t.Errorf("drafts after the discard %+v, %v", drs, err)
	}
	if _, err := op(proto.PRReviewParams{Op: "comment", Comment: proto.PRDraftComment{Path: "big.txt", Line: 20, Body: "x"}}); err == nil || !strings.Contains(err.Error(), "--pr") {
		t.Errorf("a comment after the discard: %v", err)
	}
}

// A session started to review a pull request says so, and still does
// once restored; its agent's draft is found by the session, whose pane's
// id the draft may no longer have, or by the pull request it reviews.
func TestReviewSession(t *testing.T) {
	t.Setenv("KERO_HOME", t.TempDir())
	d := testDaemon(t)
	ref := &proto.PRReviewRef{Slug: "Acme/web", Number: 5}
	s, err := d.create(proto.CreateParams{Dir: t.TempDir(), Review: ref})
	if err != nil {
		t.Fatal(err)
	}
	defer d.removeSession(s.ID)
	if s.Review == nil || *s.Review != *ref {
		t.Errorf("created %+v", s.Review)
	}
	plain, err := d.create(proto.CreateParams{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer d.removeSession(plain.ID)
	for _, st := range d.state().Sessions {
		if want := st.ID == s.ID; (st.Review != nil) != want {
			t.Errorf("session %s review %+v", st.ID, st.Review)
		}
	}
	d.saveState()
	r := testDaemon(t)
	r.dir = d.dir
	r.restore()
	defer func() {
		for _, rs := range r.state().Sessions {
			r.removeSession(rs.ID)
		}
	}()
	for _, st := range r.state().Sessions {
		if want := st.ID == s.ID; (st.Review != nil) != want || want && *st.Review != *ref {
			t.Errorf("restored session %s review %+v", st.ID, st.Review)
		}
	}

	pane := s.Panes[0].ID
	if sid, got := d.paneSession(pane); sid != s.ID || got == nil || *got != *ref {
		t.Errorf("pane's session %q, %+v", sid, got)
	}
	// Of an older kerod: the pane's id alone.
	saveReview(&proto.PRReviewDraft{Repo: "Acme/web", Number: 4, Agent: pane, Comments: []proto.PRDraftComment{}})
	if dr, err := agentReview(pane, "", nil); err != nil || dr == nil || dr.Number != 4 {
		t.Errorf("by the pane %+v, %v", dr, err)
	}
	// Of a pane gone since, bound to its session, the latest.
	saveReview(&proto.PRReviewDraft{Repo: "Acme/web", Number: 3, Agent: "gone", Session: s.ID, Comments: []proto.PRDraftComment{}})
	if dr, err := agentReview(pane, s.ID, nil); err != nil || dr == nil || dr.Number != 3 && dr.Number != 4 {
		t.Errorf("by the session %+v, %v", dr, err)
	}
	if dr, err := agentReview("other", s.ID, nil); err != nil || dr == nil || dr.Number != 3 {
		t.Errorf("by the session alone %+v, %v", dr, err)
	}
	// Of the pull request the session reviews, whatever the case.
	saveReview(&proto.PRReviewDraft{Repo: "acme/WEB", Number: 5, Comments: []proto.PRDraftComment{}})
	if dr, err := agentReview("other", "", ref); err != nil || dr == nil || dr.Number != 5 {
		t.Errorf("by the pull request %+v, %v", dr, err)
	}
	if dr, err := agentReview("other", "nope", nil); err != nil || dr != nil {
		t.Errorf("another's %+v, %v", dr, err)
	}
}

// A review's worktree, on a review/pr-<number> branch, lists with its
// changed files; removing it waits for the session in it to close, and
// for the user to say so of its changes.
func TestReviewWorktreesLeft(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	wt := filepath.Join(repo, ".kero", "worktrees", "review-pr-5")
	runGit(t, repo, "worktree", "add", "-q", "-b", "review/pr-5", wt)
	runGit(t, repo, "branch", "other")
	s, err := d.create(proto.CreateParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	defer d.removeSession(s.ID)
	os.WriteFile(filepath.Join(wt, "new.txt"), []byte("x\n"), 0o644)
	got, err := d.reviewWorktrees(proto.PRWorktreesParams{Pane: s.ID})
	if err != nil || len(got) != 1 || got[0].Number != 5 || got[0].Branch != "review/pr-5" || !samePath(got[0].Path, wt) || got[0].Files != 1 {
		t.Fatalf("review worktrees %+v, %v", got, err)
	}
	in, err := d.create(proto.CreateParams{Dir: wt})
	if err != nil {
		t.Fatal(err)
	}
	remove := func(force bool) error {
		return d.removeReviewWorktree(proto.PRWorktreeRemoveParams{Pane: s.ID, Path: got[0].Path, Force: force})
	}
	if err := remove(true); err == nil || !strings.Contains(err.Error(), "session") {
		t.Errorf("removed one a session works in: %v", err)
	}
	d.removeSession(in.ID)
	if err := remove(false); err == nil || !strings.Contains(err.Error(), "1 changed") {
		t.Errorf("removed one with changes: %v", err)
	}
	if err := d.removeReviewWorktree(proto.PRWorktreeRemoveParams{Pane: s.ID, Path: repo, Force: true}); err == nil {
		t.Error("removed the main checkout")
	}
	// Windows lets go of a folder a moment after its shell ends.
	for i := 0; ; i++ {
		err := remove(true)
		if err == nil {
			break
		}
		if i == 30 {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("the worktree stays: %v", err)
	}
}

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
