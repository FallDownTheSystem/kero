package daemon

import (
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// ClearScreen clears the console's screen too, which then draws its
// prompt's line on the first row: with it still on a lower row, PowerShell
// redraws (an inline prediction) would send the cursor back there.
func TestConsoleClearScreen(t *testing.T) {
	if windows.RtlGetVersion().BuildNumber < 26100 {
		t.Skip("the console takes no clear signal before Windows 11 24H2")
	}
	shell, err := exec.LookPath("pwsh")
	if err != nil {
		if shell, err = exec.LookPath("powershell"); err != nil {
			t.Skip("PowerShell not installed")
		}
	}
	proc, err := startPTY(ptyOptions{
		Path: shell, Args: []string{"pwsh", "-NoLogo", "-NoProfile"},
		Dir: t.TempDir(), Env: testEnv(t),
		Cols: 80, Rows: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Kill()
	var (
		mu  sync.Mutex
		out strings.Builder
	)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := proc.Read(buf)
			mu.Lock()
			out.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	// waitFor waits for the output from `from` on to match re.
	waitFor := func(from int, re *regexp.Regexp, what string) int {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			mu.Lock()
			s := out.String()
			mu.Unlock()
			if re.MatchString(s[from:]) {
				return len(s)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("no %s in %q", what, out.String()[from:])
		return 0
	}
	at := waitFor(0, regexp.MustCompile(`PS [^\r\n]*>`), "prompt")
	proc.Write([]byte("1..20\r"))
	at = waitFor(at, regexp.MustCompile(`\n20\r(.|\n)*PS [^\r\n]*>`), "prompt after the list")

	proc.ClearScreen()
	waitFor(at, regexp.MustCompile(`\x1b\[1;\d+H`), "cursor on the first row")
}
