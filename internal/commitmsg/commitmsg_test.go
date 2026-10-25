package commitmsg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"Fix login\n":                                     "Fix login",
		"```\nFix login\n\nBecause.  \n```":               "Fix login\n\nBecause.",
		"```text\nFix login\n```\n":                       "Fix login",
		`"Fix login"`:                                     "Fix login",
		"\"Fix\"\n\nbody \"quoted\"":                      "\"Fix\"\n\nbody \"quoted\"",
		"  Subject\r\n\r\nBody line one   \r\nline two  ": "Subject\n\nBody line one\nline two",
		// A launcher's banner before the message, which goes.
		"T  clodex\n|\n•  Proxy started\n<commit-message>\nFix login\n\nBecause.\n</commit-message>\n": "Fix login\n\nBecause.",
		"```\n<commit-message>\nFix login\n</commit-message>\n```":                                     "Fix login",
		"banner\n<commit-message>\nFix login\n":                                                        "Fix login",
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
	claude := &Agent{Name: "claude", ID: "abc"}
	// The user's command runs in their shell, as typed at its prompt, where
	// their profile's functions are.
	for _, c := range []struct {
		sh   string
		want []string
	}{
		{`C:\Program Files\PowerShell\7\pwsh.exe`, []string{`C:\Program Files\PowerShell\7\pwsh.exe`, "-NoLogo", "-NonInteractive", "-Command", "my-writer --short"}},
		{"/bin/zsh", []string{"/bin/zsh", "-l", "-i", "-c", "my-writer --short"}},
		{"/usr/bin/fish", []string{"/usr/bin/fish", "-l", "-c", "my-writer --short"}},
		{`C:\Windows\System32\cmd.exe`, []string{`C:\Windows\System32\cmd.exe`, "/C", "my-writer --short"}},
		{"nu", []string{"my-writer", "--short"}},
	} {
		argv, forked, err := Command("my-writer --short", claude, c.sh)
		if err != nil || forked || !slices.Equal(argv, c.want) {
			t.Errorf("in %s: %q %v %v", c.sh, argv, forked, err)
		}
	}
	// {session} forks the pane's agent's conversation by the user's command.
	argv, forked, _ := Command("cc -p --resume {session} --fork-session", claude, "pwsh")
	if !forked || argv[len(argv)-1] != "cc -p --resume abc --fork-session" {
		t.Errorf("with the session: %q %v", argv, forked)
	}
	// With no conversation to fork, it can't run: an agent's new run does.
	if argv, forked, _ := Command("cc -p --resume {session}", nil, "pwsh"); forked || slices.Contains(argv, "cc") || strings.Contains(strings.Join(argv, " "), "{session}") {
		t.Errorf("with no session: %q %v", argv, forked)
	}
	// Codex's runs as it is, for the file its message goes to.
	if argv, _, _ := Command("codex exec fork {session} -", &Agent{Name: "codex", ID: "t1"}, "pwsh"); !slices.Equal(argv, []string{"codex", "exec", "fork", "t1", "-"}) {
		t.Errorf("codex: %q", argv)
	}
	// An id that would be an option is no conversation to fork.
	if argv, forked, _ := Command("", &Agent{Name: "claude", ID: "--help"}, "pwsh"); forked || slices.Contains(argv, "--help") {
		t.Errorf("forked %q", argv)
	}
	if argv, forked, _ := Command("x --resume {session}", &Agent{Name: "claude", ID: "--help"}, "pwsh"); forked || strings.Contains(strings.Join(argv, " "), "--help") {
		t.Errorf("forked by the user's command %q", argv)
	}
}

// The user's command finds what their shell's profile defines, and gets
// the prompt on its stdin.
func TestCommandInShell(t *testing.T) {
	sh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("no pwsh")
	}
	argv := inShell(sh, "function writer { [Console]::In.ReadLine() }; writer")
	msg, err := Write(context.Background(), t.TempDir(), argv, "Fix the login\nmore\n")
	if err != nil || msg != "Fix the login" {
		t.Errorf("wrote %q, %v", msg, err)
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
