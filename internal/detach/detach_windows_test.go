package detach

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// WMI starts a program with the environment and directory it is given.
func TestWMIStart(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	cmd := exec.Command(os.Getenv("ComSpec"), "/c", "echo %KERO_PROBE% %CD%> "+out)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "KERO_PROBE=it's here")
	if _, err := wmiStart(cmd); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if b, err := os.ReadFile(out); err == nil && len(b) > 0 {
			if got := strings.TrimSpace(string(b)); got != "it's here "+dir {
				t.Errorf("got %q", got)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("it did not run")
}
