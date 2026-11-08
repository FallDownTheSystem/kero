// Package docs is Kero's user docs: the Markdown pages the website renders
// at /docs, in English, built into Kero so that what reads them where Kero
// runs reads those of its own version. Ama gets them in its folder.
package docs

import (
	"bytes"
	"cmp"
	"embed"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
)

//go:embed *.md sidebar.json
var files embed.FS

// Page is a page of the docs.
type Page struct {
	// Name is its file, as "worktrees.md".
	Name string
	// Title and Description are its frontmatter's.
	Title, Description string
	// Text is the whole file, its frontmatter too.
	Text []byte
}

// Pages are the docs' pages, in the order the website's sidebar lists
// them, then those it doesn't, by name.
func Pages() []Page {
	names, _ := fs.Glob(files, "*.md")
	order := sidebarOrder()
	slices.SortStableFunc(names, func(a, b string) int {
		ia, oka := order[a]
		ib, okb := order[b]
		switch {
		case oka && okb:
			return cmp.Compare(ia, ib)
		case oka:
			return -1
		case okb:
			return 1
		}
		return strings.Compare(a, b)
	})
	pages := make([]Page, 0, len(names))
	for _, name := range names {
		b, err := files.ReadFile(name)
		if err != nil {
			continue
		}
		p := Page{Name: name, Text: b}
		p.Title, p.Description = frontmatter(b)
		pages = append(pages, p)
	}
	return pages
}

// sidebarOrder is each page's place in sidebar.json, by its file's name.
func sidebarOrder() map[string]int {
	var sections []struct {
		Items []string `json:"items"`
	}
	order := map[string]int{}
	b, err := files.ReadFile("sidebar.json")
	if err != nil || json.Unmarshal(b, &sections) != nil {
		return order
	}
	for _, s := range sections {
		for _, item := range s.Items {
			if _, ok := order[item+".md"]; !ok {
				order[item+".md"] = len(order)
			}
		}
	}
	return order
}

// frontmatter is the title and description of a page's frontmatter, the
// lines between its first two "---".
func frontmatter(b []byte) (title, description string) {
	lines := strings.Split(string(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		switch v = strings.TrimSpace(v); strings.TrimSpace(k) {
		case "title":
			title = v
		case "description":
			description = v
		}
	}
	return title, description
}
