package daemon

import (
	"os/exec"
	"testing"
	"time"
)

// A command a session starts with runs in a process group of its own, in
// the terminal's foreground, so that tick sees it.
func TestCommandInForeground(t *testing.T) {
	for _, name := range []string{"fish", "zsh", "bash"} {
		t.Run(name, func(t *testing.T) {
			shell, err := exec.LookPath(name)
			if err != nil {
				t.Skip(name, "not installed")
			}
			proc, err := startPTY(ptyOptions{
				Path: shell, Args: shellArgs(shell, []string{"sleep", "5"}),
				Dir: t.TempDir(), Env: []string{"HOME=" + t.TempDir(), "TERM=xterm-256color"},
				Cols: 80, Rows: 24,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer proc.Kill()
			go func() {
				buf := make([]byte, 4096)
				for {
					if _, err := proc.Read(buf); err != nil {
						return
					}
				}
			}()
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
				if pg := proc.Foreground(); pg != 0 && pg != proc.PID() && procName(pg) == "sleep" {
					return
				}
			}
			t.Fatalf("foreground group %d is not sleep's (shell %d)", proc.Foreground(), proc.PID())
		})
	}
}
