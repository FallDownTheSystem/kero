package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// What can be done to a pull request, as its page offers it: merge it,
// mark it ready or make it a draft again, close or reopen it, delete its
// branch, bring its branch up to date with its base, merge it once its
// requirements are met, revert it, and approve the workflow runs a fork's
// commits wait on.

// Merge methods, as gh pr merge takes them.
const (
	MethodMerge  = "merge"
	MethodSquash = "squash"
	MethodRebase = "rebase"
)

// Methods are the merge methods, in GitHub's order.
var Methods = []string{MethodMerge, MethodSquash, MethodRebase}

// mergeOrder is the order the default merge method is taken in, of those
// the repository allows, when the repository's setup names none: squash
// first, as most repositories that allow it prefer it.
var mergeOrder = []string{MethodSquash, MethodMerge, MethodRebase}

// DefaultMethod is how a pull request merges unless the user says
// otherwise: setup, the repository's own (ReviewSetup.MergeMethod), if
// allowed has it, else the first of mergeOrder allowed has; allowed nil
// for not known yet. The app's Merge and kero pr merge take it alike.
func DefaultMethod(setup string, allowed []string) string {
	if setup != "" && (allowed == nil || slices.Contains(allowed, setup)) {
		return setup
	}
	for _, m := range mergeOrder {
		if allowed == nil || slices.Contains(allowed, m) {
			return m
		}
	}
	return MethodMerge
}

// Actions is what a pull request's actions depend on: its state, whether
// it merges as it is and why not, the repository's merge methods, and
// what the user may do.
type Actions struct {
	ID        string // its GraphQL id
	State     string // OPEN, CLOSED or MERGED
	Draft     bool
	Head      string
	HeadOID   string
	Base      string
	CrossRepo bool
	// HeadRepo is the repository its branch is in, OWNER/NAME; HeadExists
	// that the branch is still there, at HeadOID or not.
	HeadRepo   string
	HeadExists bool
	// Mergeable is MERGEABLE, CONFLICTING or UNKNOWN; MergeState GitHub's
	// mergeStateStatus: CLEAN, BEHIND, BLOCKED, DIRTY, DRAFT, HAS_HOOKS,
	// UNSTABLE or UNKNOWN.
	Mergeable  string
	MergeState string
	// AutoMerge is the method it merges with once its requirements are
	// met, "" while auto-merge is off.
	AutoMerge string
	// Methods are the merge methods the repository allows; AutoMergeAllowed
	// whether it allows auto-merge, and DeleteOnMerge whether GitHub
	// deletes a branch once merged.
	Methods          []string
	AutoMergeAllowed bool
	DeleteOnMerge    bool
	// CanWrite is that the user may merge it (write access, or more);
	// CanUpdate that they may edit it (theirs, or write access);
	// CanUpdateBranch that they may update its branch; CanDeleteBranch
	// that they may delete its branch.
	CanWrite        bool
	CanUpdate       bool
	CanUpdateBranch bool
	CanDeleteBranch bool
	// PendingRuns are the workflow runs of its head that wait on a
	// maintainer's approval, by their ids: a fork's, as GitHub asks.
	PendingRuns []int64
}

const actionsQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { ` +
	`mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed autoMergeAllowed deleteBranchOnMerge viewerPermission ` +
	`pullRequest(number: $number) { id state isDraft headRefName headRefOid baseRefName isCrossRepository mergeable mergeStateStatus ` +
	`viewerCanUpdate viewerCanUpdateBranch viewerCanDeleteHeadRef autoMergeRequest { mergeMethod } ` +
	`headRef { name } headRepository { nameWithOwner } } } }`

// ActionsOf returns what pull request number of r's actions depend on.
func ActionsOf(ctx context.Context, dir string, r Repo, number int) (Actions, error) {
	out, err := api(ctx, dir, r, "", "graphql", "-f", "query="+actionsQuery, "-f", "owner="+r.Owner, "-f", "name="+r.Name, "-F", "number="+strconv.Itoa(number))
	if err != nil {
		return Actions{}, err
	}
	a, err := parseActions(out)
	if err != nil {
		return Actions{}, err
	}
	// The runs a fork's commits wait on, which only those who may merge
	// approve. A run listed by no one leaves the rest as they are.
	if a.CrossRepo && a.State == "OPEN" && a.CanWrite {
		a.PendingRuns, _ = pendingRuns(ctx, dir, r, a.HeadOID)
	}
	return a, nil
}

func parseActions(out []byte) (Actions, error) {
	var res struct {
		Data struct {
			Repository *struct {
				MergeAllowed  bool   `json:"mergeCommitAllowed"`
				SquashAllowed bool   `json:"squashMergeAllowed"`
				RebaseAllowed bool   `json:"rebaseMergeAllowed"`
				AutoAllowed   bool   `json:"autoMergeAllowed"`
				DeleteOnMerge bool   `json:"deleteBranchOnMerge"`
				Permission    string `json:"viewerPermission"`
				PullRequest   *struct {
					ID              string `json:"id"`
					State           string `json:"state"`
					Draft           bool   `json:"isDraft"`
					Head            string `json:"headRefName"`
					HeadOID         string `json:"headRefOid"`
					Base            string `json:"baseRefName"`
					CrossRepo       bool   `json:"isCrossRepository"`
					Mergeable       string `json:"mergeable"`
					MergeState      string `json:"mergeStateStatus"`
					CanUpdate       bool   `json:"viewerCanUpdate"`
					CanUpdateBranch bool   `json:"viewerCanUpdateBranch"`
					CanDeleteHead   bool   `json:"viewerCanDeleteHeadRef"`
					AutoMerge       *struct {
						Method string `json:"mergeMethod"`
					} `json:"autoMergeRequest"`
					HeadRef *struct {
						Name string `json:"name"`
					} `json:"headRef"`
					HeadRepo *struct {
						Name string `json:"nameWithOwner"`
					} `json:"headRepository"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return Actions{}, fmt.Errorf("gh api graphql: %w", err)
	}
	repo := res.Data.Repository
	if repo == nil || repo.PullRequest == nil {
		return Actions{}, errors.New("no such pull request")
	}
	pr := repo.PullRequest
	a := Actions{ID: pr.ID, State: pr.State, Draft: pr.Draft, Head: pr.Head, HeadOID: pr.HeadOID, Base: pr.Base, CrossRepo: pr.CrossRepo,
		Mergeable: pr.Mergeable, MergeState: pr.MergeState, AutoMergeAllowed: repo.AutoAllowed, DeleteOnMerge: repo.DeleteOnMerge,
		CanUpdate: pr.CanUpdate, CanUpdateBranch: pr.CanUpdateBranch, CanDeleteBranch: pr.CanDeleteHead,
		HeadExists: pr.HeadRef != nil}
	switch repo.Permission {
	case "ADMIN", "MAINTAIN", "WRITE":
		a.CanWrite = true
	}
	if pr.AutoMerge != nil {
		a.AutoMerge = strings.ToLower(pr.AutoMerge.Method)
	}
	if pr.HeadRepo != nil {
		a.HeadRepo = pr.HeadRepo.Name
	}
	for i, ok := range []bool{repo.MergeAllowed, repo.SquashAllowed, repo.RebaseAllowed} {
		if ok {
			a.Methods = append(a.Methods, Methods[i])
		}
	}
	return a, nil
}

// pendingRuns returns the ids of the workflow runs of head that wait on a
// maintainer's approval.
func pendingRuns(ctx context.Context, dir string, r Repo, head string) ([]int64, error) {
	if head == "" {
		return nil, nil
	}
	path := "repos/" + r.Owner + "/" + r.Name + "/actions/runs?status=action_required&per_page=100&head_sha=" + head
	out, err := api(ctx, dir, r, "", path)
	if err != nil {
		return nil, err
	}
	var res struct {
		Runs []struct {
			ID      int64  `json:"id"`
			HeadSHA string `json:"head_sha"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("gh api: %w", err)
	}
	var ids []int64
	for _, run := range res.Runs {
		if run.HeadSHA == head {
			ids = append(ids, run.ID)
		}
	}
	return ids, nil
}

// Merge is how a pull request merges: with Method; once its requirements
// are met, with Auto; its branch deleted after, with DeleteBranch. Head
// is the commit the user saw it at, which it merges at alone: a push
// since fails the merge.
type Merge struct {
	Method       string
	Auto         bool
	DeleteBranch bool
	Head         string
}

// MergePR merges pull request number of r, or has it merged once its
// requirements are met. With --repo, gh deletes the remote branch alone,
// never a local one, nor switches what is checked out.
func MergePR(ctx context.Context, dir string, r Repo, number int, m Merge) error {
	switch m.Method {
	case MethodMerge, MethodSquash, MethodRebase:
	default:
		return fmt.Errorf("no merge method %q", m.Method)
	}
	args := []string{"pr", "merge", strconv.Itoa(number), "--repo", r.Slug(), "--" + m.Method}
	if m.Head != "" {
		args = append(args, "--match-head-commit", m.Head)
	}
	if m.Auto {
		args = append(args, "--auto")
	}
	if m.DeleteBranch {
		args = append(args, "--delete-branch")
	}
	_, err := run(ctx, dir, "", args...)
	return err
}

// DisableAutoMerge has pull request number of r merge no more once its
// requirements are met.
func DisableAutoMerge(ctx context.Context, dir string, r Repo, number int) error {
	_, err := run(ctx, dir, "", "pr", "merge", strconv.Itoa(number), "--repo", r.Slug(), "--disable-auto")
	return err
}

// Ready marks pull request number of r ready for review, or, with draft,
// makes it a draft again.
func Ready(ctx context.Context, dir string, r Repo, number int, draft bool) error {
	args := []string{"pr", "ready", strconv.Itoa(number), "--repo", r.Slug()}
	if draft {
		args = append(args, "--undo")
	}
	_, err := run(ctx, dir, "", args...)
	return err
}

// Close closes pull request number of r, or, with reopen, opens it again.
func Close(ctx context.Context, dir string, r Repo, number int, reopen bool) error {
	cmd := "close"
	if reopen {
		cmd = "reopen"
	}
	_, err := run(ctx, dir, "", "pr", cmd, strconv.Itoa(number), "--repo", r.Slug())
	return err
}

// UpdateBranch brings pull request number of r's branch up to date with
// its base: merges the base into it, or, with rebase, rebases it on it.
func UpdateBranch(ctx context.Context, dir string, r Repo, number int, rebase bool) error {
	args := []string{"pr", "update-branch", strconv.Itoa(number), "--repo", r.Slug()}
	if rebase {
		args = append(args, "--rebase")
	}
	_, err := run(ctx, dir, "", args...)
	return err
}

// Revert opens a pull request that reverts merged pull request id (its
// GraphQL id), and returns its URL. GitHub's mutation, rather than gh pr
// revert, which older gh hasn't.
func Revert(ctx context.Context, dir string, r Repo, id string) (string, error) {
	const mutation = `mutation($id: ID!) { revertPullRequest(input: {pullRequestId: $id}) { revertPullRequest { url } } }`
	out, err := api(ctx, dir, r, "", "graphql", "-f", "query="+mutation, "-f", "id="+id)
	if err != nil {
		return "", err
	}
	var res struct {
		Data struct {
			Revert struct {
				PR struct {
					URL string `json:"url"`
				} `json:"revertPullRequest"`
			} `json:"revertPullRequest"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("gh api graphql: %w", err)
	}
	return res.Data.Revert.PR.URL, nil
}

// DeleteBranch deletes the branch of a pull request that is closed or
// merged, in the repository it is in, as a.HeadRepo names it. It deletes
// none that moved since a was read, nor its base.
func DeleteBranch(ctx context.Context, dir string, r Repo, a Actions) error {
	switch {
	case a.State == "OPEN":
		return errors.New("the pull request is open: close or merge it first")
	case !a.HeadExists:
		return nil
	case a.HeadRepo == "" || a.Head == "":
		return errors.New("the pull request's branch is gone with its repository")
	case !a.CrossRepo && a.Head == a.Base:
		return errors.New("the pull request's branch is its base")
	}
	branch := refPath(a.Head)
	out, err := api(ctx, dir, r, "", "repos/"+a.HeadRepo+"/git/ref/heads/"+branch, "--jq", ".object.sha")
	if err != nil {
		return err
	}
	if sha := strings.TrimSpace(string(out)); a.HeadOID != "" && sha != a.HeadOID {
		return errors.New("the branch has commits the pull request hasn't: delete it yourself, if you mean to")
	}
	_, err = api(ctx, dir, r, "", "-X", "DELETE", "repos/"+a.HeadRepo+"/git/refs/heads/"+branch)
	return err
}

// refPath is a branch's name in a path of GitHub's API: each of its parts
// escaped, its slashes kept.
func refPath(branch string) string {
	parts := strings.Split(branch, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// ApproveRuns approves the workflow runs of r that wait on a maintainer,
// by their ids, and returns how many it approved.
func ApproveRuns(ctx context.Context, dir string, r Repo, ids []int64) (int, error) {
	n := 0
	for _, id := range ids {
		if _, err := api(ctx, dir, r, "", "-X", "POST", "repos/"+r.Owner+"/"+r.Name+"/actions/runs/"+strconv.FormatInt(id, 10)+"/approve"); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
