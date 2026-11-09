package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A message is one line for Claude Code's monitor: whole when short; when
// long, its start and the file holding it whole, within what Claude Code
// gives Claude of a line.
func TestInboxEvent(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if got := inboxEvent("Kero: message:\n\nline two\r\nC:\\notes", dir, now); got != `Kero: message:\n\nline two\nC:\notes` {
		t.Errorf("short: %q", got)
	}
	long := "start of it\n" + strings.Repeat("filler words ", 200) + "\nCODE-1"
	got := inboxEvent(long, dir, now)
	if n := len([]rune(got)); n > eventMax || strings.Contains(got, "\n") || !strings.HasPrefix(got, `start of it\n`) {
		t.Errorf("long, %d runes: %q", n, got)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("%d files", len(files))
	}
	path := filepath.Join(dir, files[0].Name())
	if !strings.Contains(got, path) {
		t.Errorf("names no file %s: %q", path, got)
	}
	if b, _ := os.ReadFile(path); string(b) != long {
		t.Error("the file holds not the whole message")
	}

	// Another, a week later: the old file goes.
	old := now.Add(-inboxKeep - time.Hour)
	os.Chtimes(path, old, old)
	inboxEvent(long, dir, now.Add(time.Second))
	if _, err := os.Stat(path); err == nil {
		t.Error("an old message's file stays")
	}
}

// Every command the CLI runs is one main sends to it, not to the app.
func TestCommandsListed(t *testing.T) {
	for _, cmd := range []string{"inbox", "hook", "skill", "report"} {
		if !IsCommand(cmd) {
			t.Errorf("%s goes to the app", cmd)
		}
	}
}
