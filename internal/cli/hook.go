package cli

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	"kero/internal/client"
	"kero/internal/clilink"
	"kero/internal/proto"
)

// hookCmd is what an agent runs as its session starts (the hook `kero
// skill setup` installs): it tells the agent that it runs in Kero, in
// which session and pane, and tells kerod the agent's own session, to
// resume after a restart. It never fails the agent: outside Kero, or on
// any trouble, it says nothing.
func hookCmd(args []string) error {
	agent := ""
	if len(args) > 0 {
		agent = args[0]
	}
	if agent == "context" && len(args) >= 3 {
		return contextCmd(args[1], args[2], len(args) > 3 && args[3] == "wait")
	}
	pane := os.Getenv("KERO_PANE")
	var in hookInput
	json.NewDecoder(io.LimitReader(os.Stdin, 16<<20)).Decode(&in)
	if in.AgentID != "" {
		return nil
	}
	if t := os.Getenv("CODEX_THREAD_ID"); t != "" && in.Session != "" && t != in.Session {
		return nil // a Codex run by another agent in this pane, not the pane's
	}
	// A turn's end kerod finds the pane of by the agent's session: a hook
	// with no pane's environment, as of OpenCode's service, which panes
	// share, or of an agent outside Kero, by that alone.
	if end := in.turnEnd(); end != "" {
		if in.Session != "" || pane != "" {
			turnEnded(agent, end, in, pane == "")
		}
		return nil
	}
	if pane == "" || in.Event != "SessionStart" {
		return nil
	}
	sid, away := os.Getenv("KERO_SESSION"), false
	c, err := client.Connect(true)
	if err != nil {
		c = nil
	} else {
		defer c.Close()
		if in.Session != "" {
			// kerod tells the pane: the environment may be that of
			// another, as when Codex's app server, which panes share,
			// runs the hook.
			var r proto.AgentSessionResult
			if c.Call("pane.agentSession", proto.AgentSessionParams{PID: os.Getpid(), Agent: agent, Session: in.Session, Dir: in.Cwd}, &r) != nil {
				return nil
			}
			pane, sid, away = r.Pane, r.Session, r.Away
		}
		if in.Source != "resume" && in.Source != "compact" {
			forgetAgentName(c, pane, sid)
		}
	}
	out := map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":     "SessionStart",
		"additionalContext": hookContext(c, pane, sid, away),
	}}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// hookInput is what an agent gives its hook, of the fields kero reads: as
// Claude Code and Codex give it, which Kero's plugins for other agents
// give too.
type hookInput struct {
	Event   string `json:"hook_event_name"`
	AgentID string `json:"agent_id"`   // set for Claude Code's subagents
	Session string `json:"session_id"` // the agent's own
	Cwd     string `json:"cwd"`
	Source  string `json:"source"` // "startup", "resume", "clear", "compact"
	// Of a turn's end: its last reply, null or missing for none known;
	// what still runs after it; a failed turn's error.
	Reply      *string `json:"last_assistant_message"`
	Background *[]struct {
		Type string `json:"type"`
	} `json:"background_tasks"`
	Error string `json:"error"`
}

// turnEnd is how the event ends the agent's turn, proto.TurnDone,
// TurnInterrupted or TurnFailed; "" for an event that ends none.
func (in hookInput) turnEnd() string {
	switch in.Event {
	case "Stop":
		return proto.TurnDone
	case "Interrupt":
		return proto.TurnInterrupted
	case "StopFailure":
		return proto.TurnFailed
	}
	return ""
}

// turnEnded tells kerod that the agent ended its turn, for its
// orchestrator's report. It prints nothing, which the agent takes as
// nothing to do.
func turnEnded(agent, end string, in hookInput, bySession bool) {
	c, err := client.Connect(true)
	if err != nil {
		return
	}
	defer c.Close()
	p := proto.TurnParams{PID: os.Getpid(), BySession: bySession, Agent: agent, Session: in.Session, Dir: in.Cwd, End: end}
	switch end {
	case proto.TurnFailed:
		p.Error = in.Error
		if in.Reply != nil && *in.Reply != "" {
			p.Error = *in.Reply // the error as the conversation shows it
		}
	default:
		if in.Reply != nil {
			p.Reply, p.HasReply = *in.Reply, true
		}
		if in.Background != nil {
			p.HasBackground = true
			var types []string
			for _, t := range *in.Background {
				types = append(types, t.Type)
			}
			p.Background = countOf(types)
		}
	}
	c.Call("pane.turn", p, nil)
}

// countOf tells how many of each there are, in the order they first come:
// "1 shell, 2 monitors".
func countOf(kinds []string) string {
	var order []string
	n := map[string]int{}
	for _, k := range kinds {
		if k == "" {
			k = "task"
		}
		if n[k] == 0 {
			order = append(order, k)
		}
		n[k]++
	}
	parts := make([]string, len(order))
	for i, k := range order {
		if n[k] > 1 {
			k += "s"
		}
		parts[i] = fmt.Sprintf("%d %s", n[order[i]], k)
	}
	return strings.Join(parts, ", ")
}

// forgetAgentName drops the name an agent gave session sid, as a new
// conversation starts in its first pane, the one the name came from: the
// session shows the pane's title again.
func forgetAgentName(c *client.Client, pane, sid string) {
	var st proto.State
	if c.Call("state", nil, &st) != nil {
		return
	}
	for _, s := range st.Sessions {
		if s.ID == sid && s.AgentNamed && len(s.Panes) > 0 && s.Panes[0].ID == pane {
			c.Call("rename", proto.RenameParams{ID: sid}, nil)
		}
	}
}

// contextCmd prints what to tell the agent of an agent's session, for an
// agent whose commands run away from its pane, as OpenCode's do in its
// service: nothing when no pane runs that session. wait gives a pane that
// runs the agent a moment to tell its session.
func contextCmd(agent, session string, wait bool) error {
	c, err := client.Connect(true)
	if err != nil {
		return nil
	}
	defer c.Close()
	for deadline := time.Now().Add(1500 * time.Millisecond); ; time.Sleep(150 * time.Millisecond) {
		var st proto.State
		if c.Call("state", nil, &st) != nil {
			return nil
		}
		pending := false
		for _, s := range st.Sessions {
			for _, p := range s.Panes {
				if a := p.AgentSession; a != nil && a.Agent == agent && a.ID == session {
					fmt.Println(hookContext(c, p.ID, s.ID, true))
					return nil
				}
				pending = pending || p.Agent == agent && (p.AgentSession == nil || p.AgentSession.ID != session)
			}
		}
		if !wait || !pending || time.Now().After(deadline) {
			return nil
		}
	}
}

// hookContext is what the hook tells an agent in pane, of session sid;
// c is kerod, nil when it is unreachable. away tells an agent whose
// commands don't run in its pane, and don't have its variables.
func hookContext(c *client.Client, pane, sid string, away bool) string {
	var b strings.Builder
	if away {
		fmt.Fprintf(&b, "You are running in Kero, a terminal for coding agents: session %s, pane %s. "+
			"The `kero` command (see the kero skill) runs things beside you, such as a dev server, tests or another agent. "+
			"Your commands don't run in your pane and don't have its $KERO_SESSION and $KERO_PANE: give kero your pane as the target, "+
			"as in `kero split %s --down -- npm run dev`.", sid, pane, pane)
	} else {
		fmt.Fprintf(&b, "You are running in Kero, a terminal for coding agents: session %s, pane %s ($KERO_SESSION and $KERO_PANE). "+
			"The `kero` command (see the kero skill) runs things beside you, such as a dev server, tests or another agent, "+
			"and tells the user what you are doing.", sid, pane)
	}
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
		case s.AgentNamed:
			fmt.Fprintf(&b, " This conversation named the session %q: leave its name alone.", s.Name)
		case s.Name != "":
			fmt.Fprintf(&b, " The user knows this session as %q: leave its name alone.", s.Name)
		}
		// Its part in an orchestrator's tree, which a resumed conversation,
		// or one started anew in the session, keeps to.
		switch {
		case s.Orchestrator:
			fmt.Fprintf(&b, " This session is an orchestrator: plan and delegate, starting sessions for the work with kero new, "+
				"which become your children; Kero types their reports into your conversation. Goal: %s", orDash(s.Goal))
		case s.Parent != "":
			fmt.Fprintf(&b, " This session is a child of orchestrator session %s. Kero sends it the last reply of each of your turns, "+
				"and tells it when you wait for input or fail: end a turn with what the orchestrator needs, your result or your question. "+
				"Once your task is done, send it a short summary of the result with `kero report --to-parent \"<summary>\"`.", s.Parent)
		}
	}
	return b.String()
}

// integration is what kero installs in an agent, beside its skill, to
// hook it into Kero: a hook in its settings, or a plugin of its own.
type integration interface {
	name() string // the agent, as `kero hook` takes it
	path() string // where it goes
	state() string
	install() error
	remove() error
}

// agentHook is a hook kero installs in an agent's settings: a SessionStart
// command running `kero hook <agent>`.
type agentHook struct {
	agent string // as `kero hook` takes it: "claude", "codex"
	file  string // the JSON file holding the agent's hooks
}

func (h agentHook) name() string { return h.agent }
func (h agentHook) path() string { return h.file }

// kero is the kero command of Kero's own directory (package clilink),
// which stays where it is as Kero updates.
func kero() string {
	exe := filepath.Join(proto.Dir(), "bin", "kero")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	return exe
}

// command is the hook's command line: the kero command of Kero's own
// directory (package clilink), which stays where it is as Kero updates.
func (h agentHook) command() string {
	if runtime.GOOS == "windows" {
		if h.agent == "codex" {
			return anyShell(kero()) + " hook " + h.agent
		}
		return `"` + kero() + `" hook ` + h.agent
	}
	return "'" + strings.ReplaceAll(kero(), "'", `'\''`) + "' hook " + h.agent
}

// anyShell is a program's path on Windows as a command that each shell
// Codex may run a hook in takes alike: Codex runs it in the shell of the
// user's session, cmd, Windows PowerShell, PowerShell or Git Bash, and
// in cmd without one. PowerShell takes a quoted path for a string, and
// cmd no quote but a double one, so a plain path goes bare, with forward
// slashes, which Git Bash leaves alone and the others run as well. A
// path with a space, or another character a shell reads, goes to cmd in
// double quotes: all but Git Bash take that.
func anyShell(path string) string {
	for _, r := range path {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(`_-.:\/`, r) {
			return `cmd /d /c "` + path + `"`
		}
	}
	return filepath.ToSlash(path)
}

// ours tells kero's hook commands, of this kero or another's.
func (h agentHook) ours(command string) bool {
	return strings.Contains(command, "kero") && strings.HasSuffix(command, " hook "+h.agent)
}

// hookEvent is an agent's event kero hooks into: SessionStart, to tell the
// agent of Kero and kerod of the agent's session, and the end of a turn,
// for orchestrators' reports. Async ones never hold the agent up.
type hookEvent struct {
	name    string
	async   bool
	timeout int // seconds
}

// events are the agent's events kero hooks into.
func (h agentHook) events() []hookEvent {
	start := hookEvent{name: "SessionStart", timeout: 10}
	switch h.agent {
	case "claude":
		return []hookEvent{start, {name: "Stop", async: true, timeout: 10}, {name: "StopFailure", async: true, timeout: 10}}
	case "codex":
		return []hookEvent{start, {name: "Stop", async: true, timeout: 10}, {name: "Interrupt", async: true, timeout: 3}}
	}
	return []hookEvent{start}
}

// event is the agent's event kero hooks into named name.
func (h agentHook) event(name string) (hookEvent, bool) {
	events := h.events()
	i := slices.IndexFunc(events, func(e hookEvent) bool { return e.name == name })
	if i < 0 {
		return hookEvent{}, false
	}
	return events[i], true
}

// state is "current", "outdated" (kero's hooks, of another command line,
// or not on each of its events) or "missing".
func (h agentHook) state() string {
	b, err := os.ReadFile(h.file)
	if err != nil {
		return "missing"
	}
	settings, err := parseObject(b)
	if err != nil {
		return "missing"
	}
	ours := h.commands(settings)
	if len(ours) == 0 {
		return "missing"
	}
	for _, e := range h.events() {
		if !slices.Contains(ours[e.name], h.command()) {
			return "outdated"
		}
	}
	return "current"
}

// commands are the commands of kero's hooks in settings, by event.
func (h agentHook) commands(settings object) map[string][]string {
	out := map[string][]string{}
	hooks, _ := parseObject(settings.get("hooks"))
	for _, e := range hooks {
		var groups []object
		json.Unmarshal(e.Val, &groups)
		for _, g := range groups {
			var handlers []struct{ Type, Command string }
			json.Unmarshal(g.get("hooks"), &handlers)
			for _, hd := range handlers {
				if hd.Type == "command" && h.ours(hd.Command) {
					out[e.Key] = append(out[e.Key], hd.Command)
				}
			}
		}
	}
	return out
}

// install puts the hook in the agent's settings, in place of kero's
// others, keeping the rest of the file as it is.
func (h agentHook) install() error {
	linkCLI()
	return h.edit(true)
}

// linkCLI makes the kero command that hooks and plugins run, should kerod
// not have made it yet.
func linkCLI() {
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		bin := filepath.Join(proto.Dir(), "bin")
		if clilink.Target(bin) == "" {
			clilink.Link(exe, bin)
		}
	}
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
	// Already there, once on each event: nothing to do. Else drop kero's
	// handlers, and the groups they leave empty, and add them anew.
	ours := h.commands(settings)
	if !add && len(ours) == 0 {
		return nil
	}
	if add && len(ours) == len(h.events()) && !slices.ContainsFunc(h.events(), func(e hookEvent) bool {
		return !slices.Equal(ours[e.name], []string{h.command()})
	}) {
		return nil
	}
	// The events kero has hooks on, and those it hooks into.
	var events []string
	for _, m := range hooks {
		if _, ok := h.event(m.Key); ok || len(ours[m.Key]) > 0 {
			events = append(events, m.Key)
		}
	}
	for _, e := range h.events() {
		if !slices.Contains(events, e.name) {
			events = append(events, e.name)
		}
	}
	for _, event := range events {
		var groups []object
		if raw := hooks.get(event); raw != nil {
			if err := json.Unmarshal(raw, &groups); err != nil {
				return fmt.Errorf("%s: hooks.%s: %w", proto.ShortDir(h.file), event, err)
			}
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
		if e, ok := h.event(event); add && ok {
			handler := object{{"type", marshal("command")}, {"command", marshal(h.command())}}
			if e.async {
				handler = append(handler, member{"async", marshal(true)})
			}
			handler = append(handler, member{"timeout", marshal(e.timeout)})
			groups = append(groups, object{{"hooks", marshal([]object{handler})}})
		}
		switch {
		case len(groups) > 0:
			hooks = hooks.set(event, marshal(groups))
		case hooks.get(event) != nil:
			hooks = hooks.del(event)
		}
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

// agentPlugin is a plugin kero installs for an agent: files of its own,
// in a directory the agent loads plugins from.
type agentPlugin struct {
	agent string
	dir   string            // where the agent loads it from
	files map[string]string // by name in dir: the asset (package agents)
	own   bool              // dir is the plugin's alone, to remove with it
}

//go:embed agents
var agentAssets embed.FS

func (p agentPlugin) name() string { return p.agent }

func (p agentPlugin) path() string {
	if p.own || len(p.files) != 1 {
		return p.dir
	}
	for name := range p.files {
		return filepath.Join(p.dir, name)
	}
	return p.dir
}

// content is a file as kero writes it: the asset, with the kero command
// it runs.
func (p agentPlugin) content(asset string) []byte {
	b, _ := agentAssets.ReadFile(asset)
	b = bytes.ReplaceAll(b, []byte("__KERO__"), marshal(kero()))
	return bytes.ReplaceAll(b, []byte("__AGENT__"), marshal(p.agent))
}

func (p agentPlugin) state() string {
	same, there := 0, 0
	for name, asset := range p.files {
		b, err := os.ReadFile(filepath.Join(p.dir, name))
		if err != nil {
			continue
		}
		there++
		if bytes.Equal(b, p.content(asset)) {
			same++
		}
	}
	switch {
	case same == len(p.files):
		return "current"
	case there == 0:
		return "missing"
	}
	return "outdated"
}

// ours tells a plugin written to run this Kero's kero command, of this
// kero or an older one: not another Kero's, as a development build's.
func (p agentPlugin) ours() bool {
	for name := range p.files {
		if b, err := os.ReadFile(filepath.Join(p.dir, name)); err == nil && bytes.Contains(b, marshal(kero())) {
			return true
		}
	}
	return false
}

func (p agentPlugin) install() error {
	linkCLI()
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	for name, asset := range p.files {
		if err := writeFileAtomic(filepath.Join(p.dir, name), p.content(asset)); err != nil {
			return err
		}
	}
	return nil
}

func (p agentPlugin) remove() error {
	for name := range p.files {
		if err := os.Remove(filepath.Join(p.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if p.own {
		os.Remove(p.dir) // if nothing else is in it
	}
	return nil
}

// agentHooks returns what kero installs to hook agents into Kero: for
// Claude Code, and for Codex, Pi, OMP and OpenCode when they are installed
// (or named).
func agentHooks(agent string) []integration {
	home := proto.Home()
	dirOf := func(env string, def ...string) string {
		if d := os.Getenv(env); d != "" {
			return d
		}
		return filepath.Join(append([]string{home}, def...)...)
	}
	claude := dirOf("CLAUDE_CONFIG_DIR", ".claude")
	codex := dirOf("CODEX_HOME", ".codex")
	pi := dirOf("PI_CODING_AGENT_DIR", ".pi", "agent")
	// OMP keeps its config under PI_CONFIG_DIR (default ~/.omp), with the
	// agent directory in agent/ there. It honors PI_CODING_AGENT_DIR at
	// runtime too, but that names Pi's directory as well, so install next
	// to the config root rather than risk both extensions in one file.
	ompRoot := os.Getenv("PI_CONFIG_DIR")
	if ompRoot == "" {
		ompRoot = filepath.Join(home, ".omp")
	}
	omp := filepath.Join(ompRoot, "agent")
	opencode := filepath.Join(dirOf("XDG_CONFIG_HOME", ".config"), "opencode")
	installed := func(dir string) bool {
		st, err := os.Stat(dir)
		return err == nil && st.IsDir()
	}
	all := []struct {
		agent, dir string
		it         integration
	}{
		{"claude", claude, agentHook{"claude", filepath.Join(claude, "settings.json")}},
		{"codex", codex, agentHook{"codex", filepath.Join(codex, "hooks.json")}},
		{"pi", pi, agentPlugin{agent: "pi", dir: filepath.Join(pi, "extensions"), files: map[string]string{"kero.ts": "agents/pi.ts"}}},
		{"omp", omp, agentPlugin{agent: "omp", dir: filepath.Join(omp, "extensions"), files: map[string]string{"kero.ts": "agents/pi.ts"}}},
		{"opencode", opencode, agentPlugin{agent: "opencode", dir: filepath.Join(opencode, "plugins", "kero"), own: true, files: map[string]string{
			"package.json": "agents/opencode/package.json", "index.js": "agents/opencode/index.js", "tui.js": "agents/opencode/tui.js",
		}}},
	}
	var out []integration
	for _, a := range all {
		switch {
		case agent == a.agent,
			a.agent == "claude" && agent == "",
			a.agent != "claude" && (agent == "" || agent == "agents") && installed(a.dir):
			out = append(out, a.it)
		}
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
