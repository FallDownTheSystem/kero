package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// palette is kero's colors, for the light and the dark appearance.
type palette struct {
	dark bool

	sidebar      ui.Color // under the sidebar where no material shows
	text         ui.Color
	textMuted    ui.Color // project headers, times
	textFaint    ui.Color // counts, hints
	rowHover     ui.Color
	rowSelected  ui.Color
	rowSelBorder ui.Color
	rowShadow    ui.Color

	card        ui.Color // the terminal's card
	cardBorder  ui.Color
	cardShadow  ui.Color
	cardShadow2 ui.Color
	floatShadow ui.Color
	cardDim     ui.Color // a pane the user is not in
	buttonBg    ui.Color
	divider     ui.Color
	chip        ui.Color

	attention ui.Color // needs you
	attnSoft  ui.Color
	done      ui.Color // the filled check
	onDone    ui.Color
	danger    ui.Color
	accent    ui.Color

	overlay   ui.Color // behind dialogs
	popover   ui.Color
	kbd       ui.Color
	kbdBorder ui.Color
}

var lightPalette = palette{
	sidebar:      ui.Hex("#eceef4"),
	text:         ui.Hex("#1d1d1f"),
	textMuted:    ui.Hex("#6e6e78"),
	textFaint:    ui.Hex("#a3a3ad"),
	rowHover:     ui.RGBA(0, 0, 0, 0.04),
	rowSelected:  ui.Hex("#ffffff"),
	rowSelBorder: ui.RGBA(0, 0, 0, 0.05),
	rowShadow:    ui.RGBA(20, 20, 50, 0.08),

	card:        ui.Hex("#ffffff"),
	cardBorder:  ui.RGBA(0, 0, 0, 0.07),
	cardShadow:  ui.RGBA(0, 0, 0, 0.07),
	cardShadow2: ui.RGBA(0, 0, 0, 0.035),
	floatShadow: ui.RGBA(20, 20, 40, 0.14),
	cardDim:     ui.RGBA(255, 255, 255, 0.58),
	buttonBg:    ui.Hex("#ffffff"),
	divider:     ui.RGBA(0, 0, 0, 0.06),
	chip:        ui.RGBA(0, 0, 0, 0.045),

	attention: ui.Hex("#c27b0f"),
	attnSoft:  ui.RGBA(232, 154, 40, 0.13),
	done:      ui.Hex("#1d1d1f"),
	onDone:    ui.Hex("#ffffff"),
	danger:    ui.Hex("#d93a35"),
	accent:    ui.Hex("#3b6cf6"),

	overlay:   ui.RGBA(10, 10, 30, 0.12),
	popover:   ui.Hex("#ffffff"),
	kbd:       ui.RGBA(0, 0, 0, 0.04),
	kbdBorder: ui.RGBA(0, 0, 0, 0.09),
}

var darkPalette = palette{
	dark:         true,
	sidebar:      ui.Hex("#1f1f24"),
	text:         ui.Hex("#ececf0"),
	textMuted:    ui.Hex("#9a9aa6"),
	textFaint:    ui.Hex("#6c6c78"),
	rowHover:     ui.RGBA(255, 255, 255, 0.05),
	rowSelected:  ui.RGBA(255, 255, 255, 0.10),
	rowSelBorder: ui.RGBA(255, 255, 255, 0.06),
	rowShadow:    ui.RGBA(0, 0, 0, 0.25),

	card:        ui.Hex("#161618"),
	cardBorder:  ui.RGBA(255, 255, 255, 0.08),
	cardShadow:  ui.RGBA(0, 0, 0, 0.35),
	cardShadow2: ui.RGBA(0, 0, 0, 0.2),
	floatShadow: ui.RGBA(0, 0, 0, 0.45),
	cardDim:     ui.RGBA(22, 22, 24, 0.72),
	buttonBg:    ui.Hex("#2a2a2e"),
	divider:     ui.RGBA(255, 255, 255, 0.07),
	chip:        ui.RGBA(255, 255, 255, 0.07),

	attention: ui.Hex("#f0a33a"),
	attnSoft:  ui.RGBA(240, 163, 58, 0.15),
	done:      ui.Hex("#ececf0"),
	onDone:    ui.Hex("#161618"),
	danger:    ui.Hex("#ff6961"),
	accent:    ui.Hex("#6b93ff"),

	overlay:   ui.RGBA(0, 0, 0, 0.35),
	popover:   ui.Hex("#232327"),
	kbd:       ui.RGBA(255, 255, 255, 0.06),
	kbdBorder: ui.RGBA(255, 255, 255, 0.1),
}

func paletteFor(c *ui.Context) *palette {
	if c.Theme().Dark {
		return &darkPalette
	}
	return &lightPalette
}

// Terminal themes on the card's color, with palettes tuned to read on it:
// Apple's system colors, a little deeper in the light.
var lightTermTheme = &terminal.Theme{
	Foreground:    ui.Hex("#1d1d1f"),
	Background:    lightPalette.card,
	Cursor:        ui.Hex("#1d1d1f"),
	CursorText:    ui.Hex("#ffffff"),
	Selection:     ui.Hex("#cfdcfb"),
	SelectionText: ui.Hex("#1d1d1f"),
	Palette: [16]ui.Color{
		ui.Hex("#1d1d1f"), ui.Hex("#d3302f"), ui.Hex("#22863a"), ui.Hex("#b08800"),
		ui.Hex("#2f5bd8"), ui.Hex("#a033c4"), ui.Hex("#0f8199"), ui.Hex("#8e8e93"),
		ui.Hex("#5f5f66"), ui.Hex("#ef4444"), ui.Hex("#2fa84f"), ui.Hex("#d4a017"),
		ui.Hex("#3b82f6"), ui.Hex("#c04ee0"), ui.Hex("#14a3bf"), ui.Hex("#b4b4bb"),
	},
}

var darkTermTheme = &terminal.Theme{
	Foreground:    ui.Hex("#e6e6ea"),
	Background:    darkPalette.card,
	Cursor:        ui.Hex("#e6e6ea"),
	CursorText:    ui.Hex("#161618"),
	Selection:     ui.Hex("#2f4170"),
	SelectionText: ui.Hex("#ffffff"),
	Palette: [16]ui.Color{
		ui.Hex("#3a3a40"), ui.Hex("#ff6b64"), ui.Hex("#4fd17a"), ui.Hex("#f2c14e"),
		ui.Hex("#6b9bff"), ui.Hex("#d07cf0"), ui.Hex("#4cc9e0"), ui.Hex("#c9c9cf"),
		ui.Hex("#6c6c78"), ui.Hex("#ff8a84"), ui.Hex("#74e397"), ui.Hex("#ffd479"),
		ui.Hex("#8fb3ff"), ui.Hex("#e09cf7"), ui.Hex("#7fdcec"), ui.Hex("#f5f5f7"),
	},
}

// backdrop paints the window's wash of color, under the sidebar and
// between the cards, as soft as paper: lavender at the top, rose at the
// top right, warm cream at the bottom: the base on the root, the rose
// glow on row, which holds everything else. It lies over the window's
// material, which shows through a little.
func backdrop(c *ui.Context, pal *palette, row ui.Element) {
	base, glow := lightWash, lightGlow
	if pal.dark {
		base, glow = darkWash, darkGlow
	}
	alpha := float32(1)
	if c.Vibrancy() {
		alpha = 0.95
	}
	fade := func(g ui.LinearGradient) ui.LinearGradient {
		stops := make([]ui.GradientStop, len(g.Stops))
		for i, s := range g.Stops {
			stops[i] = ui.GradientStop{Color: s.Color.Alpha(float32(s.Color.A) / 255 * alpha), At: s.At}
		}
		g.Stops = stops
		return g
	}
	c.Root().LinearGradient(fade(base))
	row.LinearGradient(glow)
}

var (
	// Rex's wash: pink lavender above, rose at the top right, butter
	// cream below.
	lightWash = ui.LinearGradient{Angle: 180, Oklab: true, Stops: []ui.GradientStop{
		{Color: ui.Hex("#ebe0f3"), At: 0},
		{Color: ui.Hex("#f3e7f1"), At: 0.38},
		{Color: ui.Hex("#f7eee4"), At: 0.72},
		{Color: ui.Hex("#f6efcf"), At: 1},
	}}
	lightGlow = ui.LinearGradient{Angle: 225, Oklab: true, Stops: []ui.GradientStop{
		{Color: ui.RGBA(247, 220, 241, 0.75), At: 0},
		{Color: ui.RGBA(247, 220, 241, 0), At: 0.5},
	}}
	darkWash = ui.LinearGradient{Angle: 165, Oklab: true, Stops: []ui.GradientStop{
		{Color: ui.Hex("#1d1a26"), At: 0},
		{Color: ui.Hex("#201c24"), At: 0.5},
		{Color: ui.Hex("#22201b"), At: 1},
	}}
	darkGlow = ui.LinearGradient{Angle: 235, Oklab: true, Stops: []ui.GradientStop{
		{Color: ui.RGBA(120, 70, 120, 0.22), At: 0},
		{Color: ui.RGBA(120, 70, 120, 0), At: 0.55},
	}}
)
