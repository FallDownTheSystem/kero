package ama

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kero/docs"
	"kero/internal/settings"
)

// The status line turns off in Ama's folder alone, as Kero's statusLine
// in its .claude/settings.json, beside the keys the user put there, and
// back on without them; a status line of the user's own there, and the
// local file, which wins over it, stay as they are.
func TestStatusLine(t *testing.T) {
	t.Setenv("KERO_CONFIG", filepath.Join(t.TempDir(), "config"))
	file := SettingsFile()
	local := filepath.Join(filepath.Dir(file), "settings.local.json")
	read := func() map[string]any {
		t.Helper()
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s: %v", b, err)
		}
		return m
	}

	// Nothing there, nothing to turn on: no file.
	if err := StatusLine(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("turning it on made %s", file)
	}

	// Off: Kero's own file, with the empty command.
	if err := StatusLine(true); err != nil {
		t.Fatal(err)
	}
	sl, _ := read()["statusLine"].(map[string]any)
	if sl["type"] != "command" || sl["command"] != "" || len(sl) != 2 {
		t.Fatalf("statusLine %v", read()["statusLine"])
	}
	if err := StatusLine(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("turning it back on left Kero's file")
	}

	// The user's keys stay, as does their local file.
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte(`{"model": "sonnet", "permissions": {"allow": ["Bash(kero:*)"]}}`), 0o644)
	userLocal := `{"statusLine": {"type": "command", "command": ""}}`
	os.WriteFile(local, []byte(userLocal), 0o644)
	if err := StatusLine(true); err != nil {
		t.Fatal(err)
	}
	m := read()
	if m["model"] != "sonnet" || m["permissions"] == nil || m["statusLine"] == nil {
		t.Fatalf("off, with the user's keys: %v", m)
	}
	if err := StatusLine(false); err != nil {
		t.Fatal(err)
	}
	if m := read(); m["model"] != "sonnet" || m["permissions"] == nil || m["statusLine"] != nil {
		t.Fatalf("on again: %v", m)
	}
	if b, _ := os.ReadFile(local); string(b) != userLocal {
		t.Errorf("settings.local.json changed: %s", b)
	}

	// A status line of the user's own for Ama: neither setting touches it.
	own := `{"statusLine": {"type": "command", "command": "echo ama"}}`
	os.WriteFile(file, []byte(own), 0o644)
	for _, hide := range []bool{true, false} {
		if err := StatusLine(hide); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(file); string(b) != own {
			t.Errorf("hide %v replaced the user's status line: %s", hide, b)
		}
	}

	// A file the user broke stays broken, not replaced.
	os.WriteFile(file, []byte(`{"model": `), 0o644)
	if err := StatusLine(true); err == nil {
		t.Error("a broken file passed")
	}
	if b, _ := os.ReadFile(file); string(b) != `{"model": ` {
		t.Errorf("a broken file was rewritten: %s", b)
	}
}

// Ama's command is its agent's with ama-agent-args after it, and its
// folder's instructions are the ama-prompt setting's, else Kero's, with
// USER.md on top for Claude Code; back to Kero's as the setting empties.
func TestPromptAndArgs(t *testing.T) {
	t.Setenv("KERO_CONFIG", filepath.Join(t.TempDir(), "config"))
	set := settings.Default()
	set.AmaAgent, set.AmaAgentArgs = "cc --yolo", `--model sonnet --append-system-prompt "be brief"`
	if got, want := Command(set), []string{"cc", "--yolo", "--model", "sonnet", "--append-system-prompt", "be brief"}; !slices.Equal(got, want) {
		t.Errorf("command %q, want %q", got, want)
	}
	set.AmaAgent, set.AmaAgentArgs = "", ""
	if got := Command(set); !slices.Equal(got, []string{"claude"}) {
		t.Errorf("default agent: %q", got)
	}

	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(Dir(), name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	set.AmaPrompt = "You are Ama. Only list sessions."
	if err := PrepareWith(set); err != nil {
		t.Fatal(err)
	}
	claude, agents := read("CLAUDE.md"), read("AGENTS.md")
	for name, text := range map[string]string{"CLAUDE.md": claude, "AGENTS.md": agents} {
		if !strings.Contains(text, "You are Ama. Only list sessions.") || strings.Contains(text, "## How you drive Kero") {
			t.Errorf("%s with ama-prompt:\n%s", name, text)
		}
	}
	if !strings.HasSuffix(claude, "\n@USER.md\n") {
		t.Errorf("CLAUDE.md lost USER.md: %q", claude[max(0, len(claude)-40):])
	}
	set.AmaPrompt = ""
	if err := PrepareWith(set); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read("CLAUDE.md"), "## How you drive Kero") || strings.Contains(read("AGENTS.md"), "Only list sessions") {
		t.Error("an empty ama-prompt didn't bring Kero's back")
	}
}

// Ama's folder has Kero's docs, which its instructions list page by page,
// with a role prompt of the user's too; a page an older Kero left there
// goes, and a file of someone else's stays.
func TestDocs(t *testing.T) {
	t.Setenv("KERO_CONFIG", filepath.Join(t.TempDir(), "config"))
	os.MkdirAll(DocsDir(), 0o755)
	stale, other := filepath.Join(DocsDir(), "gone.md"), filepath.Join(DocsDir(), "notes.txt")
	os.WriteFile(stale, []byte("# A page Kero dropped\n"), 0o644)
	os.WriteFile(other, []byte("mine"), 0o644)

	set := settings.Default()
	set.AmaPrompt = "You are Ama. Only list sessions."
	if err := PrepareWith(set); err != nil {
		t.Fatal(err)
	}
	pages := docs.Pages()
	if len(pages) == 0 {
		t.Fatal("no docs")
	}
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		b, err := os.ReadFile(filepath.Join(Dir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "## Kero's docs") {
			t.Errorf("%s has no part on the docs", name)
		}
		for _, p := range pages {
			if !strings.Contains(string(b), "- docs/"+p.Name+": "+p.Title+". ") {
				t.Errorf("%s doesn't list docs/%s", name, p.Name)
			}
		}
	}
	for _, p := range pages {
		b, err := os.ReadFile(filepath.Join(DocsDir(), p.Name))
		if err != nil || string(b) != string(p.Text) {
			t.Errorf("docs/%s isn't Kero's: %v", p.Name, err)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("a page an older Kero left stayed")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("a file of someone else's went")
	}
}
