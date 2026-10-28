package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// The index, what the next commit takes, as the Changes view's
// Uncommitted shows it: two lists, as git status has them, of what is
// staged, the index's changes since HEAD, and of what isn't, the working
// tree's since the index, with untracked files and conflicts.

// Sides of a file's text besides a tree-ish (Texts): the index's, the
// working tree's, and none, for a file a side hasn't.
const (
	SideIndex = ":index"
	SideWork  = ":work"
	SideNone  = ":none"
	// SideOursStage and SideTheirsStage are the index's versions of a file
	// in conflict, git's stages 2 and 3, read as revisions are (":2:path").
	SideOursStage   = ":2"
	SideTheirsStage = ":3"
)

// Index returns the files staged and those not: untracked ones among
// them, "untracked", and conflicts, "conflict", which are no change of
// the index's.
func (r Repo) Index(ctx context.Context) (staged, unstaged []File, err error) {
	// At once, the list as soon as the slowest of git's runs: as the
	// Changes view waits for it after each file staged, and git starts
	// slowly on Windows.
	var untracked []byte
	var errs [3]error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); staged, errs[0] = r.diffFiles(ctx, "--cached", "-M") }()
	go func() { defer wg.Done(); unstaged, errs[1] = r.diffFiles(ctx) }()
	go func() {
		defer wg.Done()
		untracked, errs[2] = run(ctx, r.Root, nil, "ls-files", "-z", "--others", "--exclude-standard")
	}()
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, nil, err
		}
	}
	var conflicts []string
	for _, f := range unstaged {
		if f.Status == "conflict" {
			conflicts = append(conflicts, f.Path)
		}
	}
	staged = slices.DeleteFunc(staged, func(f File) bool { return f.Status == "conflict" || slices.Contains(conflicts, f.Path) })
	for i, p := range strings.Split(strings.TrimSuffix(string(untracked), "\x00"), "\x00") {
		if p == "" {
			continue
		}
		f := File{Path: p, Status: "untracked"}
		// As many lines counted as a glance needs, as Status does.
		if i < 200 {
			f.Added = countLines(filepath.Join(r.Root, filepath.FromSlash(p)))
		}
		unstaged = append(unstaged, f)
	}
	return staged, unstaged, nil
}

// Staged returns what a commit of the index would take: git diff --stat's
// summary of it, and the patch, cut at max bytes, the summary telling
// what the cut left out.
func (r Repo) Staged(ctx context.Context, max int) (stat, patch string, err error) {
	out, err := run(ctx, r.Root, nil, "diff", "--cached", "--no-ext-diff", "--no-textconv", "-M", "--stat=120")
	if err != nil {
		return "", "", err
	}
	stat = strings.TrimSpace(string(out))
	out, err = run(ctx, r.Root, nil, "diff", "--cached", "--no-ext-diff", "--no-textconv", "-M", "--unified=2")
	if err != nil {
		return "", "", err
	}
	patch = string(out)
	if len(patch) > max {
		patch = patch[:max]
		if i := strings.LastIndexByte(patch, '\n'); i > 0 {
			patch = patch[:i+1]
		}
		patch += "[the rest of the diff is cut]\n"
	}
	return stat, patch, nil
}

// diffFiles returns the files git diff with args lists, with the lines
// each adds and removes; a file unmerged is a "conflict", listed once.
func (r Repo) diffFiles(ctx context.Context, args ...string) ([]File, error) {
	base := []string{"diff", "--no-ext-diff", "--no-textconv", "-z"}
	var names, out []byte
	var nameErr, err error
	done := make(chan struct{})
	go func() {
		names, nameErr = run(ctx, r.Root, nil, append(append(slices.Clone(base), args...), "--name-status")...)
		close(done)
	}()
	out, err = run(ctx, r.Root, nil, append(append(slices.Clone(base), args...), "--numstat")...)
	<-done
	if nameErr != nil {
		return nil, nameErr
	}
	if err != nil {
		return nil, err
	}
	files := parseNameStatus(names)
	byPath := map[string]int{}
	for i, f := range files {
		byPath[f.Path] = i
	}
	for _, n := range parseNumstat(out) {
		if i, ok := byPath[n.path]; ok {
			files[i].Added, files[i].Removed, files[i].Binary = n.added, n.removed, n.binary
		}
	}
	return files, nil
}

// parseNameStatus reads git diff --name-status -z: a status, then a path,
// or two for a rename or a copy.
func parseNameStatus(out []byte) []File {
	var files []File
	seen := map[string]int{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i+1 < len(fields); i++ {
		code := fields[i]
		if code == "" {
			continue
		}
		f := File{Path: fields[i+1]}
		i++
		switch code[0] {
		case 'A':
			f.Status = "added"
		case 'D':
			f.Status = "deleted"
		case 'R', 'C':
			if i+1 < len(fields) {
				f.OldPath, f.Path = f.Path, fields[i+1]
				i++
			}
			f.Status = "renamed"
			if code[0] == 'C' {
				f.Status, f.OldPath = "added", ""
			}
		case 'T':
			f.Status = "type"
		case 'U':
			f.Status = "conflict"
		default:
			f.Status = "modified"
		}
		// An unmerged path comes once as such, and again as its change.
		if j, ok := seen[f.Path]; ok {
			if f.Status == "conflict" {
				files[j].Status = "conflict"
			}
			continue
		}
		seen[f.Path] = len(files)
		files = append(files, f)
	}
	return files
}

// Texts returns a file's text in from, from oldPath ("" for path), and in
// to: each a tree-ish, SideIndex, SideWork or SideNone. Hash is the hash
// of to's content, and Index the index's, when a side is the index's,
// which a write of either takes (Write, SetIndex).
func (r Repo) Texts(ctx context.Context, from, to, path, oldPath string) (Text, error) {
	if _, err := r.local(path); err != nil {
		return Text{}, err
	}
	if oldPath == "" {
		oldPath = path
	}
	if _, err := r.local(oldPath); err != nil {
		return Text{}, err
	}
	ob, err := r.side(ctx, from, oldPath)
	if err != nil {
		return Text{}, err
	}
	nb, err := r.side(ctx, to, path)
	if err != nil {
		return Text{}, err
	}
	t := Text{Hash: Hash(nb)}
	switch {
	case from == SideIndex:
		t.Index = Hash(ob)
	case to == SideIndex:
		t.Index = t.Hash
	}
	switch {
	case binary(ob) || binary(nb):
		t.Binary = true
	case len(ob) > maxText || len(nb) > maxText:
		t.Large = true
		t.Patch, _ = r.sidesPatch(ctx, from, to, path)
	default:
		t.Old, t.New = string(ob), string(nb)
		t.Mixed = mixedEndings(ob) || mixedEndings(nb)
	}
	return t, nil
}

// side returns a file's content in a side, nil for none.
func (r Repo) side(ctx context.Context, side, path string) ([]byte, error) {
	switch side {
	case SideNone:
		return nil, nil
	case SideWork:
		full, err := r.local(path)
		if err != nil {
			return nil, err
		}
		b, err := readWorking(full)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return b, nil
	case SideIndex:
		side = ""
	}
	if err := checkRev(side); err != nil {
		return nil, err
	}
	b, err := run(ctx, r.Root, nil, "cat-file", "blob", side+":"+path)
	if err != nil {
		return nil, nil // not in that side
	}
	return b, nil
}

// sidesPatch returns a file's change from one side to another as git diff
// prints it.
func (r Repo) sidesPatch(ctx context.Context, from, to, path string) (string, error) {
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "-M"}
	if from == SideNone && to != SideIndex {
		empty, err := r.emptyTree(ctx)
		if err != nil {
			return "", err
		}
		from = empty
	}
	switch {
	case from == SideIndex && to == SideWork:
	case to == SideIndex:
		args = append(args, "--cached")
		if from != SideNone {
			args = append(args, from)
		}
	case to == SideWork:
		// Through a snapshot, which has the untracked files git diff
		// leaves out.
		return r.Patch(ctx, from, "", path)
	default:
		args = append(args, from, to)
	}
	out, err := run(ctx, r.Root, nil, append(args, "--", path)...)
	return string(out), err
}

// checkRev refuses a revision that git would take as an option.
func checkRev(rev string) error {
	if strings.HasPrefix(rev, "-") || strings.ContainsAny(rev, "\x00\n") {
		return errors.New("not a revision: " + rev)
	}
	return nil
}

// mixedEndings reports whether text has lines ending both in \r\n and in
// \n alone, which an editor's buffer, holding one ending, would write back
// as all one.
func mixedEndings(b []byte) bool {
	crlf, lf := false, false
	for i, c := range b {
		if c != '\n' {
			continue
		}
		if i > 0 && b[i-1] == '\r' {
			crlf = true
		} else {
			lf = true
		}
		if crlf && lf {
			return true
		}
	}
	return false
}
