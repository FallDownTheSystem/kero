package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Branch is a local branch, a remote's, or a tag.
type Branch struct {
	Name    string // as git shows it: main, origin/main, v1
	Remote  bool
	Tag     bool
	Current bool   // HEAD is on it
	Head    string // its commit, an annotated tag's too
	// Upstream is the branch a local one follows, and Ahead and Behind how
	// far it is from it; Gone, that the upstream is no more.
	Upstream string
	Ahead    int
	Behind   int
	Gone     bool
	Time     int64 // its commit's, in seconds
	Subject  string
	// Worktree is the worktree that has it checked out, "" for none.
	Worktree string
}

// Branches returns the repository's local branches, then its remotes',
// then its tags, each by its last commit, a tag by when it was made,
// newest first.
func (r Repo) Branches(ctx context.Context) ([]Branch, error) {
	// An annotated tag's object, subject and date are its own, its
	// commit's the dereferenced (*) fields; creatordate is a commit's
	// committer date, an annotated tag's tagger date.
	format := strings.Join([]string{"%(HEAD)", "%(refname)", "%(refname:short)", "%(objectname)", "%(upstream:short)",
		"%(upstream:track,nobracket)", "%(creatordate:unix)", "%(worktreepath)", "%(*objectname)", "%(*subject)", "%(subject)"}, "%00")
	out, err := run(ctx, r.Root, nil, "for-each-ref", "--sort=-creatordate", "--format="+format, "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	var local, remote, tags []Branch
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\x00")
		if len(f) != 11 {
			continue
		}
		b := Branch{Current: f[0] == "*", Name: f[2], Head: f[3], Upstream: f[4], Subject: f[10], Worktree: f[7]}
		if f[8] != "" {
			b.Head, b.Subject = f[8], f[9]
		}
		b.Time, _ = strconv.ParseInt(f[6], 10, 64)
		if strings.HasPrefix(f[1], "refs/tags/") {
			b.Tag = true
			tags = append(tags, b)
			continue
		}
		for _, part := range strings.Split(f[5], ",") {
			part = strings.TrimSpace(part)
			switch {
			case part == "gone":
				b.Gone = true
			case strings.HasPrefix(part, "ahead "):
				b.Ahead, _ = strconv.Atoi(part[len("ahead "):])
			case strings.HasPrefix(part, "behind "):
				b.Behind, _ = strconv.Atoi(part[len("behind "):])
			}
		}
		if strings.HasPrefix(f[1], "refs/remotes/") {
			// A remote's HEAD names its default branch, which is listed
			// itself.
			if strings.HasSuffix(f[1], "/HEAD") {
				continue
			}
			b.Remote = true
			remote = append(remote, b)
			continue
		}
		local = append(local, b)
	}
	return slices.Concat(local, remote, tags), nil
}

// checkName refuses what can't name a branch, or git would take as an
// option.
func checkName(ctx context.Context, dir, name string) error {
	if strings.HasPrefix(name, "-") {
		return errors.New("“" + name + "” can't name a branch")
	}
	return CheckBranch(ctx, dir, name)
}

// Switch checks out a branch: a remote's as a new local branch that
// follows it, unless one of its name is there, which it checks out. at,
// when set, is the commit the branch is to be at, as the commit chosen in
// the history: a local branch of the remote's name elsewhere is refused,
// rather than checked out at another commit.
func (r Repo) Switch(ctx context.Context, name string, remote bool, at string) (string, error) {
	if strings.HasPrefix(name, "-") {
		return "", errors.New("no branch " + name)
	}
	if remote {
		_, local, ok := strings.Cut(name, "/")
		if ok && r.HasBranch(local) {
			if at != "" {
				out, err := run(ctx, r.Root, nil, "rev-parse", "--verify", "--quiet", "refs/heads/"+local)
				if err != nil || strings.TrimSpace(string(out)) != at {
					return "", errors.New("the branch " + local + " is at another commit than " + name + ": switch to it from the branches, or check out the commit")
				}
			}
			return r.write(ctx, "", nil, "switch", local)
		}
		return r.write(ctx, "", nil, "switch", "--track", name)
	}
	return r.write(ctx, "", nil, "switch", name)
}

// CreateBranch makes a branch at start, HEAD when "", and checks it out
// when checkout is set.
func (r Repo) CreateBranch(ctx context.Context, name, start string, checkout bool) (string, error) {
	if err := checkName(ctx, r.Root, name); err != nil {
		return "", err
	}
	if err := checkRev(start); err != nil {
		return "", err
	}
	args := []string{"branch", name}
	if checkout {
		args = []string{"switch", "-c", name}
	}
	if start != "" {
		args = append(args, start)
	}
	return r.write(ctx, "", nil, args...)
}

// DeleteBranch deletes a local branch: one whose commits are elsewhere
// too, unless force.
func (r Repo) DeleteBranch(ctx context.Context, name string, force bool) (string, error) {
	if strings.HasPrefix(name, "-") {
		return "", errors.New("no branch " + name)
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	return r.write(ctx, "", nil, "branch", flag, name)
}

// RenameBranch renames a local branch.
func (r Repo) RenameBranch(ctx context.Context, name, to string) (string, error) {
	if strings.HasPrefix(name, "-") {
		return "", errors.New("no branch " + name)
	}
	if err := checkName(ctx, r.Root, to); err != nil {
		return "", err
	}
	return r.write(ctx, "", nil, "branch", "-m", name, to)
}

// Stash is an entry of the stash.
type Stash struct {
	Index   int    // n of stash@{n}
	Hash    string // its commit, which tells it from the one at Index later
	Time    int64
	Message string // as git stash list says it: "On main: wip"
	Base    string // the commit it was made on
}

// Stashes returns the stash, its newest first.
func (r Repo) Stashes(ctx context.Context) ([]Stash, error) {
	out, err := run(ctx, r.Root, nil, "stash", "list", "--format=%gd%x1f%H%x1f%ct%x1f%P%x1f%gs%x1e")
	if err != nil {
		return nil, err
	}
	var list []Stash
	for _, rec := range strings.Split(string(out), "\x1e") {
		f := strings.Split(strings.TrimSpace(rec), "\x1f")
		if len(f) != 5 {
			continue
		}
		s := Stash{Hash: f[1], Message: f[4]}
		s.Base, _, _ = strings.Cut(f[3], " ")
		fmt.Sscanf(f[0], "stash@{%d}", &s.Index)
		s.Time, _ = strconv.ParseInt(f[2], 10, 64)
		list = append(list, s)
	}
	return list, nil
}

// StashOptions are what a stash takes: the untracked files too, or what's
// staged alone, and a message.
type StashOptions struct {
	Message   string
	Untracked bool
	Staged    bool
}

// StashPush stashes the worktree's changes, leaving it as HEAD has it.
func (r Repo) StashPush(ctx context.Context, o StashOptions) (string, error) {
	args := []string{"stash", "push"}
	switch {
	case o.Staged:
		args = append(args, "--staged")
	case o.Untracked:
		args = append(args, "--include-untracked")
	}
	if m := strings.TrimSpace(o.Message); m != "" {
		args = append(args, "-m", m)
	}
	return r.write(ctx, "", nil, args...)
}

// ErrStashKept: the stash's changes are back, but not what of them was
// staged, which only the stash still has.
var ErrStashKept = errors.New("applied without what was staged, which the index couldn't take back: the stash is kept")

// StashApply applies the stash's entry of hash, what was staged staged
// again, and drops it when pop is set, once it applied whole: a pop
// whose staged changes don't apply keeps it (ErrStashKept), as one with
// conflicts does. StashDrop drops it.
func (r Repo) StashApply(ctx context.Context, index int, hash string, pop bool) (string, error) {
	if _, err := r.stashRef(ctx, index, hash); err != nil {
		return "", err
	}
	// By its commit, which stays the entry's however the stash moves.
	out, err := r.write(ctx, "", nil, "stash", "apply", "--index", hash)
	if err != nil {
		if !strings.Contains(out, "--index") {
			return out, err
		}
		// Git refused before it changed anything: the index has changes
		// the stash's staged ones don't apply on.
		if out, err = r.write(ctx, "", nil, "stash", "apply", hash); err != nil {
			return out, err
		}
		if pop {
			return out, ErrStashKept
		}
		return out, nil
	}
	if !pop {
		return out, nil
	}
	drop, err := r.StashDrop(ctx, -1, hash)
	return strings.TrimSpace(out + "\n" + drop), err
}

// StashDrop drops the stash's entry of hash: at index, or, for -1, where it
// is now.
func (r Repo) StashDrop(ctx context.Context, index int, hash string) (string, error) {
	if index < 0 {
		list, err := r.Stashes(ctx)
		if err != nil {
			return "", err
		}
		i := slices.IndexFunc(list, func(s Stash) bool { return s.Hash == hash })
		if i < 0 {
			return "", errors.New("the stash changed: look at it again")
		}
		index = list[i].Index
	}
	ref, err := r.stashRef(ctx, index, hash)
	if err != nil {
		return "", err
	}
	return r.write(ctx, "", nil, "stash", "drop", ref)
}

// stashRef names the stash's entry index, when it is still hash: another
// stash since moves the entries along.
func (r Repo) stashRef(ctx context.Context, index int, hash string) (string, error) {
	if err := checkRev(hash); err != nil || hash == "" {
		return "", errors.New("no stash " + hash)
	}
	ref := fmt.Sprintf("stash@{%d}", index)
	out, err := run(ctx, r.Root, nil, "rev-parse", "--verify", "--quiet", ref)
	if err != nil || strings.TrimSpace(string(out)) != hash {
		return "", errors.New("the stash changed: look at it again")
	}
	return ref, nil
}

// Commit is a commit of the log.
type Commit struct {
	Hash    string
	Parents []string
	Author  string
	Email   string
	Time    int64  // the author's, in seconds
	Refs    string // its refs, as %D prints them: "HEAD -> main, origin/main, tag: v1"
	Subject string
}

// LogOptions say which commits Log lists.
type LogOptions struct {
	// Revs are the commits whose history to list, as HEAD and its
	// upstream; All lists every branch's, remotes' and tag's instead.
	Revs        []string
	All         bool
	FirstParent bool
	Skip, Max   int
	// Grep and Author match the message and the author, ignoring case;
	// Path lists the commits that changed it, following its renames.
	Grep   string
	Author string
	Path   string
}

// ShownRevs are the revisions whose history a branch's shows: HEAD, its
// upstream, and the default branch, origin's when it has one, as far as
// they are there.
func (r Repo) ShownRevs(ctx context.Context, upstream bool) []string {
	revs := []string{"HEAD"}
	if upstream {
		revs = append(revs, "@{upstream}")
	}
	if def := r.DefaultBranch(); def != "" {
		for _, ref := range []string{"refs/remotes/origin/" + def, "refs/heads/" + def} {
			if r.hasRef(ref) {
				revs = append(revs, ref)
				break
			}
		}
	}
	return revs
}

const logFormat ="--format=%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%D%x1f%s%x1e"

// Log returns commits in the order git log --graph draws them: each
// before its parents.
func (r Repo) Log(ctx context.Context, o LogOptions) ([]Commit, error) {
	args := []string{"log", logFormat, "--decorate=short", "--decorate-refs-exclude=refs/stash"}
	if o.Path == "" {
		args = append(args, "--topo-order")
	}
	if o.FirstParent {
		args = append(args, "--first-parent")
	}
	if o.Skip > 0 {
		args = append(args, "--skip="+strconv.Itoa(o.Skip))
	}
	if o.Max > 0 {
		args = append(args, "-n", strconv.Itoa(o.Max))
	}
	if o.Grep != "" {
		args = append(args, "-i", "--fixed-strings", "--grep="+o.Grep)
	}
	if o.Author != "" {
		args = append(args, "-i", "--fixed-strings", "--author="+o.Author)
	}
	if o.All {
		args = append(args, "--branches", "--remotes", "--tags", "HEAD")
	} else {
		for _, rev := range o.Revs {
			if err := checkRev(rev); err != nil {
				return nil, err
			}
		}
		args = append(args, o.Revs...)
	}
	if o.Path != "" {
		if _, err := r.local(o.Path); err != nil {
			return nil, err
		}
		args = append(args, "--follow", "--", o.Path)
	} else {
		args = append(args, "--")
	}
	out, err := run(ctx, r.Root, nil, args...)
	if err != nil {
		if !r.hasHead(ctx) {
			return nil, nil // no commit yet
		}
		return nil, err
	}
	return parseLog(out), nil
}

func parseLog(out []byte) []Commit {
	var list []Commit
	for _, rec := range strings.Split(string(out), "\x1e") {
		f := strings.Split(strings.TrimLeft(rec, "\r\n"), "\x1f")
		if len(f) != 7 {
			continue
		}
		c := Commit{Hash: f[0], Parents: strings.Fields(f[1]), Author: f[2], Email: f[3], Refs: f[5], Subject: f[6]}
		c.Time, _ = strconv.ParseInt(f[4], 10, 64)
		list = append(list, c)
	}
	return list
}

// Unpushed returns the commits of HEAD that its upstream hasn't, or no
// remote's branch when it has none, up to max.
func (r Repo) Unpushed(ctx context.Context, upstream bool, max int) ([]string, error) {
	args := []string{"rev-list", "--max-count=" + strconv.Itoa(max), "HEAD", "--not"}
	if upstream {
		args = append(args, "@{upstream}")
	} else {
		args = append(args, "--remotes")
	}
	out, err := run(ctx, r.Root, nil, args...)
	if err != nil {
		if !r.hasHead(ctx) {
			return nil, nil
		}
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// CommitInfo is a commit as its view shows it: its message whole, and the
// files it changed since its first parent.
type CommitInfo struct {
	Commit
	Committer string
	Message   string
	Files     []File
	// Untracked are the files of a stash's untracked files, its third
	// parent, which its own diff leaves out.
	Untracked []File
}

// Show returns a commit, a stash's too.
func (r Repo) Show(ctx context.Context, rev string) (CommitInfo, error) {
	if err := checkRev(rev); err != nil {
		return CommitInfo{}, err
	}
	out, err := run(ctx, r.Root, nil, "show", "-s", "--format=%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%D%x1f%s%x1f%cn%x1f%B", "--decorate=short", rev, "--")
	if err != nil {
		return CommitInfo{}, err
	}
	f := strings.SplitN(string(out), "\x1f", 9)
	if len(f) != 9 {
		return CommitInfo{}, errors.New("git show: no commit " + rev)
	}
	ci := CommitInfo{Commit: Commit{Hash: f[0], Parents: strings.Fields(f[1]), Author: f[2], Email: f[3], Refs: f[5], Subject: f[6]},
		Committer: f[7], Message: strings.TrimSpace(f[8])}
	ci.Time, _ = strconv.ParseInt(f[4], 10, 64)
	from, err := r.Parent(ctx, ci.Commit)
	if err != nil {
		return CommitInfo{}, err
	}
	if ci.Files, err = r.diffFiles(ctx, "-M", from, ci.Hash); err != nil {
		return CommitInfo{}, err
	}
	if r.isStash(ctx, ci.Hash) && len(ci.Parents) == 3 {
		empty, err := r.emptyTree(ctx)
		if err != nil {
			return CommitInfo{}, err
		}
		if ci.Untracked, err = r.diffFiles(ctx, empty, ci.Parents[2]); err != nil {
			return CommitInfo{}, err
		}
	}
	return ci, nil
}

// Parent returns what a commit's diff is from: its first parent, else the
// empty tree.
func (r Repo) Parent(ctx context.Context, c Commit) (string, error) {
	if len(c.Parents) > 0 {
		return c.Parents[0], nil
	}
	return r.emptyTree(ctx)
}

// isStash reports whether a commit is one of the stash's.
func (r Repo) isStash(ctx context.Context, hash string) bool {
	list, err := r.Stashes(ctx)
	return err == nil && slices.ContainsFunc(list, func(s Stash) bool { return s.Hash == hash })
}
