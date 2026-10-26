package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"kero/internal/git"
	"kero/internal/github"
	"kero/internal/proto"
)

// Draft reviews of pull requests: the comments an agent that reviews one
// writes with kero pr, which the app shows on the pull request's changes
// for the user to edit before the review goes to GitHub as one. kerod
// keeps each in a file of its own, until it is posted or discarded; the
// agent never posts.

// reviewMu keeps one change to the drafts at a time.
var reviewMu sync.Mutex

func reviewDir() string { return filepath.Join(proto.Dir(), "reviews") }

func reviewFile(slug string, number int) string {
	name := strings.ReplaceAll(strings.ToLower(slug), "/", "~") + "~" + strconv.Itoa(number) + ".json"
	return filepath.Join(reviewDir(), name)
}

// loadReview returns a pull request's draft, nil for none.
func loadReview(slug string, number int) (*proto.PRReviewDraft, error) {
	b, err := os.ReadFile(reviewFile(slug, number))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var dr proto.PRReviewDraft
	if err := json.Unmarshal(b, &dr); err != nil {
		return nil, fmt.Errorf("the draft review of %s#%d: %w", slug, number, err)
	}
	return &dr, nil
}

func saveReview(dr *proto.PRReviewDraft) error {
	dr.Updated = time.Now().Unix()
	b, err := json.MarshalIndent(dr, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(reviewDir(), 0o700); err != nil {
		return err
	}
	path := reviewFile(dr.Repo, dr.Number)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// agentReview returns the draft the agent in pane writes, nil for none.
func agentReview(pane string) (*proto.PRReviewDraft, error) {
	entries, err := os.ReadDir(reviewDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found *proto.PRReviewDraft
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(reviewDir(), e.Name()))
		if err != nil {
			continue
		}
		var dr proto.PRReviewDraft
		if json.Unmarshal(b, &dr) != nil || dr.Agent != pane {
			continue
		}
		// The latest, should the pane have reviewed several.
		if found == nil || dr.Updated > found.Updated {
			found = &dr
		}
	}
	return found, nil
}

// reviewTarget is the pull request a draft is of, and the repository it
// is reviewed in.
type reviewTarget struct {
	r      git.Repo
	repo   github.Repo
	slug   string
	remote string
	number int
}

// reviewTargetOf returns the pull request p names: Number of Remote's
// repository of Pane's; with no Number, the one whose draft Pane's agent
// writes.
func (d *Daemon) reviewTargetOf(ctx context.Context, p proto.PRReviewParams) (reviewTarget, error) {
	var t reviewTarget
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return t, err
	}
	t.r = r
	repos, byName, err := gitHubRepos(ctx, r)
	if err != nil {
		return t, err
	}
	remote, number := p.Remote, p.Number
	if number <= 0 {
		dr, err := agentReview(p.Pane)
		if err != nil {
			return t, err
		}
		if dr == nil {
			return t, errors.New("no review of a pull request is started in this pane: name the pull request with --pr <number>")
		}
		remote, number = dr.Remote, dr.Number
	}
	if t.remote, err = pickRemote(ctx, r, byName, repos, remote); err != nil {
		return t, err
	}
	t.repo, t.number = byName[t.remote], number
	t.slug = t.repo.Slug()
	return t, nil
}

// prReview acts on a pull request's draft review, as p.Op says.
func (d *Daemon) prReview(p proto.PRReviewParams) (*proto.PRReviewDraft, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t, err := d.reviewTargetOf(ctx, p)
	if err != nil {
		return nil, err
	}
	reviewMu.Lock()
	defer reviewMu.Unlock()
	dr, err := loadReview(t.slug, t.number)
	if err != nil {
		return nil, err
	}
	need := func() error {
		if dr == nil {
			return fmt.Errorf("%s#%d has no draft review: start one from the Review view", t.slug, t.number)
		}
		return nil
	}
	switch p.Op {
	case "get":
		return dr, nil
	case "start":
		if dr, err = d.startReview(ctx, t, dr, p.Agent); err != nil {
			return nil, err
		}
	case "comment":
		if err := need(); err != nil {
			return nil, err
		}
		if err := addComment(ctx, t.r, dr, p.Comment); err != nil {
			return nil, err
		}
	case "delete":
		if err := need(); err != nil {
			return nil, err
		}
		n := len(dr.Comments)
		dr.Comments = slices.DeleteFunc(dr.Comments, func(c proto.PRDraftComment) bool { return c.ID == p.ID })
		if len(dr.Comments) == n {
			return nil, fmt.Errorf("the draft has no comment %d", p.ID)
		}
	case "summary":
		if err := need(); err != nil {
			return nil, err
		}
		dr.Summary = strings.TrimSpace(p.Summary)
	case "post":
		if err := need(); err != nil {
			return nil, err
		}
		url, err := github.PostReview(ctx, t.r.Root, t.repo, t.number, postOf(dr, p.Event))
		if err != nil {
			return nil, err
		}
		if err := os.Remove(reviewFile(t.slug, t.number)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		dr.Posted = url
		d.reviewChanged(t)
		return dr, nil
	case "discard":
		if err := os.Remove(reviewFile(t.slug, t.number)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		d.reviewChanged(t)
		return nil, nil
	default:
		return nil, errors.New("no review op " + p.Op)
	}
	if err := saveReview(dr); err != nil {
		return nil, err
	}
	d.reviewChanged(t)
	return dr, nil
}

// reviewChanged tells the apps a draft changed.
func (d *Daemon) reviewChanged(t reviewTarget) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.broadcastLocked(proto.Message{Event: "review", Review: &proto.PRReviewRef{Slug: t.slug, Number: t.number}})
}

// startReview makes the pull request's draft, or takes the one there, for
// agent to write: at its head, fetched with its base.
func (d *Daemon) startReview(ctx context.Context, t reviewTarget, dr *proto.PRReviewDraft, agent string) (*proto.PRReviewDraft, error) {
	dt, err := github.View(ctx, t.r.Root, t.repo, t.number)
	if err != nil {
		return nil, err
	}
	var v proto.PRView
	if err := d.prChanges(ctx, t.r, t.remote, dt, &v); err != nil {
		return nil, err
	}
	if dr == nil {
		dr = &proto.PRReviewDraft{Repo: t.slug, Number: t.number, Comments: []proto.PRDraftComment{}}
	}
	dr.Remote, dr.Title, dr.Head, dr.MergeBase = t.remote, dt.Title, dt.HeadOID, v.MergeBase
	if agent != "" {
		dr.Agent = agent
	}
	return dr, nil
}

// addComment adds c to the draft, or edits the comment of its ID: on lines
// of the pull request's diff, which GitHub takes comments on, alone.
func addComment(ctx context.Context, r git.Repo, dr *proto.PRReviewDraft, c proto.PRDraftComment) error {
	c.Body = strings.TrimSpace(c.Body)
	if c.Body == "" {
		return errors.New("the comment is empty")
	}
	if c.ID > 0 {
		i := slices.IndexFunc(dr.Comments, func(x proto.PRDraftComment) bool { return x.ID == c.ID })
		if i < 0 {
			return fmt.Errorf("the draft has no comment %d", c.ID)
		}
		// Its text alone, unless its place is given anew.
		if c.Path == "" {
			dr.Comments[i].Body = c.Body
			return nil
		}
	}
	c.Path = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(c.Path)), "./")
	switch strings.ToUpper(c.Side) {
	case "", "RIGHT":
		c.Side = "RIGHT"
	case "LEFT":
		c.Side = "LEFT"
	default:
		return fmt.Errorf("no side %q: RIGHT, the pull request's lines, or LEFT, the base's", c.Side)
	}
	if c.Start == c.Line {
		c.Start = 0
	}
	if c.Line <= 0 || c.Start < 0 || c.Start > c.Line {
		return fmt.Errorf("no lines %d to %d", c.Start, c.Line)
	}
	hunks, err := fileHunks(ctx, r, dr.MergeBase, dr.Head, c.Path)
	if err != nil {
		return err
	}
	if err := inHunks(hunks, c); err != nil {
		return err
	}
	if c.By == "" {
		c.By = "agent"
	}
	if c.ID > 0 {
		i := slices.IndexFunc(dr.Comments, func(x proto.PRDraftComment) bool { return x.ID == c.ID })
		c.Created = dr.Comments[i].Created
		dr.Comments[i] = c
		return nil
	}
	for _, x := range dr.Comments {
		c.ID = max(c.ID, x.ID)
	}
	c.ID++
	c.Created = time.Now().Unix()
	dr.Comments = append(dr.Comments, c)
	return nil
}

// hunk is a change of a diff: the old lines OldStart and the OldLines
// after it, from 1, became NewLines from NewStart.
type hunk struct{ OldStart, OldLines, NewStart, NewLines int }

var hunkHeader = regexp.MustCompile(`(?m)^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// fileHunks returns the hunks of path's diff from base to head, as
// GitHub's pull request shows them, with three lines around each change.
func fileHunks(ctx context.Context, r git.Repo, base, head, path string) ([]hunk, error) {
	files, err := r.Changes(ctx, base, head)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(files, func(f git.File) bool { return f.Path == path })
	if i < 0 {
		var paths []string
		for _, f := range files {
			paths = append(paths, f.Path)
		}
		return nil, fmt.Errorf("the pull request doesn't change %s; it changes: %s", path, strings.Join(paths, ", "))
	}
	paths := []string{path}
	if old := files[i].OldPath; old != "" {
		paths = append(paths, old)
	}
	patch, err := r.Patch(ctx, base, head, paths...)
	if err != nil {
		return nil, err
	}
	var hs []hunk
	for _, m := range hunkHeader.FindAllStringSubmatch(patch, -1) {
		n := func(s string) int {
			if s == "" {
				return 1
			}
			v, _ := strconv.Atoi(s)
			return v
		}
		hs = append(hs, hunk{OldStart: n(m[1]), OldLines: n(m[2]), NewStart: n(m[3]), NewLines: n(m[4])})
	}
	return hs, nil
}

// inHunks reports, as an error that says where it can go, a comment whose
// lines aren't all in one hunk of its side.
func inHunks(hs []hunk, c proto.PRDraftComment) error {
	start := c.Start
	if start == 0 {
		start = c.Line
	}
	var ranges []string
	for _, h := range hs {
		from, n := h.NewStart, h.NewLines
		if c.Side == "LEFT" {
			from, n = h.OldStart, h.OldLines
		}
		if n == 0 {
			continue
		}
		to := from + n - 1
		if from <= start && c.Line <= to {
			return nil
		}
		ranges = append(ranges, strconv.Itoa(from)+"-"+strconv.Itoa(to))
	}
	where := "the pull request's"
	if c.Side == "LEFT" {
		where = "the base's"
	}
	if len(ranges) == 0 {
		return fmt.Errorf("%s has no lines of %s in the diff to comment on", c.Path, where)
	}
	return fmt.Errorf("%s:%d isn't in the pull request's diff, which GitHub takes comments on alone; lines of %s you can comment on: %s", c.Path, c.Line, where, strings.Join(ranges, ", "))
}

// postOf is the review a draft posts, with event, COMMENT when "".
func postOf(dr *proto.PRReviewDraft, event string) github.NewReview {
	if event == "" {
		event = github.EventComment
	}
	v := github.NewReview{Commit: dr.Head, Body: dr.Summary, Event: event}
	for _, c := range dr.Comments {
		v.Comments = append(v.Comments, github.NewLineComment{Path: c.Path, Side: c.Side, Start: c.Start, Line: c.Line, Body: c.Body})
	}
	return v
}
