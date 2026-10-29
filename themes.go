package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/proto"
	"kero/internal/settings"
	"kero/locales"
)

// themeCache holds the files' themes the settings name, as last read: a
// theme of the user's is a file, and the editors' cards take their color
// from the theme on every frame. They are nil while they are the
// terminals'. The terminals' are the window's looks (App.looks).
type themeCache struct {
	read                bool
	fileLight, fileDark string
	fl, fd              *terminal.Theme
}

// themePreview holds the themes Settings shows in place of the settings'
// while the user looks through them or edits one, the terminals' and the
// files': nil for none. The terminals' color the whole window, as their
// looks (lookOf).
type themePreview struct{ light, dark, fileLight, fileDark *terminal.Theme }

// chosenThemes returns the terminals' themes the settings name, whatever
// Settings previews.
func (a *App) chosenThemes() (light, dark *terminal.Theme) {
	l := a.looks()
	return l.chosenLight.term, l.chosenDark.term
}

// readThemes reads the files' themes the settings name, as the names
// change, or after the cache was emptied, as a theme is saved.
func (a *App) readThemes() {
	tc, s := &a.themes, &a.set
	if tc.read && tc.fileLight == s.FileLightTheme && tc.fileDark == s.FileDarkTheme {
		return
	}
	*tc = themeCache{read: true, fileLight: s.FileLightTheme, fileDark: s.FileDarkTheme,
		fl: namedTheme(s.FileLightTheme, false), fd: namedTheme(s.FileDarkTheme, true)}
}

// fileTheme is the theme of code, in editor panes and the Explorer, in
// windows of the appearance: the one Settings previews, the one the
// settings name, or the terminals'.
func (a *App) fileTheme(dark bool) *terminal.Theme {
	a.readThemes()
	p, chosen := a.preview.fileLight, a.themes.fl
	if dark {
		p, chosen = a.preview.fileDark, a.themes.fd
	}
	switch {
	case p != nil:
		return p
	case chosen != nil:
		return chosen
	}
	return a.activeTheme(dark)
}

// namedTheme returns the theme a setting names for windows of the
// appearance: one of kero's, a Ghostty one, else kero's default (lookNamed);
// nil, the terminals', for "terminal".
func namedTheme(name string, dark bool) *terminal.Theme {
	if name == "terminal" {
		return nil
	}
	return lookNamed(name, dark).term
}

// activeTheme is the terminal theme of windows of the appearance, dark or
// light: the window's, which c.Theme().Dark tells, never a palette's,
// which may be the other appearance's, as a dark theme's in light
// windows.
func (a *App) activeTheme(dark bool) *terminal.Theme {
	light, d := a.termThemes()
	if dark {
		return d
	}
	return light
}

// cardPalette is the palette of a card that shows code in the theme th,
// a terminal's, an editor's or the Explorer's, in a window whose palette
// is pal: the window's, when th is the window's theme, else one tinted
// with th's colors as the window's look is (tinted), with th as its
// code's. Everything on the card takes its colors from it, so that what
// paints pal.card there, as a bar, paints the theme's background.
func cardPalette(pal *palette, th *terminal.Theme) *palette {
	var p palette
	if pal.card == th.Background && pal.text == th.Foreground {
		p = *pal
	} else {
		// The window's accent, unless the card is of the other appearance,
		// where kero's own reads.
		accent := pal.accent
		if d := darkColor(th.Background); d != pal.dark {
			accent = lightPalette.accent
			if d {
				accent = darkPalette.accent
			}
		}
		p = *tinted(th, accent)
	}
	p.code = th
	return &p
}

// codeOf is the theme of code on what pal colors: the card's
// (cardPalette), else the terminals' in windows of the appearance, which
// c.Theme().Dark tells.
func (a *App) codeOf(c *ui.Context, pal *palette) *terminal.Theme {
	if pal.code != nil {
		return pal.code
	}
	return a.activeTheme(c.Theme().Dark)
}

// cardBackground is the color of a card of cardPalette's, cpal, in a
// window whose palette is pal, faded while dim: a card of the window's
// appearance lets the window show through, as kero's own does; one of the
// other's stays opaque, which the window's wash, seen through it, would
// make muddy, and its veil alone fades it.
func cardBackground(pal, cpal *palette, dim bool) ui.Color {
	if dim && cpal.dark == pal.dark {
		return cpal.cardDim
	}
	return cpal.card
}

// editorTheme is the colors of code on the terminal theme th: its
// background, text, cursor, selection and search colors, and its sixteen
// colors for the kinds of code, as editors that take a terminal's colors
// map them (Helix's, Vim's in 16 colors): Ghostty's themes have no colors
// of code of their own. The rest starts from MyGo's editor theme for ut,
// the window's, made as light or as dark as th.
func editorTheme(ut *ui.Theme, th *terminal.Theme) *editor.Theme {
	bg, fg := th.Background, th.Foreground
	u := *ut
	u.Dark, u.Background, u.Text = darkColor(bg), bg, fg
	t := editor.DefaultTheme(&u)
	muted := fg.Mix(bg, 0.45)
	// Some themes give a color of the palette the background's own, as
	// Solarized's bright black, which code in it would vanish into.
	ansi := func(i int, def ui.Color) ui.Color {
		if contrast(th.Palette[i], bg) < 2 {
			return def
		}
		return th.Palette[i]
	}
	red, green, yellow, blue := ansi(1, fg), ansi(2, fg), ansi(3, fg), ansi(4, fg)
	magenta, cyan, comment := ansi(5, fg), ansi(6, fg), ansi(8, muted)

	t.Background, t.Text = bg, fg
	t.Cursor = th.Cursor
	if t.Cursor.A == 0 {
		t.Cursor = fg
	}
	// The editor draws the selection under the text, in the text's
	// colors: a little of the background through it keeps them legible
	// on a selection meant for selection-foreground.
	sel := th.Selection
	if sel.A == 0 {
		sel = fg.Alpha(1.0 / 3)
	}
	t.Selection, t.InactiveSelection = sel.Alpha(0.7), sel.Alpha(0.4)
	if th.Search.A != 0 {
		t.Match = th.Search.Alpha(0.4)
	}
	if th.SearchSelected.A != 0 {
		t.CurrentMatch = th.SearchSelected.Alpha(0.6)
	}
	t.LineNumber, t.ActiveLineNumber = muted, fg
	t.ActiveLine, t.Occurrence, t.MatchingBracket = fg.Alpha(0.05), fg.Alpha(0.1), fg.Alpha(0.35)
	t.IndentGuide, t.ActiveIndentGuide, t.WrapGuide, t.Whitespace = fg.Alpha(0.1), fg.Alpha(0.25), fg.Alpha(0.1), fg.Alpha(0.25)
	t.Error, t.Warning, t.Info, t.Hint = red, yellow, blue, muted
	// The diffs' colors, as the Changes view's counts and badges show them
	// (diffColors).
	t.Added, t.Deleted, t.Modified = th.Palette[2], th.Palette[1], th.Palette[3]

	for k := range t.Tokens {
		t.Tokens[k].Color = ui.Color{} // the text's
	}
	for k, c := range map[editor.TokenKind]ui.Color{
		editor.TokenKeyword: magenta, editor.TokenMeta: magenta,
		editor.TokenType: yellow, editor.TokenAttribute: yellow,
		editor.TokenFunction: blue, editor.TokenHeading: blue,
		editor.TokenProperty: cyan, editor.TokenEscape: cyan, editor.TokenLink: cyan,
		editor.TokenConstant: red, editor.TokenNumber: red, editor.TokenTag: red, editor.TokenDeleted: red,
		editor.TokenString: green, editor.TokenRegexp: green, editor.TokenCode: green, editor.TokenInserted: green,
		editor.TokenComment: comment,
		editor.TokenInvalid: ansi(9, red),
	} {
		t.Tokens[k].Color = c
	}
	return t
}

// contrast is the contrast ratio of two colors, as WCAG has it: from 1,
// none, to 21.
func contrast(a, b ui.Color) float64 {
	lum := func(c ui.Color) float64 {
		ch := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	l1, l2 := lum(a), lum(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// darkColor tells that light text reads better on c, as the terminal
// tells a dark theme.
func darkColor(c ui.Color) bool {
	return 299*int(c.R)+587*int(c.G)+114*int(c.B) < 128*1000
}

// pushThemes gives the terminals their themes, as Settings previews them
// or the settings name them.
func (a *App) pushThemes() {
	light, dark := a.termThemes()
	for _, e := range a.terms {
		if e.term != nil {
			e.term.SetTheme(light, dark)
		}
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// setPreview previews themes in the terminals and Settings' own preview,
// or ends the preview with none. It keeps copies: a theme being edited
// changes under it.
func (a *App) setPreview(p themePreview) {
	terms := !sameTheme(a.preview.light, p.light) || !sameTheme(a.preview.dark, p.dark)
	files := !sameTheme(a.preview.fileLight, p.fileLight) || !sameTheme(a.preview.fileDark, p.fileDark)
	if !terms && !files {
		return
	}
	a.preview = themePreview{cloneTheme(p.light), cloneTheme(p.dark), cloneTheme(p.fileLight), cloneTheme(p.fileDark)}
	if terms {
		a.pushThemes()
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.setWin != nil {
		a.setWin.Invalidate()
	}
}

func sameTheme(a, b *terminal.Theme) bool {
	return a == b || a != nil && b != nil && *a == *b
}

func cloneTheme(t *terminal.Theme) *terminal.Theme {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

// shownTheme is the theme Settings' preview shows: the one previewed,
// for either appearance, else the window's.
func (a *App) shownTheme(dark bool) *terminal.Theme {
	switch {
	case a.preview.light != nil:
		return a.preview.light
	case a.preview.dark != nil:
		return a.preview.dark
	}
	light, d := a.chosenThemes()
	if dark {
		return d
	}
	return light
}

// listedTheme returns a theme the menus list, for windows of the
// appearance, read once while Settings is open.
func (a *App) listedTheme(name string, dark bool) *terminal.Theme {
	if id := keroID(name); id != "" {
		return keroLook(id, dark).term
	}
	if t, ok := a.ss.listed[name]; ok {
		return t
	}
	if a.ss.listed == nil {
		a.ss.listed = map[string]*terminal.Theme{}
	}
	t := namedTheme(name, dark)
	a.ss.listed[name] = t
	return t
}

// ghosttyThemesDir is where Ghostty, and so the terminal plugin, finds the
// user's themes: $XDG_CONFIG_HOME/ghostty/themes, else under %LOCALAPPDATA%
// on Windows and ~/.config elsewhere.
func ghosttyThemesDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" && runtime.GOOS == "windows" {
		dir = os.Getenv("LOCALAPPDATA")
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "ghostty", "themes")
}

// themeNames are the themes the files' theme menus offer: kero's, then
// Ghostty's and the user's.
func themeNames() []string {
	var names []string
	for _, t := range settings.KeroThemes {
		names = append(names, t.Name)
	}
	return append(names, terminal.GhosttyThemes()...)
}

// colorSettings are the Appearance page's Ghostty themes, in place of the
// theme its tiles choose: one for light windows and one for dark ones,
// each previewed, in the window and its terminals, as the pointer or the
// arrows go through its menu, and each a start for a theme of the user's.
// None goes back to kero's default.
func (a *App) colorSettings(c *ui.Context, pal *palette) {
	none := locales.SetThemeNone()
	names := append([]string{none}, terminal.GhosttyThemes()...)
	want := &a.ss.preview
	row := func(slot, label string, value *string, dark bool, preview **terminal.Theme) {
		theme := func(o string) *terminal.Theme {
			if o == none {
				o = settings.Default().LightTheme
			}
			return a.listedTheme(o, dark)
		}
		settingRow(c, pal, label, "", func() {
			shown := *value
			if keroID(shown) != "" {
				shown = none
			}
			mark := func(o string) {
				if o != none {
					themeSwatch(c, theme(o))
				}
			}
			lit := a.searchPopup(c, slot, shown, names, 240, nil, mark, pal, func(v string) {
				if v == none {
					v = settings.Default().LightTheme
				}
				*value = v
			})
			if lit != "" && lit != shown {
				*preview = theme(lit)
			}
			iconButton(c, slot+"-edit", editIcon, locales.SetCustomizeTheme(), pal, func() { a.editTheme(slot) })
		})
	}
	settingsGroup(c, pal, locales.SetColors(), locales.SetColorsDetail(proto.ShortDir(ghosttyThemesDir())), func() {
		row("light", locales.SetLightWindows(), &a.set.LightTheme, false, &want.light)
		row("dark", locales.SetDarkWindows(), &a.set.DarkTheme, true, &want.dark)
	})
	if d := a.ss.themeEdit; d != nil {
		a.themeEditor(c, pal, d)
	}
	if d := a.ss.themeEdit; d != nil {
		th := d.th
		if d.slot == "light" {
			want.light = &th
		} else {
			want.dark = &th
		}
	}
}

// fileColorSettings are the Appearance page's File Colors: the theme of
// code in editor panes and the Explorer, for light windows and for dark
// ones, the terminals' or one of their own, previewed as the terminals'
// Colors are.
func (a *App) fileColorSettings(c *ui.Context, pal *palette) {
	want := &a.ss.preview
	// The terminals' theme, by a name of its own in the menus: the
	// setting's "terminal".
	same := locales.SetSameAsTerminal()
	options := append([]string{same}, themeNames()...)
	row := func(slot, label string, value *string, dark bool, preview **terminal.Theme) {
		theme := func(o string) *terminal.Theme {
			if o == same {
				return a.activeTheme(dark)
			}
			return a.listedTheme(o, dark)
		}
		settingRow(c, pal, label, "", func() {
			shown := *value
			if shown == "terminal" {
				shown = same
			}
			mark := func(o string) { themeSwatch(c, theme(o)) }
			lit := a.searchPopup(c, slot, shown, options, 240, nil, mark, pal, func(v string) {
				if v == same {
					v = "terminal"
				}
				*value = v
			})
			if lit != "" && lit != shown {
				*preview = theme(lit)
			}
		})
	}
	settingsGroup(c, pal, locales.SetFileColors(), locales.SetFileColorsDetail(), func() {
		// As Colors' preview, the theme a menu's pointer is on.
		th := a.preview.fileLight
		if th == nil {
			th = a.preview.fileDark
		}
		if th == nil {
			th = a.fileTheme(c.Theme().Dark)
		}
		ui.Column(c).Padding(12, 0).Children(func() { a.codePreview(c, pal, th) })
		groupRows = 0
		row("file-light", locales.SetLightWindows(), &a.set.FileLightTheme, false, &want.fileLight)
		row("file-dark", locales.SetDarkWindows(), &a.set.FileDarkTheme, true, &want.fileDark)
	})
}

// codePreview shows code as editor panes and the Explorer draw it in th:
// its colors, the terminals' font a point smaller, and line numbers.
func (a *App) codePreview(c *ui.Context, pal *palette, th *terminal.Theme) {
	et := editorTheme(c.Theme(), th)
	font, size := a.termFamily(), max(a.set.FontSize-1, 10)
	kind := func(text string, k editor.TokenKind) ui.Span {
		col := et.Tokens[k].Color
		if col.A == 0 {
			col = et.Text
		}
		return ui.Span{Text: text, Color: col, Font: font, Size: size}
	}
	text := func(s string) ui.Span { return kind(s, editor.TokenText) }
	lines := [][]ui.Span{
		{kind("func ", editor.TokenKeyword), kind("greet", editor.TokenFunction), text("(name "), kind("string", editor.TokenType), text(", times "), kind("int", editor.TokenType), text(") {")},
		{text("    "), kind("// Says hello, loudly.", editor.TokenComment)},
		{text("    "), kind("return ", editor.TokenKeyword), text("strings."), kind("Repeat", editor.TokenFunction), text("("), kind(`"Hello, "`, editor.TokenString), text("+name, times"), text(")"), text(" + "), kind("1", editor.TokenNumber)},
		{text("}")},
	}
	box := raised(ui.Column(c).Radius(12).Padding(14, 18, 14, 10).Background(et.Background), pal)
	box.Children(func() {
		for i, spans := range lines {
			ui.Row(c).Gap(14).Children(func() {
				ui.Text(c, strconv.Itoa(i+1)).Font(font).FontSize(size).TextColor(et.LineNumber).Width(14).Shrink(0)
				ui.RichText(c, spans...).LineHeight(a.set.LineHeight).SingleLine()
			})
		}
	})
}

// themeSwatch draws a theme as its menu shows it: its background, with
// its text's color and six of its palette's on it.
func themeSwatch(c *ui.Context, th *terminal.Theme) {
	ui.Box(c).Size(54, 14).Radius(4).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, th.Background, 4)
		x := r.X + 5
		for _, col := range []ui.Color{th.Foreground, th.Palette[1], th.Palette[2], th.Palette[3], th.Palette[4], th.Palette[5], th.Palette[6]} {
			p.Fill(ui.Rect{X: x, Y: r.Y + 4, W: 5, H: r.H - 8}, col, 1.5)
			x += 6.5
		}
		// Seen on the menu and on the highlight alike.
		p.Stroke(r, ui.RGBA(128, 128, 128, 0.4), 4, 1)
	})
}

// themeDraft is a terminal theme the user edits in Settings, to save
// among their themes of Ghostty's and choose for slot's windows.
type themeDraft struct {
	slot     string // "light" or "dark"
	base     string // the theme it started from
	name     string
	th       terminal.Theme
	checked  string // the name problem and replaces are of
	problem  string // why the name can't be saved, "" when it can
	replaces bool   // a theme of the user's has the name
}

// editTheme starts a theme of the user's from slot's theme: the theme
// itself when it is the user's already, else a copy under a new name.
func (a *App) editTheme(slot string) {
	light, dark := a.chosenThemes()
	name, th := a.set.LightTheme, *light
	if slot == "dark" {
		name, th = a.set.DarkTheme, *dark
	}
	d := &themeDraft{slot: slot, base: name, th: editable(th)}
	switch {
	case keroID(name) != "":
		// "Paper Light", as kero names its themes.
		d.base = backgroundName(keroID(name), name)
		d.name = d.base + " Light"
		if slot == "dark" {
			d.name = d.base + " Dark"
		}
	case userTheme(name):
		d.name = name
	default:
		d.name = locales.SetThemeCopyName(name)
	}
	a.ss.themeEdit = d
}

// userTheme tells that a theme of the user's, a file in Ghostty's themes
// directory, has the name.
func userTheme(name string) bool {
	dir := ghosttyThemesDir()
	if dir == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !st.IsDir()
}

// checkThemeName tells why a theme can't be saved under name, "" when it
// can, and that it replaces a theme of the user's. A theme of Ghostty's
// can't be replaced: the user's file would hide it from every terminal
// that reads the directory.
func checkThemeName(name string) (problem string, replaces bool) {
	switch {
	case name == "":
		return locales.SetThemeNameEmpty(), false
	case strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\:*?"<>|`) || strings.ContainsFunc(name, unicode.IsControl):
		return locales.SetThemeNameInvalid(), false
	case strings.EqualFold(name, "kero") || keroID(name) != "":
		// kero's own themes go by their lowercase names (keroID).
		return locales.SetThemeNameTaken(name), false
	case userTheme(name):
		return "", true
	case slices.ContainsFunc(terminal.GhosttyThemes(), func(n string) bool { return strings.EqualFold(n, name) }):
		return locales.SetThemeNameTaken(name), false
	}
	return "", false
}

// editable returns th with each color it leaves to the terminal set to
// the one the terminal picks, so that every color has a well, and the
// theme saved draws as it showed; and every color opaque, as Ghostty's
// themes have them. Text selected keeps its own colors when a theme sets
// none: the theme's text color stands in for them.
func editable(th terminal.Theme) terminal.Theme {
	or := func(c *ui.Color, def ui.Color) {
		if c.A == 0 {
			*c = def
		}
	}
	or(&th.Cursor, th.Foreground)
	or(&th.CursorText, th.Background)
	or(&th.Selection, th.Foreground.Alpha(1.0/3).Over(th.Background))
	or(&th.SelectionText, th.Foreground)
	// The terminal's own, as Ghostty's.
	or(&th.Search, ui.Hex("#ffe082"))
	or(&th.SearchText, ui.Hex("#000000"))
	or(&th.SearchSelected, ui.Hex("#f2a57e"))
	or(&th.SearchSelectedText, ui.Hex("#000000"))
	opaque(&th)
	return th
}

// opaque makes every color of th opaque: a color picker can make one
// translucent, which a theme of Ghostty's can't hold.
func opaque(th *terminal.Theme) {
	for _, c := range themeColors(th) {
		*c = ui.RGB(c.R, c.G, c.B)
	}
}

func themeColors(th *terminal.Theme) []*ui.Color {
	cs := []*ui.Color{&th.Background, &th.Foreground, &th.Cursor, &th.CursorText, &th.Selection, &th.SelectionText,
		&th.Search, &th.SearchText, &th.SearchSelected, &th.SearchSelectedText}
	for i := range th.Palette {
		cs = append(cs, &th.Palette[i])
	}
	return cs
}

// ghosttyThemeFile writes th in Ghostty's format, as ParseGhosttyTheme
// reads it, noting the theme it was made from.
func ghosttyThemeFile(th *terminal.Theme, base string) []byte {
	hex := func(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	var b strings.Builder
	fmt.Fprintf(&b, "# Made in Kero from %s.\n", base)
	for i, c := range th.Palette {
		fmt.Fprintf(&b, "palette = %d=%s\n", i, hex(c))
	}
	for _, kv := range []struct {
		key string
		c   ui.Color
	}{
		{"background", th.Background}, {"foreground", th.Foreground},
		{"cursor-color", th.Cursor}, {"cursor-text", th.CursorText},
		{"selection-background", th.Selection}, {"selection-foreground", th.SelectionText},
		{"search-background", th.Search}, {"search-foreground", th.SearchText},
		{"search-selected-background", th.SearchSelected}, {"search-selected-foreground", th.SearchSelectedText},
	} {
		fmt.Fprintf(&b, "%s = %s\n", kv.key, hex(kv.c))
	}
	return []byte(b.String())
}

// saveTheme saves the draft among the user's themes and makes it the
// theme of its windows.
func (a *App) saveTheme(d *themeDraft) {
	name := strings.TrimSpace(d.name)
	dir := ghosttyThemesDir()
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, name), ghosttyThemeFile(&d.th, d.base), 0o644)
	}
	if err != nil {
		a.ss.toast = locales.SetThemeSaveFailed(err.Error())
		return
	}
	a.ss.themeEdit = nil
	// The file may be one already read, under the same name.
	a.ss.listed = nil
	a.themes = themeCache{}
	currentLooks.Store(nil)
	if d.slot == "light" {
		a.set.LightTheme = name
	} else {
		a.set.DarkTheme = name
	}
	a.pushThemes()
}

// themeEditor edits the draft: its name and every color, under a preview
// of it, which the terminals show too.
func (a *App) themeEditor(c *ui.Context, pal *palette, d *themeDraft) {
	opaque(&d.th)
	if d.checked != d.name {
		d.checked = d.name
		d.problem, d.replaces = checkThemeName(strings.TrimSpace(d.name))
	}
	title := locales.SetEditLightTheme()
	if d.slot == "dark" {
		title = locales.SetEditDarkTheme()
	}
	// A gray face and edge, which a white or black color stands out on.
	well := func(key string, color *ui.Color, name string) {
		ui.ColorWell(c.Key(key), color).Background(pal.text.Alpha(0.06)).Border(1, pal.text.Alpha(0.12)).Label(name).Tooltip(name)
	}
	pair := func(key, label string, first, second *ui.Color, firstName, secondName string) {
		settingRow(c, pal, label, "", func() {
			well(key+"-1", first, firstName)
			well(key+"-2", second, secondName)
		})
	}
	colors := []func() string{locales.SetColorBlack, locales.SetColorRed, locales.SetColorGreen, locales.SetColorYellow,
		locales.SetColorBlue, locales.SetColorMagenta, locales.SetColorCyan, locales.SetColorWhite}
	palette := func(key, label string, from int, bright bool) {
		settingRow(c, pal, label, "", func() {
			ui.Row(c).Gap(4).Children(func() {
				for i, name := range colors {
					n := name()
					if bright {
						n = locales.SetColorBright(n)
					}
					well(fmt.Sprint(key, i), &d.th.Palette[from+i], n)
				}
			})
		})
	}
	settingsGroup(c, pal, title, locales.SetThemeSavesTo(proto.ShortDir(ghosttyThemesDir())), func() {
		detail := d.problem
		if detail == "" && d.replaces {
			detail = locales.SetThemeReplaces()
		}
		settingRow(c, pal, locales.SetThemeName(), detail, func() {
			textField(c, "theme-name", &d.name, locales.SetThemeName(), 220, pal)
		})
		ui.Column(c).Padding(4, 0, 12, 0).Children(func() { a.terminalPreview(c, pal, &d.th) })
		groupRows = 0
		pair("base", locales.SetThemeBase(), &d.th.Background, &d.th.Foreground, locales.SetThemeBackground(), locales.SetThemeForeground())
		pair("cursor", locales.SetCursor(), &d.th.Cursor, &d.th.CursorText, locales.SetCursor(), locales.SetThemeCursorText())
		pair("sel", locales.SetThemeSelection(), &d.th.Selection, &d.th.SelectionText, locales.SetThemeSelection(), locales.SetThemeSelectionText())
		pair("match", locales.SetThemeMatches(), &d.th.Search, &d.th.SearchText, locales.SetThemeMatch(), locales.SetThemeMatchText())
		pair("current", locales.SetThemeCurrentMatch(), &d.th.SearchSelected, &d.th.SearchSelectedText, locales.SetThemeCurrentMatch(), locales.SetThemeCurrentMatchText())
		palette("normal", locales.SetThemeNormal(), 0, false)
		palette("bright", locales.SetThemeBright(), 8, true)
	})
	ui.Row(c.Key("theme-actions")).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Spacer(c)
		if button(c, "theme-cancel", locales.CommonCancel(), "", false, pal) {
			a.ss.themeEdit = nil
		}
		if button(c, "theme-save", locales.CommonSave(), "", true, pal) && d.problem == "" {
			a.saveTheme(d)
		}
	})
	opaque(&d.th)
}
