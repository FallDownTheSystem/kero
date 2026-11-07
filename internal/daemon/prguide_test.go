package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kero/internal/proto"
)

// The agent that writes a pull request's guide, from its pane, which names
// no pull request: stops in the order given, or where --at puts them, on
// lines of the diff alone or on a whole file; edits keep what they leave
// out; clear empties it, discard leaves nothing; the draft review's files
// are no guide's.
func TestPRGuide(t *testing.T) {
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
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o644)
	runGit(t, repo, "add", "new.txt")
	runGit(t, repo, "commit", "-q", "-am", "fix")
	head := revParse(t, repo, "HEAD")
	s, err := d.create(proto.CreateParams{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	defer d.removeSession(s.ID)
	agent := s.Panes[0].ID
	// A draft review of another pull request, by the same agent, which the
	// guide's own file goes over.
	if err := saveReview(&proto.PRReviewDraft{Repo: "Acme/web", Remote: "origin", Number: 4, Head: head, MergeBase: base, Agent: agent, Comments: []proto.PRDraftComment{}}); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(reviewFile("Acme/web", 4))
	defer os.Remove(guideFile("Acme/web", 5))
	if err := saveGuide(&proto.PRGuide{Repo: "Acme/web", Remote: "origin", Number: 5, Head: head, MergeBase: base, Agent: agent, Stops: []proto.PRGuideStop{}}); err != nil {
		t.Fatal(err)
	}
	op := func(p proto.PRGuideParams) (*proto.PRGuide, error) {
		p.Pane = agent
		return d.prGuide(p)
	}
	stop := func(st proto.PRGuideStop, at int) *proto.PRGuide {
		t.Helper()
		g, err := op(proto.PRGuideParams{Op: "stop", Stop: st, At: at})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	ids := func(g *proto.PRGuide) (out []int) {
		for _, s := range g.Stops {
			out = append(out, s.ID)
		}
		return out
	}

	g := stop(proto.PRGuideStop{Path: "./big.txt", Start: 18, Line: 21, Title: " The fix ", Note: "Reads the change."}, 0)
	if g.Number != 5 || len(g.Stops) != 1 || g.Stops[0] != (proto.PRGuideStop{ID: 1, Path: "big.txt", Side: "RIGHT", Start: 18, Line: 21, Title: "The fix", Note: "Reads the change."}) {
		t.Fatalf("guide %+v", g)
	}
	// A whole file, whatever lines it was given none of.
	g = stop(proto.PRGuideStop{Path: "new.txt", Title: "Mechanical", Closely: true}, 0)
	if s := g.Stops[1]; s.Side != "" || s.Line != 0 || !s.Closely {
		t.Errorf("file stop %+v", s)
	}
	// One put first.
	g = stop(proto.PRGuideStop{Path: "big.txt", Line: 20, Title: "First"}, 1)
	if got := ids(g); len(got) != 3 || got[0] != 3 || got[1] != 1 || got[2] != 2 {
		t.Errorf("order %v", got)
	}
	if g.Stops[0].Start != 20 {
		t.Errorf("one line %+v", g.Stops[0])
	}

	// Lines that show none of the diff, which is 17 to 23; a file it
	// doesn't change; no title.
	if _, err := op(proto.PRGuideParams{Op: "stop", Stop: proto.PRGuideStop{Path: "big.txt", Start: 30, Line: 35, Title: "x"}}); err == nil || !strings.Contains(err.Error(), "17-23") {
		t.Errorf("lines out of the diff: %v", err)
	}
	if _, err := op(proto.PRGuideParams{Op: "stop", Stop: proto.PRGuideStop{Path: "a.txt", Title: "x"}}); err == nil || !strings.Contains(err.Error(), "big.txt") {
		t.Errorf("a file not changed: %v", err)
	}
	if _, err := op(proto.PRGuideParams{Op: "stop", Stop: proto.PRGuideStop{Path: "big.txt", Line: 20}}); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("no title: %v", err)
	}

	// An edit of the note alone keeps the rest, and the stop's place in
	// the tour; with At, it moves.
	g = stop(proto.PRGuideStop{ID: 1, Note: "Better."}, 0)
	if s := g.Stops[1]; s.ID != 1 || s.Title != "The fix" || s.Note != "Better." || s.Start != 18 || s.Line != 21 {
		t.Errorf("edited %+v", s)
	}
	g = stop(proto.PRGuideStop{ID: 2, Closely: false}, 1)
	if got := ids(g); got[0] != 2 || got[1] != 3 || got[2] != 1 || g.Stops[0].Closely {
		t.Errorf("moved %v %+v", got, g.Stops[0])
	}
	if g, err = op(proto.PRGuideParams{Op: "delete", ID: 3}); err != nil || len(g.Stops) != 2 {
		t.Errorf("deleted %+v %v", g, err)
	}
	if _, err := op(proto.PRGuideParams{Op: "delete", ID: 3}); err == nil {
		t.Error("deleted a stop twice")
	}
	if g, err = op(proto.PRGuideParams{Op: "summary", Summary: " Fixes a line. "}); err != nil || g.Summary != "Fixes a line." {
		t.Errorf("summary %+v %v", g, err)
	}
	// The draft review is untouched by the guide.
	if dr, err := loadReview("Acme/web", 4); err != nil || dr == nil || len(dr.Comments) != 0 {
		t.Errorf("draft %+v %v", dr, err)
	}
	if g, err = op(proto.PRGuideParams{Op: "clear"}); err != nil || len(g.Stops) != 0 || g.Summary != "" {
		t.Errorf("cleared %+v %v", g, err)
	}
	if g, err = op(proto.PRGuideParams{Op: "discard"}); err != nil || g != nil {
		t.Errorf("discarded %+v %v", g, err)
	}
	if g, err := loadGuide("Acme/web", 5); err != nil || g != nil {
		t.Errorf("after discard %+v %v", g, err)
	}
}
