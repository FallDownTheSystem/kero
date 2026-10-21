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
// service of the user's. It downloads the script whole before it runs
// it, so that a failed download fails the command. Batch: it asks
// nothing, as from the app.
func InstallCommand(a Addr, version string, batch bool) (*exec.Cmd, error) {
	args := ""
	if version != "" {
		version = strings.TrimPrefix(version, "v")
		if !update.ValidVersion(version) {
			return nil, errors.New(version + " is not a version of Kero")
		}
		args = " --version " + version
	}
	script := `set -e; url=` + shellQuote(update.InstallScript(version)) + `; f="$(mktemp)"; trap 'rm -f "$f"' EXIT; ` +
		`if command -v curl >/dev/null 2>&1; then curl -fsSL "$url" -o "$f"; ` +
		`elif command -v wget >/dev/null 2>&1; then wget -qO "$f" "$url"; ` +
		`else echo "installing Kero needs curl or wget" >&2; exit 1; fi; sh "$f"` + args
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
