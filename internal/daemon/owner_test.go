package daemon

import (
	"io"
	"net"
	"testing"
)

// attached attaches a client to a pane whose frames go nowhere.
func attached(t *testing.T, s *pane) *attachment {
	t.Helper()
	here, there := net.Pipe()
	go io.Copy(io.Discard, there)
	t.Cleanup(func() { here.Close(); there.Close() })
	a := newAttachment(here)
	if _, _, _, ok := s.attach(a); !ok {
		t.Fatal("attach failed")
	}
	go a.write()
	return a
}

// A pane takes the size of the client the user is in: another client's,
// resized by a layout synced from it, waits until the user is in that one.
func TestPaneSizeOwner(t *testing.T) {
	s := testPane(t)
	mac, win := attached(t, s), attached(t, s)
	size := func(when string, cols, rows int) {
		t.Helper()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.info.Cols != cols || s.info.Rows != rows {
			t.Errorf("%s: %dx%d, want %dx%d", when, s.info.Cols, s.info.Rows, cols, rows)
		}
	}
	s.ask(mac, 90, 30, 9, 18)
	size("no owner yet", 90, 30)
	s.own(mac) // the user types on the Mac
	s.ask(win, 100, 40, 9, 18)
	size("another client's size", 90, 30)
	s.ask(mac, 80, 30, 9, 18)
	size("the owner's", 80, 30)
	s.own(win) // the user moves to Windows
	size("the new owner's last asked", 100, 40)
	s.detach(win)
	s.ask(mac, 70, 20, 9, 18)
	size("owner gone", 70, 20)
}
