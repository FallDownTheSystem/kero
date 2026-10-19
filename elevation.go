package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Surfaces stand out by their color and an edge, crisp, on whole pixels.
// In the light, the edge is a small soft shadow and no line; in the dark,
// where shadows hardly show, a hairline a little lighter than the
// surface, over a dark shadow, as a lit edge. These are kero's levels.

// raised lifts a surface off what is under it a little: the card, the
// selected row, buttons.
func raised(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return e.Border(1, pal.edge).Shadow(0, 1, 3, 0, pal.cardShadow)
	}
	return e.Shadow(0, 1, 2, 0, pal.cardShadow).Shadow(0, 1, 6, 0, pal.cardShadow2)
}

// floating lifts a surface above the window: the palette, dialogs.
func floating(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		e.Border(1, pal.edgeHigh)
	}
	return e.Shadow(0, 1, 3, 0, pal.cardShadow).Shadow(0, 16, 40, 0, pal.floatShadow)
}

// face styles a control's face: see palette.control.
func face(e ui.Element, pal *palette, bg ui.Color) ui.Element {
	e.Background(bg)
	if pal.controlShadow {
		return e.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.13)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.05))
	}
	return e.Border(1, pal.edgeHigh).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.25))
}

// button is kero's button: a raised surface, or the accent's for the
// primary one, with a label and, optionally, its keys.
func button(c *ui.Context, key, label, keys string, primary bool, pal *palette) bool {
	b := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 14).Gap(8).Radius(8).AlignItems(ui.Center).Label(label)
	bg, fg := pal.control, pal.text
	if primary {
		bg, fg = pal.accent, ui.Hex("#ffffff")
	}
	switch {
	case b.Pressed():
		bg = bg.Mix(ui.Hex("#000000"), 0.08)
	case b.Hovered() && primary:
		bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
	case b.Hovered():
		bg = pal.controlHover
	}
	face(b, pal, b.AnimateColor("bg", bg, ui.Animation{Duration: 90e6})).TextColor(fg)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine()
		if keys != "" {
			ui.Text(c, keys).FontSize(12).TextColor(fg.Alpha(0.55)).SingleLine()
		}
	})
	return b.Clicked()
}

// borderless returns the theme with kero's look: its accent, and no lines
// around controls, which stand out by their fill: white fields and
// buttons on soft panels.
func borderless(c *ui.Context, pal *palette) *ui.Theme {
	t := *c.Theme()
	t.Accent = pal.accent
	t.Selection = pal.accent.Alpha(0.25)
	t.Border = ui.Transparent
	t.Surface = pal.control
	t.SurfaceHover = pal.controlHover
	t.SurfacePressed = pal.controlHover.Mix(pal.text, 0.06)
	return &t
}

// toggle is kero's switch: a track that shows on and off alike, the
// accent's when on, with a knob that springs across. It returns the
// switch, for OnChange.
func toggle(c *ui.Context, on *bool, pal *palette) ui.Element {
	sw := ui.SwitchBase(c, on).Size(38, 22).Radius(11).Padding(2).Shrink(0)
	track := pal.text.Alpha(0.14)
	if pal.dark {
		track = ui.RGBA(255, 255, 255, 0.2)
	}
	if *on {
		track = pal.accent
	}
	sw.Background(sw.AnimateColor("track", track, ui.Animation{Duration: 150 * time.Millisecond}))
	x := sw.AnimateTo("knob", b2f(*on)*16, ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.15}})
	sw.Children(func() {
		ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex("#ffffff")).Translate(x, 0).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.22)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06))
	})
	return sw
}
