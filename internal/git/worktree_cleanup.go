package git

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// What the automatic cleanup of kero's worktrees asks of one: the ref its
// work goes into, and whether it holds files of its own that git doesn't
// track, which removing it would lose.

// DefaultRef returns the ref of the default branch to compare a branch
// with: origin's, which a pull request merges into, else the local one;
// "" when the repository has neither.
func (r Repo) DefaultRef() string {
	def := r.DefaultBranch()
	switch {
	case def == "":
		return ""
	case r.hasRef("refs/remotes/origin/" + def):
		return "origin/" + def
	case r.HasBranch(def):
		return def
	}
	return ""
}

// rebuilt are the folders of ignored files a build or an install makes
// again, which removing a worktree loses nothing of.
var rebuilt = []string{"node_modules", ".venv", "venv", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache",
	".next", ".nuxt", ".svelte-kit", ".turbo", ".parcel-cache", ".cache", "dist", "build", "out", "target", "coverage", ".gradle"}

// errLocal stops the walk of an ignored folder at its first file of its
// own.
var errLocal = errors.New("a file of its own")

// LocalFiles returns the first of the worktree's ignored files that are
// its own: none in a folder of rebuilt, nor a copy of the file at the
// same place in main, the main checkout, as .worktreeinclude copies them.
// "" when it has none.
func (r Repo) LocalFiles(ctx context.Context, main string) (string, error) {
	out, err := run(ctx, r.Root, nil, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory", "--no-empty-directory")
	if err != nil {
		return "", err
	}
	for _, e := range strings.Split(string(out), "\x00") {
		if e == "" || slices.ContainsFunc(strings.Split(strings.TrimSuffix(e, "/"), "/"), func(p string) bool { return slices.Contains(rebuilt, p) }) {
			continue
		}
		path := filepath.Join(r.Root, filepath.FromSlash(e))
		if !strings.HasSuffix(e, "/") {
			if !sameFile(path, filepath.Join(main, filepath.FromSlash(e))) {
				return e, nil
			}
			continue
		}
		// An ignored folder git lists whole: its files, each a copy of
		// main's, or a folder of rebuilt in it, else it is the worktree's.
		n := 0
		err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return errLocal
			}
			if d.IsDir() {
				if p != path && slices.Contains(rebuilt, d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if n++; n > 2000 || ctx.Err() != nil {
				return errLocal // too many to tell: kept
			}
			rel, _ := filepath.Rel(r.Root, p)
			if !sameFile(p, filepath.Join(main, rel)) {
				return errLocal
			}
			return nil
		})
		if err != nil {
			return e, nil
		}
	}
	return "", nil
}

// sameFile reports whether the files at a and b hold the same bytes; a
// link, the same target.
func sameFile(a, b string) bool {
	sa, errA := os.Lstat(a)
	sb, errB := os.Lstat(b)
	if errA != nil || errB != nil || sa.Mode().Type() != sb.Mode().Type() {
		return false
	}
	if sa.Mode()&fs.ModeSymlink != 0 {
		la, errA := os.Readlink(a)
		lb, errB := os.Readlink(b)
		return errA == nil && errB == nil && la == lb
	}
	if !sa.Mode().IsRegular() || sa.Size() != sb.Size() {
		return false
	}
	ba, errA := os.ReadFile(a)
	bb, errB := os.ReadFile(b)
	return errA == nil && errB == nil && bytes.Equal(ba, bb)
}
