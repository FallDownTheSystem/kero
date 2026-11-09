package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Setup writes Kero's plugin for Claude Code into Kero's directory and
// registers it with Claude Code's own commands; its state follows the
// files and the user's settings; an update of the files needs no command;
// remove undoes both.
func TestClaudePlugin(t *testing.T) {
	t.Setenv("KERO_HOME", t.TempDir())
	claudeDir := t.TempDir()
	p := newClaudePlugin(claudeDir)
	var ran []string
	settings := func(market string, on bool) {
		s := map[string]any{"enabledPlugins": map[string]bool{claudeID: on}}
		if market != "" {
			s["extraKnownMarketplaces"] = map[string]any{claudeMarket: map[string]any{"source": map[string]string{"source": "directory", "path": market}}}
		}
		b, _ := json.Marshal(s)
		os.WriteFile(p.settings, b, 0o644)
	}
	old := claudeCLI
	claudeCLI = func(args ...string) error {
		cmd := strings.Join(args, " ")
		ran = append(ran, cmd)
		switch {
		case strings.HasPrefix(cmd, "plugin marketplace add "):
			settings(args[3], false)
		case cmd == "plugin install "+claudeID:
			market, _ := p.registered()
			settings(market, true)
		}
		return nil
	}
	t.Cleanup(func() { claudeCLI = old })

	if st := p.state(); st != "missing" {
		t.Fatalf("before setup: %s", st)
	}
	if err := p.install(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"plugin marketplace add " + p.files.dir, "plugin install " + claudeID}; !slices.Equal(ran, want) {
		t.Errorf("ran %q", ran)
	}
	if st := p.state(); st != "current" {
		t.Errorf("after setup: %s", st)
	}
	b, _ := os.ReadFile(filepath.Join(p.files.dir, "plugin", ".claude-plugin", "plugin.json"))
	var manifest struct {
		Experimental struct {
			Monitors []struct{ Command string }
		}
	}
	if err := json.Unmarshal(b, &manifest); err != nil || len(manifest.Experimental.Monitors) != 1 || !strings.HasSuffix(manifest.Experimental.Monitors[0].Command, " inbox --follow") {
		t.Errorf("plugin.json %s: %v", b, err)
	}

	// Kero updated: the files change, which sync rewrites, with no command.
	os.WriteFile(filepath.Join(p.files.dir, "plugin", ".claude-plugin", "plugin.json"), []byte("{}"), 0o644)
	if st := p.state(); st != "outdated" {
		t.Errorf("old files: %s", st)
	}
	ran = nil
	p.sync()
	if st := p.state(); st != "current" || len(ran) > 0 {
		t.Errorf("synced: %s, ran %q", st, ran)
	}

	// Another Kero's marketplace kero: setup takes it over.
	settings(filepath.Join(t.TempDir(), "other"), true)
	if st := p.state(); st != "outdated" {
		t.Errorf("another's: %s", st)
	}
	ran = nil
	p.install()
	if len(ran) != 3 || ran[0] != "plugin marketplace remove kero" || p.state() != "current" {
		t.Errorf("took over with %q: %s", ran, p.state())
	}

	// Added by an older Claude Code: in its plugins' list only.
	settings("", true)
	os.MkdirAll(filepath.Dir(p.known), 0o755)
	b, _ = json.Marshal(map[string]any{claudeMarket: map[string]any{"source": map[string]string{"source": "directory", "path": p.files.dir}}})
	os.WriteFile(p.known, b, 0o644)
	if st := p.state(); st != "current" {
		t.Errorf("in the list only: %s", st)
	}

	// Another Kero's stays as its removal of its own files goes.
	other := newClaudePlugin(claudeDir)
	other.files.dir = filepath.Join(t.TempDir(), "other-kero")
	ran = nil
	if err := other.remove(); err != nil || len(ran) > 0 {
		t.Errorf("another Kero's removal ran %q: %v", ran, err)
	}

	ran = nil
	if err := p.remove(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"plugin uninstall " + claudeID, "plugin marketplace remove " + claudeMarket}; !slices.Equal(ran, want) {
		t.Errorf("remove ran %q", ran)
	}
	if _, err := os.Stat(p.files.dir); !os.IsNotExist(err) {
		t.Error("the plugin's files stay")
	}
}
