package commitmsg

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"Fix login\n":                                   "Fix login",
		"```\nFix login\n\nBecause.  \n```":             "Fix login\n\nBecause.",
		"```text\nFix login\n```\n":                      "Fix login",
		`"Fix login"`:                                   "Fix login",
		"\"Fix\"\n\nbody \"quoted\"":                     "\"Fix\"\n\nbody \"quoted\"",
		"  Subject\r\n\r\nBody line one   \r\nline two  ": "Subject\n\nBody line one\nline two",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestWords(t *testing.T) {
	got := Words(`llm -m "gpt x" --system  "C:\Program Files\p.txt"`)
	want := []string{"llm", "-m", "gpt x", "--system", `C:\Program Files\p.txt`}
	if !slices.Equal(got, want) {
		t.Errorf("Words = %q", got)
	}
}

func TestCommand(t *testing.T) {
	argv, forked, err := Command("my-writer --short", &Agent{Name: "claude", ID: "abc"})
	if err != nil || forked || !slices.Equal(argv, []string{"my-writer", "--short"}) {
		t.Errorf("the user's command: %q %v %v", argv, forked, err)
	}
	// An id that would be an option is no conversation to fork.
	if argv, forked, _ := Command("", &Agent{Name: "claude", ID: "--help"}); forked || slices.Contains(argv, "--help") {
		t.Errorf("forked %q", argv)
	}
}

func TestPrompt(t *testing.T) {
	p := Prompt(true, "Add x\nFix y", " a.go | 2 +-", "diff --git a/a.go b/a.go\n")
	for _, want := range []string{"You made these changes", "Fix y", "a.go | 2 +-", "diff --git"} {
		if !strings.Contains(p, want) {
			t.Errorf("the prompt has no %q", want)
		}
	}
	if strings.Contains(Prompt(false, "", "s", "p"), "You made") {
		t.Error("a new run's prompt says it made the changes")
	}
}

func TestWrite(t *testing.T) {
	// A writer that echoes its stdin's first line back, fenced, as a
	// model might.
	dir := t.TempDir()
	script := filepath.Join(dir, "writer.sh")
	argv := []string{"sh", script}
	body := "#!/bin/sh\nread first\nprintf '```\\n%s\\n```\\n' \"$first\"\n"
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(`C:\Program Files\Git\bin\sh.exe`); err != nil {
			t.Skip("no sh")
		}
		argv[0] = `C:\Program Files\Git\bin\sh.exe`
	}
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err := Write(context.Background(), dir, argv, "Fix the login\nmore\n")
	if err != nil || msg != "Fix the login" {
		t.Errorf("Write = %q, %v", msg, err)
	}
	if _, err := Write(context.Background(), dir, []string{argv[0], "-c", "echo oops >&2; exit 3"}, "x"); err == nil || !strings.Contains(err.Error(), "oops") {
		t.Errorf("a failing writer: %v", err)
	}
}
