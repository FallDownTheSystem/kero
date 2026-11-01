package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// What kero writes to a repository at the user's word, from the Changes
// and History views: staging, commits, branches, stashes, and fetching,
// pulling and pushing. Each runs the user's git as they would, with their
// hooks and configuration, but never waits on a prompt: git asks for no
// password (GIT_TERMINAL_PROMPT=0) and opens no editor, and what it said
// comes back whole, as a hook's or a rejected push's words.

// Error is git's failure, with all it printed.
type Error struct {
	Args   []string
	Output string
	Err    error
}

func (e *Error) Error() string {
	if s := e.Summary(); s != "" {
		return s
	}
	return "git " + e.Args[0] + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// Summary is the line of git's output that says what went wrong: its first
// error, fatal or rejection, else its last line but a hint.
func (e *Error) Summary() string {
	var last string
	for _, l := range strings.Split(e.Output, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "hint:") {
			continue
		}
		low := strings.ToLower(l)
		if strings.HasPrefix(low, "error:") || strings.HasPrefix(low, "fatal:") || strings.Contains(low, "[rejected]") || strings.Contains(low, "conflict") {
			return l
		}
		last = l
	}
	return last
}

// lockTries is how many times a write runs while another git holds the
// index's lock, as an agent's commit does for a moment.
const lockTries = 10

// retriable are the commands that write tries again on a held lock.
var retriable = map[string]bool{"add": true, "reset": true, "checkout": true, "update-index": true, "commit": true,
	"switch": true, "branch": true, "update-ref": true}

// write runs git in the worktree to change it, with stdin, and returns
// all it printed; progress, when set, gets each line as git prints it,
// those it rewrites in place (\r) too.
func (r Repo) write(ctx context.Context, stdin string, progress func(string), args ...string) (string, error) {
	return r.writeWith(ctx, nil, stdin, progress, args...)
}

// writeWith runs git as write does, with env's variables set over those
// write sets.
func (r Repo) writeWith(ctx context.Context, env []string, stdin string, progress func(string), args ...string) (string, error) {
	for try := 1; ; try++ {
		out, err := r.writeOnce(ctx, env, stdin, progress, args...)
		// A command of one step fails on a held lock before it changes
		// anything, so trying it again is safe; one of several, as a pull
		// or a stash's pop, may have done some of them, which a second
		// run would do again.
		if err == nil || try == lockTries || ctx.Err() != nil || !retriable[args[0]] || !strings.Contains(out, ".lock") || !strings.Contains(out, "File exists") {
			return out, err
		}
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return out, err
		}
	}
}

func (r Repo) writeOnce(ctx context.Context, env []string, stdin string, progress func(string), args ...string) (string, error) {
	git := command()
	if git == "" {
		return "", ErrNoGit
	}
	cmd := exec.CommandContext(ctx, git, append([]string{"-c", "core.quotepath=false", "-c", "color.ui=false"}, args...)...)
	cmd.Dir = r.Root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_SEQUENCE_EDITOR=true", "GIT_MERGE_AUTOEDIT=no")
	cmd.Env = append(cmd.Env, env...) // the last of a variable is the one it gets
	// Canceled, git is interrupted, as Ctrl+C does, so that it removes its
	// locks, as index.lock, which killed it would leave to block every git
	// after it; killed when it can't be, as on Windows, or doesn't end.
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second
	hide(cmd)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	log := &outLog{progress: progress}
	cmd.Stdout, cmd.Stderr = log, log
	err := cmd.Run()
	out := log.String()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return out, &Error{Args: args, Output: out, Err: err}
	}
	return out, nil
}

// outLog keeps what git prints, but the lines it rewrites in place, as its
// progress, of which it keeps the last.
type outLog struct {
	mu       sync.Mutex
	done     bytes.Buffer
	line     []byte
	progress func(string)
}

func (l *outLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range p {
		switch c {
		case '\r':
			if l.progress != nil && len(l.line) > 0 {
				l.progress(string(l.line))
			}
			l.line = l.line[:0]
		case '\n':
			if l.progress != nil && len(l.line) > 0 {
				l.progress(string(l.line))
			}
			l.done.Write(l.line)
			l.done.WriteByte('\n')
			l.line = l.line[:0]
		default:
			l.line = append(l.line, c)
		}
	}
	return len(p), nil
}

func (l *outLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.TrimSpace(l.done.String() + string(l.line))
}

// pathArgs ends git's arguments with paths, after --, so that none is
// taken as an option, and each literally, so that none is taken as a
// pattern: f[ab] matches fa and fb too, which a discard would drop with
// it. None is all of the worktree.
func pathArgs(args []string, paths []string) []string {
	args = append(args, "--")
	if len(paths) == 0 {
		return append(args, ":/")
	}
	for _, p := range paths {
		args = append(args, literal(p))
	}
	return args
}

// literal is a path as a pathspec that matches it alone.
func literal(path string) string { return ":(literal)" + path }

// argBudget is how many characters of paths one git command takes:
// Windows caps a command line at 32,767, which a folder of thousands of
// files, each its own path, passes; the rest is room for git's path and
// options.
const argBudget = 24000

// batches splits paths into runs whose pathspecs fit in argBudget; none
// is one run of none, all of the worktree.
func batches(paths []string) [][]string {
	if len(paths) == 0 {
		return [][]string{nil}
	}
	var out [][]string
	start, size := 0, 0
	for i, p := range paths {
		n := len(literal(p)) + 3 // a space and quotes
		if i > start && size+n > argBudget {
			out = append(out, paths[start:i])
			start, size = i, 0
		}
		size += n
	}
	return append(out, paths[start:])
}

// writePaths runs git with args and paths as write does, in as many runs
// as the command line needs.
func (r Repo) writePaths(ctx context.Context, args []string, paths []string) error {
	for _, b := range batches(paths) {
		if _, err := r.write(ctx, "", nil, pathArgs(slices.Clone(args), b)...); err != nil {
			return err
		}
	}
	return nil
}

// hasHead reports whether the worktree has a commit.
func (r Repo) hasHead(ctx context.Context) bool {
	_, err := run(ctx, r.Root, nil, "rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

// Stage stages the files under paths, or all, as they are: new, changed
// and gone ones.
func (r Repo) Stage(ctx context.Context, paths []string) error {
	return r.writePaths(ctx, []string{"add", "--all"}, paths)
}

// Unstage puts the index's files under paths, or all, back as HEAD has
// them, leaving the working tree as it is: out of the index, before the
// first commit, as git reset without a commit does.
func (r Repo) Unstage(ctx context.Context, paths []string) error {
	return r.writePaths(ctx, []string{"reset", "-q"}, paths)
}

// Discard drops the working tree's changes of tracked files since the
// index, and removes untracked files.
func (r Repo) Discard(ctx context.Context, tracked, untracked []string) error {
	if len(tracked) > 0 {
		if err := r.writePaths(ctx, []string{"checkout", "-q"}, tracked); err != nil {
			return err
		}
	}
	if len(untracked) > 0 {
		if err := r.writePaths(ctx, []string{"clean", "-f", "-q"}, untracked); err != nil {
			return err
		}
	}
	return nil
}

// Tracked returns the files under paths that the index has: those git
// goes on tracking however .gitignore lists them.
func (r Repo) Tracked(ctx context.Context, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	var files []string
	for _, b := range batches(paths) {
		out, err := run(ctx, r.Root, nil, pathArgs([]string{"ls-files", "-z"}, b)...)
		if err != nil {
			return nil, err
		}
		for f := range strings.SplitSeq(string(out), "\x00") {
			if f != "" {
				files = append(files, f)
			}
		}
	}
	return files, nil
}

// Ignore adds files and folders, a folder's path ending in a slash, to
// the worktree's .gitignore, each as a pattern that matches it alone, as
// its line endings are; and with untrack takes them out of the index,
// leaving them on disk, so that the next commit removes them.
func (r Repo) Ignore(ctx context.Context, paths []string, untrack bool) error {
	if len(paths) == 0 {
		return errors.New("nothing to ignore")
	}
	var plain []string
	for _, p := range paths {
		p = strings.TrimSuffix(p, "/")
		if _, err := r.local(p); err != nil {
			return err
		}
		plain = append(plain, p)
	}
	file, err := r.local(".gitignore")
	if err != nil {
		return err
	}
	old, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	have := map[string]bool{}
	for l := range strings.SplitSeq(string(old), "\n") {
		have[strings.TrimRight(l, "\r")] = true
	}
	eol := "\n"
	if strings.Contains(string(old), "\r\n") {
		eol = "\r\n"
	}
	text := string(old)
	for _, p := range paths {
		line := ignorePattern(p)
		if have[line] {
			continue
		}
		have[line] = true
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += eol
		}
		text += line + eol
	}
	if text != string(old) {
		if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
			return err
		}
	}
	if !untrack {
		return nil
	}
	return r.writePaths(ctx, []string{"rm", "-r", "-q", "--cached", "--ignore-unmatch"}, plain)
}

// ignorePattern is the line of .gitignore that matches the file or folder
// at a path from the worktree's top alone: anchored there by its slash,
// its wildcards and the spaces at its end, which git would drop, escaped.
func ignorePattern(p string) string {
	dir := strings.HasSuffix(p, "/")
	p = strings.TrimSuffix(p, "/")
	var b strings.Builder
	b.WriteByte('/')
	for i, r := range p {
		switch {
		case r == '*' || r == '?' || r == '[' || r == '\\':
			b.WriteByte('\\')
		case r == ' ' && strings.TrimRight(p[i:], " ") == "":
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	if dir {
		b.WriteByte('/')
	}
	return b.String()
}

// emptyBlob is the hash of an empty file: the index's of one added with
// intent to add (git add -N), whose text is yet the working tree's.
const emptyBlob = "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"

// SetIndex makes text the index's content of a file, when the index's
// still hashes to known (Text.Index): Hash(nil) for a file the index
// hasn't, which takes its mode from the working tree's. It holds the
// index's lock throughout, as git does, so that nothing staged between
// the check and the write is written over.
func (r Repo) SetIndex(ctx context.Context, path, known, text string) error {
	full, err := r.local(path)
	if err != nil {
		return err
	}
	// The diff the text is made of is of the working tree's text as it is,
	// which a filter, as Git LFS's, would make another before git stored it.
	out, err := run(ctx, r.Root, nil, "check-attr", "filter", "ident", "working-tree-encoding", "--", path)
	if err != nil {
		return err
	}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i := strings.LastIndex(l, ": "); i >= 0 && l[i+2:] != "unspecified" && l[i+2:] != "unset" {
			return errors.New(path + " goes through a git filter: stage it whole")
		}
	}
	return r.withIndex(ctx, func(env []string) error { return r.setIndex(ctx, env, path, full, known, text) })
}

func (r Repo) setIndex(ctx context.Context, env []string, path, full, known, text string) error {
	out, err := run(ctx, r.Root, env, "ls-files", "-s", "-z", "--", literal(path))
	if err != nil {
		return err
	}
	mode := ""
	var cur []byte
	entry := strings.TrimSuffix(string(out), "\x00")
	blob := ""
	if entry != "" {
		meta, _, _ := strings.Cut(entry, "\t")
		f := strings.Fields(meta)
		if len(f) != 3 || f[2] != "0" {
			return errors.New(path + " has a conflict: resolve it first")
		}
		mode, blob = f[0], f[1]
		if cur, err = run(ctx, r.Root, env, "cat-file", "blob", blob); err != nil {
			return err
		}
	} else {
		fi, err := os.Lstat(full)
		if err != nil || !fi.Mode().IsRegular() {
			return errors.New(path + " is no file of the working tree")
		}
		mode = "100644"
		if fi.Mode().Perm()&0o111 != 0 {
			mode = "100755"
		}
	}
	if mode != "100644" && mode != "100755" {
		return errors.New(path + " is not a regular file")
	}
	if Hash(cur) != known {
		return ErrChanged
	}
	// The text of a file the index has is the index's already, which no
	// filter of the working tree's, as core.autocrlf's, is to change; that
	// of one it hasn't, or has empty with intent to add, is the working
	// tree's, which git cleans as add does.
	args := []string{"hash-object", "-w", "--stdin"}
	if entry == "" || blob == emptyBlob {
		args = append(args, "--path", path)
	}
	sha, err := r.input(ctx, text, args...)
	if err != nil {
		return err
	}
	_, err = run(ctx, r.Root, env, "update-index", "--add", "--cacheinfo", mode+","+strings.TrimSpace(string(sha))+","+path)
	return err
}

// withIndex runs fn on a copy of the worktree's index, which its env
// names (GIT_INDEX_FILE), holding the index's lock, then renames the copy
// over the index, as git renames its lock file. The copy keeps the time
// of the index, and git's write of it its own: git trusts a file's entry
// as clean when the file is older than the index, by whole seconds on
// Windows, and a later time would hide a change made in its second.
func (r Repo) withIndex(ctx context.Context, fn func(env []string) error) error {
	index := filepath.Join(r.GitDir, "index")
	lock := index + ".lock"
	var f *os.File
	for try := 1; ; try++ {
		var err error
		if f, err = os.OpenFile(lock, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666); err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) || try == lockTries {
			return fmt.Errorf("the index is locked (%s): %w", lock, err)
		}
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	defer func() {
		f.Close()
		os.Remove(lock)
	}()
	tmp, err := os.CreateTemp(r.GitDir, "kero-index-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	defer os.Remove(tmpPath + ".lock")
	cur, err := os.ReadFile(index)
	switch {
	case err == nil:
		if err = os.WriteFile(tmpPath, cur, 0o666); err == nil {
			var fi os.FileInfo
			if fi, err = os.Stat(index); err == nil {
				err = os.Chtimes(tmpPath, fi.ModTime(), fi.ModTime())
			}
		}
	case errors.Is(err, os.ErrNotExist):
		// No index yet: git makes one where it is to be.
		err = os.Remove(tmpPath)
	}
	if err != nil {
		return err
	}
	if err := fn([]string{"GIT_INDEX_FILE=" + tmpPath}); err != nil {
		return err
	}
	// Windows refuses the rename while another program reads the index for
	// a moment, which git tries again past too.
	for try := 1; ; try++ {
		if err = os.Rename(tmpPath, index); err == nil || try == lockTries {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// input runs git with stdin, returning what it printed to stdout alone.
func (r Repo) input(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	git := command()
	if git == "" {
		return nil, ErrNoGit
	}
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Dir = r.Root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = strings.NewReader(stdin)
	hide(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, &Error{Args: args, Output: strings.TrimSpace(stderr.String()), Err: err}
	}
	return out, nil
}

// CommitOptions are a commit's: its message, and whether it amends HEAD,
// signs off, and skips the hooks.
type CommitOptions struct {
	Message  string
	Amend    bool
	Signoff  bool
	NoVerify bool
}

// Commit commits the index, returning what git and the hooks printed.
func (r Repo) Commit(ctx context.Context, o CommitOptions) (string, error) {
	if strings.TrimSpace(o.Message) == "" {
		return "", errors.New("a commit needs a message")
	}
	// The message is the user's as written: no line of it, as "#12 fix",
	// is a comment to strip.
	args := []string{"commit", "-F", "-", "--cleanup=whitespace"}
	if o.Amend {
		args = append(args, "--amend")
	}
	if o.Signoff {
		args = append(args, "--signoff")
	}
	if o.NoVerify {
		args = append(args, "--no-verify")
	}
	return r.write(ctx, o.Message, nil, args...)
}

// LastMessage returns HEAD's message, as amending it starts from.
func (r Repo) LastMessage(ctx context.Context) (string, error) {
	out, err := run(ctx, r.Root, nil, "log", "-1", "--format=%B", "HEAD")
	return strings.TrimSpace(string(out)), err
}

// UndoCommit takes HEAD's commit back, its changes staged: the branch's
// first commit too, which leaves it without one.
func (r Repo) UndoCommit(ctx context.Context) error {
	out, err := run(ctx, r.Root, nil, "show", "-s", "--format=%H%x1f%P", "HEAD", "--")
	if err != nil {
		return err
	}
	head, parents, _ := strings.Cut(strings.TrimSpace(string(out)), "\x1f")
	if strings.TrimSpace(parents) != "" {
		_, err = r.write(ctx, "", nil, "reset", "--soft", "-q", "HEAD~1")
		return err
	}
	// A shallow clone's oldest commit has parents it hasn't fetched: the
	// branch is not to go with it.
	if b, err := os.ReadFile(filepath.Join(r.Common, "shallow")); err == nil && strings.Contains(string(b), head) {
		return errors.New("HEAD's parent isn't fetched in this shallow clone")
	}
	// Its first commit: the branch goes, as long as it is still there.
	_, err = r.write(ctx, "", nil, "update-ref", "-d", "HEAD", head)
	return err
}

// Continue goes on with the operation in progress, its conflicts
// resolved and staged; Abort gives it up, back to where it started.
func (r Repo) Continue(ctx context.Context) (string, error) {
	switch op := r.operation(); op {
	case "merge":
		return r.write(ctx, "", nil, "commit", "--no-edit")
	case "":
		return "", errors.New("no merge, rebase, cherry-pick or revert is in progress")
	default:
		return r.write(ctx, "", nil, op, "--continue")
	}
}

func (r Repo) Abort(ctx context.Context) (string, error) {
	op := r.operation()
	if op == "" {
		return "", errors.New("no merge, rebase, cherry-pick or revert is in progress")
	}
	return r.write(ctx, "", nil, op, "--abort")
}

// CherryPick applies a commit's change on HEAD as a commit of its own;
// RevertCommit commits its change undone.
func (r Repo) CherryPick(ctx context.Context, rev string) (string, error) {
	if err := checkRev(rev); err != nil {
		return "", err
	}
	return r.write(ctx, "", nil, "cherry-pick", rev)
}

func (r Repo) RevertCommit(ctx context.Context, rev string) (string, error) {
	if err := checkRev(rev); err != nil {
		return "", err
	}
	return r.write(ctx, "", nil, "revert", "--no-edit", rev)
}

// Reset moves the branch to rev: "soft" keeps the changes since staged,
// "mixed" keeps them unstaged, "hard" drops them.
func (r Repo) Reset(ctx context.Context, rev, mode string) error {
	if err := checkRev(rev); err != nil {
		return err
	}
	if mode != "soft" && mode != "mixed" && mode != "hard" {
		return fmt.Errorf("no reset %q", mode)
	}
	_, err := r.write(ctx, "", nil, "reset", "-q", "--"+mode, rev, "--")
	return err
}

// Checkout checks out a commit, detached.
func (r Repo) Checkout(ctx context.Context, rev string) error {
	if err := checkRev(rev); err != nil {
		return err
	}
	_, err := r.write(ctx, "", nil, "switch", "-q", "--detach", rev)
	return err
}

// gitPath returns a file of the worktree's git directory, as git names it
// (rev-parse --git-path): MERGE_MSG, say.
func (r Repo) gitPath(name string) string { return filepath.Join(r.GitDir, name) }

// MergeMessage returns the message git prepared for the commit that ends
// a merge, revert or cherry-pick, "" for none.
func (r Repo) MergeMessage() string {
	b, err := os.ReadFile(r.gitPath("MERGE_MSG"))
	if err != nil {
		return ""
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
