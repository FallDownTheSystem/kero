package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"kero/internal/assistant"
	"kero/internal/proto"
	"kero/internal/settings"

	"kero/locales"
)

// The Assistants view of the sessions sidebar: the user's assistants
// (proto.Assistant, which this machine's kerod keeps), grouped by their
// Group, each a row with its avatar, its session's status and its last
// word, and its children under it. A toggle above the list switches
// between it and the sessions; selecting an assistant shows its session
// in the panes, as any session's.

const (
	assistantsAction   = "sidebar.assistants"
	newAssistantAction = "assistants.new"
	viewAssistants     = "assistants"
)

// avatarNames are the assistants' built-in avatars, in the order the
// dialog offers them: solid shapes that read at 12 to 16 pixels, in the
// color that reads on the assistant's.
var avatarNames = []string{"star", "heart", "bolt", "moon", "sun", "leaf", "bell", "mail", "flag",
	"cat", "dog", "fox", "owl", "rabbit", "bear", "bird", "fish", "bee", "turtle", "ghost"}

var avatarGlyphs = func() map[string]*ui.SVG {
	// solid is one or more even-odd paths; a second path, as ears, doesn't
	// cut the first.
	solid := func(ds ...string) *ui.SVG {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">`
		for _, d := range ds {
			svg += `<path fill="currentColor" fill-rule="evenodd" d="` + d + `"/>`
		}
		return ui.MustParseSVG([]byte(svg + `</svg>`))
	}
	return map[string]*ui.SVG{
		"star":  solid("M8 1.2l2.1 4.3 4.7.7-3.4 3.3.8 4.7L8 12l-4.2 2.2.8-4.7L1.2 6.2l4.7-.7z"),
		"heart": solid("M8 14.2S1.6 10.3 1.6 5.8A3.3 3.3 0 0 1 8 4.4a3.3 3.3 0 0 1 6.4 1.4c0 4.5-6.4 8.4-6.4 8.4z"),
		"bolt":  solid("M9.6.8L2.8 9.2h4.4L6.4 15.2l6.8-8.4H8.8z"),
		"moon":  solid("M13.8 10.4A6.2 6.2 0 0 1 5.6 2.2a6.2 6.2 0 1 0 8.2 8.2z"),
		"sun": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><circle cx="8" cy="8" r="3.2" fill="currentColor"/>` +
			`<path d="M8 1v1.8M8 13.2V15M1 8h1.8M13.2 8H15M3 3l1.3 1.3M11.7 11.7L13 13M3 13l1.3-1.3M11.7 4.3L13 3" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>`)),
		"leaf": solid("M14 2C6.5 2 2.5 5.6 2.5 10.4c0 1 .2 1.9.6 2.7L1.6 14.6l.9.9 1.5-1.5c.8.4 1.8.6 2.8.6C11.6 14.6 14 10 14 2zM5 12.2c1.6-3 4-5.4 6.6-6.8-2.2 1.9-4.2 4.3-5.6 7.4z"),
		"bell": solid("M8 1.2a1.1 1.1 0 0 1 1.1 1.1v.5a4.6 4.6 0 0 1 3.5 4.5v2.6l1.6 2.2H1.8l1.6-2.2V7.3a4.6 4.6 0 0 1 3.5-4.5v-.5A1.1 1.1 0 0 1 8 1.2zM6.2 13h3.6a1.8 1.8 0 0 1-3.6 0z"),
		"mail": solid("M2.4 3h11.2c.8 0 1.4.6 1.4 1.4v7.2c0 .8-.6 1.4-1.4 1.4H2.4c-.8 0-1.4-.6-1.4-1.4V4.4C1 3.6 1.6 3 2.4 3zM2.6 4.6L8 8.6l5.4-4h-.1L8 7.2 2.7 4.6z"),
		"flag": solid("M3 1.4h1.5v13.2H3zM4.5 2h9l-2.2 3.6 2.2 3.6h-9z"),
		"cat":  solid("M2.4 1.6l3.2 3h4.8l3.2-3v7.6c0 3-2.5 5.2-5.6 5.2s-5.6-2.2-5.6-5.2zM5.9 8.6a.95.95 0 1 0 0 .01zM10.1 8.6a.95.95 0 1 0 0 .01z"),
		"dog": solid("M5.4 2.4h5.2c.9 0 1.5.7 1.5 1.5v5.4c0 2.9-1.8 5.1-4.1 5.1s-4.1-2.2-4.1-5.1V3.9c0-.8.6-1.5 1.5-1.5zM5.6 7.3a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0zM8.8 7.3a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0zM6.75 10.5a1.25 0.9 0 1 0 2.5 0a1.25 0.9 0 1 0 -2.5 0z",
			"M4.6 2.1L1 3.6c-.5.2-.7.7-.6 1.2l1.1 4.4c.2.7 1.1.9 1.5.3L4.6 7zM11.4 2.1L15 3.6c.5.2.7.7.6 1.2l-1.1 4.4c-.2.7-1.1.9-1.5.3L11.4 7z"),
		"fox": solid("M1.4 1.4l4.4 3.2h4.4l4.4-3.2v6.4L8 14.8 1.4 7.8zM4.75 7.6a0.85 0.85 0 1 0 1.7 0a0.85 0.85 0 1 0 -1.7 0zM9.55 7.6a0.85 0.85 0 1 0 1.7 0a0.85 0.85 0 1 0 -1.7 0zM7.2 12.1a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0z"),
		"owl": solid("M2.6 1.4l2.9 1.9c.8-.3 1.6-.4 2.5-.4s1.7.1 2.5.4l2.9-1.9v7.8c0 3-2.4 5.4-5.4 5.4s-5.4-2.4-5.4-5.4zM3.65 7a1.95 1.95 0 1 0 3.9 0a1.95 1.95 0 1 0 -3.9 0zM8.45 7a1.95 1.95 0 1 0 3.9 0a1.95 1.95 0 1 0 -3.9 0zM7.1 9.6h1.8L8 11.2z",
			"M4.8 7a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0zM9.6 7a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0z"),
		"rabbit": solid("M3.6 10.4a4.4 4.4 0 1 0 8.8 0a4.4 4.4 0 1 0 -8.8 0zM5.5 9.9a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0zM8.9 9.9a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0zM7.35 12a0.65 0.65 0 1 0 1.3 0a0.65 0.65 0 1 0 -1.3 0z",
			"M3.95 4.2a1.55 3.6 0 1 0 3.1 0a1.55 3.6 0 1 0 -3.1 0zM8.95 4.2a1.55 3.6 0 1 0 3.1 0a1.55 3.6 0 1 0 -3.1 0z"),
		"bear": solid("M2.4 9a5.6 5.6 0 1 0 11.2 0a5.6 5.6 0 1 0 -11.2 0zM5 8.1a0.9 0.9 0 1 0 1.8 0a0.9 0.9 0 1 0 -1.8 0zM9.2 8.1a0.9 0.9 0 1 0 1.8 0a0.9 0.9 0 1 0 -1.8 0zM6.6 10.9a1.4 1 0 1 0 2.8 0a1.4 1 0 1 0 -2.8 0z",
			"M1.6 4.2a2.2 2.2 0 1 0 4.4 0a2.2 2.2 0 1 0 -4.4 0zM10 4.2a2.2 2.2 0 1 0 4.4 0a2.2 2.2 0 1 0 -4.4 0z"),
		"bird": solid("M1.2 14.2l2.5-3c-.9-.8-1.5-2-1.5-3.3 0-2.9 2.6-5.1 5.8-5.1 2.4 0 4.4 1.3 5.2 3.2l2.6 1-2.5 1.2c-.3 3.4-3 6-6.6 6zM9.8 6.4a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0z"),
		"fish": solid("M1.4 8c2-3.1 4.9-4.3 7.9-4.3 2.3 0 4 1.6 4.7 4.3-.7 2.7-2.4 4.3-4.7 4.3-3 0-5.9-1.2-7.9-4.3zM.6 4.4L3.2 8 .6 11.6zM10.8 6.5a.85.85 0 1 0 0 .01z"),
		"bee": solid("M4 9.9a4 4.7 0 1 0 8 0a4 4.7 0 1 0 -8 0zM4.7 8.4h6.6v1.3H4.7zM5 11.1h6v1.3H5z",
			"M6.1 4.6a1.9 1.9 0 1 0 3.8 0a1.9 1.9 0 1 0 -3.8 0zM1.1 5.6a2.5 1.7 0 1 0 5 0a2.5 1.7 0 1 0 -5 0zM9.9 5.6a2.5 1.7 0 1 0 5 0a2.5 1.7 0 1 0 -5 0z"),
		"turtle": solid("M3.5 8.6a4.5 4.1 0 1 0 9 0a4.5 4.1 0 1 0 -9 0zM8 6.3l2 1.15v2.3L8 10.9l-2-1.15v-2.3z",
			"M6.3 2.8a1.7 1.7 0 1 0 3.4 0a1.7 1.7 0 1 0 -3.4 0zM1.6 5.3a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0 -3 0zM11.4 5.3a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0 -3 0zM1.6 11.9a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0 -3 0zM11.4 11.9a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0 -3 0zM7.2 12.4h1.6L8 15z"),
		"ghost": solid("M8 1.4a5.2 5.2 0 0 1 5.2 5.2v8.2l-1.8-1.3-1.7 1.3L8 13.5l-1.7 1.3-1.7-1.3-1.8 1.3V6.6A5.2 5.2 0 0 1 8 1.4zM6.1 6.2a1 1 0 1 0 0 .01zM9.9 6.2a1 1 0 1 0 0 .01z"),
	}
}()

// assistantColorName is an assistant's color: its own, else the one its
// name picks, which stays the same from run to run.
func assistantColorName(a *proto.Assistant) string {
	if a.Color != "" {
		return a.Color
	}
	f := fnv.New32a()
	f.Write([]byte(strings.ToLower(a.Name)))
	return settings.HostColors[f.Sum32()%uint32(len(settings.HostColors))]
}

// assistantColor is an assistant's color in this appearance: a named
// color's light or dark shade, or its custom color as it is in both.
func (pal *palette) assistantColor(a *proto.Assistant) ui.Color {
	name := assistantColorName(a)
	if assistant.IsCustomColor(name) {
		return ui.Hex(name)
	}
	c := hostColors[name]
	if pal.dark {
		return c[1]
	}
	return c[0]
}

// assistantInk is an assistant's color where it is drawn on the app's own
// surface: the text's color where it reads too faintly on that.
func (pal *palette) assistantInk(a *proto.Assistant, surface ui.Color) ui.Color {
	if col := pal.assistantColor(a); contrast(col, surface) >= 3 {
		return col
	}
	return pal.text
}

// avatarImages are the custom avatars decoded, by path and when it
// changed; on the main thread.
var avatarImages = map[string]ui.ImageSource{}

// avatarImage is an assistant's own image, nil for none or one that
// doesn't decode, which the initial stands for.
func avatarImage(a *proto.Assistant) ui.ImageSource {
	name, ok := strings.CutPrefix(a.Avatar, "file:")
	if !ok {
		return nil
	}
	path := filepath.Join(assistant.Dir(a.ID), name)
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	key := fmt.Sprintf("%s|%d|%d", path, st.ModTime().UnixNano(), st.Size())
	if img, ok := avatarImages[key]; ok {
		return img
	}
	var img ui.ImageSource
	if b, err := os.ReadFile(path); err == nil {
		img, _ = ui.DecodeImage(b)
	}
	avatarImages[key] = img
	return img
}

// initial is a name's first letter, as an avatar shows it.
func initial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

// avatar draws an assistant's avatar: a round of its color with its
// glyph or initial, or its image.
func assistantAvatar(c *ui.Context, pal *palette, a *proto.Assistant, size float32) {
	if img := avatarImage(a); img != nil {
		ui.Image(c, img).Size(size, size).Radius(size / 2).Fit(ui.Cover).Shrink(0)
		return
	}
	col := pal.assistantColor(a)
	ink := onColor(col)
	ui.Box(c).Size(size, size).Radius(size / 2).Background(col).Center().Shrink(0).Children(func() {
		if g := avatarGlyphs[a.Avatar]; g != nil {
			ui.Icon(c, g).Size(size*0.62, size*0.62).TextColor(ink)
			return
		}
		ui.Text(c, initial(a.Name)).FontSize(size * 0.55).FontWeight(650).TextColor(ink).SingleLine()
	})
}

func (a *App) assistantsShown() bool { return a.prefs.View == viewAssistants }

func (a *App) toggleAssistants() {
	if a.assistantsShown() {
		a.prefs.View = ""
	} else {
		a.prefs.View = viewAssistants
	}
	a.savePrefs()
}

// assistantOf is the assistant whose session s is, nil for none.
func (a *App) assistantOf(s *proto.Session) *proto.Assistant {
	if s == nil || s.Assistant == "" || a.hostOf(s.ID) != a.local() {
		return nil
	}
	return a.assistantByID(s.Assistant)
}

func (a *App) assistantByID(id string) *proto.Assistant {
	for i := range a.st.Assistants {
		if a.st.Assistants[i].ID == id {
			return &a.st.Assistants[i]
		}
	}
	return nil
}

// assistantSession is an assistant's live session, nil for none.
func (a *App) assistantSession(as *proto.Assistant) *proto.Session {
	for i := range a.st.Sessions {
		if s := &a.st.Sessions[i]; s.Assistant == as.ID && a.hostOf(s.ID) == a.local() {
			return s
		}
	}
	return nil
}

// assistantNeeds tells an assistant that needs the user: its session
// waits or failed. Derived, never stored, so that every place agrees.
func assistantNeeds(s *proto.Session) bool {
	return s != nil && (s.Status == proto.Waiting || s.Status == proto.Failed)
}

// assistantsAttention counts the assistants that need the user, and
// tells whether one has something unread.
func (a *App) assistantsAttention() (needs int, unread bool) {
	for i := range a.st.Assistants {
		s := a.assistantSession(&a.st.Assistants[i])
		switch {
		case assistantNeeds(s):
			needs++
		case s != nil && s.Unread:
			unread = true
		}
	}
	return needs, unread
}

// sidebarViews is the toggle above the sessions sidebar's list: Sessions
// and Assistants, the latter with a dot and a count while assistants need
// the user, or a green dot while one has news unread.
func (a *App) sidebarViews(c *ui.Context, pal *palette) {
	needs, unread := a.assistantsAttention()
	track, thumb := ui.RGBA(0, 0, 0, 0.06), ui.Hex("#ffffff")
	if pal.dark {
		track, thumb = ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.2)
	}
	ui.Row(c.Key("views")).Height(26).Shrink(0).Margin(0, 10, 6, 10).Padding(2).Radius(8).Background(track).AlignItems(ui.Center).Children(func() {
		for _, v := range []string{"", viewAssistants} {
			on := a.prefs.View == v
			label := locales.SidebarViewSessions()
			if v == viewAssistants {
				label = locales.SidebarViewAssistants()
			}
			b := ui.ButtonBase(c.Key("view-" + v)).Grow(1).Basis(0).FillHeight().Radius(6).Center().Gap(5).Label(label).Tooltip(a.tip(label, assistantsAction))
			switch {
			case on:
				b.Background(thumb)
			case b.Hovered():
				b.Background(pal.rowHover)
			}
			if b.Clicked() && !on {
				view := v
				a.defer_(func() {
					a.prefs.View = view
					a.savePrefs()
				})
			}
			b.Children(func() {
				fg := pal.textMuted
				if on {
					fg = pal.text
				}
				ui.Text(c, label).FontSize(12.5).FontWeight(550).TextColor(fg).SingleLine()
				if v != viewAssistants {
					return
				}
				switch {
				case needs > 0:
					ui.Box(c.Key("dot")).Size(6, 6).Radius(3).Background(pal.attention).Shrink(0)
					ui.Textf(c, "%d", needs).FontSize(11.5).FontWeight(650).FontFeatures("tnum").TextColor(pal.attention).SingleLine()
				case unread:
					ui.Box(c.Key("dot")).Size(6, 6).Radius(3).Background(a.codeOf(c, pal).Palette[2]).Shrink(0)
				}
			})
		}
	})
}

// assistantGroups are the assistants by their Group, those of none first
// with no header, each in the order they were made.
func (a *App) assistantGroups() (names []string, by map[string][]*proto.Assistant) {
	by = map[string][]*proto.Assistant{}
	for i := range a.st.Assistants {
		as := &a.st.Assistants[i]
		if _, ok := by[as.Group]; !ok {
			names = append(names, as.Group)
		}
		by[as.Group] = append(by[as.Group], as)
	}
	slices.SortStableFunc(names, func(x, y string) int {
		if (x == "") != (y == "") {
			if x == "" {
				return -1
			}
			return 1
		}
		return 0
	})
	return names, by
}

// assistantsList is the Assistants view: its groups, or what assistants
// are while there is none, and that checks run only while Kero runs.
func (a *App) assistantsList(c *ui.Context, pal *palette) {
	list := ui.Scroll(c.Key("assistants")).Grow(1).Padding(0, 10, 16, 10).DragWindow()
	if list.WindowDoubleClicked() {
		a.defer_(func() { a.openAssistantForm("") })
	}
	list.Children(func() {
		ui.Row(c.Key("head")).Height(28).Padding(0, 6, 0, rowInset).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SidebarViewAssistants()).FontSize(12).FontWeight(600).TextColor(pal.textMuted).Grow(1)
			iconButton(c, "new-assistant", plusIcon, a.tip(locales.AssistantsNew(), newAssistantAction), pal, func() { a.openAssistantForm("") })
		})
		if len(a.st.Assistants) == 0 {
			ui.Column(c.Key("empty")).Padding(10, rowInset, 0, rowInset).Gap(8).AlignItems(ui.Start).Children(func() {
				ui.Text(c, locales.AssistantsEmptyTitle()).FontSize(13.5).FontWeight(600).TextColor(pal.text)
				ui.Text(c, locales.AssistantsEmptyDetail()).FontSize(12.5).LineHeight(1.4).TextColor(pal.textMuted)
				if button(c, "empty-new", locales.AssistantsNew(), "", true, pal) {
					a.defer_(func() { a.openAssistantForm("") })
				}
			})
		}
		n := 0
		names, by := a.assistantGroups()
		for gi, g := range names {
			key := "assistants:" + g
			collapsed := g != "" && a.prefs.Collapsed[key]
			if g != "" {
				a.assistantGroupHeader(c, pal, g, key, collapsed, by[g], gi > 0)
			}
			if collapsed {
				continue
			}
			for _, as := range by[g] {
				a.assistantRow(c, pal, as, &n)
			}
		}
		if len(a.st.Assistants) > 0 {
			ui.Text(c, locales.AssistantsWhileRunning()).FontSize(11.5).TextColor(pal.textFaint).Padding(12, rowInset, 0, rowInset)
		}
	})
}

// assistantGroupHeader is a group's header, which folds it as a project's
// does, counting while folded those that need the user.
func (a *App) assistantGroupHeader(c *ui.Context, pal *palette, name, key string, collapsed bool, list []*proto.Assistant, gap bool) {
	top := float32(4)
	if gap {
		top = 12
	}
	hdr := ui.ButtonBase(c.Key("group-"+key)).Height(26).Margin(top, 0, 2, 0).Padding(0, 6, 0, rowInset).Gap(6).AlignItems(ui.Center).Radius(6).Label(name)
	if hdr.Hovered() {
		hdr.Background(pal.rowHover)
	}
	if hdr.Clicked() {
		a.defer_(func() {
			if a.prefs.Collapsed == nil {
				a.prefs.Collapsed = map[string]bool{}
			}
			a.prefs.Collapsed[key] = !a.prefs.Collapsed[key]
			a.savePrefs()
		})
	}
	needs := 0
	for _, as := range list {
		if assistantNeeds(a.assistantSession(as)) {
			needs++
		}
	}
	hdr.Children(func() {
		ui.Text(c, name).FontSize(12).FontWeight(600).TextColor(pal.textMuted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		ui.Textf(c, "%d", len(list)).FontSize(11.5).TextColor(pal.textFaint)
		ui.Spacer(c)
		if collapsed && needs > 0 {
			ui.Box(c).Size(6, 6).Radius(3).Background(pal.attention).Shrink(0)
			ui.Textf(c, "%d", needs).FontSize(11.5).FontWeight(650).TextColor(pal.attention)
		}
		chev := chevronIcon
		ui.Icon(c, chev).Size(10, 10).TextColor(pal.textFaint).Rotate(b2f(!collapsed) * 90)
	})
}

// assistantLine is a detailed row's second line: its status line, else
// its last report's first line, else its role.
func assistantLine(as *proto.Assistant, s *proto.Session) string {
	if s != nil && s.Message != "" {
		return s.Message
	}
	if r := as.LastReport; r != nil {
		return reportResult(r.Text)
	}
	return as.Role
}

// assistantNote is what a row has at its line's right: a problem, Paused,
// or the time of its next check.
func assistantNote(as *proto.Assistant, now time.Time) (string, bool) {
	switch {
	case as.Problem != "":
		return locales.AssistantsProblem(as.Problem), true
	case as.Paused:
		return locales.AssistantsPaused(), false
	case as.NextCheck > 0:
		t := time.UnixMilli(as.NextCheck)
		when := t.Format("15:04")
		if y, m, d := t.Date(); y != now.Year() || m != now.Month() || d != now.Day() {
			when = t.Format("Mon 15:04")
		}
		return locales.AssistantsNext(when), false
	}
	return "", false
}

// assistantRow is an assistant's row, and under it its children's, as an
// orchestrator's are, folded by the same chevron.
func (a *App) assistantRow(c *ui.Context, pal *palette, as *proto.Assistant, n *int) {
	s := a.assistantSession(as)
	*n++
	selected := s != nil && s.ID == a.selected
	detailed := a.set.SidebarRows != "compact"
	h := float32(rowH)
	if detailed {
		h = rowH2
	}
	row := ui.Row(c.Key("assistant-"+as.ID)).Height(h).Padding(0, 10, 0, rowInset).Gap(10).AlignItems(ui.Center).Radius(8).Margin(0, 0, 1, 0).Label(as.Name)
	bg := ui.Transparent
	switch {
	case selected:
		bg = pal.rowSelected
		chosen(row, pal)
	case row.Hovered():
		bg = pal.rowHover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if (row.Pressed() || row.Clicked()) && !selected {
		id := as.ID
		a.defer_(func() { a.selectAssistant(id) })
	}
	tip := as.Name + " · " + as.Role
	if s != nil && s.Message != "" {
		tip += "\n" + s.Message
	}
	if note, _ := assistantNote(as, time.Now()); note != "" {
		tip += "\n" + note
	}
	pointerTooltip(c, row, "tip-"+as.ID, tip)
	row.ContextMenu(func(m *ui.Menu) { a.assistantMenu(m, as) })
	row.Children(func() {
		size := float32(18)
		if detailed {
			size = 22
		}
		assistantAvatar(c, pal, as, size)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				name := ui.Text(c, as.Name).FontSize(13.5).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).TextColor(pal.text)
				if s != nil && s.Unread {
					name.FontWeight(650)
				}
				if s != nil && len(a.childrenOf(s.ID)) > 0 && !detailed {
					a.childrenChevron(c, pal, s)
				}
			})
			if !detailed {
				return
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, assistantLine(as, s)).FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				ui.Spacer(c)
				if s != nil && len(a.childrenOf(s.ID)) > 0 {
					a.childrenChevron(c, pal, s)
				}
				if note, bad := assistantNote(as, time.Now()); note != "" {
					fg := pal.textFaint
					if bad {
						fg = pal.danger
					}
					ui.Text(c, note).FontSize(11.5).TextColor(fg).SingleLine().Shrink(0)
				}
			})
		})
		switch {
		case s == nil:
			ui.Box(c).Size(14, 14).Shrink(0)
		default:
			statusIcon(c, pal, s.Status, s.Unread, 14)
		}
	})
	if s == nil {
		return
	}
	for _, k := range a.childrenOf(s.ID) {
		if !a.folded(k) {
			a.sessionRow(c, pal, k, n)
		}
	}
}

// childrenChevron is an orchestrator's mark's chevron and count, which
// an assistant's row has for its children, without the mark.
func (a *App) childrenChevron(c *ui.Context, pal *palette, s *proto.Session) {
	ui.Row(c.Key("kids")).Shrink(0).Children(func() { a.orchestratorKids(c, pal, s) })
}

// selectAssistant shows an assistant's session, starting one first if it
// has none.
func (a *App) selectAssistant(id string) {
	as := a.assistantByID(id)
	if as == nil {
		return
	}
	if s := a.assistantSession(as); s != nil {
		a.selectSession(s.ID)
		return
	}
	a.startAssistant(id, false)
}

// startAssistant starts an assistant's session, or a fresh chat in its
// place, and shows it.
func (a *App) startAssistant(id string, fresh bool) {
	h := a.local()
	if h.cl == nil || h.conn != connReady {
		a.toast(locales.StatusDisconnected())
		return
	}
	cl := h.cl
	command := []string{}
	if as := a.assistantByID(id); as != nil {
		command = assistant.Command(*as, a.set)
	}
	go func() {
		installAmaSkill(command)
		var s proto.Session
		err := cl.Call("assistant.start", proto.AssistantParams{ID: id, New: fresh}, &s)
		a.update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			a.addPending(h, s)
			a.selectSession(s.ID)
		})
	}()
}

// assistantCall calls kerod about an assistant, telling what fails.
func (a *App) assistantCall(method string, params any) {
	h := a.local()
	if h.cl == nil {
		return
	}
	cl := h.cl
	go func() {
		if err := cl.Call(method, params, nil); err != nil {
			a.update(func() { a.toast(err.Error()) })
		}
	}()
}

// assistantMenu is an assistant's row's menu.
func (a *App) assistantMenu(m *ui.Menu, as *proto.Assistant) {
	id, name := as.ID, as.Name
	s := a.assistantSession(as)
	if m.Item(locales.AssistantsRunChecks()).Chosen() {
		a.assistantCall("assistant.run", proto.AssistantParams{ID: id})
	}
	if len(as.Checks) > 1 {
		for _, ch := range as.Checks {
			if m.Item(locales.AssistantsRunCheck(assistant.CheckWhen(ch))).Chosen() {
				a.assistantCall("assistant.run", proto.AssistantParams{ID: id, Check: ch.ID})
			}
		}
	}
	m.Separator()
	if m.Item(locales.AssistantsNewChat()).Chosen() {
		a.startAssistant(id, true)
	}
	paused := !as.Paused
	label := locales.AssistantsPause()
	if as.Paused {
		label = locales.AssistantsResume()
	}
	if m.Item(label).Chosen() {
		a.assistantCall("assistant.update", proto.AssistantEdit{ID: id, Paused: &paused})
	}
	if m.Item(locales.AssistantsEdit()).Chosen() {
		a.defer_(func() { a.openAssistantForm(id) })
	}
	if m.Item(locales.AssistantsShowFolder()).Chosen() {
		openPath(assistant.Dir(id))
	}
	m.Separator()
	if m.Item(locales.AssistantsCloseSession()).Disabled(s == nil).Chosen() && s != nil {
		a.assistantCall("close", proto.CloseParams{ID: s.ID})
	}
	if m.Item(locales.AssistantsDelete()).Chosen() {
		a.defer_(func() { a.askDeleteAssistant(id, name) })
	}
}

// askDeleteAssistant asks before an assistant goes: its folder stays,
// unless the user deletes it too.
func (a *App) askDeleteAssistant(id, name string) {
	a.confirm = &confirmation{open: true, title: locales.AssistantsDeleteTitle(name), detail: locales.AssistantsDeleteDetail(),
		action: locales.AssistantsDeleteKeep(), destroy: true,
		run: func() { a.assistantCall("assistant.remove", proto.AssistantParams{ID: id}) },
		alt: locales.AssistantsDeleteFolder(), altDestroy: true,
		altRun: func() { a.assistantCall("assistant.remove", proto.AssistantParams{ID: id, Folder: true}) },
	}
}

// assistantsOrder is the Assistants view's sessions, in its order, for
// ⌘1–9 and next and previous session: each assistant's, then its
// children's.
func (a *App) assistantsOrder() []string {
	var ids []string
	names, by := a.assistantGroups()
	for _, g := range names {
		if g != "" && a.prefs.Collapsed["assistants:"+g] {
			continue
		}
		for _, as := range by[g] {
			s := a.assistantSession(as)
			if s == nil {
				continue
			}
			ids = append(ids, s.ID)
			for _, k := range a.childrenOf(s.ID) {
				if !a.folded(k) {
					ids = append(ids, k.ID)
				}
			}
		}
	}
	return ids
}

// avatarLabel is a built-in avatar's name, as the dialog says it.
func avatarLabel(name string) string {
	if name == "" {
		return locales.AssistantFormInitial()
	}
	r, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[size:]
}

// assistantSeen is what the app last saw of an assistant, for its
// notifications: its last report's time, its session's status, and the
// check run (its Last) a notification last told of.
type assistantSeen struct {
	reportAt int64
	status   proto.Status
	notified int64
}

// latestCheck is when an assistant's checks last went to it.
func latestCheck(as *proto.Assistant) int64 {
	var last int64
	for _, c := range as.Checks {
		last = max(last, c.Last)
	}
	return last
}

// reportResult is what a notification says of a report: its Result's
// first line, without "Result:", else its first line.
func reportResult(text string) string {
	lines := strings.Split(text, "\n")
	clean := func(l string) string { return strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "*_#>- ")) }
	for i, l := range lines {
		l = clean(l)
		if len(l) >= 7 && strings.EqualFold(l[:7], "result:") {
			if r := clean(strings.Trim(l[7:], "*_ ")); r != "" {
				return r
			}
			for _, next := range lines[i+1:] {
				if r := clean(next); r != "" {
					return r
				}
			}
		}
	}
	for _, l := range lines {
		if r := clean(l); r != "" {
			return r
		}
	}
	return ""
}

// notifyAssistants tells the user, under notify-done, that an assistant
// finished a check: it sent a report that waits for nothing (one that
// waits notifies as Waiting), or its turn after a check ended with no
// report. Its other turns, as answering a child, show its unread dot.
// Nothing notifies of the first state the app sees, nor of a session the
// user watches.
func (a *App) notifyAssistants(first bool) {
	if a.asstSeen == nil {
		a.asstSeen = map[string]assistantSeen{}
	}
	seen := map[string]bool{}
	for i := range a.st.Assistants {
		as := &a.st.Assistants[i]
		seen[as.ID] = true
		s := a.assistantSession(as)
		prev, known := a.asstSeen[as.ID]
		cur := assistantSeen{status: proto.Idle, notified: prev.notified}
		if s != nil {
			cur.status = s.Status
		}
		if as.LastReport != nil {
			cur.reportAt = as.LastReport.At
		}
		last := latestCheck(as)
		if !known {
			cur.notified = last
		}
		a.asstSeen[as.ID] = cur
		if first || !known || !a.set.NotifyDone || s == nil || a.watching(s.ID) {
			continue
		}
		title := locales.AssistantNotifyDone(as.Name)
		switch {
		case cur.reportAt > prev.reportAt:
			// A report: once, whatever turn it came in.
			cur.notified = max(cur.notified, last)
			a.asstSeen[as.ID] = cur
			if !as.LastReport.Waiting {
				a.showNotification(s.ID, title, reportResult(as.LastReport.Text))
			}
		case prev.status == proto.Working && cur.status == proto.Idle && s.Unread && last > cur.reportAt && last > cur.notified:
			cur.notified = last
			a.asstSeen[as.ID] = cur
			body := s.Message
			if body == "" {
				body = locales.AssistantNotifyNoReport()
			}
			a.showNotification(s.ID, title, body)
		}
	}
	for id := range a.asstSeen {
		if !seen[id] {
			delete(a.asstSeen, id)
		}
	}
}
