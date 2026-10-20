//go:build unix

package daemon

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The program gets the size of the client's cells in pixels, from the
// terminal's answer to CSI 16t and from the pseudo-terminal's size: what
// a program taking the mouse in pixels (mode 1016), as Claude Code does,
// counts the client's reports in.
func TestCellPixels(t *testing.T) {
	s := testPane(t)
	out := filepath.Join(t.TempDir(), "reply")
	proc, err := startPTY(ptyOptions{
		Path: "/bin/sh", Args: []string{"sh", "-c", `stty raw -echo; head -c 10 > "$0"`, out},
		Dir: t.TempDir(), Env: testEnv(t),
		Cols: 96, Rows: 28,
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
	s.mu.Lock()
	s.proc = proc
	s.mu.Unlock()

	s.resize(96, 28, 18, 36)
	var ws struct{ row, col, xpixel, ypixel uint16 }
	if err := ioctl(proc.master, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		t.Fatal(err)
	}
	if ws.xpixel != 96*18 || ws.ypixel != 28*36 {
		t.Errorf("the pseudo-terminal is %dx%d pixels, want %dx%d", ws.xpixel, ws.ypixel, 96*18, 28*36)
	}

	// The reply goes to the program once it reads raw.
	deadline := time.Now().Add(10 * time.Second) // a hang guard
	for {
		if _, err := os.Stat(out); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the program did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.output([]byte("\x1b[16t"))
	want := "\x1b[6;36;18t"
	for {
		b, _ := os.ReadFile(out)
		if string(b) == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("CSI 16t answered %q, want %q", b, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
