package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestPasteFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", tmp)
		t.Setenv("TEMP", tmp)
	}
	r, err := pasteFile(proto.PasteFileParams{Name: "../../etc/shot one.png", Data: []byte("png")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Path, pasteDir()) || filepath.Base(r.Path) != "shot one.png" {
		t.Errorf("pasted at %s, not in %s", r.Path, pasteDir())
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
