// Package actions names what kero's app does, for its menus, its command
// palette, the shortcuts the user rebinds in Settings, and `kero do`,
// which runs them from a shell or an agent (Remote Control).
package actions

import "strings"

// Action is something the app does.
type Action struct {
	ID    string
	Title string
	Menu  string // the menu it shows in
	Keys  string // the default accelerator, as "CmdOrCtrl+Shift+D"; "" none
	// Remote tells that `kero do` may run it: what needs no dialog or
	// focus of the user's.
	Remote bool
}

// All are the app's actions, in their menus' order.
var All = []Action{
	{ID: "settings.open", Title: "Settings…", Menu: "Kero", Keys: "CmdOrCtrl+,", Remote: true},
	{ID: "cli.install", Title: "Install Command Line Tool…", Menu: "Kero"},
	{ID: "app.quitAll", Title: "Quit and End All Sessions…", Menu: "Kero", Keys: "CmdOrCtrl+Alt+Q"},

	{ID: "session.new", Title: "New Session", Menu: "Session", Keys: "CmdOrCtrl+T", Remote: true},
	{ID: "session.newHome", Title: "New Session in Home", Menu: "Session", Keys: "CmdOrCtrl+Shift+T", Remote: true},
	{ID: "project.open", Title: "Open Project…", Menu: "Session", Keys: "CmdOrCtrl+O"},
	{ID: "palette.open", Title: "Go to Session…", Menu: "Session", Keys: "CmdOrCtrl+P", Remote: true},
	{ID: "session.attention", Title: "Jump to Session Needing You", Menu: "Session", Keys: "CmdOrCtrl+J", Remote: true},
	{ID: "session.next", Title: "Next Session", Menu: "Session", Keys: "CmdOrCtrl+Shift+]", Remote: true},
	{ID: "session.previous", Title: "Previous Session", Menu: "Session", Keys: "CmdOrCtrl+Shift+[", Remote: true},
	{ID: "session.rename", Title: "Rename Session", Menu: "Session", Keys: "CmdOrCtrl+R"},
	{ID: "pane.restart", Title: "Restart Pane", Menu: "Session", Keys: "CmdOrCtrl+Shift+R", Remote: true},
	{ID: "pane.close", Title: "Close", Menu: "Session", Keys: "CmdOrCtrl+W"},
	{ID: "session.close", Title: "Close Session", Menu: "Session", Keys: "CmdOrCtrl+Shift+W"},

	{ID: "pane.split.right", Title: "Split Right", Menu: "Pane", Keys: "CmdOrCtrl+D", Remote: true},
	{ID: "pane.split.down", Title: "Split Down", Menu: "Pane", Keys: "CmdOrCtrl+Shift+D", Remote: true},
	{ID: "pane.zoom", Title: "Zoom Pane", Menu: "Pane", Keys: "CmdOrCtrl+Shift+Enter", Remote: true},
	{ID: "pane.next", Title: "Next Pane", Menu: "Pane", Keys: "CmdOrCtrl+]", Remote: true},
	{ID: "pane.previous", Title: "Previous Pane", Menu: "Pane", Keys: "CmdOrCtrl+[", Remote: true},
	{ID: "pane.left", Title: "Pane Left", Menu: "Pane", Keys: "CmdOrCtrl+Alt+Left", Remote: true},
	{ID: "pane.right", Title: "Pane Right", Menu: "Pane", Keys: "CmdOrCtrl+Alt+Right", Remote: true},
	{ID: "pane.up", Title: "Pane Above", Menu: "Pane", Keys: "CmdOrCtrl+Alt+Up", Remote: true},
	{ID: "pane.down", Title: "Pane Below", Menu: "Pane", Keys: "CmdOrCtrl+Alt+Down", Remote: true},

	{ID: "sidebar.toggle", Title: "Toggle Sidebar", Menu: "View", Keys: "CmdOrCtrl+B", Remote: true},
	{ID: "text.bigger", Title: "Bigger", Menu: "View", Keys: "CmdOrCtrl+=", Remote: true},
	{ID: "text.smaller", Title: "Smaller", Menu: "View", Keys: "CmdOrCtrl+-", Remote: true},
	{ID: "text.reset", Title: "Actual Size", Menu: "View", Keys: "CmdOrCtrl+0", Remote: true},
}

// Find returns an action by its id.
func Find(id string) (Action, bool) {
	for _, a := range All {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}

// Keys returns an action's accelerator: the user's, over the default.
func Keys(a Action, overrides map[string]string) string {
	if k, ok := overrides[a.ID]; ok {
		return k
	}
	return a.Keys
}

// Glyphs writes an accelerator as macOS menus show it: "⇧⌘D".
func Glyphs(accel string) string {
	if accel == "" {
		return ""
	}
	parts := strings.Split(accel, "+")
	key := parts[len(parts)-1]
	if key == "" && len(parts) > 1 { // "CmdOrCtrl++"
		key = "+"
	}
	var ctrl, alt, shift, cmd bool
	for _, m := range parts[:len(parts)-1] {
		switch strings.ToLower(m) {
		case "cmdorctrl", "commandorcontrol", "cmd", "command", "super", "meta":
			cmd = true
		case "ctrl", "control":
			ctrl = true
		case "alt", "option":
			alt = true
		case "shift":
			shift = true
		}
	}
	var b strings.Builder
	if ctrl {
		b.WriteString("⌃")
	}
	if alt {
		b.WriteString("⌥")
	}
	if shift {
		b.WriteString("⇧")
	}
	if cmd {
		b.WriteString("⌘")
	}
	names := map[string]string{
		"enter": "↩", "return": "↩", "tab": "⇥", "space": "Space", "backspace": "⌫", "delete": "⌦",
		"escape": "⎋", "esc": "⎋", "up": "↑", "down": "↓", "left": "←", "right": "→",
		"home": "↖", "end": "↘", "pageup": "⇞", "pagedown": "⇟", "plus": "+",
	}
	if n, ok := names[strings.ToLower(key)]; ok {
		b.WriteString(n)
	} else {
		b.WriteString(strings.ToUpper(key))
	}
	return b.String()
}
