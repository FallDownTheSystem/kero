package daemon

import (
	"context"
	"fmt"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
)

// prCheckout checks a pull request out on a branch of its name, as gh pr
// checkout does: in the pane's worktree, when p.Here, which it refuses
// with changes the switch could lose or carry over; else it readies the
// branch for a session's new worktree, which addWorktree makes. A worktree
// that has the pull request already is where it is instead: one on its
// branch, as git checks a branch out in one worktree alone; and, for a
// new one, one on a branch that pushes to the pull request's, or a
// review's of it.
func (d *Daemon) prCheckout(p proto.PRCheckoutParams) (proto.PRCheckedOut, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.PRCheckedOut{}, err
	}
	if p.Here {
		st, err := r.Status(ctx)
		switch {
		case err != nil:
			return proto.PRCheckedOut{}, err
		case st.State != "":
			return proto.PRCheckedOut{}, fmt.Errorf("a %s is in progress here: finish or abort it first", st.State)
		case st.Files > 0:
			return proto.PRCheckedOut{}, fmt.Errorf("the working tree has %d changed files: commit or stash them, or check the pull request out in a new worktree", st.Files)
		}
	}
	branch, err := fetchPR(ctx, r, p.Remote, p.Number, p.Head, p.CrossRepo)
	if err != nil {
		return proto.PRCheckedOut{}, err
	}
	got := proto.PRCheckedOut{Branch: branch, Base: p.Remote + "/" + branch}
	// A fork's branch isn't the remote's to push to.
	if branch == p.Head && !p.CrossRepo {
		got.PushRemote, got.PushBranch = p.Remote, branch
	}
	cos, err := r.Checkouts(ctx)
	if err != nil {
		return proto.PRCheckedOut{}, err
	}
	if wt, ok := prWorktree(ctx, r, cos, got, p.Number, p.Here); ok {
		return wt, nil
	}
	if !p.Here {
		// A branch of the name there already, which the worktree checks
		// out as it is: brought up to the pull request's first.
		if r.HasBranch(branch) {
			if err := r.FastForward(ctx, branch, "refs/remotes/"+got.Base); err != nil {
				d.log.Printf("fast-forwarding %s to %s: %v", branch, got.Base, err)
			}
		}
		return got, nil
	}
	// The branch, made to follow the remote's; or, there already, brought
	// up to it when behind it, else left as it is, with commits of its own.
	if _, err := r.Switch(ctx, got.Base, true, ""); err != nil {
		return proto.PRCheckedOut{}, err
	}
	d.log.Printf("pull request #%d checked out on %s in %s", p.Number, branch, r.Root)
	return got, nil
}

// prWorktree returns a worktree of cos, r's, that has the pull request
// number checked out on got.Branch, other than r's own when here, which
// checks it out there anew. For a new worktree (!here), r's own does, and
// so does one on a branch pushing to the pull request's, or a review's.
func prWorktree(ctx context.Context, r git.Repo, cos []git.Checkout, got proto.PRCheckedOut, number int, here bool) (proto.PRCheckedOut, bool) {
	for _, co := range cos {
		if co.Branch == got.Branch && (!here || !samePath(co.Path, r.Root)) {
			return proto.PRCheckedOut{Branch: co.Branch, Worktree: co.Path}, true
		}
	}
	if here {
		return proto.PRCheckedOut{}, false
	}
	for _, co := range cos {
		if co.Branch == "" || co.Prunable {
			continue
		}
		if got.PushBranch != "" {
			if remote, name, err := r.PushRemote(ctx, co.Branch); err == nil && remote == got.PushRemote && name == got.PushBranch {
				return proto.PRCheckedOut{Branch: co.Branch, Worktree: co.Path}, true
			}
		}
		if m := reviewBranchRe.FindStringSubmatch(co.Branch); m != nil && m[1] == fmt.Sprint(number) {
			return proto.PRCheckedOut{Branch: co.Branch, Worktree: co.Path}, true
		}
	}
	return proto.PRCheckedOut{}, false
}
