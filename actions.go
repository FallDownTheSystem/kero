package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"kero/internal/actions"
	"kero/internal/proto"
)

// run runs one of the app's actions, from a menu, the palette or
// `kero do`, on the main thread.
func (a *App) run(id string, args map[string]string) {
	s := a.session(a.selected)
	switch id {
	case "settings.open":
		a.openSettings("")
	case "cli.install":
		a.installCLI()
	case "app.quitAll":
		a.quitAndEnd()
	case "session.new":
		if dir := args["dir"]; dir != "" {
			a.newSessionIn(dir)
		} else {
			a.newSession(args["project"])
		}
	case "session.newHome":
		a.newSessionIn(proto.Home())
	case "project.open":
		a.openProject()
	case "palette.open":
		a.openPalette()
	case "session.attention":
		a.jumpToAttention()
	case "session.next":
		a.selectOffset(1)
	case "session.previous":
		a.selectOffset(-1)
	case "session.rename":
		a.startRename(a.selected)
	case "pane.restart":
		if s != nil {
			a.restartSession(a.focusOf(s))
		}
	case "pane.close":
		a.closeFocused()
	case "session.close":
		a.closeSession(a.selected, true)
	case "pane.split.right":
		a.splitPane(s, "", "right")
	case "pane.split.down":
		a.splitPane(s, "", "down")
	case "pane.zoom":
		a.zoomPane(s, "")
	case "pane.next":
		a.cyclePane(1)
	case "pane.previous":
		a.cyclePane(-1)
	case "pane.left", "pane.right", "pane.up", "pane.down":
		a.focusDirection(id[len("pane."):])
	case "sidebar.toggle":
		a.toggleSidebar()
	case "text.bigger":
		a.zoom(1, false)
	case "text.smaller":
		a.zoom(-1, false)
	case "text.reset":
		a.zoom(0, true)
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// item is a menu item running an action, with the user's shortcut.
func (a *App) item(id string) *mygo.MenuItem {
	act, _ := actions.Find(id)
	return &mygo.MenuItem{
		Label:       act.Title,
		Accelerator: a.accelerator(act),
		Click: func(*mygo.MenuItem, *mygo.Window) {
			// Closing is the window's in front: Settings closes itself.
			if (id == "pane.close" || id == "session.close") && a.setWin != nil && mygo.FocusedWindow() == a.setWin {
				a.setWin.Close()
				return
			}
			a.update(func() { a.run(id, nil) })
		},
	}
}

// accelerator is an action's shortcut in the menus: none while the user
// types a new one in Settings.
func (a *App) accelerator(act actions.Action) string {
	if a.ss.recording != "" {
		return ""
	}
	return actions.Keys(act, a.set.Shortcuts)
}

// items returns the items of a menu's actions, in their order, with
// separators where the groups change.
func (a *App) items(menu string, groups ...[]string) []*mygo.MenuItem {
	var out []*mygo.MenuItem
	for i, g := range groups {
		if i > 0 {
			out = append(out, mygo.Separator())
		}
		for _, id := range g {
			out = append(out, a.item(id))
		}
	}
	return out
}

func (a *App) menu() *mygo.Menu {
	appMenu := []*mygo.MenuItem{{Role: mygo.RoleAbout}, mygo.Separator(), a.item("settings.open"), a.item("cli.install"),
		mygo.Separator(), {Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide}, mygo.Separator(),
		{Label: "Quit " + mygo.App.Name(), Accelerator: quitKeys, ToolTip: "Sessions keep running", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		a.item("app.quitAll")}
	// The app menu is macOS's: MyGo leaves it out elsewhere, with its items'
	// shortcuts, so they go in a menu of Kero's there, without macOS's own.
	kero := &mygo.MenuItem{Role: mygo.RoleAppMenu, Submenu: appMenu}
	if runtime.GOOS != "darwin" {
		kero = &mygo.MenuItem{Label: "Kero", Submenu: []*mygo.MenuItem{a.item("settings.open"), a.item("cli.install"), mygo.Separator(),
			{Label: "Quit", Accelerator: quitKeys, ToolTip: "Sessions keep running", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
			a.item("app.quitAll")}}
	}
	view := append(a.items("View", []string{"sidebar.toggle"}, []string{"text.bigger", "text.smaller", "text.reset"}),
		mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleToggleFullScreen}, &mygo.MenuItem{Role: mygo.RoleToggleDevTools})
	return mygo.NewMenu([]*mygo.MenuItem{
		kero,
		{Label: "Session", Submenu: a.items("Session",
			[]string{"session.new", "session.newHome", "project.open"},
			[]string{"palette.open", "session.attention", "session.next", "session.previous"},
			[]string{"session.rename", "pane.restart"},
			[]string{"pane.close", "session.close"})},
		{Role: mygo.RoleEditMenu},
		{Label: "Pane", Submenu: a.items("Pane",
			[]string{"pane.split.right", "pane.split.down"},
			[]string{"pane.zoom"},
			[]string{"pane.next", "pane.previous", "pane.left", "pane.right", "pane.up", "pane.down"})},
		{Label: "View", Submenu: view},
		{Role: mygo.RoleWindowMenu},
	})
}

// keys is an action's shortcut as the menus show it, "" when it has none.
func (a *App) keys(id string) string {
	act, _ := actions.Find(id)
	return actions.Glyphs(actions.Keys(act, a.set.Shortcuts))
}

// tip is a button's tooltip: what it does, with its action's shortcut.
func (a *App) tip(title, id string) string {
	if k := a.keys(id); k != "" {
		return title + " (" + k + ")"
	}
	return title
}

// menuShortcut shows an action's shortcut by a context menu's item.
func (a *App) menuShortcut(it *ui.MenuItem, id string) *ui.MenuItem {
	act, _ := actions.Find(id)
	parts := strings.Split(actions.Keys(act, a.set.Shortcuts), "+")
	var mods ui.Modifiers
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(p) {
		case "cmdorctrl", "commandorcontrol":
			mods |= ui.Cmd
		case "cmd", "command", "super", "meta", "win":
			mods |= ui.Super
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "alt", "option":
			mods |= ui.Alt
		case "shift":
			mods |= ui.Shift
		}
	}
	for k, name := range keyNames {
		if strings.EqualFold(name, parts[len(parts)-1]) {
			return it.Shortcut(mods, k)
		}
	}
	return it
}

// quitKeys quits the app: Control+Q is the terminal's on Windows and Linux.
var quitKeys = func() string {
	if runtime.GOOS == "darwin" {
		return "CmdOrCtrl+Q"
	}
	return "Ctrl+Shift+Q"
}()
