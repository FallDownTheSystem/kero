package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"kero/internal/proto"
)

func TestChangeTree(t *testing.T) {
	var files []proto.GitFile
	for _, p := range []string{"README.md", "src/app/main.go", "src/app/util/x.go", "docs/a/b/c.md", "src/b.go", "Zed.txt", "src/App.go"} {
		files = append(files, proto.GitFile{Path: p})
	}
	show := func(open func(string) bool) []string {
		var out []string
		for _, e := range changeTree(files, open) {
			if e.folder {
				out = append(out, fmt.Sprintf("%s%s/ %d", strings.Repeat("  ", e.depth), e.label, e.count))
			} else {
				out = append(out, strings.Repeat("  ", e.depth)+e.f.Path)
			}
		}
		return out
	}
	// Folders first, each by name; a folder of one folder alone is one row.
	want := []string{
		"docs/a/b/ 1",
		"  docs/a/b/c.md",
		"src/ 4",
		"  app/ 2",
		"    util/ 1",
		"      src/app/util/x.go",
		"    src/app/main.go",
		"  src/App.go",
		"  src/b.go",
		"README.md",
		"Zed.txt",
	}
	if got := show(func(string) bool { return true }); !slices.Equal(got, want) {
		t.Errorf("tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// A folded folder hides what it holds.
	want = []string{"docs/a/b/ 1", "  docs/a/b/c.md", "src/ 4", "README.md", "Zed.txt"}
	if got := show(func(dir string) bool { return dir != "src" }); !slices.Equal(got, want) {
		t.Errorf("src folded: %q", got)
	}
}
