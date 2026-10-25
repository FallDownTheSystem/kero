package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"kero/internal/git"
	"kero/internal/proto"
)

func TestHostPath(t *testing.T) {
	home := proto.Home()
	abs := filepath.Join(home, "x")
	for in, want := range map[[2]string]string{
		{"", ""}:             home,
		{"~", ""}:            home,
		{"~/code", ""}:       filepath.Join(home, "code"),
		{abs, "/elsewhere"}:  abs,
		{"src/a.go", abs}:    filepath.Join(abs, "src", "a.go"),
		{"src/a.go", ""}:     filepath.Join(home, "src", "a.go"),
		{"../y", abs}:        filepath.Join(home, "y"),
		{"lib", "~/project"}: filepath.Join(home, "project", "lib"),
	} {
		if got := hostPath(in[0], in[1]); got != want {
			t.Errorf("hostPath(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// fs.find finds names in all of a folder, of any case, and paths with a
// slash; never dotfiles unless asked, nor what is in a folder git ignores.
func TestFSFind(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	for _, f := range []string{"src/Main.go", "src/util/main_test.go", "docs/readme.md", ".env/main.txt", "node_modules/main/index.js"} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755)
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	found := func(p proto.FSFindParams) string {
		t.Helper()
		p.Dir = dir
		r, err := fsFind(p)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range r.Entries {
			s := e.Path
			if e.Dir {
				s += "/"
			}
			got = append(got, s)
		}
		slices.Sort(got)
		return strings.Join(got, ",")
	}
	if got := found(proto.FSFindParams{Query: "MAIN"}); got != "src/Main.go,src/util/main_test.go" {
		t.Errorf("main: %s", got)
	}
	if got := found(proto.FSFindParams{Query: "node"}); got != "node_modules/" {
		t.Errorf("an ignored folder: %s", got)
	}
	if got := found(proto.FSFindParams{Query: "main", Hidden: true}); !strings.Contains(got, ".env/main.txt") || strings.Contains(got, "index.js") {
		t.Errorf("main with dotfiles: %s", got)
	}
	if got := found(proto.FSFindParams{Query: `util\main`}); got != "src/util/main_test.go" {
		t.Errorf("a path: %s", got)
	}
	if got := found(proto.FSFindParams{Query: " "}); got != "" {
		t.Errorf("nothing asked: %s", got)
	}
	if _, err := fsFind(proto.FSFindParams{Dir: filepath.Join(dir, "none"), Query: "x"}); err == nil {
		t.Error("found in a folder that is none")
	}
}

func TestFSListAndStat(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"b-dir", "A-dir", ".hidden", "repo/.git"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hi"), 0o644)
	l, err := fsList(proto.FSListParams{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range l.Entries {
		names = append(names, e.Name)
		if e.Name == "repo" && !e.Repo {
			t.Error("repo is not a repository")
		}
	}
	if got := strings.Join(names, ","); got != "A-dir,b-dir,repo,file.txt" {
		t.Errorf("listed %s", got)
	}
	if l.Parent != filepath.Dir(dir) || l.Dir != dir {
		t.Errorf("dir %q, parent %q", l.Dir, l.Parent)
	}
	if l, _ := fsList(proto.FSListParams{Dir: dir, Hidden: true}); len(l.Entries) != 5 {
		t.Errorf("with hidden: %d entries", len(l.Entries))
	}
	if _, err := fsList(proto.FSListParams{Dir: filepath.Join(dir, "none")}); err == nil {
		t.Error("listed a directory that is none")
	}
	st := fsStat(proto.FSStatParams{Path: "file.txt", Base: dir})
	if !st.Exists || st.Dir || st.Size != 2 || st.Path != filepath.Join(dir, "file.txt") {
		t.Errorf("stat %+v", st)
	}
	if st := fsStat(proto.FSStatParams{Path: "nope", Base: dir}); st.Exists {
		t.Errorf("stat of nothing %+v", st)
	}
	if r, err := fsRead(proto.FSReadParams{Path: filepath.Join(dir, "file.txt")}); err != nil || string(r.Data) != "hi" {
		t.Errorf("read %q, %v", r.Data, err)
	}
	if _, err := fsRead(proto.FSReadParams{Path: filepath.Join(dir, "file.txt"), Max: 1}); err == nil {
		t.Error("read past Max")
	}
	if _, err := fsRead(proto.FSReadParams{Path: dir}); err == nil {
		t.Error("read a directory")
	}
}

// A save writes the file only as it was read, and through a link to the
// file it names.
func TestFSWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("old"), 0o644)
	w, err := fsWrite(proto.FSWriteParams{Path: path, Hash: git.Hash([]byte("old")), Data: []byte("new")})
	if err != nil || w.Changed || w.Hash != git.Hash([]byte("new")) {
		t.Fatalf("wrote %+v, %v", w, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Errorf("the file holds %q", b)
	}
	// Another program wrote it since it was read.
	if w, err := fsWrite(proto.FSWriteParams{Path: path, Hash: git.Hash([]byte("old")), Data: []byte("mine")}); err != nil || !w.Changed {
		t.Errorf("wrote over another's change: %+v, %v", w, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Errorf("the file holds %q after a refused write", b)
	}
	if _, err := fsWrite(proto.FSWriteParams{Path: filepath.Join(dir, "none"), Hash: git.Hash([]byte("x")), Data: []byte("x")}); err == nil {
		t.Error("wrote a file that is gone")
	}
	// A file the editor found none of is made, unless one is there since.
	fresh := filepath.Join(dir, "fresh.go")
	if w, err := fsWrite(proto.FSWriteParams{Path: fresh, Data: []byte("made")}); err != nil || w.Changed || w.Hash != git.Hash([]byte("made")) {
		t.Errorf("made %+v, %v", w, err)
	}
	if b, _ := os.ReadFile(fresh); string(b) != "made" {
		t.Errorf("the new file holds %q", b)
	}
	if w, err := fsWrite(proto.FSWriteParams{Path: fresh, Data: []byte("again")}); err != nil || !w.Changed {
		t.Errorf("made a file over one: %+v, %v", w, err)
	}
	if _, err := fsWrite(proto.FSWriteParams{Path: dir, Data: []byte("x")}); err == nil {
		t.Error("wrote a directory")
	}
	link := filepath.Join(dir, "link.go")
	if err := os.Symlink(path, link); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if _, err := fsWrite(proto.FSWriteParams{Path: link, Hash: git.Hash([]byte("new")), Data: []byte("linked")}); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Error("the write replaced the link")
	}
	if b, _ := os.ReadFile(path); string(b) != "linked" {
		t.Errorf("the linked file holds %q", b)
	}
}

// New File and New Folder make what they name, in folders made as need be,
// and never over what is there.
func TestFSCreate(t *testing.T) {
	dir := t.TempDir()
	r, err := fsCreate(proto.FSCreateParams{Path: filepath.Join(dir, "src", "main.go")})
	if err != nil || r.Path != filepath.Join(dir, "src", "main.go") {
		t.Fatalf("created %+v, %v", r, err)
	}
	if b, err := os.ReadFile(r.Path); err != nil || len(b) != 0 {
		t.Errorf("the file holds %q, %v", b, err)
	}
	os.WriteFile(r.Path, []byte("mine"), 0o644)
	if _, err := fsCreate(proto.FSCreateParams{Path: r.Path}); err == nil {
		t.Error("created a file over one")
	}
	if b, _ := os.ReadFile(r.Path); string(b) != "mine" {
		t.Errorf("the file holds %q after a refused create", b)
	}
	if _, err := fsCreate(proto.FSCreateParams{Path: filepath.Join(dir, "src"), Dir: true}); err == nil {
		t.Error("created a folder over one")
	}
	if r, err := fsCreate(proto.FSCreateParams{Path: filepath.Join(dir, "docs", "api"), Dir: true}); err != nil {
		t.Fatal(err)
	} else if st, err := os.Stat(r.Path); err != nil || !st.IsDir() {
		t.Errorf("no folder at %s: %v", r.Path, err)
	}
}

func TestPasteFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("XDG_RUNTIME_DIR", "")
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", tmp)
		t.Setenv("TEMP", tmp)
	}
	r, err := pasteFile(proto.PasteFileParams{Name: "../../etc/shot one.png", Data: []byte("png")})
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := pasteDir()
	if !strings.HasPrefix(r.Path, dir) || filepath.Base(r.Path) != "shot one.png" {
		t.Errorf("pasted at %s, not in %s", r.Path, dir)
	}
	if b, _ := os.ReadFile(r.Path); string(b) != "png" {
		t.Errorf("pasted %q", b)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(filepath.Dir(filepath.Dir(r.Path))); st.Mode().Perm() != 0o700 {
			t.Errorf("the paste directory is %v", st.Mode().Perm())
		}
	}
	// Two of one name stay apart.
	r2, _ := pasteFile(proto.PasteFileParams{Name: "shot one.png", Data: []byte("2")})
	if r2.Path == r.Path {
		t.Error("a second paste of a name took the first's place")
	}
	if _, err := pasteFile(proto.PasteFileParams{Name: "big", Data: make([]byte, maxPaste+1)}); err == nil {
		t.Error("pasted a file past the limit")
	}
	if r, _ := pasteFile(proto.PasteFileParams{Name: "", Data: []byte("x")}); filepath.Base(r.Path) != "pasted" {
		t.Errorf("a file of no name pasted as %s", r.Path)
	}
}

// A paste directory someone else could reach is made private, or refused
// when it is not the user's to make so.
func TestPasteDirPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are Unix's")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("XDG_RUNTIME_DIR", "")
	base := filepath.Join(tmp, fmt.Sprintf("kero-%d", os.Getuid()))
	os.Mkdir(base, 0o777)
	os.Chmod(base, 0o777)
	if _, err := pasteDir(); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(base); st.Mode().Perm() != 0o700 {
		t.Errorf("the base is %v", st.Mode().Perm())
	}
	// A link where the directory goes, as another user could plant.
	os.RemoveAll(base)
	os.Symlink(t.TempDir(), base)
	if _, err := pasteDir(); err == nil {
		t.Error("pasted through a link")
	}
}
