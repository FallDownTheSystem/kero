// Package shell finds the user's login shell, as terminal apps take it.
package shell

import (
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"
)

var (
	once  sync.Once
	login string
)

// Login returns the user's login shell: the account's (dscl on macOS,
// /etc/passwd elsewhere), whatever $SHELL this process inherited from
// whoever started it, else $SHELL, else zsh.
func Login() string {
	once.Do(func() {
		login = account()
		if login == "" {
			login = os.Getenv("SHELL")
		}
		if login == "" {
			login = "/bin/zsh"
		}
	})
	return login
}

func account() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	valid := func(sh string) string {
		if st, err := os.Stat(sh); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return sh
		}
		return ""
	}
	if out, err := exec.Command("/usr/bin/dscl", ".", "-read", "/Users/"+u.Username, "UserShell").Output(); err == nil {
		if _, sh, ok := strings.Cut(strings.TrimSpace(string(out)), ": "); ok {
			return valid(strings.TrimSpace(sh))
		}
	}
	if b, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Split(line, ":")
			if len(f) >= 7 && f[0] == u.Username {
				return valid(f[6])
			}
		}
	}
	return ""
}
