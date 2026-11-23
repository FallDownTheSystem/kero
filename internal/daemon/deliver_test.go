package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// agentSession starts a session whose pane, of an agent as far as kerod
// can tell, writes what it is typed to a file it returns.
func agentSession(t *testing.T, d *Daemon) (*pane, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	out := filepath.Join(t.TempDir(), "in")
	s, err := d.create(proto.CreateParams{Command: []string{"sh", "-c", "stty raw -echo; exec cat > " + out}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.removeSession(s.ID) })
	for i := 0; ; i++ {
		if _, err := os.Stat(out); err == nil {
			break
		} else if i == 1000 {
			t.Fatal("cat never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pn := d.findPane(s.Panes[0].ID)
	setAgent(pn, proto.Working)
	return pn, out
}

func setAgent(pn *pane, st proto.Status) {
	pn.mu.Lock()
	pn.info.Agent, pn.info.Status, pn.info.Since = "claude", st, time.Now().UnixMilli()
	pn.mu.Unlock()
}

// typedIn waits for what the agent's pane was typed to have want.
func typedIn(t *testing.T, out, want string) string {
	t.Helper()
	var b []byte
	for range 100 {
		if b, _ = os.ReadFile(out); strings.Contains(string(b), want) {
			return string(b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("typed %q, not %q", b, want)
	return ""
}

// app subscribes a fake app to the daemon, for the deliveries it hears.
func app(d *Daemon) chan proto.Message {
	c := &controlConn{out: make(chan proto.Message, 16), done: make(chan struct{})}
	d.mu.Lock()
	d.subs[c] = struct{}{}
	d.mu.Unlock()
	return c.out
}

func delivery(t *testing.T, events chan proto.Message) proto.Delivery {
	t.Helper()
	for {
		select {
		case m := <-events:
			if m.Event == "delivery" {
				return *m.Delivery
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no delivery told")
		}
	}
}

// A page opened from an agent's pane sends to that agent: labelled with
// the page, typed in only once the agent is at its prompt, and the app
// told it reached it.
func TestAgentSendPush(t *testing.T) {
	enterWait = 50 * time.Millisecond
	t.Cleanup(func() { enterWait = 2 * time.Second })
	d := testDaemon(t)
	events := app(d)
	agent, out := agentSession(t, d)
	s, err := d.split(proto.CreateParams{Split: agent.info.ID, URL: "file:///tmp/brief.html"})
	if err != nil {
		t.Fatal(err)
	}
	page := d.findPane(s.Focus)
	if page.info.Opener != agent.info.ID {
		t.Fatalf("the page's opener is %q, not the agent's pane %s", page.info.Opener, agent.info.ID)
	}
	page.setPage("file:///tmp/brief.html", "Decision brief")

	res, err := d.agentSend(proto.AgentSendParams{From: page.info.ID, Label: "picks", Text: "D1: A\nD2: B"})
	if err != nil {
		t.Fatal(err)
	}
	if res.State != "queued" || res.To != agent.info.ID {
		t.Fatalf("sent to a working agent: %+v", res)
	}
	d.flushOutbox(time.Now())
	if b, _ := os.ReadFile(out); len(b) > 0 {
		t.Fatalf("typed into a working agent: %q", b)
	}
	setAgent(agent, proto.Idle)
	d.flushOutbox(time.Now())
	got := typedIn(t, out, "D2: B")
	if !strings.Contains(got, `Kero: message from picks, from the page "Decision brief" (file:///tmp/brief.html) in browser pane `+page.info.ID) {
		t.Errorf("unlabelled: %q", got)
	}
	if dl := delivery(t, events); dl.ID != res.ID || dl.From != page.info.ID || dl.State != "delivered" {
		t.Errorf("told %+v", dl)
	}

	// Typed in, the agent is left alone until it has worked on it.
	if res, _ := d.agentSend(proto.AgentSendParams{To: agent.info.ID, Text: "again"}); res.State != "queued" {
		t.Errorf("typed in again at once: %+v", res)
	}
	time.Sleep(2 * time.Millisecond) // a Since of another millisecond
	setAgent(agent, proto.Idle)      // a new status, after the typing
	d.flushOutbox(time.Now().Add(time.Millisecond))
	typedIn(t, out, "again")

	// Showing the page again from another pane makes that its opener.
	other, err := d.split(proto.CreateParams{Split: agent.info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.split(proto.CreateParams{Split: other.Focus, URL: "file:///tmp/brief.html"}); err != nil {
		t.Fatal(err)
	}
	if page.info.Opener != other.Focus {
		t.Errorf("the page shown again from %s keeps %s for its opener", other.Focus, page.info.Opener)
	}
}

// An agent waiting for a page's messages takes them, those the page kept
// for it first; one that times out leaves the next for the next wait.
func TestAgentSendWait(t *testing.T) {
	d := testDaemon(t)
	events := app(d)
	s, err := d.create(proto.CreateParams{URL: "file:///tmp/brief.html"})
	if err != nil {
		t.Fatal(err)
	}
	page := d.findPane(s.Panes[0].ID)
	if _, err := d.waitMessage(nil, page, 20*time.Millisecond); err == nil || err.Error() != "timed out" {
		t.Fatalf("waited with nothing sent: %v", err)
	}
	kept, err := d.agentSend(proto.AgentSendParams{From: page.info.ID, Text: "first", Mode: "wait"})
	if err != nil || kept.State != "waiting" {
		t.Fatalf("kept: %+v, %v", kept, err)
	}
	// With no agent waiting nor an opener, a push goes nowhere: the app
	// asks which agent.
	if res, err := d.agentSend(proto.AgentSendParams{From: page.info.ID, Text: "lost"}); err != nil || res.State != "nowhere" {
		t.Fatalf("sent with nowhere to go: %+v, %v", res, err)
	}
	msg, err := d.waitMessage(nil, page, time.Second)
	if err != nil || msg.Text != "first" || msg.ID != kept.ID || msg.URL != "file:///tmp/brief.html" {
		t.Fatalf("took %+v, %v", msg, err)
	}
	if dl := delivery(t, events); dl.ID != kept.ID || dl.State != "delivered" {
		t.Errorf("told %+v", dl)
	}

	got := make(chan proto.AgentMessage)
	go func() {
		m, _ := d.waitMessage(nil, page, 5*time.Second)
		got <- m
	}()
	for {
		d.mail.mu.Lock()
		n := len(d.mail.waiters[page.info.ID])
		d.mail.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	res, err := d.agentSend(proto.AgentSendParams{From: page.info.ID, Text: "second"})
	if err != nil || res.State != "waiting" {
		t.Fatalf("sent to the agent waiting: %+v, %v", res, err)
	}
	if m := <-got; m.Text != "second" || !strings.Contains(m.Prompt(), "the page file:///tmp/brief.html") {
		t.Errorf("the agent waiting took %+v", m)
	}
	// Delivered once the waiter has it.
	if dl := delivery(t, events); dl.ID != res.ID || dl.State != "delivered" {
		t.Errorf("told %+v", dl)
	}

	// Its page closing ends a wait for it.
	errs := make(chan error)
	go func() {
		_, err := d.waitMessage(nil, page, 5*time.Second)
		errs <- err
	}()
	time.Sleep(20 * time.Millisecond)
	d.removeSession(s.ID)
	if err := <-errs; err == nil || err.Error() != "pane closed" {
		t.Errorf("the wait on a closed page ended with %v", err)
	}
}

// What waits for an agent whose pane closes is told never to come.
func TestAgentSendGone(t *testing.T) {
	d := testDaemon(t)
	events := app(d)
	agent, _ := agentSession(t, d)
	s, err := d.split(proto.CreateParams{Split: agent.info.ID, URL: "file:///tmp/brief.html"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := d.agentSend(proto.AgentSendParams{From: s.Focus, Text: "D1: A"})
	if err != nil || res.State != "queued" {
		t.Fatalf("%+v, %v", res, err)
	}
	if err := d.removePane(agent.info.ID, "by a test"); err != nil {
		t.Fatal(err)
	}
	if dl := delivery(t, events); dl.ID != res.ID || dl.State != "failed" || dl.From != s.Focus {
		t.Errorf("told %+v", dl)
	}
	// And a pane no agent runs in takes no message.
	if _, err := d.agentSend(proto.AgentSendParams{To: s.Focus, Text: "x"}); err == nil {
		t.Error("sent to a browser pane")
	}
}

// What waits for an agent that quit fails once it has been gone a while,
// rather than go to the next agent started in its pane; one from no pane
// is told too, for the user to hear of it.
func TestAgentSendAgentQuit(t *testing.T) {
	d := testDaemon(t)
	events := app(d)
	agent, out := agentSession(t, d)
	res, err := d.agentSend(proto.AgentSendParams{To: agent.info.ID, Text: "review it"})
	if err != nil || res.State != "queued" {
		t.Fatalf("%+v, %v", res, err)
	}
	agent.mu.Lock()
	agent.info.Agent, agent.info.Status = "", proto.Idle
	agent.mu.Unlock()
	now := time.Now()
	d.flushOutbox(now)
	d.flushOutbox(now.Add(agentGone / 2))
	select {
	case m := <-events:
		t.Fatalf("told %+v before the agent was gone a while", m)
	default:
	}
	d.flushOutbox(now.Add(agentGone))
	if dl := delivery(t, events); dl.ID != res.ID || dl.State != "failed" || dl.From != "" {
		t.Errorf("told %+v", dl)
	}
	setAgent(agent, proto.Idle)
	d.flushOutbox(now.Add(2 * agentGone))
	time.Sleep(50 * time.Millisecond)
	if b, _ := os.ReadFile(out); len(b) > 0 {
		t.Errorf("typed %q into the next agent", b)
	}
}

// onScreen shows one of Claude Code's screens in pn and lets kerod read
// it, as its poll would, past idleHold.
func onScreen(t *testing.T, pn *pane, name string) {
	t.Helper()
	pn.mu.Lock()
	pn.info.Title = "✳ Claude Code"
	pn.mu.Unlock()
	pn.show(claudeScreen(t, name))
	now := time.Now()
	pn.mu.Lock()
	pn.look(now)
	pn.look(now.Add(idleHold + time.Second))
	pn.mu.Unlock()
}

// declare puts records on pn, as kero report would, in place of those it
// had.
func declare(pn *pane, recs ...proto.Record) {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	pn.applyReport(proto.Record{State: "clear", Progress: -1})
	for _, r := range recs {
		r.Progress, r.Updated = -1, time.Now().UnixMilli()
		pn.applyReport(r)
	}
}

// message sends text to a session or pane as kero message does: through
// the agent.send call, naming no pane it is from.
func message(t *testing.T, d *Daemon, to, label, text string) proto.AgentSendResult {
	t.Helper()
	raw, _ := json.Marshal(proto.AgentSendParams{To: to, Label: label, Text: text, Mode: "push"})
	res, err := d.call(nil, "agent.send", raw)
	if err != nil {
		t.Fatal(err)
	}
	return res.(proto.AgentSendResult)
}

func ready(pn *pane) bool {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	return pn.agentReady()
}

// An agent that declared it waits on a question takes a message once its
// screen shows it at its prompt: not while a dialog shows, nor while its
// turn goes on, nor when it declared it works too, nor for a permission or
// a login it waits on, which typing would answer.
func TestReadyWhileBlocked(t *testing.T) {
	d := testDaemon(t)
	pn, _ := agentSession(t, d)
	setAgent(pn, proto.Idle)
	question := proto.Record{ID: "kero", State: "blocked", Kind: "question", Msg: "Keep the legacy export?"}
	for _, tc := range []struct {
		name   string
		recs   []proto.Record
		screen string
		want   bool
	}{
		{"a question, at its prompt", []proto.Record{question}, "idle", true},
		{"a block of no kind, at its prompt", []proto.Record{{ID: "kero", State: "blocked", Msg: "Which?"}}, "idle", true},
		{"a question, a dialog on screen", []proto.Record{question}, "blocked", false},
		{"a question, its turn going on", []proto.Record{question}, "working", false},
		{"a permission, at its prompt", []proto.Record{{ID: "kero", State: "blocked", Kind: "permission", Msg: "Run rm?"}}, "idle", false},
		{"a login, at its prompt", []proto.Record{{ID: "kero", State: "blocked", Kind: "auth", Msg: "Sign in"}}, "idle", false},
		{"working declared, at its prompt", []proto.Record{{ID: "kero", State: "working", Msg: "Building"}}, "idle", false},
		{"working and a question, at its prompt", []proto.Record{{ID: "kero/a", State: "working"}, question}, "idle", false},
	} {
		declare(pn, tc.recs...)
		onScreen(t, pn, tc.screen)
		if got := ready(pn); got != tc.want {
			t.Errorf("%s: ready %v", tc.name, got)
		}
	}
}

// An orchestrator's kero message reaches a child that declared it waits
// on a question and is back at its prompt; the answer takes the block
// back. A message that answers nothing, as a pull request watch's,
// reaches it too, and leaves the question showing for the user.
func TestMessageToBlockedChild(t *testing.T) {
	enterWait = 50 * time.Millisecond
	t.Cleanup(func() { enterWait = 2 * time.Second })
	d := testDaemon(t)
	orch, _ := agentSession(t, d)
	child, out := agentSession(t, d)
	setAgent(orch, proto.Idle)
	setAgent(child, proto.Idle)
	d.mu.Lock()
	o, k := d.sessionOfLocked(orch), d.sessionOfLocked(child)
	o.orchestrator, k.parent = true, o.id
	d.mu.Unlock()
	declare(child, proto.Record{ID: "kero", State: "blocked", Kind: "question", Msg: "Keep the legacy export?"})
	onScreen(t, child, "idle")
	if st := sessionOf(d, k.id).Pane.Status; st != proto.Waiting {
		t.Fatalf("declared blocked: %s", st)
	}
	if res := message(t, d, k.id, "your orchestrator", "Keep it."); res.State != "delivered" {
		t.Fatalf("to the blocked child: %+v", res)
	}
	typedIn(t, out, "Keep it.")
	if p := sessionOf(d, k.id).Pane; p.Status == proto.Waiting || len(p.Records) > 0 {
		t.Errorf("answered, it still waits: %s %+v", p.Status, p.Records)
	}

	other, out2 := agentSession(t, d)
	setAgent(other, proto.Idle)
	declare(other, proto.Record{ID: "kero", State: "blocked", Kind: "question", Msg: "Merge it?"})
	onScreen(t, other, "idle")
	if res, _ := d.agentSend(proto.AgentSendParams{To: other.info.ID, Text: "PR #4: checks passed."}); res.State != "delivered" {
		t.Fatalf("a watch's news: %+v", res)
	}
	typedIn(t, out2, "checks passed")
	d.mu.Lock()
	oid := d.sessionOfLocked(other).id
	d.mu.Unlock()
	if p := sessionOf(d, oid).Pane; p.Status != proto.Waiting || p.Message != "Merge it?" {
		t.Errorf("news that answers nothing took the question away: %s %q", p.Status, p.Message)
	}
}
