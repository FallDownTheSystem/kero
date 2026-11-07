package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// MergedAt reports whether a pull request into r was merged with head as
// its last commit: number's, when it isn't 0, else one of branch's, of
// any fork. A worktree still at that commit lost nothing as it merged.
func MergedAt(ctx context.Context, dir string, r Repo, number int, branch, head string) (bool, error) {
	args := []string{"pr", "list", "--repo", r.Slug(), "--state", "merged", "--head", branch, "--limit", "20", "--json", "headRefOid"}
	if number != 0 {
		args = []string{"pr", "view", strconv.Itoa(number), "--repo", r.Slug(), "--json", "state,headRefOid"}
	}
	out, err := run(ctx, dir, "", args...)
	if err != nil {
		return false, err
	}
	type pr struct {
		State string `json:"state"`
		Head  string `json:"headRefOid"`
	}
	var prs []pr
	if number != 0 {
		var p pr
		err = json.Unmarshal(out, &p)
		if p.State != "MERGED" {
			return false, err
		}
		prs = []pr{p}
	} else {
		err = json.Unmarshal(out, &prs)
	}
	if err != nil {
		return false, fmt.Errorf("gh %s: %w", args[1], err)
	}
	for _, p := range prs {
		if p.Head != "" && p.Head == head {
			return true, nil
		}
	}
	return false, nil
}
