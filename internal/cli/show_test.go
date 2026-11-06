package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kero/internal/proto"
)

// showFlags parses kero split's flags of what it shows.
func showFlags(t *testing.T, args ...string) *flags {
	t.Helper()
	f, err := parse(args, map[string]string{"u": "url", "url": "url", "file": "file"}, map[string]string{"preview": "preview", "wait": "wait"})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// --preview shows a Markdown file, and nothing else; --file and --url are
// made absolute, of the shell's folder.
func TestShownIn(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, ".backlog", "pages"), 0o755)
	os.WriteFile(filepath.Join(dir, ".backlog", "pages", "brief.html"), []byte("<p>brief"), 0o644)

	page, file, err := shownIn("split", showFlags(t, "--file", "plan.md", "--preview"), nil, "")
	if err != nil || page != "" || file != filepath.Join(dir, "plan.md") {
		t.Errorf("--file plan.md --preview: %q %q %v", page, file, err)
	}
	for _, args := range [][]string{
		{"--file", "main.go", "--preview"},
		{"--preview"},
		{"--url", "localhost:3000", "--preview"},
		{"--file", "plan.md", "--url", "localhost:3000"},
		{"--file", "plan.md", "--wait"},
	} {
		if _, _, err := shownIn("split", showFlags(t, args...), nil, ""); !errors.Is(err, errUsage) {
			t.Errorf("%q: %v, want a usage error", args, err)
		}
	}
	if _, _, err := shownIn("new", showFlags(t, "--url", "localhost:3000"), []string{"npm", "start"}, ""); !errors.Is(err, errUsage) {
		t.Errorf("--url with a command: %v", err)
	}
	page, _, err = shownIn("split", showFlags(t, "--url", ".backlog/pages/brief.html"), nil, "")
	if err != nil || page != proto.FileURL(filepath.Join(dir, ".backlog", "pages", "brief.html")) {
		t.Errorf("--url of a local page: %q %v", page, err)
	}
}

// A local file kero split --url names is its file:// URL; what names no
// file stays for kerod, but a missing HTML file is a mistake; on another
// host nothing is of this disk.
func TestLocalPage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	brief := filepath.Join(dir, "brief.html")
	os.WriteFile(brief, []byte("<p>brief"), 0o644)
	for in, want := range map[string]string{
		"brief.html":             proto.FileURL(brief),
		"./brief.html":           proto.FileURL(brief),
		brief:                    proto.FileURL(brief),
		"localhost:3000":         "localhost:3000",
		"kero.sh":                "kero.sh",
		"file:///C:/x/a.html":    "file:///C:/x/a.html",
		"https://kero.sh/a.html": "https://kero.sh/a.html",
		"":                       "",
	} {
		if got, err := localPage(in, ""); err != nil || got != want {
			t.Errorf("localPage(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := localPage("pages/none.html", ""); !errors.Is(err, errNotFound) {
		t.Errorf("a missing page: %v", err)
	}
	if got, _ := localPage("brief.html", "devbox"); got != "brief.html" {
		t.Errorf("on another host: %q", got)
	}
}

// kero do pane.preview finds the pane named, else the session's focused
// editor of Markdown, else its one; a terminal named, or none, or several,
// it tells of.
func TestEditorPaneTarget(t *testing.T) {
	s := proto.Session{ID: "s1", Focus: "t", Panes: []proto.Pane{
		{ID: "t", Shell: "zsh"},
		{ID: "p", File: "/x/plan.md"},
		{ID: "g", File: "/x/main.go"},
	}}
	for _, c := range []struct {
		t     target
		named bool
		want  string
	}{
		{target{s: s, pane: "t"}, false, "p"},
		{target{s: s}, false, "p"},
		{target{s: s, pane: "p"}, true, "p"},
		{target{s: s, pane: "t"}, true, ""},
		{target{s: s, pane: "g"}, true, ""},
	} {
		p, err := editorPane(c.t, c.named)
		switch {
		case c.want == "" && err == nil:
			t.Errorf("%+v: found %s", c, p.ID)
		case c.want != "" && (err != nil || p.ID != c.want):
			t.Errorf("%+v: %v, want %s", c, err, c.want)
		}
	}
	two := s
	two.Panes = append(two.Panes, proto.Pane{ID: "q", File: "/x/notes.md"})
	if _, err := editorPane(target{s: two}, false); err == nil || !strings.Contains(err.Error(), "p, q") {
		t.Errorf("of two: %v", err)
	}
	two.Focus = "q"
	if p, err := editorPane(target{s: two}, false); err != nil || p.ID != "q" {
		t.Errorf("of two, the focused: %v", err)
	}
}
