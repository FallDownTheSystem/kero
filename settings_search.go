package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/actions"
)

// searchEntry is a setting the Settings window's search finds: its page,
// its label as its row shows it, and words that find it besides, as its
// key in the configuration file.
type searchEntry struct {
	page, label, words string
}

var settingsIndex = []searchEntry{
	{"general", "New sessions start in", "new-session-directory folder current project home tab"},
	{"general", "Ask before closing a running program", "confirm-close quit kill"},
	{"general", "Restore sessions after a restart", "restore-sessions reboot persist"},
	{"general", "Open Kero at login", "startup launch login items"},
	{"general", "Version", "updates check for updates software update about"},
	{"general", "Check for updates automatically", "updates automatic background daily"},
	{"general", "Download and install updates automatically", "updates automatic install download"},

	{"appearance", "Appearance", "window-theme dark light mode system"},
	{"appearance", "Translucent window", "window-translucent vibrancy blur material transparency"},
	{"appearance", "Background", "background wash petal mist meadow dusk plain gradient color"},

	{"terminal", "Font", "font-family typeface jetbrains mono nerd"},
	{"terminal", "Size", "font-size zoom bigger smaller text"},
	{"terminal", "Line height", "line-height spacing leading cell"},
	{"terminal", "Weight", "font-weight bold light"},
	{"terminal", "Ligatures", "font-ligatures calt arrows"},
	{"terminal", "Thicken text", "font-thicken stroke smoothing"},
	{"terminal", "Light windows", "theme colors color scheme ghostty palette"},
	{"terminal", "Dark windows", "theme colors color scheme ghostty palette"},
	{"terminal", "Shape", "cursor-style cursor block bar underline caret"},
	{"terminal", "Blink", "cursor-style-blink cursor caret"},
	{"terminal", "Option is Meta", "macos-option-as-alt alt meta emacs readline keyboard"},
	{"terminal", "Copy on select", "copy-on-select clipboard selection"},
	{"terminal", "Right click", "right-click-action context menu paste mouse"},
	{"terminal", "Programs reading the clipboard", "clipboard-read osc 52 paste privacy"},

	{"shell", "Shell", "command fish zsh bash login"},
	{"shell", "Scrollback", "scrollback-limit history buffer lines memory"},
	{"shell", "Environment", "env variables path"},

	{"notifications", "A session needs me", "notify-on-waiting attention blocked permission"},
	{"notifications", "An agent finishes", "notify-on-done done complete"},
	{"notifications", "A program fails", "notify-on-error error"},
	{"notifications", "Play a sound", "notify-sound audio"},
	{"notifications", "Badge the Dock icon", "dock-badge count"},
	{"notifications", "Bounce the Dock icon", "dock-bounce attention"},

	{"agents", "Let commands drive this window", "remote-control kero do focus automation"},
	{"agents", "kero in your terminal", "cli command line tool install path"},
	{"agents", "Claude Code", "skill agents install"},
	{"agents", "Codex, OpenCode, Pi", "skill agents install"},
	{"agents", "Program status", "osc 7501 report progress"},

	{"advanced", "Config file", "configuration ghostty edit file dotfiles"},
	{"advanced", "kerod", "daemon background service restart sessions"},
	{"advanced", "Log", "kerod log debug"},
	{"advanced", "Folder", "data directory finder"},
	{"advanced", "Reset all settings", "defaults reset"},
}

// searchEntries are the index and the actions, which the keyboard page
// lists, with the shortcuts they have.
func searchEntries(shortcuts map[string]string) []searchEntry {
	out := append([]searchEntry(nil), settingsIndex...)
	for _, act := range actions.All {
		keys := actions.Keys(act, shortcuts)
		out = append(out, searchEntry{"keyboard", strings.TrimSuffix(act.Title, "…"),
			act.ID + " shortcut keybind " + act.Menu + " " + strings.ToLower(keys) + " " + actions.Glyphs(keys)})
	}
	return out
}

// matches reports whether every word of a query is in a text.
func matches(text string, terms []string) bool {
	text = strings.ToLower(text)
	for _, t := range terms {
		if !strings.Contains(text, t) {
			return false
		}
	}
	return len(terms) > 0
}

// searchResults returns what a query finds, by page: the labels of the
// settings it finds, an empty list for a page whose title it is, and
// nothing for the others. It returns nil for an empty query.
func searchResults(query string, shortcuts map[string]string) map[string][]string {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}
	out := map[string][]string{}
	for _, p := range settingsPages {
		if matches(p.title, terms) {
			out[p.id] = []string{}
		}
	}
	for _, e := range searchEntries(shortcuts) {
		if matches(e.label+" "+e.words, terms) {
			out[e.page] = append(out[e.page], e.label)
		}
	}
	return out
}

// The search's finds on the page shown, by label, which settingRow and
// settingsGroup highlight as they build; nil while not searching. The
// Settings window is the only view that builds them, and sets these
// before it does.
var (
	searchHits   map[string]bool
	searchReveal string // the find to scroll to, in this build only
)

// searchHighlight marks a find of the search behind el, a row or a
// title, as a tint wider than it by dx on each side, and taller by dy.
func searchHighlight(c *ui.Context, el ui.Element, label string, pal *palette, dx, dy float32) {
	hit := searchHits[label]
	t := el.Animate("hit", b2f(hit), 180*time.Millisecond)
	if label == searchReveal {
		el.ScrollIntoView()
	}
	if t > 0 {
		ui.Box(c.Key("hit")).Absolute().Top(-dy).Bottom(-dy).Left(-dx).Right(-dx).Radius(8).PassThrough().
			Background(pal.accent.Alpha(0.13*t)).InsetShadow(0, 0, 0, 1, pal.accent.Alpha(0.28*t))
	}
}
