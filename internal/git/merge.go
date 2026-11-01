package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Merges and rebases of a branch into the worktree's, and the conflicts
// they stop on, resolved a file at a time by one side's version of it.

// Merge merges rev into HEAD, as a commit of its own when noFF, else by a
// fast forward where it can be one. The changes not committed are stashed
// first (--autostash), which git puts back as the merge ends, by its
// commit or aborted, or keeps in the stash when they conflict.
func (r Repo) Merge(ctx context.Context, rev string, noFF bool) (string, error) {
	if err := checkRev(rev); err != nil || rev == "" {
		return "", errors.New("not a revision: " + rev)
	}
	args := []string{"merge", "--no-edit", "--autostash"}
	if noFF {
		args = append(args, "--no-ff")
	}
	return r.write(ctx, "", nil, append(args, rev)...)
}

// Rebase replays HEAD's branch's commits since onto on it. autostash
// stashes the changes not committed first, which git puts back as the
// rebase ends, aborted too, or keeps in the stash when they conflict.
func (r Repo) Rebase(ctx context.Context, onto string, autostash bool) (string, error) {
	if err := checkRev(onto); err != nil || onto == "" {
		return "", errors.New("not a revision: " + onto)
	}
	args := []string{"rebase"}
	if autostash {
		args = append(args, "--autostash")
	}
	return r.write(ctx, "", nil, append(args, onto)...)
}

// Conflict sides: ours, the version of HEAD's, and theirs, of what comes
// into it.
const (
	SideOurs   = "ours"
	SideTheirs = "theirs"
)

// ConflictSides names the sides of the operation in progress, as the user
// knows them: a branch where one is at it, else a short hash. Ours is
// HEAD's: the branch merged into, and in a rebase the one rebased onto;
// theirs, the branch merged, or rebased, or the commit picked or reverted.
func (r Repo) ConflictSides(ctx context.Context) (ours, theirs string) {
	current := func() string {
		if out, err := run(ctx, r.Root, nil, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
			if b := strings.TrimSpace(string(out)); b != "" {
				return b
			}
		}
		return r.refName(ctx, "HEAD")
	}
	read := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(r.GitDir, name))
		return strings.TrimSpace(string(b))
	}
	switch r.operation() {
	case "merge":
		return current(), r.refName(ctx, "MERGE_HEAD")
	case "rebase":
		for _, dir := range []string{"rebase-merge", "rebase-apply"} {
			onto := read(filepath.Join(dir, "onto"))
			if onto == "" {
				continue
			}
			theirs = strings.TrimPrefix(read(filepath.Join(dir, "head-name")), "refs/heads/")
			if theirs == "" || theirs == "detached HEAD" {
				theirs = r.refName(ctx, "REBASE_HEAD")
			}
			return r.refName(ctx, onto), theirs
		}
		return current(), r.refName(ctx, "REBASE_HEAD")
	case "cherry-pick":
		return current(), r.refName(ctx, "CHERRY_PICK_HEAD")
	case "revert":
		return current(), r.refName(ctx, "REVERT_HEAD")
	}
	return "", ""
}

// refName names a commit by a branch at it, local before remote, else its
// short hash; "" for none.
func (r Repo) refName(ctx context.Context, rev string) string {
	if checkRev(rev) != nil {
		return ""
	}
	out, err := run(ctx, r.Root, nil, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	hash := strings.TrimSpace(string(out))
	if err != nil || hash == "" {
		return ""
	}
	if out, err := run(ctx, r.Root, nil, "for-each-ref", "--points-at", hash, "--format=%(refname:short)", "refs/heads", "refs/remotes"); err == nil {
		for _, name := range strings.Fields(string(out)) {
			if !strings.HasSuffix(name, "/HEAD") {
				return name
			}
		}
	}
	if out, err := run(ctx, r.Root, nil, "rev-parse", "--short", hash); err == nil {
		return strings.TrimSpace(string(out))
	}
	return hash[:min(len(hash), 7)]
}

// ResolveSide resolves a file's conflict by one side's version of it,
// SideOurs or SideTheirs, staged: the file as that side has it, or gone
// where that side deleted it.
func (r Repo) ResolveSide(ctx context.Context, path, side string) error {
	if _, err := r.local(path); err != nil {
		return err
	}
	stage := ""
	switch side {
	case SideOurs:
		stage = "2"
	case SideTheirs:
		stage = "3"
	default:
		return errors.New("no side " + side)
	}
	out, err := run(ctx, r.Root, nil, "ls-files", "-u", "-z", "--", literal(path))
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return errors.New(path + " has no conflict")
	}
	has := false
	for _, e := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		meta, _, _ := strings.Cut(e, "\t")
		if f := strings.Fields(meta); len(f) == 3 && f[2] == stage {
			has = true
		}
	}
	if !has {
		_, err = r.write(ctx, "", nil, pathArgs([]string{"rm", "-q"}, []string{path})...)
		return err
	}
	if _, err := r.write(ctx, "", nil, pathArgs([]string{"checkout", "--" + side}, []string{path})...); err != nil {
		return err
	}
	_, err = r.write(ctx, "", nil, pathArgs([]string{"add"}, []string{path})...)
	return err
}
