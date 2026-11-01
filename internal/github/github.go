// Package github reads and makes a repository's pull requests on GitHub
// through the GitHub CLI (gh), as the user signed in to it: kerod runs it
// on the host the repository is on, as it runs git.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNoGH: the GitHub CLI isn't installed, or not where kerod finds it.
var ErrNoGH = errors.New("the GitHub CLI (gh) isn't installed: install it from cli.github.com, then sign in with gh auth login")

// ErrSignedOut: gh is installed, but not signed in to the repository's
// host.
var ErrSignedOut = errors.New("the GitHub CLI isn't signed in: run gh auth login in a terminal")

// Repo is a repository on GitHub, as a git remote's URL names it.
type Repo struct {
	Host  string // github.com, or a GitHub Enterprise host
	Owner string
	Name  string
}

// Slug is the repository as gh's --repo takes it: OWNER/NAME, the host
// before them when it isn't github.com.
func (r Repo) Slug() string {
	if r.Host == "" || r.Host == "github.com" {
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}

// URL is the repository's page.
func (r Repo) URL() string {
	return "https://" + r.host() + "/" + r.Owner + "/" + r.Name
}

func (r Repo) host() string {
	if r.Host == "" {
		return "github.com"
	}
	return r.Host
}

// ParseRemote returns the repository a remote's URL is of: an https one,
// an ssh one, or scp's form (git@github.com:owner/name.git). It reports
// false for a URL of no GitHub repository: of a host that doesn't look
// like GitHub's, as GitLab's, or a local path.
func ParseRemote(raw string) (Repo, bool) {
	raw = strings.TrimSpace(raw)
	var host, path string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil {
			return Repo{}, false
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(raw, ":") && !strings.Contains(raw, `\`):
		// scp's form: [user@]host:owner/name
		i := strings.Index(raw, ":")
		host, path = raw[:i], raw[i+1:]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
	default:
		return Repo{}, false
	}
	// ssh.github.com is GitHub's ssh over port 443.
	host = strings.TrimPrefix(strings.ToLower(host), "ssh.")
	if host != "github.com" && !strings.Contains(host, "github") {
		return Repo{}, false
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, false
	}
	return Repo{Host: host, Owner: parts[0], Name: parts[1]}, true
}

// run runs gh in dir with args, stdin given to it when not "", and returns
// what it printed. It never asks: what would need an answer fails.
func run(ctx context.Context, dir, stdin string, args ...string) ([]byte, error) {
	gh, err := exec.LookPath("gh")
	if err != nil {
		return nil, ErrNoGH
	}
	cmd := exec.CommandContext(ctx, gh, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "GH_SPINNER_DISABLED=1", "NO_COLOR=1", "GIT_TERMINAL_PROMPT=0")
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	hide(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("gh %s: %w", args[0], ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "gh auth login") {
			return out, ErrSignedOut
		}
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("gh %s: %s", args[0], msg)
	}
	return out, nil
}

// PR is a pull request, as the list shows it.
type PR struct {
	Number    int
	Title     string
	Body      string
	URL       string
	Author    string
	Head      string // its branch
	HeadOwner string // the owner of the repository its branch is in
	CrossRepo bool   // its branch is in another repository: a fork's
	Base      string
	Draft     bool
	// Checks sums its checks up: "success", "failure", "pending", or ""
	// for none.
	Checks string
	// Review is GitHub's review decision: "APPROVED", "CHANGES_REQUESTED",
	// "REVIEW_REQUIRED", or "".
	Review  string
	Updated time.Time
}

// prFields are the fields of gh's JSON that PR is made of.
const prFields = "number,title,body,url,author,headRefName,headRepositoryOwner,isCrossRepository,baseRefName,isDraft,statusCheckRollup,reviewDecision,updatedAt"

type ghPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Head      string `json:"headRefName"`
	HeadOwner struct {
		Login string `json:"login"`
	} `json:"headRepositoryOwner"`
	CrossRepo bool      `json:"isCrossRepository"`
	Base      string    `json:"baseRefName"`
	Draft     bool      `json:"isDraft"`
	Checks    []ghCheck `json:"statusCheckRollup"`
	Review    string    `json:"reviewDecision"`
	Updated   time.Time `json:"updatedAt"`
}

// ghCheck is a check run's (status, conclusion) or a commit status's
// (state).
type ghCheck struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

func (p ghPR) pr() PR {
	return PR{Number: p.Number, Title: p.Title, Body: p.Body, URL: p.URL, Author: p.Author.Login, Head: p.Head, HeadOwner: p.HeadOwner.Login,
		CrossRepo: p.CrossRepo, Base: p.Base, Draft: p.Draft, Checks: checks(p.Checks), Review: p.Review, Updated: p.Updated}
}

// checks sums checks up: failure when one failed, else pending when one
// hasn't finished, else success; "" for none.
func checks(cs []ghCheck) string {
	if len(cs) == 0 {
		return ""
	}
	pending := false
	for _, c := range cs {
		switch strings.ToUpper(c.Conclusion + c.State) {
		case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
			return "failure"
		case "PENDING", "EXPECTED", "":
			pending = true
		}
		if c.Status != "" && !strings.EqualFold(c.Status, "COMPLETED") {
			pending = true
		}
	}
	if pending {
		return "pending"
	}
	return "success"
}

// Filters of List: every open pull request, the user's own, or those the
// user's review is asked of.
const (
	FilterAll    = ""
	FilterMine   = "mine"
	FilterReview = "review"
)

// List returns a repository's open pull requests, the latest updated
// first, as filter picks them.
func List(ctx context.Context, dir string, r Repo, filter string, max int) ([]PR, error) {
	args := []string{"pr", "list", "--repo", r.Slug(), "--state", "open", "--limit", fmt.Sprint(max), "--json", prFields}
	switch filter {
	case FilterAll:
	case FilterMine:
		args = append(args, "--author", "@me")
	case FilterReview:
		args = append(args, "--search", "review-requested:@me")
	default:
		return nil, errors.New("no pull request filter " + filter)
	}
	return list(ctx, dir, args)
}

// ForBranch returns the open pull request of a branch of headOwner's into
// r, nil for none.
func ForBranch(ctx context.Context, dir string, r Repo, headOwner, branch string) (*PR, error) {
	prs, err := list(ctx, dir, []string{"pr", "list", "--repo", r.Slug(), "--state", "open", "--head", branch, "--limit", "20", "--json", prFields})
	if err != nil {
		return nil, err
	}
	for _, p := range prs {
		// --head matches the branch's name in any fork.
		if strings.EqualFold(p.HeadOwner, headOwner) {
			return &p, nil
		}
	}
	return nil, nil
}

func list(ctx context.Context, dir string, args []string) ([]PR, error) {
	out, err := run(ctx, dir, "", args...)
	if err != nil {
		return nil, err
	}
	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	prs := make([]PR, len(raw))
	for i, p := range raw {
		prs[i] = p.pr()
	}
	return prs, nil
}

// New is a pull request to make: of Head, a branch of HeadOwner's, into
// Base of the repository.
type New struct {
	Base, Head, HeadOwner string
	Title, Body           string
	Draft                 bool
}

// head is the pull request's branch as GitHub takes it: owner:branch
// when it is in another repository than r.
func (n New) head(r Repo) string {
	if n.HeadOwner != "" && !strings.EqualFold(n.HeadOwner, r.Owner) {
		return n.HeadOwner + ":" + n.Head
	}
	return n.Head
}

// Create makes the pull request in r and returns its URL. Its branch must
// be pushed already: gh would offer to push it, and asks nothing here.
func Create(ctx context.Context, dir string, r Repo, n New) (string, error) {
	if strings.TrimSpace(n.Title) == "" {
		return "", errors.New("a pull request needs a title")
	}
	args := []string{"pr", "create", "--repo", r.Slug(), "--base", n.Base, "--head", n.head(r), "--title", n.Title, "--body-file", "-"}
	if n.Draft {
		args = append(args, "--draft")
	}
	// An empty body still goes by stdin, which --body-file - waits for.
	body := n.Body
	if body == "" {
		body = "\n"
	}
	out, err := run(ctx, dir, body, args...)
	if err != nil {
		return "", err
	}
	// gh prints the pull request's URL last.
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 {
		return "", errors.New("gh pr create printed no pull request")
	}
	return lines[len(lines)-1], nil
}

// CompareURL is GitHub's page that opens a pull request of n in r, its
// title and body filled in, for the user to make there.
func CompareURL(r Repo, n New) string {
	q := url.Values{}
	q.Set("expand", "1")
	if n.Title != "" {
		q.Set("title", n.Title)
	}
	if n.Body != "" {
		q.Set("body", n.Body)
	}
	return r.URL() + "/compare/" + url.PathEscape(n.Base) + "..." + url.PathEscape(n.head(r)) + "?" + q.Encode()
}
