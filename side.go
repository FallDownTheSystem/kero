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

// The side panel stands at the right of a session's panes, a card of its
// own, with what the session's worktree is about. It has views, each a
// list whose item opens in the panel as a file: Changes, the files the
// session's agent or the user changed, each opening as its diff, which
// Uncommitted stages and commits (staging.go, commitbox.go); Files
// (files.go), the worktree's tree, each file opening as its text; and
// History (history.go), its commits. The Changes and History views have
// the worktree's branch atop them (gitbar.go).

// sideChanges is the Changes view's id, as prefs.Side holds it.
const sideChanges = "changes"

// sideCompact is how much of their labels the header's tabs show.
type sideCompact int

const (
	tabsLabels sideCompact = iota
	tabsShownLabel
	tabsIcons
)

const (
	defaultSide = 420 // the panel's width, as it first opens
	minSide     = 300
	// compactSide is the width below which the header's tabs but the shown
	// one show their icons alone, beside the window's controls: the four
	// tabs' labels, the side's buttons and the controls take some 520.
	// Below tinySide, the shown one's label gives its room too.
	compactSide = 530
	tinySide    = 400
	minPanes    = 360 // the room the panel leaves the panes
	splitWidth  = 760 // past which diff-layout auto shows diffs side by side
)

// sideState is the panel's, beside the prefs that open it and size it.
type sideState struct {
	drag    float32 // the width as a drag of the edge began, 0 while none
	focused bool    // the panel has the keyboard: its Escape, its diff's find
	// editing tells that an editor of the panel's, a diff or a file, has
	// the keyboard, as the panel last built it: Find opens its find bar,
	// and the Search view elsewhere in the panel.
	editing bool
	// drafting is the review whose draft comment has the keyboard, as the
	// panel last built it, which the panel's Escape drops.
	drafting *review
	// wide zooms the panel: it takes the panes' room, hiding them, as Zoom
	// Pane does a pane, until the user turns it off or hides the panel.
	wide    bool
	changes changesState
	files   filesState
	search  searchState
	pulls   pullsState
	git     gitUI
	// theme is the colors of code, made again only as the theme they are
	// of, themeOf, changes.
	theme   *editor.Theme
	themeOf terminal.Theme
}

// changesState is the Changes view's: the files changed since a base,
// each with its diff, one after the other in one scroll.
type changesState struct {
	base string // the base chosen: git.BaseTurn, BaseHead or BaseBranch
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
	// indexes are Uncommitted's lists, staged and not, by scope, of
	// which lists has the files of both.
	indexes map[string]*proto.GitIndex
	// files are the files' diffs, by scope and path, kept as the list is
	// fetched anew; collapsed the files the user folded or unfolded, by
	// the same key; focus the file whose diff has the keyboard, for Find.
	files     map[string]*sideFile
	collapsed map[string]bool
	focus     *sideFile
	// folded are the tree's folders the user folded, by scope, section and
	// path (fileKey).
	folded map[string]bool
	// places are the lists' places, by scope.
	places map[string]*ui.ListState
	// moves are the files staged or unstaged that the index fetched last
	// may not show moved yet, which the list shows moved at once.
	moves []*indexMove
}

// sideFile is a changed file's diff since a base, or between two sides:
// staged, unstaged, or a commit's (base "rev", its sides those of f).
type sideFile struct {
	pane, base string
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

// The sides of the Uncommitted list's diffs, and of a commit's, as
// git.file takes them for its base.
const (
	baseStaged   = "staged"
	baseUnstaged = "unstaged"
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

// toggleSide shows a view of the panel, or hides the panel when it shows
// it.
func (a *App) toggleSide(view string) {
	if a.prefs.Side != view {
		a.showSide(view)
		return
	}
	a.prefs.Side = ""
	a.side.focused, a.side.wide = false, false
	a.focusTerminal()
	a.savePrefs()
}

// showSide shows a view of the panel.
func (a *App) showSide(view string) {
	if a.prefs.Side == view && a.prefs.SideView == view {
		return
	}
	a.prefs.Side, a.prefs.SideView = view, view
	a.savePrefs()
}

// sideView is the view the panel shows, else the one it showed last, which
// showing it again shows.
func (a *App) sideView() string {
	if a.prefs.Side != "" {
		return a.prefs.Side
	}
	return knownView(a.prefs.SideView)
}

// knownView is a view of the panel: Changes for one it doesn't know.
func knownView(view string) string {
	switch view {
	case sideFiles, sideHistory, sideSearch, sideReview:
		return view
	}
	return sideChanges
}

// sideAction is the action that shows and hides a view of the panel.
func sideAction(view string) string {
	switch view {
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

// sideLeft tells that the panel stands at the left of the panes: the
// sessions' sidebar takes the window's right.
func (a *App) sideLeft() bool { return a.set.SessionsSide == "right" }

// sideWidth is the panel's width in a row of width w, leaving the panes
// their room.
func (a *App) sideWidth(w float32) float32 {
	sw := a.prefs.SideWidth
	if sw == 0 {
		sw = defaultSide
	}
	if w > 0 {
		sw = min(sw, w-minPanes)
	}
	return max(sw, minSide)
}

// sideEdge is the gap between the panes and the panel, which a drag moves;
// left tells that the panel is at the left of it.
func (a *App) sideEdge(c *ui.Context, pal *palette, row float32, left bool) {
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
		if left {
			a.side.drag += dx
		} else {
			a.side.drag -= dx
		}
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

// sideFolder is what the panel's header shows of its folder, as the user
// chose (settings' ExplorerFolder).
func (a *App) sideFolder(c *ui.Context, s *proto.Session, dir string) string {
	if dir == "" {
		return ""
	}
	return a.folderShown(c, a.set.ExplorerFolder, a.hostOf(s.ID).shortDir(dir), proto.DirName(dir), "side", 13, 8, 0)
}

// sidePanel is the panel's card: its header, then its view. left and right
// tell that it touches the window's left and right edges, where its header
// holds that side's buttons: at the left (sidebarButtons) clear of the
// traffic lights, at the right with the window's controls (edgeControls).
func (a *App) sidePanel(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32, s *proto.Session, width float32, left, right bool) {
	card := ui.Column(c.Key("side")).Width(width).Shrink(0).MinHeight(0).Radius(14).Clip().
		Focusable().Bind(a.paneRef("side-card"))
	a.side.focused = card.FocusWithin()
	if a.set.FocusFollowsMouse && a.pointerEntered(c, "side-card", card, card.Hovered()) && !a.side.focused {
		a.paneRef("side-card").Focus()
	}
	// While the user is elsewhere, the panel fades as a terminal they are
	// not in does, as they chose (settings' DimExplorer).
	dim := a.set.DimExplorer && !a.side.focused
	// The panel shows code as the editor panes do, on the files' theme's
	// background; the window's palette draws its shadow still.
	wpal := pal
	pal = cardPalette(pal, a.fileTheme(c.Theme().Dark))
	paned(card.Background(card.AnimateColor("bg", cardBackground(wpal, pal, dim), ui.Animation{Duration: 140 * time.Millisecond})), wpal)
	view := knownView(a.prefs.Side)
	a.side.drafting, a.side.editing = nil, false
	// The side's buttons and the window's controls take the header's room
	// at the window's edge: in a narrow panel the tabs give theirs, down to
	// their icons, so that the controls stay in it.
	compact := tabsLabels
	switch {
	case (right || left) && width < tinySide:
		compact = tabsIcons
	case (right || left) && width < compactSide:
		compact = tabsShownLabel
	}
	// The first tab's icon starts where a header's title does.
	pad := float32(10)
	if left {
		pad = a.chrome.headerPad - 8
	}
	card.Children(func() {
		ui.Row(c.Key("hdr")).Height(headerH).Shrink(0).AlignItems(ui.Center).Padding(0, 10, 0, pad).Gap(2).Children(func() {
			if left {
				a.sidebarButtons(c, pal, bar, shown)
			}
			// The views, as tabs: the one shown says what the panel shows,
			// as a pane's header says its program.
			a.sideTab(c, pal, sideFiles, folderIcon, locales.SideFiles(), "files.toggle", compact)
			a.sideTab(c, pal, sideChanges, diffIcon, locales.SideChanges(), "changes.toggle", compact)
			a.sideTab(c, pal, sideHistory, historyIcon, locales.SideHistory(), "history.toggle", compact)
			a.sideTab(c, pal, sideReview, prIcon, locales.SideReview(), "review.toggle", compact)
			dir := a.changesDir(s)
			switch view {
			case sideFiles, sideSearch:
				_, dir = a.filesRoot(s)
			case sideHistory:
				dir = a.historyDir(s)
			}
			if name := a.sideFolder(c, s, dir); name != "" {
				ui.Text(c, name).FontSize(13).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0).Margin(0, 0, 0, 6).Tooltip(dir).Bind(a.paneRef("side-dir"))
			}
			ui.Box(c.Key("drag")).Grow(1).AlignSelf(ui.Stretch).DragWindow().Bind(a.paneRef("side-gap"))
			if a.side.wide {
				iconButton(c, "wide", unzoomIcon, locales.PaneShowAll(), pal, func() { a.side.wide = false })
			} else {
				iconButton(c, "wide", zoomIcon, locales.SideZoom(), pal, func() {
					a.side.wide = true
					// The panes it hides can't keep the keyboard.
					a.paneRef("side-card").Focus()
				})
			}
			// The panel's own button, which hides it, is at the window's
			// edge it stands at: the left's with the sidebar's buttons,
			// the right's with the window's controls.
			if right {
				a.edgeControls(c, pal, "-p")
			}
		})
		ui.Column(c.Key("body")).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			switch view {
			case sideFiles:
				a.filesView(c, pal, s)
			case sideHistory:
				a.historyView(c, pal, s, width)
			case sideSearch:
				a.searchView(c, pal, s)
			case sideReview:
				a.pullsView(c, pal, s, width)
			default:
				a.changesView(c, pal, s, width)
			}
		})
		a.nameView(c, pal)
		if a.set.DimExplorer {
			v := ui.Box(c.Key("veil")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(pal.veil)
			v.Opacity(v.Animate("on", b2f(dim), 160*time.Millisecond))
		}
	})
	if card.Shortcut(0, ui.KeyEscape) && !a.cancelDraft() {
		a.focusTerminal()
	}
}

// sideTab is a view's tab in the panel's header, which shows it: its icon
// alone while compact, but the shown view's while only somewhat. Search,
// which the Files view's toolbar opens, has none: Files' stands for it.
func (a *App) sideTab(c *ui.Context, pal *palette, view string, svg *ui.SVG, label, action string, compact sideCompact) {
	shown := knownView(a.prefs.Side)
	on := shown == view || view == sideFiles && shown == sideSearch
	b := ui.ButtonBase(c.Key("tab-"+view)).Height(28).Padding(0, 8).Gap(6).Radius(8).AlignItems(ui.Center).Shrink(0).
		TextColor(pal.textMuted).Label(label).Tooltip(a.tip(label, action))
	iconOnly := compact == tabsIcons || compact == tabsShownLabel && !on
	if iconOnly {
		b.Padding(0, 7)
	}
	switch {
	case on:
		b.Background(pal.rowHover).TextColor(pal.text)
	case b.Hovered() || b.Pressed():
		b.Background(pal.rowHover.Alpha(0.5)).TextColor(pal.text)
	}
	b.Children(func() {
		ui.Icon(c, svg).Size(14, 14).Opacity(0.8)
		if !iconOnly {
			ui.Text(c, label).FontSize(13.5).FontWeight(600).SingleLine()
		}
	})
	if b.Clicked() && shown != view {
		a.defer_(func() { a.showSide(view) })
	}
}

// --- the Changes view ---

// changesPanes returns the panes the Changes view is of: the one the user
// is in, whose worktree it shows, and the one of the session whose agent
// last started a turn, the user's first, nil for none.
func (a *App) changesPanes(s *proto.Session) (here, turn *proto.Pane) {
	here = s.PaneByID(a.focusOf(s))
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

// changesDir is the worktree the Changes view lists: the agent's where
// it shows its turn, else the one of the pane the user is in; "" for none.
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

// changesScope names what the Changes view lists for a pane since a
// base: its worktree's changes, or its turn's, which the list, its files'
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

func (a *App) changesView(c *ui.Context, pal *palette, s *proto.Session, width float32) {
	ch := &a.side.changes
	here, turn := a.changesPanes(s)
	if !a.set.GitStatus {
		a.sideMessage(c, pal, locales.ChangesGitOff(), locales.ChangesGitOffDetail())
		return
	}
	if here == nil || here.Git == nil && turn == nil {
		a.sideMessage(c, pal, locales.ChangesNoRepo(), locales.ChangesNoRepoDetail())
		return
	}

	// The bases this worktree offers, and the one shown: the agent's turn
	// first, as the panel opens.
	var bases, labels, tips []string
	if turn != nil {
		bases, labels, tips = append(bases, git.BaseTurn), append(labels, locales.ChangesTurn()), append(tips, locales.ChangesTurnTip())
	}
	if here.Git != nil {
		bases, labels, tips = append(bases, git.BaseHead), append(labels, locales.ChangesUncommitted()), append(tips, locales.ChangesUncommittedTip())
		if d := here.Git.Default; d != "" {
			bases, labels, tips = append(bases, git.BaseBranch), append(labels, locales.ChangesBranch()), append(tips, locales.ChangesBranchTip(d))
		}
	}
	if !slices.Contains(bases, ch.base) {
		ch.base = bases[0]
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
	working := false
	for _, p := range s.Panes {
		working = working || p.Status == proto.Working
	}
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

	if here.Git != nil {
		a.gitBar(c, pal, s, here)
	}
	ix := movedIndex(ch.indexes[scope], ch.moves, scope)
	if ch.base != git.BaseHead {
		ix = nil
	}
	ui.Column(c.Key("bases")).Shrink(0).Padding(2, 12, 6, 12).Gap(6).AlignItems(ui.Stretch).Children(func() {
		if len(bases) > 1 {
			ui.Row(c).Children(func() { segmented(c, "base", &ch.base, bases, labels) })
		}
		i := slices.Index(bases, ch.base)
		ui.Row(c).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
			if res := list; res != nil && len(res.Files) > 0 {
				added, removed := 0, 0
				for _, f := range res.Files {
					added, removed = added+f.Added, removed+f.Removed
				}
				if ch.base == git.BaseHead && pane.Git != nil {
					// Since HEAD, as the status counts it: a file both
					// staged and not is one change.
					added, removed = pane.Git.Added, pane.Git.Removed
				}
				// Uncommitted's sections each have theirs, in their titles.
				if ix == nil {
					keys := make([]string, len(res.Files))
					for i, f := range res.Files {
						keys[i] = fileKey(scope, "", f.Path)
					}
					a.foldAllButton(c, pal, keys)
				}
				ui.Text(c, locales.ChangesFiles(len(res.Files))).FontSize(12.5).TextColor(pal.textMuted).SingleLine().Shrink(0)
				a.lineCounts(c, pal, added, removed, 12.5)
				if ch.base == git.BaseBranch && res.BaseName != "" {
					ui.Text(c, locales.ChangesSince(res.BaseName)).FontSize(12.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0)
				}
			} else {
				ui.Text(c, tips[i]).FontSize(12.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0)
			}
			ui.Spacer(c)
			ui.Row(c.Key("tools")).Gap(0).AlignItems(ui.Center).Shrink(0).Children(func() {
				a.treeButton(c, pal)
				a.wrapButton(c, pal)
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
			})
		})
	})

	res := list
	if held {
		c.After(400 * time.Millisecond)
	}
	switch {
	case res == nil:
		if ch.err != "" && !ch.busy && ch.scope == scope {
			a.sideMessage(c, pal, locales.ChangesCantRead(), ch.err)
		} else if c.Now().Sub(ch.switched) > 250*time.Millisecond {
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		} else {
			c.After(260 * time.Millisecond)
			ui.Spacer(c)
		}
	case len(res.Files) == 0 && held:
		// Another base's "no changes" would say the wrong thing.
		ui.Spacer(c)
	case len(res.Files) == 0:
		detail := map[string]string{git.BaseTurn: locales.ChangesNoneTurn(), git.BaseHead: locales.ChangesNoneUncommitted()}[ch.base]
		if ch.base == git.BaseBranch {
			detail = locales.ChangesNoneBranch(res.BaseName)
		}
		a.sideMessage(c, pal, locales.ChangesNone(), detail)
	default:
		// Uncommitted lists what isn't staged, then what is, each file
		// under its section's title; the other bases their files alone.
		// What isn't staged comes first: staging a file takes its row out
		// from under the pointer, the rows above it staying, and the next
		// file comes where it was; what is staged grows below.
		var rows []changeRow
		files := func(section, base string, list []proto.GitFile) {
			if !held {
				a.syncFiles(c, scope, section, base, pane, list)
			}
			if !a.prefs.ChangesTree {
				for _, f := range list {
					key := fileKey(scope, section, f.Path)
					rows = append(rows, changeRow{f: f, section: section})
					if ch.files[key] != nil && !a.fileCollapsed(key) {
						rows = append(rows, changeRow{f: f, section: section, diff: true})
					}
				}
				return
			}
			for _, e := range changeTree(list, func(dir string) bool { return !ch.folded[fileKey(scope, section, dir)] }) {
				if e.folder {
					rows = append(rows, changeRow{section: section, tree: &e})
					continue
				}
				key := fileKey(scope, section, e.f.Path)
				rows = append(rows, changeRow{f: e.f, section: section, tree: &e})
				if ch.files[key] != nil && !a.fileCollapsed(key) {
					rows = append(rows, changeRow{f: e.f, section: section, diff: true})
				}
			}
		}
		if ix != nil {
			for _, sec := range []struct {
				name string
				list []proto.GitFile
			}{{baseUnstaged, ix.Unstaged}, {baseStaged, ix.Staged}} {
				if len(sec.list) > 0 {
					rows = append(rows, changeRow{section: sec.name, title: true})
					files(sec.name, sec.name, sec.list)
				}
			}
		} else {
			files("", ch.base, res.Files)
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
		// pull request do.
		ls.Header = func(i int) bool { r := rows[i]; return !r.diff && !r.title && (r.tree == nil || !r.tree.folder) }
		ls.Key = func(i int) any {
			r := rows[i]
			k := changeKey{path: r.f.Path, section: r.section, diff: r.diff, title: r.title}
			if r.tree != nil && r.tree.folder {
				k.dir = r.tree.dir
			}
			return k
		}
		box := ui.Column(c.Key("files-" + scope)).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
		box.Opacity(box.Animate("held", 1-0.45*b2f(held), 120*time.Millisecond))
		box.Children(func() {
			// The scroll bar runs below the header pinned at the top, not
			// under it, in room of its own at the right, as History's does,
			// clear of the files' line counts.
			ui.List(c.Key("rows"), ls, len(rows), func(i int) {
				r := rows[i]
				key := fileKey(scope, r.section, r.f.Path)
				// A section's files and diffs have its color down their
				// left edge, as its title has, so that the two sections are
				// told apart as they scroll.
				stripe := func(e ui.Element) ui.Element {
					if r.section == "" {
						return e
					}
					col := a.sectionColor(c, pal, r.section).Alpha(0.55)
					return e.DrawOver(func(p *ui.Painter, rr ui.Rect) { p.Fill(ui.Rect{X: rr.X, Y: rr.Y, W: 3, H: rr.H}, col, 0) })
				}
				switch {
				case r.title:
					a.indexTitle(c, pal, here, ix, r.section)
					return
				case r.diff:
					stripe(ui.Column(c).FillWidth().AlignItems(ui.Stretch)).Children(func() { a.fileDiff(c, pal, s, ch.files[key], width) })
					return
				case r.tree != nil && r.tree.folder:
					var tools func()
					var menu func(*ui.Menu)
					if r.section != "" {
						dir := r.tree.dir
						list := ix.Unstaged
						if r.section == baseStaged {
							list = ix.Staged
						}
						tools = func() { a.indexListTools(c, pal, here, r.section, list, dir) }
						menu = func(m *ui.Menu) {
							if m.Item(locales.GitIgnoreFolder()).Chosen() {
								a.ignore(here, dir+"/")
							}
						}
					}
					stripe(ui.Column(c).FillWidth().AlignItems(ui.Stretch).Background(pal.card)).Children(func() {
						a.folderRow(c, pal, fileKey(scope, r.section, r.tree.dir), *r.tree, tools, menu)
					})
					return
				}
				// Opaque, over the diff scrolling under it; a folded file's
				// with the room below its section.
				gap, depth := float32(sectionGap), flatList
				if r.section != "" {
					gap = treeGap // Uncommitted's rows close together too
				}
				if r.tree != nil {
					// A tree's rows close together, its folders' among them.
					gap, depth = treeGap, r.tree.depth
				}
				if i+1 < len(rows) && rows[i+1].diff {
					gap = 0
				}
				stripe(ui.Column(c).FillWidth().AlignItems(ui.Stretch).Padding(0, 0, gap, 0).Background(pal.card)).Children(func() {
					var tools func()
					var menu func(*ui.Menu)
					if r.section != "" {
						f := r.f
						tools = func() { a.indexTools(c, pal, here, ix, r.section, f) }
						menu = func(m *ui.Menu) {
							if m.Item(locales.GitIgnore()).Chosen() {
								a.ignore(here, f.Path)
							}
						}
					}
					a.fileHeader(c, pal, key, here.ID, res.Root, r.f, depth, changeRowH, func() {
						// Folding it as the list pins it brings it back where
						// it stays: at the top.
						ls.ScrollIntoView(i)
					}, tools, menu)
				})
			}).Grow(1).MinHeight(0).Padding(0, 6, 16, 0).ScrollbarInsets(changeRowH, 0, 0, 0)
		})
	}

	// The review's comments and Send to Agent, below what they are on and
	// above the commit box.
	root := ""
	switch {
	case ch.base == git.BaseTurn && pane.Turn != nil:
		root = pane.Turn.Root
	case pane.Git != nil:
		root = pane.Git.Root
	}
	if root != "" {
		a.reviewBar(c, pal, pane, root)
	}
	if ch.base == git.BaseHead && here.Git != nil && ix != nil {
		a.commitBox(c, pal, here, ix)
	}
}

// changeRow is a row of the list of changes: a file's header, or its
// diff, of a section of Uncommitted's, or the section's title, or in the
// tree, a folder (tree); changeKey what it is, as the list keeps a row's
// state by.
type changeRow struct {
	f       proto.GitFile
	section string
	diff    bool
	title   bool
	tree    *treeEntry
}

type changeKey struct {
	path, section string
	diff, title   bool
	dir           string
}

// stickyH is the height of a file's header, which the list pins at its
// top; sectionGap the room below each file's section.
const (
	stickyH    = 32
	sectionGap = 6
	treeGap    = 2
)

// changeRowH is the height of a row of the Changes view's list, a file's
// or a folder's: shorter than a header elsewhere, for more files at once.
const changeRowH = 26

// flatList is the depth of a file's header in a list, not a tree, which
// shows its folder beside its name; treeIndent the room each of a tree's
// levels takes.
const (
	flatList   = -1
	treeIndent = 14
)

// syncFiles keeps the diffs of a list's files: it asks for the text of
// each file shown that has none yet, a few at a time, and again of one
// whose change the list counts anew (Known, so that kerod sends only what
// changed), or of every one as the worktree's HEAD moves.
func (a *App) syncFiles(c *ui.Context, scope, section, base string, pane *proto.Pane, files []proto.GitFile) {
	ch := &a.side.changes
	if ch.files == nil || len(ch.files) > 400 {
		ch.files = map[string]*sideFile{}
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
		if a.fileCollapsed(key) || sf.busy || sf.writing || a.hostOf(pane.ID).cl == nil {
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
// folded: as the user left it, else folded, so that a list of changes
// reads as the files it changed, each of which a click unfolds.
func (a *App) fileCollapsed(key string) bool {
	if v, ok := a.side.changes.collapsed[key]; ok {
		return v
	}
	return true
}

// foldAllButton folds every file of keys (fileKey) while one is unfolded,
// else unfolds them all. It starts a row 12 DIPs in, as a file's header
// does, its icon over the files' chevrons.
func (a *App) foldAllButton(c *ui.Context, pal *palette, keys []string) {
	if len(keys) == 0 {
		return
	}
	fold := slices.ContainsFunc(keys, func(k string) bool { return !a.fileCollapsed(k) })
	icon, tip := unfoldAllIcon, locales.ChangesUnfoldFiles()
	if fold {
		icon, tip = foldAllIcon, locales.ChangesFoldFiles()
	}
	// 20 wide from 5 before the row's start: centered on the chevrons,
	// which are 10 wide.
	b := ui.ButtonBase(c.Key("fold-all")).Size(20, 22).Margin(0, 0, 0, -5).Radius(5).Center().Shrink(0).TextColor(pal.textMuted).Label(tip).Tooltip(tip)
	if b.Pressed() {
		b.Background(pal.rowHover.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, icon).Size(14, 14) })
	if b.Clicked() {
		ch := &a.side.changes
		if ch.collapsed == nil {
			ch.collapsed = map[string]bool{}
		}
		for _, k := range keys {
			ch.collapsed[k] = fold
		}
	}
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
// change, its name and, in a list (depth flatList), its folder, and the
// lines it adds and removes, which a click folds or unfolds, running
// folding first when set; in a tree, indented by its depth. tools, when
// set, draws its buttons at its end. Its context menu opens the file, of
// pane's host in its worktree's root, and drags it onto a pane, unless it
// was deleted, or root is ""; then come menu's items, when set. height is
// the row's: stickyH, but in the Changes view's list.
func (a *App) fileHeader(c *ui.Context, pal *palette, key, pane, root string, f proto.GitFile, depth int, height float32, folding, tools func(), menu func(*ui.Menu)) {
	ch := &a.side.changes
	collapsed := a.fileCollapsed(key)
	// Across the panel, edge to edge, its content in line with the bases'.
	hdr := ui.Row(c.Key("hdr")).Height(height).Padding(0, 10, 0, 12+treeIndent*float32(max(depth, 0))).Gap(8).AlignItems(ui.Center).Background(pal.chip)
	if hdr.Hovered() {
		hdr.Background(pal.rowHover.Mix(pal.chip, 0.5))
	}
	// Without a root, the file is on no disk: of a pull request no worktree
	// has checked out.
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
		ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(fg).SingleLine().Ellipsis("…").Shrink(0).MaxWidth(260).Tooltip(tip)
		ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if f.Binary {
			ui.Text(c, locales.ChangesBinary()).FontSize(11.5).TextColor(pal.textFaint).SingleLine()
		} else {
			a.lineCounts(c, pal, f.Added, f.Removed, 12)
		}
		if tools != nil {
			tools()
		}
	})
}

// folderRow is a folder of the tree of changes, by its key (fileKey),
// which a click folds or unfolds; tools, when set, draws its buttons at
// its end, as a file's header does, and menu is its context menu.
func (a *App) folderRow(c *ui.Context, pal *palette, key string, e treeEntry, tools func(), menu func(*ui.Menu)) {
	ch := &a.side.changes
	folded := ch.folded[key]
	row := ui.Row(c.Key("folder")).Height(changeRowH).Margin(0, 0, treeGap, 0).Padding(0, 10, 0, 12+treeIndent*float32(e.depth)).Gap(8).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	row.Tooltip(e.dir)
	if menu != nil {
		row.ContextMenu(menu)
	}
	if row.Clicked() {
		if ch.folded == nil {
			ch.folded = map[string]bool{}
		}
		ch.folded[key] = !folded
	}
	row.Children(func() {
		chev := ui.Icon(c, chevronIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		chev.Rotate(chev.AnimateTo("turn", 90*(1-b2f(folded)), chevronTurn))
		ui.Icon(c, folderIcon).Size(14, 14).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, e.label).FontSize(12.5).FontWeight(550).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
		ui.Text(c, itoa(e.count)).FontSize(11).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Shrink(0)
		if tools != nil {
			ui.Spacer(c)
			tools()
		}
	})
}

// treeButton turns the Changes view's tree of folders on and off.
func (a *App) treeButton(c *ui.Context, pal *palette) {
	b := ui.ButtonBase(c.Key("tree")).Size(28, 26).Margin(0, 2).Radius(6).Center().TextColor(pal.textMuted).Label(locales.ChangesTree()).Tooltip(locales.ChangesTree())
	switch {
	case a.prefs.ChangesTree:
		b.Background(pal.rowHover).TextColor(pal.text)
	case b.Hovered() || b.Pressed():
		b.Background(pal.rowHover.Alpha(0.5)).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, treeIcon).Size(15, 15) })
	if b.Clicked() {
		a.prefs.ChangesTree = !a.prefs.ChangesTree
		a.savePrefs()
	}
}

// sectionNote says why a file shows no diff.
func (a *App) sectionNote(c *ui.Context, pal *palette, text string) {
	ui.Text(c, text).FontSize(12.5).TextColor(pal.textMuted).Padding(10, 12).LineHeight(1.4)
}

// fetchChanges asks kerod for the list of a pane's changes since a base.
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

// statusBadge is a file's change as a letter in its color: A added, M
// modified, D deleted, R renamed, T a change of type, U untracked, and !
// a conflict.
func (a *App) statusBadge(c *ui.Context, pal *palette, status string) {
	added, removed, modified := a.diffColors(c, pal)
	letter, col := "M", modified
	switch status {
	case "added":
		letter, col = "A", added
	case "deleted":
		letter, col = "D", removed
	case "renamed":
		letter, col = "R", modified
	case "type":
		letter = "T"
	case "untracked":
		letter, col = "U", added
	case "conflict":
		letter, col = "!", removed
	}
	ui.Box(c).Size(18, 18).Radius(5).Shrink(0).Center().Background(col.Alpha(0.14)).Children(func() {
		ui.Text(c, letter).FontSize(11).FontWeight(650).TextColor(col).SingleLine()
	})
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
// edits, so that the diff keeps where it was scrolled.
func (a *App) showText(sf *sideFile, t proto.GitText) {
	sf.hash, sf.index = t.Hash, t.Index
	sf.text = proto.GitText{Binary: t.Binary, Large: t.Large, Mixed: t.Mixed}
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
	default:
		replaceText(sf.d.Old, t.Old)
		replaceText(sf.d.New, t.New)
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

// diffSplit tells whether diffs show side by side in a panel this wide.
func (a *App) diffSplit(width float32) bool {
	switch a.set.DiffLayout {
	case "split":
		return true
	case "unified":
		return false
	}
	return width >= splitWidth
}

// diffOptions sets a file's diff's looks and actions for this frame: in a
// turn, Revert and Keep on each change, as the user reviews what the
// agent did; in Uncommitted, Stage and Discard on a change not staged,
// and Unstage on one staged; in Branch, Revert.
func (a *App) diffOptions(c *ui.Context, pal *palette, s *proto.Session, sf *sideFile, width float32) {
	d := sf.d
	d.FitContent = true
	d.Font = ui.Font{Family: a.termFamily(), Size: max(a.set.FontSize-1, 10)}
	d.Theme = a.codeTheme(c, pal)
	d.Layout = editor.DiffUnified
	if a.diffSplit(width) {
		d.Layout = editor.DiffSplit
	}
	d.Wrap = a.prefs.Wrap
	d.RenderSeparator = func(c *ui.Context, sep ui.Element, s *editor.DiffSeparator) { a.separator(c, pal, sep, s) }
	// A change is edited in its buffer, then written whole: never of a
	// patch, nor of a text whose lines end two ways, which the buffer
	// would write back as one; nor of a file the index has in conflict.
	// Uncommitted's stage and unstage the lines selected by their numbers
	// too, from the bar of a change they are of (lineActions).
	// Branch's Revert puts the branch's base's lines back in the file, a
	// change not committed yet.
	editable := !sf.patch && !sf.writing && !sf.text.Mixed
	switch sf.base {
	case git.BaseTurn:
	case baseStaged, baseUnstaged, git.BaseBranch:
		editable = editable && sf.f.Status != "conflict"
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

// bufferText is a buffer's text with the line endings it came with, as
// the file has them.
func bufferText(b *editor.Buffer) string {
	var s strings.Builder
	b.WriteTo(&s)
	return s.String()
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
	sf.writing = true
	// The file as shown now, which a refresh while the call goes would
	// pair with a newer hash.
	p := proto.GitWriteParams{Pane: sf.pane, Base: sf.base, Path: sf.f.Path, Hash: sf.hash, Text: text, Anyway: anyway}
	go func() {
		var hash string
		err := cl.Call("git.revert", p, &hash)
		a.update(func() {
			sf.writing = false
			a.side.git.done++
			if err != nil {
				a.toast(locales.ChangesRevertFailed(err.Error()))
				a.fetchFile(sf, "")
				return
			}
			sf.hash = hash
		})
	}()
}
