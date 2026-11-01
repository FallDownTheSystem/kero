package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kero/internal/proto"
)

// project makes a folder of files, by their paths and texts, a repository
// with git, which ignores node_modules.
func project(t *testing.T, repo bool, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for f, text := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755)
		os.WriteFile(filepath.Join(dir, f), []byte(text), 0o644)
	}
	if repo {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("no git")
		}
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644)
		if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, out)
		}
	}
	return dir
}

// fs.files lists a repository's files as git does, without what it
// ignores or what was deleted, and dotfiles only when asked; any other
// folder's, all of them.
func TestFSFiles(t *testing.T) {
	files := map[string]string{"src/main.go": "", "README.md": "", ".env": "", "node_modules/x/index.js": "", "gone.txt": ""}
	dir := project(t, true, files)
	if out, err := exec.Command("git", "-C", dir, "add", "gone.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	os.Remove(filepath.Join(dir, "gone.txt"))
	list := func(dir string, hidden bool) string {
		t.Helper()
		r, err := fsFiles(proto.FSFilesParams{Dir: dir, Hidden: hidden})
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(r.Paths, ",")
	}
	if got := list(dir, false); got != "README.md,src/main.go" {
		t.Errorf("a repository's files: %s", got)
	}
	if got := list(dir, true); got != ".env,.gitignore,README.md,src/main.go" {
		t.Errorf("with dotfiles: %s", got)
	}
	plain := project(t, false, map[string]string{"a/b.txt": "", "c.txt": "", ".hidden/d.txt": ""})
	if got := list(plain, false); got != "a/b.txt,c.txt" {
		t.Errorf("a folder's files: %s", got)
	}
	if _, err := fsFiles(proto.FSFilesParams{Dir: filepath.Join(plain, "c.txt")}); err == nil {
		t.Error("listed a file's files")
	}
}

// fs.search finds the lines that hold the query, in the files fs.files
// lists, as asked: of any case, by case, as a word, or as a pattern; never
// in a binary file.
func TestFSSearch(t *testing.T) {
	dir := project(t, true, map[string]string{
		"a.go":                    "package a\r\n\r\nfunc Hello() {}\r\n\t// hello world\r\n",
		"b/c.txt":                 "say HELLO\nshell\n",
		"bin.dat":                 "hello\x00world",
		"node_modules/x/index.js": "hello",
	})
	search := func(p proto.FSSearchParams) string {
		t.Helper()
		p.Dir = dir
		r, err := fsSearch(p)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, f := range r.Files {
			for _, m := range f.Matches {
				got = append(got, fmt.Sprintf("%s:%d:%s", f.Path, m.Line, m.Text[m.Start:m.End]))
			}
		}
		return strings.Join(got, ",")
	}
	if got := search(proto.FSSearchParams{Query: "hello"}); got != "a.go:3:Hello,a.go:4:hello,b/c.txt:1:HELLO" {
		t.Errorf("any case: %s", got)
	}
	if got := search(proto.FSSearchParams{Query: "Hello", Case: true}); got != "a.go:3:Hello" {
		t.Errorf("by case: %s", got)
	}
	if got := search(proto.FSSearchParams{Query: "hell", Word: true}); got != "" {
		t.Errorf("a word: %s", got)
	}
	if got := search(proto.FSSearchParams{Query: `s?h[e]ll\b`, Regex: true}); got != "b/c.txt:2:shell" {
		t.Errorf("a pattern: %s", got)
	}
	if _, err := fsSearch(proto.FSSearchParams{Dir: dir, Query: "(", Regex: true}); err == nil {
		t.Error("an invalid pattern searched")
	}
	r, _ := fsSearch(proto.FSSearchParams{Dir: dir, Query: "world"})
	if len(r.Files) != 1 || r.Files[0].Matches[0].Text != "// hello world" {
		t.Errorf("a line's text shows without its indentation: %+v", r.Files)
	}
}

// A long line's match shows with some of the line around it, cut where
// the … are.
func TestMatchOf(t *testing.T) {
	line := []byte("\t\t" + strings.Repeat("a", 400) + "NEEDLE" + strings.Repeat("b", 400))
	m := matchOf(7, line, 402, 408)
	if m.Text[m.Start:m.End] != "NEEDLE" || !strings.HasPrefix(m.Text, "…a") || !strings.HasSuffix(m.Text, "b…") || len(m.Text) > matchText+2*len("…") {
		t.Errorf("cut: %q %d-%d", m.Text, m.Start, m.End)
	}
	bad := []byte("x\xffy NEEDLE")
	if m := matchOf(1, bad, 4, 10); m.Text[m.Start:m.End] != "NEEDLE" {
		t.Errorf("after a byte not UTF-8: %q %d-%d", m.Text, m.Start, m.End)
	}
}
