package docs

import (
	"strings"
	"testing"
)

// Every page has the title and description Ama's list of the docs gives
// it, and is in the website's sidebar, which orders the list; the sidebar
// lists no page that isn't there.
func TestPages(t *testing.T) {
	pages := Pages()
	if len(pages) == 0 || pages[0].Name != "index.md" {
		t.Fatalf("pages don't start at the sidebar's first, index.md: %v", pages)
	}
	order := sidebarOrder()
	seen := map[string]bool{}
	for _, p := range pages {
		seen[p.Name] = true
		if p.Title == "" || p.Description == "" {
			t.Errorf("%s: no title or description in its frontmatter", p.Name)
		}
		if _, ok := order[p.Name]; !ok {
			t.Errorf("%s isn't in sidebar.json", p.Name)
		}
		if !strings.Contains(string(p.Text), "# ") {
			t.Errorf("%s: no heading", p.Name)
		}
	}
	for name := range order {
		if !seen[name] {
			t.Errorf("sidebar.json lists %s, which isn't there", name)
		}
	}
}

func TestFrontmatter(t *testing.T) {
	title, desc := frontmatter([]byte("---\r\ntitle: Worktrees\r\ndescription: Give each agent a worktree: its own.\r\n---\r\n\r\n# Worktrees\r\n"))
	if title != "Worktrees" || desc != "Give each agent a worktree: its own." {
		t.Errorf("frontmatter = %q, %q", title, desc)
	}
	if title, desc := frontmatter([]byte("# No frontmatter\n")); title != "" || desc != "" {
		t.Errorf("no frontmatter = %q, %q", title, desc)
	}
}
