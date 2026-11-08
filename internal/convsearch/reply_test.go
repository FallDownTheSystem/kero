package convsearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastReply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	write := func(path string, lines ...string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	claude := filepath.Join(home, ".claude", "projects", "C--proj", "c1.jsonl")
	write(claude,
		`{"type":"user","message":{"content":"fix the bug"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Looking."},{"type":"tool_use","name":"Read"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"..."}]}}`,
		`{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"a subagent's"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hm"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Fixed it."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Push the branch?"}]}}`,
		`{"type":"system","subtype":"compact_boundary"}`,
		`{"type":"user","isCompactSummary":true,"message":{"content":"Summary of the conversation"}}`,
		`{"type":"user","isMeta":true,"message":{"content":"<system-reminder>x</system-reminder>"}}`,
	)
	r, err := LastReply(home, "claude", "c1")
	if err != nil || r.Text != "Fixed it.\n\nPush the branch?" {
		t.Fatalf("claude: %q, %v", r.Text, err)
	}
	// The compaction after it is no new reply: it ends where it ended.
	before := r.At
	f, _ := os.OpenFile(claude, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"system","subtype":"compact_boundary"}` + "\n")
	f.Close()
	if r, _ := LastReply(home, "claude", "c1"); r.At != before {
		t.Fatalf("a compaction moved the reply: %d, was %d", r.At, before)
	}
	// A new prompt with no reply yet: none.
	f, _ = os.OpenFile(claude, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","message":{"content":"and the tests?"}}` + "\n")
	f.Close()
	if r, _ := LastReply(home, "claude", "c1"); r.Text != "" {
		t.Fatalf("a new prompt kept the old reply: %q", r.Text)
	}

	write(filepath.Join(home, ".codex", "sessions", "2026", "11", "08", "rollout-2026-11-08T10-00-00-x2.jsonl"),
		`{"type":"session_meta","payload":{"id":"x2"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Running it."}]}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"shell"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","output":"ok"}}`,
		`{"type":"response_item","payload":{"type":"reasoning"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"All green."}]}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"All green."}}`,
	)
	if r, err := LastReply(home, "codex", "x2"); err != nil || r.Text != "All green." {
		t.Fatalf("codex: %q, %v", r.Text, err)
	}

	write(filepath.Join(home, ".pi", "agent", "sessions", "--proj--", "2026-11-08_p3.jsonl"),
		`{"type":"session","id":"p3"}`,
		`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","name":"bash"},{"type":"text","text":"Done here."}]}}`,
	)
	if r, err := LastReply(home, "pi", "p3"); err != nil || r.Text != "Done here." {
		t.Fatalf("pi: %q, %v", r.Text, err)
	}

	for _, c := range [][2]string{{"gemini", "c1"}, {"claude", "nope"}, {"claude", "../c1"}} {
		if _, err := LastReply(home, c[0], c[1]); err != ErrNoTranscript {
			t.Fatalf("%s %s: %v, want ErrNoTranscript", c[0], c[1], err)
		}
	}
}

// A reply ends a transcript larger than the first window read from its
// end, whose turn starts before it.
func TestLastReplyLongTurn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	path := filepath.Join(home, ".claude", "projects", "p", "c.jsonl")
	os.MkdirAll(filepath.Dir(path), 0o755)
	big := strings.Repeat("x", 3<<20)
	content := `{"type":"user","message":{"content":"go"}}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"` + big + `"}]}}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"end"}]}}` + "\n"
	os.WriteFile(path, []byte(content), 0o644)
	r, err := LastReply(home, "claude", "c")
	if err != nil || !strings.HasPrefix(r.Text, "xxx") || !strings.HasSuffix(r.Text, "\n\nend") {
		t.Fatalf("%d bytes, %v", len(r.Text), err)
	}
}
