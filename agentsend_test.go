package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

// Send to Session… sends the text selected in a terminal to the agent
// chosen of the host's, as a fenced block labelled with the pane; closed
// unchosen, it sends nothing.
func TestSendExcerpt(t *testing.T) {
	a := testApp(t)
	sent := make(chan proto.AgentSendParams, 4)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method == "agent.send" {
			var p proto.AgentSendParams
			json.Unmarshal(raw, &p)
			sent <- p
			return proto.AgentSendResult{ID: 7, To: p.To, State: "queued"}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().cl = cl
	// A shell at its prompt runs no process of its own: its shell names it.
	a.st.Sessions[0].Panes[1].Agent = "claude"
	a.st.Sessions = append(a.st.Sessions, proto.Session{ID: "s2", Project: "p1", Name: "other",
		Panes: []proto.Pane{{ID: "c", Dir: "/tmp", Agent: "codex", Status: proto.Working, Progress: -1}}, Focus: "c"})
	tt := ui.NewTester(a.view, 1100, 640)

	a.sendExcerpt("a", "FAIL TestX\n  want ```a```\n")
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("other"); !ok || !a.pal.open {
		t.Fatalf("no choice of agents; texts %q", tt.Texts())
	}
	shot(t, tt, "send-to-session")
	// Only the agents: the shell of pane a is no choice.
	if items := a.agentItems(); len(items) != 2 || items[0].session.ID != "s1" {
		t.Fatalf("offered %d agents, first %+v", len(items), items)
	}
	// Down to the other session's, and Enter.
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	var p proto.AgentSendParams
	select {
	case p = <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("nothing sent")
	}
	if p.To != "c" || p.From != "a" || p.Mode != "push" {
		t.Errorf("sent %+v", p)
	}
	if want := "````\nFAIL TestX\n  want ```a```\n````"; p.Text != want {
		t.Errorf("sent the text as %q, not %q", p.Text, want)
	}
	if p.Label != `the terminal pane a (fish) of session "demo"` {
		t.Errorf("labelled %q", p.Label)
	}

	// Escape closes the choice, and nothing goes.
	a.sendExcerpt("a", "more")
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.pal.open || a.pal.choose != nil {
		t.Error("the choice stayed open")
	}
	select {
	case p := <-sent:
		t.Errorf("sent %+v unchosen", p)
	case <-time.After(100 * time.Millisecond):
	}
}

// A pane's source names its agent before its program, its program before
// its shell, and the title its program set, unless it only repeats it.
func TestPaneSource(t *testing.T) {
	for _, c := range []struct {
		p    proto.Pane
		want string
	}{
		{proto.Pane{Shell: "pwsh"}, " (pwsh)"},
		{proto.Pane{Shell: "pwsh", Process: "go", Title: "go test ./..."}, ` (go, titled "go test ./...")`},
		{proto.Pane{Shell: "zsh", Process: "claude", Agent: "claude", Title: "✳ Fix login"}, ` (Claude Code, titled "✳ Fix login")`},
		{proto.Pane{Shell: "fish", Title: "FISH"}, " (fish)"},
		{proto.Pane{Title: "build"}, ` (titled "build")`},
		{proto.Pane{}, ""},
	} {
		if got := paneSource(&c.p); got != c.want {
			t.Errorf("%+v: %q, want %q", c.p, got, c.want)
		}
	}
}
