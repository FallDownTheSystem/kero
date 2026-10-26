package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// opsRepo is a repository as repo makes it, with a committer, as a commit
// of kero's needs.
func opsRepo(t *testing.T, files map[string]string) Repo {
	t.Helper()
	r := repo(t, files)
	gitIn(t, r.Root, "config", "user.name", "t")
	gitIn(t, r.Root, "config", "user.email", "t@t")
	gitIn(t, r.Root, "config", "commit.gpgsign", "false")
	gitIn(t, r.Root, "config", "core.autocrlf", "false")
	gitIn(t, r.Root, "branch", "-M", "main")
	return r
}

func paths(files []File) string {
	var s []string
	for _, f := range files {
		s = append(s, f.Path+":"+f.Status)
	}
	return strings.Join(s, ",")
}

func TestIgnore(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n", "build/out.bin": "o\n", ".gitignore": "*.log\r\n"})
	write(t, filepath.Join(r.Root, "build", "new.bin"), "n\n")
	write(t, filepath.Join(r.Root, "notes [old].md"), "n\n")
	if got, err := r.Tracked(ctx, []string{"build", "notes [old].md"}); err != nil || !slices.Equal(got, []string{"build/out.bin"}) {
		t.Errorf("tracked = %q, %v", got, err)
	}

	// Each matches itself alone, as the file's lines end; one listed
	// already isn't again.
	if err := r.Ignore(ctx, []string{"notes [old].md", "build/"}, false); err != nil {
		t.Fatal(err)
	}
	if err := r.Ignore(ctx, []string{"build/"}, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(r.Root, ".gitignore"))
	if want := "*.log\r\n/notes \\[old].md\r\n/build/\r\n"; string(b) != want {
		t.Errorf(".gitignore = %q, want %q", b, want)
	}
	if out := gitIn(t, r.Root, "status", "--porcelain", "--untracked-files=all"); out != " M .gitignore\n" {
		t.Errorf("status = %q, want .gitignore alone", out)
	}
	// Tracked, the folder's file stays, until it is taken out of the index:
	// on disk still.
	if got, _ := r.Tracked(ctx, []string{"build"}); len(got) != 1 {
		t.Errorf("ignored, build tracks %q", got)
	}
	if err := r.Ignore(ctx, []string{"build/"}, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Tracked(ctx, []string{"build"}); len(got) != 0 {
		t.Errorf("untracked, build tracks %q", got)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "build", "out.bin")); err != nil {
		t.Errorf("untracking removed the file: %v", err)
	}
	if err := r.Ignore(ctx, []string{"../x"}, false); err == nil {
		t.Error("ignored a path outside the worktree")
	}
}

func TestIndex(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "one\ntwo\n", "b.txt": "b\n"})
	write(t, filepath.Join(r.Root, "a.txt"), "one\nTWO\n")
	gitIn(t, r.Root, "add", "a.txt")
	write(t, filepath.Join(r.Root, "a.txt"), "one\nTWO\nthree\n")
	write(t, filepath.Join(r.Root, "b.txt"), "B\n")
	write(t, filepath.Join(r.Root, "new.txt"), "n1\nn2\n")

	staged, unstaged, err := r.Index(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(staged); got != "a.txt:modified" {
		t.Errorf("staged %s", got)
	}
	if got := paths(unstaged); got != "a.txt:modified,b.txt:modified,new.txt:untracked" {
		t.Errorf("unstaged %s", got)
	}
	if unstaged[2].Added != 2 {
		t.Errorf("untracked file's lines %d", unstaged[2].Added)
	}

	// Staged: HEAD to the index; unstaged: the index to the working tree.
	txt, err := r.Texts(ctx, "HEAD", SideIndex, "a.txt", "")
	if err != nil || txt.Old != "one\ntwo\n" || txt.New != "one\nTWO\n" || txt.Index != Hash([]byte("one\nTWO\n")) {
		t.Errorf("staged text %+v, %v", txt, err)
	}
	txt, err = r.Texts(ctx, SideIndex, SideWork, "a.txt", "")
	if err != nil || txt.Old != "one\nTWO\n" || txt.New != "one\nTWO\nthree\n" || txt.Index != Hash([]byte("one\nTWO\n")) {
		t.Errorf("unstaged text %+v, %v", txt, err)
	}
	if txt, _ := r.Texts(ctx, SideIndex, SideWork, "new.txt", ""); txt.Old != "" || txt.New != "n1\nn2\n" {
		t.Errorf("untracked text %+v", txt)
	}

	// A hunk staged: the index's text as the view made it.
	if err := r.SetIndex(ctx, "a.txt", Hash([]byte("stale")), "x\n"); err != ErrChanged {
		t.Errorf("a stale index write: %v", err)
	}
	if err := r.SetIndex(ctx, "a.txt", Hash([]byte("one\nTWO\n")), "one\nTWO\nthree\n"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.Root, "show", ":a.txt"); got != "one\nTWO\nthree\n" {
		t.Errorf("index has %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(b) != "one\nTWO\nthree\n" {
		t.Errorf("the working tree's file changed: %q", b)
	}
	// One the index hasn't yet.
	if err := r.SetIndex(ctx, "new.txt", Hash(nil), "n1\n"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.Root, "show", ":new.txt"); got != "n1\n" {
		t.Errorf("index has %q of a new file", got)
	}
	// A new file's text is the working tree's, which git cleans as add
	// does; a known file's is the index's, as it is.
	gitIn(t, r.Root, "config", "core.autocrlf", "true")
	write(t, filepath.Join(r.Root, "crlf.txt"), "c1\r\nc2\r\n")
	if err := r.SetIndex(ctx, "crlf.txt", Hash(nil), "c1\r\n"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.Root, "show", ":crlf.txt"); got != "c1\n" {
		t.Errorf("index has %q of a new file of CRLF lines", got)
	}
	if err := r.SetIndex(ctx, "crlf.txt", Hash([]byte("c1\n")), "c1\nc2\n"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.Root, "show", ":crlf.txt"); got != "c1\nc2\n" {
		t.Errorf("index has %q of a known file", got)
	}
	gitIn(t, r.Root, "rm", "-q", "-f", "crlf.txt")
	gitIn(t, r.Root, "config", "core.autocrlf", "false")

	if err := r.Unstage(ctx, []string{"a.txt", "new.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Stage(ctx, []string{"b.txt"}); err != nil {
		t.Fatal(err)
	}
	staged, unstaged, _ = r.Index(ctx)
	if paths(staged) != "b.txt:modified" || paths(unstaged) != "a.txt:modified,new.txt:untracked" {
		t.Errorf("after unstaging and staging: %s | %s", paths(staged), paths(unstaged))
	}
	if err := r.Discard(ctx, []string{"a.txt"}, []string{"new.txt"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(b) != "one\ntwo\n" {
		t.Errorf("discarded a.txt is %q", b)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "new.txt")); !os.IsNotExist(err) {
		t.Error("discarding left the untracked file")
	}
	if err := r.Stage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Unstage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if staged, _, _ := r.Index(ctx); len(staged) != 0 {
		t.Errorf("unstaging all left %s", paths(staged))
	}
}

// SetIndex writes as git does, holding the index's lock: it waits on
// another's, leaves none, and is refused a file a filter changes, whose
// text it can't know clean.
func TestSetIndexLock(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	lock := filepath.Join(r.GitDir, "index.lock")
	if err := os.WriteFile(lock, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err := r.SetIndex(short, "a.txt", Hash([]byte("a\n")), "x\n")
	cancel()
	if err == nil {
		t.Fatal("SetIndex wrote past another's lock")
	}
	os.Remove(lock)
	if err := r.SetIndex(ctx, "a.txt", Hash([]byte("a\n")), "x\n"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.Root, "show", ":a.txt"); got != "x\n" {
		t.Errorf("index has %q", got)
	}
	if staged, _, _ := r.Index(ctx); paths(staged) != "a.txt:modified" {
		t.Errorf("staged %s", paths(staged))
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Error("SetIndex left the index's lock")
	}
	if left, _ := filepath.Glob(filepath.Join(r.GitDir, "kero-index-*")); len(left) != 0 {
		t.Errorf("SetIndex left %v", left)
	}

	// A file added with intent to add has its text cleaned as a new one's.
	gitIn(t, r.Root, "config", "core.autocrlf", "true")
	write(t, filepath.Join(r.Root, "n.txt"), "n1\r\nn2\r\n")
	gitIn(t, r.Root, "add", "-N", "n.txt")
	if err := r.SetIndex(ctx, "n.txt", Hash(nil), "n1\r\n"); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.Root, "show", ":n.txt"); got != "n1\n" {
		t.Errorf("index has %q of a file added with intent to add", got)
	}
	gitIn(t, r.Root, "config", "core.autocrlf", "false")

	write(t, filepath.Join(r.Root, ".gitattributes"), "b.txt filter=upper\n")
	if err := r.SetIndex(ctx, "b.txt", Hash([]byte("b\n")), "c\n"); err == nil {
		t.Error("SetIndex wrote a file a filter changes")
	}
}

// A pop gives back what was staged too, and keeps the stash when it
// can't.
func TestStashPopIndex(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	write(t, filepath.Join(r.Root, "a.txt"), "staged\n")
	gitIn(t, r.Root, "add", "a.txt")
	write(t, filepath.Join(r.Root, "a.txt"), "a\n")
	if _, err := r.StashPush(ctx, StashOptions{}); err != nil {
		t.Fatal(err)
	}
	list, _ := r.Stashes(ctx)
	if len(list) != 1 {
		t.Fatalf("stashes %+v", list)
	}
	if _, err := r.StashApply(ctx, 0, list[0].Hash, true); err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, r.Root, "show", ":a.txt"); got != "staged\n" {
		t.Errorf("the pop staged %q, not what was", got)
	}
	if list, _ := r.Stashes(ctx); len(list) != 0 {
		t.Errorf("the pop kept %+v", list)
	}

	// Staged anew over what the stash staged, the index can't take it back.
	if _, err := r.StashPush(ctx, StashOptions{}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(r.Root, "a.txt"), "other\n")
	gitIn(t, r.Root, "add", "a.txt")
	list, _ = r.Stashes(ctx)
	if _, err := r.StashApply(ctx, 0, list[0].Hash, true); err == nil {
		t.Error("the pop dropped a stash it couldn't apply whole")
	}
	if list, _ := r.Stashes(ctx); len(list) != 1 {
		t.Errorf("stashes after the pop that failed: %+v", list)
	}
}

func TestIndexConflict(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	gitIn(t, r.Root, "switch", "-q", "-c", "other")
	write(t, filepath.Join(r.Root, "a.txt"), "other\n")
	gitIn(t, r.Root, "commit", "-q", "-am", "other")
	gitIn(t, r.Root, "switch", "-q", "main")
	write(t, filepath.Join(r.Root, "a.txt"), "main\n")
	gitIn(t, r.Root, "commit", "-q", "-am", "main")
	if _, err := r.write(ctx, "", nil, "merge", "other"); err == nil {
		t.Fatal("the merge had no conflict")
	} else if e := new(Error); !errors.As(err, &e) || !strings.Contains(e.Summary(), "CONFLICT") {
		t.Errorf("merge error summary %q", err)
	}
	staged, unstaged, err := r.Index(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 0 || paths(unstaged) != "a.txt:conflict" {
		t.Errorf("conflict: %s | %s", paths(staged), paths(unstaged))
	}
	if r.MergeMessage() == "" {
		t.Error("no merge message")
	}
	if err := r.SetIndex(ctx, "a.txt", "", "x\n"); err == nil {
		t.Error("SetIndex wrote over a conflict")
	}
	write(t, filepath.Join(r.Root, "a.txt"), "both\n")
	if err := r.Stage(ctx, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Continue(ctx); err != nil {
		t.Fatal(err)
	}
	if r.operation() != "" {
		t.Errorf("still in %s", r.operation())
	}
}

func TestCommit(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	if _, err := r.Commit(ctx, CommitOptions{Message: "  "}); err == nil {
		t.Error("committed with no message")
	}
	write(t, filepath.Join(r.Root, "a.txt"), "b\n")
	r.Stage(ctx, nil)
	if _, err := r.Commit(ctx, CommitOptions{Message: "Change a\n\nThe body.", Signoff: true}); err != nil {
		t.Fatal(err)
	}
	msg, err := r.LastMessage(ctx)
	if err != nil || !strings.HasPrefix(msg, "Change a\n\nThe body.") || !strings.Contains(msg, "Signed-off-by: t <t@t>") {
		t.Errorf("message %q, %v", msg, err)
	}
	if _, err := r.Commit(ctx, CommitOptions{Message: "Change a, amended", Amend: true}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(gitIn(t, r.Root, "log", "--oneline"), "\n"); n != 2 {
		t.Errorf("%d commits after amending; want 2", n)
	}
	// A hook's refusal comes back in its words.
	hook := filepath.Join(r.GitDir, "hooks", "pre-commit")
	write(t, hook, "#!/bin/sh\necho 'lint failed: a.txt' >&2\nexit 1\n")
	os.Chmod(hook, 0o755)
	write(t, filepath.Join(r.Root, "a.txt"), "c\n")
	r.Stage(ctx, nil)
	if _, err := r.Commit(ctx, CommitOptions{Message: "c"}); err == nil || !strings.Contains(err.(*Error).Output, "lint failed") {
		t.Errorf("hook's refusal: %v", err)
	}
	if _, err := r.Commit(ctx, CommitOptions{Message: "c", NoVerify: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.UndoCommit(ctx); err != nil {
		t.Fatal(err)
	}
	if staged, _, _ := r.Index(ctx); paths(staged) != "a.txt:modified" {
		t.Errorf("undone commit's changes: %s", paths(staged))
	}
}

func TestBranchesAndStash(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	if _, err := r.CreateBranch(ctx, "feature", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateBranch(ctx, "-x", "", false); err == nil {
		t.Error("made a branch named as an option")
	}
	if _, err := r.CreateBranch(ctx, "spare", "main", false); err != nil {
		t.Fatal(err)
	}
	list, err := r.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range list {
		names = append(names, b.Name)
		if b.Current != (b.Name == "feature") {
			t.Errorf("%s current %v", b.Name, b.Current)
		}
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "feature,main,spare" {
		t.Errorf("branches %v", names)
	}
	if _, err := r.RenameBranch(ctx, "spare", "spare2"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DeleteBranch(ctx, "spare2", false); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(r.Root, "a.txt"), "changed\n")
	write(t, filepath.Join(r.Root, "u.txt"), "untracked\n")
	if _, err := r.StashPush(ctx, StashOptions{Message: "wip", Untracked: true}); err != nil {
		t.Fatal(err)
	}
	if _, unstaged, _ := r.Index(ctx); len(unstaged) != 0 {
		t.Errorf("stashing left %s", paths(unstaged))
	}
	stashes, err := r.Stashes(ctx)
	if err != nil || len(stashes) != 1 || stashes[0].Index != 0 || !strings.Contains(stashes[0].Message, "wip") {
		t.Fatalf("stashes %+v, %v", stashes, err)
	}
	ci, err := r.Show(ctx, stashes[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if paths(ci.Files) != "a.txt:modified" || paths(ci.Untracked) != "u.txt:added" {
		t.Errorf("stash's files %s, untracked %s", paths(ci.Files), paths(ci.Untracked))
	}
	if txt, err := r.Texts(ctx, SideNone, ci.Parents[2], "u.txt", ""); err != nil || txt.New != "untracked\n" {
		t.Errorf("stash's untracked file %+v, %v", txt, err)
	}
	if _, err := r.StashApply(ctx, 0, "0000", false); err == nil {
		t.Error("applied a stash that changed")
	}
	if _, err := r.Switch(ctx, "main", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StashApply(ctx, 0, stashes[0].Hash, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "u.txt")); string(b) != "untracked\n" {
		t.Errorf("popped stash's untracked file %q", b)
	}
	if s, _ := r.Stashes(ctx); len(s) != 0 {
		t.Errorf("popping left %+v", s)
	}
	r.StashPush(ctx, StashOptions{})
	s, _ := r.Stashes(ctx)
	if _, err := r.StashDrop(ctx, 0, s[0].Hash); err != nil {
		t.Fatal(err)
	}
}

func TestLog(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	gitIn(t, r.Root, "switch", "-q", "-c", "side")
	write(t, filepath.Join(r.Root, "s.txt"), "s\n")
	gitIn(t, r.Root, "add", "-A")
	gitIn(t, r.Root, "commit", "-q", "-m", "side work")
	gitIn(t, r.Root, "switch", "-q", "main")
	write(t, filepath.Join(r.Root, "a.txt"), "a2\n")
	gitIn(t, r.Root, "commit", "-q", "-am", "main work")
	gitIn(t, r.Root, "merge", "-q", "--no-edit", "side")
	gitIn(t, r.Root, "tag", "v1")

	list, err := r.Log(ctx, LogOptions{Revs: []string{"HEAD"}, Max: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 || len(list[0].Parents) != 2 || !strings.Contains(list[0].Refs, "HEAD -> main") || !strings.Contains(list[0].Refs, "tag: v1") {
		t.Fatalf("log %+v", list)
	}
	// Each commit before its parents.
	at := map[string]int{}
	for i, c := range list {
		at[c.Hash] = i
	}
	for i, c := range list {
		for _, p := range c.Parents {
			if at[p] <= i {
				t.Errorf("%s comes before its child", p)
			}
		}
	}
	if fp, _ := r.Log(ctx, LogOptions{Revs: []string{"HEAD"}, FirstParent: true}); len(fp) != 3 {
		t.Errorf("first parents %d; want 3", len(fp))
	}
	if page, _ := r.Log(ctx, LogOptions{Revs: []string{"HEAD"}, Skip: 3, Max: 10}); len(page) != 1 || page[0].Subject != "first" {
		t.Errorf("last page %+v", page)
	}
	if g, _ := r.Log(ctx, LogOptions{All: true, Grep: "SIDE WORK"}); len(g) != 1 || g[0].Subject != "side work" {
		t.Errorf("grep %+v", g)
	}
	if p, _ := r.Log(ctx, LogOptions{Revs: []string{"HEAD"}, Path: "s.txt"}); len(p) != 1 {
		t.Errorf("history of s.txt %+v", p)
	}
	if _, err := r.Log(ctx, LogOptions{Revs: []string{"--all"}}); err == nil {
		t.Error("Log took an option as a revision")
	}
	ci, err := r.Show(ctx, list[0].Hash)
	if err != nil || ci.Message == "" || paths(ci.Files) != "s.txt:added" {
		t.Errorf("merge's files %s, %v", paths(ci.Files), err)
	}
	if un, err := r.Unpushed(ctx, false, 100); err != nil || len(un) != 4 {
		t.Errorf("unpushed %d, %v", len(un), err)
	}
}

// A branch made from origin/main tracks it: its push goes to a branch of
// its own name, which it then tracks, and never over main.
func TestPushBranchTrackingAnother(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitIn(t, r.Root, "init", "-q", "--bare", bare)
	gitIn(t, r.Root, "remote", "add", "origin", bare)
	gitIn(t, r.Root, "push", "-q", "-u", "origin", "main")
	main := strings.TrimSpace(gitIn(t, bare, "rev-parse", "main"))

	gitIn(t, r.Root, "switch", "-q", "-c", "feature", "origin/main")
	write(t, filepath.Join(r.Root, "f.txt"), "f\n")
	r.Stage(ctx, nil)
	r.Commit(ctx, CommitOptions{Message: "feature work"})
	args, err := r.PushArgs(ctx, "feature", PushOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(ctx, args, nil); err != nil {
		t.Fatalf("%q: %v", args, err)
	}
	if got := strings.TrimSpace(gitIn(t, bare, "rev-parse", "main")); got != main {
		t.Errorf("%q pushed over main", args)
	}
	if got := strings.TrimSpace(gitIn(t, bare, "rev-parse", "feature")); got != strings.TrimSpace(gitIn(t, r.Root, "rev-parse", "HEAD")) {
		t.Errorf("%q didn't push feature", args)
	}
	if st, _ := r.Status(ctx); st.Upstream != "origin/feature" {
		t.Errorf("upstream %q after the push", st.Upstream)
	}
}

func TestRemote(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitIn(t, r.Root, "init", "-q", "--bare", bare)
	gitIn(t, r.Root, "remote", "add", "origin", bare)

	push := func(o PushOptions, progress func(string)) (string, error) {
		t.Helper()
		args, err := r.PushArgs(ctx, "main", o)
		if err != nil {
			t.Fatal(err)
		}
		return r.Push(ctx, args, progress)
	}
	var lines []string
	if _, err := push(PushOptions{}, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.Upstream != "origin/main" {
		t.Errorf("upstream %q after the first push", st.Upstream)
	}
	if len(lines) == 0 {
		t.Error("no progress")
	}

	// Another clone pushes; this one is behind, and its push is rejected.
	other := filepath.Join(t.TempDir(), "other")
	gitIn(t, r.Root, "clone", "-q", bare, other)
	write(t, filepath.Join(other, "b.txt"), "b\n")
	gitIn(t, other, "add", "-A")
	gitIn(t, other, "commit", "-q", "-m", "theirs")
	gitIn(t, other, "push", "-q")
	write(t, filepath.Join(r.Root, "c.txt"), "c\n")
	r.Stage(ctx, nil)
	r.Commit(ctx, CommitOptions{Message: "ours"})
	if _, err := push(PushOptions{}, nil); err == nil {
		t.Error("pushed over the remote's commit")
	} else if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("rejection's summary %q", err)
	}
	if _, err := r.Fetch(ctx, FetchOptions{Prune: true}, nil); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.Ahead != 1 || st.Behind != 1 {
		t.Errorf("ahead %d behind %d", st.Ahead, st.Behind)
	}
	if _, err := r.Pull(ctx, PullFF, nil); err == nil {
		t.Error("fast-forward-only pull of diverged branches")
	}
	// Over a change not committed, which the pull stashes and puts back.
	write(t, filepath.Join(r.Root, "a.txt"), "a changed\n")
	if _, err := r.Pull(ctx, PullRebase, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(b) != "a changed\n" {
		t.Errorf("the change not committed is %q after the pull", b)
	}
	write(t, filepath.Join(r.Root, "a.txt"), "a\n")
	if _, err := push(PushOptions{Force: true}, nil); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(ctx); st.Ahead != 0 || st.Behind != 0 {
		t.Errorf("after the pull and push: ahead %d behind %d", st.Ahead, st.Behind)
	}

	// A push is of the branch alone, though push.default=matching would
	// push each of the remote's names.
	gitIn(t, r.Root, "push", "-q", "origin", "main:other")
	gitIn(t, r.Root, "branch", "other")
	gitIn(t, r.Root, "config", "push.default", "matching")
	write(t, filepath.Join(r.Root, "d.txt"), "d\n")
	r.Stage(ctx, nil)
	r.Commit(ctx, CommitOptions{Message: "on both"})
	gitIn(t, r.Root, "branch", "-f", "other", "HEAD")
	before := strings.TrimSpace(gitIn(t, bare, "rev-parse", "other"))
	if _, err := push(PushOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if after := strings.TrimSpace(gitIn(t, bare, "rev-parse", "other")); after != before {
		t.Error("the push of main pushed other too")
	}
}

// A path is a path, not a pattern: discarding f[ab] leaves fa and fb, and
// an untracked u[xy] goes alone.
func TestLiteralPaths(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"f[ab]": "1\n", "fa": "a\n", "fb": "b\n"})
	write(t, filepath.Join(r.Root, "f[ab]"), "2\n")
	write(t, filepath.Join(r.Root, "fa"), "A\n")
	write(t, filepath.Join(r.Root, "u[xy]"), "u\n")
	write(t, filepath.Join(r.Root, "ux"), "x\n")
	if err := r.Stage(ctx, []string{"f[ab]"}); err != nil {
		t.Fatal(err)
	}
	if staged, _, _ := r.Index(ctx); paths(staged) != "f[ab]:modified" {
		t.Errorf("staged %s", paths(staged))
	}
	if err := r.Unstage(ctx, []string{"f[ab]"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Discard(ctx, []string{"f[ab]"}, []string{"u[xy]"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Root, "fa")); string(b) != "A\n" {
		t.Errorf("fa discarded with f[ab]: %q", b)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "ux")); err != nil {
		t.Errorf("ux deleted with u[xy]: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "u[xy]")); !os.IsNotExist(err) {
		t.Errorf("u[xy] not deleted: %v", err)
	}
}

// A message is kept as written, its lines starting with # too; undoing a
// branch's first commit leaves it unborn, its files staged.
func TestCommitMessageAndFirstUndo(t *testing.T) {
	ctx := context.Background()
	r := opsRepo(t, map[string]string{"a.txt": "a\n"})
	write(t, filepath.Join(r.Root, "a.txt"), "b\n")
	r.Stage(ctx, nil)
	if _, err := r.Commit(ctx, CommitOptions{Message: "#12 Fix a\n\nThe body."}); err != nil {
		t.Fatal(err)
	}
	if msg, _ := r.LastMessage(ctx); msg != "#12 Fix a\n\nThe body." {
		t.Errorf("message %q", msg)
	}
	if err := r.UndoCommit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.UndoCommit(ctx); err != nil {
		t.Fatal(err)
	}
	if r.hasHead(ctx) {
		t.Error("the first commit undone, HEAD is still a commit")
	}
	if staged, _, _ := r.Index(ctx); paths(staged) != "a.txt:added" {
		t.Errorf("staged after undoing the first commit: %s", paths(staged))
	}
	if err := r.Unstage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if staged, _, _ := r.Index(ctx); len(staged) != 0 {
		t.Errorf("staged after unstaging all with no commit: %s", paths(staged))
	}
}
