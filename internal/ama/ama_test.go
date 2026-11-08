package ama

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
