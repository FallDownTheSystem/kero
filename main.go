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
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/term"

	"kero/internal/cli"
	"kero/internal/clilink"
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
				// On Windows the command is a copy of the app's executable
				// (package clilink), which knows what it is a copy of.
				if bin := filepath.Dir(real); runtime.GOOS == "windows" && strings.EqualFold(bin, filepath.Join(proto.Dir(), "bin")) {
					if app := clilink.Target(bin); app != "" && exec.Command(app).Start() == nil {
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
		a.win = mygo.NewWindow(framed(mygo.WindowOptions{
			Title:     "Kero",
			Width:     1180,
			Height:    760,
			MinWidth:  560,
			MinHeight: 360,
			StateKey:  "main",
			Vibrancy:  vibrancyFor(a.set),
			Content:   ui.View(a.view),
		}))
		a.win.OnFocus(func() { a.update(func() { a.focused = true; a.reportView() }) })
		a.win.OnBlur(func() { a.update(func() { a.focused = false; a.reportView() }) })
		a.connect()
		go a.watchConfig()
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

func stopDaemon() error { return cli.StopDaemon() }
