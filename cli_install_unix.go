//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"kero/internal/proto"
)

// cliLinks are where installCLI puts the kero command, and where Settings
// looks for it.
func cliLinks() []string {
	return []string{"/usr/local/bin/kero", "/opt/homebrew/bin/kero", filepath.Join(proto.Home(), ".local/bin/kero")}
}

// installCLI links the kero command to this executable: /usr/local/bin's
// on macOS, asking for an administrator's password when the directory
// needs it; ~/.local/bin's on Linux, which needs none.
func (a *App) installCLI() {
	exe, err := os.Executable()
	if err != nil {
		a.toast(err.Error())
		return
	}
	go func() {
		link := "/usr/local/bin/kero"
		if runtime.GOOS != "darwin" {
			link = filepath.Join(proto.Home(), ".local/bin/kero")
			os.MkdirAll(filepath.Dir(link), 0o755)
		}
		os.Remove(link)
		err := os.Symlink(exe, link)
		if err != nil && runtime.GOOS == "darwin" {
			script := fmt.Sprintf(`do shell script "mkdir -p /usr/local/bin && ln -sf %s %s" with administrator privileges`,
				strings.ReplaceAll(fmt.Sprintf("%q", exe), `\`, `\`), link)
			err = exec.Command("/usr/bin/osascript", "-e", script).Run()
		}
		a.update(func() {
			if err != nil {
				a.toast("Couldn’t install the command line tool")
				return
			}
			a.toast("Installed the kero command in " + proto.ShortDir(filepath.Dir(link)))
		})
	}()
}

// cliInstalled returns where the kero command is installed, and what it
// runs, "" when it is nowhere.
func cliInstalled() string {
	for _, p := range cliLinks() {
		if t, err := os.Readlink(p); err == nil {
			return p + " → " + proto.ShortDir(t)
		}
	}
	return ""
}
