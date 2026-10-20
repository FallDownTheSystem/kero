package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"kero/internal/actions"
	"kero/internal/cli"
	"kero/internal/client"
	"kero/internal/proto"
	"kero/internal/settings"
	"kero/internal/shell"
)

// settingsPage is a page of the Settings window.
type settingsPage struct {
	id, title string
	icon      *ui.SVG
}

var settingsPages = []settingsPage{
	{"general", "General", glyph("f7-gear-alt")},
	{"appearance", "Appearance", glyph("f7-paintbrush")},
	{"terminal", "Terminal", terminalIcon},
	{"shell", "Shell", glyph("f7-chevron-left-slash-chevron-right")},
	{"notifications", "Notifications", glyph("f7-bell")},
	{"keyboard", "Keyboard", glyph("f7-keyboard")},
	{"agents", "Agents", glyph("f7-sparkles")},
	{"advanced", "Advanced", glyph("f7-slider-horizontal-3")},
}

// settingsState is what the Settings window keeps besides the settings.
type settingsState struct {
	page         string
	recording    string // the action whose shortcut is being typed
	recordRef    ui.Handle
	filter       string // the keyboard page's search
	query        string // the sidebar's search, of every page
	searchRef    ui.Handle
	revealed     string // the query and page the search last scrolled to
	draft        map[string]string
	envDraft     string
	shellDraft   string
	shellEditing bool
	loginOpen    bool // open at login, as the system says
	cliPath      string
	service      string // kerod, as last asked
	notice       string // a line under the keyboard page, as of a conflict
	toast        string
	menus        map[string]*searchMenu     // the search menus', by key
	problems     []string                   // what the configuration file got wrong
	scrolls      map[string]*ui.ScrollState // each page's, for its scroll edge
}

// openSettings shows the Settings window, on a page.
func (a *App) openSettings(page string) {
	if page != "" {
		a.ss.page = page
	}
	if a.ss.page == "" {
		a.ss.page = "general"
	}
	if a.setWin != nil {
		a.setWin.Show()
		a.setWin.Focus()
		return
	}
	a.ss.loginOpen = mygo.App.OpenAtLogin()
	a.ss.draft = map[string]string{}
	a.refreshAgents()
	a.refreshService()
	a.setWin = mygo.NewWindow(framed(mygo.WindowOptions{
		Title: "Settings", Width: 820, Height: 620, MinWidth: 700, MinHeight: 480,
		StateKey: "settings",
		Vibrancy: vibrancyFor(a.set),
		Content:  ui.View(a.settingsView),
	}))
	a.setWin.OnClosed(func() {
		a.setWin = nil
		a.setRecording("") // the menus take their shortcuts back
		a.ss.query = ""
	})
}

func (a *App) refreshAgents() {
	a.ss.cliPath = cliInstalled()
}

// refreshService asks kerod how it is, off the main thread.
func (a *App) refreshService() {
	go func() {
		line := "Not running"
		if c, err := client.Connect(true); err == nil {
			var st proto.State
			c.Call("state", nil, &st)
			panes := 0
			for _, s := range st.Sessions {
				panes += len(s.Panes)
			}
			up := time.Since(time.UnixMilli(c.Welcome.Started)).Round(time.Minute)
			line = fmt.Sprintf("Running (pid %d) for %s · %d sessions, %d panes", c.Welcome.PID, durationWords(up), len(st.Sessions), panes)
			c.Close()
		}
		a.update(func() {
			a.ss.service = line
			if a.setWin != nil {
				a.setWin.Invalidate()
			}
		})
	}()
}

func durationWords(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%.0f h", d.Hours())
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

func (a *App) settingsView(c *ui.Context) {
	pal := paletteFor(c)
	c.SetTheme(borderless(c, pal))
	if a.ss.toast != "" {
		c.Toast(a.ss.toast)
		a.ss.toast = ""
	}
	found := a.searchSettings()
	c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.ss.searchRef.Focus(c) })
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	backdrop(c, pal, row, a.set)
	row.Children(func() {
		// Under the traffic lights on macOS; elsewhere the search starts
		// the sidebar, centered on barMid as the page's title and the
		// window controls are.
		top := float32(topBar)
		if ownControls {
			top = barMid - (controlH+2)/2
		}
		ui.Column(c).Width(212).Shrink(0).Padding(top, 10, 12, 10).Gap(2).DragWindow().Children(func() {
			a.settingsSearch(c, pal)
			for _, p := range settingsPages {
				if found == nil || found[p.id] != nil {
					a.settingsNav(c, pal, p)
				}
			}
			if found != nil && len(found) == 0 {
				ui.Text(c, "No Results").FontSize(13).TextColor(pal.textFaint).Padding(8, 10)
			}
			ui.Spacer(c)
			link := ui.ButtonBase(c.Key("open-config")).Padding(6, 10).Gap(6).Radius(7).AlignItems(ui.Center).Label("Open the config file")
			fg := pal.textMuted
			if link.Hovered() {
				fg = pal.text
				link.Background(pal.rowHover)
			}
			link.TextColor(fg).Tooltip(proto.ShortDir(settings.Path())).Children(func() {
				ui.Icon(c, glyph("f7-chevron-left-slash-chevron-right")).Size(13, 13)
				ui.Text(c, "Open config file").FontSize(12.5).SingleLine()
			})
			if link.Clicked() {
				a.openConfig()
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Padding(cardGap, cardGap, cardGap, 0).Children(func() {
			card := raised(ui.Column(c).Grow(1).MinHeight(0).Radius(14).Background(pal.card), pal).Clip()
			card.Children(func() {
				page := settingsPages[0]
				for _, p := range settingsPages {
					if p.id == a.ss.page {
						page = p
					}
				}
				// The page scrolls under its header, which macOS 27's hard
				// scroll edge frosts it under, as System Settings does.
				// The edge shows, as AppKit's, once content is under the bar,
				// or while the pointer is over it.
				if a.ss.scrolls == nil {
					a.ss.scrolls = map[string]*ui.ScrollState{}
				}
				sc := a.ss.scrolls[page.id]
				if sc == nil {
					sc = &ui.ScrollState{}
					a.ss.scrolls[page.id] = sc
				}
				box := ui.Box(c.Key("page")).Grow(1).MinHeight(0)
				_, py, over := box.PointerPosition()
				edgeOn := sc.Y > 0.5 || over && py < headerH
				box.Children(func() {
					ui.Scroll(c.Key("page-"+page.id)).Fill().Padding(headerH+16, 24, 28, 24).ScrollbarInsets(headerH, 0, 0).TrackScroll(sc).Children(func() {
						ui.Column(c).Gap(22).MaxWidth(620).Children(func() {
							switch page.id {
							case "general":
								a.generalPage(c, pal)
							case "appearance":
								a.appearancePage(c, pal)
							case "terminal":
								a.terminalPage(c, pal)
							case "shell":
								a.shellPage(c, pal)
							case "notifications":
								a.notificationsPage(c, pal)
							case "keyboard":
								a.keyboardPage(c, pal)
							case "agents":
								a.agentsPage(c, pal)
							case "advanced":
								a.advancedPage(c, pal)
							}
						})
					})
					edge := ui.Box(c.Key("edge")).Absolute().Top(0).Left(0).Right(0).Height(headerH).PassThrough().
						Material(glass.ScrollEdge{Hard: true, Background: pal.card})
					edge.Opacity(edge.Animate("on", b2f(edgeOn), 160*time.Millisecond))
					ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(headerH).AlignItems(ui.Center).Padding(0, 10, 0, 24).DragWindow().Children(func() {
						ui.Text(c, page.title).FontSize(15).FontWeight(650).TextColor(pal.text)
						ui.Spacer(c)
						windowControls(c, pal, a.setWin)
					})
				})
			})
		})
	})
	// The controls edit a.set: what changed applies, and saves.
	if !reflect.DeepEqual(a.set, a.applied) {
		a.applySettings(a.set)
	}
}

// searchSettings applies the sidebar's search: it shows a page with
// finds, when the page shown has none, and sets what the pages highlight.
// It returns what searchResults does.
func (a *App) searchSettings() map[string][]string {
	found := searchResults(a.ss.query, a.set.Shortcuts)
	searchHits, searchReveal = nil, ""
	if found == nil {
		a.ss.revealed = ""
		return nil
	}
	if found[a.ss.page] == nil {
		for _, p := range settingsPages {
			if found[p.id] != nil {
				a.ss.page = p.id
				a.setRecording("")
				break
			}
		}
	}
	labels := found[a.ss.page]
	searchHits = map[string]bool{}
	for _, l := range labels {
		searchHits[l] = true
	}
	// The first find comes into view once for each query and page, after
	// which the page scrolls as the user likes.
	if at := a.ss.query + "\x00" + a.ss.page; len(labels) > 0 && a.ss.revealed != at {
		a.ss.revealed, searchReveal = at, labels[0]
	}
	return found
}

// settingsSearch is the sidebar's search field, which finds settings in
// every page by their labels, their keys in the configuration file, and
// words for them.
func (a *App) settingsSearch(c *ui.Context, pal *palette) {
	var in ui.Element
	frame := ui.Row(c.Key("search")).Cursor(ui.CursorText).Margin(0, 0, 10, 0)
	frame.Children(func() {
		ui.Icon(c, searchIcon).Size(13, 13).TextColor(pal.textFaint).Shrink(0)
		in = ui.TextInputBase(c.Key("in"), &a.ss.query).Bind(&a.ss.searchRef).Grow(1).MinWidth(0).FontSize(13).
			Placeholder("Search").Label("Search settings")
		in.OnShortcut(0, ui.KeyEscape, func() { a.ss.query = "" })
		if a.ss.query != "" {
			x := ui.ButtonBase(c.Key("clear")).Size(16, 16).Radius(8).Center().Shrink(0).Label("Clear the search").
				Background(pal.textFaint.Alpha(0.55)).TextColor(pal.card)
			if x.Hovered() {
				x.Background(pal.textMuted)
			}
			x.Children(func() { ui.Icon(c, closeIcon).Size(8, 8) })
			if x.Clicked() {
				a.ss.query = ""
				a.ss.searchRef.Focus(c)
			}
		}
	})
	field(c, frame, in.Focused(), pal)
}

func (a *App) settingsNav(c *ui.Context, pal *palette, p settingsPage) {
	sel := a.ss.page == p.id
	item := ui.Row(c.Key(p.id)).Height(32).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(8)
	if pal.dark {
		item.Border(1, ui.Transparent) // room for the chosen item's edge
	}
	bg := ui.Transparent
	switch {
	case sel:
		bg = pal.rowSelected
		chosen(item, pal)
	case item.Hovered():
		bg = pal.rowHover
	}
	item.Background(item.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if item.Pressed() && !sel {
		a.ss.page = p.id
		a.setRecording("")
	}
	item.Children(func() {
		ic := ui.Icon(c, p.icon).Size(15, 15).TextColor(pal.textMuted)
		if sel {
			ic.TextColor(pal.accent)
		}
		ui.Text(c, p.title).FontSize(13.5).TextColor(pal.text).SingleLine()
	})
}

// groupTitle is a group's title, which the search highlights as it does
// a setting.
func groupTitle(c *ui.Context, pal *palette, title string) {
	row := ui.Row(c.Key("title-"+title)).Padding(0, 4)
	row.Children(func() {
		searchHighlight(c, row, title, pal, 2, 3)
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(pal.textMuted)
	})
}

// group is a titled set of settings, on a soft panel.
func settingsGroup(c *ui.Context, pal *palette, title, note string, fn func()) {
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			groupTitle(c, pal, title)
		}
		if fn != nil { // a group of a note alone has no panel
			ui.Column(c).Radius(12).Background(pal.chip).Padding(4, 16).Children(fn)
		}
		if note != "" {
			ui.Text(c, note).FontSize(12).TextColor(pal.textFaint).LineHeight(1.4).Padding(0, 4)
		}
	})
}

// setting is a row of a group: a label, what it does, and its control.
func settingRow(c *ui.Context, pal *palette, label, detail string, control func()) {
	row := ui.Row(c.Key("row-"+label)).Padding(11, 0).Gap(16).AlignItems(ui.Center)
	row.Children(func() {
		searchHighlight(c, row, label, pal, 10, -2)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(13.5).TextColor(pal.text)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35)
			}
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

// segmented chooses one of ids into value, as macOS's segmented control
// shows it: a soft track, the choice a lighter fill raised a hair, thin
// separators between the others, and labels of one weight.
func segmented(c *ui.Context, key string, value *string, ids, labels []string) {
	pal := paletteFor(c)
	i := max(slices.Index(ids, *value), 0)
	track, thumb := ui.RGBA(0, 0, 0, 0.06), ui.Hex("#ffffff")
	if pal.dark {
		track, thumb = ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.2)
	}
	seg := ui.SegmentedBase(c.Key(key), &i, len(labels))
	seg.Track.Height(24).Padding(2).Radius(7).Background(track).AlignItems(ui.Center).Children(func() {
		for n, label := range labels {
			if n > 0 {
				// A separator, where neither segment beside it is chosen.
				sep := ui.Box(c).Width(1).Height(12).Shrink(0)
				if n != i && n-1 != i {
					sep.Background(pal.text.Alpha(0.14))
				}
			}
			s := seg.Segment(n).FillHeight().Padding(0, 12).Radius(5).Center()
			if n == i {
				s.Background(thumb)
				if pal.dark {
					s.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.25))
				} else {
					s.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.16)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.04))
				}
			}
			s.Children(func() {
				ui.Text(c, label).FontSize(12.5).SingleLine().TextColor(pal.text)
			})
		}
	})
	// The choice lands after this runs: take it then.
	seg.Track.OnChange(func() { *value = ids[i] })
}

func (a *App) generalPage(c *ui.Context, pal *palette) {
	s := &a.set
	settingsGroup(c, pal, "Sessions", "Sessions keep running in the background when Kero quits; "+a.tip("Quit and End All Sessions", "app.quitAll")+" ends them.", func() {
		settingRow(c, pal, "New sessions start in", "", func() {
			segmented(c, "newdir", &s.NewSessionDir, []string{"current", "project", "home"}, []string{"Current Folder", "Project", "Home"})
		})
		settingRow(c, pal, "Ask before closing a running program", "When a pane or session you close has a program other than the shell running.", func() {
			toggle(c.Key("confirm"), &s.ConfirmClose, pal)
		})
		settingRow(c, pal, "Restore sessions after a restart", "If the background service stops, as when the Mac restarts, sessions start again in their folders, under their history.", func() {
			toggle(c.Key("restore"), &s.RestoreSessions, pal)
		})
	})
	settingsGroup(c, pal, "Startup", "", func() {
		settingRow(c, pal, "Open Kero at login", "", func() {
			toggle(c.Key("login"), &a.ss.loginOpen, pal).OnChange(func() {
				if err := mygo.App.SetOpenAtLogin(a.ss.loginOpen); err != nil {
					a.ss.loginOpen = !a.ss.loginOpen
					a.ss.toast = "Couldn’t change it: " + err.Error()
				}
			})
		})
	})
}

func (a *App) appearancePage(c *ui.Context, pal *palette) {
	s := &a.set
	settingsGroup(c, pal, "", "", func() {
		settingRow(c, pal, "Appearance", "", func() {
			segmented(c, "appearance", &s.Appearance, []string{"system", "light", "dark"}, []string{"System", "Light", "Dark"})
		})
		settingRow(c, pal, "Translucent window", "Let the desktop show through the window’s background.", func() {
			toggle(c.Key("vibrancy"), &s.Vibrancy, pal)
		})
	})
	ui.Column(c).Gap(8).Children(func() {
		groupTitle(c, pal, "Background")
		ui.Row(c).Gap(12).Wrap().Children(func() {
			for _, b := range settings.Backgrounds {
				a.backgroundTile(c, pal, b.ID, b.Name)
			}
		})
	})
}

// backgroundTile previews a wash, choosing it on a click.
func (a *App) backgroundTile(c *ui.Context, pal *palette, id, name string) {
	sel := a.set.Background == id
	tile := ui.ButtonBase(c.Key("bg-" + id)).Column().Width(104).Gap(8).AlignItems(ui.Stretch).Label(name)
	if tile.Clicked() {
		a.set.Background = id
	}
	tile.Children(func() {
		sw := raised(ui.Box(c).Size(104, 68).Radius(12), pal).Clip()
		base, glow := washes(id, pal.dark)
		sw.LinearGradient(base)
		sw.Children(func() {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).LinearGradient(glow)
			// A card on it, as the window shows them.
			ui.Box(c).Absolute().Top(12).Left(26).Right(8).Bottom(-6).Radius(7).Background(pal.card)
			ui.Box(c).Absolute().Top(20).Left(8).Width(12).Height(4).Radius(2).Background(pal.textFaint.Alpha(0.5))
			if sel {
				ui.Box(c).Absolute().Top(6).Right(6).Size(18, 18).Radius(9).Center().Background(pal.accent).Children(func() {
					ui.Box(c).Size(10, 10).Draw(func(p *ui.Painter, r ui.Rect) {
						ck := (&ui.Path{}).MoveTo(r.X+1, r.Y+5.2).LineTo(r.X+4, r.Y+8).LineTo(r.X+9, r.Y+2)
						p.StrokePath(ck, 1.6, ui.Hex("#ffffff"))
					})
				})
			}
		})
		lbl := ui.Text(c, name).FontSize(12.5).TextColor(pal.textMuted).TextAlign(ui.Center)
		if sel {
			lbl.TextColor(pal.text).FontWeight(600)
		}
	})
}

// fontChoices are the fonts the picker offers: kero's own first, then
// the system's, then common coding fonts the user may have installed.
var fontChoices = []string{"JetBrains Mono Nerd Font", "System", "SF Mono", "Menlo", "Monaco", "JetBrains Mono", "Fira Code", "Cascadia Code",
	"Iosevka", "Berkeley Mono", "Geist Mono", "IBM Plex Mono", "Source Code Pro", "Hack", "Maple Mono"}

func (a *App) terminalPage(c *ui.Context, pal *palette) {
	s := &a.set
	a.terminalPreview(c, pal)
	settingsGroup(c, pal, "Text", "", func() {
		settingRow(c, pal, "Font", "JetBrains Mono with Nerd Fonts’ icons comes with Kero. Any installed monospaced font works; names separated by commas are tried in order.", func() {
			// The picker's names for the settings' values: "" is kero's
			// font, "monospace" the system's.
			family := s.FontFamily
			switch family {
			case "":
				family = "JetBrains Mono Nerd Font"
			case "monospace":
				family = "System"
			}
			opts := fontChoices
			if !slices.Contains(opts, family) {
				opts = append([]string{family}, opts...)
			}
			a.searchPopup(c, "font", family, opts, 240, "Use “%s”", pal, func(v string) {
				switch v {
				case "JetBrains Mono Nerd Font":
					v = ""
				case "System":
					v = "monospace"
				}
				a.set.FontFamily = v
			})
		})
		settingRow(c, pal, "Size", a.keys("text.bigger")+" and "+a.keys("text.smaller")+" change it too.", func() {
			sizes := []float32{10, 11, 12, 13, 14, 15, 16, 18, 20, 22, 24, 28}
			if !slices.Contains(sizes, s.FontSize) {
				sizes = append(sizes, s.FontSize)
				slices.Sort(sizes)
			}
			popup(c, "size", &s.FontSize, sizes, func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) + " pt" }, 90, pal)
		})
		settingRow(c, pal, "Line height", "", func() {
			v := float64(s.LineHeight)
			slider(c, "lh", &v, 1, 2, 160, pal).OnChange(func() { a.set.LineHeight = float32(int(v*100+0.5)) / 100 })
			ui.Textf(c, "%.2f×", s.LineHeight).FontSize(12.5).TextColor(pal.textMuted).Width(40).FontFeatures("tnum")
		})
		settingRow(c, pal, "Weight", "", func() {
			popup(c, "weight", &s.FontWeight, []int{300, 400, 500, 600}, func(w int) string {
				return map[int]string{300: "Light", 400: "Regular", 500: "Medium", 600: "Semibold"}[w]
			}, 140, pal)
		})
		settingRow(c, pal, "Ligatures", "Join characters as the font draws them, as => and !=.", func() {
			toggle(c.Key("liga"), &s.Ligatures, pal)
		})
		settingRow(c, pal, "Thicken text", "A heavier stroke, as Ghostty’s font-thicken.", func() {
			toggle(c.Key("thicken"), &s.Thicken, pal)
		})
	})
	themes := append([]string{"kero"}, terminal.GhosttyThemes()...)
	settingsGroup(c, pal, "Colors", "Ghostty’s themes, and your own in ~/.config/ghostty/themes.", func() {
		settingRow(c, pal, "Light windows", "", func() {
			a.searchPopup(c, "light", s.LightTheme, themes, 240, "", pal, func(v string) { a.set.LightTheme = v })
		})
		settingRow(c, pal, "Dark windows", "", func() {
			a.searchPopup(c, "dark", s.DarkTheme, themes, 240, "", pal, func(v string) { a.set.DarkTheme = v })
		})
	})
	settingsGroup(c, pal, "Cursor", "", func() {
		settingRow(c, pal, "Shape", "Programs may change it.", func() {
			segmented(c, "cursor", &s.Cursor, []string{"block", "bar", "underline"}, []string{"Block", "Bar", "Underline"})
		})
		settingRow(c, pal, "Blink", "", func() { toggle(c.Key("blink"), &s.CursorBlink, pal) })
	})
	settingsGroup(c, pal, "Input", "", func() {
		settingRow(c, pal, "Option is Meta", "Option sends Alt to programs, as for Emacs and readline, instead of typing accents.", func() {
			toggle(c.Key("optalt"), &s.OptionAsAlt, pal)
		})
		settingRow(c, pal, "Copy on select", "What you select goes to the clipboard.", func() {
			toggle(c.Key("cos"), &s.CopyOnSelect, pal)
		})
		settingRow(c, pal, "Right click", "", func() {
			popup(c, "rc", &s.RightClick, []string{"menu", "paste", "copyOrPaste"}, func(id string) string {
				return map[string]string{"menu": "Shows a menu", "paste": "Pastes", "copyOrPaste": "Copies, or pastes"}[id]
			}, 170, pal)
		})
		settingRow(c, pal, "Programs reading the clipboard", "As with OSC 52: agents asking for what you copied.", func() {
			segmented(c, "clip", &s.Clipboard, []string{"ask", "allow", "deny"}, []string{"Ask", "Allow", "Deny"})
		})
	})
}

// terminalPreview shows text as the terminals would draw it: the font,
// its size and line height, and the colors of the window's appearance.
func (a *App) terminalPreview(c *ui.Context, pal *palette) {
	light, dark := a.termThemes()
	th := light
	if pal.dark {
		th = dark
	}
	font := a.termFamily()
	features := ""
	if !a.set.Ligatures {
		features = "-calt,-liga"
	}
	line := func(spans ...ui.Span) {
		for i := range spans {
			spans[i].Font, spans[i].Size, spans[i].Weight, spans[i].Features = font, a.set.FontSize, a.set.FontWeight, features
		}
		ui.RichText(c, spans...).LineHeight(a.set.LineHeight).SingleLine()
	}
	box := raised(ui.Column(c).Radius(12).Padding(16, 18).Gap(0).Background(th.Background), pal)
	box.Children(func() {
		// Nerd Fonts' folder and branch icons: fonts without them show
		// boxes, which is worth knowing before picking one.
		line(ui.Span{Text: "\uf07b ~/dev/kero ", Color: th.Palette[4]}, ui.Span{Text: "\ue0a0 main ", Color: th.Palette[5]}, ui.Span{Text: "❯ ", Color: th.Palette[2]}, ui.Span{Text: "git status", Color: th.Foreground})
		line(ui.Span{Text: "modified:   ", Color: th.Foreground}, ui.Span{Text: "settings_view.go", Color: th.Palette[1]})
		line(ui.Span{Text: "func ", Color: th.Palette[5]}, ui.Span{Text: "main", Color: th.Palette[4]}, ui.Span{Text: "() { x := a != b && c >= d => ok }", Color: th.Foreground})
		ui.Row(c).Gap(4).Margin(10, 0, 0, 0).Children(func() {
			for i := range 16 {
				ui.Box(c).Size(16, 10).Radius(3).Background(th.Palette[i])
			}
		})
	})
}

func (a *App) shellPage(c *ui.Context, pal *palette) {
	s := &a.set
	login := shell.Login()
	settingsGroup(c, pal, "Shell", "Changes apply to panes started after them.", func() {
		settingRow(c, pal, "Shell", "Empty: your login shell, "+proto.ShortDir(login)+".", func() {
			if !a.ss.shellEditing {
				a.ss.shellDraft = s.Shell
			}
			in := textField(c, "shell", &a.ss.shellDraft, proto.ShortDir(login), 220, pal)
			a.ss.shellEditing = in.Focused()
			// Kept as it is submitted, or as the field loses the focus.
			in.OnSubmit(func() { a.set.Shell = strings.TrimSpace(a.ss.shellDraft) })
			if !a.ss.shellEditing && strings.TrimSpace(a.ss.shellDraft) != s.Shell {
				a.set.Shell = strings.TrimSpace(a.ss.shellDraft)
			}
		})
		settingRow(c, pal, "Scrollback", "How much output each pane keeps, also while no window shows it.", func() {
			sizes := []int{0, 1, 4, 16, 64}
			if !slices.Contains(sizes, s.Scrollback) {
				sizes = append(sizes, s.Scrollback)
			}
			popup(c, "sb", &s.Scrollback, sizes, func(mb int) string {
				if mb == 0 {
					return "None"
				}
				return strconv.Itoa(mb) + " MB"
			}, 130, pal)
		})
	})
	settingsGroup(c, pal, "Environment", "Added to every pane’s, over what kero sets (TERM, KERO_SESSION…).", func() {
		for i, kv := range s.Env {
			k, v, _ := strings.Cut(kv, "=")
			ui.Row(c.Key("env"+strconv.Itoa(i))).Padding(8, 0).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, k).Font("monospace").FontSize(12.5).FontWeight(600).TextColor(pal.text)
				ui.Text(c, v).Font("monospace").FontSize(12.5).TextColor(pal.textMuted).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
				idx := i
				iconButton(c, "rm", closeIcon, "Remove", pal, func() {
					a.set.Env = slices.Delete(slices.Clone(a.set.Env), idx, idx+1)
				})
			})
		}
		ui.Row(c).Padding(8, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			addEnv := func() {
				if kv := strings.TrimSpace(a.ss.envDraft); strings.Contains(kv, "=") {
					a.set.Env = append(slices.Clone(a.set.Env), kv)
					a.ss.envDraft = ""
				}
			}
			textField(c, "env-new", &a.ss.envDraft, "NAME=value", 0, pal).Font("monospace").OnSubmit(addEnv)
			if button(c, "env-add", "Add", "", false, pal) {
				addEnv()
			}
		})
	})
}

func (a *App) notificationsPage(c *ui.Context, pal *palette) {
	s := &a.set
	note := ""
	if !mygo.NotificationsSupported() {
		note = "This build can’t post notifications: macOS shows those of packaged apps only."
	}
	settingsGroup(c, pal, "Notify me when", note, func() {
		settingRow(c, pal, "A session needs me", "A program asks something, rings the bell, or reports it is blocked.", func() {
			toggle(c.Key("nw"), &s.NotifyWaiting, pal)
		})
		settingRow(c, pal, "An agent finishes", "While you look at another session, or another app.", func() {
			toggle(c.Key("nd"), &s.NotifyDone, pal)
		})
		settingRow(c, pal, "A program fails", "One that reports an error (OSC 7501).", func() {
			toggle(c.Key("nf"), &s.NotifyFailed, pal)
		})
	})
	settingsGroup(c, pal, "How", "", func() {
		settingRow(c, pal, "Play a sound", "", func() { toggle(c.Key("ns"), &s.NotifySound, pal) })
		settingRow(c, pal, "Badge the Dock icon", "With the number of sessions that need you.", func() { toggle(c.Key("nb"), &s.DockBadge, pal) })
		settingRow(c, pal, "Bounce the Dock icon", "Once, when a session comes to need you.", func() { toggle(c.Key("nbo"), &s.BounceDock, pal) })
	})
	if button(c, "test-notif", "Send a Test Notification", "", false, pal) {
		n := mygo.NewNotification(mygo.NotificationOptions{Title: "Kero", Body: "Notifications work.", Silent: !s.NotifySound})
		go func() {
			if err := n.Show(); err != nil {
				a.update(func() {
					a.ss.toast = "Couldn’t notify: " + err.Error()
					if a.setWin != nil {
						a.setWin.Invalidate()
					}
				})
			}
		}()
	}
}

func (a *App) agentsPage(c *ui.Context, pal *palette) {
	s := &a.set
	settingsGroup(c, pal, "Remote Control", "", func() {
		settingRow(c, pal, "Let commands drive this window", "kero do and kero focus run the app’s actions, as its menus do: splitting, switching sessions, opening the palette.", func() {
			toggle(c.Key("remote"), &s.RemoteControl, pal)
		})
	})
	codeBlock(c, pal, "kero do pane.split.down\nkero do session.attention\nkero actions        # what kero do runs")
	settingsGroup(c, pal, "Command line tool", "Inside Kero, every pane has the kero command on its PATH already.", func() {
		detail := "Not installed: other terminals don’t know the kero command."
		if a.ss.cliPath != "" {
			detail = a.ss.cliPath
		}
		settingRow(c, pal, "kero in your terminal", detail, func() {
			label := "Install"
			if a.ss.cliPath != "" {
				label = "Reinstall"
			}
			if button(c, "cli", label, "", a.ss.cliPath == "", pal) {
				a.installCLI()
				time.AfterFunc(1500*time.Millisecond, func() { a.update(a.refreshAgents) })
			}
		})
	})
	settingsGroup(c, pal, "Skills for agents", "Teaches agents to start sessions and panes, drive them, wait for them and report their status.", func() {
		for _, st := range cli.SkillStates() {
			name := map[string]string{"claude": "Claude Code", "agents": "Codex, OpenCode, Pi"}[st.Agent]
			detail := map[string]string{"current": "Installed", "outdated": "Installed, from an older Kero", "missing": "Not installed"}[st.State] + " · " + proto.ShortDir(st.Dir)
			settingRow(c, pal, name, detail, func() {
				agent := st.Agent
				if st.State != "missing" && button(c, "rm-"+agent, "Remove", "", false, pal) {
					if err := cli.RemoveSkill(agent); err != nil {
						a.toastIn(c, err.Error())
					}
				}
				if st.State != "current" {
					label := "Install"
					if st.State == "outdated" {
						label = "Update"
					}
					if button(c, "in-"+agent, label, "", true, pal) {
						if err := cli.InstallSkill(agent); err != nil {
							a.toastIn(c, err.Error())
						}
					}
				}
			})
		}
	})
	settingsGroup(c, pal, "Program status", "Programs report what they do with OSC 7501, Superlogical’s program status protocol: the sidebar shows it, and notifies you.", nil)
	codeBlock(c, pal, "kero report working \"Deploying\" --progress 40\nkero report blocked \"Approve the migration?\" --kind permission\nkero report done \"Deployed\"")
}

func codeBlock(c *ui.Context, pal *palette, code string) {
	ui.Text(c, code).Font("monospace").FontSize(12).LineHeight(1.55).TextColor(pal.text).
		Padding(12, 16).Radius(10).Background(pal.chip).Selectable()
}

func (a *App) advancedPage(c *ui.Context, pal *palette) {
	settingsGroup(c, pal, "Configuration", "Every setting here is a line of this file, as Ghostty’s: edit it in any editor, and what you save applies at once. kero config show lists every key.", func() {
		settingRow(c, pal, "Config file", proto.ShortDir(settings.Path()), func() {
			if button(c, "reveal-config", "Show in "+fileManager, "", false, pal) {
				a.ensureConfig()
				revealPath(settings.Path())
			}
			if button(c, "edit-config", "Open", "", true, pal) {
				a.openConfig()
			}
		})
		for i, p := range a.ss.problems {
			ui.Row(c.Key("problem"+strconv.Itoa(i))).Padding(0, 0, 8, 0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(pal.attention).Shrink(0)
				ui.Text(c, p).FontSize(12.5).TextColor(pal.attention).Grow(1).MinWidth(0)
			})
		}
	})
	settingsGroup(c, pal, "Background service", "kerod runs your sessions: quitting kero leaves it running.", func() {
		settingRow(c, pal, "kerod", a.ss.service, func() {
			if button(c, "svc-restart", "Restart", "", false, pal) {
				a.confirm = &confirmation{
					open: true, title: "Restart the background service?",
					detail: "Every pane’s program ends; the shells start again in their folders, under their history.",
					action: "Restart", destroy: true,
					run: func() { a.restartDaemon(); time.AfterFunc(2*time.Second, func() { a.update(a.refreshService) }) },
				}
				if a.win != nil {
					a.win.Focus()
				}
			}
		})
		settingRow(c, pal, "Log", proto.ShortDir(filepath.Join(proto.Dir(), "kerod.log")), func() {
			if button(c, "log", "Show", "", false, pal) {
				revealPath(filepath.Join(proto.Dir(), "kerod.log"))
			}
		})
	})
	settingsGroup(c, pal, "Data", "Settings, sessions and their scrollback.", func() {
		settingRow(c, pal, "Folder", proto.ShortDir(proto.Dir()), func() {
			if button(c, "reveal", "Show in "+fileManager, "", false, pal) {
				openPath(proto.Dir())
			}
		})
		settingRow(c, pal, "Reset all settings", "Back to how Kero comes, shortcuts included.", func() {
			if button(c, "reset", "Reset…", "", false, pal) {
				a.confirm = &confirmation{
					open: true, title: "Reset all settings?", detail: "Your sessions stay.", action: "Reset", destroy: true,
					run: func() { a.applySettings(settings.Default()) },
				}
				if a.win != nil {
					a.win.Focus()
				}
			}
		})
	})
}

// ensureConfig writes the configuration file, with its header, when
// there is none yet, so that there is a file to open.
func (a *App) ensureConfig() {
	if _, err := os.Stat(settings.Path()); err != nil {
		settings.Save(a.set)
	}
}

// openConfig opens the configuration in the user's text editor.
func (a *App) openConfig() {
	a.ensureConfig()
	openTextFile(settings.Path())
}

// toastIn shows a short message in the window c is of.
func (a *App) toastIn(c *ui.Context, msg string) { c.Toast(msg) }

// --- keyboard ---

func (a *App) keyboardPage(c *ui.Context, pal *palette) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		textField(c, "kb-search", &a.ss.filter, "Search actions or shortcuts", 0, pal)
		if len(a.set.Shortcuts) > 0 && button(c, "kb-reset", "Restore Defaults", "", false, pal) {
			a.set.Shortcuts = nil
			a.ss.notice = ""
		}
	})
	if a.ss.notice != "" {
		ui.Text(c, a.ss.notice).FontSize(12).TextColor(pal.attention).Padding(0, 4)
	}
	q := strings.ToLower(strings.TrimSpace(a.ss.filter))
	menus := []string{"Kero", "Session", "Pane", "View"}
	for _, menu := range menus {
		var acts []actions.Action
		for _, act := range actions.All {
			keys := actions.Glyphs(actions.Keys(act, a.set.Shortcuts))
			found := q == "" || strings.Contains(strings.ToLower(act.Title), q) || strings.Contains(strings.ToLower(keys), q) || strings.Contains(act.ID, q)
			if searchHits != nil {
				found = found && searchHits[strings.TrimSuffix(act.Title, "…")]
			}
			if act.Menu == menu && found {
				acts = append(acts, act)
			}
		}
		if len(acts) == 0 {
			continue
		}
		title := menu
		if menu == "Kero" {
			title = "App"
		}
		settingsGroup(c, pal, title, "", func() {
			for _, act := range acts {
				a.shortcutRow(c, pal, act)
			}
		})
	}
	ui.Text(c, "Click a shortcut, then type the new one. Delete removes it; Escape keeps it.").FontSize(12).TextColor(pal.textFaint).Padding(0, 4)
}

func (a *App) shortcutRow(c *ui.Context, pal *palette, act actions.Action) {
	keys := actions.Keys(act, a.set.Shortcuts)
	_, custom := a.set.Shortcuts[act.ID]
	ui.Row(c.Key(act.ID)).Padding(7, 0).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, strings.TrimSuffix(act.Title, "…")).FontSize(13.5).TextColor(pal.text).Grow(1).SingleLine()
		if custom && keys != act.Keys {
			b := ui.ButtonBase(c.Key("undo")).Size(22, 22).Radius(6).Center().TextColor(pal.textFaint).Label("Use the default, " + actions.Glyphs(act.Keys)).Tooltip("Default: " + actions.Glyphs(act.Keys))
			if b.Hovered() {
				b.Background(pal.rowHover).TextColor(pal.text)
			}
			b.Children(func() { ui.Icon(c, returnIcon).Size(12, 12) })
			if b.Clicked() {
				delete(a.set.Shortcuts, act.ID)
				if len(a.set.Shortcuts) == 0 {
					a.set.Shortcuts = nil
				}
			}
		}
		recording := a.ss.recording == act.ID
		chip := ui.ButtonBase(c.Key("keys")).Height(26).MinWidth(84).Padding(0, 10).Radius(7).Center().Label("Shortcut for " + act.Title)
		bg := pal.control
		switch {
		case recording:
			bg = pal.accent.Alpha(0.18)
		case chip.Hovered():
			bg = pal.controlHover
		}
		face(chip, pal, bg)
		if chip.Clicked() && !recording {
			a.setRecording(act.ID)
			a.ss.recordRef.Focus()
		}
		if recording {
			chip.Bind(&a.ss.recordRef).HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown {
					return ev.Kind == ui.InputText
				}
				return a.recordKey(act, ev.Mods, ev.Key)
			})
			if !a.ss.recordRef.Focused(c) && !chip.Pressed() {
				// Focus went elsewhere: the shortcut stays.
				c.After(200 * time.Millisecond)
			}
		}
		chip.Children(func() {
			switch {
			case recording:
				ui.Text(c, "Type a shortcut").FontSize(12.5).TextColor(pal.accent).SingleLine()
			case keys == "":
				ui.Text(c, "None").FontSize(12.5).TextColor(pal.textFaint).SingleLine()
			default:
				ui.Text(c, actions.Glyphs(keys)).FontSize(13).TextColor(pal.text).SingleLine().FontFeatures("tnum")
			}
		})
	})
}

// setRecording starts or ends typing a shortcut. While one is typed, the
// menus give up their key equivalents, which macOS gives keys to first:
// else typing one bound already, as ⌘D, would split a pane, not record.
func (a *App) setRecording(id string) {
	if a.ss.recording == id {
		return
	}
	was := a.ss.recording != ""
	a.ss.recording = id
	if was != (id != "") && a.win != nil {
		mygo.App.SetMenu(a.menu())
	}
}

// shortcutNeeds tells what a shortcut needs, typed without it.
var shortcutNeeds = func() string {
	if runtime.GOOS == "darwin" {
		return "A shortcut needs ⌘, ⌃ or ⌥, unless it is a function key."
	}
	return "A shortcut needs Ctrl or Alt, unless it is a function key."
}()

// recordKey takes a key typed for a shortcut, reporting whether it took it.
func (a *App) recordKey(act actions.Action, mods ui.Modifiers, key ui.Key) bool {
	switch {
	case key == ui.KeyEscape && mods == 0:
		a.setRecording("")
		return true
	case (key == ui.KeyBackspace || key == ui.KeyDelete) && mods == 0:
		a.setShortcut(act.ID, "")
		a.setRecording("")
		return true
	}
	name, ok := keyNames[key]
	if !ok {
		return false // a modifier alone, or a key menus can't take
	}
	if mods&(ui.Super|ui.Ctrl|ui.Alt) == 0 && !strings.HasPrefix(name, "F") {
		a.ss.notice = shortcutNeeds
		return true
	}
	var parts []string
	if mods&ui.Super != 0 && runtime.GOOS == "darwin" {
		parts = append(parts, "CmdOrCtrl")
	} else if mods&ui.Super != 0 {
		parts = append(parts, "Super")
	}
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	accel := strings.Join(append(parts, name), "+")
	a.setShortcut(act.ID, accel)
	a.setRecording("")
	return true
}

// setShortcut binds an action, taking the keys from any other that had
// them.
func (a *App) setShortcut(id, accel string) {
	sc := map[string]string{}
	for k, v := range a.set.Shortcuts {
		sc[k] = v
	}
	a.ss.notice = ""
	if accel != "" {
		for _, other := range actions.All {
			if other.ID != id && strings.EqualFold(actions.Keys(other, sc), accel) {
				sc[other.ID] = ""
				a.ss.notice = fmt.Sprintf("%s was %s’s: it has none now.", actions.Glyphs(accel), strings.TrimSuffix(other.Title, "…"))
			}
		}
	}
	act, _ := actions.Find(id)
	if accel == act.Keys {
		delete(sc, id)
	} else {
		sc[id] = accel
	}
	if len(sc) == 0 {
		sc = nil
	}
	a.set.Shortcuts = sc
}

// keyNames are the keys a shortcut takes, by their accelerator names.
var keyNames = func() map[ui.Key]string {
	m := map[ui.Key]string{
		ui.KeyEnter: "Enter", ui.KeyTab: "Tab", ui.KeySpace: "Space", ui.KeyBackspace: "Backspace", ui.KeyDelete: "Delete",
		ui.KeyEscape: "Escape", ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
		ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
		ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";",
		ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
	}
	fs := []ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12}
	for i, k := range fs {
		m[k] = "F" + strconv.Itoa(i+1)
	}
	digits := []ui.Key{ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
	for i, k := range digits {
		m[k] = strconv.Itoa(i)
	}
	letters := []ui.Key{ui.KeyA, ui.KeyB, ui.KeyC, ui.KeyD, ui.KeyE, ui.KeyF, ui.KeyG, ui.KeyH, ui.KeyI, ui.KeyJ, ui.KeyK, ui.KeyL, ui.KeyM,
		ui.KeyN, ui.KeyO, ui.KeyP, ui.KeyQ, ui.KeyR, ui.KeyS, ui.KeyT, ui.KeyU, ui.KeyV, ui.KeyW, ui.KeyX, ui.KeyY, ui.KeyZ}
	for i, k := range letters {
		m[k] = string(rune('A' + i))
	}
	return m
}()
