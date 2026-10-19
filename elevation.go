package main

import "github.com/egoist/mygo/ui"

// kero draws no border lines: surfaces stand out by their color and a
// small, soft shadow, crisp at their edge. These are its levels.

// raised lifts a surface off what is under it a little: the card, the
// selected row, buttons.
func raised(e ui.Element, pal *palette) ui.Element {
	return e.Shadow(0, 1, 2, 0, pal.cardShadow).Shadow(0, 1, 6, 0, pal.cardShadow2)
}

// floating lifts a surface above the window: the palette, dialogs.
func floating(e ui.Element, pal *palette) ui.Element {
	return e.Shadow(0, 1, 3, 0, pal.cardShadow).Shadow(0, 16, 40, 0, pal.floatShadow)
}

// button is kero's button: a raised surface, or the accent's for the
// primary one, with a label and, optionally, its keys.
func button(c *ui.Context, key, label, keys string, primary bool, pal *palette) bool {
	b := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 14).Gap(8).Radius(8).AlignItems(ui.Center).Label(label)
	bg, fg := pal.buttonBg, pal.text
	if primary {
		bg, fg = pal.accent, ui.Hex("#ffffff")
	}
	switch {
	case b.Pressed():
		bg = bg.Mix(ui.Hex("#000000"), 0.08)
	case b.Hovered():
		bg = bg.Mix(ui.Hex("#000000"), 0.04)
		if pal.dark {
			bg = bg.Mix(ui.Hex("#ffffff"), 0.06)
		}
	}
	raised(b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 90e6})), pal).TextColor(fg)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine()
		if keys != "" {
			ui.Text(c, keys).FontSize(12).TextColor(fg.Alpha(0.55)).SingleLine()
		}
	})
	return b.Clicked()
}
