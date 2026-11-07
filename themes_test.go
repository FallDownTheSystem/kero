package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestThemeFileRoundTrip(t *testing.T) {
	th := editable(*darkTermTheme)
	th.Palette[3] = ui.RGBA(10, 20, 30, 0.5) // a picker's translucent color
	opaque(&th)
	got, err := terminal.ParseGhosttyTheme(ghosttyThemeFile(&th, themeExtras{}, "Kero"))
	if err != nil {
		t.Fatal(err)
	}
	if *got != th {
		t.Errorf("read back\n%+v\nwant\n%+v", *got, th)
	}
}

// A theme's accent and background glow go in lines Ghostty takes as
// comments, which kero reads back; ones the theme gives anyway aren't
// written, so they follow its cursor.
func TestThemeExtras(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := ghosttyThemesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	th := editable(*darkTermTheme)
	x := themeExtras{accent: ui.Hex("#5a6f9e"), glow: ui.Hex("#c0306a")}
	data := ghosttyThemeFile(&th, x, "Kero")
	if got, err := terminal.ParseGhosttyTheme(data); err != nil || *got != th {
		t.Fatalf("the extras' lines changed the theme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Mine"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readThemeExtras("Mine"); got != x {
		t.Errorf("extras %+v, want %+v", got, x)
	}
	l := lookNamed("Mine", true)
	if l.glow.Stops[0].Color.R != x.glow.R || l.glow.Stops[0].Color.B != x.glow.B {
		t.Errorf("the look glows %v", l.glow.Stops[0].Color)
	}
	if l.pal.accent != x.accent || l.term.Cursor != th.Cursor {
		t.Errorf("accent %v, cursor %v: want the accent saved, the cursor kept", l.pal.accent, l.term.Cursor)
	}

	// A glow of the accent chosen follows the accent, not the cursor.
	if got := string(ghosttyThemeFile(&th, themeExtras{accent: x.accent, glow: x.accent}, "Kero")); strings.Contains(got, glowLine) {
		t.Errorf("the accent's glow was written:\n%s", got)
	}

	plain := ghosttyThemeFile(&th, themeExtras{accent: themeAccent(&th), glow: themeAccent(&th)}, "Kero")
	if strings.Contains(string(plain), glowLine) || strings.Contains(string(plain), accentLine) {
		t.Errorf("the cursor's extras were written:\n%s", plain)
	}
	if err := os.WriteFile(filepath.Join(dir, "Plain"), plain, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readThemeExtras("Plain"); got != (themeExtras{}) {
		t.Errorf("a theme with no extras' lines has %+v", got)
	}
	if got := readThemeExtras("Dracula"); got != (themeExtras{}) {
		t.Errorf("a theme of Ghostty's has %+v", got)
	}
}

func TestEditableFillsTerminalDefaults(t *testing.T) {
	th := editable(terminal.Theme{Foreground: ui.Hex("#ffffff"), Background: ui.Hex("#000000")})
	for i, c := range themeColors(&th) {
		if c.A != 255 {
			t.Errorf("color %d left unset: %+v", i, *c)
		}
	}
	if th.Cursor != th.Foreground || th.CursorText != th.Background {
		t.Errorf("cursor %v on %v, want the text's colors swapped", th.Cursor, th.CursorText)
	}
}

func TestCheckThemeName(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	dir := filepath.Join(config, "ghostty", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Mine"), []byte("background = #101010\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		ok, replaces bool
	}{
		{"New Theme", true, false},
		{"Mine", true, true},
		{"Dracula", false, false},
		{"dracula", false, false},
		{"Kero", false, false},
		{"paper", false, false}, // kero's
		{"dusk", false, false},
		{"", false, false},
		{".hidden", false, false},
		{"a/b", false, false},
		{`a\b`, false, false},
		{"a:b", false, false},
	} {
		problem, replaces := checkThemeName(tc.name)
		if (problem == "") != tc.ok || replaces != tc.replaces {
			t.Errorf("checkThemeName(%q) = %q, %v; want ok %v, replaces %v", tc.name, problem, replaces, tc.ok, tc.replaces)
		}
	}
}

// The theme the arrows or the pointer are on in a Ghostty themes' menu
// shows in the terminals and colors the window until the menu closes,
// without being chosen.
func TestThemeMenuPreviews(t *testing.T) {
	a := testApp(t)
	a.ss.page = "appearance"
	tt := ui.NewTester(a.settingsView, 820, 1900)
	tt.Frame()
	if err := tt.Click("None"); err != nil { // the light windows' menu
		t.Fatal(err)
	}
	tt.Frame()
	if a.preview.light != nil {
		t.Errorf("the theme chosen is previewed as the menu opens")
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Frame()
	shot(t, tt, "theme-menu")
	names := terminal.GhosttyThemes()
	want, _ := terminal.GhosttyTheme(names[0])
	if light, _ := a.termThemes(); a.preview.light == nil || *light != *want {
		t.Fatalf("previewing %+v, want %s", a.preview.light, names[0])
	}
	if card := a.looks().light.pal.card; card != want.Background {
		t.Errorf("the window's cards %v while previewing, want the theme's %v", card, want.Background)
	}
	if a.set.LightTheme != "paper" {
		t.Errorf("light theme %q chosen by looking", a.set.LightTheme)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.preview.light != nil {
		t.Errorf("still previewing after the menu closed")
	}
	if a.looks().light != keroLook("paper", false) {
		t.Errorf("the window keeps the look previewed after the menu closed")
	}
}

// A terminal pane's card is its theme's background: the terminal draws on
// the card, and a card of kero's own color showed every theme on it.
func TestTerminalCardTakesThemeBackground(t *testing.T) {
	a := testApp(t)
	a.set.LightTheme = "Dracula"
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	shot(t, tt, "card-dracula")
	want, _ := terminal.GhosttyTheme("Dracula")
	r, g, b, _ := tt.Image().At(480, 400).RGBA()
	got := ui.RGB(uint8(r>>8), uint8(g>>8), uint8(b>>8))
	near := func(x, y uint8) bool { return x-y < 3 || y-x < 3 }
	if !near(got.R, want.Background.R) || !near(got.G, want.Background.G) || !near(got.B, want.Background.B) {
		t.Errorf("the card is %v, want Dracula's background %v", got, want.Background)
	}
}

func TestThemeEditorShows(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp(t)
	a.ss.page = "appearance"
	a.editTheme("light")
	tt := ui.NewTester(a.settingsView, 820, 3000)
	tt.Frame()
	shot(t, tt, "theme-editor")
	for _, txt := range []string{"Theme for Light Windows", "Normal colors", "Bright colors", "Save"} {
		if !tt.HasText(txt) {
			t.Errorf("no %q; texts %q", txt, tt.Texts())
		}
	}
	if a.preview.light == nil {
		t.Errorf("the theme edited is not previewed")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.ss.themeEdit != nil || a.preview.light != nil {
		t.Errorf("Cancel kept the draft or its preview")
	}
}

// Code takes the terminal theme's colors, but none the background hides,
// as Solarized Dark's bright black, its comments' color, is its background.
func TestEditorThemeFromTerminal(t *testing.T) {
	th, err := terminal.GhosttyTheme("Dracula")
	if err != nil {
		t.Fatal(err)
	}
	et := editorTheme(ui.DarkTheme(), th)
	if et.Background != th.Background || et.Text != th.Foreground {
		t.Errorf("code on %v in %v, want the theme's %v and %v", et.Background, et.Text, th.Background, th.Foreground)
	}
	if et.Tokens[editor.TokenKeyword].Color != th.Palette[5] || et.Tokens[editor.TokenString].Color != th.Palette[2] {
		t.Errorf("keywords %v, strings %v: not the theme's magenta and green", et.Tokens[editor.TokenKeyword].Color, et.Tokens[editor.TokenString].Color)
	}
	if et.Tokens[editor.TokenComment].Color != th.Palette[8] {
		t.Errorf("comments %v, want bright black %v", et.Tokens[editor.TokenComment].Color, th.Palette[8])
	}
	hidden := *th
	hidden.Palette[8] = hidden.Background
	et = editorTheme(ui.LightTheme(), &hidden)
	if c := et.Tokens[editor.TokenComment].Color; contrast(c, hidden.Background) < 2 {
		t.Errorf("comments %v vanish into the background %v", c, hidden.Background)
	}
}

// The files' colors are an Appearance setting, apart from the terminals'.
func TestFileColorsOnAppearance(t *testing.T) {
	for page, want := range map[string]bool{"appearance": true, "terminal": false} {
		a := testApp(t)
		a.ss.page = page
		tt := ui.NewTester(a.settingsView, 820, 1400)
		tt.Frame()
		shot(t, tt, "file-colors-"+page)
		if got := tt.HasText("File Colors"); got != want {
			t.Errorf("%s page shows File Colors: %v, want %v", page, got, want)
		}
	}
}

// A card takes its theme's background, in the palette that reads on it.
func TestCardPalette(t *testing.T) {
	dracula, _ := terminal.GhosttyTheme("Dracula")
	cpal := cardPalette(&lightPalette, dracula)
	if cpal.card != dracula.Background || !cpal.dark || cpal.code != dracula {
		t.Errorf("card %v, dark %v: want Dracula's background, in the dark palette", cpal.card, cpal.dark)
	}
	if cpal := cardPalette(&darkPalette, darkTermTheme); cpal.card != darkPalette.card || cpal.cardDim != darkPalette.cardDim {
		t.Errorf("kero's own theme changed the dark card")
	}
}

// The files follow the terminals' theme, previewed or chosen, until they
// have one of their own.
func TestFileTheme(t *testing.T) {
	a := testApp(t)
	a.set.DarkTheme = "Dracula"
	dracula, _ := terminal.GhosttyTheme("Dracula")
	if th := a.fileTheme(true); *th != *dracula {
		t.Errorf("files in %+v, want the terminals' Dracula", th)
	}
	nord, _ := terminal.GhosttyTheme("Nord")
	a.setPreview(themePreview{dark: nord})
	if th := a.fileTheme(true); *th != *nord {
		t.Errorf("files in %+v, want the terminals' previewed Nord", th)
	}
	a.setPreview(themePreview{})
	a.set.FileDarkTheme = "plain"
	if th := a.fileTheme(true); th != darkTermTheme {
		t.Errorf("files in %+v, want plain's, kero's own", th)
	}
	if light := a.fileTheme(false); light != keroLook("paper", false).term {
		t.Errorf("light files in %+v, want the terminals' paper", light)
	}
}

func TestSaveThemeChoosesIt(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	a := testApp(t)
	a.editTheme("dark")
	d := a.ss.themeEdit
	if d == nil || d.name != "Paper Dark" || d.base != "Paper" {
		t.Fatalf("draft %+v, want one named Paper Dark", d)
	}
	d.th.Background = ui.Hex("#123456")
	d.extras = themeExtras{accent: ui.Hex("#5a6f9e"), glow: ui.Hex("#c0306a")}
	a.saveTheme(d)
	if a.ss.themeEdit != nil {
		t.Fatalf("draft kept after saving: %s", a.ss.toast)
	}
	if a.set.DarkTheme != "Paper Dark" {
		t.Errorf("dark theme %q, want Paper Dark", a.set.DarkTheme)
	}
	if _, dark := a.termThemes(); dark.Background != ui.Hex("#123456") {
		t.Errorf("dark background %v, want the one saved", dark.Background)
	}
	// Edited again, it is the user's theme: saved in place.
	a.editTheme("dark")
	if a.ss.themeEdit.name != "Paper Dark" || !userTheme("Paper Dark") {
		t.Errorf("draft named %q, want the theme itself", a.ss.themeEdit.name)
	}
	if x := a.ss.themeEdit.extras; x != (themeExtras{accent: ui.Hex("#5a6f9e"), glow: ui.Hex("#c0306a")}) {
		t.Errorf("draft extras %+v, want the ones saved", x)
	}
}
