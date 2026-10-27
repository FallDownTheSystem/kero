package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// A new worktree has only the files git tracks, not the ignored ones a
// working checkout leans on, as .claude/settings.local.json, CLAUDE.local.md
// or .env. A .worktreeinclude at the top of the main checkout lists those
// to copy, in .gitignore's syntax, as Claude Code's own worktrees read it.

// Included returns the files of the main checkout, as paths from its top
// with slashes, that .worktreeinclude lists and git ignores: none without
// one. A tracked file never is, as the worktree checks out its own. Files
// under node_modules, in a nested repository or linked worktree (a folder
// with a .git), or under one of skip, paths from the top, are left out.
func (r Repo) Included(ctx context.Context, skip ...string) ([]string, error) {
	main := r.Main()
	if main == "" {
		return nil, nil
	}
	b, err := os.ReadFile(filepath.Join(main, ".worktreeinclude"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := parseInclude(b)
	if len(m) == 0 {
		return nil, nil
	}
	// --directory names a wholly ignored folder, as node_modules, rather
	// than every file in it, so that it is walked only when a pattern can
	// reach in.
	out, err := run(ctx, main, nil, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return nil, err
	}
	// left reports whether rel is left out by a folder above it: for a
	// folder itself, ask of a path inside it.
	nested := map[string]bool{}
	left := func(rel string) bool {
		for _, s := range skip {
			s = strings.Trim(filepath.ToSlash(s), "/")
			if rel == s || strings.HasPrefix(rel, s+"/") {
				return true
			}
		}
		for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
			if path.Base(dir) == "node_modules" {
				return true
			}
			n, ok := nested[dir]
			if !ok {
				_, err := os.Lstat(filepath.Join(main, filepath.FromSlash(dir), ".git"))
				n = err == nil
				nested[dir] = n
			}
			if n {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	var files []string
	add := func(rel string) {
		if !seen[rel] && !left(rel) && m.match(rel, false) {
			seen[rel] = true
			files = append(files, rel)
		}
	}
	for _, p := range bytes.Split(out, []byte{0}) {
		rel := string(p)
		if rel == "" {
			continue
		}
		if !strings.HasSuffix(rel, "/") {
			add(rel)
			continue
		}
		rel = strings.TrimSuffix(rel, "/")
		if left(rel+"/x") || !m.reaches(rel) {
			continue
		}
		root := filepath.Join(main, filepath.FromSlash(rel))
		filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			sub, _ := filepath.Rel(main, p)
			sub = filepath.ToSlash(sub)
			if e.IsDir() {
				if p != root && (e.Name() == "node_modules" || left(sub+"/x")) {
					return filepath.SkipDir
				}
				return nil
			}
			if e.Type().IsRegular() {
				add(sub)
			}
			return nil
		})
	}
	return files, nil
}

// CopyIncluded copies the files Included returns into the worktree at
// dst, keeping their paths and modes, but those dst has already. It goes
// on past a file that fails, and returns how many it copied.
func (r Repo) CopyIncluded(ctx context.Context, dst string, skip ...string) (int, error) {
	files, err := r.Included(ctx, skip...)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, rel := range files {
		from := filepath.Join(r.Main(), filepath.FromSlash(rel))
		to := filepath.Join(dst, filepath.FromSlash(rel))
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		if err := copyFile(from, to); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	return out.Close()
}

// includePattern is a line of .worktreeinclude: the segments of the path
// it matches, from the top, "**" for any number of them.
type includePattern struct {
	segs   []string
	negate bool
	dir    bool // with a trailing slash, it matches folders alone
}

type includes []includePattern

func parseInclude(b []byte) includes {
	var m includes
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" || line[0] == '#' {
			continue
		}
		var p includePattern
		if line[0] == '!' {
			p.negate, line = true, line[1:]
		} else if line[0] == '\\' {
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			p.dir, line = true, strings.TrimRight(line, "/")
		}
		// A pattern with no slash, but for a trailing one, matches at
		// any depth.
		if !strings.Contains(line, "/") {
			line = "**/" + line
		}
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			continue
		}
		p.segs = strings.Split(strings.ReplaceAll(line, "[!", "[^"), "/")
		m = append(m, p)
	}
	return m
}

// match reports whether the last pattern to match rel, or a folder above
// it, includes it, as .gitignore decides what it ignores: a file in a
// folder matched is matched too, which a later "!" can't undo.
func (m includes) match(rel string, dir bool) bool {
	segs := strings.Split(rel, "/")
	for i := 1; i < len(segs); i++ {
		if m.last(segs[:i], true) {
			return true
		}
	}
	return m.last(segs, dir)
}

func (m includes) last(segs []string, dir bool) bool {
	in := false
	for _, p := range m {
		if (!p.dir || dir) && matchSegs(p.segs, segs) {
			in = !p.negate
		}
	}
	return in
}

// reaches reports whether a file in the folder rel can match: the folder
// matches, or a pattern names a path inside it from the top. A pattern of
// a name anywhere, as .env or **/CLAUDE.md, doesn't reach into a wholly
// ignored folder, which would walk all of node_modules, .venv or dist.
func (m includes) reaches(rel string) bool {
	if m.match(rel, true) {
		return true
	}
	segs := strings.Split(rel, "/")
	for _, p := range m {
		if p.negate || p.segs[0] == "**" || len(p.segs) <= len(segs) {
			continue
		}
		inside := true
		for i, s := range segs {
			if p.segs[i] == "**" {
				break
			}
			if ok, _ := path.Match(p.segs[i], s); !ok {
				inside = false
				break
			}
		}
		if inside {
			return true
		}
	}
	return false
}

func matchSegs(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchSegs(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	ok, _ := path.Match(pat[0], segs[0])
	return ok && matchSegs(pat[1:], segs[1:])
}
