package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
)

// Review guides of pull requests: a tour of one, its stops in reading
// order, each on lines of its diff with a note, which an agent writes with
// kero pr guide and the app's Guide tab shows. kerod keeps each in a file
// of its own, apart from the draft review, until it is discarded; nothing
// of it goes to GitHub.

func guideDir() string { return filepath.Join(proto.Dir(), "guides") }

func guideFile(slug string, number int) string {
	name := strings.ReplaceAll(strings.ToLower(slug), "/", "~") + "~" + strconv.Itoa(number) + ".json"
	return filepath.Join(guideDir(), name)
}

// loadGuide returns a pull request's guide, nil for none.
func loadGuide(slug string, number int) (*proto.PRGuide, error) {
	b, err := os.ReadFile(guideFile(slug, number))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var g proto.PRGuide
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("the guide of %s#%d: %w", slug, number, err)
	}
	return &g, nil
}

func saveGuide(g *proto.PRGuide) error {
	g.Updated = time.Now().Unix()
	b, err := json.MarshalIndent(g, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(guideDir(), 0o700); err != nil {
		return err
	}
	path := guideFile(g.Repo, g.Number)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// agentGuide returns the guide the agent in pane, of session sid, writes,
// the latest; nil for none.
func agentGuide(pane, sid string) *proto.PRGuide {
	entries, err := os.ReadDir(guideDir())
	if err != nil {
		return nil
	}
	var found *proto.PRGuide
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(guideDir(), e.Name()))
		if err != nil {
			continue
		}
		var g proto.PRGuide
		if json.Unmarshal(b, &g) != nil {
			continue
		}
		mine := g.Agent != "" && g.Agent == pane || sid != "" && g.Session == sid
		if mine && (found == nil || g.Updated > found.Updated) {
			found = &g
		}
	}
	return found
}

// guideTargetOf returns the pull request p names: Number of Remote's
// repository of Pane's; with no Number, the one whose guide Pane's agent
// writes, else the one whose draft review it writes.
func (d *Daemon) guideTargetOf(ctx context.Context, p proto.PRGuideParams) (reviewTarget, error) {
	rp := proto.PRReviewParams{Pane: p.Pane, Remote: p.Remote, Number: p.Number}
	if rp.Number <= 0 {
		pane := p.Pane
		if pn := d.findPane(p.Pane); pn != nil {
			pane = pn.info.ID
		}
		sid, _ := d.paneSession(p.Pane)
		if g := agentGuide(pane, sid); g != nil {
			rp.Remote, rp.Number = g.Remote, g.Number
		}
	}
	return d.reviewTargetOf(ctx, rp)
}

// prGuide acts on a pull request's guide, as p.Op says. A stop or a
// summary written with no guide yet starts one.
func (d *Daemon) prGuide(p proto.PRGuideParams) (*proto.PRGuide, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t, err := d.guideTargetOf(ctx, p)
	if err != nil {
		return nil, err
	}
	reviewMu.Lock()
	defer reviewMu.Unlock()
	g, err := loadGuide(t.slug, t.number)
	if err != nil {
		return nil, err
	}
	switch p.Op {
	case "get":
		return g, nil
	case "discard":
		if err := os.Remove(guideFile(t.slug, t.number)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		d.reviewChanged(t)
		return nil, nil
	case "start":
		// An agent starts the tour anew, as a rewrite asks.
		if g, err = d.startGuide(ctx, t, nil, p.Agent); err != nil {
			return nil, err
		}
	case "stop", "delete", "summary", "clear":
		if g == nil {
			if g, err = d.startGuide(ctx, t, nil, ""); err != nil {
				return nil, err
			}
		}
		if err := editGuide(ctx, t.r, g, p); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("no guide op " + p.Op)
	}
	if err := saveGuide(g); err != nil {
		return nil, err
	}
	d.reviewChanged(t)
	return g, nil
}

// startGuide makes the pull request's guide, or takes the one there, for
// agent to write: at its head, fetched with its base. One written at an
// older head starts empty, to write anew.
func (d *Daemon) startGuide(ctx context.Context, t reviewTarget, g *proto.PRGuide, agent string) (*proto.PRGuide, error) {
	// What a draft review starts at, which it fetches alike.
	at, err := d.startReview(ctx, t, nil, "")
	if err != nil {
		return nil, err
	}
	if g == nil || g.Head != at.Head {
		g = &proto.PRGuide{Repo: t.slug, Number: t.number, Stops: []proto.PRGuideStop{}}
	}
	g.Remote, g.Title, g.Head, g.MergeBase = at.Remote, at.Title, at.Head, at.MergeBase
	if agent != "" {
		g.Agent = agent
		g.Session, _ = d.paneSession(agent)
	}
	return g, nil
}

// editGuide applies a stop, delete, summary or clear op to g.
func editGuide(ctx context.Context, r git.Repo, g *proto.PRGuide, p proto.PRGuideParams) error {
	switch p.Op {
	case "summary":
		g.Summary = strings.TrimSpace(p.Summary)
	case "clear":
		g.Summary, g.Stops = "", []proto.PRGuideStop{}
	case "delete":
		n := len(g.Stops)
		g.Stops = slices.DeleteFunc(g.Stops, func(s proto.PRGuideStop) bool { return s.ID == p.ID })
		if len(g.Stops) == n {
			return fmt.Errorf("the guide has no stop %d", p.ID)
		}
	case "stop":
		return keepStop(ctx, r, g, p.Stop, p.At)
	}
	return nil
}

// keepStop adds s to the guide at at (from 1; 0 last), or edits the stop
// of its ID: its place when Path is given, its title and note when given,
// and Closely as given; moved to at when at is given.
func keepStop(ctx context.Context, r git.Repo, g *proto.PRGuide, s proto.PRGuideStop, at int) error {
	s.Title, s.Note = strings.TrimSpace(s.Title), strings.TrimSpace(s.Note)
	i := -1
	if s.ID > 0 {
		if i = slices.IndexFunc(g.Stops, func(x proto.PRGuideStop) bool { return x.ID == s.ID }); i < 0 {
			return fmt.Errorf("the guide has no stop %d", s.ID)
		}
		old := g.Stops[i]
		if s.Path == "" {
			s.Path, s.Side, s.Start, s.Line = old.Path, old.Side, old.Start, old.Line
		}
		if s.Title == "" {
			s.Title = old.Title
		}
		if s.Note == "" {
			s.Note = old.Note
		}
	}
	if s.Title == "" {
		return errors.New("the stop has no title: give it one with --title")
	}
	if err := placeStop(ctx, r, g, &s); err != nil {
		return err
	}
	if i >= 0 {
		g.Stops = slices.Delete(g.Stops, i, i+1)
	} else {
		for _, x := range g.Stops {
			s.ID = max(s.ID, x.ID)
		}
		s.ID++
	}
	pos := len(g.Stops)
	switch {
	case at > 0:
		pos = min(at-1, len(g.Stops))
	case i >= 0:
		pos = i // an edit stays where it was
	}
	g.Stops = slices.Insert(g.Stops, pos, s)
	return nil
}

// placeStop checks where a stop is: a file the pull request changes, and,
// with lines, lines of its diff, which the tour shows; or the whole file.
func placeStop(ctx context.Context, r git.Repo, g *proto.PRGuide, s *proto.PRGuideStop) error {
	s.Path = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(s.Path)), "./")
	hunks, err := fileHunks(ctx, r, g.MergeBase, g.Head, s.Path)
	if err != nil {
		return err
	}
	if s.Line == 0 && s.Start == 0 {
		s.Side = ""
		return nil
	}
	switch strings.ToUpper(s.Side) {
	case "", "RIGHT":
		s.Side = "RIGHT"
	case "LEFT":
		s.Side = "LEFT"
	default:
		return fmt.Errorf("no side %q: RIGHT, the pull request's lines, or LEFT, the base's", s.Side)
	}
	if s.Start == 0 {
		s.Start = s.Line
	}
	if s.Start <= 0 || s.Start > s.Line {
		return fmt.Errorf("no lines %d to %d", s.Start, s.Line)
	}
	var ranges []string
	for _, h := range hunks {
		from, n := h.NewStart, h.NewLines
		if s.Side == "LEFT" {
			from, n = h.OldStart, h.OldLines
		}
		if n == 0 {
			continue
		}
		to := from + n - 1
		if s.Start <= to && from <= s.Line {
			return nil
		}
		ranges = append(ranges, strconv.Itoa(from)+"-"+strconv.Itoa(to))
	}
	where := "the pull request's"
	if s.Side == "LEFT" {
		where = "the base's"
	}
	if len(ranges) == 0 {
		return fmt.Errorf("%s has no lines of %s in the diff: make the stop the whole file, with no lines", s.Path, where)
	}
	return fmt.Errorf("%s:%d-%d shows nothing of the pull request's diff; lines of %s in it: %s", s.Path, s.Start, s.Line, where, strings.Join(ranges, ", "))
}
