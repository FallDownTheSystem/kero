package git

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// Remotes returns the names of the repository's remotes.
func (r Repo) Remotes(ctx context.Context) ([]string, error) {
	out, err := run(ctx, r.Root, nil, "remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// FetchOptions are a fetch's: every remote's, or the upstream's alone,
// and whether the remotes' branches that went go too.
type FetchOptions struct {
	All   bool
	Prune bool
}

// Fetch fetches, progress getting each line git prints.
func (r Repo) Fetch(ctx context.Context, o FetchOptions, progress func(string)) (string, error) {
	return r.write(ctx, "", progress, append(FetchArgs(o), "--progress")...)
}

// FetchArgs are git's arguments of a fetch, as a terminal runs it too:
// these, and those of PullArgs and PushArgs, are how git is run, and
// the command shown to run again in a terminal.
func FetchArgs(o FetchOptions) []string {
	args := []string{"fetch"}
	if o.All {
		args = append(args, "--all")
	}
	if o.Prune {
		args = append(args, "--prune")
	}
	return args
}

// Pull modes: as the user's configuration says, merge, rebase, or a fast
// forward alone.
const (
	PullDefault = ""
	PullMerge   = "merge"
	PullRebase  = "rebase"
	PullFF      = "ff-only"
)

// Pull pulls the upstream's commits into HEAD.
func (r Repo) Pull(ctx context.Context, mode string, progress func(string)) (string, error) {
	args, err := PullArgs(mode)
	if err != nil {
		return "", err
	}
	return r.write(ctx, "", progress, append(args, "--progress")...)
}

// PullArgs are git's arguments of a pull, as a terminal runs it too: the
// changes not committed stashed first and put back after (--autostash),
// which a rebase would refuse to pull over, and a merge where they touch
// its files; kept in the stash when they conflict.
func PullArgs(mode string) ([]string, error) {
	args := []string{"pull", "--autostash"}
	switch mode {
	case PullDefault:
	case PullMerge:
		args = append(args, "--no-rebase")
	case PullRebase:
		args = append(args, "--rebase")
	case PullFF:
		args = append(args, "--ff-only")
	default:
		return nil, errors.New("no pull " + mode)
	}
	return args, nil
}

// PushOptions are a push's: over the remote's branch where it has commits
// HEAD hasn't, as long as it is as last fetched (--force-with-lease), and
// with the tags of the commits pushed.
type PushOptions struct {
	Force bool
	Tags  bool
}

// Push runs a push of PushArgs's, progress getting each line git prints.
func (r Repo) Push(ctx context.Context, args []string, progress func(string)) (string, error) {
	return r.write(ctx, "", progress, append(slices.Clone(args), "--progress")...)
}

// PushArgs are git's arguments of a push of HEAD's branch alone: to where
// it pushes to, as branch.<name>.pushRemote and push.default say, else to
// its upstream; a branch with none to a remote, origin when it has one,
// which becomes its upstream. Named, the push is of the branch alone,
// where push.default=matching would push every branch of the remote's
// names.
func (r Repo) PushArgs(ctx context.Context, branch string, o PushOptions) ([]string, error) {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return nil, errors.New("HEAD is on no branch to push")
	}
	args := []string{"push"}
	if o.Force {
		args = append(args, "--force-with-lease", "--force-if-includes")
	}
	if o.Tags {
		args = append(args, "--follow-tags")
	}
	out, err := run(ctx, r.Root, nil, "for-each-ref", "--format=%(push:remotename)%00%(push:remoteref)%00%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	if err != nil {
		return nil, err
	}
	f := strings.Split(strings.TrimSpace(string(out)), "\x00")
	if len(f) == 4 {
		for _, to := range [][2]string{{f[0], f[1]}, {f[2], f[3]}} {
			if to[0] != "" && to[1] != "" && !strings.HasPrefix(to[0], "-") {
				return append(args, to[0], "refs/heads/"+branch+":"+to[1]), nil
			}
		}
	}
	remotes, err := r.Remotes(ctx)
	if err != nil {
		return nil, err
	}
	if len(remotes) == 0 {
		return nil, errors.New("the repository has no remote to push to")
	}
	remote := remotes[0]
	if slices.Contains(remotes, "origin") {
		remote = "origin"
	}
	return append(args, "--set-upstream", remote, branch), nil
}
