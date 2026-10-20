package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// testPane is a pane with no program, whose output the test writes.
func testPane(t *testing.T) *pane {
	t.Helper()
	t.Setenv("KERO_CONFIG", filepath.Join(t.TempDir(), "config"))
	d := &Daemon{changeCh: make(chan struct{}, 1), saveCh: make(chan struct{}, 1)}
	d.viewed.Store(new(string))
	s, err := newPane(d, "s", proto.Pane{ID: "p", Status: proto.Idle, Cols: 96, Rows: 28})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.term.Close() })
	return s
}

// show writes a program's redraw of the whole screen.
func (s *pane) show(text string) {
	s.output([]byte("\x1b[H\x1b[2J" + strings.ReplaceAll(strings.TrimRight(text, "\n"), "\n", "\r\n")))
}

func claudeScreen(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "detect", "testdata", "claude-"+name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (s *pane) status(now time.Time) proto.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.look(now)
	return s.info.Status
}

// Claude Code at its prompt repaints its screen as the sidebar hides and
// the pane resizes, over and over: still idle. Its turn is work, and the
// prompt back is idle again.
func TestClaudeRedrawIsNoWork(t *testing.T) {
	s := testPane(t)
	s.info.Agent, s.info.Title = "claude", "✳ Claude Code"
	now := time.Now()
	idle := claudeScreen(t, "idle")
	s.show(idle)
	if st := s.status(now); st != proto.Idle {
		t.Fatalf("at the prompt: %s", st)
	}
	for i := range 20 {
		s.resize(96-i, 28)
		s.show(idle)
		now = now.Add(16 * time.Millisecond)
		if st := s.status(now); st != proto.Idle {
			t.Fatalf("redraw %d after a resize: %s", i, st)
		}
	}
	s.show(claudeScreen(t, "working"))
	if st := s.status(now); st != proto.Working {
		t.Fatalf("in a turn: %s", st)
	}
	// Redraws of the turn, as a resize makes, are the same turn.
	s.resize(80, 28)
	s.show(claudeScreen(t, "working"))
	if st := s.status(now); st != proto.Working {
		t.Fatalf("in a turn, resized: %s", st)
	}
	s.show(idle)
	if st := s.status(now); st != proto.Idle {
		t.Fatalf("after the turn: %s", st)
	}
	if !s.info.Unread {
		t.Error("a turn that ended unseen is not unread")
	}
	s.show(claudeScreen(t, "blocked"))
	if st := s.status(now); st != proto.Waiting {
		t.Fatalf("asking to run a command: %s", st)
	}
	// Keys that move in the dialog leave it asking.
	s.input(nil)
	if st := s.status(now); st != proto.Waiting {
		t.Fatalf("asking, after a key: %s", st)
	}
	s.show(idle)
	if st := s.status(now); st != proto.Idle {
		t.Fatalf("back at the prompt: %s", st)
	}
}

// A program that is no agent works while its title turns as a spinner:
// a clock it draws, however long it ticks, is not work.
func TestProgramWorksByTitle(t *testing.T) {
	s := testPane(t)
	now := time.Now()
	for i := range 30 {
		s.output([]byte("\x1b[32;1H" + now.Format("05")))
		now = now.Add(time.Second)
		if st := s.status(now); st != proto.Idle {
			t.Fatalf("clock tick %d: %s", i, st)
		}
	}
	s.output([]byte("\x1b]0;⠋ building\x07"))
	if st := s.status(now); st != proto.Working {
		t.Fatalf("title turning: %s", st)
	}
	s.output([]byte("\x1b]0;done\x07"))
	if st := s.status(now); st != proto.Idle {
		t.Fatalf("title still: %s", st)
	}
}

// An agent's screen that stops showing work without showing its prompt is
// idle only once it has stayed so (idleHold).
func TestWorkingHeldBeforeIdle(t *testing.T) {
	s := testPane(t)
	s.info.Agent = "claude"
	now := time.Now()
	s.show(claudeScreen(t, "working"))
	if st := s.status(now); st != proto.Working {
		t.Fatalf("in a turn: %s", st)
	}
	s.show("⏺ Some answer")
	if st := s.status(now); st != proto.Working {
		t.Fatalf("between redraws: %s", st)
	}
	if st := s.status(now.Add(idleHold)); st != proto.Idle {
		t.Fatalf("after %s: %s", idleHold, st)
	}
}
