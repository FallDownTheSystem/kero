package remote

import (
	"errors"
	"os/exec"
	"strings"

	"kero/internal/update"
)

// InstallCommand is the command that installs the server build of Kero,
// of version ("" the latest), on a host reached over SSH or WSL: the
// release's install script, which puts it in ~/.local and runs kerod as a
// service of the user's. Batch: it asks nothing, as from the app.
func InstallCommand(a Addr, version string, batch bool) (*exec.Cmd, error) {
	url := update.InstallScript(version)
	args := ""
	if version != "" {
		args = " -s -- --version " + shellQuote(strings.TrimPrefix(version, "v"))
	}
	script := `set -e; url=` + shellQuote(url) + `; ` +
		`if command -v curl >/dev/null 2>&1; then curl -fsSL "$url"; ` +
		`elif command -v wget >/dev/null 2>&1; then wget -qO- "$url"; ` +
		`else echo "installing Kero needs curl or wget" >&2; exit 1; fi | sh` + args
	var cmd *exec.Cmd
	switch a.Scheme {
	case "ssh":
		sa := []string{"-T", "-o", "ConnectTimeout=15"}
		if batch {
			sa = append(sa, "-o", "BatchMode=yes")
		}
		if a.Port != "" {
			sa = append(sa, "-p", a.Port)
		}
		if a.User != "" {
			sa = append(sa, "-l", a.User)
		}
		cmd = exec.Command(SSHCommand(), append(sa, "--", a.Host, "sh -c "+shellQuote(script))...)
	case "wsl":
		wa := []string{}
		if a.Host != "" {
			wa = append(wa, "-d", a.Host)
		}
		cmd = exec.Command("wsl.exe", append(wa, "--exec", "sh", "-c", script)...)
	default:
		return nil, errors.New("Kero installs itself on a host over SSH or WSL: install it there with the script of its releases")
	}
	hideWindow(cmd)
	return cmd, nil
}
