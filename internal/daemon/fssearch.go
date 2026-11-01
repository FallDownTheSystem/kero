package daemon

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"kero/internal/git"
	"kero/internal/proto"
)

// A project's files, as Go to File finds them and the Search view searches
// them: git's, in a repository, so that what it ignores, as node_modules
// or a build's output, never crowds out the project's own.

const (
	// maxFiles bounds the files fs.files lists.
	maxFiles = 100000
	// maxMatches bounds the lines fs.search finds, all told, and
	// maxSearched the files it reads, larger ones being data rather than
	// what a search of code looks for.
	maxMatches  = 2000
	maxSearched = 2 << 20
	// matchText is how much of a long line a match shows, around it.
	matchText = 240
)

func fsFiles(p proto.FSFilesParams) (proto.FSFiles, error) {
	_, paths, more, err := listFiles(p.Dir, p.Hidden)
	if paths == nil {
		paths = []string{}
	}
	return proto.FSFiles{Paths: paths, More: more}, err
}

// listFiles lists the files of a project (FSFilesParams), from its root,
// which it returns too, absolute.
func listFiles(dir string, hidden bool) (root string, paths []string, more bool, err error) {
	root = hostPath(dir, "")
	// The folder itself may be a link, which the walk would not go in.
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if st, err := os.Stat(root); err != nil {
		return root, nil, false, err
	} else if !st.IsDir() {
		return root, nil, false, fmt.Errorf("%s is no folder", root)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if files, err := git.Files(ctx, root); err == nil {
		for _, f := range files {
			if !hidden && dotted(f) {
				continue
			}
			if len(paths) >= maxFiles {
				return root, paths, true, nil
			}
			paths = append(paths, f)
		}
		return root, paths, false, nil
	}
	err = filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		switch {
		case path == root:
			return err
		case err != nil:
			// A folder in it that can't be read is left out.
			return nil
		}
		name := e.Name()
		if name == ".git" || !hidden && strings.HasPrefix(name, ".") {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsDir() || !e.Type().IsRegular() {
			return nil
		}
		if len(paths) >= maxFiles {
			more = true
			return filepath.SkipAll
		}
		if rel, err := filepath.Rel(root, path); err == nil {
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	return root, paths, more, err
}

// dotted tells a path, with slashes, that is a dotfile or in a dot folder.
func dotted(p string) bool {
	return strings.HasPrefix(p, ".") || strings.Contains(p, "/.")
}

// searchPattern is what fs.search looks for.
func searchPattern(p proto.FSSearchParams) (*regexp.Regexp, error) {
	expr := p.Query
	if !p.Regex {
		expr = regexp.QuoteMeta(expr)
	}
	if p.Word {
		expr = `\b(?:` + expr + `)\b`
	}
	if !p.Case {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	return re, nil
}

func fsSearch(p proto.FSSearchParams) (proto.FSSearch, error) {
	out := proto.FSSearch{Files: []proto.FSSearchFile{}}
	if p.Query == "" {
		return out, nil
	}
	re, err := searchPattern(p)
	if err != nil {
		return out, err
	}
	root, paths, more, err := listFiles(p.Dir, p.Hidden)
	if err != nil {
		return out, err
	}
	found := make([][]proto.FSMatch, len(paths))
	var total atomic.Int64
	var full atomic.Bool
	deadline := time.Now().Add(15 * time.Second)
	next := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				if full.Load() {
					continue
				}
				found[i] = searchFile(filepath.Join(root, filepath.FromSlash(paths[i])), re, &total, &full)
			}
		}()
	}
	for i := range paths {
		if full.Load() || time.Now().After(deadline) {
			more = true
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()
	for i, ms := range found {
		if len(ms) > 0 {
			out.Files = append(out.Files, proto.FSSearchFile{Path: paths[i], Matches: ms})
		}
	}
	out.More = more || full.Load()
	return out, nil
}

// searchFile finds re's matches in a file, a line each: none in a file too
// large, or binary, as a NUL in its start tells, as git tells one. total
// counts the matches of every file, past maxMatches of which full stops
// the search.
func searchFile(path string, re *regexp.Regexp, total *atomic.Int64, full *atomic.Bool) []proto.FSMatch {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxSearched {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 || !re.Match(b) {
		return nil
	}
	var ms []proto.FSMatch
	for n := 1; len(b) > 0; n++ {
		line := b
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			line, b = b[:i], b[i+1:]
		} else {
			b = nil
		}
		line = bytes.TrimSuffix(line, []byte("\r"))
		m := re.FindIndex(line)
		if m == nil {
			continue
		}
		if total.Add(1) > maxMatches {
			full.Store(true)
			break
		}
		ms = append(ms, matchOf(n, line, m[0], m[1]))
	}
	return ms
}

// matchOf is a line's match, its text without its indentation, and cut
// to matchText around the match, which a … stands for what is cut of.
func matchOf(n int, line []byte, start, end int) proto.FSMatch {
	indent := len(line) - len(bytes.TrimLeft(line, " \t"))
	from, to := min(indent, start), len(line)
	if to-from > matchText {
		// From a little before the match, to as much after it as fits.
		from = max(from, start-matchText/4)
		for from > 0 && !utf8.RuneStart(line[from]) {
			from--
		}
		to = min(len(line), from+matchText)
		for to < len(line) && !utf8.RuneStart(line[to]) {
			to--
		}
		to = max(to, min(end, len(line)))
	}
	end = min(end, to)
	// Each part made valid apart, as what replaces a byte that is not
	// UTF-8 is longer than it, which would move the match.
	valid := func(b []byte) string { return strings.ToValidUTF8(string(b), "�") }
	pre, hit, post := valid(line[from:start]), valid(line[start:end]), valid(line[end:to])
	if from > indent {
		pre = "…" + pre
	}
	if to < len(line) {
		post += "…"
	}
	return proto.FSMatch{Line: n, Text: pre + hit + post, Start: len(pre), End: len(pre) + len(hit)}
}
