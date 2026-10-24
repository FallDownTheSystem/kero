package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/actions"
	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// The Files view is the side panel's tree of the worktree of the pane the
// user is in, else of its folder, as kerod lists it (fs.list): on a remote
// host, that host's. The file chosen in it opens below it, highlighted as
// its language is, to read and edit: a save writes it back through kerod
// (fs.write), only as it was read, never over what an agent wrote since.

// sideFiles is the Files view's id, as prefs.Side holds it.
const sideFiles = "files"

const (
	// maxPreview bounds the text a file opens with: past it, the editor
	// it opens in is the user's.
	maxPreview = 4 << 20
	// maxImage bounds an image to show, as fs.read does what it reads.
	maxImage = 32 << 20
	// treeShare is the tree's share of the view's height while a file
	// shows below it, as the view first opens.
	treeShare = 0.4
)

// filesState is the Files view's: its trees, by the host and folder they
// are of, which a folder the user comes back to shows as they left it.
type filesState struct {
	trees map[string]*fileTree
	// shown is the tree the view last showed, whose open file Find
	// searches.
	shown *fileTree
	// share is the tree's share of the height while a file shows, 0 for
	// treeShare; dragging the divider moves it, from drag.
	share    float32
	drag     float32
	dragging bool
}

// fileTree is a folder's tree: its folders' listings, those open, and the
// files open, each in its editor.
type fileTree struct {
	pane string // whose host lists it
	root string
	// dirs are the listings, by path; isDir tells the paths listed that
	// are folders.
	dirs    map[string]*fileDir
	isDir   map[string]bool
	outline ui.OutlineState[string]
	chosen  int
	// stamp is what of the worktree's status the listings follow: they
	// are listed anew as it changes.
	stamp string
	// files are the files opened, by path: those edited stay, unsaved, as
	// another shows; shown is the one that shows, "" for none.
	files map[string]*openFile
	shown string
	// adding is the file or folder the user names to make, nil for none;
	// reveal a path to choose in the tree once its folder lists it.
	adding *newEntry
	reveal string
}

// newEntry is a file or folder the user names, to make in a folder of the
// tree: New File and New Folder.
type newEntry struct {
	in    string // the folder
	dir   bool
	name  string
	focus bool // the name's field takes the keyboard as it shows
	busy  bool
	err   string
}

// fileDir is a folder's listing: the paths in it, folders first.
type fileDir struct {
	paths   []string
	stamp   string
	fetched time.Time
	busy    bool
	err     string
}

// openFile is a file opened in the view: its text in an editor, or the
// image it is, or why it shows neither.
type openFile struct {
	path string
	st   *editor.State // nil but for text
	bm   *ui.Bitmap
	// hash is that of the file's content on disk as the editor's saved
	// text is, which a save must find there still.
	hash    string
	stamp   string
	fetched time.Time
	busy    bool
	saving  bool
	// changed tells that a save found the file changed on disk since it
	// was read: the user reloads it, or overwrites it.
	changed bool
	binary  bool
	large   bool
	err     string
	// fresh tells a file that may be none yet, as an editor pane's that
	// kero split --file names: absent while it is, it opens empty, and its
	// save makes it.
	fresh  bool
	absent bool
}

// modified tells that the file has edits not saved.
func (f *openFile) modified() bool { return f.st != nil && f.st.Buffer.Modified() }

// filesRoot returns the folder the Files view shows for a session, and the
// pane whose it is: the worktree of the pane the user is in, else its
// folder, else another terminal's of the session; nil for none.
func (a *App) filesRoot(s *proto.Session) (*proto.Pane, string) {
	panes := []*proto.Pane{s.PaneByID(a.focusOf(s))}
	for i := range s.Panes {
		panes = append(panes, &s.Panes[i])
	}
	for _, p := range panes {
		switch {
		case p == nil || !p.Terminal():
		case p.Git != nil:
			return p, p.Git.Root
		case p.WorkDir() != "":
			return p, p.WorkDir()
		}
	}
	return nil, ""
}

// tree returns the tree of a host's folder, made as it first shows.
func (a *App) tree(pane *proto.Pane, root string) *fileTree {
	fs := &a.side.files
	key := a.hostOf(pane.ID).name + "\x00" + root
	t := fs.trees[key]
	if t == nil {
		if fs.trees == nil {
			fs.trees = map[string]*fileTree{}
		}
		t = &fileTree{root: root, chosen: -1, dirs: map[string]*fileDir{}, isDir: map[string]bool{root: true}, files: map[string]*openFile{}}
		t.outline.List.Selected = &t.chosen
		t.outline.Indent = 12
		fs.trees[key] = t
	}
	// Another pane of the same folder lists it as well, while its host is
	// the same.
	t.pane = pane.ID
	return t
}

// children returns the paths in a folder, nil for a file: those of a
// folder not yet listed come as it opens.
func (t *fileTree) children(path string) []string {
	if !t.isDir[path] {
		return nil
	}
	if d := t.dirs[path]; d != nil && d.paths != nil {
		return d.paths
	}
	return []string{}
}

// rel is a path from the tree's folder.
func (t *fileTree) rel(path string) string {
	return strings.TrimLeft(strings.TrimPrefix(path, t.root), `/\`)
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

// join returns a path in a folder of the tree, written as the tree writes
// its paths: rel's elements, either slash between them, each joined on.
func (t *fileTree) join(dir, rel string) string {
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		dir = hostJoin(dir, part)
	}
	return dir
}

// within returns a path as the tree writes it, when it is in the tree's
// folder: a path from a shell may have the other slashes, or on Windows,
// letters of another case; a relative one is the folder's.
func (t *fileTree) within(path string) (string, bool) {
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, `\`, "/"), "/") }
	root, p := norm(t.root), norm(path)
	drive := len(root) > 1 && root[1] == ':'
	if !strings.HasPrefix(p, "/") && !(len(p) > 1 && p[1] == ':') {
		return t.join(t.root, path), true
	}
	switch {
	case p == root || drive && strings.EqualFold(p, root):
		return t.root, true
	case len(p) <= len(root) || p[len(root)] != '/':
		return "", false
	case drive && !strings.EqualFold(p[:len(root)], root), !drive && p[:len(root)] != root:
		return "", false
	}
	return t.join(t.root, p[len(root)+1:]), true
}

// revealPath chooses a path in the tree, opening the folders it is in,
// once they list it.
func (t *fileTree) revealPath(path string) {
	t.reveal = path
	for d := hostParent(path); len(d) > len(t.root); d = hostParent(d) {
		t.isDir[d] = true
		t.outline.Open.Add(d)
	}
}

// revealed chooses the path to reveal once the tree shows it, and forgets
// it once its folder is listed without it.
func (t *fileTree) revealed() {
	if t.reveal == "" {
		return
	}
	for i := range t.outline.Rows() {
		if t.outline.Item(i) == t.reveal {
			t.chosen, t.reveal = i, ""
			t.outline.List.ScrollIntoView(i)
			return
		}
	}
	if d := t.dirs[hostParent(t.reveal)]; d != nil && !d.busy && d.paths != nil && d.stamp == t.stamp && !slices.Contains(d.paths, t.reveal) {
		t.reveal = ""
	}
}

// refresh marks the tree's listings, and the file it shows, to be read
// anew.
func (t *fileTree) refresh() {
	for _, d := range t.dirs {
		d.stamp = "\x00"
	}
	if f := t.files[t.shown]; f != nil {
		f.stamp = "\x00"
	}
}

// filesView is the Files view: the tree, and the file chosen in it below.
func (a *App) filesView(c *ui.Context, pal *palette, s *proto.Session) {
	fs := &a.side.files
	pane, root := a.filesRoot(s)
	if pane == nil {
		fs.shown = nil
		a.sideMessage(c, pal, locales.FilesNoFolder(), locales.FilesNoFolderDetail())
		return
	}
	t := a.tree(pane, root)
	fs.shown = t
	working := false
	for _, p := range s.Panes {
		working = working || p.Status == proto.Working
	}
	t.stamp = gitStamp(pane.Git)
	a.syncTree(c, t, working)
	if working {
		c.After(5 * time.Second)
	}

	f := t.files[t.shown]
	body := ui.Column(c.Key("files-" + root)).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Bind(a.paneRef("files-body"))
	body.Children(func() {
		share := float32(1)
		if f != nil {
			share = fs.share
			if share == 0 {
				share = treeShare
			}
		}
		ui.Column(c.Key("tree")).Grow(share).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.treeView(c, pal, t)
			if t.adding != nil {
				a.newEntryView(c, pal, t)
			}
			a.filesToolbar(c, pal, t)
		})
		if f != nil {
			a.filesEdge(c, pal)
			ui.Column(c.Key("file")).Grow(1 - share).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				a.fileView(c, pal, t, f, working)
			})
		}
	})
}

// syncTree lists the tree's folder and its folders open that are not yet
// listed, a few at a time, and again as the worktree's status changes, or
// every few seconds while an agent works in the session; and reads the
// file shown again as they are.
func (a *App) syncTree(c *ui.Context, t *fileTree, working bool) {
	if a.hostOf(t.pane).cl == nil {
		return
	}
	stale := func(stamp string, fetched time.Time) bool {
		return stamp != t.stamp || working && time.Since(fetched) > 5*time.Second
	}
	asking := 0
	for _, d := range t.dirs {
		if d.busy {
			asking++
		}
	}
	want := []string{t.root}
	for p := range t.outline.Open.All() {
		if t.isDir[p] {
			want = append(want, p)
		}
	}
	for _, p := range want {
		d := t.dirs[p]
		if d != nil && (d.busy || !stale(d.stamp, d.fetched)) {
			continue
		}
		if asking >= 4 {
			c.After(50 * time.Millisecond)
			break
		}
		asking++
		a.fetchDir(t, p)
	}
	if f := t.files[t.shown]; f != nil && f.stale(t.stamp, working) {
		a.readFile(t.pane, t.stamp, f)
	}
}

// stale tells that a file shown is to be read anew: the worktree's
// status changed since it was, else an agent works and a few seconds
// went; never while it has edits, which a save checks against the disk.
func (f *openFile) stale(stamp string, working bool) bool {
	if f.busy || f.saving || f.modified() {
		return false
	}
	return f.stamp != stamp || working && time.Since(f.fetched) > 5*time.Second
}

// fetchDir asks kerod for a folder's listing: dotfiles too, but .git.
func (a *App) fetchDir(t *fileTree, path string) {
	d := t.dirs[path]
	if d == nil {
		d = &fileDir{}
		t.dirs[path] = d
	}
	cl := a.hostOf(t.pane).cl
	d.busy = true
	stamp := t.stamp
	go func() {
		var l proto.FSList
		err := cl.Call("fs.list", proto.FSListParams{Dir: path, Hidden: true}, &l)
		a.update(func() { t.listed(path, stamp, l, err) })
	}()
}

// listed puts a folder's listing, as kerod answered, in the tree.
func (t *fileTree) listed(path, stamp string, l proto.FSList, err error) {
	d := t.dirs[path]
	if d == nil {
		d = &fileDir{}
		t.dirs[path] = d
	}
	d.busy, d.fetched, d.stamp = false, time.Now(), stamp
	if err != nil {
		d.err = err.Error()
		return
	}
	d.err = ""
	paths := make([]string, 0, len(l.Entries))
	for _, e := range l.Entries {
		if e.Name == ".git" {
			continue
		}
		p := hostJoin(path, e.Name)
		t.isDir[p] = e.Dir
		paths = append(paths, p)
	}
	d.paths = paths
}

// treeView is the tree: a folder's files, its folders opening on their
// arrows, Right and Left, or a double click. Choosing a file opens it; a
// double click, or Enter, opens it in the user's editor.
func (a *App) treeView(c *ui.Context, pal *palette, t *fileTree) {
	d := t.dirs[t.root]
	switch {
	case d == nil || d.paths == nil && d.err == "":
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	case d.paths == nil:
		a.sideMessage(c, pal, locales.FilesCantList(), d.err)
		return
	case len(d.paths) == 0:
		a.sideMessage(c, pal, locales.FilesEmpty(), "")
		return
	}
	tree := ui.Outline(c.Key("outline"), &t.outline, d.paths, t.children, func(path string) {
		dir := t.isDir[path]
		svg := docIcon
		if dir {
			svg = folderIcon
		}
		row := ui.Row(c).Grow(1).MinWidth(0).Gap(7).AlignItems(ui.Center)
		row.ContextMenu(func(m *ui.Menu) { a.fileMenu(c, m, t, path, dir) })
		row.Children(func() {
			ui.Icon(c, svg).Size(14, 14).Shrink(0).Opacity(0.7)
			ui.Text(c, proto.DirName(path)).FontSize(13).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
			if f := t.files[path]; f != nil && f.modified() {
				ui.Text(c, "●").FontSize(9).TextColor(pal.attention).Shrink(0).Tooltip(locales.FilesUnsaved())
			}
			if sub := t.dirs[path]; dir && sub != nil && sub.err != "" {
				ui.Text(c, sub.err).FontSize(11.5).TextColor(pal.danger).SingleLine().Ellipsis("…").MinWidth(0).Shrink(2)
			}
		})
	}).Grow(1).MinHeight(0).Padding(0, 6, 8, 6).FontSize(13)
	t.revealed()
	if tree.Changed() && t.chosen >= 0 && t.chosen < t.outline.Rows() {
		if p := t.outline.Item(t.chosen); !t.isDir[p] {
			a.openFile(t, p)
		}
	}
	if tree.Submitted() && t.chosen >= 0 && t.chosen < t.outline.Rows() {
		p := t.outline.Item(t.chosen)
		switch {
		case t.isDir[p] && t.outline.Open.Has(p):
			t.outline.Open.Remove(p)
		case t.isDir[p]:
			t.outline.Open.Add(p)
		case !a.hostOf(t.pane).remote():
			a.openInEditor(p)
		}
	}
}

// fileMenu is the menu of a file or folder of the tree: what opens it in
// a pane, a folder's new file, folder and terminal, then on this machine,
// for a host's but this one's, its editor and file manager, and what
// copies its path.
func (a *App) fileMenu(c *ui.Context, m *ui.Menu, t *fileTree, path string, dir bool) {
	if dir {
		if padItem(m, locales.FilesNewFile()).Chosen() {
			a.defer_(func() { t.startAdding(path, false) })
		}
		if padItem(m, locales.FilesNewFolder()).Chosen() {
			a.defer_(func() { t.startAdding(path, true) })
		}
		if padItem(m, locales.FilesNewTerminal()).Chosen() {
			a.terminalIn(t.pane, path)
		}
		m.Separator()
	} else if padItem(m, locales.FilesOpenInPane()).Chosen() {
		a.openInPane(t.pane, path)
	}
	if !a.hostOf(t.pane).remote() {
		if padItem(m, locales.FilesOpenInEditor()).Chosen() {
			a.openInEditor(path)
		}
		if canOpenWith && !dir && padItem(m, locales.FilesOpenWith()).Chosen() {
			if err := openWith(path); err != nil {
				a.toast(locales.FilesCantOpen(err.Error()))
			}
		}
		if padItem(m, locales.CommonShowIn(fileManager())).Chosen() {
			revealPath(path)
		}
		m.Separator()
	}
	if padItem(m, locales.FilesCopyPath()).Chosen() {
		c.WriteClipboard(path)
	}
	if padItem(m, locales.FilesCopyRelativePath()).Chosen() {
		c.WriteClipboard(t.rel(path))
	}
}

// openInEditor opens a file or folder of this machine in the user's
// editor, else in the app the system opens it with.
func (a *App) openInEditor(path string) {
	if err := openInEditor(a.set.Editor, path); err != nil {
		a.toast(locales.FilesCantOpen(err.Error()))
	}
}

// openFile shows a file of the tree, read as it first opens; the files
// left without edits are let go.
func (a *App) openFile(t *fileTree, path string) {
	for p, f := range t.files {
		if p != path && !f.modified() {
			delete(t.files, p)
		}
	}
	t.shown = path
	if t.files[path] == nil {
		f := &openFile{path: path}
		t.files[path] = f
		a.readFile(t.pane, t.stamp, f)
	}
}

// openInPane opens a file of a pane's host in an editor pane, beside the
// pane the user is in.
func (a *App) openInPane(pane, path string) {
	s := a.sessionOfPane(pane)
	if s == nil {
		return
	}
	a.split(s, proto.CreateParams{Split: a.focusOf(s), Direction: "right", File: path})
}

// terminalIn opens a terminal in a folder of a pane's host, beside the
// pane the user is in.
func (a *App) terminalIn(pane, dir string) {
	s := a.sessionOfPane(pane)
	if s == nil {
		return
	}
	a.split(s, proto.CreateParams{Split: a.focusOf(s), Direction: "right", Dir: dir})
}

// startAdding shows the field that names a new file or folder in a folder
// of the tree, opening the folder.
func (t *fileTree) startAdding(in string, dir bool) {
	t.adding = &newEntry{in: in, dir: dir, focus: true}
	if in != t.root {
		t.outline.Open.Add(in)
	}
}

// addingIn is the folder New File and New Folder make in: the folder
// chosen in the tree, or the file's, else the tree's own.
func (t *fileTree) addingIn() string {
	if t.chosen < 0 || t.chosen >= t.outline.Rows() {
		return t.root
	}
	p := t.outline.Item(t.chosen)
	if t.isDir[p] {
		return p
	}
	return hostParent(p)
}

// newEntryView is the field that names a new file or folder: Enter makes
// it, through kerod (fs.create), and Escape leaves it.
func (a *App) newEntryView(c *ui.Context, pal *palette, t *fileTree) {
	n := t.adding
	folder := proto.DirName(n.in) + "/"
	if rel := t.rel(n.in); rel != "" {
		folder = strings.ReplaceAll(rel, `\`, "/") + "/"
	}
	place := locales.FilesNewFileIn(folder)
	if n.dir {
		place = locales.FilesNewFolderIn(folder)
	}
	ui.Column(c.Key("adding")).Shrink(0).Padding(6, 10, 4, 10).Gap(4).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c.Key("field")).AlignItems(ui.Center).Gap(6).Children(func() {
			svg := newFileIcon
			if n.dir {
				svg = newFolderIcon
			}
			ui.Icon(c, svg).Size(14, 14).TextColor(pal.textFaint).Shrink(0)
			in := textField(c, "name", &n.name, place, 0, pal)
			in.Tooltip(locales.FilesNewFileTip())
			if n.focus {
				in.Focus()
				n.focus = false
			}
			in.OnSubmit(func() { a.addEntry(t, n) })
			in.OnShortcut(0, ui.KeyEscape, func() { t.adding = nil })
			if n.busy {
				ui.Spinner(c).Size(12, 12)
			}
			iconButton(c, "cancel", closeIcon, locales.CommonCancel(), pal, func() { t.adding = nil })
		})
		if n.err != "" {
			ui.Text(c, locales.FilesCantCreate(n.err)).FontSize(11.5).TextColor(pal.danger).LineHeight(1.3)
		}
	})
}

// addEntry makes the file or folder the user named, through the kerod of
// the tree's host, then shows it: a file opens below the tree.
func (a *App) addEntry(t *fileTree, n *newEntry) {
	name := strings.TrimSpace(n.name)
	if n.busy {
		return
	}
	if name == "" {
		t.adding = nil
		return
	}
	h := a.hostOf(t.pane)
	if h.cl == nil {
		n.err = locales.HostNotConnected(h.name)
		return
	}
	path, cl := t.join(n.in, name), h.cl
	n.busy, n.err = true, ""
	go func() {
		var r proto.FSCreate
		err := cl.Call("fs.create", proto.FSCreateParams{Path: path, Dir: n.dir}, &r)
		a.update(func() { a.added(t, n, path, err) })
	}()
}

// added shows what making a new file or folder came to: the new one,
// chosen in the tree and, a file, open; or why there's none.
func (a *App) added(t *fileTree, n *newEntry, path string, err error) {
	n.busy = false
	if err != nil {
		n.err = err.Error()
		return
	}
	if t.adding == n {
		t.adding = nil
	}
	t.refresh()
	t.revealPath(path)
	if n.dir {
		t.isDir[path] = true
		return
	}
	a.openFile(t, path)
}

// filesToolbar is the tree's buttons, under it: they stay clear of the
// panel's header, which keeps the window's controls in a narrow panel.
func (a *App) filesToolbar(c *ui.Context, pal *palette, t *fileTree) {
	ui.Row(c.Key("tools")).Height(stickyH).Shrink(0).Padding(0, 6).Gap(2).AlignItems(ui.Center).Background(pal.chip).Children(func() {
		iconButton(c, "new-file", newFileIcon, locales.FilesNewFile(), pal, func() { a.defer_(func() { t.startAdding(t.addingIn(), false) }) })
		iconButton(c, "new-folder", newFolderIcon, locales.FilesNewFolder(), pal, func() { a.defer_(func() { t.startAdding(t.addingIn(), true) }) })
		ui.Spacer(c)
		iconButton(c, "refresh", reloadIcon, locales.FilesRefresh(), pal, t.refresh)
		iconButton(c, "collapse", collapseIcon, locales.FilesCollapse(), pal, func() { t.outline.Open.Clear() })
	})
}

// showFiles shows the Files view, and in it a path, as kero do files.show
// asks: the folders it is in open, a folder opens, and a file opens below
// the tree.
func (a *App) showFiles(path string) {
	if a.prefs.Side != sideFiles {
		a.prefs.Side = sideFiles
		a.savePrefs()
	}
	s := a.session(a.selected)
	if path == "" || s == nil {
		return
	}
	pane, root := a.filesRoot(s)
	if pane == nil {
		a.toast(locales.FilesNotInTree(path))
		return
	}
	t := a.tree(pane, root)
	p, ok := t.within(path)
	if !ok {
		a.toast(locales.FilesNotInTree(path))
		return
	}
	cl := a.hostOf(pane.ID).cl
	if cl == nil {
		return
	}
	go func() {
		var st proto.FSStat
		err := cl.Call("fs.stat", proto.FSStatParams{Path: p}, &st)
		a.update(func() { a.shown(t, p, st, err) })
	}()
}

// shown shows a path in the tree, as showFiles found it.
func (a *App) shown(t *fileTree, path string, st proto.FSStat, err error) {
	switch {
	case err != nil:
		a.toast(locales.FilesCantOpen(err.Error()))
		return
	case !st.Exists:
		a.toast(locales.FilesCantOpen(locales.FilesGone()))
		return
	}
	t.refresh()
	if path == t.root {
		return
	}
	t.revealPath(path)
	if st.Dir {
		t.isDir[path] = true
		t.outline.Open.Add(path)
		return
	}
	a.openFile(t, path)
}

// readFile reads a file from the kerod of a pane's host (fs.stat, then
// fs.read): an image to show, else text, which goes in its editor as its
// edits, keeping where it is scrolled. A file too large, or binary, is
// told of instead. stamp is what the file is read as of.
func (a *App) readFile(pane, stamp string, f *openFile) {
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	f.busy = true
	path, fresh := f.path, f.fresh
	image := peekable[strings.ToLower(filepath.Ext(path))]
	go func() {
		var st proto.FSStat
		var r proto.FSRead
		var bm *ui.Bitmap
		err := cl.Call("fs.stat", proto.FSStatParams{Path: path}, &st)
		absent := err == nil && !st.Exists && fresh
		large := err == nil && (image && st.Size > maxImage || !image && st.Size > maxPreview)
		if err == nil && !st.Exists && !fresh {
			err = errors.New(locales.FilesGone())
		}
		if err == nil && !large && !absent {
			err = cl.Call("fs.read", proto.FSReadParams{Path: path, Max: max(maxImage, maxPreview)}, &r)
		}
		if err == nil && !large && !absent && image {
			bm, err = ui.DecodeBitmap(r.Data)
		}
		a.update(func() {
			f.absent = absent
			f.read(stamp, large, image && !absent, bm, r.Data, err)
		})
	}()
}

// read puts what reading a file came to in its view: the image, or the
// text in its editor.
func (f *openFile) read(stamp string, large, image bool, bm *ui.Bitmap, data []byte, err error) {
	f.busy, f.stamp, f.fetched, f.large = false, stamp, time.Now(), large
	if err != nil {
		f.err = err.Error()
		return
	}
	f.err = ""
	switch {
	case large:
	case image:
		f.bm = bm
	case f.modified():
		// Edited as it was read: the edits stay, and a save tells what
		// changed on disk.
	case isBinary(data):
		f.binary, f.st = true, nil
	default:
		f.binary, f.hash = false, git.Hash(data)
		if f.absent {
			// No file to find as it was read: fs.write makes it, unless
			// another program did since.
			f.hash = ""
		}
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		if f.st == nil {
			b := editor.NewBuffer(string(data))
			b.SetLanguage(editor.LanguageFor(proto.DirName(f.path)))
			f.st = &editor.State{Buffer: b}
		} else if f.st.Buffer.String() != text {
			replaceText(f.st.Buffer, text)
			f.st.Buffer.MarkSaved()
		}
	}
}

// isBinary tells text from what is not: a NUL in its first bytes, as git
// tells, or bytes that are no UTF-8.
func isBinary(b []byte) bool {
	head := b[:min(len(b), 8000)]
	return bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(b)
}

// filesEdge is the gap between the tree and the file, which a drag moves.
func (a *App) filesEdge(c *ui.Context, pal *palette) {
	fs := &a.side.files
	d := ui.Box(c.Key("files-edge")).Height(paneGap).Shrink(0).Cursor(ui.CursorResizeRow).KeepFocus()
	if d.Hovered() || d.Pressed() {
		d.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + r.W/2 - 18, Y: r.Y + r.H/2 - 1.5, W: 36, H: 3}, pal.textFaint.Alpha(0.7), 1.5)
		})
	}
	if _, dy, ok := d.Dragged(); ok {
		if h := a.paneRef("files-body").Bounds(c).H - paneGap; h > 0 {
			if !fs.dragging {
				fs.dragging, fs.drag = true, fs.share
				if fs.drag == 0 {
					fs.drag = treeShare
				}
			}
			fs.drag += dy / h
			fs.share = min(max(fs.drag, 0.12), 0.88)
		}
	} else if !d.Pressed() {
		fs.dragging = false
	}
	if d.DoubleClicked() {
		fs.share = 0
	}
}

// fileView is the file shown: its header, then its text in its editor, or
// its image, or why it shows neither.
func (a *App) fileView(c *ui.Context, pal *palette, t *fileTree, f *openFile, working bool) {
	ui.Column(c.Key("open-" + f.path)).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		hdr := ui.Row(c.Key("hdr")).Height(stickyH).Shrink(0).Padding(0, 6, 0, 12).Gap(8).AlignItems(ui.Center).Background(pal.chip)
		hdr.ContextMenu(func(m *ui.Menu) { a.fileMenu(c, m, t, f.path, false) })
		hdr.Children(func() {
			ui.Icon(c, docIcon).Size(13, 13).TextColor(pal.textFaint).Shrink(0)
			// The path shows over the name and its folder, not over the
			// buttons, nor the room between them.
			ui.Text(c, proto.DirName(f.path)).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").Shrink(0).MaxWidth(220).Tooltip(f.path)
			rel := t.rel(f.path)
			ui.Text(c, strings.TrimSuffix(strings.TrimSuffix(rel, proto.DirName(f.path)), "/")).FontSize(12).TextColor(pal.textFaint).
				SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).Tooltip(f.path)
			ui.Spacer(c)
			a.fileButtons(c, pal, t.pane, f)
			iconButton(c, "pane", splitRightIcon, locales.FilesOpenInPane(), pal, func() { a.openInPane(t.pane, f.path) })
			iconButton(c, "close", closeIcon, locales.FilesCloseFile(), pal, func() { a.defer_(func() { t.shown, t.chosen = "", -1 }) })
		})
		a.fileBody(c, pal, t.pane, f, nil)
	})
}

// fileButtons are what a file's header has to act on it: its save while
// it has edits, the wrapping of its lines, and on this machine, its
// editor.
func (a *App) fileButtons(c *ui.Context, pal *palette, pane string, f *openFile) {
	switch {
	case f.saving:
		ui.Spinner(c).Size(12, 12)
	case f.modified():
		ui.Text(c, locales.FilesUnsaved()).FontSize(11.5).TextColor(pal.attention).SingleLine().Shrink(0)
		iconButton(c, "save", checkIcon, a.saveTip(), pal, func() { a.saveFile(pane, f, false) })
	case f.absent && f.st != nil:
		ui.Text(c, locales.FilesNew()).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0).Tooltip(locales.FilesNewTip())
		iconButton(c, "save", checkIcon, a.saveTip(), pal, func() { a.saveFile(pane, f, false) })
	}
	if f.st != nil {
		wrap := ui.ButtonBase(c.Key("wrap")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.FilesWrap()).Tooltip(locales.FilesWrap())
		switch {
		case a.prefs.Wrap:
			wrap.Background(pal.rowHover).TextColor(pal.text)
		case wrap.Hovered() || wrap.Pressed():
			wrap.Background(pal.rowHover.Alpha(0.5)).TextColor(pal.text)
		}
		wrap.Children(func() { ui.Icon(c, wrapIcon).Size(15, 15) })
		if wrap.Clicked() {
			a.prefs.Wrap = !a.prefs.Wrap
			a.savePrefs()
		}
	}
	if !a.hostOf(pane).remote() {
		iconButton(c, "open", openFileIcon, locales.FilesOpenInEditor(), pal, func() { a.openInEditor(f.path) })
	}
}

// fileBody is a file's text in its editor, which ref binds, or its image,
// or why it shows neither; and what a save found changed on disk.
func (a *App) fileBody(c *ui.Context, pal *palette, pane string, f *openFile, ref *ui.Handle) {
	col := ui.Column(c.Key("body")).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	col.Children(func() {
		if f.changed {
			a.fileChanged(c, pal, pane, f)
		}
		switch {
		case f.err != "" && f.st == nil && f.bm == nil:
			a.sectionNote(c, pal, locales.FilesCantRead(f.err))
		case f.large:
			a.sectionNote(c, pal, locales.ChangesTooLarge())
		case f.binary:
			a.sectionNote(c, pal, locales.ChangesBinary())
		case f.bm != nil:
			ui.Box(c.Key("image")).Grow(1).MinHeight(0).Padding(12).Center().Children(func() {
				ui.Image(c, f.bm).Fill().Fit(ui.Contain)
			})
		case f.st == nil:
			ui.Column(c.Key("loading")).Grow(1).Center().Children(func() { ui.Spinner(c) })
		default:
			st := f.st
			st.Font = ui.Font{Family: a.termFamily(), Size: max(a.set.FontSize-1, 10)}
			st.Theme = a.codeTheme(c, pal)
			st.Label = proto.DirName(f.path)
			st.SoftWrap = a.prefs.Wrap
			// What shows as it saves is what it writes.
			st.ReadOnly = f.saving
			ed := editor.View(c.Key("text"), st).Grow(1).MinHeight(0)
			if ref != nil {
				ed.Bind(ref)
			}
		}
	})
	if col.Shortcut(ui.Cmd, ui.KeyS) && (f.modified() || f.absent) && !f.saving {
		a.saveFile(pane, f, false)
	}
}

// saveTip is the save button's tooltip, with its shortcut.
func (a *App) saveTip() string {
	return locales.FilesSave() + " (" + actions.Glyphs("CmdOrCtrl+S") + ")"
}

// fileChanged tells that the file changed on disk since it was read, as a
// save found: reloading it drops the edits, overwriting it the change.
func (a *App) fileChanged(c *ui.Context, pal *palette, pane string, f *openFile) {
	ui.Row(c.Key("changed")).Shrink(0).Padding(6, 6, 6, 12).Gap(6).AlignItems(ui.Center).Background(pal.attnSoft).Children(func() {
		ui.Text(c, locales.FilesChangedOnDisk()).FontSize(12.5).TextColor(pal.attention).Grow(1).MinWidth(0).LineHeight(1.3)
		if button(c, "reload", locales.FilesReload(), "", false, pal) {
			f.changed = false
			if f.st != nil {
				f.st.Buffer.Reset(f.st.Buffer.String())
			}
			f.hash = ""
			a.readFile(pane, f.stamp, f)
		}
		if button(c, "overwrite", locales.FilesOverwrite(), "", false, pal) {
			a.saveFile(pane, f, true)
		}
	})
}

// saveFile writes a file's text through kerod, as its line endings were,
// only as it was read; over is the user overwriting what changed on disk
// since, which it reads first for its hash.
func (a *App) saveFile(pane string, f *openFile, over bool) {
	h := a.hostOf(pane)
	cl := h.cl
	if f.st == nil {
		return
	}
	if cl == nil {
		a.toast(locales.FilesCantSave(locales.HostNotConnected(h.name)))
		return
	}
	var buf bytes.Buffer
	f.st.Buffer.WriteTo(&buf)
	data, version, hash, path := buf.Bytes(), f.st.Buffer.Version(), f.hash, f.path
	f.saving = true
	go func() {
		var err error
		if over {
			var r proto.FSRead
			if err = cl.Call("fs.read", proto.FSReadParams{Path: path, Max: maxPreview}, &r); err == nil {
				hash = git.Hash(r.Data)
			}
		}
		var w proto.FSWrite
		if err == nil {
			err = cl.Call("fs.write", proto.FSWriteParams{Path: path, Hash: hash, Data: data}, &w)
		}
		a.update(func() {
			if err != nil {
				a.toast(locales.FilesCantSave(err.Error()))
			}
			f.saved(version, w, err)
		})
	}()
}

// saved puts what a save of the editor's text at version came to in the
// file's view.
func (f *openFile) saved(version uint64, w proto.FSWrite, err error) {
	f.saving = false
	switch {
	case err != nil:
	case w.Changed:
		f.changed = true
	default:
		f.changed, f.hash, f.absent = false, w.Hash, false
		// Typed on as it saved: what was typed since is not saved.
		if f.st.Buffer.Version() == version {
			f.st.Buffer.MarkSaved()
		}
	}
}
