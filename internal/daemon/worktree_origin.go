package daemon

import (
	"context"
	"regexp"
	"time"

	"kero/internal/git"
)

// originFetchTimeout bounds the fetch of a new worktree's base: offline,
// or against a remote that doesn't answer, the session starts from the
// local ref past it rather than wait.
const originFetchTimeout = 20 * time.Second

// forkPRRef is the name fetchPR gives a fork's pull request, fetched from
// the remote's refs/pull/N/head just before: no branch of the remote's, so
// fetching refs/heads/pr-N of it would fail, or fetch another branch.
var forkPRRef = regexp.MustCompile(`^pr-[0-9]+$`)

// originFetch is what fromOrigin fetched, as a worktree's setup shows it:
// Ref, "" for nothing, and Err, why the fetch failed.
type originFetch struct {
	Ref string
	Err error
}

// fromOrigin returns what a new worktree's branch starts from in place of
// from, with the worktree-from-origin setting on: a local branch's remote
// copy, fetched fresh, unless the local branch has commits it lacks; a
// remote's branch, fetched fresh, as it is. Else, as when the fetch fails,
// from. untrack is whether the new branch is to track nothing, as from a
// local branch it would.
func (d *Daemon) fromOrigin(ctx context.Context, r git.Repo, from string) (base string, untrack bool, f originFetch) {
	local := r.HasBranch(from)
	var remote, name string
	if local {
		remote, name = r.TrackedRemote(ctx, from)
	} else if rm, n, ok := r.RemoteOf(ctx, from); ok && !forkPRRef.MatchString(n) {
		remote, name = rm, n
	}
	if remote == "" {
		return from, false, f
	}
	f.Ref = remote + "/" + name
	ref := "refs/remotes/" + remote + "/" + name
	fctx, cancel := context.WithTimeout(ctx, originFetchTimeout)
	err := r.FetchRef(fctx, remote, "refs/heads/"+name, ref)
	cancel()
	if err != nil {
		d.log.Printf("fetching %s/%s for a new worktree, starting from %s: %v", remote, name, from, err)
		f.Err = err
		return from, false, f
	}
	if !local {
		return from, false, f
	}
	if !r.IsAncestor(ctx, "refs/heads/"+from, ref) {
		d.log.Printf("%s has commits %s/%s lacks: a new worktree starts from it", from, remote, name)
		return from, false, f
	}
	return remote + "/" + name, true, f
}
