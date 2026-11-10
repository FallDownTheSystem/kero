package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// quietAgent starts a session whose pane runs a program that reads no
// input, standing for an agent kerod knows, idle at its prompt.
func quietAgent(t *testing.T, d *Daemon, agent string) *pane {
	t.Helper()
	cmd := []string{"sleep", "600"}
	if runtime.GOOS == "windows" {
		cmd = []string{"ping", "-n", "600", "127.0.0.1"}
	}
	s, err := d.create(proto.CreateParams{Command: cmd})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.removeSession(s.ID) })
	pn := d.findPane(s.Panes[0].ID)
	pn.mu.Lock()
	pn.info.Agent, pn.info.Status, pn.info.Since = agent, proto.Idle, time.Now().UnixMilli()
	pn.mu.Unlock()
	return pn
}

// idleAgain gives the pane a new idle status, as after a turn, which lets
// kerod give its agent the next message.
func idleAgain(pn *pane) {
	time.Sleep(2 * time.Millisecond)
	pn.mu.Lock()
	pn.info.Status, pn.info.Since = proto.Idle, time.Now().UnixMilli()
	pn.mu.Unlock()
}

// listen starts a listener of the pane's agent, as kero inbox, and
// returns what it gets.
func listen(t *testing.T, d *Daemon, pn *pane) chan proto.InboxMessage {
	return listenAs(t, d, pn, false)
}

// listenAs starts a listener that gives the agent its messages as the
// user's turns, or as events of its own.
func listenAs(t *testing.T, d *Daemon, pn *pane, turns bool) chan proto.InboxMessage {
	t.Helper()
	got := make(chan proto.InboxMessage, 1)
	go func() {
		m, err := d.agentInbox(nil, pn, 3*time.Second, turns)
		if err != nil {
			t.Error(err)
		}
		got <- m
	}()
	for i := 0; ; i++ {
		d.mail.mu.Lock()
		n := len(d.mail.inbox[pn.info.ID])
		d.mail.mu.Unlock()
		if n > 0 {
			return got
		}
		if i == 500 {
			t.Fatal("the listener never waited")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func queued(d *Daemon, id string) int {
	d.mail.mu.Lock()
	defer d.mail.mu.Unlock()
	return len(d.mail.out[id])
}

// A message for an agent with a listener goes to the listener, never into
// its prompt; one that comes while the listener is between two waits
// waits for it; once the listener is gone, messages are typed again.
func TestInboxTakesMessages(t *testing.T) {
	d := testDaemon(t)
	pn := quietAgent(t, d, "claude")
	id := pn.info.ID

	got := listen(t, d, pn)
	res, err := d.agentSend(proto.AgentSendParams{To: id, Label: "orchestrator", Text: "child a\nis done"})
	if err != nil || res.State != "delivered" {
		t.Fatalf("sent %+v, %v", res, err)
	}
	if m := <-got; !strings.Contains(m.Text, "Kero: message from orchestrator:") || !strings.Contains(m.Text, "child a\nis done") {
		t.Errorf("the listener got %q", m.Text)
	}

	// Between two waits: kept for the listener's next.
	idleAgain(pn)
	if res, _ := d.agentSend(proto.AgentSendParams{To: id, Text: "second"}); res.State != "queued" {
		t.Fatalf("not kept for the listener: %+v", res)
	}
	got = listen(t, d, pn)
	d.flushOutbox(time.Now())
	if m := <-got; m.Text != "second" {
		t.Errorf("the listener's next got %q", m.Text)
	}

	// Gone past inboxGrace: typed into the prompt.
	idleAgain(pn)
	if res, _ := d.agentSend(proto.AgentSendParams{To: id, Text: "third"}); res.State != "queued" {
		t.Fatalf("not kept: %+v", res)
	}
	d.flushOutbox(time.Now().Add(inboxGrace))
	if n := queued(d, id); n != 0 {
		t.Errorf("%d messages still wait for a listener that left", n)
	}
}

// A message the user sent never goes to a listener that gives it as an
// event of the agent's own, which the agent would take for no word of the
// user's: it is typed in. One that gives the user's turns takes it.
func TestInboxLeavesTheUsersWords(t *testing.T) {
	d := testDaemon(t)
	pn := quietAgent(t, d, "claude")
	id := pn.info.ID

	got := listenAs(t, d, pn, false)
	res, err := d.agentSend(proto.AgentSendParams{To: id, Label: "decision brief", Text: "accept", User: true})
	if err != nil || res.State != "delivered" {
		t.Fatalf("sent %+v, %v", res, err)
	}
	if n := queued(d, id); n != 0 {
		t.Errorf("%d messages wait", n)
	}
	d.mail.mu.Lock()
	still := len(d.mail.inbox[id])
	d.mail.mu.Unlock()
	if still != 1 {
		t.Errorf("the monitor's listener was used: %d waiting", still)
	}
	select {
	case m := <-got:
		t.Errorf("the monitor got the user's words: %q", m.Text)
	default:
	}
	<-got

	pn2 := quietAgent(t, d, "pi")
	got = listenAs(t, d, pn2, true)
	if res, err := d.agentSend(proto.AgentSendParams{To: pn2.info.ID, Text: "accept", User: true}); err != nil || res.State != "delivered" {
		t.Fatalf("sent %+v, %v", res, err)
	}
	if m := <-got; m.Text != "accept" {
		t.Errorf("the extension got %q", m.Text)
	}
}

// A listener's wait that ends with no message gives "", and a message
// given to it as it stops waiting is still its own.
func TestInboxTimeout(t *testing.T) {
	d := testDaemon(t)
	pn := quietAgent(t, d, "claude")
	m, err := d.agentInbox(nil, pn, 20*time.Millisecond, false)
	if err != nil || m.Text != "" {
		t.Errorf("timed out with %+v, %v", m, err)
	}
	d.mail.mu.Lock()
	_, between := d.mail.listenerLocked(pn.info.ID, time.Now())
	d.mail.mu.Unlock()
	if !between {
		t.Error("a listener that just timed out is not coming back")
	}
}

// Codex takes a message through codex queue, by the session's thread; a
// queue that fails leaves the message to be typed.
func TestNativeCodex(t *testing.T) {
	d := testDaemon(t)
	pn := quietAgent(t, d, "codex")
	pn.mu.Lock()
	pn.info.AgentSession = &proto.AgentSession{Agent: "codex", ID: "019a-thread"}
	pn.mu.Unlock()
	var thread, text string
	fail := false
	old := codexQueue
	codexQueue = func(_ context.Context, th, tx string) error {
		if fail {
			return errors.New("no such thread")
		}
		thread, text = th, tx
		return nil
	}
	t.Cleanup(func() { codexQueue = old })

	if !d.sendNative(pn, "report") || thread != "019a-thread" || text != "report" {
		t.Errorf("queued %q to %q", text, thread)
	}
	fail = true
	if d.sendNative(pn, "report") {
		t.Error("a failed queue counts as given")
	}
	pn.mu.Lock()
	pn.info.AgentSession = &proto.AgentSession{Agent: "claude", ID: "c1"}
	pn.mu.Unlock()
	fail = false
	thread = ""
	if d.sendNative(pn, "report") || thread != "" {
		t.Error("queued for a session of another agent")
	}
}

// A message whose way in fails waits again, first, and the next flush
// types it in, as it does the session's later ones.
func TestNativeFailureTypes(t *testing.T) {
	d := testDaemon(t)
	pn := quietAgent(t, d, "codex")
	id := pn.info.ID
	pn.mu.Lock()
	pn.info.AgentSession = &proto.AgentSession{Agent: "codex", ID: "019a-thread"}
	pn.mu.Unlock()
	tries := make(chan string, 4)
	old := codexQueue
	codexQueue = func(_ context.Context, _, tx string) error {
		tries <- tx
		return errors.New("no such thread")
	}
	t.Cleanup(func() { codexQueue = old })

	d.agentSend(proto.AgentSendParams{To: id, Text: "report"})
	d.flushOutbox(time.Now())
	if tx := <-tries; !strings.Contains(tx, "report") {
		t.Fatalf("queued %q", tx)
	}
	for i := 0; ; i++ {
		d.mail.mu.Lock()
		waits, noNative := len(d.mail.out[id]), d.mail.noNative[id]
		d.mail.mu.Unlock()
		if waits == 1 && noNative == "019a-thread" {
			break
		}
		if i == 500 {
			t.Fatalf("after the failure: %d waiting, no way in for %q", waits, noNative)
		}
		time.Sleep(2 * time.Millisecond)
	}
	d.flushOutbox(time.Now())
	if n := queued(d, id); n != 0 {
		t.Errorf("%d messages not typed in", n)
	}
	select {
	case tx := <-tries:
		t.Errorf("queued again: %q", tx)
	default:
	}
}

// A message a listener took and could not give its agent waits again,
// first, for the pane's next listener.
func TestInboxBack(t *testing.T) {
	d := testDaemon(t)
	pn := quietAgent(t, d, "claude")
	id := pn.info.ID
	d.mail.mu.Lock()
	d.mail.initLocked()
	d.mail.out[id] = []outMsg{{text: "later"}}
	d.mail.mu.Unlock()
	d.inboxBack(pn, "first")
	got := listen(t, d, pn)
	d.flushOutbox(time.Now())
	if m := <-got; m.Text != "first\n\nlater" {
		t.Errorf("the next listener got %q", m.Text)
	}
}

// OpenCode takes a message from its service, as the user's prompt queued
// after the turn, with the service's password.
func TestNativeOpenCode(t *testing.T) {
	var path, auth string
	var body map[string]any
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	svc := filepath.Join(t.TempDir(), "service.json")
	b, _ := json.Marshal(map[string]any{"url": srv.URL + "/", "password": "pw", "pid": 1})
	os.WriteFile(svc, b, 0o600)
	old := openCodeService
	openCodeService = func() string { return svc }
	t.Cleanup(func() { openCodeService = old })

	d := testDaemon(t)
	pn := quietAgent(t, d, "opencode")
	pn.mu.Lock()
	pn.info.AgentSession = &proto.AgentSession{Agent: "opencode", ID: "ses_1"}
	pn.mu.Unlock()
	if !d.sendNative(pn, "report") {
		t.Fatal("not given")
	}
	if path != "/api/session/ses_1/prompt" || auth != "Basic b3BlbmNvZGU6cHc=" || body["text"] != "report" || body["delivery"] != "queue" {
		t.Errorf("posted %s %q %v", path, auth, body)
	}
	status = http.StatusNotFound
	if d.sendNative(pn, "report") {
		t.Error("a refused prompt counts as given")
	}
	os.Remove(svc)
	if d.sendNative(pn, "report") {
		t.Error("given with no service")
	}
}
