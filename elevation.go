package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Surfaces stand out by their color and an edge, crisp, on whole pixels.
// In the light, the edge is a small soft shadow and no line, but for the
// panes', a hairline (paned); in the dark,
// where shadows hardly show, a hairline a little lighter than the
// surface, over a dark shadow, as a lit edge. These are kero's levels.

// raised lifts a surface off what is under it a little: the card, the
// selected row, buttons.
func raised(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return darkShadow(litEdge(e, pal))
	}
	return e.Shadow(0, 1, 2, 0, pal.cardShadow).Shadow(0, 1, 6, 0, pal.cardShadow2)
}

// paned edges a pane's card, and the settings', flat on the wash: in the
// light a hairline and no shadow, as Rex's panes; in the dark, as raised.
func paned(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return raised(e, pal)
	}
	// Measured from Rex's at 2x: a darker outer pixel, a lighter inner.
	return e.Border(0.5, ui.RGBA(0, 0, 0, 0.106)).InsetShadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.063))
}

// chosen lifts the chosen row of a list, the sidebars', a hair: an even
// ring a shade darker than what is under it, and a faint shadow close
// under it, as Rex's rows, lighter than a card's. In the dark, as raised,
// but its lit edge is drawn by inner shadows, not a border: a border takes
// room, and would move the chosen row's content a pixel off its
// neighbours', as a history row's graph off the lanes above and below.
func chosen(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return darkShadow(e.InsetShadow(0, 0, 0, 1, pal.text.Alpha(0.06)).InsetShadow(0, 1, 0, 0, pal.text.Alpha(0.12)))
	}
	return e.Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06)).Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.05))
}

// menu lifts a menu as macOS 27 does, measured from its own: a fill a
// shade off white, lighter at the top; a dark hairline, a pixel of about
// 30% black, with a pixel of white inside it top and bottom; and a soft
// shadow, a little
// below, darkening what is under the menu's edge by about 9% at its sides
// and 16% under it, gone some 9 DIPs out.
func menu(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return e.Background(pal.popover).InsetShadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.12)).
			Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.25)).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.4))
	}
	return e.LinearGradient(ui.LinearGradient{Angle: 180, From: ui.Hex("#fbfbfb"), To: ui.Hex("#f4f4f5")}).
		InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.9)).InsetShadow(0, -1, 0, 0, ui.RGBA(255, 255, 255, 0.9)).
		InsetShadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.3)).
		Shadow(0, 5, 10, 0, ui.RGBA(0, 0, 0, 0.22))
}

// floating lifts a surface above the window: the palette, dialogs.
func floating(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return darkShadow(litEdge(e, pal)).Shadow(0, 16, 40, 0, pal.floatShadow)
	}
	return e.Shadow(0, 1, 3, 0, pal.cardShadow).Shadow(0, 16, 40, 0, pal.floatShadow)
}

// litEdge draws a dark surface's edge as Rex's docs draw their code
// blocks: a 1px border in the text's color, 18% at the top fading
// evenly to 6% at the bottom, as if lit from above.
func litEdge(e ui.Element, pal *palette) ui.Element {
	return e.Border(1, pal.text.Alpha(0.12)).BorderLinearGradient(ui.LinearGradient{
		Angle: 180, From: pal.text.Alpha(0.18), To: pal.text.Alpha(0.06),
	})
}

// darkShadow is Rex's surface shadow in the dark: three tight layers.
func darkShadow(e ui.Element) ui.Element {
	return e.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.2)).Shadow(0, 2, 2, -1, ui.RGBA(0, 0, 0, 0.2)).Shadow(0, 4, 6, -3, ui.RGBA(0, 0, 0, 0.24))
}

// face styles a control's face: see palette.control.
func face(e ui.Element, pal *palette, bg ui.Color) ui.Element {
	e.Background(bg)
	if pal.controlShadow {
		return e.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.13)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.05))
	}
	return darkShadow(litEdge(e, pal))
}

// dialogButtons is a dialog's row of buttons, at its end, which wraps
// onto more lines when a label, as a long branch's name, leaves no room
// for them on one.
func dialogButtons(c *ui.Context, fn func()) ui.Element {
	return ui.Row(c).Gap(8).Wrap().Justify(ui.End).Margin(14, 0, 0, 0).Children(fn)
}

// buttonsWidth is how wide a row of buttons, as button draws each, is on
// one line, for a dialog to be as wide; each of keys is its button's,
// "" for none.
func buttonsWidth(c *ui.Context, labels, keys []string) float32 {
	var w float32
	for i, label := range labels {
		lw, _ := c.MeasureText(0, ui.Span{Text: label, Size: 13, Weight: 500})
		w += lw + 28
		if keys[i] != "" {
			kw, _ := c.MeasureText(0, ui.Span{Text: keys[i], Size: 12})
			w += kw + 8
		}
	}
	return w + 8*float32(len(labels)-1)
}

// button is kero's button: a raised surface, or the accent's for the
// primary one, with a label and, optionally, its keys.
func button(c *ui.Context, key, label, keys string, primary bool, pal *palette) bool {
	return sizedButton(c, key, label, keys, primary, 30, 8, pal)
}

// defaultButton is button, focused when its dialog first appears, so that
// Return presses it.
func defaultButton(c *ui.Context, key, label, keys string, primary bool, pal *palette) bool {
	return sizedButtonAt(c, key, label, keys, primary, 30, 8, pal, true)
}

// fieldButton is a button beside a text field, of the field's height and
// corners, as a form's Add.
func fieldButton(c *ui.Context, key, label string, primary bool, pal *palette) bool {
	return sizedButton(c, key, label, "", primary, controlH+2, 7, pal)
}

func sizedButton(c *ui.Context, key, label, keys string, primary bool, height, radius float32, pal *palette) bool {
	return sizedButtonAt(c, key, label, keys, primary, height, radius, pal, false)
}

func sizedButtonAt(c *ui.Context, key, label, keys string, primary bool, height, radius float32, pal *palette, focus bool) bool {
	b := ui.ButtonBase(c.Key(key)).Height(height).Padding(0, 14).Gap(8).Radius(radius).AlignItems(ui.Center).MinWidth(0).Label(label)
	if focus {
		b.AutoFocus()
	}
	bg, fg := pal.control, pal.text
	switch {
	case primary:
		bg, fg = accentFace(pal, b.Hovered(), b.Pressed()), pal.onAccent
	case b.Pressed():
		bg = bg.Mix(ui.Hex("#000000"), 0.08)
	case b.Hovered():
		bg = pal.controlHover
	}
	face(b, pal, b.AnimateColor("bg", bg, ui.Animation{Duration: 90e6})).TextColor(fg)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…").MinWidth(0)
		if keys != "" {
			ui.Text(c, keys).FontSize(12).TextColor(fg.Alpha(0.55)).SingleLine().Shrink(0)
		}
	})
	return b.Clicked()
}

// accentFace is the face of a primary control, the highlight's
// background, lighter under the pointer and darker pressed; its text is
// pal.onAccent.
func accentFace(pal *palette, hovered, pressed bool) ui.Color {
	switch {
	case pressed:
		return pal.accentBg.Mix(ui.Hex("#000000"), 0.08)
	case hovered:
		return pal.accentBg.Mix(ui.Hex("#ffffff"), 0.08)
	}
	return pal.accentBg
}

// highlightRow gives a menu's row the highlight's face, as the pointer or
// the arrows are on it, and returns its text's colors: plain and faint.
func highlightRow(row ui.Element, pal *palette) (fg, faint ui.Color) {
	row.Background(pal.accentBg)
	return pal.onAccent, pal.onAccent.Alpha(0.75)
}

// borderless returns the theme with kero's look: its accent, and no lines
// around controls, which stand out by their fill: white fields and
// buttons on soft panels.
func borderless(c *ui.Context, pal *palette) *ui.Theme {
	t := *c.Theme()
	// MyGo draws its highlighted menu rows and checked boxes in Accent,
	// with AccentText on them: kero's faces of the highlight. Its carets,
	// links and focused inputs' borders are lines, in LineAccent: the
	// highlight itself, which a light background of it would wash out.
	t.Accent, t.AccentText = pal.accentBg, pal.onAccent
	t.LineAccent = pal.accent
	t.AccentHover, t.AccentPressed = accentFace(pal, true, false), accentFace(pal, false, true)
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
		track = pal.accentBg
	}
	sw.Background(sw.AnimateColor("track", track, ui.Animation{Duration: 150 * time.Millisecond}))
	x := sw.AnimateTo("knob", b2f(*on)*16, ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.15}})
	sw.Children(func() {
		ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex("#ffffff")).Translate(x, 0).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.22)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06))
	})
	return sw
}
