package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Linked worktrees of a session's own (plans/git.md, Phase 3): what kero
// writes to a repository besides a revert, adding the worktree and its
// branch as a session starts in it, and removing them as it closes.

// Main returns the repository's main checkout, "" for a bare repository's
// or a submodule's worktree, whose git directory is no .git of one.
func (r Repo) Main() string {
	if filepath.Base(r.Common) != ".git" {
		return ""
	}
	return filepath.Dir(r.Common)
}

// Linked reports whether the worktree is a linked one, whose git directory
// is its own under the repository's, not the main checkout.
func (r Repo) Linked() bool { return r.GitDir != r.Common }

// CheckBranch reports whether name may name a new branch.
func CheckBranch(ctx context.Context, dir, name string) error {
	if name == "" || strings.HasPrefix(name, "-") {
		return errors.New("a branch needs a name")
	}
	if _, err := run(ctx, dir, nil, "check-ref-format", "--branch", name); err != nil {
		return errors.New("“" + name + "” can't name a branch")
	}
	return nil
}

// Checkout is a worktree of the repository: where it is, the commit it
// has checked out, and its branch, "" when detached.
type Checkout struct {
	Path   string
	Head   string
	Branch string
	// Prunable: its folder is gone, which git worktree prune forgets;
	// Locked: git keeps it, prune or remove, until it's unlocked.
	Prunable, Locked bool
}

// Checkouts returns the repository's worktrees, the main one first, but
// a bare one's, which has no files.
func (r Repo) Checkouts(ctx context.Context) ([]Checkout, error) {
	out, err := run(ctx, r.Root, nil, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Checkout
	var cur Checkout
	bare := false
	flush := func() {
		if cur.Path != "" && !bare {
			list = append(list, cur)
		}
		cur, bare = Checkout{}, false
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			cur.Path = filepath.FromSlash(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "bare":
			bare = true
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			cur.Prunable = true
		case line == "locked" || strings.HasPrefix(line, "locked "):
			cur.Locked = true
		}
	}
	flush()
	return list, nil
}

// Used returns when HEAD last moved in the worktree at path, in
// milliseconds, 0 when unknown: its reflog's time, which every commit,
// checkout and reset there appends to, and the worktree's adding starts;
// else its HEAD file's. Neither changes as git status refreshes the
// index, which the app does of every worktree it shows.
func Used(path string) int64 {
	admin := filepath.Join(path, ".git")
	if b, err := os.ReadFile(admin); err == nil {
		// A linked worktree's .git is a file naming its git directory.
		dir, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
		if !ok {
			return 0
		}
		admin = filepath.FromSlash(strings.TrimSpace(dir))
		if !filepath.IsAbs(admin) {
			admin = filepath.Join(path, admin)
		}
	}
	for _, f := range []string{filepath.Join(admin, "logs", "HEAD"), filepath.Join(admin, "HEAD")} {
		if st, err := os.Stat(f); err == nil {
			return st.ModTime().UnixMilli()
		}
	}
	return 0
}

// HasBranch reports whether the repository has a local branch name.
func (r Repo) HasBranch(name string) bool { return r.hasRef("refs/heads/" + name) }

// AddWorktree checks out a new branch from base in a linked worktree at
// path; an existing branch, with base "", as it is.
func (r Repo) AddWorktree(ctx context.Context, path, branch, base string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	args := []string{"worktree", "add"}
	if base != "" {
		args = append(args, "-b", branch, "--", path, base)
	} else {
		args = append(args, "--", path, branch)
	}
	_, err := run(ctx, r.Root, nil, args...)
	return err
}

// Exclude adds a pattern to the repository's info/exclude, which ignores
// it as .gitignore would, without a change of the repository's own.
func (r Repo) Exclude(pattern string) error {
	path := filepath.Join(r.Common, "info", "exclude")
	b, _ := os.ReadFile(path)
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == pattern {
			return nil
		}
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	b = append(b, pattern+"\n"...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// WorktreeState is what removing a linked worktree would lose.
type WorktreeState struct {
	// Files changed, untracked ones too, which removing discards.
	Files int
	// Unpushed are the commits of its branch on neither base nor any
	// remote's branch, which deleting the branch would lose; Merged, that
	// base has every commit of the branch.
	Unpushed int
	Merged   bool
}

// State returns what removing the worktree, a linked one on branch from
// base, would lose.
func (r Repo) State(ctx context.Context, branch, base string) (WorktreeState, error) {
	st, err := r.Status(ctx)
	if err != nil {
		return WorktreeState{}, err
	}
	ws := WorktreeState{Files: st.Files}
	if branch == "" || !r.HasBranch(branch) {
		return ws, nil
	}
	ref := "refs/heads/" + branch
	args := []string{"rev-list", "--count", ref, "--not", "--remotes"}
	if base != "" {
		args = append(args, base)
	}
	out, err := run(ctx, r.Root, nil, args...)
	if err != nil {
		return WorktreeState{}, err
	}
	ws.Unpushed, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	if base != "" {
		_, err := run(ctx, r.Root, nil, "merge-base", "--is-ancestor", ref, base)
		ws.Merged = err == nil
	}
	return ws, nil
}

// RemoveWorktree removes the linked worktree at path, what it holds
// included, from the repository whose main checkout is main; then its
// branch, when base has every commit of it. A worktree git forgot, though
// its folder stayed in use, loses its branch too: no worktree has it, and
// it would only take the name of the next.
func RemoveWorktree(ctx context.Context, main, path, branch, base string) error {
	_, forgot, err := removeWorktree(ctx, main, path, func() (string, error) {
		_, err := run(ctx, main, nil, "worktree", "remove", "--force", "--", path)
		return "", err
	})
	if err != nil && !forgot {
		return err
	}
	if branch == "" || base == "" {
		return err
	}
	ref := "refs/heads/" + branch
	if _, merr := run(ctx, main, nil, "merge-base", "--is-ancestor", ref, base); merr != nil {
		return err // not merged: the branch stays, with its commits
	}
	_, berr := run(ctx, main, nil, "branch", "-D", "--", branch)
	return errors.Join(err, berr)
}

// RemoveCheckout removes the linked worktree at path, with force the
// changes it has too, which git otherwise refuses to lose; then, unless
// branch is "", its branch, as git branch -d does: only when merged, so
// that no commit goes with it.
func (r Repo) RemoveCheckout(ctx context.Context, path string, force bool, branch string) (string, error) {
	if p := filepath.Clean(path); p == filepath.Clean(r.Root) || r.Main() != "" && p == filepath.Clean(r.Main()) {
		return "", errors.New("the worktree kero works in, or the main one, stays")
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	out, forgot, err := removeWorktree(ctx, r.Root, path, func() (string, error) {
		return r.write(ctx, "", nil, append(args, "--", path)...)
	})
	if err != nil && !forgot || branch == "" {
		return out, err
	}
	// Git forgot it, its folder in use or not: the branch goes as asked.
	bout, berr := r.DeleteBranch(ctx, branch, false)
	if berr != nil {
		berr = fmt.Errorf("the worktree is removed, but its branch stays: %w", berr)
		return bout, errors.Join(err, berr)
	}
	return out, err
}

// PruneCheckouts forgets the worktrees whose folders are gone.
func (r Repo) PruneCheckouts(ctx context.Context) (string, error) {
	return r.write(ctx, "", nil, "worktree", "prune")
}

// lockWait is how long removing a worktree tries again while its folder
// is in use: Windows deletes no folder that a process works in, nor a
// file that one has open, and an agent's programs, or git run by Kero,
// may take a moment to let go of it.
var lockWait = 5 * time.Second

// removeWorktree runs remove, git's removing of the linked worktree at
// path of the repository with a worktree at root, again while its folder
// is in use, for up to lockWait. Git forgets the worktree even when it
// fails to delete the folder, leaving it empty, or with the files it
// couldn't delete: those go then, as git meant them to; and so does an
// empty folder that such a removal left before. forgot tells that git no
// longer has the worktree, whether or not its folder went.
func removeWorktree(ctx context.Context, root, path string, remove func() (string, error)) (out string, forgot bool, err error) {
	r := Repo{Root: root}
	known := r.registers(ctx, path)
	// An empty folder git doesn't know goes alone, never what fills it
	// meanwhile.
	leftover := !known && emptyDir(path)
	forgot = leftover
	deadline := time.Now().Add(lockWait)
	for {
		err = nil
		if !forgot {
			out, err = remove()
			forgot = err != nil && known && !r.registers(ctx, path)
		}
		switch {
		case leftover:
			err = os.Remove(path)
		case forgot:
			err = os.RemoveAll(path)
		}
		if err == nil || !forgot && !inUse(err) || time.Now().After(deadline) {
			if err == nil && forgot {
				out = ""
			}
			return out, forgot, err
		}
		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			return out, forgot, err
		}
	}
}

// registers reports whether git has a worktree at path; true when it
// can't tell, which deletes nothing on its word.
func (r Repo) registers(ctx context.Context, path string) bool {
	cos, err := r.Checkouts(ctx)
	if err != nil {
		return true
	}
	return slices.ContainsFunc(cos, func(co Checkout) bool { return samePath(co.Path, path) })
}

// samePath reports whether a and b are one path, as git and the system
// may spell it apart: in case on Windows and macOS, and git through the
// links, junctions and subst drives on the way, which it resolves.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b || (runtime.GOOS == "windows" || runtime.GOOS == "darwin") && strings.EqualFold(a, b) {
		return true
	}
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}

// emptyDir reports whether path is a folder with nothing in it.
func emptyDir(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

// inUse reports whether git failed on a folder or file in use, as
// Windows says of one that a process works in or has open.
func inUse(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"permission denied", "access is denied", "being used by another process", "resource busy", "directory not empty"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
