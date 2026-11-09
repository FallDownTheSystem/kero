package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kero/internal/proto"
)

// claudePlugin is Kero's plugin for Claude Code: a monitor that runs kero
// inbox for the whole session, which gives Claude the messages Kero has
// for it, as orchestrators' reports, rather than Kero typing them into
// its prompt over what the user is typing. It lives in a marketplace of
// its own, a directory of Kero's, which Claude Code reads the plugin from
// as it starts a session: an update of its files needs nothing else.
// Claude Code's own commands register the marketplace and install the
// plugin, in the user's settings.
type claudePlugin struct {
	files    agentPlugin
	settings string // Claude Code's settings.json
	known    string // its plugins' known_marketplaces.json
}

const (
	claudeMarket = "kero"
	claudeID     = "kero@kero"
)

func newClaudePlugin(claudeDir string) claudePlugin {
	return claudePlugin{
		files: agentPlugin{agent: "claude", dir: filepath.Join(proto.Dir(), "claude-plugin"), own: true, files: map[string]string{
			filepath.Join(".claude-plugin", "marketplace.json"):      "agents/claude/marketplace.json",
			filepath.Join("plugin", ".claude-plugin", "plugin.json"): "agents/claude/plugin.json",
		}},
		settings: filepath.Join(claudeDir, "settings.json"),
		known:    filepath.Join(claudeDir, "plugins", "known_marketplaces.json"),
	}
}

func (c claudePlugin) name() string { return "claude" }
func (c claudePlugin) path() string { return c.files.dir }

type claudeMarkets map[string]struct {
	Source struct{ Source, Path string }
}

// dir is where the marketplace named kero is a directory, "" for none.
func (m claudeMarkets) dir() string {
	if s := m[claudeMarket].Source; s.Source == "directory" {
		return s.Path
	}
	return ""
}

// registered tells where Claude Code has the marketplace named kero, ""
// for nowhere, and whether the user's settings have the plugin on. Claude
// Code adds a marketplace to the settings and to its plugins' list of
// them; one added by an older version is in the list only.
func (c claudePlugin) registered() (market string, on bool) {
	var s struct {
		Markets claudeMarkets   `json:"extraKnownMarketplaces"`
		Enabled map[string]bool `json:"enabledPlugins"`
	}
	if b, err := os.ReadFile(c.settings); err == nil {
		json.Unmarshal(b, &s)
	}
	if market = s.Markets.dir(); market == "" {
		var known claudeMarkets
		if b, err := os.ReadFile(c.known); err == nil && json.Unmarshal(b, &known) == nil {
			market = known.dir()
		}
	}
	return market, s.Enabled[claudeID]
}

// ours tells that the settings' marketplace kero is this Kero's.
func (c claudePlugin) ours(market string) bool {
	return market != "" && strings.EqualFold(filepath.Clean(market), filepath.Clean(c.files.dir))
}

func (c claudePlugin) state() string {
	market, on := c.registered()
	files := c.files.state()
	switch {
	case files == "current" && c.ours(market) && on:
		return "current"
	case files == "missing" && market == "" && !on:
		return "missing"
	}
	return "outdated"
}

// claudeCLI runs Claude Code's command, as the user would.
var claudeCLI = func(args ...string) error {
	exe, err := exec.LookPath("claude")
	if err != nil {
		return errors.New("Claude Code's claude command is not on PATH, to install Kero's plugin with")
	}
	cmd := exec.Command(exe, args...)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("claude %s: %v: %s", strings.Join(args, " "), err, proto.OneLine(string(out)))
	}
	return nil
}

func (c claudePlugin) install() error {
	if err := c.files.install(); err != nil {
		return err
	}
	market, on := c.registered()
	if !c.ours(market) {
		if market != "" {
			// Another Kero's, as a development build's: this one's now.
			claudeCLI("plugin", "marketplace", "remove", claudeMarket)
			on = false
		}
		if err := claudeCLI("plugin", "marketplace", "add", c.files.dir); err != nil {
			return err
		}
	}
	if !on {
		return claudeCLI("plugin", "install", claudeID)
	}
	return nil
}

// remove takes this Kero's plugin out of Claude Code, and its files: the
// settings' kero@kero stays when it is another Kero's, as the installed
// one's beside a development build.
func (c claudePlugin) remove() error {
	if market, on := c.registered(); c.ours(market) {
		if on {
			if err := claudeCLI("plugin", "uninstall", claudeID); err != nil {
				return err
			}
		}
		if err := claudeCLI("plugin", "marketplace", "remove", claudeMarket); err != nil {
			return err
		}
	}
	return os.RemoveAll(c.files.dir)
}

// sync rewrites the plugin's files of this Kero as Kero updates, when it
// is installed: Claude Code reads them from there. Every kero runs it, so
// the settings are read only for files out of date.
func (c claudePlugin) sync() {
	if c.files.state() != "outdated" {
		return
	}
	if market, _ := c.registered(); c.ours(market) {
		c.files.install()
	}
}
