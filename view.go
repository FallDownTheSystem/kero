package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

const (
	// barMid is the line the title bar's contents center on: the traffic
	// lights, the sidebar's buttons, the panes' headers. topBar is the
	// sidebar's title bar, centered on it, and headerH a header of a card,
	// which starts cardGap down, centered on it too.
	barMid  = 30
	topBar  = 2 * barMid
	headerH = 2 * (barMid - cardGap)
	cardGap = 8 // around the card
	rowH    = 34
)

// chrome is where the sidebar's animation puts things, in whole pixels so
// that edges stay crisp. Each is rounded once, from one progress quantized
// to the sidebar's pixels, so that they move together: rounded apart,
// they would step at different frames, and a title, their sum, would
// shake by a pixel as the spring settles.
type chrome struct {
	shown     float32 // the sidebar's progress, 0 hidden to 1 shown
	sideW     float32 // the sidebar's width as shown
	mainPad   float32 // the left padding of the cards' column
	headerPad float32 // the left padding of the header at the left edge
}

// buttonsLeft places the sidebar's buttons that the header holds while
// the sidebar hides, in the header's coordinates, so that they stay where
// they are in the window, after the traffic lights, as the card slides.
func (ch chrome) buttonsLeft(bar ui.TitleBar) float32 {
	return bar.Left + cardGap - (ch.sideW + ch.mainPad)
}

var sidebarMotion = ui.Animation{Duration: 260 * time.Millisecond, Ease: ui.CubicBezier(0.25, 1, 0.5, 1)}

func chromeFor(progress, width float32, bar ui.TitleBar) chrome {
	round := func(v float32) float32 { return float32(math.Round(float64(v))) }
	w := round(width * min(max(progress, 0), 1))
	s := w / width
	pad := round(cardGap * (1 - s))
	// Where the header's title starts in the window, rounded once: after
	// the traffic lights and the sidebar's buttons with the sidebar
	// hidden, 16 into the card with it shown.
	hidden, shown := cardGap+16+bar.Left+60, width+16
	x := round(hidden + (shown-hidden)*s)
	return chrome{shown: s, sideW: w, mainPad: pad, headerPad: x - w - pad}
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (a *App) view(c *ui.Context) {
	pal := paletteFor(c)
	c.SetTheme(borderless(c, pal))

	if a.pendingToast != "" {
		c.Toast(a.pendingToast)
		a.pendingToast = ""
	}
	a.shortcuts(c)

	bar := c.TitleBar()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	backdrop(c, pal, row, a.set)
	// A curve, not a spring: a spring's long tail crawls under a pixel a
	// frame, which whole pixels turn into hops and holds, a shake as it
	// settles. This one lands in about 13 frames, moving in every one.
	progress := row.AnimateTo("sidebar", b2f(!a.prefs.SidebarHidden), sidebarMotion)
	a.chrome = chromeFor(progress, a.prefs.SidebarWidth, bar)
	shown := a.chrome.shown
	row.Children(func() {
		w := a.chrome.sideW
		side := ui.Row(c).Width(w).Shrink(0).AlignItems(ui.Stretch).ClipX()
		side.Children(func() {
			ui.Column(c).Width(a.prefs.SidebarWidth).Shrink(0).Margin(0, 0, 0, w-a.prefs.SidebarWidth).
				Opacity(0.4 + 0.6*shown).Children(func() { a.sidebar(c, pal, bar) })
		})
		// The gaps around the cards move the window too.
		main := ui.Column(c).Grow(1).MinWidth(0).Padding(cardGap, cardGap, cardGap, a.chrome.mainPad).DragWindow()
		main.Children(func() {
			if shown > 0.01 {
				a.resizeHandle(c, pal)
			}
			a.card(c, pal, bar, shown)
		})
	})
	a.paletteView(c, pal)
	a.confirmView(c, pal)
	// A preview that showed no more fades in anew next time.
	if !a.previewShown {
		a.previewKey = ""
	}
	a.previewShown = false
	if later := a.later; len(later) > 0 {
		a.later = nil
		for _, fn := range later {
			fn()
		}
		c.Invalidate()
	}
}

// resizeHandle is the sidebar's edge, which a drag moves.
func (a *App) resizeHandle(c *ui.Context, pal *palette) {
	h := ui.Box(c.Key("resize")).Absolute().Left(-4).Top(0).Bottom(0).Width(9).Cursor(ui.CursorResizeColumn)
	if dx, _, ok := h.Dragged(); ok {
		if a.sideDrag == 0 {
			a.sideDrag = a.prefs.SidebarWidth
		}
		a.sideDrag += dx
		a.prefs.SidebarWidth = min(max(a.sideDrag, minSidebar), maxSidebar)
	} else if a.sideDrag != 0 && !h.Pressed() {
		a.sideDrag = 0
		a.savePrefs()
	}
	if h.DoubleClicked() {
		a.prefs.SidebarWidth = defaultSidebar
		a.savePrefs()
	}
}

// iconButton is a quiet square button with an icon, as in a toolbar.
func iconButton(c *ui.Context, key string, svg *ui.SVG, label string, pal *palette, fn func()) {
	b := ui.ButtonBase(c.Key(key)).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).Tooltip(label)
	if b.Pressed() {
		b.Background(pal.rowHover.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal.rowHover)
		b.TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, svg).Size(16, 16) })
	if b.Clicked() {
		fn()
	}
}

func kbd(c *ui.Context, pal *palette, keys string) {
	ui.Text(c, keys).FontSize(11).TextColor(pal.textMuted).Padding(1, 5).Radius(4).
		Background(pal.kbd).SingleLine()
}

// --- the sidebar ---

func (a *App) sidebar(c *ui.Context, pal *palette, bar ui.TitleBar) {
	ui.Row(c).Height(topBar).Shrink(0).AlignItems(ui.Center).Padding(0, 12, 0, bar.Left+8).Gap(2).DragWindow().Children(func() {
		ui.Spacer(c)
		iconButton(c, "hide-sidebar", sidebarIcon, "Hide Sidebar (⌘B)", pal, a.toggleSidebar)
		iconButton(c, "new-session", plusIcon, "New Session (⌘T)", pal, func() { a.newSession("") })
	})
	ui.Scroll(c.Key("sessions")).Grow(1).Padding(0, 10, 16, 10).Children(func() {
		if !a.loaded {
			return
		}
		n := 0 // the rows shown, for ⌘1–9
		for gi, g := range a.groups() {
			a.groupView(c, pal, g, gi, &n)
		}
		if len(a.st.Sessions) == 0 && a.conn == connReady {
			ui.Text(c, "No sessions").FontSize(12.5).TextColor(pal.textFaint).Padding(8, 10)
		}
	})
	a.sidebarFooter(c, pal)
	c.After(20 * time.Second) // times ago move on
}

func (a *App) groupView(c *ui.Context, pal *palette, g group, gi int, n *int) {
	id := g.project.ID
	collapsed := a.prefs.Collapsed[id]
	needs := 0
	for _, s := range g.sessions {
		if s.Status == proto.Waiting {
			needs++
		}
	}
	top := float32(18)
	if gi == 0 {
		top = 2
	}
	hdr := ui.Row(c.Key("project-"+id)).Height(28).Margin(top, 0, 3, 0).Padding(0, 6, 0, 10).Gap(6).AlignItems(ui.Center).Radius(6)
	hovered := hdr.Hovered()
	if hdr.Clicked() {
		a.toggleProject(id)
	}
	hdr.Tooltip(proto.ShortDir(g.project.Dir))
	hdr.ContextMenu(func(m *ui.Menu) {
		if m.Item("New Session").Chosen() {
			a.newSession(id)
		}
		label := "Collapse"
		if collapsed {
			label = "Expand"
		}
		if m.Item(label).Chosen() {
			a.toggleProject(id)
		}
		m.Separator()
		if m.Item("Close Project…").Disabled(id == "").Chosen() {
			name, count := g.project.Name, len(g.sessions)
			a.confirm = &confirmation{
				open:    true,
				title:   fmt.Sprintf("Close “%s”?", name),
				detail:  fmt.Sprintf("This ends its %d sessions, with what runs in them.", count),
				action:  "Close Project",
				destroy: true,
				run:     func() { a.call("project.close", proto.IDParams{ID: id}, nil) },
			}
		}
	})
	hdr.Children(func() {
		ui.Text(c, g.project.Name).FontSize(12.5).FontWeight(600).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		ui.Textf(c, "%d", len(g.sessions)).FontSize(12.5).TextColor(pal.textFaint).FontFeatures("tnum")
		chev := ui.Icon(c, chevronIcon).Size(11, 11).TextColor(pal.textFaint)
		turn := chev.AnimateTo("turn", 90*(1-b2f(collapsed)), ui.Snappy)
		chev.Rotate(turn).Opacity(chev.Animate("shown", b2f(hovered || collapsed), 120*time.Millisecond))
		ui.Spacer(c)
		if needs > 0 {
			ui.Textf(c, "%d needs you", needs).FontSize(12.5).FontWeight(500).TextColor(pal.attention).SingleLine()
		}
		if hovered {
			b := ui.ButtonBase(c.Key("add")).Size(20, 20).Radius(5).Center().TextColor(pal.textMuted).Label("New Session in " + g.project.Name).Tooltip("New Session")
			if b.Hovered() {
				b.Background(pal.rowHover).TextColor(pal.text)
			}
			b.Children(func() { ui.Icon(c, plusIcon).Size(13, 13) })
			if b.Clicked() {
				a.newSession(id)
			}
		}
	})
	if collapsed {
		// The session shown stays in view.
		for _, s := range g.sessions {
			if s.ID == a.selected {
				a.sessionRow(c, pal, s, n)
			}
		}
		return
	}
	for _, s := range g.sessions {
		a.sessionRow(c, pal, s, n)
	}
}

func (a *App) sessionRow(c *ui.Context, pal *palette, s *proto.Session, n *int) {
	*n++
	index := *n
	selected := s.ID == a.selected
	row := ui.Row(c.Key(s.ID)).Height(rowH).Padding(0, 10, 0, 12).Gap(10).AlignItems(ui.Center).Radius(8).Margin(0, 0, 1, 0)
	hovered := row.Hovered()
	if pal.dark {
		// Room for the chosen row's edge in every row, so that choosing
		// one moves no text.
		row.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	switch {
	case selected:
		bg = pal.rowSelected
		raised(row, pal) // as the cards: a soft shadow, a lit edge in the dark
	case hovered:
		bg = pal.rowHover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if (row.Pressed() || row.Clicked()) && !selected {
		a.selectSession(s.ID)
	}
	if row.DoubleClicked() {
		a.startRename(s.ID)
	}
	tip := proto.ShortDir(s.Dir)
	if s.Process != "" {
		tip = s.Process + " · " + tip
	}
	if s.Message != "" {
		tip = s.Message + "\n" + tip
	}
	row.Tooltip(tip)
	row.ContextMenu(func(m *ui.Menu) { a.sessionMenu(m, s) })
	row.Children(func() {
		if a.renaming == s.ID {
			in := ui.TextInputBase(c.Key("rename"), &a.renameBuf).Grow(1).MinWidth(0).FontSize(13.5).AutoFocus()
			if in.Submitted() || in.Shortcut(0, ui.KeyEscape) {
				if in.Shortcut(0, ui.KeyEscape) {
					a.renaming = ""
					a.focusTerminal()
				} else {
					a.commitRename()
				}
			} else if !in.Focused() && !in.Pressed() && in.Valid() && a.renameStarted(c) {
				a.commitRename()
			}
			return
		}
		name := ui.Text(c, s.DisplayName()).FontSize(13.5).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).TextColor(pal.text)
		if s.Unread {
			name.FontWeight(650)
		}
		if s.Status == proto.Exited && !s.Unread {
			name.TextColor(pal.textMuted)
		}
		cmd := c.Modifiers() == ui.Super
		if cmd && index <= 9 {
			ui.Textf(c, "⌘%d", index).FontSize(12).TextColor(pal.textMuted).FontFeatures("tnum").SingleLine()
		} else {
			ui.Text(c, proto.Ago(s.Since, c.Now())).FontSize(12.5).TextColor(pal.textFaint).FontFeatures("tnum").SingleLine()
		}
		statusIconP(c, pal, s.Status, s.Unread, s.Progress, 15)
	})
}

// renameStarted keeps a rename from ending in the frame it began in,
// before its input took the focus.
func (a *App) renameStarted(c *ui.Context) bool {
	if a.renameFrames < 3 {
		a.renameFrames++
		c.Invalidate()
		return false
	}
	return true
}

func (a *App) sessionMenu(m *ui.Menu, s *proto.Session) {
	id := s.ID
	if m.Item("Rename").Shortcut(ui.Super, ui.KeyR).Chosen() {
		a.startRename(id)
	}
	if s.Status == proto.Exited {
		if m.Item("Restart").Chosen() {
			a.restartSession(id)
		}
	}
	if s.Unread {
		if m.Item("Mark as Read").Chosen() {
			a.call("view", proto.ViewParams{ID: id}, func(error) { a.viewSent = nil; a.reportView() })
		}
	}
	m.Submenu("Move to Project", func(m *ui.Menu) {
		for _, p := range a.st.Projects {
			if m.Item(p.Name).Checked(p.ID == s.Project).Disabled(p.ID == s.Project).Chosen() {
				pid := p.ID
				a.defer_(func() { a.moveSession(id, pid) })
			}
		}
	})
	m.Separator()
	if m.Item("Copy Session ID").Chosen() {
		a.copyText = id
	}
	if m.Item("Copy Directory").Disabled(s.Dir == "").Chosen() {
		a.copyText = s.Dir
	}
	if m.Item("Reveal in Finder").Disabled(s.Dir == "").Chosen() {
		a.openLink(id, s.Dir)
	}
	m.Separator()
	if m.Item("Close Session").Shortcut(ui.Super|ui.Shift, ui.KeyW).Chosen() {
		a.defer_(func() { a.closeSession(id, true) })
	}
}

func (a *App) sidebarFooter(c *ui.Context, pal *palette) {
	ui.Row(c).Height(36).Shrink(0).Padding(0, 12, 4, 18).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
		var label string
		dot := pal.textFaint
		switch a.conn {
		case connReady:
			running := 0
			for _, s := range a.st.Sessions {
				if s.Status != proto.Exited {
					running++
				}
			}
			dot = ui.Hex("#34c759")
			label = fmt.Sprintf("%d running", running)
			if running == 1 {
				label = "1 running"
			}
		case connConnecting:
			label = "Connecting…"
		default:
			dot = pal.danger
			label = "Disconnected"
		}
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, label).FontSize(12).TextColor(pal.textFaint).SingleLine().
			Tooltip("Sessions run in kerod, in the background: they keep running when you quit kero.")
		ui.Spacer(c)
		iconButton(c, "open-project", folderIcon, "Open Project… (⌘O)", pal, a.openProject)
		iconButton(c, "settings", gearIcon, "Settings (⌘,)", pal, func() { a.openSettings("") })
		iconButton(c, "palette", searchIcon, "Go to Session… (⌘P)", pal, a.openPalette)
	})
}

// statusIcon draws a session's status, as the sidebar shows it: a ring
// turning while it works, an amber ring when it needs the user, a filled
// check when it finished unseen, a faint check once seen.
func statusIcon(c *ui.Context, pal *palette, st proto.Status, unread bool, size float32) ui.Element {
	return statusIconP(c, pal, st, unread, -1, size)
}

// statusIconP is statusIcon with a program's progress (OSC 7501), 0–100,
// which a working ring fills instead of turning; -1 for none.
func statusIconP(c *ui.Context, pal *palette, st proto.Status, unread bool, progress int, size float32) ui.Element {
	box := ui.Box(c).Size(size, size).Shrink(0)
	switch {
	case st == proto.Working:
		box.Label("Working")
	case st == proto.Waiting:
		box.Label("Needs you")
	case st == proto.Exited:
		box.Label("Exited")
	case st == proto.Failed:
		box.Label("Failed")
	case unread:
		box.Label("Done")
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		rad := min(r.W, r.H)/2 - 1
		switch {
		case st == proto.Working && progress >= 0:
			var track ui.Path
			track.Circle(cx, cy, rad)
			p.StrokePath(&track, 1.7, pal.text.Alpha(0.13))
			// From the top, clockwise: the circle starts at 3 o'clock.
			p.Transform(ui.Rotation(-90), cx, cy, func() {
				var arc ui.Path
				arc.Circle(cx, cy, rad)
				p.StrokePath(arc.Trim(0, max(0.02, float32(progress)/100)), 1.7, pal.text)
			})
		case st == proto.Working:
			var track ui.Path
			track.Circle(cx, cy, rad)
			p.StrokePath(&track, 1.7, pal.text.Alpha(0.13))
			p.AnimationFrame()
			deg := float32(p.Now().UnixMilli()%1000) / 1000 * 360
			p.Transform(ui.Rotation(deg), cx, cy, func() {
				var arc ui.Path
				arc.Circle(cx, cy, rad)
				p.StrokePath(arc.Trim(0, 0.3), 1.7, pal.text)
			})
		case st == proto.Waiting:
			var ring ui.Path
			ring.Circle(cx, cy, rad)
			p.StrokePath(&ring, 1.7, pal.attention)
			p.AnimationFrame()
			pulse := float32(0.5 + 0.5*math.Sin(float64(p.Now().UnixMilli()%1600)/1600*2*math.Pi))
			var dot ui.Path
			dot.Circle(cx, cy, rad*0.42)
			p.FillPath(&dot, pal.attention.Alpha(0.65+0.35*pulse))
		case st == proto.Failed:
			var disc ui.Path
			disc.Circle(cx, cy, rad+0.5)
			col := pal.danger
			if !unread {
				col = pal.danger.Alpha(0.75)
			}
			p.FillPath(&disc, col)
			p.Line(cx, cy-rad*0.45, cx, cy+rad*0.08, 1.7, pal.onDone)
			var dot ui.Path
			dot.Circle(cx, cy+rad*0.42, 1)
			p.FillPath(&dot, pal.onDone)
		case st == proto.Exited:
			col := pal.textFaint
			if unread {
				col = pal.danger
			}
			var ring ui.Path
			ring.Circle(cx, cy, rad)
			p.StrokePath(&ring, 1.5, col)
			k := rad * 0.38
			p.Line(cx-k, cy-k, cx+k, cy+k, 1.5, col)
			p.Line(cx+k, cy-k, cx-k, cy+k, 1.5, col)
		case unread:
			var disc ui.Path
			disc.Circle(cx, cy, rad+0.5)
			p.FillPath(&disc, pal.done)
			check := (&ui.Path{}).MoveTo(cx-rad*0.42, cy+0.2).LineTo(cx-rad*0.08, cy+rad*0.36).LineTo(cx+rad*0.46, cy-rad*0.34)
			p.StrokePath(check, 1.6, pal.onDone)
		default:
			check := (&ui.Path{}).MoveTo(cx-rad*0.62, cy+0.4).LineTo(cx-rad*0.12, cy+rad*0.5).LineTo(cx+rad*0.66, cy-rad*0.46)
			p.StrokePath(check, 1.5, pal.textFaint)
		}
	})
	return box
}

// --- the card ---

func (a *App) card(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32) {
	s := a.session(a.selected)
	if s != nil && a.conn != connVersion && a.loaded {
		a.panes(c, pal, bar, shown, s)
		return
	}
	// No session: one card, for what the app has to say.
	card := raised(ui.Column(c).Grow(1).MinHeight(0).Radius(14).Background(pal.card), pal).Clip()
	card.Children(func() {
		left := a.chrome.headerPad
		ui.Row(c).Height(headerH).Shrink(0).AlignItems(ui.Center).Padding(0, 12, 0, left).DragWindow().Children(func() {
			if shown < 0.5 {
				ui.Row(c).Absolute().Left(a.chrome.buttonsLeft(bar)).Top(0).Bottom(0).AlignItems(ui.Center).Gap(2).Opacity(1 - 2*shown).Children(func() {
					iconButton(c, "show-sidebar", sidebarIcon, "Show Sidebar (⌘B)", pal, a.toggleSidebar)
					iconButton(c, "new-session-2", plusIcon, "New Session (⌘T)", pal, func() { a.newSession("") })
				})
			}
			ui.Text(c, "kero").FontSize(14).FontWeight(600).TextColor(pal.text)
		})
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			switch {
			case a.conn == connVersion:
				a.message(c, pal, "kero was updated",
					"The background service that runs your sessions is from another version. Restart it: your sessions start again where they were, with their history.",
					"Restart Service", a.restartDaemon)
			case a.conn == connFailed && a.cl == nil:
				a.message(c, pal, "Can’t reach kerod", a.connErr, "Try Again", func() { a.retries = 0; a.conn = connConnecting; a.connect() })
			case !a.loaded:
				ui.Column(c).Fill().Center().Children(func() {
					ui.Spinner(c).Label("Connecting")
				})
			default:
				a.emptyState(c, pal)
			}
		})
	})
}

func (a *App) message(c *ui.Context, pal *palette, title, detail, action string, fn func()) {
	ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(17).FontWeight(600).TextColor(pal.text)
		ui.Text(c, detail).FontSize(13).TextColor(pal.textMuted).MaxWidth(420).TextAlign(ui.Center).LineHeight(1.4)
		if action != "" {
			ui.Box(c).Height(8)
			if button(c, "msg-action", action, "", true, pal) {
				fn()
			}
		}
	})
}

func (a *App) emptyState(c *ui.Context, pal *palette) {
	ui.Column(c).Fill().Center().Gap(0).Padding(24).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(pal.chip).TextColor(pal.textMuted).Margin(0, 0, 18, 0).Children(func() {
			ui.Icon(c, terminalIcon).Size(26, 26)
		})
		ui.Text(c, "Start a session").FontSize(19).FontWeight(650).TextColor(pal.text)
		ui.Text(c, "Sessions keep running in the background — quit kero and come back to everything just as you left it.").
			FontSize(13.5).TextColor(pal.textMuted).MaxWidth(380).TextAlign(ui.Center).LineHeight(1.45).Margin(8, 0, 22, 0)
		ui.Row(c).Gap(10).Children(func() {
			if button(c, "empty-new", "New Session", "⌘T", true, pal) {
				a.newSession("")
			}
			if button(c, "empty-open", "Open Project…", "⌘O", false, pal) {
				a.openProject()
			}
		})
		ui.Column(c).Margin(40, 0, 0, 0).Padding(14, 16).Gap(6).Radius(10).Background(pal.chip).MaxWidth(420).Children(func() {
			ui.Text(c, "For agents").FontSize(11.5).FontWeight(600).TextColor(pal.textMuted)
			ui.Text(c, "kero new -n review -- claude\nkero prompt review \"run the tests and fix them\"\nkero wait review && kero read review").
				Font("monospace").FontSize(12).TextColor(pal.text).LineHeight(1.5).Selectable()
		})
	})
}
