package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"kero/internal/github"
	"kero/internal/proto"
)

// A pull request's actions, as its page and the list offer them: merging
// it, marking it ready or a draft, closing or reopening it, deleting or
// updating its branch, auto-merge, reverting it and approving a fork's
// workflow runs, with gh as the user signed in to it; and what they
// depend on, read after each.

// prAction does p.Op to a pull request, or, with none, reads what its
// actions depend on.
func (d *Daemon) prAction(p proto.PRActionParams) (proto.PRActionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var res proto.PRActionResult
	if p.Number <= 0 {
		return res, errors.New("which pull request? give its number")
	}
	t, err := d.reviewTargetOf(ctx, proto.PRReviewParams{Pane: p.Pane, Remote: p.Remote, Number: p.Number})
	if err != nil {
		return res, err
	}
	dir, repo, n := t.r.Root, t.repo, p.Number
	switch p.Op {
	case proto.PRActState:
	case proto.PRActMerge, proto.PRActAutoMerge:
		err = github.MergePR(ctx, dir, repo, n, github.Merge{Method: p.Method, Auto: p.Op == proto.PRActAutoMerge, DeleteBranch: p.DeleteBranch, Head: p.Head})
	case proto.PRActDisableAutoMerge:
		err = github.DisableAutoMerge(ctx, dir, repo, n)
	case proto.PRActReady, proto.PRActDraft:
		err = github.Ready(ctx, dir, repo, n, p.Op == proto.PRActDraft)
	case proto.PRActClose, proto.PRActReopen:
		err = github.Close(ctx, dir, repo, n, p.Op == proto.PRActReopen)
	case proto.PRActUpdateBranch:
		err = github.UpdateBranch(ctx, dir, repo, n, p.Method == github.MethodRebase)
	case proto.PRActDeleteBranch, proto.PRActRevert, proto.PRActApproveRuns:
		// These act on the pull request as GitHub has it now: its id, its
		// branch as it is, the runs that still wait.
		var a github.Actions
		if a, err = github.ActionsOf(ctx, dir, repo, n); err != nil {
			return res, err
		}
		switch p.Op {
		case proto.PRActDeleteBranch:
			err = github.DeleteBranch(ctx, dir, repo, a)
		case proto.PRActRevert:
			if a.State != "MERGED" {
				return res, errors.New("only a merged pull request can be reverted")
			}
			res.URL, err = github.Revert(ctx, dir, repo, a.ID)
		default:
			if p.Head != "" && a.HeadOID != p.Head {
				return res, errors.New("the pull request has new commits since: look at them before approving its runs")
			}
			res.Approved, err = github.ApproveRuns(ctx, dir, repo, a.PendingRuns)
		}
	default:
		return res, fmt.Errorf("no pull request action %q", p.Op)
	}
	if err != nil {
		return res, err
	}
	// What the actions depend on now, which the page shows at once. An
	// action done whose state can't be read is done all the same.
	a, err := github.ActionsOf(ctx, dir, repo, n)
	if err != nil {
		if p.Op == proto.PRActState {
			return res, err
		}
		return res, nil
	}
	res.State = &proto.PRActions{State: a.State, Draft: a.Draft, Head: a.Head, HeadOID: a.HeadOID, Base: a.Base, CrossRepo: a.CrossRepo,
		HeadExists: a.HeadExists, Mergeable: a.Mergeable, MergeState: a.MergeState, AutoMerge: a.AutoMerge,
		Methods: append([]string{}, a.Methods...), AutoMergeAllowed: a.AutoMergeAllowed, DeleteOnMerge: a.DeleteOnMerge,
		CanWrite: a.CanWrite, CanUpdate: a.CanUpdate, CanUpdateBranch: a.CanUpdateBranch, CanDeleteBranch: a.CanDeleteBranch,
		PendingRuns: len(a.PendingRuns)}
	return res, nil
}
