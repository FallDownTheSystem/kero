package main

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// The right sidebar stands at the window's right edge, on its wash as the
// left sidebar does, with what the session's worktree is about, in tabs:
// Changes, the files the session's agent or the user changed, in their
// folders, which Uncommitted stages and commits (staging.go,
// commitbox.go); Files, the worktree's tree (files.go), and Search, its
// files' text (search.go); History, its commits (history.go); and Review,
// its pull requests (pulls.go). What is chosen in Changes, Files and
// Search opens in the viewer, a card beside the panes: Changes' as the
// diffs of them all, one after the other, scrolled to it; a file as its
// text; Review's pull request as its page. History shows a commit's diffs
// in the tab. The Changes and History tabs have the worktree's branch
// atop them (gitbar.go).

// The sidebar's tabs, as prefs.Side holds them, and the viewer's kinds.
const (
	sideChanges = "changes"
	sideFiles   = "files"
)

const (
	defaultSide = 420 // the viewer's width, as it first opens
	minSide     = 300
	minPanes    = 360 // the room the viewer leaves the panes
	splitWidth  = 760 // past which diff-layout auto shows diffs side by side
)

// sideState is the right sidebar's and the viewer's, beside the prefs
// that show them and size them.
type sideState struct {
	drag    float32 // the viewer's width as a drag of its edge began, 0 while none
	barDrag float32 // the sidebar's, as a drag of its edge began
	barW    float32 // the sidebar's width as it shows this frame, 0 while hidden
	// crowded tells that the window is too narrow for the sidebar beside
	// the cards (minCards), which it then leaves to them, shown as it is,
	// unless the user shows it anyway, forced, until the window widens.
	crowded, forced bool
	// rowW is the width of the row of the cards, the panes and the viewer,
	// this frame.
	rowW    float32
	edge    float32 // the cards' column's right padding this frame, none beside the sidebar
	focused bool    // the viewer has the keyboard: its Escape, its find
	// wide zooms the viewer: it takes the panes' room, hiding them, as
	// Zoom does a pane.
	wide bool
	// barWide zooms the right sidebar, as wide does the viewer: it takes
	// the cards' room, the panes' and the viewer's, hiding them; zoomW is
	// its width so zoomed this frame, 0 while it isn't.
	barWide bool
	zoomW   float32
	// barFocused tells that the sidebar has the keyboard, as it last
	// built: Find opens Search there.
	barFocused bool
	// editing tells that an editor of the viewer's, a diff or a file, has
	// the keyboard, as the viewer last built it.
	editing bool
	// drafting is the review whose draft comment has the keyboard, as the
	// viewer last built it, which its Escape drops.
	drafting *review
	changes  changesState
	files    filesState
	search   searchState
	pulls    pullsState
	git      gitUI
	// views are what the viewer shows, by session; backup what the app
	// keeps of them and of their edits not saved as it quits (backup.go).
	views  map[string]*sideView
	backup backupState
	// cf is the Changes tab's for this frame, which the sidebar's tree
	// and the viewer's diffs share (changesFrameOf).
	cf *changesFrame
	// theme is the colors of code, made again only as the theme they are
	// of, themeOf, changes.
	theme   *editor.Theme
	themeOf terminal.Theme
}

// sideView is what a session's viewer shows: the diffs of Changes, a file
// of Files (or Search), or the pull request Review opened; kind is "" while
// it is closed.
type sideView struct {
	kind string
	file *fileView // the file of Files it shows last
}

// changesState is the Changes tab's: the files changed since a base,
// each with its diff, one after the other in one scroll.
type changesState struct {
	base string // the base chosen: git.BaseTurn, BaseHead or BaseBranch
	// rows are the viewer's list's, as it last built them.
	rows []changeRow
	// scope is the pane and base the list is of, and stamp what of its
	// worktree it was fetched at; busy while a fetch is under way.
	scope   string
	stamp   string
	busy    bool
	since   time.Time
	fetched time.Time
	res     *proto.GitChanges
	err     string
	// lists are the lists fetched, by scope, which a base shows again at
	// once as the user comes back to it, while it is fetched anew; shown
	// is the scope the view last showed, and switched when it came to.
	lists    map[string]*proto.GitChanges
	shown    string
	switched time.Time
	// files are the files' diffs, by scope and path, kept as the list is
	// fetched anew; collapsed the files the user folded or unfolded, by
	// the same key; focus the file whose diff has the keyboard, for Find.
	files     map[string]*sideFile
	collapsed map[string]bool
	focus     *sideFile
	// places are the diffs' places, by scope.
	places map[string]*ui.ListState
	// indexes are Uncommitted's lists, staged and not, by scope, of
	// which lists has the files of both; moves the files staged or
	// unstaged that the index fetched last may not show moved yet, which
	// the lists show moved at once.
	indexes map[string]*proto.GitIndex
	moves   []*indexMove
	// toStash are the files of Uncommitted the user chose to stash, by
	// scope and path, which the tree lists in a section of their own
	// (treeSections); stashName the name typed for the stash, by scope.
	toStash   map[string]map[string]bool
	stashName map[string]string
	// tree is the tree of the files of treeOf, the list it was made of
	// (changes_tree.go); treeClosed the folders the user folded in it, by
	// scope and path; treePlaces its places, by scope. picked is the file
	// last chosen in it, and current the one it showed as there, which it
	// scrolls to as that changes; reveal the one the diffs are to scroll
	// to, as they come.
	tree       []*treeNode
	treeOf     *proto.GitChanges
	trees      map[string]treeCache // Uncommitted's sections', by section
	treeClosed map[string]bool
	treePlaces map[string]*ui.ListState
	picked     string
	current    string
	reveal     string
}

// sideFile is a changed file's diff since a base, or between two sides:
// staged, unstaged, or a commit's (base "rev", its sides those of f); the
// user edits its new side while it is the file on disk.
type sideFile struct {
	pane, base string
	root       string // the worktree's, on the host
	edits      edited
	f          proto.GitFile     // the file as listed when its text was asked for
	d          *editor.DiffState // nil until its text comes, or for a binary file
	patch      bool              // d shows a patch: a file too large to show whole
	hash       string            // the new side's, the file's on disk for the working tree, as d shows it
	index      string            // the index's, when a side is
	text       proto.GitText     // what came of it last, but its text
	stamp      string            // the worktree's stamp it was fetched at
	busy       bool
	writing    bool
	err        string
	pr         *prPage // the pull request whose change it is, nil for none
}

// known is what of the file's text the client has, as git.file takes it:
// the index's too, when it is the old side.
func (sf *sideFile) known() string {
	if sf.base == baseUnstaged {
		return sf.index + sf.hash
	}
	return sf.hash
}

// onDisk tells that the diff's new side is the file on disk, which the
// user edits, as its turn's, its worktree's or its branch's changes have
// it: not the index's, a commit's or a pull request's.
func (sf *sideFile) onDisk() bool {
	switch sf.base {
	case git.BaseTurn, git.BaseHead, git.BaseBranch, baseUnstaged:
		return sf.pr == nil
	}
	return false
}

// The sides of the Uncommitted list's diffs, and of a commit's, as
// git.file takes them for its base.
const (
	baseStaged   = "staged"
	baseUnstaged = "unstaged"
	baseStash    = "stash" // the tree's section of the files to stash, no side of a diff
	baseRev      = "rev"
)

// fileKey is the key a list's file's diff and fold are kept by: of its
// section too, as Uncommitted lists a file both staged and not.
func fileKey(scope, section, path string) string {
	if section == "" {
		return scope + "|" + path
	}
	return scope + "|" + section + "|" + path
}

// sideTab is the tab the sidebar shows, or showed last while it hides.
func (a *App) sideTab() string {
	tab := a.prefs.Side
	if tab == "" {
		tab = a.prefs.SideTab
	}
	return knownTab(tab)
}

// knownTab is a tab of the sidebar: Changes for one it doesn't know.
func knownTab(tab string) string {
	switch tab {
	case sideFiles, sideSearch, sideHistory, sideReview:
		return tab
	}
	return sideChanges
}

// sideAction is the action that shows and hides a tab of the sidebar.
func sideAction(tab string) string {
	switch tab {
	case sideFiles:
		return "files.toggle"
	case sideHistory:
		return "history.toggle"
	case sideSearch:
		return "search.toggle"
	case sideReview:
		return "review.toggle"
	}
	return "changes.toggle"
}

// showTab shows a tab of the sidebar.
func (a *App) showTab(tab string) {
	if a.prefs.Side == tab && a.prefs.SideTab == tab && a.rightShown() {
		return
	}
	a.prefs.Side, a.prefs.SideTab = tab, tab
	a.side.forced = a.side.crowded
	a.savePrefs()
}

// toggleSide shows a tab of the sidebar, and for Changes their diffs; or,
// when the sidebar shows it, hides the sidebar, closing the viewer with
// what the tab opened in it.
func (a *App) toggleSide(tab string) {
	s := a.session(a.selected)
	if a.prefs.Side == tab && a.rightShown() {
		a.prefs.Side, a.prefs.SideTab = "", tab
		a.side.forced = false
		if v := a.viewOf(s); v != nil && v.kind == viewKind(tab) {
			a.closeView(s)
		}
		a.focusTerminal()
	} else {
		a.prefs.Side, a.prefs.SideTab = tab, tab
		a.side.forced = a.side.crowded
		if tab == sideChanges && s != nil {
			a.openView(s, sideChanges, nil)
			// A program asking through kero do leaves the user's keyboard be.
			if a.from == nil {
				a.focusViewer()
			}
		}
	}
	a.savePrefs()
}

// viewKind is the kind of what a tab opens in the viewer: Search's files
// open as Files' do.
func viewKind(tab string) string {
	if tab == sideSearch {
		return sideFiles
	}
	return tab
}

// toggleRight shows the sidebar, on the tab it showed last, or hides it.
func (a *App) toggleRight() {
	if a.rightShown() {
		a.prefs.Side, a.prefs.SideTab = "", a.prefs.Side
		a.side.forced, a.side.barWide = false, false
	} else {
		if a.prefs.Side == "" {
			a.prefs.Side = a.sideTab()
		}
		a.side.forced = a.side.crowded
	}
	a.savePrefs()
}

// minCards is the room the right sidebar leaves the cards: in a window
// narrower, it gets out of their way (sideState.crowded).
const minCards = 420

// rightShown tells that the right sidebar shows, or comes: shown, and the
// window wide enough for it, or shown anyway.
func (a *App) rightShown() bool {
	return a.prefs.Side != "" && (!a.side.crowded || a.side.forced)
}

// viewOf is what a session's viewer shows, nil while it is closed.
func (a *App) viewOf(s *proto.Session) *sideView {
	if s == nil {
		return nil
	}
	if v := a.side.views[s.ID]; v != nil && v.kind != "" {
		return v
	}
	return nil
}

// openView opens a session's viewer on the diffs of Changes, or on a file
// of Files, f, or the one it showed last when nil.
func (a *App) openView(s *proto.Session, kind string, f *fileView) {
	if a.side.views == nil {
		a.side.views = map[string]*sideView{}
	}
	v := a.side.views[s.ID]
	if v == nil {
		v = &sideView{}
		a.side.views[s.ID] = v
	}
	v.kind = kind
	// The viewer the sidebar zoomed over would not show.
	a.side.barWide = false
	if f != nil {
		v.file = f
	}
}

// closeView closes a session's viewer, giving the keyboard back to its
// pane when the viewer had it.
func (a *App) closeView(s *proto.Session) {
	v := a.viewOf(s)
	if v == nil {
		return
	}
	// Asking first to save what the user edited in it.
	a.askToSave(a.viewTargets(v), func() {
		v.kind = ""
		a.side.wide = false
		if a.side.focused {
			a.side.focused = false
			a.focusTerminal()
		}
	})
}

// focusViewer gives the viewer the keyboard, as it comes if it has yet to:
// a click that opens it leaves the keyboard with no one, where Close would
// take the pane beside it.
func (a *App) focusViewer() { a.paneRef("side-card").Focus() }

// rightWidth is the sidebar's width, shown.
func (a *App) rightWidth() float32 {
	if a.side.zoomW > 0 {
		return a.side.zoomW
	}
	if w := a.prefs.RightWidth; w != 0 {
		return w
	}
	return defaultSidebar
}

// rightHandle is the sidebar's edge, which a drag moves: in the window,
// over the edge between the cards and the sidebar, right wide, from the
// window's right, or its left with the sidebar there (sideLeft).
func (a *App) rightHandle(c *ui.Context, right float32) {
	h := ui.Box(c.Key("resize-right")).Absolute().Top(0).Bottom(0).Width(9).Cursor(ui.CursorResizeColumn).KeepFocus()
	left := a.sideLeft()
	if left {
		h.Left(right - 4)
	} else {
		h.Right(right - 5)
	}
	if dx, _, ok := h.Dragged(); ok {
		if a.side.barDrag == 0 {
			a.side.barDrag = a.rightWidth()
		}
		if left {
			dx = -dx
		}
		a.side.barDrag -= dx
		a.prefs.RightWidth = min(max(a.side.barDrag, minSidebar), maxRight)
	} else if a.side.barDrag != 0 && !h.Pressed() {
		a.side.barDrag = 0
		a.savePrefs()
	}
	if h.DoubleClicked() {
		a.prefs.RightWidth = 0
		a.savePrefs()
	}
}

// rightSidebar is the sidebar at the window's right, or its left with the
// sessions at the right (sideLeft): its tabs, in a title bar as the left
// sidebar's, then the tab's view.
func (a *App) rightSidebar(c *ui.Context, pal *palette, s *proto.Session) {
	tab := a.sideTab()
	left := a.sideLeft()
	// The bar's tabs and button slide in from the window's edge with the
	// sidebar, under the window controls, which stay in the window's
	// corner (mainControls): they show up to the controls, not under them.
	w := a.rightWidth() - 10
	shown := w
	if showControls && a.win != nil && !left {
		w -= controlsW + 8
		shown = min(max(a.side.barW-a.side.edge-10-(controlsW+8), 0), w)
	}
	// The tabs, labeled while they fit, else the chosen one's label alone,
	// else their icons: the button that hides the sidebar takes its room.
	tabs := []sideTabInfo{
		{sideFiles, folderIcon, locales.SideFiles()},
		{sideChanges, diffIcon, locales.SideChanges()},
		{sideHistory, historyIcon, locales.SideHistory()},
		{sideReview, prIcon, locales.SideReview()},
	}
	full, chosenOnly := float32(2*controlW+12), float32(2*controlW+12)
	for _, t := range tabs {
		lw, _ := c.MeasureText(0, ui.Span{Text: t.label, Size: 13, Weight: 500})
		full += lw + 20 + 2
		if t.id == knownTab(tabOf(tab)) {
			chosenOnly += lw + 20 + 2
		} else {
			chosenOnly += tabIconW + 2
		}
	}
	compact := tabsLabels
	switch {
	case full > w && chosenOnly <= w:
		compact = tabsChosenLabel
	case full > w:
		compact = tabsIcons
	}
	ui.Row(c.Key("right-bar")).Height(topBar).Shrink(0).AlignItems(ui.Stretch).DragWindow().Children(func() {
		bar := ui.Row(c.Key("shown")).Width(shown).Shrink(0).AlignItems(ui.Stretch).ClipX()
		if left {
			// At the window's left, clear of the traffic lights.
			bar.Margin(0, 0, 0, a.chrome.headerPad-cardGap)
		}
		bar.Children(func() {
			ui.Row(c.Key("tabs")).Width(w).Shrink(0).AlignItems(ui.Center).Padding(0, 0, 0, 10).Gap(2).Children(func() {
				for _, t := range tabs {
					a.tabButton(c, pal, t, tab, compact)
				}
				ui.Spacer(c)
				if a.side.barWide {
					iconButton(c, "zoom-right", unzoomIcon, locales.PaneShowAll(), pal, func() { a.side.barWide = false })
				} else {
					iconButton(c, "zoom-right", zoomIcon, locales.SideBarZoom(), pal, func() {
						a.side.barWide = true
						// The panes it hides can't keep the keyboard.
						a.paneRef("side-bar").Focus()
					})
				}
				svg := sidebarRightIcon
				if left {
					svg = sidebarIcon
				}
				iconButton(c, "hide-right", svg, a.tip(locales.RightSidebarHide(), "rightSidebar.toggle"), pal, a.toggleRight)
			})
		})
	})
	body := ui.Column(c.Key("tab-" + tab)).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Bind(a.paneRef("side-bar"))
	a.side.barFocused = body.FocusWithin()
	body.Children(func() {
		switch tab {
		case sideFiles:
			a.filesSide(c, pal, s)
		case sideSearch:
			a.searchView(c, pal, s)
		case sideHistory:
			a.historyView(c, pal, s, a.rightWidth())
		case sideReview:
			a.pullsView(c, pal, s, a.rightWidth())
		default:
			a.changesSide(c, pal, s)
		}
		a.nameView(c, pal)
	})
	if body.Shortcut(0, ui.KeyEscape) {
		a.focusTerminal()
	}
}

// sideTabInfo is a tab of the sidebar: what it shows, its icon and label.
type sideTabInfo struct {
	id    string
	svg   *ui.SVG
	label string
}

// tabOf is the tab that stands for a tab: Files' for Search, which the
// Files tab opens.
func tabOf(tab string) string {
	if tab == sideSearch {
		return sideFiles
	}
	return tab
}

// sideCompact is how much of their labels the sidebar's tabs show.
type sideCompact int

const (
	tabsLabels sideCompact = iota
	tabsChosenLabel
	tabsIcons
)

// tabIconW is a tab's width with its icon alone.
const tabIconW = 30

// tabButton is a tab of the sidebar, its own chosen as a session's row is,
// in the same weight as the others: its label, or as compact asks, its
// icon alone.
func (a *App) tabButton(c *ui.Context, pal *palette, t sideTabInfo, tab string, compact sideCompact) {
	on := t.id == tabOf(tab)
	iconOnly := compact == tabsIcons || compact == tabsChosenLabel && !on
	b := ui.ButtonBase(c.Key("tab-"+t.id)).Height(26).Padding(0, 10).Radius(7).Center().Label(t.label).Tooltip(a.tip(t.label, sideAction(t.id)))
	if iconOnly {
		b.Width(tabIconW).Padding(0)
	}
	bg, fg := ui.Transparent, pal.textMuted
	switch {
	case on:
		bg, fg = pal.rowSelected, pal.text
		chosen(b, pal)
	case b.Hovered():
		bg, fg = pal.rowHover, pal.text
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	b.Children(func() {
		if iconOnly {
			ui.Icon(c, t.svg).Size(14, 14).TextColor(fg)
			return
		}
		ui.Text(c, t.label).FontSize(13).FontWeight(500).TextColor(fg).SingleLine()
	})
	if b.Clicked() && !on {
		id := t.id
		a.defer_(func() { a.showTab(id) })
	}
}

// sideLeft tells that the right sidebar stands at the window's left, and
// the viewer at the left of the panes: the sessions' sidebar takes the
// window's right (settings' SessionsSide).
func (a *App) sideLeft() bool { return a.set.SessionsSide == "right" }

// sideWidth is the viewer's width in a row of width w, leaving the panes
// their room, or half of a row too narrow for both.
func (a *App) sideWidth(w float32) float32 {
	sw := a.prefs.SideWidth
	if sw == 0 {
		sw = defaultSide
	}
	switch {
	case w <= 0:
	case w < minPanes+minSide+paneGap:
		// Too narrow for either's least: they share it, the viewer never
		// running past the row, under the right sidebar.
		return (w - paneGap) / 2
	default:
		sw = min(sw, w-minPanes)
	}
	return max(sw, minSide)
}

// sideEdge is the gap between the panes and the viewer, which a drag
// moves.
func (a *App) sideEdge(c *ui.Context, pal *palette, row float32) {
	d := ui.Box(c.Key("side-edge")).Width(paneGap).Shrink(0).Cursor(ui.CursorResizeColumn)
	if d.Hovered() || d.Pressed() {
		d.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + r.W/2 - 1.5, Y: r.Y + r.H/2 - 18, W: 3, H: 36}, pal.textFaint.Alpha(0.7), 1.5)
		})
	}
	if dx, _, ok := d.Dragged(); ok {
		if a.side.drag == 0 {
			a.side.drag = a.sideWidth(row)
		}
		if a.sideLeft() {
			// The viewer at the panes' left widens to the right.
			dx = -dx
		}
		a.side.drag -= dx
		a.prefs.SideWidth = min(max(a.side.drag, minSide), max(row-minPanes, minSide))
	} else if a.side.drag != 0 && !d.Pressed() {
		a.side.drag = 0
		a.savePrefs()
	}
	if d.DoubleClicked() {
		a.prefs.SideWidth = defaultSide
		a.savePrefs()
	}
}

// viewerCard is the viewer, a card beside the panes as theirs: a header
// saying what it shows, then the diffs, the file, the commit or the pull
// request. It is the card at the window's edge, whose header keeps room
// for the window's controls (controlsRoom).
func (a *App) viewerCard(c *ui.Context, pal *palette, s *proto.Session, v *sideView, width float32) {
	card := ui.Column(c.Key("side")).Width(width).Shrink(0).MinHeight(0).Radius(14).Clip().
		Focusable().Bind(a.paneRef("side-card"))
	a.side.focused = card.FocusWithin()
	if a.set.FocusFollowsMouse && a.pointerEntered(c, "side-card", card, card.Hovered()) && !a.side.focused {
		a.paneRef("side-card").Focus()
	}
	// While the user is elsewhere, the viewer fades as a terminal they are
	// not in does, as they chose (settings' DimExplorer). It shows code as
	// the editor panes do, on the files' theme's background; the window's
	// palette draws its shadow still.
	dim := a.set.DimExplorer && !a.side.focused
	wpal := pal
	pal = cardPalette(pal, a.fileTheme(c.Theme().Dark))
	paned(card.Background(card.AnimateColor("bg", cardBackground(wpal, pal, dim), ui.Animation{Duration: 140 * time.Millisecond})), wpal)
	a.side.drafting, a.side.editing = nil, false
	card.Children(func() {
		switch {
		case v.kind == sideFiles && v.file != nil:
			a.fileViewer(c, pal, s, v.file)
		case v.kind == sideReview:
			a.prViewer(c, pal, s, width)
		case v.kind == sideHistory:
			a.historyViewer(c, pal, s, width)
		default:
			a.changesViewer(c, pal, s, width)
		}
		if a.set.DimExplorer {
			veil := ui.Box(c.Key("veil")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(pal.veil)
			veil.Opacity(veil.Animate("on", b2f(dim), 160*time.Millisecond))
		}
	})
	if card.Shortcut(0, ui.KeyEscape) && !a.cancelDraft() {
		a.focusTerminal()
	}
	if card.Shortcut(ui.Cmd, ui.KeyS) {
		a.saveFocused(v)
	}
}

// viewerHeader is the viewer's header: an icon and a title, as a pane's
// header shows its program, what follows it, then the viewer's buttons,
// the one that zooms it over the panes, and its close button.
func (a *App) viewerHeader(c *ui.Context, pal *palette, s *proto.Session, svg *ui.SVG, title string, dirty bool, after, buttons func()) {
	left := a.sideLeft()
	pad := float32(16)
	if left {
		pad = a.chrome.headerPad
	}
	ui.Row(c.Key("hdr")).Height(headerH).Shrink(0).AlignItems(ui.Center).Padding(0, 10, 0, pad).Gap(8).Children(func() {
		if left {
			a.sidebarButtons(c, pal, c.TitleBar(), a.chrome.shown)
		}
		ui.Icon(c, svg).Size(14, 14).TextColor(pal.textFaint).Shrink(0)
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		if after != nil {
			after()
		}
		ui.Box(c.Key("drag")).Grow(1).AlignSelf(ui.Stretch).DragWindow().Bind(a.paneRef("side-gap"))
		if buttons != nil {
			buttons()
		}
		if a.side.wide {
			iconButton(c, "wide", unzoomIcon, locales.PaneShowAll(), pal, func() { a.side.wide = false })
		} else {
			iconButton(c, "wide", zoomIcon, locales.SideZoom(), pal, func() {
				a.side.wide = true
				// The panes it hides can't keep the keyboard.
				a.paneRef("side-card").Focus()
			})
		}
		sid := s.ID
		closeButton(c, pal, locales.SideClose(), dirty, func() { a.defer_(func() { a.closeView(a.session(sid)) }) })
		if !left || a.side.wide {
			a.cornerButtons(c, pal, "-v")
			a.controlsRoom(c, 8)
		}
	})
}

// viewerFolder shows the folder the viewer is of after its title, as the
// user chose (settings' ExplorerFolder): its path, its name, or neither.
func (a *App) viewerFolder(c *ui.Context, pal *palette, s *proto.Session, dir string) {
	if dir == "" {
		return
	}
	h := a.hostOf(s.ID)
	if name := a.folderShown(c, a.set.ExplorerFolder, h.shortDir(dir), proto.DirName(dir), "side", 13, 8, 0); name != "" {
		ui.Text(c, name).FontSize(13).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0).Tooltip(dir).Bind(a.paneRef("side-dir"))
	}
}

// folderShown is what a header shows of a folder, as mode (settings'
// PaneFolder, ExplorerFolder) asks: its path, full; its name; or "" for
// neither. Fit asks for the most of them that fits the room the folder's
// text and the gap after it, bound to key+"-dir" and key+"-gap", took as
// last laid out, of which the folder takes gap more with its text shown,
// and less is taken as it comes and goes, as a header's controls.
func (a *App) folderShown(c *ui.Context, mode, full, name, key string, size, gap, less float32) string {
	switch mode {
	case "hidden":
		return ""
	case "folder":
		return name
	case "full":
		return full
	}
	text, after := a.paneRef(key+"-dir").Bounds(c), a.paneRef(key+"-gap").Bounds(c)
	if after.H == 0 && text.H == 0 {
		// Not laid out yet: the next frame knows the room.
		c.Invalidate()
		return ""
	}
	room := text.W + after.W - max(less, 0)
	if text.W == 0 {
		room -= gap
	}
	for _, s := range []string{full, name} {
		if w, _ := c.MeasureText(0, ui.Span{Text: s, Size: size}); w <= room {
			return s
		}
	}
	return ""
}

// showRightButton shows the right sidebar, at the right of the header at
// the window's top right while the sidebar hides, as the left sidebar's
// shows it.
func (a *App) showRightButton(c *ui.Context, pal *palette, key string) {
	if !a.rightShown() {
		svg := sidebarRightIcon
		if a.sideLeft() {
			svg = sidebarIcon
		}
		iconButton(c, "show-right"+key, svg, a.tip(locales.RightSidebarShow(), "rightSidebar.toggle"), pal, a.toggleRight)
	}
}

// --- the Changes tab ---

// changesPanes returns the panes the Changes tab is of: the one the user
// is in, whose worktree it shows, and the one of the session whose agent
// last started a turn, the user's first, nil for none.
func (a *App) changesPanes(s *proto.Session) (here, turn *proto.Pane) {
	here = withGit(s, s.PaneByID(a.focusOf(s)))
	if here == nil {
		return nil, nil
	}
	if here.Turn != nil {
		return here, here
	}
	for i := range s.Panes {
		p := &s.Panes[i]
		if p.Turn != nil && (turn == nil || p.Turn.At > turn.Turn.At) {
			turn = p
		}
	}
	return here, turn
}

// withGit is a pane with its worktree, as kerod last told it; for one it
// has yet to look at, as a pane just split from another, the worktree of
// another pane of the session that holds its directory, so that the
// sidebar keeps showing it rather than no repository for that moment.
func withGit(s *proto.Session, p *proto.Pane) *proto.Pane {
	if p == nil || p.Git != nil {
		return p
	}
	dir := p.WorkDir()
	if dir == "" {
		return p
	}
	for i := range s.Panes {
		if g := s.Panes[i].Git; g != nil && withinDir(dir, g.Root) {
			cp := *p
			cp.Git = g
			return &cp
		}
	}
	return p
}

// withinDir tells that dir is root or a directory in it, with the paths
// of any host.
func withinDir(dir, root string) bool {
	root = strings.TrimRight(root, `/\`)
	if dir == root {
		return true
	}
	return strings.HasPrefix(dir, root+"/") || strings.HasPrefix(dir, root+`\`)
}

// changesDir is the worktree the Changes tab lists: the agent's where it
// shows its turn, else the one of the pane the user is in; "" for none.
func (a *App) changesDir(s *proto.Session) string {
	here, turn := a.changesPanes(s)
	switch {
	case turn != nil && (a.side.changes.base == git.BaseTurn || here.Git == nil):
		return turn.Turn.Root
	case here != nil && here.Git != nil:
		return here.Git.Root
	}
	return ""
}

// changesScope names what the Changes tab lists for a pane since a base:
// its worktree's changes, or its turn's, which the list, its files'
// diffs, folds and scroll position are kept by.
func changesScope(p *proto.Pane, base string) string {
	if base == git.BaseTurn && p.Turn != nil {
		return p.Turn.Root + "|" + p.Turn.Tree + "|" + base
	}
	if p.Git != nil {
		return p.Git.Root + "|" + base
	}
	return p.ID + "|" + base
}

// gitStamp is what of a worktree's status changes its changes.
func gitStamp(g *proto.GitInfo) string {
	if g == nil {
		return ""
	}
	return fmt.Sprint(g.Root, g.Head, g.Branch, g.Files, g.Added, g.Removed, g.Staged, g.State)
}

// sessionWorking tells that an agent of the session works.
func sessionWorking(s *proto.Session) bool {
	for _, p := range s.Panes {
		if p.Status == proto.Working {
			return true
		}
	}
	return false
}

// changesFrame is the Changes tab for a frame: what it lists, made once
// for the sidebar's tree and the viewer's diffs both.
type changesFrame struct {
	title, detail       string // why it lists nothing: git is off, no repository
	bases, labels, tips []string
	here                *proto.Pane // the pane the user is in
	pane                *proto.Pane // whose worktree, or turn, it lists
	scope               string
	list                *proto.GitChanges // nil until one comes
	// ix is Uncommitted's list, in its sections, staged and not, as the
	// files the user moved show at once; nil for another base.
	ix   *proto.GitIndex
	held bool // list is another base's, faded for a moment
	// rows are the viewer's rows this frame, heads the row of each file's
	// header, by its key (fileKey), and ls their list: nil while the
	// viewer shows no diffs.
	rows  []changeRow
	heads map[string]int
	ls    *ui.ListState
}

// sections are the frame's lists of files: Uncommitted's, not staged then
// staged, each by its section; another base's one, of no section.
func (cf *changesFrame) sections() []changeSection {
	if cf.ix != nil {
		var out []changeSection
		for _, sec := range []changeSection{{baseUnstaged, cf.ix.Unstaged}, {baseStaged, cf.ix.Staged}} {
			if len(sec.files) > 0 {
				out = append(out, sec)
			}
		}
		return out
	}
	if cf.list == nil {
		return nil
	}
	return []changeSection{{"", cf.list.Files}}
}

// changeSection is a section of the Changes tab's list: its name, the
// base its diffs are of, and its files.
type changeSection struct {
	name  string
	files []proto.GitFile
}

// changesFrameOf makes the Changes tab's frame, once a frame: the bases
// the worktree offers and the list of the one chosen, fetched as the
// worktree changes.
func (a *App) changesFrameOf(c *ui.Context, s *proto.Session) *changesFrame {
	if a.side.cf != nil {
		return a.side.cf
	}
	cf := &changesFrame{}
	a.side.cf = cf
	ch := &a.side.changes
	here, turn := a.changesPanes(s)
	cf.here = here
	if !a.set.GitStatus {
		cf.title, cf.detail = locales.ChangesGitOff(), locales.ChangesGitOffDetail()
		return cf
	}
	if here == nil || here.Git == nil && turn == nil {
		cf.title, cf.detail = locales.ChangesNoRepo(), locales.ChangesNoRepoDetail()
		return cf
	}

	// The bases this worktree offers, and the one shown: the agent's turn
	// first, as the tab opens.
	if turn != nil {
		cf.bases, cf.labels, cf.tips = append(cf.bases, git.BaseTurn), append(cf.labels, locales.ChangesTurn()), append(cf.tips, locales.ChangesTurnTip())
	}
	if here.Git != nil {
		cf.bases, cf.labels, cf.tips = append(cf.bases, git.BaseHead), append(cf.labels, locales.ChangesUncommitted()), append(cf.tips, locales.ChangesUncommittedTip())
		if d := here.Git.Default; d != "" {
			cf.bases, cf.labels, cf.tips = append(cf.bases, git.BaseBranch), append(cf.labels, locales.ChangesBranch()), append(cf.tips, locales.ChangesBranchTip(d))
		}
	}
	if !slices.Contains(cf.bases, ch.base) {
		ch.base = cf.bases[0]
	}
	pane := here
	if ch.base == git.BaseTurn {
		pane = turn
	}
	// The list is of a worktree, not of a pane: the panes the user moves
	// between in one show it as it was, scrolled where it was. A turn's is
	// of its snapshot.
	scope := changesScope(pane, ch.base)
	// Kero's own operations change what the list shows, as a hunk staged,
	// which the status may not say.
	stamp := gitStamp(pane.Git) + "|" + itoa(a.side.git.done)
	if pane.Turn != nil && ch.base == git.BaseTurn {
		stamp += "|" + pane.Turn.Tree
	}
	working := sessionWorking(s)
	// The list follows the worktree: as its status changes, and every few
	// seconds while an agent works in the session, as a file can change
	// without its count of lines.
	stale := scope != ch.scope || stamp != ch.stamp || working && time.Since(ch.fetched) > 5*time.Second
	if stale && !ch.busy && a.hostOf(pane.ID).cl != nil {
		a.fetchChanges(scope, stamp, pane.ID, ch.base)
	}
	if working {
		c.After(5 * time.Second)
	}

	// The list of the base shown, else the one fetched last of it; one of
	// another base stays, faded, for the moment this one takes to come,
	// rather than blink away.
	if scope != ch.shown {
		ch.shown, ch.switched = scope, c.Now()
	}
	list, current := ch.res, ch.res != nil && ch.scope == scope
	if !current && ch.lists[scope] != nil {
		list, current = ch.lists[scope], true
	}
	held := !current && list != nil && c.Now().Sub(ch.switched) < 400*time.Millisecond
	if !current && !held {
		list = nil
	}
	if held {
		c.After(400 * time.Millisecond)
	}
	cf.pane, cf.scope, cf.list, cf.held = pane, scope, list, held
	if ch.base == git.BaseHead && !held {
		cf.ix = movedIndex(ch.indexes[scope], ch.moves, scope)
	}
	return cf
}

// changesNone shows why the Changes tab lists no files, a spinner while
// they come, and tells whether it showed one.
func (a *App) changesNone(c *ui.Context, pal *palette, cf *changesFrame) bool {
	ch := &a.side.changes
	res := cf.list
	switch {
	case cf.title != "":
		a.sideMessage(c, pal, cf.title, cf.detail)
	case res == nil:
		if ch.err != "" && !ch.busy && ch.scope == cf.scope {
			a.sideMessage(c, pal, locales.ChangesCantRead(), ch.err)
		} else if c.Now().Sub(ch.switched) > 250*time.Millisecond {
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		} else {
			c.After(260 * time.Millisecond)
			ui.Spacer(c)
		}
	case len(res.Files) == 0 && cf.held:
		// Another base's "no changes" would say the wrong thing.
		ui.Spacer(c)
	case len(res.Files) == 0:
		detail := map[string]string{git.BaseTurn: locales.ChangesNoneTurn(), git.BaseHead: locales.ChangesNoneUncommitted()}[a.side.changes.base]
		if a.side.changes.base == git.BaseBranch {
			detail = locales.ChangesNoneBranch(res.BaseName)
		}
		a.sideMessage(c, pal, locales.ChangesNone(), detail)
	default:
		return false
	}
	return true
}

// changesSide is the Changes tab: the worktree's branch, the bases to
// choose from, what changed since the one chosen, and the tree of the
// files, a click on one of which opens the diffs in the viewer, scrolled
// to it; under it the review's comments, and in Uncommitted, the commit
// box.
func (a *App) changesSide(c *ui.Context, pal *palette, s *proto.Session) {
	cf := a.changesFrameOf(c, s)
	ch := &a.side.changes
	if cf.title != "" {
		a.sideMessage(c, pal, cf.title, cf.detail)
		return
	}
	if cf.here.Git != nil {
		a.gitBar(c, pal, s, cf.here)
	}
	list := cf.list
	ui.Column(c.Key("bases")).Shrink(0).Padding(0, 12, 6, 12).Gap(6).AlignItems(ui.Stretch).Children(func() {
		if len(cf.bases) > 1 {
			segmentedIn(c, "base", &ch.base, cf.bases, cf.labels, true)
		}
		i := slices.Index(cf.bases, ch.base)
		ui.Row(c).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
			if list == nil || len(list.Files) == 0 {
				ui.Text(c, cf.tips[i]).FontSize(12.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(cf.tips[i])
				return
			}
			added, removed := 0, 0
			for _, f := range list.Files {
				added, removed = added+f.Added, removed+f.Removed
			}
			if ch.base == git.BaseHead && cf.pane.Git != nil {
				// Since HEAD, as the status counts it: a file both staged
				// and not is one change.
				added, removed = cf.pane.Git.Added, cf.pane.Git.Removed
			}
			ui.Text(c, locales.ChangesFiles(len(list.Files))).FontSize(12.5).TextColor(pal.textMuted).SingleLine().Shrink(0)
			a.lineCounts(c, pal, added, removed, 12.5)
			if ch.base == git.BaseBranch && list.BaseName != "" {
				ui.Text(c, locales.ChangesSince(list.BaseName)).FontSize(12.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0)
			}
			ui.Spacer(c)
			if v := a.viewOf(s); v == nil || v.kind != sideChanges {
				iconButton(c, "show-diffs", diffIcon, locales.ChangesShowDiffs(), pal, func() {
					a.openView(s, sideChanges, nil)
					a.focusViewer()
				})
			}
		})
	})
	if !a.changesNone(c, pal, cf) {
		box := ui.Column(c.Key("tree-" + cf.scope)).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
		box.Opacity(box.Animate("held", 1-0.45*b2f(cf.held), 120*time.Millisecond))
		box.Children(func() { a.changesTree(c, pal, s, cf) })
	}

	// The review's comments and Send to Agent, below what they are on and
	// above the commit box.
	root := ""
	switch {
	case ch.base == git.BaseTurn && cf.pane.Turn != nil:
		root = cf.pane.Turn.Root
	case cf.pane.Git != nil:
		root = cf.pane.Git.Root
	}
	if root != "" {
		a.reviewBar(c, pal, cf.pane, root)
	}
	if ch.base == git.BaseHead && cf.here.Git != nil && cf.ix != nil {
		a.commitBox(c, pal, cf.here, cf.ix)
	}
}

// changesViewer is the viewer of Changes: the diffs of the files the tab
// lists, one after the other, in Uncommitted under their sections'
// titles, scrolled to the one chosen in the tree.
func (a *App) changesViewer(c *ui.Context, pal *palette, s *proto.Session, width float32) {
	cf := a.changesFrameOf(c, s)
	ch := &a.side.changes
	secs := cf.sections()
	var keys []string
	var all []proto.GitFile
	for _, sec := range secs {
		for _, f := range sec.files {
			keys, all = append(keys, fileKey(cf.scope, sec.name, f.Path)), append(all, f)
		}
	}
	dirty := false
	for _, sf := range ch.files {
		dirty = dirty || a.diffTarget(sf).dirty()
	}
	a.viewerHeader(c, pal, s, diffIcon, locales.SideChanges(), dirty, func() {
		a.viewerFolder(c, pal, s, a.changesDir(s))
	}, func() {
		a.wrapButton(c, pal)
		if len(keys) > 0 {
			a.foldAllButton(c, pal, keys, all, func(fold bool) {
				// The file at the top stays there, as the list, built
				// after this header, shows its rows folded.
				rows, ls := ch.rows, ch.places[cf.scope]
				if ls == nil || !fold {
					return
				}
				if first, _ := ls.Visible(); first >= 0 && first < len(rows) && !rows[first].title {
					ch.reveal = fileKey(cf.scope, rows[first].section, rows[first].f.Path)
				}
			})
		}
		a.layoutButton(c, pal, width)
	})
	if a.changesNone(c, pal, cf) {
		return
	}
	res, scope := cf.list, cf.scope
	for _, sec := range secs {
		if !cf.held {
			base := sec.name
			if base == "" {
				base = ch.base
			}
			a.syncFiles(c, scope, sec.name, base, cf.pane, res.Root, sec.files)
		}
	}
	if ch.places == nil {
		ch.places = map[string]*ui.ListState{}
	}
	ls := ch.places[scope]
	if ls == nil {
		ls = &ui.ListState{}
		ch.places[scope] = ls
	}
	// A file is two rows: its header, then its diff while unfolded,
	// which scrolls under the header as the list pins it at the top,
	// until the next file's header pushes it out, as the files of a
	// pull request do. Uncommitted's sections have their titles above
	// their files.
	var rows []changeRow
	heads := map[string]int{}
	for _, sec := range secs {
		if sec.name != "" {
			rows = append(rows, changeRow{section: sec.name, title: true})
		}
		for _, f := range sec.files {
			key := fileKey(scope, sec.name, f.Path)
			heads[key] = len(rows)
			rows = append(rows, changeRow{f: f, section: sec.name})
			if ch.files[key] != nil && !a.fileCollapsed(key, f) {
				rows = append(rows, changeRow{f: f, section: sec.name, diff: true})
			}
		}
	}
	cf.rows, cf.heads, cf.ls = rows, heads, ls
	ch.rows = rows
	// The file chosen in the tree, as the viewer opens on it.
	if ch.reveal != "" {
		if i, ok := heads[ch.reveal]; ok {
			ls.ScrollTo(i, ui.Start)
		}
		ch.reveal = ""
	}
	ls.Header = func(i int) bool { return !rows[i].diff && !rows[i].title }
	ls.Key = func(i int) any {
		r := rows[i]
		return changeKey{path: r.f.Path, section: r.section, diff: r.diff, title: r.title}
	}
	box := ui.Row(c.Key("files-" + scope)).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	box.Opacity(box.Animate("held", 1-0.45*b2f(cf.held), 120*time.Millisecond))
	box.Children(func() {
		// The scroll bar runs below the header pinned at the top, not
		// under it.
		ui.List(c.Key("rows"), ls, len(rows), func(i int) {
			r := rows[i]
			key := fileKey(scope, r.section, r.f.Path)
			// A section's files and diffs have its color down their left
			// edge, as its title has, so that the two sections are told
			// apart as they scroll.
			stripe := func(e ui.Element) ui.Element {
				if r.section == "" {
					return e
				}
				col := a.sectionColor(c, pal, r.section).Alpha(0.55)
				return e.DrawOver(func(p *ui.Painter, rr ui.Rect) { p.Fill(ui.Rect{X: rr.X, Y: rr.Y, W: 3, H: rr.H}, col, 0) })
			}
			switch {
			case r.title:
				a.sectionTitle(c, pal, r.section, i > 0)
				return
			case r.diff:
				stripe(ui.Column(c).FillWidth().AlignItems(ui.Stretch)).Children(func() { a.fileDiff(c, pal, s, ch.files[key], width) })
				return
			}
			// Opaque, over the diff scrolling under it; a folded file's
			// with the room below its section.
			gap := float32(sectionGap)
			if i+1 < len(rows) && rows[i+1].diff {
				gap = 0
			}
			stripe(ui.Column(c).FillWidth().AlignItems(ui.Stretch).Padding(0, 0, gap, 0).Background(pal.card)).Children(func() {
				var tools func()
				if r.section != "" {
					f := r.f
					tools = func() { a.indexTools(c, pal, cf.here, cf.ix, r.section, f) }
				}
				a.fileHeader(c, pal, key, cf.pane.ID, res.Root, r.f, flatList, stickyH, func() {
					// Folding it as the list pins it brings it back where
					// it stays: at the top.
					ls.ScrollIntoView(i)
				}, tools, a.ignoreItems(cf, r.section, r.f.Path, false))
			})
		}).Grow(1).MinWidth(0).MinHeight(0).Padding(0, 0, 16, 0).ScrollbarInsets(stickyH, 0, 0, 0)
	})
}

// layoutButton switches the diffs of a viewer width wide between side by
// side and one column.
func (a *App) layoutButton(c *ui.Context, pal *palette, width float32) {
	split := a.diffSplit(width)
	layoutIcon, layoutTip := splitRightIcon, locales.ChangesSplit()
	if split {
		layoutIcon, layoutTip = splitDownIcon, locales.ChangesUnified()
	}
	iconButton(c, "layout", layoutIcon, layoutTip, pal, func() {
		set := a.set
		set.DiffLayout = "split"
		if split {
			set.DiffLayout = "unified"
		}
		a.applySettings(set)
	})
}

// ignoreItems is the menu items of a file or folder of Uncommitted that
// add it to the worktree's .gitignore; nil for another base's.
func (a *App) ignoreItems(cf *changesFrame, section, p string, dir bool) func(*ui.Menu) {
	if section == "" || cf.here == nil || cf.here.Git == nil {
		return nil
	}
	here := cf.here
	return func(m *ui.Menu) {
		label := locales.GitIgnore()
		if dir {
			label, p = locales.GitIgnoreFolder(), p+"/"
		}
		if m.Item(label).Chosen() {
			a.ignore(here, p)
		}
	}
}

// sectionTitle is the title of a section of Uncommitted in the viewer,
// in its color, apart from the section above it.
func (a *App) sectionTitle(c *ui.Context, pal *palette, section string, apart bool) {
	title, icon := locales.GitChanges(), diffIcon
	if section == baseStaged {
		title, icon = locales.GitStagedChanges(), checkIcon
	}
	col := a.sectionColor(c, pal, section)
	top := float32(0)
	if apart {
		top = 10
	}
	band := ui.Row(c.Key("title")).Height(28).Margin(top, 0, 4, 0).Padding(0, 6, 0, 12).Gap(7).AlignItems(ui.Center).Background(col.Alpha(0.1))
	band.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(ui.Rect{X: r.X, Y: r.Y, W: 3, H: r.H}, col, 0) })
	band.Children(func() {
		ui.Icon(c, icon).Size(13, 13).TextColor(col).Shrink(0)
		ui.Text(c, title).FontSize(12.5).FontWeight(650).TextColor(pal.text).SingleLine().Shrink(0)
	})
}

// changeRow is a row of the viewer's list of changes: a file's header, or
// its diff, of a section of Uncommitted's, or the section's title;
// changeKey what it is, as the list keeps a row's state by.
type changeRow struct {
	f       proto.GitFile
	section string
	diff    bool
	title   bool
}

type changeKey struct {
	path, section string
	diff, title   bool
}

// stickyH is the height of a file's header, which the list pins at its
// top; sectionGap the room below each file's section.
const (
	stickyH    = 32
	sectionGap = 6
)

// flatList is the depth of a file's header in a list, not a tree, which
// shows its folder beside its name; treeIndent the room each of a tree's
// levels takes.
const (
	flatList   = -1
	treeIndent = 14
)

// syncFiles keeps the diffs of a list's files, of a section: it asks for
// the text of each file shown that has none yet, a few at a time, and
// again of one whose change the list counts anew (Known, so that kerod
// sends only what changed), or of every one as the worktree's HEAD moves;
// the new side not of one the user edited (showText).
func (a *App) syncFiles(c *ui.Context, scope, section, base string, pane *proto.Pane, root string, files []proto.GitFile) {
	ch := &a.side.changes
	if ch.files == nil || len(ch.files) > 400 {
		// Keeping those with edits to save.
		kept := map[string]*sideFile{}
		for k, sf := range ch.files {
			if a.diffTarget(sf).dirty() {
				kept[k] = sf
			}
		}
		ch.files = kept
	}
	stamp := gitStamp(pane.Git)
	asking := 0
	for _, sf := range ch.files {
		if sf.busy {
			asking++
		}
	}
	for _, f := range files {
		key := fileKey(scope, section, f.Path)
		sf := ch.files[key]
		if sf == nil {
			sf = &sideFile{pane: pane.ID, base: base, f: f}
			ch.files[key] = sf
		}
		sf.root = root
		if a.fileCollapsed(key, f) || sf.busy || sf.writing || a.hostOf(pane.ID).cl == nil {
			continue
		}
		known := sf.known()
		switch {
		case sf.d == nil && !sf.text.Binary && !sf.text.Large && sf.err == "":
			known = "" // never shown
		case sf.stamp != stamp && sf.base != git.BaseTurn:
			known = "" // HEAD moved: the base's text too
		case sf.f != f:
		default:
			continue
		}
		if asking >= 4 {
			c.After(50 * time.Millisecond)
			return
		}
		asking++
		sf.f, sf.stamp = f, stamp
		a.fetchFile(sf, known)
	}
}

// fileCollapsed tells whether a file's diff, by its key (fileKey), is
// folded: as the user left it, else folded for a binary file or one of
// very many lines changed; and a commit's or a pull request's, which list
// their files to unfold one at a time (histKey, prFileKey, the form's).
func (a *App) fileCollapsed(key string, f proto.GitFile) bool {
	if v, ok := a.side.changes.collapsed[key]; ok {
		return v
	}
	for _, list := range []string{"prv|", "pr|"} {
		if strings.HasPrefix(key, list) {
			return true
		}
	}
	return f.Binary || f.Added+f.Removed > 600
}

// wrapButton wraps the viewer's long lines, of diffs and files, or stops
// wrapping them, as wrap-lines keeps.
func (a *App) wrapButton(c *ui.Context, pal *palette) {
	svg, tip := wrapIcon, locales.ViewerWrap()
	if a.set.WrapLines {
		svg, tip = unwrapIcon, locales.ViewerNoWrap()
	}
	iconButton(c, "wrap", svg, tip, pal, func() {
		set := a.set
		set.WrapLines = !set.WrapLines
		a.applySettings(set)
	})
}

// foldAllButton folds every file's diff of keys (fileKey), the files
// files, while any shows, else unfolds them all, running first then, told
// which.
func (a *App) foldAllButton(c *ui.Context, pal *palette, keys []string, files []proto.GitFile, then func(fold bool)) {
	ch := &a.side.changes
	fold := false
	for i, k := range keys {
		fold = fold || !a.fileCollapsed(k, files[i])
	}
	svg, label := collapseIcon, locales.ChangesCollapseFiles()
	if !fold {
		svg, label = expandIcon, locales.ChangesExpandFiles()
	}
	iconButton(c, "fold-all", svg, label, pal, func() {
		if then != nil {
			then(fold)
		}
		if ch.collapsed == nil {
			ch.collapsed = map[string]bool{}
		}
		for _, k := range keys {
			ch.collapsed[k] = fold
		}
	})
}

// fileDiff is a changed file's diff, under its header, or why it shows
// none.
func (a *App) fileDiff(c *ui.Context, pal *palette, s *proto.Session, sf *sideFile, width float32) {
	ch := &a.side.changes
	ui.Column(c).FillWidth().AlignItems(ui.Stretch).Padding(4, 0, sectionGap, 0).Children(func() {
		switch {
		case sf.err != "" && sf.d == nil:
			a.sectionNote(c, pal, locales.ChangesCantRead()+": "+sf.err)
		case sf.text.Binary:
			a.sectionNote(c, pal, locales.ChangesBinary())
		case sf.text.Large && sf.d == nil:
			a.sectionNote(c, pal, locales.ChangesTooLarge())
		case sf.d == nil:
			ui.Column(c.Key("loading")).Height(48).Center().Children(func() { ui.Spinner(c) })
		default:
			a.diffOptions(c, pal, s, sf, width)
			diff := editor.DiffView(c.Key("diff"), sf.d)
			if diff.FocusWithin() {
				ch.focus, a.side.editing = sf, true
			}
		}
	})
}

// fileHeader is a changed file's header, by its key (fileKey): its
// change, its name and, in a list (depth flatList), its folder, a dot
// while its new side has edits to save, and the lines it adds and
// removes, which a click folds or unfolds, running folding first when
// set; in a tree, indented by its depth. tools, when set, draws its
// buttons at its end. Its context menu opens the file, of pane's host in
// its worktree's root, and drags it onto a pane, unless it was deleted,
// or root is ""; then come menu's items, when set. height is the row's.
func (a *App) fileHeader(c *ui.Context, pal *palette, key, pane, root string, f proto.GitFile, depth int, height float32, folding, tools func(), menu func(*ui.Menu)) {
	ch := &a.side.changes
	collapsed := a.fileCollapsed(key, f)
	// Across the viewer, edge to edge.
	hdr := ui.Row(c.Key("hdr")).Height(height).Padding(0, 10, 0, 12+treeIndent*float32(max(depth, 0))).Gap(8).AlignItems(ui.Center).Background(pal.chip)
	if hdr.Hovered() {
		hdr.Background(pal.rowHover.Mix(pal.chip, 0.5))
	}
	// Without a root, the file is on no disk: of a pull request no worktree
	// has checked out.
	h := a.hostOf(pane)
	full, gone := joinRel(root, f.Path), f.Status == "deleted" || root == ""
	if !gone || menu != nil {
		hdr.ContextMenu(func(m *ui.Menu) {
			if !gone {
				a.openItems(m, pane, full, 0)
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
		hdr.Drag(fileDrag{pane: pane, path: full})
		if hdr.Dragging() {
			hdr.Opacity(0.4)
		}
	}
	if hdr.Clicked() {
		if folding != nil {
			folding()
		}
		if ch.collapsed == nil {
			ch.collapsed = map[string]bool{}
		}
		ch.collapsed[key] = !collapsed
	}
	hdr.Children(func() {
		chev := ui.Icon(c, chevronIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		chev.Rotate(chev.AnimateTo("turn", 90*(1-b2f(collapsed)), chevronTurn))
		a.statusBadge(c, pal, f.Status)
		dir, name := path.Split(f.Path)
		if depth != flatList {
			dir = "" // the tree's folders show it
		}
		fg := pal.text
		if f.Status == "deleted" {
			fg = pal.textMuted
		}
		// The path on the name only: across the row, it hid what the row
		// already shows.
		tip := f.Path
		if f.OldPath != "" {
			tip = f.OldPath + " → " + f.Path
		}
		fileName(c, name, fg).Tooltip(tip)
		if sf := ch.files[key]; sf != nil && a.diffTarget(sf).dirty() {
			ui.Box(c).Size(7, 7).Radius(3.5).Shrink(0).Background(pal.textMuted).Description(locales.ViewerUnsaved())
		}
		// The folder gives way first: the name is what the row is for.
		ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Grow(1).Shrink(1000).MinWidth(0)
		if f.Binary {
			ui.Text(c, locales.ChangesBinary()).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0)
		} else {
			a.lineCounts(c, pal, f.Added, f.Removed, 12)
		}
		if tools != nil {
			tools()
		}
		if !gone && !h.remote() {
			iconButton(c, "open", openFileIcon, locales.ChangesOpenFile(), pal, func() { openPath(full) })
		}
	})
}

// fileName is a file's name in a row, whole while the row has room, else
// cut before its extension, which stays: "report_FI….pdf", not
// "report_FI…".
func fileName(c *ui.Context, name string, fg ui.Color) ui.Element {
	stem, ext := name, path.Ext(name)
	if ext == name || len(ext) > 8 {
		ext = ""
	}
	stem = strings.TrimSuffix(name, ext)
	// One name to read, as a screen reader and a search for it read it.
	return ui.Row(c.Key("name")).AlignItems(ui.Center).Shrink(1).MinWidth(40).Label(name).Children(func() {
		// Cut, its ellipsis against the extension, not the room a
		// glyph too wide to fit leaves.
		ui.Text(c, stem).FontSize(13).FontWeight(600).TextColor(fg).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).TextAlign(ui.End)
		if ext != "" {
			ui.Text(c, ext).FontSize(13).FontWeight(600).TextColor(fg).SingleLine().Shrink(0)
		}
	})
}

// sectionNote says why a file shows no diff.
func (a *App) sectionNote(c *ui.Context, pal *palette, text string) {
	ui.Text(c, text).FontSize(12.5).TextColor(pal.textMuted).Padding(10, 12).LineHeight(1.4)
}

// fetchChanges asks kerod for the list of a pane's changes since a base:
// for Uncommitted, the index's, staged and not.
func (a *App) fetchChanges(scope, stamp, pane, base string) {
	ch := &a.side.changes
	ch.busy, ch.since = true, time.Now()
	if ch.scope != scope {
		ch.err = ""
	}
	cl := a.hostOf(pane).cl
	if cl == nil {
		ch.busy = false
		return
	}
	asked := time.Now()
	go func() {
		var res proto.GitChanges
		var ix *proto.GitIndex
		var err error
		if base == git.BaseHead {
			// Uncommitted: what is staged and what isn't, the files of both
			// its list.
			ix = &proto.GitIndex{}
			if err = cl.Call("git.index", proto.GitParams{Pane: pane}, ix); err == nil {
				res = proto.GitChanges{Root: ix.Root, BaseName: "HEAD", Files: unionFiles(ix.Staged, ix.Unstaged)}
			}
		} else {
			err = cl.Call("git.changes", proto.GitParams{Pane: pane, Base: base}, &res)
		}
		a.update(func() {
			ch.busy, ch.fetched = false, time.Now()
			ch.scope, ch.stamp = scope, stamp
			// A move shows in an index asked for after its operation ended;
			// one whose end never came goes after a while.
			ch.moves = slices.DeleteFunc(ch.moves, func(m *indexMove) bool {
				return m.scope == scope && !m.done.IsZero() && m.done.Before(asked) || time.Since(m.started) > time.Minute
			})
			if err != nil {
				ch.res, ch.err = nil, err.Error()
				delete(ch.lists, scope)
				delete(ch.indexes, scope)
				return
			}
			ch.res, ch.err = &res, ""
			if ch.lists == nil || len(ch.lists) > 12 {
				ch.lists, ch.indexes = map[string]*proto.GitChanges{}, map[string]*proto.GitIndex{}
			}
			ch.lists[scope] = &res
			if ix != nil {
				if ch.indexes == nil {
					ch.indexes = map[string]*proto.GitIndex{}
				}
				ch.indexes[scope] = ix
			}
		})
	}()
}

// unionFiles are the files of two lists, each once, as the first has it.
func unionFiles(a, b []proto.GitFile) []proto.GitFile {
	out := slices.Clone(a)
	seen := make(map[string]bool, len(a))
	for _, f := range a {
		seen[f.Path] = true
	}
	for _, f := range b {
		if !seen[f.Path] {
			out = append(out, f)
		}
	}
	return out
}

// statusBadge is a file's change as a letter in its color (statusLetter).
func (a *App) statusBadge(c *ui.Context, pal *palette, status string) {
	letter, col := a.statusLetter(c, pal, status)
	ui.Box(c).Size(18, 18).Radius(5).Shrink(0).Center().Background(col.Alpha(0.14)).Children(func() {
		ui.Text(c, letter).FontSize(11).FontWeight(650).TextColor(col).SingleLine()
	})
}

// statusLetter is a file's change as a letter, and its color: A added, M
// modified, D deleted, R renamed, T a change of type, U untracked, and !
// a conflict.
func (a *App) statusLetter(c *ui.Context, pal *palette, status string) (string, ui.Color) {
	added, removed, modified := a.diffColors(c, pal)
	switch status {
	case "added":
		return "A", added
	case "deleted":
		return "D", removed
	case "renamed":
		return "R", modified
	case "type":
		return "T", modified
	case "untracked":
		return "U", added
	case "conflict":
		return "!", removed
	}
	return "M", modified
}

// lineCounts shows lines added and removed, as +12 −3, in the diffs'
// colors; nothing for none.
func (a *App) lineCounts(c *ui.Context, pal *palette, added, removed int, size float32) {
	if added == 0 && removed == 0 {
		return
	}
	plus, minus, _ := a.diffColors(c, pal)
	ui.Row(c).Gap(5).Shrink(0).Children(func() {
		if added > 0 {
			ui.Text(c, "+"+itoa(added)).FontSize(size).FontFeatures("tnum").TextColor(plus).SingleLine()
		}
		if removed > 0 {
			ui.Text(c, "−"+itoa(removed)).FontSize(size).FontFeatures("tnum").TextColor(minus).SingleLine()
		}
	})
}

// diffColors are the colors of lines added, removed and modified: the
// terminals' green, red and yellow, so that a diff reads as one printed in
// the pane beside it.
func (a *App) diffColors(c *ui.Context, pal *palette) (added, removed, modified ui.Color) {
	t := a.codeOf(c, pal)
	return t.Palette[2], t.Palette[1], t.Palette[3]
}

// codeTheme is the colors of code, in diffs and editors, on what pal
// colors (editorTheme, codeOf).
func (a *App) codeTheme(c *ui.Context, pal *palette) *editor.Theme {
	th := a.codeOf(c, pal)
	if a.side.theme == nil || a.side.themeOf != *th {
		a.side.theme, a.side.themeOf = editorTheme(c.Theme(), th), *th
	}
	return a.side.theme
}

// sideMessage says why the view shows nothing.
func (a *App) sideMessage(c *ui.Context, pal *palette, title, detail string) {
	ui.Column(c).Grow(1).Center().Gap(6).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(pal.text).TextAlign(ui.Center)
		if detail != "" {
			ui.Text(c, detail).FontSize(12.5).TextColor(pal.textMuted).MaxWidth(300).TextAlign(ui.Center).LineHeight(1.4)
		}
	})
}

// --- a file's diff ---

// fetchFile asks kerod for a file's text in the base and now; known is
// the hash of the text shown, which kerod then sends again only when it
// changed.
func (a *App) fetchFile(sf *sideFile, known string) {
	cl := a.hostOf(sf.pane).cl
	if cl == nil {
		return
	}
	sf.busy = true
	go func() {
		var t proto.GitText
		err := cl.Call("git.file", proto.GitParams{Pane: sf.pane, Base: sf.base, Path: sf.f.Path, OldPath: sf.f.OldPath, Known: known, From: sf.f.From, To: sf.f.To}, &t)
		a.update(func() {
			sf.busy = false
			if err != nil {
				sf.err = err.Error()
				return
			}
			sf.err = ""
			if !t.Same {
				a.showText(sf, t)
			}
		})
	}()
}

// showText puts a file's text in its diff: anew for its first, else as
// edits, so that the diff keeps where it was scrolled; the new side not
// while the user has edits of it to write.
func (a *App) showText(sf *sideFile, t proto.GitText) {
	sf.text = proto.GitText{Binary: t.Binary, Large: t.Large, Mixed: t.Mixed}
	sf.index = t.Index
	if sf.d != nil && !sf.patch && !t.Binary && !t.Large && (sf.d.New.Modified() || sf.edits.saving) {
		replaceText(sf.d.Old, t.Old)
		return
	}
	sf.hash = t.Hash
	switch {
	case t.Binary || t.Large && t.Patch == "":
		sf.d, sf.patch = nil, false
	case t.Large:
		sf.d, sf.patch = &editor.DiffState{}, true
		if files, err := editor.ParsePatch(t.Patch); err == nil && len(files) > 0 {
			sf.d.SetPatch(files[0])
		}
	case sf.d == nil || sf.patch:
		lang := editor.LanguageFor(path.Base(sf.f.Path))
		old, now := editor.NewBuffer(t.Old), editor.NewBuffer(t.New)
		old.SetLanguage(lang)
		now.SetLanguage(lang)
		sf.d, sf.patch = &editor.DiffState{Old: old, New: now}, false
		if sf.onDisk() {
			h := a.hostOf(sf.pane)
			a.restoreEdit(sideChanges, h, h.joinPath(sf.root, sf.f.Path), now, &sf.hash)
		}
	default:
		replaceText(sf.d.Old, t.Old)
		replaceText(sf.d.New, t.New)
		sf.d.New.MarkSaved()
	}
}

// replaceText makes a buffer's text text by one edit of what differs
// between them, on whole characters. The buffer holds its lines ending in
// \n, and the ending the text has besides.
func replaceText(b *editor.Buffer, text string) {
	if strings.Contains(text, "\r\n") {
		b.SetLineEnding("\r\n")
		text = strings.ReplaceAll(text, "\r\n", "\n")
	} else {
		b.SetLineEnding("\n")
	}
	cur := b.String()
	if cur == text {
		return
	}
	p := 0
	for p < len(cur) && p < len(text) && cur[p] == text[p] {
		p++
	}
	for p > 0 && p < len(cur) && !utf8.RuneStart(cur[p]) {
		p--
	}
	q := 0
	for q < len(cur)-p && q < len(text)-p && cur[len(cur)-1-q] == text[len(text)-1-q] {
		q++
	}
	for q > 0 && !utf8.RuneStart(cur[len(cur)-q]) {
		q--
	}
	b.Edit(editor.Edit{Start: p, End: len(cur) - q, Text: text[p : len(text)-q]})
}

// diffSplit tells whether diffs show side by side in a viewer this wide.
func (a *App) diffSplit(width float32) bool {
	switch a.set.DiffLayout {
	case "split":
		return true
	case "unified":
		return false
	}
	return width >= splitWidth
}

// diffOptions sets a file's diff's looks and actions for this frame: its
// new side edits while it is the file on disk (edits.go); in a turn,
// Revert and Keep on each change, as the user reviews what the agent did;
// in Uncommitted, Stage and Discard on a change not staged, and Unstage
// on one staged; in Branch, Revert; and Comment wherever the diff takes
// comments.
func (a *App) diffOptions(c *ui.Context, pal *palette, s *proto.Session, sf *sideFile, width float32) {
	d := sf.d
	d.FitContent = true
	d.Wrap = a.set.WrapLines
	d.Font = ui.Font{Family: a.termFamily(), Size: max(a.set.FontSize-1, 10)}
	d.Theme = a.codeTheme(c, pal)
	d.Layout = editor.DiffUnified
	if a.diffSplit(width) {
		d.Layout = editor.DiffSplit
	}
	d.RenderSeparator = func(c *ui.Context, sep ui.Element, s *editor.DiffSeparator) { a.separator(c, pal, sep, s) }
	// The new side, the file now, edits: never of a patch, nor of a text
	// whose lines end two ways, which the buffer would write back as one.
	d.Editable = sf.onDisk() && !sf.patch && !sf.writing && !sf.text.Mixed && sf.f.Status != "deleted"
	// A change is edited in its buffer, then written whole; nor of a file
	// the index has in conflict. Uncommitted's stage and unstage the lines
	// selected by their numbers too, from the bar of a change they are of
	// (lineActions). Branch's Revert puts the branch's base's lines back in
	// the file, a change not committed yet. None while the new side has
	// edits not saved, which a change written would take or lose.
	editable := !sf.patch && !sf.writing && !sf.text.Mixed && !d.New.Modified()
	switch sf.base {
	case git.BaseTurn:
	case baseStaged, baseUnstaged, git.BaseBranch:
		editable = editable && sf.f.Status != "conflict" && sf.pr == nil
	default:
		editable = false
	}
	// Lines are selected to stage them, and to comment on them, in any
	// diff that takes comments (reviewable).
	review := reviewable(sf)
	d.LineSelection = review && !sf.writing
	if !d.LineSelection && len(d.SelectedRanges()) > 0 {
		d.SetSelectedRanges(nil)
	}
	a.reviewOptions(c, pal, sf, width)
	d.HunkActions = editable || review
	d.RenderHunkActions = func(c *ui.Context, bar ui.Element, h *editor.DiffHunkActions) {
		floating(bar.Gap(2).Padding(3).Radius(8).Background(pal.popover).AlignItems(ui.Center), pal)
		bar.Children(func() {
			if review && hunkButton(c, "comment", commentIcon, locales.ReviewComment(), locales.ReviewCommentTip(), pal) {
				if side, start, end, ok := hunkLines(d.SelectedRanges(), h.Hunk); ok {
					d.SetSelectedRanges(nil)
					a.startComment(sf, side, start, end)
				}
			}
			if !editable {
				return
			}
			if (sf.base == baseStaged || sf.base == baseUnstaged) && a.lineActions(c, pal, sf.base, sf, h.Hunk) {
				return
			}
			switch sf.base {
			case baseStaged:
				// Unstaged: the index's text, the new side, without it.
				// The text is of the lines, not of the buffer after Revert or
				// Keep, which keeps an empty text's empty line as one.
				if hunkButton(c, "unstage", minusIcon, locales.GitUnstage(), locales.GitUnstageChangeTip(), pal) {
					text := applyLines(d.Old, d.New, d.Hunks(), not(inHunk(h.Hunk)), d.New.LineEnding())
					h.Revert()
					a.writeIndex(sf, text)
				}
				return
			case baseUnstaged:
				if hunkButton(c, "revert", revertIcon, locales.GitDiscard(), locales.GitDiscardChangeTip(), pal) {
					a.revertChange(sf, h)
				}
				// Staged: the index's text, the old side, with it.
				if hunkButton(c, "stage", plusIcon, locales.GitStage(), locales.GitStageChangeTip(), pal) {
					text := applyLines(d.Old, d.New, d.Hunks(), inHunk(h.Hunk), indexEnding(sf))
					h.Keep()
					a.writeIndex(sf, text)
				}
				return
			}
			if hunkButton(c, "revert", revertIcon, locales.ChangesRevert(), locales.ChangesRevertTip(), pal) {
				a.revertChange(sf, h)
			}
			// What a branch changed is committed: there is nothing to mark
			// as reviewed.
			if sf.base != git.BaseBranch && hunkButton(c, "keep", checkIcon, locales.ChangesKeep(), locales.ChangesKeepTip(), pal) {
				h.Keep()
			}
		})
	}
}

// revertChange puts a change's old lines back in its file: at once while
// no agent works in the worktree, else as the user says, so long as the
// change is still the one they clicked.
func (a *App) revertChange(sf *sideFile, h *editor.DiffHunkActions) {
	g := a.paneGit(sf.pane)
	if g == nil {
		return
	}
	d, i, hunk, hash := sf.d, h.Index, h.Hunk, sf.hash
	anyway := len(a.agentsAt(sf.pane, g.Root)) > 0
	a.guardedNow(sf.pane, g.Root, locales.ChangesRevert(), func() {
		if sf.d != d || sf.hash != hash || i >= len(d.Hunks()) || d.Hunks()[i] != hunk {
			a.toast(locales.GitChangeMoved())
			return
		}
		text := applyLines(d.Old, d.New, d.Hunks(), not(inHunk(hunk)), d.New.LineEnding())
		d.Revert(i)
		a.writeBack(sf, text, anyway)
	})
}

// writeIndex makes text the index's content of a file, as staging or
// unstaging a change of it made it, through kerod, which writes it only
// as the index was shown.
func (a *App) writeIndex(sf *sideFile, text string) {
	cl := a.hostOf(sf.pane).cl
	if cl == nil {
		return
	}
	sf.writing = true
	var root string
	if g := a.paneGit(sf.pane); g != nil {
		root = g.Root
	}
	p := proto.GitOpParams{Pane: sf.pane, Op: "setIndex", Root: root, Path: sf.f.Path, Known: sf.index, Text: text}
	go func() {
		var res proto.GitOpResult
		err := cl.Call("git.op", p, &res)
		a.update(func() {
			sf.writing = false
			a.side.git.done++
			if err == nil && res.Error != "" {
				err = errors.New(res.Error)
			}
			if err != nil {
				a.toast(locales.GitStageFailed(err.Error()))
				a.fetchFile(sf, "")
			}
		})
	}()
}

// separator is a diff's unchanged lines collapsed between its changes:
// chevrons that show more of them, up or down, and "N unmodified lines",
// which shows them all, on the diff's own background, with no bar; or a
// patch's hunk's header. Shift shows them all from any of them.
func (a *App) separator(c *ui.Context, pal *palette, sep ui.Element, s *editor.DiffSeparator) {
	sep.Children(func() {
		// Its chevron under the file's header's: the header's starts 12 in,
		// 10 wide, and the button's icon, 11 wide, sits in 24.
		ui.Row(c).Grow(1).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(2).Padding(0, 12, 0, 5).Children(func() {
			if s.Lines == 0 {
				ui.Text(c, s.Header).Font(s.Font().Family).FontSize(s.Font().Size-1).TextColor(pal.textFaint).SingleLine().Shrink(1).Padding(0, 7)
				return
			}
			expand := func(fn func()) func() {
				return func() {
					if c.Modifiers()&ui.Shift != 0 {
						fn = s.ExpandAll
					}
					fn()
				}
			}
			chevron := func(key string, svg *ui.SVG, label string, fn func()) {
				b := ui.ButtonBase(c.Key(key)).Size(24, 24).Radius(6).Center().TextColor(pal.textMuted).Label(label).Tooltip(label)
				if b.Hovered() || b.Pressed() {
					b.Background(pal.rowHover).TextColor(pal.text)
				}
				b.Children(func() { ui.Icon(c, svg).Size(11, 11) })
				if b.Clicked() {
					expand(fn)()
				}
			}
			switch {
			case s.First:
				chevron("up", chevronUpIcon, locales.ChangesExpandUp(), s.ExpandUp)
			case s.Last:
				chevron("down", chevronDownIcon, locales.ChangesExpandDown(), s.ExpandDown)
			case s.Lines <= s.Step:
				chevron("all", chevronIcon, locales.ChangesExpandAll(), s.ExpandAll)
			default:
				chevron("down", chevronDownIcon, locales.ChangesExpandDown(), s.ExpandDown)
				chevron("up", chevronUpIcon, locales.ChangesExpandUp(), s.ExpandUp)
			}
			label := locales.ChangesUnmodified(s.Lines)
			b := ui.ButtonBase(c.Key("lines")).Height(24).Padding(0, 4).Radius(6).AlignItems(ui.Center).Label(label).Tooltip(locales.ChangesExpandAll())
			b.Children(func() {
				fg := pal.textFaint
				if b.Hovered() {
					fg = pal.textMuted
				}
				ui.Text(c, label).FontSize(12).TextColor(fg).SingleLine()
			})
			if b.Clicked() {
				s.ExpandAll()
			}
		})
	})
}

// hunkButton is a small button of a change's actions, an icon and a
// label.
func hunkButton(c *ui.Context, key string, svg *ui.SVG, label, tip string, pal *palette) bool {
	b := ui.ButtonBase(c.Key(key)).Height(24).Padding(0, 8).Gap(5).Radius(6).AlignItems(ui.Center).
		TextColor(pal.textMuted).Label(label).Tooltip(tip)
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() {
		ui.Icon(c, svg).Size(13, 13)
		ui.Text(c, label).FontSize(12).FontWeight(500).SingleLine()
	})
	return b.Clicked()
}

// writeBack writes text, a file's diff's new text as a change reverted
// made it, to the file, through kerod, which writes it only as the file
// was shown: one changed meanwhile is shown anew instead. anyway writes it
// while an agent works there, as the user said to.
func (a *App) writeBack(sf *sideFile, text string, anyway bool) {
	cl := a.hostOf(sf.pane).cl
	if cl == nil {
		return
	}
	v := sf.d.New.Version()
	sf.writing, sf.edits.saving = true, true
	// The file as shown now, which a refresh while the call goes would
	// pair with a newer hash.
	p := proto.GitWriteParams{Pane: sf.pane, Base: sf.base, Path: sf.f.Path, Hash: sf.hash, Text: text, Anyway: anyway}
	go func() {
		var hash string
		err := cl.Call("git.revert", p, &hash)
		a.update(func() {
			sf.writing, sf.edits.saving = false, false
			a.side.git.done++
			if err != nil {
				a.toast(locales.ChangesRevertFailed(err.Error()))
				sf.d.New.MarkSaved()
				a.fetchFile(sf, "")
				return
			}
			sf.hash = hash
			if sf.d.New.Version() == v {
				sf.d.New.MarkSaved()
			}
		})
	}()
}
