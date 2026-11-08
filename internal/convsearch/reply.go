package convsearch

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Reply is what an agent said last in a conversation: the text it ended
// its last turn with, after its last tool call, as an orchestrator reads
// it of a child.
type Reply struct {
	Text string
	// At is where in the transcript the reply ends, which tells one reply
	// from the next, as two that say the same; a transcript only grows.
	At int64
}

// ErrNoTranscript is LastReply's for a conversation whose transcript is
// not there, or of an agent whose transcripts Kero has no reader of.
var ErrNoTranscript = errors.New("no transcript")

// replyIDRe is what an agent's session id is made of: no pattern's
// letters, as it goes into a glob.
var replyIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// replyStep is what an entry of a transcript is to its last reply, read
// from its end: text of it, the turn's start or a tool call it comes
// after, which ends the reply, or neither.
type replyStep int

const (
	stepSkip replyStep = iota
	stepText
	stepStop
)

// replyReaders read the entries of each agent's transcripts whose last
// replies Kero reads: what an entry is, and its text.
var replyReaders = map[string]func(line []byte) (replyStep, []string){
	"claude": claudeStep,
	"codex":  codexStep,
	"pi":     piStep,
}

// LastReply is the last reply in conversation id of agent, of the
// agents whose transcripts Kero reads (Sources), under home: an empty
// Text when its last turn ended with none, as a turn that was
// interrupted. A conversation's file is found anew each time: the
// folders hold one file for each, which a glob finds at once.
func LastReply(home, agent, id string) (Reply, error) {
	step := replyReaders[agent]
	if step == nil || !replyIDRe.MatchString(id) {
		return Reply{}, ErrNoTranscript
	}
	path := transcriptPath(home, agent, id)
	if path == "" {
		return Reply{}, ErrNoTranscript
	}
	return lastReply(path, step)
}

// TranscriptSize is how long conversation id's transcript is now, -1 when
// Kero has none of it: where what comes next in it starts.
func TranscriptSize(home, agent, id string) int64 {
	if replyReaders[agent] == nil || !replyIDRe.MatchString(id) {
		return -1
	}
	path := transcriptPath(home, agent, id)
	if path == "" {
		return -1
	}
	st, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return st.Size()
}

// TurnsAfter counts what started or cut a turn in conversation id's
// transcript after off: the user's messages, tool calls and their
// results. A child's report, sent by a tool call, has its own result
// after it, one, when nothing came after it.
func TurnsAfter(home, agent, id string, off int64) (int, error) {
	step := replyReaders[agent]
	if step == nil || !replyIDRe.MatchString(id) {
		return 0, ErrNoTranscript
	}
	path := transcriptPath(home, agent, id)
	if path == "" {
		return 0, ErrNoTranscript
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	n := 0
	for line := range bytes.SplitSeq(b, []byte("\n")) {
		if line = bytes.TrimSpace(line); len(line) > 0 {
			if kind, _ := step(line); kind == stepStop {
				n++
			}
		}
	}
	return n, nil
}

// transcriptPath is the file of conversation id of agent, "" for none.
func transcriptPath(home, agent, id string) string {
	var patterns []string
	switch agent {
	case "claude":
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			dir = filepath.Join(home, ".claude")
		}
		patterns = []string{filepath.Join(dir, "projects", "*", id+".jsonl")}
	case "codex":
		dir := os.Getenv("CODEX_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".codex")
		}
		patterns = []string{filepath.Join(dir, "sessions", "*", "*", "*", "rollout-*"+id+".jsonl")}
	case "pi":
		dir := piDir(home)
		patterns = []string{filepath.Join(dir, "*", "*"+id+".jsonl"), filepath.Join(dir, "*"+id+".jsonl")}
	}
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && st.Mode().IsRegular() {
				return m
			}
		}
	}
	return ""
}

// lastReply reads path from its end, a window at a time that grows till
// it holds the last turn's end: a transcript can be large, and the reply
// is at its end.
func lastReply(path string, step func([]byte) (replyStep, []string)) (Reply, error) {
	f, err := os.Open(path)
	if err != nil {
		return Reply{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Reply{}, err
	}
	size := st.Size()
	for window := int64(1 << 20); ; window *= 4 {
		off := max(0, size-window)
		buf := make([]byte, size-off)
		if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
			return Reply{}, err
		}
		if off > 0 {
			// The window's first line is cut: the next window has it.
			i := bytes.IndexByte(buf, '\n')
			if i < 0 {
				continue
			}
			buf, off = buf[i+1:], off+int64(i+1)
		}
		r, done := scanReply(buf, off, step)
		if done || off == 0 {
			return r, nil
		}
	}
}

// scanReply reads the lines of buf, at off in its file, from the last:
// the reply is the text of the entries that come after the last turn's
// start or tool call. done tells that buf held where it starts.
func scanReply(buf []byte, off int64, step func([]byte) (replyStep, []string)) (r Reply, done bool) {
	var parts []string
	end := len(buf)
	for end > 0 {
		start := bytes.LastIndexByte(buf[:end-1], '\n') + 1
		line := bytes.TrimSpace(buf[start:end])
		at := off + int64(end)
		end = start
		if len(line) == 0 {
			continue
		}
		kind, texts := step(line)
		if len(texts) > 0 {
			if r.At == 0 {
				r.At = at
			}
			parts = append(texts, parts...)
		}
		if kind == stepStop {
			done = true
			break
		}
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	parts = slices.DeleteFunc(parts, func(s string) bool { return s == "" })
	r.Text = strings.Join(parts, "\n\n")
	return r, done
}

// blocksAfterTool is the text of a message's blocks after its last
// block of a tool kind, and whether it has one: an entry may hold both.
func blocksAfterTool(raw json.RawMessage, text string, tools ...string) ([]string, bool) {
	if len(raw) == 0 || raw[0] == '"' {
		return content(raw, text), false
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return nil, false
	}
	var out []string
	tool := false
	for _, b := range blocks {
		switch {
		case slices.Contains(tools, b.Type):
			out, tool = nil, true
		case b.Type == text && b.Text != "":
			out = append(out, b.Text)
		}
	}
	return out, tool
}

// claudeStep reads an entry of Claude Code's: its own messages, not a
// subagent's; the user's any message, a tool's result, a command or its
// output, starts or cuts a turn; the summary that compacting writes, and
// what Claude Code adds unseen, are neither.
func claudeStep(line []byte) (replyStep, []string) {
	user := bytes.Contains(line, []byte(`"type":"user"`))
	if !user && !bytes.Contains(line, []byte(`"type":"assistant"`)) {
		return stepSkip, nil
	}
	var e struct {
		Type             string `json:"type"`
		IsMeta           bool   `json:"isMeta"`
		IsSidechain      bool   `json:"isSidechain"`
		IsCompactSummary bool   `json:"isCompactSummary"`
		Message          struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &e) != nil || e.IsSidechain || e.IsMeta || e.IsCompactSummary {
		return stepSkip, nil
	}
	switch e.Type {
	case "user":
		return stepStop, nil
	case "assistant":
		texts, tool := blocksAfterTool(e.Message.Content, "text", "tool_use", "server_tool_use")
		if tool {
			return stepStop, texts
		}
		return stepText, texts
	}
	return stepSkip, nil
}

// codexStep reads an item of a Codex rollout: the agent's messages; a
// tool call, its output or any message of the user's role, as Codex
// sends its instructions, starts or cuts a turn. Its events repeat what
// its items hold.
func codexStep(line []byte) (replyStep, []string) {
	if !bytes.Contains(line, []byte(`"type":"response_item"`)) {
		return stepSkip, nil
	}
	var e struct {
		Type    string `json:"type"`
		Payload struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &e) != nil || e.Type != "response_item" {
		return stepSkip, nil
	}
	p := &e.Payload
	switch {
	case p.Type == "message" && p.Role == "assistant":
		return stepText, content(p.Content, "output_text")
	case p.Type == "message" && p.Role == "user":
		return stepStop, nil
	case p.Type == "reasoning", p.Type == "message":
		return stepSkip, nil
	}
	// A call of a tool of any kind, or its output.
	return stepStop, nil
}

// piStep reads an entry of a Pi session: the agent's messages; the
// user's, a tool's result or a command's starts or cuts a turn; its
// summaries are neither.
func piStep(line []byte) (replyStep, []string) {
	var e struct {
		Type    string `json:"type"`
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &e) != nil || e.Type != "message" {
		return stepSkip, nil
	}
	switch e.Message.Role {
	case "assistant":
		texts, tool := blocksAfterTool(e.Message.Content, "text", "toolCall")
		if tool {
			return stepStop, texts
		}
		return stepText, texts
	case "user", "toolResult", "bashExecution":
		return stepStop, nil
	}
	return stepSkip, nil
}
