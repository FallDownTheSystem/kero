package git

import (
	"os/exec"
	"testing"
)

// holdBusy keeps the folder at path from being deleted until release, as
// an agent's program does that still works in it: a process whose current
// folder it is.
func holdBusy(t *testing.T, path string) (release func()) {
	t.Helper()
	cmd := exec.Command("ping", "-n", "60", "127.0.0.1")
	cmd.Dir = path
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	release = func() {
		cmd.Process.Kill()
		cmd.Wait()
	}
	t.Cleanup(release)
	return release
}
