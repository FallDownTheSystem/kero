// Package commitmsg writes a commit message of what is staged, with a
// coding agent's help: by default a fork of the conversation of the agent
// that made the changes, which knows why it made them, without a word of
// it reaching that conversation; else a new run of an agent's, or the
// command the user set (settings' commit-message-command), each given the
// prompt on its stdin and printing the message.
package commitmsg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Agent is a coding agent's conversation to fork, as kerod knows a pane's.
type Agent struct {
	Name string // "claude" or "codex"
	ID   string
}

// ErrNoWriter: there is no agent to ask, nor a command set.
var ErrNoWriter = errors.New("no coding agent to write the message: set a command in Settings › Git")

// Prompt is what the message is asked for with: the repository's recent
// subjects, as the style to keep, then what is staged.
func Prompt(forked bool, recent, stat, patch string) string {
	var b strings.Builder
	b.WriteString("Write a git commit message for the staged changes below.\n\n")
	if forked {
		b.WriteString("You made these changes, or some of them, in this conversation: say why they were made, as you know it. The diff is what the commit takes; describe it, not the rest of the conversation's work.\n\n")
	}
	b.WriteString("Keep to the style of the repository's recent commit messages. Reply with the message alone: a subject line of at most 72 characters, then, when the change needs one, a blank line and a body wrapped at 72 that says why. No code fences, no quotes, no commentary, and no tools.\n\n")
	if recent = strings.TrimSpace(recent); recent != "" {
		b.WriteString("Recent commit subjects:\n" + recent + "\n\n")
	}
	b.WriteString("Staged changes:\n" + stat + "\n\n" + patch)
	return b.String()
}

// SessionPlaceholder is what the user's command has for the id of the
// pane's agent's conversation, to fork it as the agent's own command does,
// as "cc -p --resume {session} --fork-session".
const SessionPlaceholder = "{session}"

// Command returns the command that writes the message: the user's when
// set, run in sh as typed at its prompt; else a fork of agent's
// conversation, else a new run of an agent that is installed. forked
// tells it is a fork: the user's too, when it has the conversation's id,
// which it then runs without when the pane has no agent's.
func Command(user string, agent *Agent, sh string) (argv []string, forked bool, err error) {
	if strings.Contains(user, SessionPlaceholder) {
		if agent == nil || !validID.MatchString(agent.ID) {
			user = ""
		} else {
			user, forked = strings.ReplaceAll(user, SessionPlaceholder, agent.ID), true
		}
	}
	if words := Words(user); len(words) > 0 {
		// Codex's is run as it is, for Write to give it the file its
		// message goes to.
		if isCodex(words[0]) {
			return words, forked, nil
		}
		return inShell(sh, user), forked, nil
	}
	settings, err := noHooks()
	if err != nil {
		return nil, false, err
	}
	if agent != nil && validID.MatchString(agent.ID) {
		switch agent.Name {
		case "claude":
			if _, err := exec.LookPath("claude"); err == nil {
				// Without tools, hooks or a session of its own on disk: the
				// fork reads the conversation, and leaves no trace of itself.
				return []string{"claude", "-p", "--resume", agent.ID, "--fork-session", "--no-session-persistence",
					"--tools", "", "--settings", settings, "--output-format", "text"}, true, nil
			}
		case "codex":
			if _, err := exec.LookPath("codex"); err == nil {
				return []string{"codex", "exec", "fork", agent.ID, "-", "--ephemeral", "--skip-git-repo-check",
					"-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`}, true, nil
			}
		}
	}
	if _, err := exec.LookPath("claude"); err == nil {
		return []string{"claude", "-p", "--no-session-persistence", "--tools", "", "--settings", settings, "--output-format", "text"}, false, nil
	}
	if _, err := exec.LookPath("codex"); err == nil {
		return []string{"codex", "exec", "-", "--ephemeral", "--skip-git-repo-check", "-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`}, false, nil
	}
	return nil, false, ErrNoWriter
}

func isCodex(program string) bool {
	base := filepath.Base(program)
	return base == "codex" || strings.HasPrefix(base, "codex.")
}

// inShell is the argv that runs line in the shell sh as the user types it
// at its prompt, its profile read, so that what the profile defines runs
// too, as a function of PowerShell's profile does. A shell it doesn't
// know runs line's words as a program.
func inShell(sh, line string) []string {
	// Either separator, as filepath.Base splits a Windows path only on
	// Windows.
	name := sh[strings.LastIndexAny(sh, `/\`)+1:]
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	switch strings.ToLower(name) {
	case "pwsh", "powershell":
		return []string{sh, "-NoLogo", "-NonInteractive", "-Command", line}
	case "cmd":
		return []string{sh, "/C", line}
	case "fish":
		return []string{sh, "-l", "-c", line}
	case "sh", "bash", "zsh", "dash", "ksh", "mksh", "yash":
		// Interactive, for the functions and aliases of its rc file.
		return []string{sh, "-l", "-i", "-c", line}
	}
	return Words(line)
}

// noHooks returns a file of Claude Code's settings that turns its hooks
// off, kero's own among them, which would take the fork for the pane's
// agent. A file, not the JSON as an argument, whose quotes a launcher
// script, as npm's claude.cmd, would mangle.
func noHooks() (string, error) {
	path := filepath.Join(os.TempDir(), "kero-commit-message-settings.json")
	want := []byte(`{"disableAllHooks":true}` + "\n")
	if b, err := os.ReadFile(path); err == nil && bytes.Equal(b, want) {
		return path, nil
	}
	return path, os.WriteFile(path, want, 0o644)
}

// validID is what an agent's conversation's id may be, as kerod resumes
// one: nothing an argument could be taken as an option by.
var validID = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,127}$`)

// Write runs argv in dir with prompt on its stdin, and returns the message
// it printed.
func Write(ctx context.Context, dir string, argv []string, prompt string) (string, error) {
	if len(argv) == 0 {
		return "", ErrNoWriter
	}
	// Codex prints its progress besides the message: the last message,
	// written to a file, is the message alone.
	var last string
	if isCodex(argv[0]) {
		f, err := os.CreateTemp("", "kero-message-*.txt")
		if err != nil {
			return "", err
		}
		f.Close()
		last = f.Name()
		defer os.Remove(last)
		argv = append(argv, "--output-last-message", last)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	cmd.Stdin = strings.NewReader(prompt)
	cmd.WaitDelay = 2 * time.Second
	hide(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = strings.TrimSpace(msg[i+1:])
		}
		return "", fmt.Errorf("%s: %v: %s", filepath.Base(argv[0]), err, msg)
	}
	out := stdout.String()
	if last != "" {
		if b, err := os.ReadFile(last); err == nil && len(bytes.TrimSpace(b)) > 0 {
			out = string(b)
		}
	}
	msg := Clean(out)
	if msg == "" {
		return "", errors.New(filepath.Base(argv[0]) + " wrote no message")
	}
	return msg, nil
}

// Clean takes a message out of what a model printed: its code fence, the
// quotes around it, and the trailing spaces of its lines.
func Clean(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') && !strings.Contains(s[1:len(s)-1], "\n") {
		s = s[1 : len(s)-1]
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Words splits a command into its words at spaces, but those within
// double quotes, which go: a Windows path keeps its backslashes.
func Words(s string) []string {
	var words []string
	var b strings.Builder
	quoted, word := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted, word = !quoted, true
		case (r == ' ' || r == '\t') && !quoted:
			if word {
				words = append(words, b.String())
				b.Reset()
			}
			word = false
		default:
			b.WriteRune(r)
			word = true
		}
	}
	if word {
		words = append(words, b.String())
	}
	return words
}
