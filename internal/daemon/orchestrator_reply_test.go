package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kero/internal/proto"
)

// A child's idle line carries the reply it ended its turn with; a turn
// with no new reply, as a compaction, is no news; nor is one its report
// told the end of, unless work still runs in the background.
func TestIdleLineReply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, "projects", "p", "c1.jsonl")
	os.MkdirAll(filepath.Dir(path), 0o755)
	write := func(lines ...string) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(strings.Join(lines, "\n") + "\n")
		f.Close()
	}
	pn := &pane{info: proto.Pane{AgentSession: &proto.AgentSession{Agent: "claude", ID: "c1"}}}
	c := childNow{name: "alpha", pane: pn}
	k := &kid{}

	size := func() int64 {
		st, _ := os.Stat(path)
		return st.Size()
	}

	write(`{"type":"user","message":{"content":"go"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Done. Push the branch?"}]}}`)
	got, _ := idleLine("s1", c, k)
	if !strings.Contains(got, `s1 "alpha" finished its turn: it is idle. Its reply:`) || !strings.Contains(got, "\n  Done. Push the branch?") {
		t.Fatalf("idle line:\n%s", got)
	}
	// Compacted: the same reply, which may yet come: look again.
	write(`{"type":"system","subtype":"compact_boundary"}`,
		`{"type":"user","isCompactSummary":true,"message":{"content":"summary"}}`)
	if got, retry := idleLine("s1", c, k); got != "" || !retry {
		t.Fatalf("a compaction was news, or final: %q %v", got, retry)
	}
	// A turn its report told the end of: the report's tool call, its
	// result, the reply.
	write(`{"type":"user","message":{"content":"push it"}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`)
	k.reported, k.reportAt = true, size()
	write(`{"type":"user","message":{"content":[{"type":"tool_result","content":""}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Pushed."}]}}`)
	if got, _ := idleLine("s1", c, k); got != "" {
		t.Fatalf("a reported turn was news:\n%s", got)
	}
	// Its reply was taken note of: the next idle with it is none either.
	k.reported = false
	if got, _ := idleLine("s1", c, k); got != "" {
		t.Fatalf("the reported reply came again:\n%s", got)
	}
	// A report mid-turn, and work after it: the turn's reply goes.
	write(`{"type":"user","message":{"content":"next phase"}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`)
	k.reported, k.reportAt = true, size()
	write(`{"type":"user","message":{"content":[{"type":"tool_result","content":""}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":""}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Which database?"}]}}`)
	if got, _ := idleLine("s1", c, k); !strings.Contains(got, "Which database?") {
		t.Fatalf("a mid-turn report hid the turn's question:\n%s", got)
	}
	// An agent whose transcript Kero cannot read: the turn's end alone.
	other := childNow{name: "beta", pane: &pane{}}
	if got, _ := idleLine("s2", other, &kid{}); got != `- session s2 "beta" finished its turn: it is idle.` {
		t.Fatalf("no transcript: %q", got)
	}
}

func TestClipReply(t *testing.T) {
	short := strings.Repeat("a", 2000)
	if clipReply(short, "s1") != short {
		t.Error("cut a short reply")
	}
	var lines []string
	for i := range 200 {
		lines = append(lines, strings.Repeat("é", 30)+" line "+string(rune('a'+i%26)))
	}
	long := strings.Join(lines, "\n")
	got := clipReply(long, "s1")
	head, tail, ok := strings.Cut(got, "\n[… ")
	if !ok || !strings.Contains(tail, " characters cut: kero read s1 --reply prints the whole reply …]\n") {
		t.Fatalf("no marker:\n%s", got)
	}
	if !strings.HasPrefix(long, head) || !strings.HasSuffix(long, tail[strings.Index(tail, "\n")+1:]) {
		t.Error("the kept start or end is not the reply's")
	}
	if n := len([]rune(got)); n > 2*clipHalf+200 {
		t.Errorf("kept %d characters", n)
	}
}

func TestBackgroundWork(t *testing.T) {
	for screen, want := range map[string]string{
		"✻ Brewed for 12s · 1 shell, 1 monitor still running\n> \n":            "1 shell, 1 monitor",
		"✻ Cooked for 3m · 2 MCP tasks still running\n\n> \n  ? for shortcuts": "2 MCP tasks",
		"● Done.\n> \n": "",
	} {
		if got := backgroundWork(screen); got != want {
			t.Errorf("%q: %q, want %q", screen, got, want)
		}
	}
}
