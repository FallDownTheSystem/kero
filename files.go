package main

import (
	"bytes"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"kero/internal/actions"
	"kero/internal/client"
	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// The Files tab is the tree of the worktree of the pane the user is in,
// else of its directory, as kerod lists it (fs.list), a folder at a time
// as the user unfolds it: what git ignores shows faded, and .git not at
// all, nor dotfiles while the user hides them (settings' HiddenFiles). A
// click on a file opens it in the viewer, as text the user edits
// (edits.go), which follows the file as it changes, or as the image it
// is; a file drags onto a pane, or its menu opens it in an editor pane of
// its own. The filter above the tree finds files and folders by name in
// all of its folder (fs.find).

const (
	// maxView bounds the files the viewer shows as text.
	maxView = 4 << 20
	// maxImage bounds an image to show, as fs.read does what it reads.
	maxImage = 32 << 20
)

// filesState is the Files tab's.
type filesState struct {
	// dirs are the folders listed, open the folders unfolded, by host and
	// path (filesKey); places the trees' places, by host and root.
	dirs   map[string]*dirList
	open   map[string]bool
	places map[string]*ui.ListState
	// gen counts the times the tab came to show, each of which lists its
	// folders anew; shown tells it shows this frame, and showed that it
	// did the frame before.
	gen           int
	shown, showed bool
	// edit is the name the user types in the tree (files_ops.go).
	edit *nameEdit
	// find is the tree's filter.
	find fileFind
	// reveal is a path to show in the tree, by host and path (filesKey),
	// as kero do files.show asks, once its folders list it.
	reveal string
	// cursor is the row the keys move in the tree, by host and path
	// (filesKey): Up and Down move it, Right and Left open and close its
	// folder, Enter opens it; it shows while the tree has the focus,
	// focused as of the frame before.
	cursor  string
	focused bool
}

// fileFind is the tree's filter: what the user types above the tree, and
// the files and folders in all of the tree's folder whose names hold it,
// as kerod finds them (fs.find), in a tree of their own.
type fileFind struct {
	query string
	// last is the query as it was the frame before, and typed when it
	// changed: a search waits a moment after, as the user types on.
	last  string
	typed time.Time
	// asked is the query found, of key's host and root, and stamp the
	// worktree's status it was found at: found anew as it changes.
	asked, key, stamp string
	busy              bool
	err               string
	found             []proto.FSFound // nil before the first search
	nodes             []*treeNode
	more              bool
	// closed are the folders the user folded in the filter's tree.
	closed map[string]bool
}

// end leaves the filter, and forgets what it found.
func (f *fileFind) end() {
	f.query, f.last, f.asked, f.err = "", "", "", ""
	f.found, f.nodes, f.more, f.closed = nil, nil, false, nil
}

// dirList is a folder as kerod listed it.
type dirList struct {
	entries []proto.FSEntry
	err     string
	busy    bool
	stamp   string // the worktree's status it was listed at (gitStamp)
	gen     int
	fetched time.Time
}

// fileItem is a row of the tree: a file or a folder, depth folders in, or
// why a folder shows nothing, err, or the field the user types a name in,
// edit.
type fileItem struct {
	proto.FSEntry
	path      string // on the host
	rel       string // in the root, with slashes
	parent    string // the folder it is in, on the host
	parentRel string
	depth     int
	open      bool
	err       string
	edit      bool
}

// fileView is a file the viewer shows, or an editor pane: its text, or
// the image it is.
type fileView struct {
	pane  string // a pane of the host it is on
	path  string // on the host
	rel   string // in the Files' root, which its header shows
	st    *editor.State
	bm    *ui.Bitmap // an image's
	hash  string     // of the file as its text shows it (git.Hash)
	edits edited
	err   string
	// gone, large and binary say why it shows no text: there is no file
	// there any more, or one too large, or one that isn't text.
	gone, large, binary bool
	busy                bool
	stamp               string
	fetched             time.Time
	// fresh tells a file that may be none yet, as an editor pane's that
	// kero split --file names: absent while it is, it opens empty, and its
	// save makes it.
	fresh, absent bool
	// toolsW is an editor pane's file's buttons' width in its header,
	// as they last showed all of them, and toolsFold that they show folded
	// in a menu, the header being too narrow for them (headerRoom).
	toolsW    float32
	toolsFold bool
	// preview shows a Markdown file as it reads, in place of its text,
	// which md shows it by.
	preview bool
	md      *markdown.Options
	// goTo is where the caret goes once the text shows (openAt).
	goTo *editor.Pos
}

// modified tells that the file has edits not saved.
func (fv *fileView) modified() bool { return fv.st != nil && fv.st.Buffer.Modified() }

// stale tells that a file shown is to be read anew: the worktree's status
// changed since it was, else an agent works and a few seconds went; never
// while it has edits, which a save checks against the disk.
func (fv *fileView) stale(stamp string, working bool) bool {
	if fv.busy || fv.modified() || fv.edits.saving {
		return false
	}
	return fv.fetched.IsZero() || fv.stamp != stamp || working && time.Since(fv.fetched) > 3*time.Second
}

// markdownFile tells a file of Markdown, which previews, by its name.
func markdownFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// filesKey keys a path of a host.
func filesKey(h *host, p string) string { return h.name + "\x00" + p }

// filesRoot is the folder the Files tab shows, and the pane it is of: the
// worktree of the one the user is in, else its directory.
func (a *App) filesRoot(s *proto.Session) (*proto.Pane, string) {
	p := withGit(s, s.PaneByID(a.focusOf(s)))
	switch {
	case p == nil:
		return nil, ""
	case p.Git != nil:
		return p, p.Git.Root
	case p.ProcessDir != "":
		return p, p.ProcessDir
	}
	return p, p.Dir
}

// filesSide is the Files tab: the root's tree, a folder listed as it
// unfolds, and again as the worktree changes; or what the filter found.
func (a *App) filesSide(c *ui.Context, pal *palette, s *proto.Session) {
	fs := &a.side.files
	if !fs.showed {
		fs.gen++
	}
	fs.shown, fs.showed = true, true
	p, root := a.filesRoot(s)
	if root == "" {
		a.sideMessage(c, pal, locales.FilesNoFolder(), locales.FilesNoFolderDetail())
		return
	}
	h := a.hostOf(p.ID)
	if fs.dirs == nil || len(fs.dirs) > 2000 {
		fs.dirs = map[string]*dirList{}
	}
	if fs.open == nil {
		fs.open = map[string]bool{}
	}
	stamp := gitStamp(p.Git)
	if !a.set.HiddenFiles {
		// Listed anew, without dotfiles, as the user hides them.
		stamp += "|nodot"
	}
	working := sessionWorking(s)
	if working {
		c.After(5 * time.Second)
	}
	asking := 0
	for _, l := range fs.dirs {
		if l.busy {
			asking++
		}
	}
	// The folders shown, listed again as the worktree's status changes,
	// as the tab shows anew, and every few seconds while an agent works,
	// a few at a time.
	list := func(dir string) *dirList {
		l := fs.dirs[filesKey(h, dir)]
		stale := l == nil || !l.busy && (l.stamp != stamp || l.gen != fs.gen || working && time.Since(l.fetched) > 5*time.Second)
		if stale && asking < 4 && h.cl != nil {
			asking++
			l = a.fetchDir(h, dir, p.Git != nil, stamp)
		}
		return l
	}
	var items []fileItem
	var walk func(dir, rel string, depth int)
	walk = func(dir, rel string, depth int) {
		l := list(dir)
		if l == nil || l.entries == nil && l.err == "" {
			return
		}
		if l.err != "" && l.entries == nil {
			items = append(items, fileItem{path: dir, rel: rel + "/!", depth: depth, err: l.err})
			return
		}
		// A new entry's name, at the top of its folder.
		if e := fs.edit; e != nil && e.op != editRename && e.host == h.name && e.dir == dir {
			items = append(items, fileItem{FSEntry: proto.FSEntry{Dir: e.op == editNewFolder}, rel: rel + "/+", parent: dir, parentRel: rel, depth: depth, edit: true})
		}
		for _, e := range l.entries {
			if e.Dir && e.Name == ".git" {
				continue
			}
			it := fileItem{FSEntry: e, path: h.joinPath(dir, e.Name), rel: path.Join(rel, e.Name), parent: dir, parentRel: rel, depth: depth}
			it.open = e.Dir && fs.open[filesKey(h, it.path)]
			it.edit = fs.edit != nil && fs.edit.op == editRename && fs.edit.host == h.name && fs.edit.path == it.path
			items = append(items, it)
			if it.open {
				walk(it.path, it.rel, depth+1)
			}
		}
	}
	walk(root, "", 0)

	a.syncFind(c, h, root, stamp)
	f := &fs.find
	filtering := strings.TrimSpace(f.query) != ""
	ui.Row(c.Key("root")).Height(28).Shrink(0).Padding(0, 10, 0, 16).Gap(0).AlignItems(ui.Center).Children(func() {
		// Its path where it fits beside the buttons, else its folder's name,
		// as a pane's header fits its folder (folderShown); the name cut
		// short where even that doesn't fit.
		shown := a.folderShown(c, "fit", h.shortDir(root), proto.DirName(root), "files-root", 12.5, 0, 0)
		if shown == "" {
			shown = proto.DirName(root)
		}
		ui.Text(c, shown).FontSize(12.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0).Grow(1).Tooltip(root).Bind(a.paneRef("files-root-dir"))
		iconButton(c, "new-file", newFileIcon, locales.FilesNewFile(), pal, func() { a.startCreate(s, p.ID, root, "", false) })
		iconButton(c, "new-folder", newFolderIcon, locales.FilesNewFolder(), pal, func() { a.startCreate(s, p.ID, root, "", true) })
		iconButton(c, "search", searchIcon, a.tip(locales.SideSearch(), "search.toggle"), pal, func() { a.defer_(func() { a.showSearch("") }) })
		iconButton(c, "refresh", reloadIcon, locales.FilesRefresh(), pal, func() { fs.gen++ })
		iconButton(c, "fold-folders", collapseIcon, locales.FilesCollapseFolders(), pal, func() {
			if filtering {
				f.closed = map[string]bool{}
				for _, it := range treeItems(f.nodes, func(string) bool { return false }) {
					if it.file < 0 {
						f.closed[it.path] = true
					}
				}
				return
			}
			prefix := filesKey(h, root)
			for k := range fs.open {
				if strings.HasPrefix(k, prefix) {
					delete(fs.open, k)
				}
			}
		})
	})
	ui.Row(c.Key("filter")).Shrink(0).Padding(0, 10, 6, 12).Gap(2).AlignItems(ui.Center).Children(func() {
		in := textField(c, "query", &f.query, locales.FilesFilter(), 0, pal).Grow(1)
		in.Tooltip(locales.FilesFilterTip())
		in.OnShortcut(0, ui.KeyEscape, f.end)
		// The first file found opens in the viewer.
		first := func() {
			if !filtering {
				return
			}
			for _, it := range treeItems(f.nodes, func(string) bool { return false }) {
				if it.file >= 0 && !f.found[it.file].Dir {
					a.openInViewer(s.ID, p.ID, joinRel(root, it.path), it.path)
					return
				}
			}
		}
		in.OnSubmit(first)
		in.OnShortcut(0, ui.KeyDown, first)
		if f.busy {
			ui.Spinner(c).Size(12, 12).Margin(0, 4)
		}
		if f.query != "" {
			iconButton(c, "clear", closeIcon, locales.FilesClearFilter(), pal, f.end)
		}
	})
	if filtering {
		a.foundTree(c, pal, s, p, root)
		return
	}
	if l := fs.dirs[filesKey(h, root)]; l == nil || l.entries == nil {
		switch {
		case l != nil && l.err != "" && !l.busy:
			a.sideMessage(c, pal, locales.FilesCantReadFolder(), l.err)
		case h.cl != nil:
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		}
		return
	}

	if fs.places == nil {
		fs.places = map[string]*ui.ListState{}
	}
	ls := fs.places[filesKey(h, root)]
	if ls == nil {
		ls = &ui.ListState{}
		fs.places[filesKey(h, root)] = ls
	}
	ls.Key = func(i int) any { return treeKey{items[i].rel, items[i].Dir} }
	if fs.reveal != "" {
		for i, it := range items {
			if filesKey(h, it.path) == fs.reveal {
				ls.ScrollIntoView(i)
				fs.reveal = ""
				break
			}
		}
	}
	shown, dirty := a.viewerFile(s)
	sid, pid := s.ID, p.ID
	focused, clicked := fs.focused, false
	// at is where the cursor is in the rows, -1 if it isn't in them.
	at := -1
	for i, it := range items {
		if !it.edit && it.err == "" && filesKey(h, it.path) == fs.cursor {
			at = i
		}
	}
	toggle := func(it fileItem) {
		if key := filesKey(h, it.path); it.open {
			delete(fs.open, key)
		} else {
			fs.open[key] = true
		}
	}
	tree := ui.List(c.Key("files"), ls, len(items), func(i int) {
		it := items[i]
		key := filesKey(h, it.path)
		switch {
		case it.edit:
			a.nameRow(c, pal, it)
		case it.err != "":
			ui.Text(c, it.err).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Padding(4, 8, 4, 6+12*float32(it.depth)+28).Tooltip(it.err)
		case it.Dir:
			row := a.treeFolder(c, pal, it.Name, it.rel, it.depth, it.open, it.Ignored, func() {
				fs.cursor, clicked = key, true
				toggle(it)
			}, nil)
			if focused && i == at {
				chosen(row.Background(pal.rowSelected), pal)
			}
			row.ContextMenu(func(m *ui.Menu) { a.entryMenu(c, m, s, pid, it, root) })
		default:
			row := a.treeEntry(c, pal, it, it.path == shown || focused && i == at, it.path == shown && dirty, func() {
				fs.cursor, clicked = key, true
				a.openInViewer(sid, pid, it.path, it.rel)
			})
			row.ContextMenu(func(m *ui.Menu) { a.entryMenu(c, m, s, pid, it, root) })
			// A file drags onto a pane, to open there (paneCard).
			row.Drag(fileDrag{pane: pid, path: it.path})
			if row.Dragging() {
				row.Opacity(0.4)
			}
		}
	}).Grow(1).MinHeight(0).Padding(0, 10, 16, 10).Focusable().FocusRing(false)
	fs.focused = tree.FocusWithin()
	if clicked {
		tree.Focus()
	}
	// The keys, while the tree has the focus and no name is being typed.
	move := func(i int) {
		if i >= 0 && i < len(items) {
			fs.cursor = filesKey(h, items[i].path)
			ls.ScrollIntoView(i)
		}
	}
	step := func(d int) {
		if fs.edit != nil {
			return
		}
		i := at
		if i < 0 {
			i = -d
			if d < 0 {
				i = len(items)
			}
		}
		for i += d; i >= 0 && i < len(items); i += d {
			if !items[i].edit && items[i].err == "" {
				move(i)
				return
			}
		}
	}
	open := func(pane bool) {
		if fs.edit != nil || at < 0 {
			return
		}
		switch it := items[at]; {
		case it.Dir:
			toggle(it)
		case pane:
			a.openInPane(pid, it.path)
		default:
			a.openInViewer(sid, pid, it.path, it.rel)
		}
	}
	tree.OnShortcut(0, ui.KeyDown, func() { step(1) })
	tree.OnShortcut(0, ui.KeyUp, func() { step(-1) })
	tree.OnShortcut(0, ui.KeyEnter, func() { open(false) })
	tree.OnShortcut(ui.Cmd, ui.KeyEnter, func() { open(true) })
	tree.OnShortcut(0, ui.KeyRight, func() {
		switch {
		case fs.edit != nil || at < 0:
		case !items[at].Dir:
			open(false)
		case !items[at].open:
			toggle(items[at])
		default:
			step(1)
		}
	})
	tree.OnShortcut(0, ui.KeyLeft, func() {
		if fs.edit != nil || at < 0 {
			return
		}
		if it := items[at]; it.Dir && it.open {
			toggle(it)
			return
		}
		for i := at - 1; i >= 0; i-- {
			if items[i].Dir && items[i].path == items[at].parent {
				move(i)
				return
			}
		}
	})
}

// viewerFile is the file a session's viewer shows, if any, and whether it
// has edits not saved.
func (a *App) viewerFile(s *proto.Session) (string, bool) {
	if v := a.viewOf(s); v != nil && v.kind == sideFiles && v.file != nil {
		return v.file.path, v.file.modified()
	}
	return "", false
}

// openInViewer opens a file of a pane's host in its session's viewer, in
// place of the one it shows, saved first, as the user says; the one it
// shows, again, as it is.
func (a *App) openInViewer(sid, pane, path, rel string) *fileView {
	s := a.session(sid)
	if s == nil {
		return nil
	}
	v := a.side.views[sid]
	if v != nil && v.file != nil && v.file.path == path && a.hostOf(v.file.pane) == a.hostOf(pane) {
		a.openView(s, sideFiles, nil) // the one it shows
		return v.file
	}
	fv := &fileView{pane: pane, path: path, rel: rel, preview: false}
	// In the place of the one it shows, saved first.
	var ts []editTarget
	if v != nil && v.file != nil {
		if t := a.fileTarget(v.file); t.dirty() {
			ts = append(ts, t)
		}
	}
	a.askToSave(ts, func() {
		if s := a.session(sid); s != nil {
			a.openView(s, sideFiles, fv)
		}
	})
	return fv
}

// treeEntry is a file of the Files tab's tree, which a click opens in the
// viewer; faded, it is one git ignores; dirty, it has edits not saved.
func (a *App) treeEntry(c *ui.Context, pal *palette, it fileItem, current, dirty bool, open func()) ui.Element {
	row := treeRow(c, pal, it.depth, current).Tooltip(it.rel)
	if it.Ignored {
		row.Opacity(0.55)
	}
	if row.Clicked() {
		open()
	}
	row.Children(func() {
		ui.Box(c).Size(9, 9).Shrink(0)
		fileIconImage(c, pal, it.Name, false, false)
		ui.Text(c, it.Name).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
		if dirty {
			ui.Box(c).Size(7, 7).Radius(3.5).Shrink(0).Background(pal.textMuted).Description(locales.ViewerUnsaved())
		}
	})
	return row
}

// fetchDir asks kerod for a folder's entries, dotfiles too as the user
// chose (settings' HiddenFiles), marking what git ignores in a worktree.
func (a *App) fetchDir(h *host, dir string, ignored bool, stamp string) *dirList {
	fs := &a.side.files
	key := filesKey(h, dir)
	l := fs.dirs[key]
	if l == nil {
		l = &dirList{}
		fs.dirs[key] = l
	}
	l.busy = true
	gen, cl, hidden := fs.gen, h.cl, a.set.HiddenFiles
	go func() {
		var res proto.FSList
		err := cl.Call("fs.list", proto.FSListParams{Dir: dir, Hidden: hidden, Ignored: ignored}, &res)
		a.update(func() {
			l.busy, l.fetched, l.stamp, l.gen = false, time.Now(), stamp, gen
			if err != nil {
				l.err = err.Error()
				return
			}
			l.entries, l.err = res.Entries, ""
		})
	}()
	return l
}

// syncFind asks kerod for what the filter finds in a host's root, a
// moment after the user last typed, and again as the worktree's status
// changes; one search at a time, the next asked once it answers.
func (a *App) syncFind(c *ui.Context, h *host, root, stamp string) {
	f := &a.side.files.find
	if f.query != f.last {
		if strings.TrimSpace(f.query) == "" {
			f.end()
			return
		}
		f.last, f.typed = f.query, time.Now()
	}
	q, key := strings.TrimSpace(f.query), filesKey(h, root)
	if q == "" || f.busy || h.cl == nil || q == f.asked && f.key == key && f.stamp == stamp {
		return
	}
	if wait := 150*time.Millisecond - time.Since(f.typed); wait > 0 {
		c.After(wait)
		return
	}
	f.busy = true
	cl, hidden := h.cl, a.set.HiddenFiles
	go func() {
		var r proto.FSFind
		err := cl.Call("fs.find", proto.FSFindParams{Dir: root, Query: q, Hidden: hidden}, &r)
		a.update(func() {
			f.busy = false
			if q != strings.TrimSpace(f.query) {
				return // the user typed on: asked again
			}
			if q != f.asked {
				f.closed = nil
			}
			f.asked, f.key, f.stamp = q, key, stamp
			if err != nil {
				f.found, f.nodes, f.err = nil, nil, err.Error()
				return
			}
			// A folder found that holds what else was found shows once, as
			// the folder of it.
			f.err, f.more, f.found = "", r.More, []proto.FSFound{}
			for _, e := range r.Entries {
				if e.Dir && slices.ContainsFunc(r.Entries, func(o proto.FSFound) bool { return strings.HasPrefix(o.Path, e.Path+"/") }) {
					continue
				}
				f.found = append(f.found, e)
			}
			files := make([]proto.GitFile, len(f.found))
			for i, e := range f.found {
				files[i] = proto.GitFile{Path: e.Path}
			}
			f.nodes = fileTree(files)
		})
	}()
}

// foundTree is what the filter found, in the folders it is in: a file a
// click opens in the viewer; a folder found shows in the whole tree.
func (a *App) foundTree(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane, root string) {
	fs := &a.side.files
	f := &fs.find
	switch {
	case f.err != "":
		a.sideMessage(c, pal, locales.FilesCantFind(), f.err)
		return
	case f.found == nil:
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	case len(f.found) == 0:
		a.sideMessage(c, pal, locales.FilesNoMatch(f.asked), "")
		return
	}
	h := a.hostOf(p.ID)
	items := treeItems(f.nodes, func(dir string) bool { return f.closed[dir] })
	shown, dirty := a.viewerFile(s)
	sid, pid := s.ID, p.ID
	ui.List(c.Key("found"), &ui.ListState{}, len(items), func(i int) {
		it := items[i]
		full := joinRel(root, it.path)
		if it.file >= 0 && f.found[it.file].Dir {
			// A folder found: shown in the whole tree, open.
			e := fileItem{FSEntry: proto.FSEntry{Name: it.name, Dir: true}, path: full, rel: it.path, parent: hostParent(full), parentRel: path.Dir(it.path), depth: it.depth}
			row := a.treeFolder(c, pal, it.name, it.path, it.depth, false, false, func() {
				f.end()
				fs.open[filesKey(h, full)] = true
				fs.reveal = filesKey(h, full)
			}, nil)
			row.ContextMenu(func(m *ui.Menu) { a.entryMenu(c, m, s, pid, e, root) })
			return
		}
		if it.file < 0 {
			// A folder for what was found in it, not for its name.
			a.treeFolder(c, pal, it.name, it.path, it.depth, it.open, true, func() {
				if f.closed == nil {
					f.closed = map[string]bool{}
				}
				f.closed[it.path] = it.open
			}, nil)
			return
		}
		e := fileItem{FSEntry: proto.FSEntry{Name: path.Base(it.path)}, path: full, rel: it.path, parent: hostParent(full), parentRel: path.Dir(it.path), depth: it.depth}
		row := a.treeEntry(c, pal, e, full == shown, full == shown && dirty, func() { a.openInViewer(sid, pid, full, it.path) })
		row.ContextMenu(func(m *ui.Menu) { a.entryMenu(c, m, s, pid, e, root) })
		row.Drag(fileDrag{pane: pid, path: full})
		if row.Dragging() {
			row.Opacity(0.4)
		}
	}).Grow(1).MinHeight(0).Padding(0, 10, 8, 10)
	if f.more {
		ui.Text(c, locales.FilesMoreMatches()).FontSize(11.5).TextColor(pal.textFaint).LineHeight(1.3).Shrink(0).Padding(4, 16, 10, 16)
	}
}

// fileViewer is the viewer of a file of Files: its text, which it reads
// again as the worktree changes, and every few seconds while an agent
// works, while the user has no edits of it to save; or its image.
func (a *App) fileViewer(c *ui.Context, pal *palette, s *proto.Session, fv *fileView) {
	h := a.hostOf(fv.pane)
	stamp := ""
	if p := withGit(s, s.PaneByID(fv.pane)); p != nil {
		stamp = gitStamp(p.Git)
	}
	working := sessionWorking(s)
	if working {
		c.After(3 * time.Second)
	}
	if h.cl != nil && fv.stale(stamp, working) {
		a.loadFile(h.cl, fv, stamp)
	}

	rel := fv.rel
	if rel == "" {
		rel = proto.DirName(fv.path)
	}
	dir, name := path.Split(rel)
	editing := fv.modified() || fv.edits.saving
	a.viewerHeader(c, pal, s, fileIcon, name, editing, func() {
		if dir = strings.TrimSuffix(dir, "/"); dir != "" {
			ui.Text(c, dir).FontSize(13).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0).Tooltip(fv.path)
		}
	}, func() {
		a.fileActions(c, pal, fv)
		if !fv.gone {
			pane, file := fv.pane, fv.path
			iconButton(c, "pane", splitRightIcon, locales.FilesOpenInPane(), pal, func() { a.openInPane(pane, file) })
		}
	})
	a.fileBody(c, pal, fv, nil)
}

// fileBody is a file's text in its editor, or its image, its Markdown
// preview, or why it shows none: in the viewer, and in an editor pane,
// whose ref binds the editor, else the body itself, so that the pane takes
// the keyboard, as a click or the pointer moving in focuses it, whatever
// it shows.
func (a *App) fileBody(c *ui.Context, pal *palette, fv *fileView, ref *ui.Handle) {
	col := ui.Column(c.Key("body-" + fv.path)).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	editing := false
	col.Children(func() {
		switch {
		case fv.gone && !fv.fresh:
			a.sideMessage(c, pal, locales.FilesGone(), fv.rel)
		case fv.large:
			a.sideMessage(c, pal, locales.ChangesTooLarge(), "")
		case fv.binary:
			a.sideMessage(c, pal, locales.ChangesBinary(), "")
		case fv.bm != nil:
			ui.Box(c.Key("image")).Grow(1).MinHeight(0).Padding(12).Center().Children(func() {
				ui.Image(c, fv.bm).Fill().Fit(ui.Contain)
			})
		case fv.st == nil && fv.err != "":
			a.sideMessage(c, pal, locales.FilesCantReadFile(), fv.err)
		case fv.st == nil:
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		case fv.preview && markdownFile(fv.path):
			if fv.md == nil {
				fv.md = &markdown.Options{}
				// Its images are files beside it, which only this machine's
				// disk has.
				if !a.hostOf(fv.pane).remote() {
					fv.md.LoadImage = markdown.FileImages(filepath.Dir(fv.path))
				}
			}
			// Zoomed as the file's text is, from the window's size.
			fv.md.Theme = &markdown.Theme{FontSize: max(c.Theme().FontSize+float32(a.prefs.FileZoom), 6)}
			ui.Scroll(c.Key("preview")).Grow(1).MinHeight(0).Padding(12, 16, 24, 16).Children(func() {
				markdown.View(c, fv.st.Buffer.String(), fv.md)
			})
		default:
			st := fv.st
			st.Font = ui.Font{Family: a.termFamily(), Size: a.fileFontSize()}
			st.Theme = a.codeTheme(c, pal)
			st.Label = proto.DirName(fv.path)
			st.SoftWrap = a.set.WrapLines
			st.NoScrollPastEnd = true
			// What shows as it saves is what it writes.
			st.ReadOnly = fv.edits.saving
			if fv.goTo != nil {
				st.GoTo(*fv.goTo)
				fv.goTo = nil
			}
			ed := editor.View(c.Key("text-"+fv.path), st).Grow(1).FillWidth().MinHeight(0)
			if ref != nil {
				ed.Bind(ref)
			}
			if ed.FocusWithin() {
				a.side.editing = true
			}
			editing = true
		}
	})
	if ref != nil && !editing {
		col.Focusable().FocusRing(false).Bind(ref)
	}
	// An editor pane's save; the viewer's card has its own (saveFocused).
	if ref != nil && col.Shortcut(ui.Cmd, ui.KeyS) && (fv.modified() || fv.absent && fv.st != nil) && !fv.edits.saving {
		a.saveTargets([]editTarget{a.fileTarget(fv)}, nil)
	}
}

// fileStatus tells that a file saves, has edits not saved, or is new.
func (a *App) fileStatus(c *ui.Context, pal *palette, fv *fileView) {
	switch {
	case fv.edits.saving:
		ui.Spinner(c).Size(12, 12)
	case fv.modified():
		// A dot, as the viewer's close button and the tree mark it.
		ui.Box(c.Key("unsaved")).Size(8, 8).Radius(4).Shrink(0).Margin(0, 8, 0, 4).Background(pal.textMuted).Description(locales.ViewerUnsaved()).Tooltip(locales.ViewerUnsaved())
	case fv.absent && fv.st != nil:
		ui.Text(c, locales.FilesNew()).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0).Margin(0, 4, 0, 0).Tooltip(locales.FilesNewTip())
	}
}

// fileActions are what acts on a file: its save while it is new, its
// Markdown preview, the size and the wrapping of its text, and on this
// machine, its editor.
func (a *App) fileActions(c *ui.Context, pal *palette, fv *fileView) {
	if !fv.edits.saving && fv.absent && fv.st != nil && !fv.modified() {
		iconButton(c, "save", checkIcon, a.saveTip(), pal, func() { a.saveTargets([]editTarget{a.fileTarget(fv)}, nil) })
	}
	if fv.st != nil && markdownFile(fv.path) {
		b := ui.ButtonBase(c.Key("preview")).Size(28, 26).Margin(0, 2).Radius(6).Center().TextColor(pal.textMuted).Label(locales.FilesPreviewMarkdown()).Tooltip(locales.FilesPreviewMarkdown())
		switch {
		case fv.preview:
			b.Background(pal.rowHover).TextColor(pal.text)
		case b.Hovered() || b.Pressed():
			b.Background(pal.rowHover.Alpha(0.5)).TextColor(pal.text)
		}
		b.Children(func() { ui.Icon(c, previewIcon).Size(16, 16) })
		if b.Clicked() {
			fv.preview = !fv.preview
		}
	}
	if fv.st != nil {
		iconButton(c, "smaller", smallerIcon, a.tip(locales.FilesSmaller(), "text.smaller"), pal, func() { a.zoomFiles(-1, false) })
		iconButton(c, "bigger", biggerIcon, a.tip(locales.FilesBigger(), "text.bigger"), pal, func() { a.zoomFiles(1, false) })
	}
	if fv.st != nil && !fv.preview {
		a.wrapButton(c, pal)
	}
	if !a.hostOf(fv.pane).remote() && !fv.gone && !fv.absent {
		file := fv.path
		iconButton(c, "open", openFileIcon, locales.FilesOpenInEditor(), pal, func() { a.openInEditor(file) })
	}
}

// fileActionsMenu is fileActions as a menu's items, for an editor pane's
// header too narrow for their buttons.
func (a *App) fileActionsMenu(m *ui.Menu, fv *fileView) {
	if !fv.edits.saving && fv.absent && fv.st != nil && !fv.modified() {
		if m.Item(locales.FilesSave()).Chosen() {
			a.saveTargets([]editTarget{a.fileTarget(fv)}, nil)
		}
	}
	if fv.st != nil && markdownFile(fv.path) && m.Item(locales.FilesPreviewMarkdown()).Checked(fv.preview).Chosen() {
		fv.preview = !fv.preview
	}
	if fv.st != nil {
		if a.menuShortcut(m.Item(locales.FilesSmaller()), "text.smaller").Chosen() {
			a.zoomFiles(-1, false)
		}
		if a.menuShortcut(m.Item(locales.FilesBigger()), "text.bigger").Chosen() {
			a.zoomFiles(1, false)
		}
	}
	if fv.st != nil && !fv.preview && m.Item(locales.ViewerWrap()).Checked(a.set.WrapLines).Chosen() {
		set := a.set
		set.WrapLines = !set.WrapLines
		a.applySettings(set)
	}
	if !a.hostOf(fv.pane).remote() && !fv.gone && !fv.absent && m.Item(locales.FilesOpenInEditor()).Chosen() {
		a.openInEditor(fv.path)
	}
}

// fileActionsButton is the button whose menu has fileActionsMenu's items.
func (a *App) fileActionsButton(c *ui.Context, pal *palette, fv *fileView, more func(m *ui.Menu)) {
	b := ui.ButtonBase(c.Key("file-actions")).Size(controlW, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.FilesActions()).Tooltip(locales.FilesActions())
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, editIcon).Size(15, 15) })
	b.Menu(func(m *ui.Menu) {
		if more != nil {
			more(m)
			m.Separator()
		}
		a.fileActionsMenu(m, fv)
	})
}

// loadFile reads a file of the viewer or an editor pane: an image to show,
// else its text into its editor, anew for its first, else as an edit, so
// that it stays scrolled where it was. A file too large, or binary, is
// told of instead; a fresh one there is none of yet opens empty.
func (a *App) loadFile(cl *client.Client, fv *fileView, stamp string) {
	fv.busy = true
	path, fresh := fv.path, fv.fresh
	image := peekable[strings.ToLower(filepath.Ext(path))]
	limit := int64(maxView)
	if image {
		limit = maxImage
	}
	go func() {
		var st proto.FSStat
		var data []byte
		var bm *ui.Bitmap
		err := cl.Call("fs.stat", proto.FSStatParams{Path: path}, &st)
		if err == nil && st.Exists && !st.Dir && st.Size <= limit {
			var r proto.FSRead
			err = cl.Call("fs.read", proto.FSReadParams{Path: path, Max: limit}, &r)
			data = r.Data
		}
		if err == nil && image && data != nil {
			bm, err = ui.DecodeBitmap(data)
		}
		a.update(func() {
			fv.busy, fv.fetched, fv.stamp = false, time.Now(), stamp
			if err != nil {
				fv.err = err.Error()
				return
			}
			fv.err = ""
			fv.absent = fresh && !st.Exists
			fv.gone = !fv.absent && (!st.Exists || st.Dir)
			fv.large = !fv.gone && st.Size > limit
			if image && !fv.absent {
				fv.bm = bm
				return
			}
			fv.binary = !fv.gone && !fv.large && isBinary(data)
			if fv.gone || fv.large || fv.binary {
				return
			}
			// Not over what the user typed as it came.
			if fv.modified() || fv.edits.saving {
				return
			}
			a.showFile(fv, data)
		})
	}()
}

// showFile puts a file as read in its view, and with its first text the
// user's edit of it backed up as the app last quit, if any.
func (a *App) showFile(fv *fileView, data []byte) {
	first := fv.st == nil
	fv.hash = git.Hash(data)
	if fv.absent {
		// No file to find as it was read: fs.write makes it, unless
		// another program did since.
		fv.hash = ""
	}
	showFileText(fv, string(data))
	if first {
		a.restoreEdit(sideFiles, a.hostOf(fv.pane), fv.path, fv.st.Buffer, &fv.hash)
	}
}

// showFileText puts a file's text in its view's editor, as saved.
func showFileText(fv *fileView, text string) {
	if fv.st == nil {
		buf := editor.NewBuffer(text)
		buf.SetLanguage(editor.LanguageFor(proto.DirName(fv.path)))
		fv.st = &editor.State{Buffer: buf}
		return
	}
	replaceText(fv.st.Buffer, text)
	fv.st.Buffer.MarkSaved()
}

// isBinary tells text from what is not: a NUL in its first bytes, as git
// tells, or bytes that are no UTF-8, which a save would not write back as
// they were.
func isBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 || !utf8.Valid(data)
}

// filesRel is a path's path from the root of the Files tab of the session
// of pane, or "" when it is outside it.
func (a *App) filesRel(pane, p string) string {
	s := a.sessionOfPane(pane)
	if s == nil {
		return ""
	}
	_, root := a.filesRoot(s)
	if root == "" {
		return ""
	}
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, `\`, "/"), "/") }
	r, q := norm(root), norm(p)
	if len(q) <= len(r) || q[len(r)] != '/' || !strings.EqualFold(q[:len(r)], r) {
		return ""
	}
	return q[len(r)+1:]
}

// hostJoin joins a name to a folder of a host, with the host's separator,
// which its paths tell: a Windows host's have backslashes, or a drive.
func hostJoin(dir, name string) string {
	sep := "/"
	if strings.Contains(dir, `\`) || len(dir) > 1 && dir[1] == ':' {
		sep = `\`
	}
	if strings.HasSuffix(dir, sep) {
		return dir + name
	}
	return dir + sep + name
}

// hostParent is the folder a path of a host is in, as hostJoin joins them.
func hostParent(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i > 0 {
		return path[:i]
	}
	return path
}

// joinRel joins a path, from a folder of a host, with either slashes, to
// the folder, with the host's separator (hostJoin).
func joinRel(dir, rel string) string {
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		dir = hostJoin(dir, part)
	}
	return dir
}

// pathItems are the items of a file's or folder's menu, of pane's host,
// that open it on this machine, which a host's but this one's has none
// of: in the user's editor, the app the system asks for and the file
// manager; then what copies its path, and rel, its path from the folder
// the user works in, unless "".
func (a *App) pathItems(c *ui.Context, m *ui.Menu, pane, path, rel string, dir bool) {
	if !a.hostOf(pane).remote() {
		if m.Item(locales.FilesOpenInEditor()).Chosen() {
			a.openInEditor(path)
		}
		if canOpenWith && !dir && m.Item(locales.FilesOpenWith()).Chosen() {
			if err := openWith(path); err != nil {
				a.toast(locales.FilesCantOpen(err.Error()))
			}
		}
		if m.Item(locales.CommonShowIn(fileManager())).Chosen() {
			revealPath(path)
		}
		m.Separator()
	}
	if m.Item(locales.FilesCopyPath()).Chosen() {
		c.WriteClipboard(path)
	}
	if rel != "" && m.Item(locales.FilesCopyRelativePath()).Chosen() {
		c.WriteClipboard(rel)
	}
}

// editorMenu is the menu of an editor pane's title: what acts on its file
// as the Files tab's menu of it does, its relative path from the folder
// that tab shows for the session.
func (a *App) editorMenu(c *ui.Context, m *ui.Menu, s *proto.Session, p *proto.Pane) {
	rel := a.filesRel(p.ID, p.File)
	if rel != "" {
		if rp, root := a.filesRoot(s); rp != nil && rp.Git != nil && rp.Git.Root == root && m.Item(locales.FilesShowHistory()).Chosen() {
			a.showHistory(rel)
		}
	}
	a.pathItems(c, m, p.ID, p.File, rel, false)
}

// openItems are the items of a menu of a file of a pane's host, outside
// the tree, that open it: in the user's editor, on this machine, then in
// an editor pane of its own, there with the caret at a line, from 1,
// unless 0.
func (a *App) openItems(m *ui.Menu, pane, file string, line int) {
	if !a.hostOf(pane).remote() && m.Item(locales.FilesOpenInEditor()).Chosen() {
		a.openInEditor(file)
	}
	a.paneItems(m, pane, file, line)
}

// paneItems is the item of a file's menu that opens it in an editor pane
// of its own, with the caret at a line, from 1, unless 0.
func (a *App) paneItems(m *ui.Menu, pane, file string, line int) {
	if m.Item(locales.FilesOpenInPane()).Chosen() {
		if line > 0 {
			a.openAt(pane, file, line)
		} else {
			a.openInPane(pane, file)
		}
	}
}

// openInEditor opens a file or folder of this machine in the user's
// editor, else in the app the system opens it with.
func (a *App) openInEditor(path string) {
	if err := openInEditor(a.set.Editor, path); err != nil {
		a.toast(locales.FilesCantOpen(err.Error()))
	}
}

// openInPane opens a file of a pane's host in an editor pane of its own,
// beside the pane the user is in. A file dragged onto an editor pane
// takes the place of its file instead (dropFile).
func (a *App) openInPane(pane, path string) {
	if s := a.sessionOfPane(pane); s != nil {
		a.split(s, proto.CreateParams{Split: a.focusOf(s), Direction: "right", File: path})
	}
}

// replaceFile shows a file in an editor pane in place of its own (pane.file),
// and puts the user in it; a kerod that can't, as one before pane.file,
// opens it in a pane of its own.
func (a *App) replaceFile(sid, pane, path string) {
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	go func() {
		var ok bool
		err := cl.Call("pane.file", proto.FileParams{ID: pane, File: path}, &ok)
		a.update(func() {
			s := a.session(sid)
			switch {
			case s == nil:
			case err != nil:
				a.split(s, proto.CreateParams{Split: a.focusOf(s), Direction: "right", File: path})
			case s.PaneByID(pane) != nil:
				a.setFocus(s, pane)
				a.paneRef(pane).Focus()
			}
		})
	}()
}

// terminalIn opens a terminal in a folder of a pane's host, beside the
// pane the user is in.
func (a *App) terminalIn(pane, dir string) {
	if s := a.sessionOfPane(pane); s != nil {
		a.split(s, proto.CreateParams{Split: a.focusOf(s), Direction: "right", Dir: dir})
	}
}

// saveTip is the save button's tooltip, with its shortcut.
func (a *App) saveTip() string {
	return locales.FilesSave() + " (" + actions.Glyphs("CmdOrCtrl+S") + ")"
}

// showFiles shows the Files tab, and in it a path, as kero do files.show
// asks: the folders it is in open, and a file opens in the viewer.
func (a *App) showFiles(p string) {
	a.showTab(sideFiles)
	s := a.session(a.selected)
	if p == "" || s == nil {
		return
	}
	pane, root := a.filesRoot(s)
	if pane == nil {
		a.toast(locales.FilesNotInTree(p))
		return
	}
	full := p
	if !filepath.IsAbs(p) && !strings.HasPrefix(p, "/") && !(len(p) > 1 && p[1] == ':') {
		full = joinRel(root, p)
	}
	rel := a.filesRel(pane.ID, full)
	if rel == "" && full != root {
		a.toast(locales.FilesNotInTree(p))
		return
	}
	h := a.hostOf(pane.ID)
	if h.cl == nil {
		return
	}
	full = joinRel(root, rel)
	sid, pid, cl := s.ID, pane.ID, h.cl
	go func() {
		var st proto.FSStat
		err := cl.Call("fs.stat", proto.FSStatParams{Path: full}, &st)
		a.update(func() {
			fs := &a.side.files
			switch {
			case err != nil:
				a.toast(locales.FilesCantOpen(err.Error()))
				return
			case !st.Exists:
				a.toast(locales.FilesCantOpen(locales.FilesGone()))
				return
			}
			// It shows in the whole tree, which the filter would hide.
			fs.find.end()
			fs.gen++
			if full == root {
				return
			}
			if fs.open == nil {
				fs.open = map[string]bool{}
			}
			fs.reveal = filesKey(h, full)
			// The folders it is in open.
			for d := hostParent(full); len(d) > len(root); d = hostParent(d) {
				fs.open[filesKey(h, d)] = true
			}
			if st.Dir {
				fs.open[filesKey(h, full)] = true
				return
			}
			a.openInViewer(sid, pid, full, rel)
		})
	}()
}
