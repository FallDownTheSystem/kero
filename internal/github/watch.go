package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Watched is what a session's watch of its pull request looks at: its
// state, its head, whether it merges, its checks, and its conversation's
// comments and reviews (its comments on lines are LineComments').
type Watched struct {
	Number    int
	Title     string
	URL       string
	State     string // OPEN, CLOSED or MERGED
	HeadOID   string
	Head      string
	Base      string
	Mergeable string // MERGEABLE, CONFLICTING or UNKNOWN
	Checks    []Check
	Comments  []Comment
	Reviews   []Review
}

const watchFields = "number,title,url,state,headRefOid,headRefName,baseRefName,mergeable,statusCheckRollup,comments,reviews"

// Watch returns how pull request number of r stands.
func Watch(ctx context.Context, dir string, r Repo, number int) (Watched, error) {
	out, err := run(ctx, dir, "", "pr", "view", strconv.Itoa(number), "--repo", r.Slug(), "--json", watchFields)
	if err != nil {
		return Watched{}, err
	}
	return parseWatched(out)
}

func parseWatched(out []byte) (Watched, error) {
	var raw struct {
		ghDetail
		Mergeable string `json:"mergeable"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Watched{}, fmt.Errorf("gh pr view: %w", err)
	}
	w := Watched{Number: raw.Number, Title: raw.Title, URL: raw.URL, State: raw.State, HeadOID: raw.HeadOID, Head: raw.Head, Base: raw.Base, Mergeable: raw.Mergeable}
	for _, c := range raw.Rollup {
		w.Checks = append(w.Checks, c.check())
	}
	for _, c := range raw.Comments {
		w.Comments = append(w.Comments, Comment{Author: c.Author.Login, Body: c.Body, Created: c.Created, URL: c.URL})
	}
	for _, v := range raw.Reviews {
		w.Reviews = append(w.Reviews, Review{Author: v.Author.Login, State: v.State, Body: v.Body, Submitted: v.Submitted})
	}
	return w, nil
}

// ParsePRURL returns the repository and number of a pull request's page,
// as https://github.com/OWNER/NAME/pull/42; false for another URL.
func ParsePRURL(raw string) (Repo, int, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return Repo{}, 0, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return Repo{}, 0, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n <= 0 {
		return Repo{}, 0, false
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && !strings.Contains(host, "github") {
		return Repo{}, 0, false
	}
	return Repo{Host: host, Owner: parts[0], Name: parts[1]}, n, true
}

// ParseSlug returns the repository of a slug, as Repo.Slug writes it:
// OWNER/NAME, or HOST/OWNER/NAME.
func ParseSlug(slug string) (Repo, bool) {
	parts := strings.Split(slug, "/")
	for _, p := range parts {
		if p == "" {
			return Repo{}, false
		}
	}
	switch len(parts) {
	case 2:
		return Repo{Host: "github.com", Owner: parts[0], Name: parts[1]}, true
	case 3:
		return Repo{Host: parts[0], Owner: parts[1], Name: parts[2]}, true
	}
	return Repo{}, false
}
