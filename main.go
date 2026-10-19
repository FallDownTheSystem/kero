// kero is a terminal for coding agents: sessions grouped by project in a
// sidebar that shows which work, which need you and which are done. The
// sessions run in kerod, a daemon of their own, so quitting kero leaves
// them running, and opening it again shows everything as it was.
//
// The same executable is the kero CLI (kero help), which drives sessions
// from a shell or an agent, and kerod itself (kero __kerod).
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/term"

	"kero/internal/cli"
	"kero/internal/daemon"
	"kero/internal/proto"
)

func main() {
	if len(os.Args) > 1 {
		switch {
		case os.Args[1] == "__kerod":
			if err := daemon.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "kerod:", err)
				os.Exit(1)
			}
			return
		case slices.Contains(cli.Commands, os.Args[1]):
			os.Exit(cli.Run(os.Args[1:]))
		}
	}
	// `kero` alone in a terminal, through the CLI's link to the app's
	// executable: open the app as the Finder would.
	if len(os.Args) == 1 && term.IsTerminal(int(os.Stdin.Fd())) {
		if exe, err := os.Executable(); err == nil {
			if real, err := filepath.EvalSymlinks(exe); err == nil {
				if i := strings.Index(real, ".app/Contents/MacOS/"); i >= 0 {
					if exec.Command("/usr/bin/open", real[:i+4]).Run() == nil {
						return
					}
				}
			}
		}
	}
	runApp()
}

func runApp() {
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := newApp()
	mygo.App.SetMenu(a.menu())
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.win != nil {
			a.win.Restore()
			a.win.Focus()
		}
	})
	mygo.App.OnDidBecomeActive(func() {
		mygo.ClearNotifications()
		a.update(func() { a.active = true; a.reportView() })
	})
	mygo.App.OnDidResignActive(func() {
		a.update(func() { a.active = false; a.reportView() })
	})
	mygo.App.OnActivate(func(hasVisible bool) {
		if !hasVisible && a.win != nil {
			a.win.Show()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.savePrefs()
	})
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:          "kero",
			Width:          1180,
			Height:         760,
			MinWidth:       560,
			MinHeight:      360,
			StateKey:       "main",
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: topBar,
			// The lights' buttons are 16 tall, and MyGo puts them a DIP
			// above the position: centered on barMid, as the sidebar's
			// buttons and the panes' headers are.
			TrafficLightPosition: &mygo.Point{X: 18, Y: barMid - 7},
			Vibrancy:             mygo.VibrancySidebar,
			Content:              ui.View(a.view),
		})
		a.win.OnFocus(func() { a.update(func() { a.focused = true; a.reportView() }) })
		a.win.OnBlur(func() { a.update(func() { a.focused = false; a.reportView() }) })
		a.connect()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// do runs an action of the app from a menu, on the main thread.
func (a *App) do(fn func()) func(*mygo.MenuItem, *mygo.Window) {
	return func(*mygo.MenuItem, *mygo.Window) { a.update(fn) }
}

func (a *App) menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			{Label: "Install Command Line Tool…", Click: a.do(a.installCLI)},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Label: "Quit kero", Accelerator: "CmdOrCtrl+Q", ToolTip: "Sessions keep running", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
			{Label: "Quit and End All Sessions…", Accelerator: "CmdOrCtrl+Alt+Q", Click: a.do(a.quitAndEnd)},
		}},
		{Label: "Session", Submenu: []*mygo.MenuItem{
			{Label: "New Session", Accelerator: "CmdOrCtrl+T", Click: a.do(func() { a.newSession("") })},
			{Label: "New Session in Home", Accelerator: "CmdOrCtrl+Shift+T", Click: a.do(func() { a.newSessionIn(proto.Home()) })},
			{Label: "Open Project…", Accelerator: "CmdOrCtrl+O", Click: a.do(a.openProject)},
			mygo.Separator(),
			{Label: "Go to Session…", Accelerator: "CmdOrCtrl+P", Click: a.do(a.openPalette)},
			{Label: "Jump to Session Needing You", Accelerator: "CmdOrCtrl+J", Click: a.do(a.jumpToAttention)},
			{Label: "Next Session", Accelerator: "CmdOrCtrl+Shift+]", Click: a.do(func() { a.selectOffset(1) })},
			{Label: "Previous Session", Accelerator: "CmdOrCtrl+Shift+[", Click: a.do(func() { a.selectOffset(-1) })},
			mygo.Separator(),
			{Label: "Rename Session", Accelerator: "CmdOrCtrl+R", Click: a.do(func() { a.startRename(a.selected) })},
			{Label: "Restart Pane", Accelerator: "CmdOrCtrl+Shift+R", Click: a.do(func() {
				if s := a.session(a.selected); s != nil {
					a.restartSession(a.focusOf(s))
				}
			})},
			mygo.Separator(),
			{Label: "Close", Accelerator: "CmdOrCtrl+W", Click: a.do(a.closeFocused)},
			{Label: "Close Session", Accelerator: "CmdOrCtrl+Shift+W", Click: a.do(func() { a.closeSession(a.selected, true) })},
		}},
		{Label: "Pane", Submenu: []*mygo.MenuItem{
			{Label: "Split Right", Accelerator: "CmdOrCtrl+D", Click: a.do(func() { a.splitPane(a.session(a.selected), "", "right") })},
			{Label: "Split Down", Accelerator: "CmdOrCtrl+Shift+D", Click: a.do(func() { a.splitPane(a.session(a.selected), "", "down") })},
			mygo.Separator(),
			{Label: "Zoom Pane", Accelerator: "CmdOrCtrl+Shift+Enter", Click: a.do(func() { a.zoomPane(a.session(a.selected), "") })},
			mygo.Separator(),
			{Label: "Next Pane", Accelerator: "CmdOrCtrl+]", Click: a.do(func() { a.cyclePane(1) })},
			{Label: "Previous Pane", Accelerator: "CmdOrCtrl+[", Click: a.do(func() { a.cyclePane(-1) })},
			{Label: "Pane Left", Accelerator: "CmdOrCtrl+Alt+Left", Click: a.do(func() { a.focusDirection("left") })},
			{Label: "Pane Right", Accelerator: "CmdOrCtrl+Alt+Right", Click: a.do(func() { a.focusDirection("right") })},
			{Label: "Pane Above", Accelerator: "CmdOrCtrl+Alt+Up", Click: a.do(func() { a.focusDirection("up") })},
			{Label: "Pane Below", Accelerator: "CmdOrCtrl+Alt+Down", Click: a.do(func() { a.focusDirection("down") })},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Label: "Toggle Sidebar", Accelerator: "CmdOrCtrl+B", Click: a.do(a.toggleSidebar)},
			mygo.Separator(),
			{Label: "Bigger", Accelerator: "CmdOrCtrl+=", Click: a.do(func() { a.zoom(1, false) })},
			{Label: "Smaller", Accelerator: "CmdOrCtrl+-", Click: a.do(func() { a.zoom(-1, false) })},
			{Label: "Actual Size", Accelerator: "CmdOrCtrl+0", Click: a.do(func() { a.zoom(0, true) })},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
			{Role: mygo.RoleToggleDevTools},
		}},
		{Role: mygo.RoleWindowMenu},
	})
}

// newSessionIn starts a session in a directory, in its project.
func (a *App) newSessionIn(dir string) {
	if a.cl == nil {
		return
	}
	cl := a.cl
	go func() {
		var s proto.Session
		err := cl.Call("create", proto.CreateParams{Dir: dir}, &s)
		a.update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			a.addPending(s)
			a.selectSession(s.ID)
		})
	}()
}

// installCLI links /usr/local/bin/kero to this executable, asking for an
// administrator's password when the directory needs it.
func (a *App) installCLI() {
	exe, err := os.Executable()
	if err != nil {
		a.toast(err.Error())
		return
	}
	go func() {
		const link = "/usr/local/bin/kero"
		os.Remove(link)
		err := os.Symlink(exe, link)
		if err != nil {
			script := fmt.Sprintf(`do shell script "mkdir -p /usr/local/bin && ln -sf %s %s" with administrator privileges`,
				strings.ReplaceAll(fmt.Sprintf("%q", exe), `\`, `\\`), link)
			err = exec.Command("/usr/bin/osascript", "-e", script).Run()
		}
		a.update(func() {
			if err != nil {
				a.toast("Couldn’t install the command line tool")
				return
			}
			a.toast("Installed kero in /usr/local/bin")
		})
	}()
}

func stopDaemon() error { return cli.StopDaemon() }
