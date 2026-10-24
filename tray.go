package main

import (
	_ "embed"
	"runtime"

	"github.com/egoist/mygo"

	"kero/locales"
)

// On Windows, closing Kero's window while sessions of this machine run
// hides it in the notification area instead: kerod runs them on, and
// holds Kero.exe, unseen otherwise, so that an installer can't replace it
// and nothing tells why. The icon opens the window again, and its menu
// quits, keeping the sessions, or ends them and kerod with the app.

//go:embed icons/tray.png
var trayIcon []byte

// toTray tells that closing the window keeps Kero in the tray while
// sessions run: macOS keeps an app without windows in the Dock.
var toTray = runtime.GOOS == "windows"

// keepInTray makes closing the window hide it in the tray while this
// machine's sessions run; a quit closes it.
func (a *App) keepInTray() {
	if !toTray {
		return
	}
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting || len(a.sessionsOf(a.local())) == 0 {
			return
		}
		e.PreventDefault()
		a.win.Hide()
		a.showTray()
	})
}

// showTray shows the tray's icon, while the window is hidden.
func (a *App) showTray() {
	n := len(a.sessionsOf(a.local()))
	if a.tray != nil {
		a.tray.SetToolTip(locales.TrayTip(n))
		return
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayOpen() + labelPad, Click: func(*mygo.MenuItem, *mygo.Window) { a.update(a.showWindow) }},
		mygo.Separator(),
		{Label: locales.TrayQuit() + labelPad, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.update(func() {
				a.quitting = true
				mygo.App.Quit()
			})
		}},
		{Label: locales.ActionQuitAll() + labelPad, Click: func(*mygo.MenuItem, *mygo.Window) {
			// It asks first, in the window.
			a.update(func() {
				a.showWindow()
				a.quitAndEnd()
			})
		}},
	})
	t, err := mygo.NewTray(mygo.TrayOptions{ID: "kero", Icon: trayIcon, ToolTip: locales.TrayTip(n), Menu: menu})
	if err != nil {
		// No icon to bring it back by: the window stays.
		a.win.Show()
		return
	}
	t.OnClick(func() { a.update(a.showWindow) })
	a.tray = t
}

// showWindow shows the window, hidden in the tray or minimized, in front,
// and takes the tray's icon away.
func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	a.win.Show()
	// Restoring a maximized window would make it a normal one.
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Focus()
	if a.tray != nil {
		a.tray.Destroy()
		a.tray = nil
	}
}
