package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"kero/internal/client"
	"kero/internal/clilink"
	"kero/internal/proto"
)

// hookCmd is what an agent runs as its session starts (the hook `kero
// skill setup` installs): it tells the agent that it runs in Kero, in
// which session and pane, and to name the session after its task; and
// tells kerod the agent's own session, to resume after a restart. It never
// fails the agent: outside Kero, or on any trouble, it says nothing.
func hookCmd(args []string) error {
	agent := ""
	if len(args) > 0 {
		agent = args[0]
	}
	pane := os.Getenv("KERO_PANE")
	var in struct {
		Event   string `json:"hook_event_name"`
		AgentID string `json:"agent_id"`   // set for Claude Code's subagents
		Session string `json:"session_id"` // the agent's own
	}
	json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&in)
	if pane == "" || in.Event != "SessionStart" || in.AgentID != "" {
		return nil
	}
	if t := os.Getenv("CODEX_THREAD_ID"); t != "" && in.Session != "" && t != in.Session {
		return nil // a Codex run by another agent in this pane, not the pane's
	}
	c, err := client.Connect(true)
	if err != nil {
		c = nil
	} else {
		defer c.Close()
		if in.Session != "" {
			c.Call("pane.agentSession", proto.AgentSessionParams{ID: pane, Agent: agent, Session: in.Session}, nil)
		}
	}
	out := map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":     "SessionStart",
		"additionalContext": hookContext(c, pane, os.Getenv("KERO_SESSION")),
	}}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// hookContext is what the hook tells an agent in pane, of session sid;
// c is kerod, nil when it is unreachable.
func hookContext(c *client.Client, pane, sid string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are running in Kero, a terminal for coding agents: session %s, pane %s ($KERO_SESSION and $KERO_PANE). "+
		"The `kero` command (see the kero skill) runs things beside you, such as a dev server, tests or another agent, "+
		"and tells the user what you are doing.", sid, pane)
	if c == nil {
		return b.String()
	}
	var st proto.State
	if c.Call("state", nil, &st) != nil {
		return b.String()
	}
	for _, s := range st.Sessions {
		if s.ID != sid || len(s.Panes) == 0 {
			continue
		}
		switch {
		case s.Name != "":
			fmt.Fprintf(&b, " The user knows this session as %q: leave its name alone.", s.Name)
		case s.Panes[0].ID != pane:
			b.WriteString(" You run in one of the session's other panes: leave its name alone.")
		default:
			fmt.Fprintf(&b, " The session has no name yet. As soon as you know your task, name the session after it "+
				"in a few words joined by dashes, so the user finds it in Kero's sidebar: `kero rename %s fix-login-redirect`. "+
				"Do it once, without asking or mentioning it.", sid)
		}
	}
	return b.String()
}

// agentHook is a hook kero installs in an agent's settings: a SessionStart
// command running `kero hook <agent>`.
type agentHook struct {
	agent string // as `kero hook` takes it: "claude", "codex"
	file  string // the JSON file holding the agent's hooks
}

// command is the hook's command line: the kero command of Kero's own
// directory (package clilink), which stays where it is as Kero updates.
func (h agentHook) command() string {
	exe := filepath.Join(proto.Dir(), "bin", "kero")
	if runtime.GOOS == "windows" {
		return `"` + exe + `.exe" hook ` + h.agent
	}
	return "'" + strings.ReplaceAll(exe, "'", `'\''`) + "' hook " + h.agent
}

// ours tells kero's hook commands, of this kero or another's.
func (h agentHook) ours(command string) bool {
	return strings.Contains(command, "kero") && strings.HasSuffix(command, " hook "+h.agent)
}

// state is "current", "outdated" (kero's hook, of another command line)
// or "missing".
func (h agentHook) state() string {
	b, err := os.ReadFile(h.file)
	if err != nil {
		return "missing"
	}
	settings, err := parseObject(b)
	if err != nil {
		return "missing"
	}
	state := "missing"
	h.each(settings, func(command string) {
		if command == h.command() {
			state = "current"
		} else if state == "missing" {
			state = "outdated"
		}
	})
	return state
}

// each calls fn with the command of each of kero's SessionStart hooks.
func (h agentHook) each(settings object, fn func(command string)) {
	hooks, _ := parseObject(settings.get("hooks"))
	var groups []object
	json.Unmarshal(hooks.get("SessionStart"), &groups)
	for _, g := range groups {
		var handlers []struct{ Type, Command string }
		json.Unmarshal(g.get("hooks"), &handlers)
		for _, hd := range handlers {
			if hd.Type == "command" && h.ours(hd.Command) {
				fn(hd.Command)
			}
		}
	}
}

// install puts the hook in the agent's settings, in place of kero's
// others, keeping the rest of the file as it is.
func (h agentHook) install() error {
	// The command the hook runs, should kerod not have made it yet.
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		bin := filepath.Join(proto.Dir(), "bin")
		if clilink.Target(bin) == "" {
			clilink.Link(exe, bin)
		}
	}
	return h.edit(true)
}

// remove takes kero's hooks out of the agent's settings.
func (h agentHook) remove() error {
	if _, err := os.Stat(h.file); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return h.edit(false)
}

func (h agentHook) edit(add bool) error {
	b, err := os.ReadFile(h.file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	settings := object{}
	if len(bytes.TrimSpace(b)) > 0 {
		if settings, err = parseObject(b); err != nil {
			return fmt.Errorf("%s: %w", proto.ShortDir(h.file), err)
		}
	}
	hooks, err := parseObject(settings.get("hooks"))
	if err != nil {
		return fmt.Errorf("%s: hooks: %w", proto.ShortDir(h.file), err)
	}
	var groups []object
	if raw := hooks.get("SessionStart"); raw != nil {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return fmt.Errorf("%s: hooks.SessionStart: %w", proto.ShortDir(h.file), err)
		}
	}
	// Already there, alone: nothing to do. Else drop kero's handlers, and
	// the groups they leave empty.
	n, exact := 0, false
	h.each(settings, func(command string) {
		n++
		exact = exact || command == h.command()
	})
	if add && n == 1 && exact || !add && n == 0 {
		return nil
	}
	kept := groups[:0]
	for _, g := range groups {
		var handlers []json.RawMessage
		json.Unmarshal(g.get("hooks"), &handlers)
		n := len(handlers)
		handlers = slices.DeleteFunc(handlers, func(raw json.RawMessage) bool {
			var hd struct{ Type, Command string }
			json.Unmarshal(raw, &hd)
			return hd.Type == "command" && h.ours(hd.Command)
		})
		if n > 0 && len(handlers) == 0 {
			continue
		}
		if len(handlers) != n {
			g = g.set("hooks", marshal(handlers))
		}
		kept = append(kept, g)
	}
	groups = kept
	if add {
		handler := object{{"type", marshal("command")}, {"command", marshal(h.command())}, {"timeout", marshal(10)}}
		groups = append(groups, object{{"hooks", marshal([]object{handler})}})
	}
	if len(groups) > 0 {
		hooks = hooks.set("SessionStart", marshal(groups))
	} else {
		hooks = hooks.del("SessionStart")
	}
	if len(hooks) > 0 {
		settings = settings.set("hooks", hooks.marshal())
	} else {
		settings = settings.del("hooks")
	}
	out := settings.marshal()
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, out, "", "  "); err != nil {
		return err
	}
	pretty.WriteByte('\n')
	if bytes.Equal(pretty.Bytes(), b) {
		return nil
	}
	return writeFileAtomic(h.file, pretty.Bytes())
}

// agentHooks returns the hooks of the agents kero has hooks for: Claude
// Code's, and Codex's when it is installed (or named).
func agentHooks(agent string) []agentHook {
	claude := os.Getenv("CLAUDE_CONFIG_DIR")
	if claude == "" {
		claude = filepath.Join(proto.Home(), ".claude")
	}
	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(proto.Home(), ".codex")
	}
	var out []agentHook
	switch agent {
	case "", "claude":
		out = append(out, agentHook{"claude", filepath.Join(claude, "settings.json")})
	}
	switch agent {
	case "", "agents":
		if st, err := os.Stat(codex); err != nil || !st.IsDir() {
			break
		}
		fallthrough
	case "codex":
		out = append(out, agentHook{"codex", filepath.Join(codex, "hooks.json")})
	}
	return out
}

// object is a JSON object that keeps its keys' order, so that editing a
// settings file leaves the rest of it as it was.
type object []member

type member struct {
	Key string
	Val json.RawMessage
}

func parseObject(b []byte) (object, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return object{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	var o object
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		o = o.set(t.(string), v)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("more than one JSON value")
	}
	return o, nil
}

func (o *object) UnmarshalJSON(b []byte) error {
	p, err := parseObject(b)
	*o = p
	return err
}

func (o object) get(key string) json.RawMessage {
	for _, m := range o {
		if m.Key == key {
			return m.Val
		}
	}
	return nil
}

func (o object) set(key string, v json.RawMessage) object {
	for i, m := range o {
		if m.Key == key {
			o[i].Val = v
			return o
		}
	}
	return append(o, member{key, v})
}

func (o object) del(key string) object {
	return slices.DeleteFunc(o, func(m member) bool { return m.Key == key })
}

func (o object) marshal() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(marshal(m.Key))
		b.WriteByte(':')
		b.Write(m.Val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

func (o object) MarshalJSON() ([]byte, error) { return o.marshal(), nil }

func marshal(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return bytes.TrimSpace(b.Bytes())
}

// writeFileAtomic replaces a file whole, keeping its mode.
func writeFileAtomic(path string, b []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
