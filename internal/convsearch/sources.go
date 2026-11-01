package convsearch

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Sources are where the agents that Kero resumes keep their
// conversations, under home: Claude Code's, Codex's and Pi's. OpenCode
// keeps its own in a SQLite database, which Kero has no reader of.
func Sources(home string) []Source {
	claude := os.Getenv("CLAUDE_CONFIG_DIR")
	if claude == "" {
		claude = filepath.Join(home, ".claude")
	}
	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(home, ".codex")
	}
	return []Source{
		{Agent: "claude", List: func() ([]string, error) { return claudeFiles(filepath.Join(claude, "projects")) }, Parse: parseClaude},
		{Agent: "codex", List: func() ([]string, error) { return walkJSONL(filepath.Join(codex, "sessions")) }, Parse: parseCodex},
		{Agent: "pi", List: func() ([]string, error) { return walkJSONL(piDir(home)) }, Parse: parsePi},
	}
}

// claudeFiles are Claude Code's transcripts, one for each conversation,
// in the folders of the projects they ran in. Its subagents' are apart,
// as agent-*.jsonl, or in a folder of their conversation's.
func claudeFiles(dir string) ([]string, error) {
	projects, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, p.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if n := f.Name(); !f.IsDir() && strings.HasSuffix(n, ".jsonl") && !strings.HasPrefix(n, "agent-") {
				out = append(out, filepath.Join(dir, p.Name(), n))
			}
		}
	}
	return out, nil
}

// walkJSONL is every .jsonl file under dir; none when there is no dir.
func walkJSONL(dir string) ([]string, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var out []string
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			return nil
		}
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

// piDir is where Pi keeps its sessions: as its environment says, else
// its settings, else in its agent folder.
func piDir(home string) string {
	if d := os.Getenv("PI_CODING_AGENT_SESSION_DIR"); d != "" {
		return expandHome(d, home)
	}
	agent := os.Getenv("PI_CODING_AGENT_DIR")
	if agent == "" {
		agent = filepath.Join(home, ".pi", "agent")
	}
	agent = expandHome(agent, home)
	if b, err := os.ReadFile(filepath.Join(agent, "settings.json")); err == nil {
		var s struct {
			SessionDir string `json:"sessionDir"`
		}
		if json.Unmarshal(b, &s) == nil && s.SessionDir != "" {
			return expandHome(s.SessionDir, home)
		}
	}
	return filepath.Join(agent, "sessions")
}

func expandHome(p, home string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}

// maxText is how much of a conversation's messages the index keeps, the
// first: more than any conversation holds of what was said, but one an
// agent filled with what it was fed.
const maxText = 16 << 20

// builder gathers a conversation's messages into a Doc.
type builder struct {
	doc   Doc
	text  strings.Builder
	first string // the user's first message, for a title
}

func (b *builder) add(user bool, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if user {
		b.doc.Turns++
		if b.first == "" {
			b.first = text
		}
	}
	if b.text.Len() >= maxText {
		return
	}
	role := byte(roleAgent)
	if user {
		role = roleUser
	}
	b.text.WriteByte(sep)
	b.text.WriteByte(role)
	// Neither mark may come in what was said, which the index splits at.
	b.text.WriteString(strings.Map(func(r rune) rune {
		if r == sep {
			return ' '
		}
		return r
	}, text))
}

func (b *builder) at(ts string) {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return
	}
	ms := t.UnixMilli()
	if b.doc.Started == 0 || ms < b.doc.Started {
		b.doc.Started = ms
	}
	b.doc.Updated = max(b.doc.Updated, ms)
}

// done is the Doc, none when the user said nothing in it, titled title,
// else by its first message.
func (b *builder) done(title, path string) []Doc {
	if b.doc.Turns == 0 || b.doc.ID == "" {
		return nil
	}
	b.doc.Text = b.text.String()
	b.doc.Title = strings.TrimSpace(title)
	if b.doc.Title == "" {
		b.doc.Title = titleOf(b.first)
	}
	if b.doc.Updated == 0 {
		if st, err := os.Stat(path); err == nil {
			b.doc.Updated = st.ModTime().UnixMilli()
		}
	}
	if b.doc.Started == 0 {
		b.doc.Started = b.doc.Updated
	}
	return []Doc{b.doc}
}

// titleOf is a message as a title: its first line, to 100 bytes, at a
// word.
func titleOf(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	s = strings.TrimSpace(s)
	if len(s) <= 100 {
		return s
	}
	cut := 100
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if i := strings.LastIndexByte(s[:cut], ' '); i > 60 {
		cut = i
	}
	return s[:cut] + "…"
}

// lines calls fn with each line of a history, which may be long: a tool's
// output, or a file an agent wrote, is in one.
func lines(path string, fn func(line []byte) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			// Longer than the buffer: the rest of it, too.
			long := append([]byte(nil), line...)
			for err == bufio.ErrBufferFull {
				line, err = r.ReadSlice('\n')
				long = append(long, line...)
			}
			line = long
		}
		if len(line) > 0 && !fn(line) {
			return nil
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// content is a message's text: a string, or the text of its blocks of
// the kinds asked for.
func content(raw json.RawMessage, kinds ...string) []string {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return []string{s}
		}
		return nil
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		for _, k := range kinds {
			if b.Type == k && b.Text != "" {
				out = append(out, b.Text)
			}
		}
	}
	return out
}

// injected tells what an agent's harness wrote as the user's, not the
// user: slash commands and their output, reminders, instructions.
func injected(s string) bool {
	s = strings.TrimSpace(s)
	for _, p := range []string{
		"<command-", "<local-command-", "<bash-", "<task-notification", "<system-reminder", "Caveat: ", "[Request interrupted",
		"<environment_context>", "<user_instructions>", "<permissions", "<INSTRUCTIONS>", "# AGENTS.md instructions",
		"<user_shell_command>", "<turn_aborted>", "<subagent_notification>",
	} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// slashCommand is what the user typed for a slash command, which Claude
// Code writes as markup: its name and arguments, as /review the push fix.
// Claude Code's own commands it writes with the name first, and those
// without arguments, as /clear and /model, tell what was done, not what
// was said: none.
func slashCommand(s string) (string, bool) {
	s = strings.TrimSpace(s)
	_, name, ok := strings.Cut(s, "<command-name>")
	if !ok || !strings.HasPrefix(s, "<command-") {
		return "", false
	}
	name, _, _ = strings.Cut(name, "</command-name>")
	name = strings.TrimSpace(name)
	_, args, _ := strings.Cut(s, "<command-args>")
	args, _, _ = strings.Cut(args, "</command-args>")
	args = strings.TrimSpace(args)
	if name == "" || args == "" && strings.HasPrefix(s, "<command-name>") {
		return "", false
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	if args == "" {
		return name, true
	}
	return name + " " + args, true
}

// parseClaude reads a Claude Code transcript: its user's and Claude's
// messages, not the tools', nor what Claude Code adds, and its title, the
// user's (/rename) over Claude's. A conversation run by a program, as
// claude -p runs one, is none to look for.
func parseClaude(path string) ([]Doc, error) {
	b := &builder{}
	b.doc.Agent = "claude"
	b.doc.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	custom, ai := "", ""
	program := false
	err := lines(path, func(line []byte) bool {
		user := bytes.Contains(line, []byte(`"type":"user"`))
		if !user && !bytes.Contains(line, []byte(`"type":"assistant"`)) && !bytes.Contains(line, []byte(`-title"`)) {
			return true
		}
		// A tool's result, which comes as the user's.
		if user && bytes.Contains(line, []byte(`"type":"tool_result"`)) {
			return true
		}
		var e struct {
			Type             string `json:"type"`
			IsMeta           bool   `json:"isMeta"`
			IsSidechain      bool   `json:"isSidechain"`
			IsCompactSummary bool   `json:"isCompactSummary"`
			Cwd              string `json:"cwd"`
			Timestamp        string `json:"timestamp"`
			Entrypoint       string `json:"entrypoint"`
			CustomTitle      string `json:"customTitle"`
			AITitle          string `json:"aiTitle"`
			Message          struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		switch e.Type {
		case "custom-title":
			custom = e.CustomTitle
			return true
		case "ai-title":
			ai = e.AITitle
			return true
		case "user", "assistant":
		default:
			return true
		}
		if strings.HasPrefix(e.Entrypoint, "sdk") {
			program = true
			return false
		}
		if e.IsSidechain || e.IsMeta || e.IsCompactSummary {
			return true
		}
		if b.doc.Dir == "" {
			b.doc.Dir = e.Cwd
		}
		b.at(e.Timestamp)
		for _, t := range content(e.Message.Content, "text") {
			if e.Type == "user" {
				if c, ok := slashCommand(t); ok {
					t = c
				} else if injected(t) {
					continue
				}
			}
			b.add(e.Type == "user", t)
		}
		return true
	})
	if err != nil || program {
		return nil, err
	}
	if custom == "" {
		custom = ai
	}
	return b.done(titleLine(custom), path), nil
}

// parseCodex reads a Codex rollout: what the user wrote, as Codex tells
// it apart from the instructions it sends as the user's, and the agent's
// answers. A conversation that codex exec or a subagent ran is none to
// look for.
func parseCodex(path string) ([]Doc, error) {
	b := &builder{}
	b.doc.Agent = "codex"
	type msg struct {
		user, event bool
		text        string
	}
	var msgs []msg
	meta, skip := false, false
	err := lines(path, func(line []byte) bool {
		var kind string
		switch {
		case bytes.Contains(line, []byte(`"type":"session_meta"`)):
			kind = "meta"
		case bytes.Contains(line, []byte(`"type":"event_msg"`)) &&
			(bytes.Contains(line, []byte(`"type":"user_message"`)) || bytes.Contains(line, []byte(`"type":"UserMessage"`))):
			kind = "event"
		case bytes.Contains(line, []byte(`"type":"response_item"`)) &&
			(bytes.Contains(line, []byte(`"role":"assistant"`)) || bytes.Contains(line, []byte(`"role":"user"`))):
			kind = "item"
		default:
			return true
		}
		var e struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Payload   struct {
				Type    string          `json:"type"`
				ID      string          `json:"id"`
				Cwd     string          `json:"cwd"`
				Source  json.RawMessage `json:"source"`
				Role    string          `json:"role"`
				Message string          `json:"message"`
				Content json.RawMessage `json:"content"`
				Item    struct {
					Type    string          `json:"type"`
					Content json.RawMessage `json:"content"`
				} `json:"item"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		p := &e.Payload
		switch kind {
		case "meta":
			// The first is the rollout's own; a fork's next is its parent's.
			if meta || e.Type != "session_meta" {
				return true
			}
			meta = true
			// A subagent's source is an object; an older Codex told none.
			var source string
			if len(p.Source) > 0 && (json.Unmarshal(p.Source, &source) != nil || source == "exec" || source == "mcp") {
				skip = true
				return false
			}
			b.doc.ID, b.doc.Dir = p.ID, p.Cwd
			b.at(e.Timestamp)
			return true
		case "event":
			if e.Type != "event_msg" {
				return true
			}
			switch {
			case p.Type == "user_message":
				msgs = append(msgs, msg{true, true, p.Message})
			case p.Type == "item_completed" && p.Item.Type == "UserMessage":
				for _, t := range content(p.Item.Content, "text") {
					msgs = append(msgs, msg{true, true, t})
				}
			default:
				return true
			}
		case "item":
			if e.Type != "response_item" || p.Type != "message" {
				return true
			}
			switch p.Role {
			case "assistant":
				for _, t := range content(p.Content, "output_text") {
					msgs = append(msgs, msg{text: t})
				}
			case "user":
				for _, t := range content(p.Content, "input_text") {
					msgs = append(msgs, msg{user: true, text: t})
				}
			}
		}
		b.at(e.Timestamp)
		return true
	})
	if err != nil || skip {
		return nil, err
	}
	// What the user wrote, as its events tell, else as sent, but what
	// Codex sends as the user's.
	events := false
	for _, m := range msgs {
		events = events || m.event
	}
	for _, m := range msgs {
		if m.user && (m.event != events || injected(m.text)) {
			continue
		}
		b.add(m.user, m.text)
	}
	return b.done("", path), nil
}

// parsePi reads a Pi session: the user's and the agent's messages, those
// of extensions it shows, and its summaries; its name for a title.
func parsePi(path string) ([]Doc, error) {
	b := &builder{}
	b.doc.Agent = "pi"
	name := ""
	err := lines(path, func(line []byte) bool {
		var e struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Cwd       string          `json:"cwd"`
			Timestamp string          `json:"timestamp"`
			Name      string          `json:"name"`
			Display   bool            `json:"display"`
			Content   json.RawMessage `json:"content"`
			Summary   string          `json:"summary"`
			Message   struct {
				Role      string          `json:"role"`
				Display   bool            `json:"display"`
				Content   json.RawMessage `json:"content"`
				Timestamp json.RawMessage `json:"timestamp"` // unix ms
			} `json:"message"`
		}
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		switch e.Type {
		case "session":
			b.doc.ID, b.doc.Dir = e.ID, e.Cwd
			b.at(e.Timestamp)
		case "session_info":
			if e.Name != "" {
				name = e.Name
			}
		case "message":
			m := &e.Message
			switch {
			case m.Role == "user", m.Role == "assistant":
			case (m.Role == "custom" || m.Role == "hookMessage") && m.Display:
			default:
				return true
			}
			if ms, err := strconv.ParseInt(string(m.Timestamp), 10, 64); err == nil && ms > 0 {
				b.doc.Updated = max(b.doc.Updated, ms)
			} else {
				b.at(e.Timestamp)
			}
			for _, t := range content(m.Content, "text") {
				b.add(m.Role == "user", t)
			}
		case "custom_message":
			if e.Display {
				for _, t := range content(e.Content, "text") {
					b.add(false, t)
				}
			}
		case "compaction", "branch_summary":
			b.add(false, e.Summary)
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return b.done(titleLine(name), path), nil
}

// titleLine keeps a title to its first line.
func titleLine(t string) string {
	t, _, _ = strings.Cut(strings.TrimSpace(t), "\n")
	return strings.TrimSpace(t)
}
