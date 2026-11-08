package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"kero/locales"
)

// Ama's window is a child window of the main one, which the user moves
// and resizes, and which keeps its place relative to the main window: its
// size and the offset of its top-left corner from the corner of the main
// window nearest it, in DIPs, which keep a size the same on a display of
// any scale (amaPlace). It goes with the main window as that moves or
// resizes, and stays inside the work area of a display as the displays
// change, without forgetting where the user put it.

// The window's size until the user gives it another, and the smallest.
const (
	amaW    = 620
	amaH    = 460
	amaMinW = 380
	amaMinH = 240
)

// amaPlace is where Ama's window is, as prefs keep it: its size, and the
// offset of its top-left corner from a corner of the main window, the
// right one's or the bottom one's as Right and Bottom tell, in DIPs.
type amaPlace struct {
	X      int  `json:"x"`
	Y      int  `json:"y"`
	Width  int  `json:"width"`
	Height int  `json:"height"`
	Right  bool `json:"right,omitempty"`
	Bottom bool `json:"bottom,omitempty"`
}

// defaultAmaPlace is the window's place until the user moves it: above
// the sessions sidebar's footer, the frog's and the search's, at the main
// window's left, or its right with the sessions there.
func defaultAmaPlace(frogLeft bool) amaPlace {
	p := amaPlace{X: 12, Y: -amaH - 84, Width: amaW, Height: amaH, Bottom: true}
	if !frogLeft {
		p.Right, p.X = true, -amaW-12
	}
	return p
}

// rect is the window's bounds, of a main window's.
func (p amaPlace) rect(main mygo.Rectangle) mygo.Rectangle {
	x, y := main.X, main.Y
	if p.Right {
		x += main.Width
	}
	if p.Bottom {
		y += main.Height
	}
	return mygo.Rectangle{X: x + p.X, Y: y + p.Y, Width: max(p.Width, amaMinW), Height: max(p.Height, amaMinH)}
}

// placeOf is the place of a window's bounds r, of a main window's: from
// the main window's corner nearest r's center, so that a window by its
// bottom edge stays there as the main window grows taller.
func placeOf(main, r mygo.Rectangle) amaPlace {
	p := amaPlace{Width: r.Width, Height: r.Height,
		Right:  2*r.X+r.Width > 2*main.X+main.Width,
		Bottom: 2*r.Y+r.Height > 2*main.Y+main.Height,
	}
	x, y := main.X, main.Y
	if p.Right {
		x += main.Width
	}
	if p.Bottom {
		y += main.Height
	}
	p.X, p.Y = r.X-x, r.Y-y
	return p
}

// onScreen moves r into the work area of the display it is the most on,
// or the nearest, as small as it has to be to fit there. In MyGo's
// screen coordinates, displays of different scales may have gaps between
// them, which a window would be lost in.
func onScreen(r mygo.Rectangle, areas []mygo.Rectangle) mygo.Rectangle {
	best, most, near := -1, 0, 0
	for i, wa := range areas {
		if wa.Width <= 0 || wa.Height <= 0 {
			continue
		}
		w := min(r.X+r.Width, wa.X+wa.Width) - max(r.X, wa.X)
		h := min(r.Y+r.Height, wa.Y+wa.Height) - max(r.Y, wa.Y)
		if w > 0 && h > 0 && w*h > most {
			best, most = i, w*h
		}
		if most == 0 {
			// Off every display: the one nearest its center.
			cx, cy := r.X+r.Width/2, r.Y+r.Height/2
			dx := max(wa.X-cx, 0, cx-(wa.X+wa.Width))
			dy := max(wa.Y-cy, 0, cy-(wa.Y+wa.Height))
			if d := dx*dx + dy*dy; best < 0 || d < near {
				best, near = i, d
			}
		}
	}
	if best < 0 {
		return r
	}
	wa := areas[best]
	r.Width, r.Height = min(r.Width, wa.Width), min(r.Height, wa.Height)
	r.X = min(max(r.X, wa.X), wa.X+wa.Width-r.Width)
	r.Y = min(max(r.Y, wa.Y), wa.Y+wa.Height-r.Height)
	return r
}

// workAreas are the displays' work areas.
func workAreas() []mygo.Rectangle {
	var out []mygo.Rectangle
	for _, d := range mygo.Screen.Displays() {
		out = append(out, d.WorkArea)
	}
	return out
}

// amaBounds are where Ama's window goes, of the main window's bounds now.
func (a *App) amaBounds() mygo.Rectangle {
	p := defaultAmaPlace(a.set.SessionsSide != "right")
	if a.prefs.AmaWindow != nil {
		p = *a.prefs.AmaWindow
	}
	return onScreen(p.rect(a.win.Bounds()), workAreas())
}

// makeAmaWindow makes Ama's window, hidden: a child of the main window,
// above it, which closes with it. Closing it hides it, Ama running on.
func (a *App) makeAmaWindow() {
	m := &a.ama
	w := mygo.NewWindow(framed(mygo.WindowOptions{
		Title:  locales.AmaTitle(),
		Parent: a.win,
		Hidden: true,
		Width:  amaW, Height: amaH, MinWidth: amaMinW, MinHeight: amaMinH,
		SkipTaskbar:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Vibrancy:          vibrancyFor(a.set),
		Content:           ui.View(a.amaView),
	}))
	m.win = w
	// Both on the main thread. Closed, it went with the main window, whose
	// updates would never run.
	w.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		a.closeAma()
	})
	w.OnClosed(func() {
		if m.win == w {
			m.win, m.open = nil, false
		}
	})
	moved := func() { a.update(a.amaMoved) }
	w.OnMove(moved)
	w.OnResize(moved)
}

// showAmaWindow shows Ama's window where it goes, making it first.
func (a *App) showAmaWindow() {
	m := &a.ama
	if a.win == nil || !a.winOpen {
		return
	}
	if m.win == nil || m.win.IsDestroyed() {
		a.makeAmaWindow()
	}
	m.placed = a.amaBounds()
	m.win.SetBounds(m.placed)
	m.win.Show()
	m.win.Focus()
}

// placeAma puts Ama's window where it goes again, as the main window
// moved or resized, or the displays changed: a move of the main window
// moves it with it, back from a display's edge, where it went, once
// there is room.
func (a *App) placeAma() {
	m := &a.ama
	if !m.open || m.win == nil || a.win == nil || a.win.IsMinimized() {
		return
	}
	r := a.amaBounds()
	if r == m.win.Bounds() {
		return
	}
	m.placed = r
	m.win.SetBounds(r)
}

// amaMoved remembers where the user moved or resized Ama's window, of the
// main window: not where placeAma put it, which is where it was, or a
// display's edge kept it from.
func (a *App) amaMoved() {
	m := &a.ama
	if !m.open || m.win == nil || a.win == nil || a.win.IsMinimized() {
		return
	}
	r := m.win.Bounds()
	if near(r, m.placed) {
		return
	}
	m.placed = r
	p := placeOf(a.win.Bounds(), r)
	a.prefs.AmaWindow = &p
	// Saved once the user lets go: a drag moves it many times a second.
	if m.save != nil {
		m.save.Stop()
	}
	m.save = time.AfterFunc(500*time.Millisecond, func() { a.update(a.savePrefs) })
}

// near tells bounds apart from those set, which a display of a scale over
// 100% may round to its pixels.
func near(r, s mygo.Rectangle) bool {
	d := func(x, y int) bool { return x-y <= 2 && y-x <= 2 }
	return d(r.X, s.X) && d(r.Y, s.Y) && d(r.Width, s.Width) && d(r.Height, s.Height)
}

// resetAmaWindow puts Ama's window back above the frog, at its first
// size.
func (a *App) resetAmaWindow() {
	a.prefs.AmaWindow = nil
	a.savePrefs()
	a.placeAma()
}
