package main

import (
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

const paneGap = 8 // between panes, where the divider is

// paneRef returns the handle a pane's terminal binds, which its focus
// follows.
func (a *App) paneRef(id string) *ui.Handle {
	if a.refs == nil {
		a.refs = map[string]*ui.Handle{}
	}
	h := a.refs[id]
	if h == nil {
		h = &ui.Handle{}
		a.refs[id] = h
	}
	return h
}

// focusOf returns the pane of a session the user is in.
func (a *App) focusOf(s *proto.Session) string {
	if id := a.focus[s.ID]; id != "" && s.PaneByID(id) != nil {
		return id
	}
	if p := s.FocusPane(); p != nil {
		return p.ID
	}
	return ""
}

// shownPanes returns the panes of a session the window shows, as panes
// lays them out: the zoomed one alone, or the one the user is in without
// a layout.
func shownPanes(s *proto.Session) []string {
	p := s.PaneByID(s.Zoomed)
	if p == nil && s.Layout != nil && len(s.Panes) > 1 {
		ids := make([]string, len(s.Panes))
		for i, p := range s.Panes {
			ids[i] = p.ID
		}
		return ids
	}
	if p == nil {
		p = s.FocusPane()
	}
	if p == nil {
		return nil
	}
	return []string{p.ID}
}

// setFocus moves the user into a pane, and tells kerod, which remembers it.
func (a *App) setFocus(s *proto.Session, pane string) {
	if a.focus == nil {
		a.focus = map[string]string{}
	}
	if a.focus[s.ID] == pane && s.Focus == pane {
		return
	}
	a.focus[s.ID] = pane
	s.Focus = pane
	if s.Zoomed != "" {
		s.Zoomed = pane
	}
	a.ensureTerm(pane)
	a.touch(pane)
	a.call(s.ID, "focus", proto.FocusParams{Session: s.ID, Pane: pane}, nil)
}

// focusTerminal gives the keyboard to the pane the user is in.
func (a *App) focusTerminal() {
	if s := a.session(a.selected); s != nil {
		if id := a.focusOf(s); id != "" {
			a.paneRef(id).Focus()
		}
		a.claimShown()
	}
}

// panes shows a session's panes in their layout: the zoomed one alone.
// left and right tell that they reach the window's left and right edges,
// which the side panel or the sidebar take while they show.
func (a *App) panes(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32, s *proto.Session, left, right bool) {
	multi := len(s.Panes) > 1
	if z := s.PaneByID(s.Zoomed); z != nil || s.Layout == nil || !multi {
		p := z
		if p == nil {
			p = s.FocusPane()
		}
		if p != nil {
			a.paneCard(c, pal, bar, shown, s, p, true, left, right, multi)
		}
		return
	}
	a.layoutNode(c, pal, bar, shown, s, s.Layout, "", true, left, right)
}

func (a *App) layoutNode(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32, s *proto.Session, l *proto.Layout, path string, top, left, right bool) {
	if l.Leaf() {
		if p := s.PaneByID(l.Pane); p != nil {
			a.paneCard(c, pal, bar, shown, s, p, top, left, right, true)
		}
		return
	}
	ratio := float32(l.Ratio)
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.5
	}
	// Row and Column each build an element, so make only the one used: a
	// Row made and replaced would stay, an empty sibling under the key.
	key := "split-" + s.ID + path
	var box ui.Element
	if l.Row {
		box = ui.Row(c.Key(key))
	} else {
		box = ui.Column(c.Key(key))
	}
	box.Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Bind(a.paneRef(key))
	box.Children(func() {
		ui.Column(c.Key("a")).Grow(ratio).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.layoutNode(c, pal, bar, shown, s, l.A, path+"a", top, left, right && !l.Row)
		})
		a.divider(c, pal, s, l, key)
		ui.Column(c.Key("b")).Grow(1 - ratio).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.layoutNode(c, pal, bar, shown, s, l.B, path+"b", top && l.Row, left && !l.Row, right)
		})
	})
}

// divider is the gap between two panes, which a drag moves.
func (a *App) divider(c *ui.Context, pal *palette, s *proto.Session, l *proto.Layout, key string) {
	// A drag leaves the keyboard in the pane the user is in.
	d := ui.Box(c.Key("div")).Shrink(0).KeepFocus()
	if l.Row {
		d.Width(paneGap).Cursor(ui.CursorResizeColumn)
	} else {
		d.Height(paneGap).Cursor(ui.CursorResizeRow)
	}
	if d.Hovered() || d.Pressed() {
		// A short grip, so the gap shows it moves.
		d.Draw(func(p *ui.Painter, r ui.Rect) {
			if l.Row {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 1.5, Y: r.Y + r.H/2 - 18, W: 3, H: 36}, pal.textFaint.Alpha(0.7), 1.5)
			} else {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 18, Y: r.Y + r.H/2 - 1.5, W: 36, H: 3}, pal.textFaint.Alpha(0.7), 1.5)
			}
		})
	}
	if dx, dy, ok := d.Dragged(); ok {
		// The ratio shares out the room the panes take, the split's less
		// the divider's.
		b := a.paneRef(key).Bounds(c)
		size, delta := b.W-paneGap, dx
		if !l.Row {
			size, delta = b.H-paneGap, dy
		}
		if size > 0 {
			// The ratio goes on past its bounds, unclamped, so the divider
			// comes back as the pointer comes back to it.
			if a.layoutDrag.split != key {
				r := l.Ratio
				if r <= 0 || r >= 1 {
					r = 0.5
				}
				a.layoutDrag.session, a.layoutDrag.split, a.layoutDrag.ratio = s.ID, key, r
			}
			a.layoutDrag.ratio += float64(delta / size)
			l.Ratio = math.Min(math.Max(a.layoutDrag.ratio, 0.12), 0.88)
		}
	} else if a.layoutDrag.split == key && !d.Pressed() {
		// Only the divider dragged saves the layout: another of the same
		// session would save it every frame of the drag.
		a.layoutDrag.session, a.layoutDrag.split = "", ""
		a.call(s.ID, "layout", proto.LayoutParams{Session: s.ID, Layout: s.Layout.Clone()}, nil)
	}
	if d.DoubleClicked() {
		l.Ratio = 0.5
		a.call(s.ID, "layout", proto.LayoutParams{Session: s.ID, Layout: s.Layout.Clone()}, nil)
	}
}

// paneCard is a pane: a card with a header and the terminal. top, left and
// right tell that it touches the window's edges, where the header drags
// the window, keeps clear of the traffic lights on macOS, and holds the
// window controls elsewhere (windowControls).
func (a *App) paneCard(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32, s *proto.Session, p *proto.Pane, top, left, right, multi bool) {
	focused := p.ID == a.focusOf(s)
	bg := pal.card
	// A browser's page stays opaque, so its toolbar does too, and neither
	// fades: a page is read, not typed into, from beside it; so is a file.
	if multi && !focused && p.Terminal() && a.set.DimUnfocused {
		bg = pal.cardDim
	}
	card := ui.Column(c.Key("pane-" + p.ID)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Radius(14).Clip().Bind(a.paneRef("card-" + p.ID))
	raised(card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 140 * time.Millisecond})), pal)
	hovered := card.Hovered()
	if hovered && a.menuPane != "" && a.menuPane != p.ID {
		// Another pane under the pointer: the menu is gone.
		a.menuPane = ""
	}
	if a.set.FocusFollowsMouse && multi {
		a.followMouse(c, s, p, card, hovered, focused)
	}
	// The card takes presses: one on its padding, beside the terminal,
	// focuses the pane, rather than going on to what drags the window.
	if card.Pressed() && !focused {
		a.setFocus(s, p.ID)
		a.paneRef(p.ID).Focus()
	}

	// Another pane dragged over this one: where it would go.
	zone := zoneNone
	if multi {
		if d, over := ui.DragOver[paneDrag](card); over && d.session == s.ID && d.pane != p.ID {
			if x, y, in := card.PointerPosition(); in {
				b := a.paneRef("card-" + p.ID).Bounds(c)
				zone = zoneAt(x/max(b.W, 1), y/max(b.H, 1))
			}
		}
		if d, ok := ui.Drop[paneDrag](card); ok && d.session == s.ID && d.pane != p.ID {
			sid, target := s.ID, p.ID
			drop := a.lastZone
			a.defer_(func() { a.movePane(sid, d.pane, target, drop) })
		}
	}
	if zone != zoneNone {
		a.lastZone = zone
	}

	dragging := false
	card.Children(func() {
		switch {
		case p.Browser():
			// A browser's address bar is its header.
			dragging = a.paneBrowser(c, pal, bar, shown, s, p, top, left, right, multi, focused, hovered)
		case p.Editor():
			dragging = a.paneHeader(c, pal, bar, shown, s, p, top, left, right, multi, focused, hovered)
			a.paneEditor(c, pal, s, p)
		default:
			dragging = a.paneHeader(c, pal, bar, shown, s, p, top, left, right, multi, focused, hovered)
			a.paneTerminal(c, pal, s, p)
		}
		if multi && p.Terminal() && a.set.DimUnfocused {
			// The terminals the user is not in fade, text and all.
			v := ui.Box(c.Key("veil")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(pal.veil)
			v.Opacity(v.Animate("on", b2f(!focused), 160*time.Millisecond))
		}
		if zone != zoneNone {
			a.dropPreview(c, pal, p.ID, zone)
		}
	})

	// The pane dragged sinks back where it was; one just moved springs in
	// at its new place.
	pose := ui.Pose{}
	switch {
	case dragging:
		pose = ui.Pose{Scale: 0.985, Fade: 0.45}
	case a.arrived[p.ID]:
		pose = ui.Pose{Scale: 0.94, Fade: 0.7}
		delete(a.arrived, p.ID)
		c.Invalidate()
	}
	card.AnimatePose(pose, ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0.18}})
}

// followMouse focuses the pane the pointer moves into. Only the pointer
// moving does: a pane that comes under a pointer standing still, as another
// session shows or the layout changes, leaves the focus where the keyboard
// put it. And only going in does, so the keyboard can take the focus from
// the pane the pointer rests in.
func (a *App) followMouse(c *ui.Context, s *proto.Session, p *proto.Pane, card ui.Element, hovered, focused bool) {
	x, y, over := card.PointerPosition()
	if !over {
		if a.hoverPane == p.ID {
			a.hoverPane = ""
		}
		return
	}
	b := a.paneRef("card-" + p.ID).Bounds(c)
	at := [2]float32{b.X + x, b.Y + y}
	moved := at != a.hoverAt
	a.hoverAt = at
	// A press begun in another pane, as a selection dragged on past its
	// edge, does not hover this one.
	if !hovered || a.hoverPane == p.ID {
		return
	}
	a.hoverPane = p.ID
	if moved && !focused {
		a.setFocus(s, p.ID)
		a.paneRef(p.ID).Focus()
	}
}

// paneDrag is a pane dragged by its title to another place.
type paneDrag struct{ session, pane string }

// dropZone is where a pane dropped on another goes: beside it, on a side,
// or in its place, which the other takes (swap).
type dropZone int

const (
	zoneNone dropZone = iota
	zoneLeft
	zoneRight
	zoneUp
	zoneDown
	zoneSwap
)

// zoneAt returns the zone under the pointer, at u, v of a pane (0 to 1):
// its middle swaps, else the nearest edge.
func zoneAt(u, v float32) dropZone {
	if u > 0.3 && u < 0.7 && v > 0.3 && v < 0.7 {
		return zoneSwap
	}
	best, zone := u, zoneLeft
	for _, e := range []struct {
		d float32
		z dropZone
	}{{1 - u, zoneRight}, {v, zoneUp}, {1 - v, zoneDown}} {
		if e.d < best {
			best, zone = e.d, e.z
		}
	}
	return zone
}

func (z dropZone) side() string {
	return [...]string{"", "left", "right", "up", "down", ""}[z]
}

// dropPreview shows, over the pane under the pointer, the room the
// dragged pane would take, gliding from zone to zone, with what dropping
// does.
func (a *App) dropPreview(c *ui.Context, pal *palette, target string, zone dropZone) {
	key := target
	a.previewShown = true
	if a.previewKey != key {
		a.previewKey, a.previewSince = key, c.Now()
	}
	// left, top, width, height in percent of the pane
	r := [...][4]float32{
		zoneLeft:  {0, 0, 50, 100},
		zoneRight: {50, 0, 50, 100},
		zoneUp:    {0, 0, 100, 50},
		zoneDown:  {0, 50, 100, 50},
		zoneSwap:  {0, 0, 100, 100},
	}[zone]
	label, glyph := map[dropZone]string{
		zoneLeft: locales.PaneSplitLeft(), zoneRight: locales.ActionSplitRight(), zoneUp: locales.PaneSplitUp(), zoneDown: locales.ActionSplitDown(), zoneSwap: locales.PaneSwap(),
	}[zone], splitRightIcon
	if zone == zoneUp || zone == zoneDown {
		glyph = splitDownIcon
	}
	if zone == zoneSwap {
		glyph = swapIcon
	}
	fadeIn := min(float32(c.Now().Sub(a.previewSince))/float32(140*time.Millisecond), 1)
	if fadeIn < 1 {
		c.AnimationFrame()
	}
	spring := ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.12}}
	o := ui.Column(c.Key("drop")).Absolute().PassThrough().Padding(8).Opacity(fadeIn)
	o.LeftPercent(o.AnimateTo("l", r[0], spring)).TopPercent(o.AnimateTo("t", r[1], spring)).
		WidthPercent(o.AnimateTo("w", r[2], spring)).HeightPercent(o.AnimateTo("h", r[3], spring))
	o.Children(func() {
		room := ui.Column(c).Grow(1).Radius(10).AlignItems(ui.Center).
			Background(pal.accent.Alpha(0.10)).InsetShadow(0, 0, 28, 0, pal.accent.Alpha(0.22))
		room.Children(func() {
			// The label sits high in the room, clear of the title the
			// pointer carries through its middle.
			ui.Box(c).HeightPercent(22).Shrink(0)
			pill := ui.Row(c).Height(30).Padding(0, 12).Gap(7).Radius(15).AlignItems(ui.Center).
				Background(pal.accent).TextColor(ui.Hex("#ffffff"))
			floating(pill, pal).Children(func() {
				ui.Icon(c, glyph).Size(14, 14)
				ui.Text(c, label).FontSize(12.5).FontWeight(600).SingleLine()
			})
			pill.AnimatePose(ui.Pose{Scale: 0.9 + 0.1*fadeIn}, ui.Snappy)
		})
	})
}

// movePane puts a pane dropped on another beside it, or in its place.
func (a *App) movePane(sid, pane, target string, zone dropZone) {
	s := a.session(sid)
	if s == nil || s.Layout == nil || zone == zoneNone {
		return
	}
	if zone == zoneSwap {
		s.Layout = s.Layout.Swap(pane, target)
		a.arrived[target] = true
	} else {
		s.Layout = s.Layout.Move(pane, target, zone.side())
	}
	a.arrived[pane] = true
	s.Zoomed = ""
	a.setFocus(s, pane)
	a.paneRef(pane).Focus()
	a.call(sid, "layout", proto.LayoutParams{Session: sid, Layout: s.Layout.Clone()}, nil)
}

// paneHeader builds a pane's header, and reports whether its title is
// being dragged.
func (a *App) paneHeader(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32, s *proto.Session, p *proto.Pane, top, left, right, multi, focused, hovered bool) (dragging bool) {
	pad := float32(16)
	if top && left {
		// With the sidebar hidden, the header at the window's top left
		// starts after the traffic lights and the sidebar's buttons,
		// which come here.
		pad = a.chrome.headerPad
	}
	h := ui.Row(c.Key("hdr")).Height(headerH).Shrink(0).AlignItems(ui.Center).Padding(0, 10, 0, pad).Gap(8)
	if h.Pressed() && !focused {
		a.setFocus(s, p.ID)
		a.paneRef(p.ID).Focus()
	}
	h.Children(func() {
		room := a.headerRoom(c, p, pad, titleRoom, top && right, multi)
		if top && left {
			a.sidebarButtons(c, pal, bar, shown)
		}
		// The title is the pane's handle: dragged, it takes the pane to
		// another place in the session.
		grip := ui.Row(c.Key("grip")).Gap(8).AlignItems(ui.Center).Shrink(1).MinWidth(0).Height(28).Padding(0, 8).Margin(0, 0, 0, -8).Radius(8)
		if multi {
			grip.Drag(paneDrag{s.ID, p.ID}).Tooltip(locales.PaneDragTip())
			dragging = grip.Dragging()
			switch {
			case dragging:
				// What follows the pointer is a copy of the grip: a chip.
				floating(grip.Background(pal.popover), pal)
			case grip.Hovered():
				grip.Background(pal.rowHover).Cursor(ui.CursorGrab)
			}
		}
		grip.Children(func() {
			if multi {
				if p.Status != proto.Idle || p.Unread {
					statusIconP(c, pal, p.Status, p.Unread, p.Progress, 13)
				} else {
					ui.Icon(c, programIcon(p)).Size(14, 14).TextColor(pal.textFaint)
				}
			}
			title, size := p.PaneName(), float32(13.5)
			if !multi {
				title, size = s.DisplayName(a.set.SessionName), 14
			}
			name := ui.Text(c, title).FontSize(size).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…")
			shortTitle(name, title)
			if multi && !focused && !dragging {
				name.TextColor(pal.textMuted)
			}
			if !multi && name.DoubleClicked() {
				a.startRename(s.ID)
			}
			// The home directory names itself "~", which the title may
			// already say; another machine's, as scp writes it: devbox:~/api.
			// The controls the pointer shows don't change what fits.
			h := a.hostOf(p.ID)
			key := "pane-" + p.ID
			gap := float32(8)
			if h.remote() {
				gap = 0 // the host's name stays, which the folder is in
			}
			dir := a.folderShown(c, a.set.PaneFolder, h.shortDir(p.WorkDir()), proto.DirName(p.WorkDir()), key, 13, gap,
				room.controls-a.paneRef("ctl-"+p.ID).Bounds(c).W)
			if dir == title && !h.remote() {
				dir = ""
			}
			switch {
			case h.remote():
				// The host's name in its color, as the sidebar's strip
				// marks it: no pane passes for another machine's.
				ui.Row(c.Key("where")).Shrink(2).MinWidth(0).AlignItems(ui.Center).Children(func() {
					label := h.name
					if dir != "" {
						label += ":"
					}
					ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(pal.hostColor(h)).SingleLine().Shrink(0)
					if dir != "" {
						ui.Text(c, dir).FontSize(13).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).Bind(a.paneRef(key + "-dir"))
					}
				})
			case dir != "":
				ui.Text(c, dir).FontSize(13).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0).Bind(a.paneRef(key + "-dir"))
			}
		})
		if p.Git != nil {
			a.gitChip(c, pal, s, p, room.chip)
		}
		// The header's empty stretch moves the window, as a title bar
		// does, and zooms it on a double click: the header around it
		// takes clicks, which focus the pane.
		gap := ui.Box(c.Key("drag")).Grow(1).AlignSelf(ui.Stretch).Bind(a.paneRef("pane-" + p.ID + "-gap"))
		if top {
			gap.DragWindow()
		}
		ui.Row(c.Key("chips")).Gap(8).AlignItems(ui.Center).Shrink(0).Bind(a.paneRef("chips-" + p.ID)).Children(func() {
			a.paneChips(c, pal, p, !multi)
		})
		a.paneControls(c, pal, s, p, (hovered || focused && multi) && !dragging, multi, room.fold)
		if top && right {
			a.edgeControls(c, pal, "-h")
		}
	})
	return dragging
}

// sidebarButtons are the sidebar's buttons, which the header at the
// window's top left holds while the sidebar hides, and always while the
// sidebar is at the window's right.
func (a *App) sidebarButtons(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32) {
	fade := 1 - 2*shown
	if a.chrome.right {
		fade = 1
	}
	if fade <= 0 {
		return
	}
	ui.Row(c).Absolute().Left(a.chrome.buttonsLeft(bar)).Top(0).Bottom(0).AlignItems(ui.Center).Gap(2).Opacity(fade).Children(func() {
		a.leftButtons(c, pal, "-2")
	})
}

// Each of the window's sides has the buttons of what stands at it: the
// sessions' sidebar's, which shows and hides it and starts a session; or
// the side panel's, which shows and hides it. They stay at the window's
// top corner as what they show comes and goes, in the sidebar or panel
// while it shows, else in the header there. key tells their copies apart.

// leftButtons are the buttons of the window's top left.
func (a *App) leftButtons(c *ui.Context, pal *palette, key string) {
	if a.chrome.right {
		a.panelButton(c, pal, key)
		return
	}
	a.sessionButtons(c, pal, key)
}

// rightButtons are the buttons of the window's top right.
func (a *App) rightButtons(c *ui.Context, pal *palette, key string) {
	if a.chrome.right {
		a.sessionButtons(c, pal, key)
		return
	}
	a.panelButton(c, pal, key)
}

// rightButtonsW is the width of the buttons of the window's top right,
// with the gap after them.
func (a *App) rightButtonsW() float32 {
	if a.chrome.right {
		return sideButtonsW(2)
	}
	return sideButtonsW(1)
}

// sideButtonsW is the width of n of the sides' buttons, with the gap
// after them.
func sideButtonsW(n float32) float32 { return n * (controlW + 2) }

// edgeControls are what the header at the window's top right ends with:
// its side's buttons, then the window's controls. One row, so that they
// stand where they do whatever the gap of the header they are in: the
// panel's toggle stays put as the panel opens.
func (a *App) edgeControls(c *ui.Context, pal *palette, key string) {
	ui.Row(c.Key("edge")).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		a.rightButtons(c, pal, key)
		windowControls(c, pal, a.win)
	})
}

// sessionButtons show and hide the sessions' sidebar, at the window's
// corner, and start a session.
func (a *App) sessionButtons(c *ui.Context, pal *palette, key string) {
	newSession := func() {
		iconButton(c, "new-session"+key, plusIcon, a.tip(locales.ActionNewSession(), "session.new"), pal, func() { a.newSession("") })
	}
	svg := sidebarIcon
	if a.chrome.right {
		svg = panelIcon
		newSession()
	}
	tip := locales.SidebarHide()
	if a.prefs.SidebarHidden {
		tip = locales.SidebarShow()
	}
	iconButton(c, "sidebar"+key, svg, a.tip(tip, "sidebar.toggle"), pal, a.toggleSidebar)
	if !a.chrome.right {
		newSession()
	}
}

// panelButton shows and hides the side panel, with the view it showed
// last.
func (a *App) panelButton(c *ui.Context, pal *palette, key string) {
	svg := panelIcon
	if a.chrome.right {
		svg = sidebarIcon
	}
	view := a.sideView()
	tip := locales.SideShow()
	if a.prefs.Side != "" {
		tip = locales.SideHide()
	}
	iconButton(c, "panel"+key, svg, a.tip(tip, sideAction(view)), pal, func() { a.defer_(func() { a.toggleSide(view) }) })
}

// shortTitle lets a title shrink only as far as its ellipsis still saves
// room: a short one, as "pwsh", stays whole, as cut it shows as "pws…", as
// wide; a longer one keeps a few letters, which tell it.
func shortTitle(name ui.Element, title string) {
	const short = 8 // letters an ellipsis can't shorten by much
	if utf8.RuneCountInString(title) <= short {
		name.Shrink(0)
		return
	}
	name.Shrink(1).MinWidth(56)
}

// paneRoom is how a pane's header fits its width: whether its controls
// fold into a menu, the width they take as they show, and the room left
// for its git chip.
type paneRoom struct {
	fold     bool
	controls float32
	chip     float32
}

const (
	// titleRoom is what a header keeps for its title and folder, which give
	// up the rest of theirs as the header narrows.
	titleRoom = 140
	controlW  = 28 // an icon button's width
	// browserAddressRoom is what a browser's header keeps for its address.
	browserAddressRoom = 200
	// windowControlsW is the width of the window's controls (windowControls),
	// with their margin.
	windowControlsW = 6 + 3*controlW + 2*2
)

// headerRoom measures a pane's header as it was last laid out: its card's
// width, less its padding, lead (what the header keeps for what leads it:
// its title, or a browser's buttons and address), the side's buttons and
// the window's controls (edgeControls), the status chips, and the controls, folded when
// they don't fit whole. The controls count while they hide too, so that
// the chip keeps its form as the pointer comes and goes.
func (a *App) headerRoom(c *ui.Context, p *proto.Pane, pad, lead float32, controls, multi bool) paneRoom {
	w := a.paneRef("card-" + p.ID).Bounds(c).W
	if w <= 0 {
		// Not laid out yet: as wide as it may be.
		return paneRoom{chip: math.MaxFloat32}
	}
	const gap = 8
	used := pad + 10 + lead
	if controls {
		used += a.rightButtonsW() + gap
		if ownControls && a.win != nil {
			used += windowControlsW
		}
	}
	if b := a.paneRef("chips-" + p.ID).Bounds(c).W; b > 0 {
		used += b + gap
	}
	full, folded := float32(2), float32(1)
	if multi {
		full, folded = 4, 3
	}
	var r paneRoom
	r.controls = full * controlW
	if w-used < full*controlW+gap {
		r.fold, r.controls = true, folded*controlW
	}
	used += r.controls + gap
	r.chip = w - used - gap
	return r
}

// paneControls are a header's buttons acting on its pane, which show while
// the pointer is over it. Folded, those that make panes go in a menu, and
// zooming and closing stay.
func (a *App) paneControls(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane, show, multi, fold bool) {
	// The item chosen of a button's menu reaches it only as the button is
	// built again, which the pointer, over the menu, may have left.
	show = show || a.menuPane == p.ID
	ctl := ui.Row(c.Key("controls")).Gap(0).AlignItems(ui.Center).Bind(a.paneRef("ctl-" + p.ID))
	ctl.Opacity(ctl.Animate("shown", b2f(show), 120*time.Millisecond)).Children(func() {
		if !show {
			return
		}
		if fold {
			a.moreControls(c, pal, s, p)
		} else {
			a.splitButton(c, pal, s, p, "right")
			a.splitButton(c, pal, s, p, "down")
		}
		if multi {
			sid, pid := s.ID, p.ID
			zoom := func() { a.defer_(func() { a.zoomPane(a.session(sid), pid) }) }
			if s.Zoomed == p.ID {
				iconButton(c, "zoom", unzoomIcon, a.tip(locales.PaneShowAll(), "pane.zoom"), pal, zoom)
			} else {
				iconButton(c, "zoom", zoomIcon, a.tip(locales.ActionZoomPane(), "pane.zoom"), pal, zoom)
			}
			iconButton(c, "close", closeIcon, a.tip(locales.PaneClose(), "pane.close"), pal, func() { a.defer_(func() { a.closePane(a.session(sid), pid, true) }) })
		}
	})
}

// splitButton is a header's button that splits its pane, right or down,
// with a terminal; its menu, on a right click, with a browser or a file.
func (a *App) splitButton(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane, dir string) {
	sid, pid := s.ID, p.ID
	svg, title, action := splitRightIcon, locales.ActionSplitRight(), "pane.split.right"
	if dir == "down" {
		svg, title, action = splitDownIcon, locales.ActionSplitDown(), "pane.split.down"
	}
	b := ui.ButtonBase(c.Key("split-"+dir)).Size(controlW, 26).Radius(6).Center().TextColor(pal.textMuted).Label(title).Tooltip(a.tip(title, action))
	if b.Pressed() {
		b.Background(pal.rowHover.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, svg).Size(16, 16) })
	b.ContextMenu(func(m *ui.Menu) { a.splitMenu(m, sid, pid, dir, action) })
	if b.Clicked() {
		a.splitPane(s, pid, dir)
	}
}

// splitMenu is the menu of what a split of a pane, right or down, puts
// beside it: a terminal, which action's shortcut makes; a browser; or a
// new file, in the pane's folder, which its first save makes.
func (a *App) splitMenu(m *ui.Menu, sid, pid, dir, action string) {
	a.menuPane = pid
	split := func(p proto.CreateParams) {
		a.menuPane = ""
		if s := a.session(sid); s != nil {
			p.Split, p.Direction = pid, dir
			a.split(s, p)
		}
	}
	if a.menuShortcut(padItem(m, locales.SplitTerminal()), action).Chosen() {
		split(proto.CreateParams{})
	}
	if padItem(m, locales.SplitBrowser()).Chosen() {
		split(proto.CreateParams{URL: proto.BlankURL})
	}
	if padItem(m, locales.SplitFile()).Chosen() {
		a.menuPane = ""
		a.untitled(sid, pid, func(path string) { split(proto.CreateParams{File: path}) })
	}
}

// untitled finds a name for a new file in the folder of a pane, on its
// host: untitled.txt, else untitled-2.txt and on, none a file there or a
// pane's of the app shows; and gives its path to fn.
func (a *App) untitled(sid, pid string, fn func(path string)) {
	s := a.session(sid)
	cl := a.hostOf(sid).cl
	if s == nil || cl == nil {
		return
	}
	dir := ""
	if p := s.PaneByID(pid); p != nil {
		dir = p.WorkDir()
	}
	if dir == "" {
		a.toast(locales.FilesNoFolder())
		return
	}
	open := map[string]bool{}
	for _, s := range a.st.Sessions {
		for _, p := range s.Panes {
			if p.Editor() {
				open[p.File] = true
			}
		}
	}
	go func() {
		var path string
		var err error
		for i := 1; i <= 100 && path == "" && err == nil; i++ {
			name := "untitled.txt"
			if i > 1 {
				name = fmt.Sprintf("untitled-%d.txt", i)
			}
			p := hostJoin(dir, name)
			var st proto.FSStat
			if err = cl.Call("fs.stat", proto.FSStatParams{Path: p}, &st); err == nil && !st.Exists && !open[p] {
				path = p
			}
		}
		a.update(func() {
			switch {
			case err != nil:
				a.toast(locales.FilesCantCreate(err.Error()))
			case path != "":
				fn(path)
			}
		})
	}()
}

// chooseFile asks the user for a file of this machine, with the system's
// dialog, and gives it to fn.
func (a *App) chooseFile(fn func(path string)) {
	win := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: locales.SplitFileTitle(), ButtonLabel: locales.CommonOpen()})
		if err != nil || len(paths) == 0 {
			return
		}
		a.update(func() { fn(paths[0]) })
	}()
}

// moreControls is the button of a narrow header's folded controls: a menu
// of what makes panes beside it, each a menu of what goes there.
func (a *App) moreControls(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane) {
	sid, pid := s.ID, p.ID
	b := ui.ButtonBase(c.Key("more")).Size(controlW, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.PaneMore()).Tooltip(locales.PaneMore())
	if b.Pressed() {
		b.Background(pal.rowHover.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, moreIcon).Size(16, 16) })
	b.Menu(func(m *ui.Menu) {
		a.menuPane = pid
		padSub(m, locales.ActionSplitRight(), func(m *ui.Menu) { a.splitMenu(m, sid, pid, "right", "pane.split.right") })
		padSub(m, locales.ActionSplitDown(), func(m *ui.Menu) { a.splitMenu(m, sid, pid, "down", "pane.split.down") })
	})
}

// paneChips says what the pane is doing. With several panes the title
// already leads with the status icon, so the chips go without (icon false).
func (a *App) paneChips(c *ui.Context, pal *palette, p *proto.Pane, icon bool) {
	switch p.Status {
	case proto.Waiting:
		msg := p.Message
		if msg == "" {
			msg = map[string]string{"permission": locales.PaneNeedsPermission(), "question": locales.PaneHasQuestion(), "auth": locales.PaneNeedsSignIn()}[p.Kind]
		}
		if msg == "" {
			msg = locales.StatusNeedsYou()
		}
		chip(c, pal, pal.attnSoft, pal.attention, func() {
			if icon {
				statusIcon(c, pal, p.Status, p.Unread, 11)
			}
			ui.Text(c, msg).FontSize(12).FontWeight(500).SingleLine().Ellipsis("…").MaxWidth(320)
		})
	case proto.Working:
		chip(c, pal, pal.chip, pal.textMuted, func() {
			if icon {
				statusIconP(c, pal, p.Status, false, p.Progress, 11)
			}
			label := ago(p.Since, c.Now())
			if p.Progress >= 0 {
				label = itoa(p.Progress) + "%"
			}
			if p.Message != "" {
				label = p.Message + " · " + label
			}
			ui.Text(c, label).FontSize(12).FontFeatures("tnum").SingleLine().Ellipsis("…").MaxWidth(320)
		})
	case proto.Failed:
		chip(c, pal, pal.danger.Alpha(0.1), pal.danger, func() {
			if icon {
				statusIcon(c, pal, p.Status, true, 11)
			}
			msg := p.Message
			if msg == "" {
				msg = locales.StatusFailed()
			}
			ui.Text(c, msg).FontSize(12).FontWeight(500).SingleLine().Ellipsis("…").MaxWidth(320)
		})
	case proto.Exited:
		chip(c, pal, pal.danger.Alpha(0.1), pal.danger, func() {
			ui.Text(c, locales.PaneExited(itoa(p.ExitCode))).FontSize(12).FontWeight(500).SingleLine()
		})
		if button(c, "restart-"+p.ID, locales.CommonRestart(), "", false, pal) {
			a.restartSession(p.ID)
		}
	case proto.Idle:
		if p.Message != "" {
			chip(c, pal, pal.chip, pal.textMuted, func() {
				if icon {
					statusIcon(c, pal, p.Status, p.Unread, 11)
				}
				ui.Text(c, p.Message).FontSize(12).SingleLine().Ellipsis("…").MaxWidth(320)
			})
		}
	}
	if p.Process != "" && p.Status != proto.Exited {
		chip(c, pal, pal.chip, pal.textMuted, func() {
			ui.Text(c, p.Process).FontSize(12).SingleLine()
		}).Tooltip(locales.PaneForegroundTip())
	}
}

func (a *App) paneTerminal(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane) {
	e := a.terms[p.ID]
	if e == nil {
		a.ensureTerm(p.ID)
		e = a.terms[p.ID]
	}
	// The terminal fills the card under the header: its own padding
	// takes the colors of the cells beside it, so that a full-screen
	// program's background reaches the card's edges.
	ui.Box(c.Key("body")).Grow(1).MinHeight(0).Children(func() {
		switch {
		case e != nil && e.term != nil:
			ref := a.paneRef(p.ID)
			terminal.View(c.Key("term"), e.term).Fill().Bind(ref)
			if link, isPath, at := e.term.HoveredLink(); isPath {
				a.imagePeek(p.ID, link, at, ref.Bounds(c))
			}
			if ref.Focused(c) && a.focusOf(s) != p.ID {
				a.setFocus(s, p.ID)
			}
			if a.find.pane == p.ID {
				a.findBar(c, pal, p, e.term)
			}
		case e == nil && a.hostOf(p.ID).remote() && a.hostOf(p.ID).conn != connReady:
			// Shown as it was, until its host is back.
			h := a.hostOf(p.ID)
			switch {
			case h.conn == connConnecting || h.busy != "":
				ui.Column(c).Fill().Center().Gap(10).Children(func() {
					ui.Spinner(c)
					ui.Text(c, locales.HostReconnectingTo(h.name)).FontSize(12.5).TextColor(pal.textFaint)
				})
			default:
				a.message(c, pal, a.hostProblem(h), h.err, locales.CommonTryAgain(), func() { a.reconnectHost(h) })
			}
		case e != nil && e.err != "":
			a.message(c, pal, locales.PaneCantShow(), e.err, locales.CommonTryAgain(), func() {
				a.dropTerm(p.ID)
				a.ensureTerm(p.ID)
			})
		default:
			// Attaching takes a moment for a long scrollback: a spinner
			// only once it is a noticeable one.
			if e != nil && time.Since(e.started) > 250*time.Millisecond {
				ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c).Label(locales.PaneAttaching()) })
			} else {
				c.After(260 * time.Millisecond)
			}
		}
	})
}

// --- pane actions ---

func (a *App) splitPane(s *proto.Session, at, direction string) {
	if s == nil {
		return
	}
	if at == "" {
		at = a.focusOf(s)
	}
	a.split(s, proto.CreateParams{Split: at, Direction: direction})
}

// split makes a pane beside another, as p asks, on the session's host,
// and puts the user in it.
func (a *App) split(s *proto.Session, p proto.CreateParams) {
	if s == nil {
		return
	}
	h := a.hostOf(s.ID)
	if pane := s.PaneByID(p.Split); pane != nil && pane.Terminal() {
		p.Cols, p.Rows = pane.Cols, pane.Rows
		if p.Direction == "down" || p.Direction == "up" {
			p.Rows = max(p.Rows/2, 4)
		} else {
			p.Cols = max(p.Cols/2, 10)
		}
	}
	cl := h.cl
	if cl == nil {
		return
	}
	go func() {
		var ns proto.Session
		err := cl.Call("split", p, &ns)
		a.update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			a.replaceSession(h, ns)
			if cur := a.session(ns.ID); cur != nil {
				if a.focus == nil {
					a.focus = map[string]string{}
				}
				a.focus[ns.ID] = ns.Focus
				a.ensureTerm(ns.Focus)
				a.touch(ns.Focus)
				if !a.isBrowser(ns.Focus) {
					// A browser takes the keyboard in its address bar, or
					// its page once it has one.
					a.paneRef(ns.Focus).Focus()
				}
			}
		})
	}()
}

// replaceSession puts a session as kerod answered into the state, before
// the state that has it comes.
func (a *App) replaceSession(h *host, ns proto.Session) {
	// Its new pane is the host's from now, as its terminal attaches there.
	a.own(h, ns)
	for i := range a.st.Sessions {
		if a.st.Sessions[i].ID == ns.ID {
			a.st.Sessions[i] = ns
			return
		}
	}
	a.addPending(h, ns)
}

func (a *App) closePane(s *proto.Session, pane string, ask bool) {
	if s == nil {
		return
	}
	// An editor's edits not saved go with it: the user says so.
	if f := a.editorUnsaved(pane); ask && f != nil {
		sid := s.ID
		a.confirm = &confirmation{
			open:    true,
			title:   locales.FilesConfirmCloseTitle(),
			detail:  locales.FilesConfirmCloseDetail(proto.DirName(f.path)),
			action:  locales.FilesDiscard(),
			destroy: true,
			run: func() {
				if s := a.session(sid); s != nil {
					a.closePane(s, pane, false)
				}
			},
		}
		return
	}
	if len(s.Panes) <= 1 {
		a.closeSession(s.ID, ask)
		return
	}
	p := s.PaneByID(pane)
	if p == nil {
		return
	}
	if ask && a.set.ConfirmClose && p.Status != proto.Exited && p.Process != "" {
		sid := s.ID
		a.confirm = &confirmation{
			open:    true,
			title:   locales.ConfirmClosePaneTitle(),
			detail:  locales.ConfirmClosePaneDetail(p.Process),
			action:  locales.PaneClose(),
			destroy: true,
			run: func() {
				if s := a.session(sid); s != nil {
					a.closePane(s, pane, false)
				}
			},
		}
		return
	}
	// The neighbor taking the room takes the focus, as kerod does.
	if a.focusOf(s) == pane && s.Layout != nil {
		for _, dir := range []string{"left", "up", "right", "down"} {
			if n := s.Layout.Neighbor(pane, dir); n != "" {
				a.focus[s.ID] = n
				break
			}
		}
	}
	s.Layout = s.Layout.Remove(pane)
	if s.Layout.Leaf() {
		s.Layout = nil
	}
	if s.Zoomed == pane {
		s.Zoomed = ""
	}
	for i := range s.Panes {
		if s.Panes[i].ID == pane {
			s.Panes = append(s.Panes[:i], s.Panes[i+1:]...)
			break
		}
	}
	a.dropTerm(pane)
	a.dropWeb(pane)
	delete(a.edits, pane)
	a.call(pane, "pane.close", proto.IDParams{ID: pane}, nil)
	a.focusTerminal()
}

func (a *App) zoomPane(s *proto.Session, pane string) {
	if s == nil || len(s.Panes) < 2 {
		return
	}
	if pane == "" {
		pane = a.focusOf(s)
	}
	if s.Zoomed == pane {
		s.Zoomed = ""
	} else {
		s.Zoomed = pane
		a.setFocus(s, pane)
	}
	a.call(s.ID, "zoom", proto.FocusParams{Session: s.ID, Pane: pane}, nil)
	a.paneRef(pane).Focus()
}

// focusDirection moves into the pane next to the focused one.
func (a *App) focusDirection(dir string) {
	s := a.session(a.selected)
	if s == nil || s.Layout == nil || s.Zoomed != "" {
		return
	}
	if n := s.Layout.Neighbor(a.focusOf(s), dir); n != "" {
		a.setFocus(s, n)
		a.paneRef(n).Focus()
	}
}

// cyclePane moves to the next or previous pane, in the layout's order.
func (a *App) cyclePane(d int) {
	s := a.session(a.selected)
	if s == nil || len(s.Panes) < 2 {
		return
	}
	order := s.Layout.Panes()
	i := 0
	for j, id := range order {
		if id == a.focusOf(s) {
			i = j
		}
	}
	n := order[(i+d+len(order))%len(order)]
	a.setFocus(s, n)
	a.paneRef(n).Focus()
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// defer_ runs fn once the view is built: for actions that change the
// panes or sessions being built.
func (a *App) defer_(fn func()) { a.later = append(a.later, fn) }

func chip(c *ui.Context, pal *palette, bg, fg ui.Color, fn func()) ui.Element {
	return ui.Row(c).Height(22).Padding(0, 9).Gap(6).Radius(11).AlignItems(ui.Center).Background(bg).TextColor(fg).Children(fn)
}

// closeFocused closes the pane the user is in, or the session with its
// last pane.
func (a *App) closeFocused() {
	if s := a.session(a.selected); s != nil {
		a.closePane(s, a.focusOf(s), true)
	}
}
