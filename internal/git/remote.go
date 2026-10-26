package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// RemoteURL returns the URL a remote fetches from.
func (r Repo) RemoteURL(ctx context.Context, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "-") {
		return "", errors.New("not a remote: " + name)
	}
	out, err := run(ctx, r.Root, nil, "remote", "get-url", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RemoteDefault returns the branch a remote's HEAD is on, as the last
// fetch left it, else main or master of those the remote has; "" for
// none.
func (r Repo) RemoteDefault(remote string) string {
	if b := r.remoteHead(remote); b != "" {
		return b
	}
	for _, name := range []string{"main", "master"} {
		if r.hasRef("refs/remotes/" + remote + "/" + name) {
			return name
		}
	}
	return ""
}

// FetchRef fetches a ref of a remote into a ref of the repository's,
// over what it was: src as the remote names it, dst as
// refs/remotes/<remote>/<name>.
func (r Repo) FetchRef(ctx context.Context, remote, src, dst string) error {
	for _, s := range []string{remote, src, dst} {
		if s == "" || strings.HasPrefix(s, "-") || strings.ContainsAny(s, " :\x00\n~^?*[\\") {
			return errors.New("not a ref to fetch: " + s)
		}
	}
	_, err := r.write(ctx, "", nil, "fetch", "--no-tags", remote, "+"+src+":"+dst)
	return err
}

// remoteHead returns the branch a remote's HEAD is on, as the last fetch
// left it, "" when unknown. It reads the ref's file, running nothing.
func (r Repo) remoteHead(remote string) string {
	b, err := os.ReadFile(filepath.Join(r.Common, "refs", "remotes", remote, "HEAD"))
	if err != nil {
		return ""
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "ref: refs/remotes/"+remote+"/")
	if !ok {
		return ""
	}
	return ref
}

// MergeBase returns the commit where a and b's histories meet.
func (r Repo) MergeBase(ctx context.Context, a, b string) (string, error) {
	if err := checkRev(a); err != nil {
		return "", err
	}
	if err := checkRev(b); err != nil {
		return "", err
	}
	out, err := run(ctx, r.Root, nil, "merge-base", a, b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
// git push pushes it, as branch.<name>.pushRemote and push.default say;
// else to its name on its push remote, origin when it has none, which
// becomes its upstream, as it does of a branch with none. Never to an
// upstream of another name that push.default doesn't push to: a branch
// made from origin/main tracks it, and pushing it there would push it
// over main. Named, the push is of the branch alone, where
// push.default=matching would push every branch of the remote's names.
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
	remote, ref, setUpstream, err := r.pushTarget(ctx, branch)
	if err != nil {
		return nil, err
	}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	return append(args, remote, "refs/heads/"+branch+":"+ref), nil
}

// PushRemote returns the remote a push of branch goes to, and the branch
// it is there, as PushArgs pushes it.
func (r Repo) PushRemote(ctx context.Context, branch string) (remote, name string, err error) {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return "", "", errors.New("HEAD is on no branch to push")
	}
	remote, ref, _, err := r.pushTarget(ctx, branch)
	return remote, strings.TrimPrefix(ref, "refs/heads/"), err
}

// pushTarget returns where PushArgs pushes branch: the remote and the ref
// there, and whether the push makes it branch's upstream, as a branch
// with none takes the one it's pushed to.
func (r Repo) pushTarget(ctx context.Context, branch string) (remote, ref string, setUpstream bool, err error) {
	out, err := run(ctx, r.Root, nil, "for-each-ref", "--format=%(push:remotename)%00%(push:remoteref)%00%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	if err != nil {
		return "", "", false, err
	}
	var pushRemote, pushRef, upstream string
	if f := strings.Split(strings.TrimSpace(string(out)), "\x00"); len(f) == 4 {
		pushRemote, pushRef, upstream = f[0], f[1], f[2]
	}
	if strings.HasPrefix(pushRemote, "-") {
		pushRemote = ""
	}
	if pushRemote != "" && pushRef != "" {
		return pushRemote, pushRef, upstream == "", nil
	}
	remote = pushRemote
	if remote == "" {
		remotes, err := r.Remotes(ctx)
		if err != nil {
			return "", "", false, err
		}
		if len(remotes) == 0 {
			return "", "", false, errors.New("the repository has no remote to push to")
		}
		remote = remotes[0]
		if slices.Contains(remotes, "origin") {
			remote = "origin"
		}
	}
	return remote, "refs/heads/" + branch, true, nil
}
