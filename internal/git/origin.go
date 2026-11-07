package git

import (
	"context"
	"slices"
	"strings"
)

// TrackedRemote returns the remote and the branch there that a local
// branch is the copy of: its upstream, when it tracks a remote's branch,
// else origin's branch of its name; "", "" when the repository has no
// origin.
func (r Repo) TrackedRemote(ctx context.Context, branch string) (remote, name string) {
	out, err := run(ctx, r.Root, nil, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	if err == nil {
		f := strings.Split(strings.TrimSpace(string(out)), "\x00")
		if len(f) == 2 && f[0] != "" && f[0] != "." && !strings.HasPrefix(f[0], "-") {
			if n, ok := strings.CutPrefix(f[1], "refs/heads/"); ok {
				return f[0], n
			}
		}
	}
	if remotes, err := r.Remotes(ctx); err == nil && slices.Contains(remotes, "origin") {
		return "origin", branch
	}
	return "", ""
}

// RemoteOf splits ref, as origin/main, into a remote of the repository
// and the branch there; ok false when it names no remote's branch.
func (r Repo) RemoteOf(ctx context.Context, ref string) (remote, name string, ok bool) {
	remotes, err := r.Remotes(ctx)
	if err != nil {
		return "", "", false
	}
	for _, rm := range remotes {
		if n, found := strings.CutPrefix(ref, rm+"/"); found && n != "" {
			return rm, n, true
		}
	}
	return "", "", false
}

// IsAncestor reports whether b has every commit of a.
func (r Repo) IsAncestor(ctx context.Context, a, b string) bool {
	_, err := run(ctx, r.Root, nil, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// UnsetUpstream makes branch track nothing, as git's branching from a
// remote's branch had it track that.
func (r Repo) UnsetUpstream(ctx context.Context, branch string) error {
	_, err := r.write(ctx, "", nil, "branch", "--unset-upstream", branch)
	return err
}
