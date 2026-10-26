package main

import (
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/actions"
	"kero/internal/proto"

	"kero/locales"
)

// Go to File finds a file of the project the user is in by its name, or
// its path, as they type, and opens it in an editor pane; openAt opens one
// at a line, as the Search view's matches do.

// gotoKey is what a.gotos keeps the place of a file about to open by: its
// host's, and its path.
func gotoKey(h *host, file string) string { return h.name + "\x00" + file }

// openAt opens a file of a pane's host in an editor pane with the caret at
// a line, from 1: in an editor pane of the session that shows it already,
// else as openInPane opens it.
func (a *App) openAt(pane, file string, line int) {
	s := a.sessionOfPane(pane)
	if s == nil {
		return
	}
	pos := editor.Pos{Line: max(line-1, 0)}
	h := a.hostOf(pane)
	for i := range s.Panes {
		p := &s.Panes[i]
		if p.Editor() && p.File == file && a.hostOf(p.ID) == h {
			f := a.editorOf(p)
			// The line is in the text, which a preview doesn't show.
			f.goTo, f.preview = &pos, false
			a.setFocus(s, p.ID)
			a.paneRef(p.ID).Focus()
			return
		}
	}
	a.goToLine(pane, file, line)
	a.openInPane(pane, file)
}

// goToLine has the editor pane a file of a pane's host opens in next put
// the caret at a line, from 1; 0 leaves it where the file opens.
func (a *App) goToLine(pane, file string, line int) {
	if line < 1 {
		return
	}
	if a.gotos == nil {
		a.gotos = map[string]editor.Pos{}
	}
	a.gotos[gotoKey(a.hostOf(pane), file)] = editor.Pos{Line: line - 1}
}

// goToFile is Go to File's: the files of the project of the pane the user
// is in, as kerod lists them (fs.files), and those the query finds.
type goToFile struct {
	open bool
	host *host
	pane string // the pane the files open beside
	root string
	// paths are the files, from root, nil while they come; err why they
	// did not, more that there are more; gen tells the listing asked for
	// last.
	paths []string
	more  bool
	err   string
	gen   int
	query string
	index int
	// found are the paths the query finds, best first, as of found's
	// query and paths.
	found   []string
	foundQ  string
	foundOf int
	scroll  ui.ScrollState
	reveal  bool
	// cache is the files listed last, by gotoKey of their root, with
	// dotfiles or without as cacheHidden tells.
	cache       map[string][]string
	cacheHidden bool
}

// maxGoToShown bounds the files Go to File shows: a query finds the rest.
const maxGoToShown = 200

func (a *App) openGoToFile() {
	s := a.session(a.selected)
	if s == nil {
		return
	}
	p, root := a.filesRoot(s)
	if p == nil || root == "" {
		a.toast(locales.GoToFileNoFolder())
		return
	}
	h := a.hostOf(p.ID)
	g := &a.gotoF
	cache := g.cache
	if g.cacheHidden != a.set.HiddenFiles {
		cache = nil
	}
	*g = goToFile{open: true, host: h, pane: a.focusOf(s), root: root, gen: g.gen + 1, reveal: true, cache: cache, cacheHidden: a.set.HiddenFiles, foundOf: -1}
	// What was listed of the folder last shows at once, while it is listed
	// anew.
	g.paths = g.cache[gotoKey(h, root)]
	cl := h.cl
	if cl == nil {
		g.err = locales.HostNotConnected(h.name)
		return
	}
	gen, hidden := g.gen, a.set.HiddenFiles
	go func() {
		var r proto.FSFiles
		err := cl.Call("fs.files", proto.FSFilesParams{Dir: root, Hidden: hidden}, &r)
		a.update(func() {
			g := &a.gotoF
			if g.gen != gen {
				return
			}
			if err != nil {
				g.err = err.Error()
				return
			}
			g.paths, g.more, g.foundOf = r.Paths, r.More, -1
			if g.cache == nil {
				g.cache = map[string][]string{}
			}
			g.cache[gotoKey(h, root)] = r.Paths
		})
	}()
}

// goToFound are the files the query finds, best first: by their names
// before their paths, and a query with a slash by their paths alone.
func (g *goToFile) goToFound() []string {
	q := strings.TrimSpace(strings.ReplaceAll(g.query, `\`, "/"))
	if g.foundQ == q && g.foundOf == len(g.paths) && g.found != nil {
		return g.found
	}
	g.foundQ, g.foundOf = q, len(g.paths)
	if q == "" {
		g.found = g.paths[:min(len(g.paths), maxGoToShown)]
		if g.found == nil {
			g.found = []string{}
		}
		return g.found
	}
	type scored struct {
		path  string
		score int
	}
	byPath := strings.Contains(q, "/")
	var found []scored
	for _, p := range g.paths {
		sc := -1
		if !byPath {
			if sc = fuzzy(q, path.Base(p)); sc >= 0 {
				sc += 1 << 20
			}
		}
		if sc < 0 {
			sc = fuzzy(q, p)
		}
		if sc >= 0 {
			found = append(found, scored{p, sc})
		}
	}
	// Of the same score, the shorter path first, as the nearer the root.
	slices.SortStableFunc(found, func(x, y scored) int {
		if x.score != y.score {
			return y.score - x.score
		}
		return len(x.path) - len(y.path)
	})
	g.found = make([]string, 0, min(len(found), maxGoToShown))
	for _, f := range found[:min(len(found), maxGoToShown)] {
		g.found = append(g.found, f.path)
	}
	return g.found
}

func (a *App) goToFileView(c *ui.Context, pal *palette) {
	g := &a.gotoF
	if !g.open {
		return
	}
	found := g.goToFound()
	g.index = min(max(g.index, 0), max(len(found)-1, 0))
	var chosen string
	newPane := false
	ui.DialogBase(c.Key("goto"), &g.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay).Justify(ui.Start).Padding(84, 0, 0, 0)
		floating(panel.Width(600).MaxHeight(460).Radius(14).Background(pal.popover), pal).Clip().
			Label(locales.ActionGoToFile())
		ui.Row(c).Height(50).Shrink(0).Padding(0, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, docIcon).Size(16, 16).TextColor(pal.textMuted)
			in := ui.TextInputBase(c.Key("q"), &g.query).Grow(1).FontSize(15).Placeholder(locales.GoToFilePlaceholder(proto.DirName(g.root))).AutoFocus()
			if in.Changed() {
				g.index, g.reveal = 0, true
			}
			if c.Shortcut(0, ui.KeyDown) || c.Shortcut(ui.Ctrl, ui.KeyN) {
				g.index, g.reveal = min(g.index+1, len(found)-1), true
			}
			if c.Shortcut(0, ui.KeyUp) || c.Shortcut(ui.Ctrl, ui.KeyP) {
				g.index, g.reveal = max(g.index-1, 0), true
			}
			switch {
			case len(found) == 0:
			case c.Shortcut(ui.Cmd, ui.KeyEnter):
				chosen, newPane = found[g.index], true
			case in.Submitted() || c.Shortcut(0, ui.KeyEnter):
				chosen = found[g.index]
			}
		})
		list := ui.Scroll(c.Key("files")).Grow(1).MinHeight(0).Padding(6).TrackScroll(&g.scroll)
		reveal := g.reveal
		g.reveal = false
		list.Children(func() {
			switch {
			case g.err != "" && g.paths == nil:
				ui.Text(c, g.err).FontSize(13).TextColor(pal.danger).Padding(12, 12).Selectable()
			case g.paths == nil:
				ui.Row(c).Padding(12).Children(func() { ui.Spinner(c) })
			case len(found) == 0:
				ui.Text(c, locales.PaletteNoMatches()).FontSize(13).TextColor(pal.textFaint).Padding(12, 12)
			}
			for i, p := range found {
				row := ui.Row(c.Key(p)).Height(34).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(8)
				if i == g.index {
					row.Background(pal.accent.Alpha(0.12))
					if reveal {
						row.ScrollIntoView()
					}
				} else if row.Hovered() {
					row.Background(pal.rowHover)
				}
				if row.Clicked() {
					chosen = p
				}
				row.Children(func() {
					ui.Icon(c, docIcon).Size(14, 14).TextColor(pal.textFaint).Shrink(0)
					ui.Text(c, path.Base(p)).FontSize(13.5).TextColor(pal.text).SingleLine().Ellipsis("…").Shrink(0).MaxWidth(300)
					if dir := path.Dir(p); dir != "." {
						ui.Text(c, dir).FontSize(12.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
					}
				})
			}
		})
		ui.Row(c).Height(40).Shrink(0).Padding(0, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			hint := locales.GoToFileHint(actions.Glyphs("CmdOrCtrl+Enter"))
			if g.more {
				hint = locales.GoToFileMore(len(g.paths))
			}
			ui.Text(c, hint).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		})
	})
	if chosen != "" {
		g.open = false
		file := joinRel(g.root, chosen)
		if newPane {
			a.openInNewPane(g.pane, file)
		} else {
			a.openInPane(g.pane, file)
		}
		return
	}
	if !g.open {
		a.focusTerminal()
	}
}
