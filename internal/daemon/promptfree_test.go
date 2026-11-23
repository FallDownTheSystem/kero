package daemon

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// claudePane is a pane of Claude Code, of session conv, as kerod knows it
// once its hook told its session: no program runs in it, as the gate needs
// none, so it runs on any system.
func claudePane(t *testing.T, d *Daemon, conv string) *pane {
	t.Helper()
	pn, err := newPane(d, "s1", proto.Pane{ID: "p1", Cols: 80, Rows: 24, Agent: "claude",
		AgentSession: &proto.AgentSession{Agent: "claude", ID: conv}})
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.panes[pn.info.ID] = pn
	d.sessions = append(d.sessions, &session{id: "s1", panes: []*pane{pn}})
	d.mu.Unlock()
	return pn
}

// declareBackground declares what Claude Code does while a subagent it sent
// to the background runs: its own record working, and the subagent's.
func declareBackground(pn *pane) {
	declare(pn,
		proto.Record{State: "working", App: "claude-code"},
		proto.Record{ID: "a7e89adb798af4104", State: "working", App: "claude-code", Title: "Research"})
}

func hook(t *testing.T, d *Daemon, method string, at time.Time) {
	t.Helper()
	raw, _ := json.Marshal(proto.TurnParams{Agent: "claude", Session: "conv", BySession: true, End: proto.TurnDone, At: at.UnixMilli()})
	if method == "pane.prompted" {
		raw, _ = json.Marshal(proto.TurnParams{Agent: "claude", Session: "conv", BySession: true, At: at.UnixMilli()})
	}
	if _, err := d.call(nil, method, raw); err != nil {
		t.Fatal(err)
	}
}

// Claude Code's turn ended while a subagent of its runs in the background:
// its prompt takes a message, though it stays working for the sidebar and
// kero wait; its next turn, however it starts, takes the prompt again.
func TestPromptFreeWhileBackgroundWorks(t *testing.T) {
	d := testDaemon(t)
	pn := claudePane(t, d, "conv")
	declareBackground(pn)
	if pn.info.Status != proto.Working {
		t.Fatalf("status %s, not working", pn.info.Status)
	}
	if ready(pn) {
		t.Fatal("ready in its turn")
	}

	hook(t, d, "pane.turn", time.Now())
	if !ready(pn) {
		t.Fatal("not ready after its turn ended, its background task still working")
	}
	if pn.info.Status != proto.Working {
		t.Errorf("status %s after the turn's end, not working", pn.info.Status)
	}

	// The user's Enter in the pane sends a prompt.
	pn.input([]byte("go on\r"))
	if ready(pn) {
		t.Error("ready after an Enter")
	}

	hook(t, d, "pane.turn", time.Now())
	hook(t, d, "pane.prompted", time.Now())
	if ready(pn) {
		t.Error("ready after its prompt hook")
	}

	// The background task ends, which Claude Code tells its agent of in a
	// turn that no hook marks the start of.
	hook(t, d, "pane.turn", time.Now())
	pn.mu.Lock()
	pn.applyReport(proto.Record{ID: "a7e89adb798af4104", State: "clear", Progress: -1})
	pn.mu.Unlock()
	if ready(pn) {
		t.Error("ready as a background task ended")
	}

	// A permission asked meanwhile holds messages, free or not.
	hook(t, d, "pane.turn", time.Now())
	declare(pn, proto.Record{State: "blocked", Kind: "permission", App: "claude-code", Msg: "Run rm?"},
		proto.Record{ID: "a7e89adb798af4104", State: "working", App: "claude-code"})
	if ready(pn) {
		t.Error("ready with a permission asked")
	}
}

// An async Stop hook that comes after the next prompt, of the turn before,
// frees no prompt; nor does a status the screen tells, of no records.
func TestPromptFreeOrder(t *testing.T) {
	d := testDaemon(t)
	pn := claudePane(t, d, "conv")
	declareBackground(pn)
	stop := time.Now()
	hook(t, d, "pane.prompted", stop.Add(time.Second))
	hook(t, d, "pane.turn", stop)
	if ready(pn) {
		t.Error("a late Stop freed the prompt of the turn after it")
	}
	hook(t, d, "pane.turn", stop.Add(2*time.Second))
	if !ready(pn) {
		t.Error("the turn's own end freed nothing")
	}

	declare(pn)
	setAgent(pn, proto.Working)
	if ready(pn) {
		t.Error("ready while its screen shows it working, of no records")
	}
}

// Turns no hook marks the start of take the prompt too: Claude Code's own
// record going to work after its prompt was free and idle, as for a
// monitor's event, and a background task ending after it asked for a
// permission. A status the agent declared itself, with kero report, frees
// nothing, as a turn's end doesn't make it stale.
func TestPromptFreeUnhookedTurns(t *testing.T) {
	d := testDaemon(t)
	pn := claudePane(t, d, "conv")
	declare(pn, proto.Record{State: "done", App: "claude-code"})
	hook(t, d, "pane.turn", time.Now().Add(time.Second))
	pn.mu.Lock()
	pn.applyReport(proto.Record{State: "working", App: "claude-code", Progress: -1})
	pn.applyReport(proto.Record{ID: "bg1", State: "working", App: "claude-code", Progress: -1})
	pn.mu.Unlock()
	if ready(pn) {
		t.Error("ready in a turn Claude Code started itself, after an idle prompt")
	}

	hook(t, d, "pane.turn", time.Now().Add(2*time.Second))
	pn.mu.Lock()
	pn.applyReport(proto.Record{ID: "bg1", State: "blocked", Kind: "permission", App: "claude-code", Progress: -1})
	pn.applyReport(proto.Record{ID: "bg1", State: "clear", Progress: -1})
	pn.mu.Unlock()
	if ready(pn) {
		t.Error("ready as a background task ended that asked for a permission")
	}

	declare(pn, proto.Record{State: "working", Msg: "Migrating"})
	hook(t, d, "pane.turn", time.Now().Add(3*time.Second))
	if ready(pn) {
		t.Error("ready with a status of kero report working")
	}
}

// A message typed in holds the next one back until its agent worked on it:
// the end of the turn it started does that, though the background task
// keeps the status as it was.
func TestPromptFreeEndsHold(t *testing.T) {
	d := testDaemon(t)
	pn := claudePane(t, d, "conv")
	declareBackground(pn)
	hook(t, d, "pane.turn", time.Now())
	d.agentSend(proto.AgentSendParams{To: pn.info.ID, Text: "first"})
	if res, _ := d.agentSend(proto.AgentSendParams{To: pn.info.ID, Text: "second"}); res.State != "queued" {
		t.Fatalf("typed in at once after the first: %+v", res)
	}
	hook(t, d, "pane.turn", time.Now().Add(time.Second))
	d.flushOutbox(time.Now())
	d.mail.mu.Lock()
	left := len(d.mail.out[pn.info.ID])
	d.mail.mu.Unlock()
	if left > 0 {
		t.Error("the second waits for deliverHold, though the turn of the first ended")
	}
}

// A page's answer, queued while Claude Code works, reaches it once its turn
// ends, though its background task still runs; kerod's log tells both, and
// who closed a pane.
func TestPageAnswerAtTurnEnd(t *testing.T) {
	d := testDaemon(t)
	var logged bytes.Buffer
	d.log = log.New(&logged, "", 0)
	pn := claudePane(t, d, "conv")
	declareBackground(pn)

	res, err := d.agentAnswer(proto.AgentSendParams{To: pn.info.ID, Label: "answers", Text: "D1: A", User: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.State != "queued" {
		t.Fatalf("sent while it works: %+v", res)
	}
	hook(t, d, "pane.turn", time.Now())
	d.flushOutbox(time.Now())
	d.mail.mu.Lock()
	left := len(d.mail.out[pn.info.ID])
	d.mail.mu.Unlock()
	if left > 0 {
		t.Fatalf("%d message(s) still wait after the turn ended:\n%s", left, logged.String())
	}
	if ready(pn) {
		t.Error("ready again at once, its prompt taken by the message")
	}

	other, err := newPane(d, "s1", proto.Pane{ID: "p2", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.panes["p2"] = other
	d.sessions[0].panes = append(d.sessions[0].panes, other)
	d.mu.Unlock()
	raw, _ := json.Marshal(proto.ClosePaneParams{ID: "p2", By: "kero close, run in p1"})
	if _, err := d.call(nil, "pane.close", raw); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"message 1 for p1 queued: its agent works, as it declared (OSC 7501)",
		"message 1 reached p1's claude, typed into its prompt",
		"pane p2 closed by kero close, run in p1",
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("no %q in kerod's log:\n%s", want, logged.String())
		}
	}
}
