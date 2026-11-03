package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

// The Changes tab's tree is the files changed, in their folders: a click
// on a file opens the viewer on their diffs, scrolled to it, one on a
// folder folds it, and while the viewer shows them the file whose diff is
// at the top shows as the one there, the tree scrolling along to it. It
// lists them in the diffs' order, the worktree's paths', so that it reads
// down as they do.

// treeNode is a folder of the tree, or a file: a folder with a folder
// alone in it is one, named "internal/auth", as GitHub shows them.
type treeNode struct {
	name string
	path string // the folder's, or the file's
	file int    // the file's index in the list, -1 for a folder
	kids []*treeNode
}

// treeItem is a row of the tree as it shows, its folders' folds as the
// user left them.
type treeItem struct {
	*treeNode
	depth int
	open  bool
}

// fileTree makes the tree of a list's files, in their order.
func fileTree(files []proto.GitFile) []*treeNode {
	root := &treeNode{file: -1}
	dirs := map[string]*treeNode{}
	for i, f := range files {
		parent := root
		parts := strings.Split(f.Path, "/")
		for j, name := range parts[:len(parts)-1] {
			p := strings.Join(parts[:j+1], "/")
			d := dirs[p]
			if d == nil {
				d = &treeNode{name: name, path: p, file: -1}
				dirs[p] = d
				parent.kids = append(parent.kids, d)
			}
			parent = d
		}
		parent.kids = append(parent.kids, &treeNode{name: parts[len(parts)-1], path: f.Path, file: i})
	}
	var squash func(n *treeNode)
	squash = func(n *treeNode) {
		for _, k := range n.kids {
			for k.file < 0 && len(k.kids) == 1 && k.kids[0].file < 0 {
				only := k.kids[0]
				k.name, k.path, k.kids = k.name+"/"+only.name, only.path, only.kids
			}
			squash(k)
		}
	}
	squash(root)
	return root.kids
}

// treeItems are the rows of a tree, but those inside a folder closed.
func treeItems(nodes []*treeNode, closed func(dir string) bool) []treeItem {
	var items []treeItem
	var walk func(nodes []*treeNode, depth int)
	walk = func(nodes []*treeNode, depth int) {
		for _, n := range nodes {
			open := n.file < 0 && !closed(n.path)
			items = append(items, treeItem{n, depth, open})
			if open {
				walk(n.kids, depth+1)
			}
		}
	}
	walk(nodes, 0)
	return items
}

// treeCache is the tree of a list's files, and the list it was made of.
type treeCache struct {
	of    []proto.GitFile
	nodes []*treeNode
}

// sectionTree is the tree of a section's files, made again only as its
// list changes.
func (a *App) sectionTree(section string, files []proto.GitFile) []*treeNode {
	ch := &a.side.changes
	if ch.trees == nil {
		ch.trees = map[string]treeCache{}
	}
	t, ok := ch.trees[section]
	if !ok || !slices.Equal(t.of, files) {
		t = treeCache{of: files, nodes: fileTree(files)}
		ch.trees[section] = t
	}
	return t.nodes
}

// sideRow is a row of the Changes tab's tree: a section's title, in
// Uncommitted, or a folder or file of a section.
type sideRow struct {
	section string
	title   bool
	bar     bool // the stash's name and button, under its files
	files   []proto.GitFile // the section's
	treeItem
}

// changesTree is the tree of the files the Changes tab lists, in
// Uncommitted under their sections' titles, with the buttons that stage,
// unstage and discard them.
func (a *App) changesTree(c *ui.Context, pal *palette, s *proto.Session, cf *changesFrame) {
	ch := &a.side.changes
	scope := cf.scope
	var rows []sideRow
	for _, sec := range a.treeSections(cf) {
		if sec.name != "" {
			rows = append(rows, sideRow{section: sec.name, title: true, files: sec.files})
		}
		nodes := a.sectionTree(sec.name, sec.files)
		for _, it := range treeItems(nodes, func(dir string) bool { return ch.treeClosed[fileKey(scope, sec.name, dir)] }) {
			rows = append(rows, sideRow{section: sec.name, files: sec.files, treeItem: it})
		}
		if sec.name == baseStash {
			rows = append(rows, sideRow{section: sec.name, bar: true, files: sec.files})
		}
	}

	// The file there, while the viewer shows the diffs: the one whose diff
	// is at the top; at the end of the list, where the file chosen last
	// may not come to the top, that one while it shows.
	cur := ""
	if ls := cf.ls; ls != nil {
		first, last := ls.Visible()
		if first >= 0 && first < len(cf.rows) && !cf.rows[first].title {
			r := cf.rows[first]
			cur = fileKey(scope, r.section, r.f.Path)
		}
		if i, ok := cf.heads[ch.picked]; ok && ls.AtEnd() && i >= first && i <= last {
			cur = ch.picked
		}
	}

	if ch.treePlaces == nil {
		ch.treePlaces = map[string]*ui.ListState{}
	}
	tl := ch.treePlaces[scope]
	if tl == nil {
		tl = &ui.ListState{}
		ch.treePlaces[scope] = tl
	}
	tl.Key = func(i int) any {
		r := rows[i]
		if r.title || r.bar {
			return [3]any{"title", r.section, r.bar}
		}
		return [2]any{r.section, treeKey{r.path, r.file < 0}}
	}
	if cur != ch.current {
		ch.current = cur
		for i, r := range rows {
			if !r.title && r.file >= 0 && fileKey(scope, r.section, r.path) == cur {
				tl.ScrollIntoView(i)
			}
		}
	}
	here, ix := cf.here, cf.ix
	ui.List(c.Key("tree"), tl, len(rows), func(i int) {
		r := rows[i]
		switch {
		case r.title:
			a.indexTitle(c, pal, here, r.section, r.files, i > 0)
			return
		case r.bar:
			a.stashBar(c, pal, here, scope, r.files)
			return
		}
		if r.file < 0 {
			var tools func()
			if r.section != "" {
				dir := r.path
				tools = func() { a.indexListTools(c, pal, here, r.section, r.files, dir) }
			}
			row := a.treeFolder(c, pal, r.name, r.path, r.depth, r.open, false, func() {
				if ch.treeClosed == nil {
					ch.treeClosed = map[string]bool{}
				}
				ch.treeClosed[fileKey(scope, r.section, r.path)] = r.open
			}, tools)
			if menu := a.ignoreItems(cf, r.section, r.path, true); menu != nil {
				row.ContextMenu(menu)
			}
			return
		}
		f := r.files[r.file]
		// A file to stash shows its diff under its own section.
		key := fileKey(scope, diffSection(ix, r.section, f.Path), f.Path)
		var tools func()
		if r.section != "" {
			tools = func() { a.indexTools(c, pal, here, ix, r.section, f) }
		}
		row := a.treeFile(c, pal, r.treeItem, f, key == cur, func() {
			ch.picked = key
			a.focusViewer()
			if i, ok := cf.heads[key]; ok && cf.ls != nil {
				cf.ls.ScrollTo(i, ui.Start)
				return
			}
			ch.reveal = key
			a.openView(s, sideChanges, nil)
		}, tools)
		// Its menu opens it, and adds it to .gitignore; it drags onto a
		// pane, to open there (paneCard).
		full := joinRel(cf.list.Root, f.Path)
		gone := f.Status == "deleted" || cf.list.Root == ""
		menu := a.ignoreItems(cf, r.section, f.Path, false)
		pid := cf.pane.ID
		if !gone || menu != nil {
			row.ContextMenu(func(m *ui.Menu) {
				if !gone {
					a.openItems(m, pid, full, 0)
				}
				if menu != nil {
					if !gone {
						m.Separator()
					}
					menu(m)
				}
			})
		}
		if !gone {
			row.Drag(fileDrag{pane: pid, path: full})
			if row.Dragging() {
				row.Opacity(0.4)
			}
		}
	}).Grow(1).MinHeight(0).Padding(0, 10, 16, 10)
}

// diffSection is the section of the viewer a file of the tree's section
// shows its diff in: its own, or for one to stash, the one git lists it
// in, not staged first.
func diffSection(ix *proto.GitIndex, section, path string) string {
	if section != baseStash || ix == nil {
		return section
	}
	if slices.ContainsFunc(ix.Unstaged, func(f proto.GitFile) bool { return f.Path == path }) {
		return baseUnstaged
	}
	return baseStaged
}

// treeKey is what a row of a tree is, as its list keeps its state by: a
// file and a folder may have the same path, as one became the other.
type treeKey struct {
	path string
	dir  bool
}

// treeRow is a row of a tree, depth folders in, the one there chosen as
// a session's row is.
func treeRow(c *ui.Context, pal *palette, depth int, current bool) ui.Element {
	row := ui.Row(c.Key("row")).Height(26).Padding(0, 8, 0, 6+12*float32(depth)).Gap(5).AlignItems(ui.Center).Radius(7)
	if pal.dark {
		// Room for the chosen row's edge in every row.
		row.Border(1, ui.Transparent)
	}
	switch {
	case current:
		row.Background(pal.rowSelected)
		chosen(row, pal)
	case row.Hovered():
		row.Background(pal.rowHover)
	}
	return row
}

// treeFolder is a folder of a tree, which a click folds or unfolds;
// faded, it is one git ignores.
func (a *App) treeFolder(c *ui.Context, pal *palette, name, tip string, depth int, open, faded bool, toggle func(), tools func()) ui.Element {
	row := treeRow(c, pal, depth, false).Tooltip(tip)
	hovered := row.Hovered()
	if faded {
		row.Opacity(0.55)
	}
	if row.Clicked() {
		toggle()
	}
	row.Children(func() {
		chev := ui.Icon(c, chevronIcon).Size(9, 9).TextColor(pal.textFaint).Shrink(0)
		chev.Rotate(chev.AnimateTo("turn", 90*b2f(open), chevronTurn))
		fileIconImage(c, pal, name, true, open)
		ui.Text(c, name).FontSize(12.5).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MinWidth(0).Grow(1)
		// Its buttons, as the pointer is over it.
		if tools != nil && hovered {
			ui.Row(c.Key("tools")).Height(24).Gap(0).AlignItems(ui.Center).Shrink(0).Margin(0, -6, 0, 0).Children(tools)
		}
	})
	return row
}

// treeFile is a changed file of the tree: its icon, its name, and its
// change as a letter in its color at the right, as VS Code shows them;
// tools, when set, draws its buttons before the letter, as the pointer is
// over it.
func (a *App) treeFile(c *ui.Context, pal *palette, it treeItem, f proto.GitFile, current bool, pick func(), tools func()) ui.Element {
	tip := f.Path
	if f.OldPath != "" {
		tip = f.OldPath + " → " + f.Path
	}
	row := treeRow(c, pal, it.depth, current).Tooltip(tip)
	hovered := row.Hovered()
	if row.Clicked() {
		pick()
	}
	row.Children(func() {
		ui.Box(c).Size(9, 9).Shrink(0)
		fileIconImage(c, pal, it.name, false, false)
		fg := pal.text
		if f.Status == "deleted" {
			fg = pal.textMuted
		}
		ui.Text(c, it.name).FontSize(12.5).TextColor(fg).SingleLine().Ellipsis("…").MinWidth(0).Grow(1)
		if tools != nil && hovered {
			ui.Row(c.Key("tools")).Height(24).Gap(0).AlignItems(ui.Center).Shrink(0).Children(tools)
		}
		letter, col := a.statusLetter(c, pal, f.Status)
		ui.Text(c, letter).FontSize(10.5).FontWeight(650).TextColor(col).SingleLine().Shrink(0)
	})
	return row
}
