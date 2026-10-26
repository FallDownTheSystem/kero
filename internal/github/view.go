package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Detail is a pull request as its page shows it: besides the list's, its
// state, its commits' range, its conversation, its reviews and its checks.
type Detail struct {
	PR
	State        string // OPEN, CLOSED or MERGED
	HeadOID      string
	BaseOID      string
	Additions    int
	Deletions    int
	ChangedFiles int
	Labels       []string
	Created      time.Time
	// Requested are those whose review is asked and not given yet: users'
	// logins, and teams' names.
	Requested []string
	Reviews   []Review
	Comments  []Comment
	CheckRuns []Check
}

// Review is a review given: its state is APPROVED, CHANGES_REQUESTED,
// COMMENTED, DISMISSED or PENDING.
type Review struct {
	Author    string
	State     string
	Body      string
	Submitted time.Time
}

// Comment is a comment of the pull request's conversation.
type Comment struct {
	Author  string
	Body    string
	Created time.Time
	URL     string
}

// Check is a check run's, or a commit status's: State is "success",
// "failure", "pending", "skipped" or "neutral".
type Check struct {
	Name     string
	Workflow string
	State    string
	URL      string
}

// detailFields are the fields of gh's JSON that Detail is made of.
const detailFields = prFields + ",state,headRefOid,baseRefOid,additions,deletions,changedFiles,labels,createdAt,reviewRequests,reviews,comments"

type ghDetail struct {
	ghPR
	State        string    `json:"state"`
	HeadOID      string    `json:"headRefOid"`
	BaseOID      string    `json:"baseRefOid"`
	Additions    int       `json:"additions"`
	Deletions    int       `json:"deletions"`
	ChangedFiles int       `json:"changedFiles"`
	Created      time.Time `json:"createdAt"`
	Labels       []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Requests []struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Slug  string `json:"slug"`
	} `json:"reviewRequests"`
	Reviews []struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		State     string    `json:"state"`
		Body      string    `json:"body"`
		Submitted time.Time `json:"submittedAt"`
	} `json:"reviews"`
	Comments []struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Body    string    `json:"body"`
		Created time.Time `json:"createdAt"`
		URL     string    `json:"url"`
	} `json:"comments"`
	Rollup []ghRun `json:"statusCheckRollup"`
}

// ghRun is a check run (name, workflowName, status, conclusion,
// detailsUrl) or a commit status (context, state, targetUrl).
type ghRun struct {
	ghCheck
	Name     string `json:"name"`
	Workflow string `json:"workflowName"`
	Details  string `json:"detailsUrl"`
	Context  string `json:"context"`
	Target   string `json:"targetUrl"`
}

func (g ghRun) check() Check {
	c := Check{Name: g.Name, Workflow: g.Workflow, URL: g.Details}
	if c.Name == "" {
		c.Name, c.URL = g.Context, g.Target
	}
	switch strings.ToUpper(g.Conclusion + g.State) {
	case "SUCCESS":
		c.State = "success"
	case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		c.State = "failure"
	case "SKIPPED":
		c.State = "skipped"
	case "NEUTRAL", "STALE":
		c.State = "neutral"
	default:
		c.State = "pending"
	}
	if g.Status != "" && !strings.EqualFold(g.Status, "COMPLETED") {
		c.State = "pending"
	}
	return c
}

// View returns pull request number of r.
func View(ctx context.Context, dir string, r Repo, number int) (Detail, error) {
	out, err := run(ctx, dir, "", "pr", "view", fmt.Sprint(number), "--repo", r.Slug(), "--json", detailFields)
	if err != nil {
		return Detail{}, err
	}
	return parseDetail(out)
}

func parseDetail(out []byte) (Detail, error) {
	var g ghDetail
	if err := json.Unmarshal(out, &g); err != nil {
		return Detail{}, fmt.Errorf("gh pr view: %w", err)
	}
	d := Detail{PR: g.pr(), State: g.State, HeadOID: g.HeadOID, BaseOID: g.BaseOID, Additions: g.Additions, Deletions: g.Deletions,
		ChangedFiles: g.ChangedFiles, Created: g.Created}
	for _, l := range g.Labels {
		d.Labels = append(d.Labels, l.Name)
	}
	for _, q := range g.Requests {
		switch {
		case q.Login != "":
			d.Requested = append(d.Requested, q.Login)
		case q.Slug != "":
			d.Requested = append(d.Requested, q.Slug)
		case q.Name != "":
			d.Requested = append(d.Requested, q.Name)
		}
	}
	for _, v := range g.Reviews {
		d.Reviews = append(d.Reviews, Review{Author: v.Author.Login, State: v.State, Body: v.Body, Submitted: v.Submitted})
	}
	for _, c := range g.Comments {
		d.Comments = append(d.Comments, Comment{Author: c.Author.Login, Body: c.Body, Created: c.Created, URL: c.URL})
	}
	// Rollup shadows ghPR's checks, of the same field of gh's.
	var sum []ghCheck
	for _, c := range g.Rollup {
		d.CheckRuns = append(d.CheckRuns, c.check())
		sum = append(sum, c.ghCheck)
	}
	d.Checks = checks(sum)
	return d, nil
}

// LineComment is a comment of a review on lines of a file of the pull
// request's diff. Line and StartLine are the file's lines, from 1, on
// Side's file (LEFT the base's, RIGHT the head's), as of the pull
// request's head; 0 for Line when the lines changed since, which leaves
// it outdated, at OriginalLine of the commit it was written on. ReplyTo
// is the comment it answers, 0 for one that starts a thread.
type LineComment struct {
	ID           int64
	ReplyTo      int64
	Path         string
	Body         string
	Author       string
	Created      time.Time
	URL          string
	Side         string
	Line         int
	StartLine    int
	OriginalLine int
}

type ghLineComment struct {
	ID           int64     `json:"id"`
	ReplyTo      int64     `json:"in_reply_to_id"`
	Path         string    `json:"path"`
	Body         string    `json:"body"`
	Created      time.Time `json:"created_at"`
	URL          string    `json:"html_url"`
	Side         string    `json:"side"`
	Line         int       `json:"line"`
	StartLine    int       `json:"start_line"`
	OriginalLine int       `json:"original_line"`
	User         struct {
		Login string `json:"login"`
	} `json:"user"`
}

// LineComments returns the comments on lines of pull request number of r,
// the oldest first.
func LineComments(ctx context.Context, dir string, r Repo, number int) ([]LineComment, error) {
	out, err := api(ctx, dir, r, "", "--paginate", fmt.Sprintf("repos/%s/%s/pulls/%d/comments?per_page=100", r.Owner, r.Name, number))
	if err != nil {
		return nil, err
	}
	return parseLineComments(out)
}

func parseLineComments(out []byte) ([]LineComment, error) {
	// --paginate prints each page's array after the other.
	var cs []LineComment
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var page []ghLineComment
		if err := dec.Decode(&page); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("gh api: %w", err)
		}
		for _, g := range page {
			cs = append(cs, LineComment{ID: g.ID, ReplyTo: g.ReplyTo, Path: g.Path, Body: g.Body, Author: g.User.Login, Created: g.Created, URL: g.URL,
				Side: g.Side, Line: g.Line, StartLine: g.StartLine, OriginalLine: g.OriginalLine})
		}
	}
	return cs, nil
}

// Viewer returns the login of the user gh is signed in as on r's host.
func Viewer(ctx context.Context, dir string, r Repo) (string, error) {
	out, err := api(ctx, dir, r, "", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Review events, as a review is posted with one.
const (
	EventComment        = "COMMENT"
	EventApprove        = "APPROVE"
	EventRequestChanges = "REQUEST_CHANGES"
)

// NewLineComment is a comment of a review to post, on lines Start to Line,
// from 1, of Side's file (LEFT or RIGHT); Start 0 for Line alone.
type NewLineComment struct {
	Path  string
	Side  string
	Start int
	Line  int
	Body  string
}

// NewReview is a review to post on the pull request's Commit: its body,
// its event, and its comments on lines.
type NewReview struct {
	Commit   string
	Body     string
	Event    string
	Comments []NewLineComment
}

// PostReview posts a review on pull request number of r, at once, and
// returns its URL.
func PostReview(ctx context.Context, dir string, r Repo, number int, v NewReview) (string, error) {
	switch v.Event {
	case EventComment, EventApprove, EventRequestChanges:
	default:
		return "", errors.New("no review event " + v.Event)
	}
	if v.Event != EventApprove && strings.TrimSpace(v.Body) == "" && len(v.Comments) == 0 {
		return "", errors.New("the review is empty")
	}
	type comment struct {
		Path      string `json:"path"`
		Body      string `json:"body"`
		Side      string `json:"side"`
		Line      int    `json:"line"`
		StartLine int    `json:"start_line,omitempty"`
		StartSide string `json:"start_side,omitempty"`
	}
	body := struct {
		Commit   string    `json:"commit_id,omitempty"`
		Body     string    `json:"body,omitempty"`
		Event    string    `json:"event"`
		Comments []comment `json:"comments"`
	}{Commit: v.Commit, Body: v.Body, Event: v.Event, Comments: []comment{}}
	for _, c := range v.Comments {
		nc := comment{Path: c.Path, Body: c.Body, Side: c.Side, Line: c.Line}
		if c.Start > 0 && c.Start < c.Line {
			nc.StartLine, nc.StartSide = c.Start, c.Side
		}
		body.Comments = append(body.Comments, nc)
	}
	in, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	out, err := api(ctx, dir, r, string(in), "--method", "POST", fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", r.Owner, r.Name, number), "--input", "-")
	if err != nil {
		return "", err
	}
	var res struct {
		URL string `json:"html_url"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("gh api: %w", err)
	}
	return res.URL, nil
}

// api runs gh api on r's host with args.
func api(ctx context.Context, dir string, r Repo, stdin string, args ...string) ([]byte, error) {
	return run(ctx, dir, stdin, append([]string{"api", "--hostname", r.host()}, args...)...)
}
