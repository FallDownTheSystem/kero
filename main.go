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
	loadFonts()
	mygo.Theme.SetSource(mygo.ThemeSource(a.set.Appearance))
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
			Vibrancy:             vibrancyFor(a.set),
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
