package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// kero's form controls, as macOS 27 draws its own: a pop-up button, a
// rounded bezel with the up and down chevrons, opening a menu whose
// current item has a check; a text field, rounded, with a faint stroke
// inside and the accent's glow around it while it has the focus; and a
// combo field, a text field with a menu that its text filters.

var (
	updownIcon = glyph("f7-chevron-up-chevron-down")
	checkIcon  = glyph("f7-checkmark-alt")
)

const controlH = 24 // the height of macOS's regular controls

// popup chooses *value among options, labeled by label, as a pop-up
// button. It binds *value itself: the choice lands in it.
func popup[T comparable](c *ui.Context, key string, value *T, options []T, label func(T) string, width float32, pal *palette) ui.Element {
	sel := ui.SelectBase(c.Key(key), value)
	trig := sel.Trigger.Height(controlH).Width(width).Padding(0, 6, 0, 10).Gap(6).Radius(7).AlignItems(ui.Center)
	bg := pal.control
	if trig.Hovered() || sel.Open() {
		bg = pal.controlHover
	}
	face(trig, pal, bg)
	trig.Children(func() {
		ui.Text(c, label(*value)).FontSize(13).TextColor(pal.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		ui.Icon(c, updownIcon).Size(9, 11).TextColor(pal.textMuted)
	})
	sel.Popup(func(panel ui.Element) {
		menuPanel(panel, pal).MinWidth(width)
		for _, o := range options {
			menuItem(c, sel.Item(o), label(o), o == *value, pal)
		}
	})
	return trig
}

// menuPanel styles a menu: rounded, lifted off the window.
func menuPanel(panel ui.Element, pal *palette) ui.Element {
	return floating(panel.Margin(4, 0, 0, 0).Padding(5).Radius(11).Background(pal.popover), pal)
}

// menuItem styles an item of a menu: a check by the current one, and the
// accent behind the one the pointer or the arrows are on.
func menuItem(c *ui.Context, item ui.Element, label string, current bool, pal *palette) {
	item.Height(24).Padding(0, 10, 0, 6).Gap(5).Radius(6).AlignItems(ui.Center)
	fg := pal.text
	if item.Highlighted() {
		item.Background(pal.accent)
		fg = ui.Hex("#ffffff")
	}
	item.TextColor(fg).Children(func() {
		mark := ui.Icon(c, checkIcon).Size(11, 11)
		if !current {
			mark.Opacity(0)
		}
		ui.Text(c, label).FontSize(13).SingleLine()
	})
}

// field styles a text field's frame, around its input.
func field(c *ui.Context, frame ui.Element, focused bool, pal *palette) ui.Element {
	frame.Height(controlH+2).Padding(0, 8).Gap(6).Radius(7).AlignItems(ui.Center)
	stroke := ui.RGBA(0, 0, 0, 0.13)
	bg := ui.Hex("#ffffff")
	if pal.dark {
		stroke, bg = ui.RGBA(255, 255, 255, 0.13), ui.RGBA(255, 255, 255, 0.06)
	}
	frame.Background(bg).InsetShadow(0, 0, 0, 1, stroke)
	glow := frame.Animate("glow", b2f(focused), 120*time.Millisecond)
	if glow > 0 {
		frame.Shadow(0, 0, 0, 3.5*glow, pal.accent.Alpha(0.32*glow)).InsetShadow(0, 0, 0, 1, pal.accent.Alpha(0.75*glow))
	}
	return frame
}

// textField edits *value in a field, returning its input, for
// Submitted, OnSubmit and Focused.
func textField(c *ui.Context, key string, value *string, placeholder string, width float32, pal *palette) ui.Element {
	var in ui.Element
	frame := ui.Row(c.Key(key)).Cursor(ui.CursorText)
	if width > 0 {
		frame.Width(width)
	} else {
		frame.Grow(1)
	}
	frame.Children(func() {
		in = ui.TextInputBase(c.Key("in"), value).Grow(1).MinWidth(0).FontSize(13).Placeholder(placeholder).Label(placeholder)
	})
	field(c, frame, in.Focused(), pal)
	return in
}

// comboField chooses value among options, which typing in its field
// filters, calling choose with the choice.
func (a *App) comboField(c *ui.Context, key, value string, options []string, width float32, pal *palette, choose func(string)) {
	if a.ss.combo == nil {
		a.ss.combo = map[string]*string{}
	}
	text := a.ss.combo[key]
	if text == nil {
		text = new(string)
		a.ss.combo[key] = text
	}
	cb := ui.ComboboxBase(c.Key(key), text)
	if !cb.Open() && !cb.Input.Focused() {
		*text = value
	}
	cb.Input.Width(width).FontSize(13)
	field(c, cb.Input, cb.Input.Focused() || cb.Open(), pal).Padding(0, 26, 0, 8)
	muted := pal.textMuted
	cb.Input.DrawOver(func(p *ui.Painter, r ui.Rect) {
		p.Icon(updownIcon, ui.Rect{X: r.X + r.W - 18, Y: r.Y + (r.H-11)/2, W: 9, H: 11}, muted)
	})
	if v, ok := cb.Chosen(); ok && v != value {
		choose(v)
	}
	cb.Popup(func(panel ui.Element) {
		menuPanel(panel, pal).MinWidth(width)
		q := strings.ToLower(strings.TrimSpace(*text))
		ui.Scroll(c.Key("items")).MaxHeight(280).Children(func() {
			n := 0
			for _, o := range options {
				if cb.Filtering() && q != "" && !strings.Contains(strings.ToLower(o), q) {
					continue
				}
				if n++; n > 300 {
					break
				}
				menuItem(c, cb.Item(o), o, o == value, pal)
			}
		})
	})
}
