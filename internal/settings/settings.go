// Package settings is kero's settings: one file, settings.json in kero's
// directory, that the app writes from its Settings window and kerod reads
// as it starts panes, so that the shell, its environment and the
// scrollback apply to new panes at once.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"kero/internal/proto"
)

// Settings are the user's choices. The zero value of a field is never a
// choice of its own: Load fills defaults in.
type Settings struct {
	// General
	NewSessionDir   string `json:"newSessionDir"`   // "current", "project" or "home"
	ConfirmClose    bool   `json:"confirmClose"`    // before closing a pane a program runs in
	RestoreSessions bool   `json:"restoreSessions"` // after kerod restarts, as after a reboot
	QuitConfirm     bool   `json:"quitConfirm"`     // tell, the first time, that quitting keeps sessions

	// Appearance
	Appearance string `json:"appearance"` // "system", "light" or "dark"
	Background string `json:"background"` // a wash, see Backgrounds
	Vibrancy   bool   `json:"vibrancy"`   // the window's material under the wash

	// Terminal
	FontFamily   string  `json:"fontFamily"` // "" the font kero carries; "monospace" the system's
	FontSize     float32 `json:"fontSize"`
	LineHeight   float32 `json:"lineHeight"`
	FontWeight   int     `json:"fontWeight"`
	Ligatures    bool    `json:"ligatures"`
	Thicken      bool    `json:"thicken"`
	Cursor       string  `json:"cursor"` // "block", "bar" or "underline"
	CursorBlink  bool    `json:"cursorBlink"`
	LightTheme   string  `json:"lightTheme"` // "kero", or a Ghostty theme's name
	DarkTheme    string  `json:"darkTheme"`
	OptionAsAlt  bool    `json:"optionAsAlt"`
	CopyOnSelect bool    `json:"copyOnSelect"`
	Clipboard    string  `json:"clipboard"`  // programs reading it: "ask", "allow" or "deny"
	RightClick   string  `json:"rightClick"` // "menu", "paste" or "copyOrPaste"

	// Shell
	Shell      string   `json:"shell"` // "" the account's login shell
	Env        []string `json:"env"`   // "KEY=value", added to every pane's
	Scrollback int      `json:"scrollback"`

	// Notifications
	NotifyWaiting bool `json:"notifyWaiting"` // a session needs you
	NotifyDone    bool `json:"notifyDone"`    // an agent finished
	NotifyFailed  bool `json:"notifyFailed"`  // a program reported it failed
	NotifySound   bool `json:"notifySound"`
	DockBadge     bool `json:"dockBadge"`
	BounceDock    bool `json:"bounceDock"`

	// Agents
	RemoteControl bool `json:"remoteControl"` // kero focus and kero do drive the app

	// Keyboard: accelerators by action, over the defaults; "" unbinds.
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
}

// Default returns the settings of a new install.
func Default() Settings {
	return Settings{
		NewSessionDir:   "current",
		ConfirmClose:    true,
		RestoreSessions: true,
		QuitConfirm:     true,
		Appearance:      "system",
		Background:      "rex",
		Vibrancy:        true,
		FontSize:        13,
		LineHeight:      1.22,
		FontWeight:      400,
		Ligatures:       true,
		Cursor:          "block",
		CursorBlink:     true,
		LightTheme:      "kero",
		DarkTheme:       "kero",
		Clipboard:       "ask",
		RightClick:      "menu",
		Scrollback:      4,
		NotifyWaiting:   true,
		NotifyDone:      true,
		NotifyFailed:    true,
		DockBadge:       true,
		RemoteControl:   true,
	}
}

// Backgrounds are the washes the window can show.
var Backgrounds = []struct{ ID, Name string }{
	{"rex", "Petal"},
	{"mist", "Mist"},
	{"meadow", "Meadow"},
	{"dusk", "Dusk"},
	{"plain", "Plain"},
}

// Path returns the settings' file.
func Path() string { return filepath.Join(proto.Dir(), "settings.json") }

var (
	mu    sync.Mutex
	cache *Settings
	mtime int64
)

// Load returns the settings, the defaults where the file says nothing.
// It reads the file again when it changed, so that a long-running reader,
// as kerod, sees the app's changes.
func Load() Settings {
	mu.Lock()
	defer mu.Unlock()
	st, err := os.Stat(Path())
	if err != nil {
		return Default()
	}
	if cache != nil && st.ModTime().UnixNano() == mtime {
		return clone(*cache)
	}
	s := Default()
	if b, err := os.ReadFile(Path()); err == nil {
		// Over the defaults: fields the file leaves out keep them.
		json.Unmarshal(b, &s)
	}
	s.normalize()
	cache, mtime = &s, st.ModTime().UnixNano()
	return clone(s)
}

// Save writes the settings.
func Save(s Settings) error {
	s.normalize()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(proto.Dir(), 0o700); err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, Path()); err != nil {
		return err
	}
	mu.Lock()
	c := clone(s)
	cache = &c
	if st, err := os.Stat(Path()); err == nil {
		mtime = st.ModTime().UnixNano()
	}
	mu.Unlock()
	return nil
}

// Clone returns a copy that shares nothing with s.
func (s Settings) Clone() Settings { return clone(s) }

func clone(s Settings) Settings {
	s.Env = append([]string(nil), s.Env...)
	if s.Shortcuts != nil {
		m := make(map[string]string, len(s.Shortcuts))
		for k, v := range s.Shortcuts {
			m[k] = v
		}
		s.Shortcuts = m
	}
	return s
}

// normalize keeps values in range, as a file edited by hand may not.
func (s *Settings) normalize() {
	d := Default()
	oneOf := func(v *string, def string, ok ...string) {
		for _, o := range ok {
			if *v == o {
				return
			}
		}
		*v = def
	}
	oneOf(&s.NewSessionDir, d.NewSessionDir, "current", "project", "home")
	oneOf(&s.Appearance, d.Appearance, "system", "light", "dark")
	oneOf(&s.Cursor, d.Cursor, "block", "bar", "underline")
	oneOf(&s.Clipboard, d.Clipboard, "ask", "allow", "deny")
	oneOf(&s.RightClick, d.RightClick, "menu", "paste", "copyOrPaste")
	known := false
	for _, b := range Backgrounds {
		known = known || b.ID == s.Background
	}
	if !known {
		s.Background = d.Background
	}
	if s.FontSize < 8 || s.FontSize > 36 {
		s.FontSize = d.FontSize
	}
	if s.LineHeight < 0.8 || s.LineHeight > 2.5 {
		s.LineHeight = d.LineHeight
	}
	if s.FontWeight < 100 || s.FontWeight > 900 {
		s.FontWeight = d.FontWeight
	}
	if s.Scrollback < 0 || s.Scrollback > 256 {
		s.Scrollback = d.Scrollback
	}
	if s.LightTheme == "" {
		s.LightTheme = d.LightTheme
	}
	if s.DarkTheme == "" {
		s.DarkTheme = d.DarkTheme
	}
}
