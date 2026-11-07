package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

// A page's message goes to its opener, as kerod picks it; for a page
// with none, to the agent the user chooses, and there again after.
func TestPageSend(t *testing.T) {
	a := testApp(t)
	sent := make(chan proto.AgentSendParams, 8)
	var opened atomic.Bool // the page has an opener, which kerod sends to
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method != "agent.send" {
			return nil, errors.New("no " + method + " in a test")
		}
		var p proto.AgentSendParams
		json.Unmarshal(raw, &p)
		sent <- p
		if p.To == "" && !opened.Load() {
			return proto.AgentSendResult{ID: 1, State: "nowhere"}, nil
		}
		return proto.AgentSendResult{ID: 2, To: p.To, State: "queued"}, nil
	})
	a.local().cl = cl
	s := &a.st.Sessions[0]
	s.Panes[1].Agent = "claude"
	s.Panes = append(s.Panes, proto.Pane{ID: "w", URL: "file:///tmp/brief.html", Progress: -1})
	a.webs["w"] = &webEntry{}
	tt := ui.NewTester(a.view, 1100, 640)

	type result struct {
		res PageSent
		err error
	}
	send := func() <-chan result {
		done := make(chan result, 1)
		go func() {
			res, err := a.pageSend(context.Background(), "w", PageMessage{Text: "D1: A", Label: "picks"})
			done <- result{res, err}
		}()
		return done
	}
	next := func() proto.AgentSendParams {
		t.Helper()
		for range 200 {
			tt.Frame()
			select {
			case p := <-sent:
				return p
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("nothing sent")
		return proto.AgentSendParams{}
	}

	done := send()
	if p := next(); p.To != "" || p.From != "w" || p.Label != "picks" || p.Text != "D1: A" {
		t.Fatalf("first sent %+v", p)
	}
	for i := 0; a.pal.choose == nil; i++ {
		if i == 200 {
			t.Fatal("no choice of agents for a page no agent opened")
		}
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEnter) // pane b's agent, the only one
	if p := next(); p.To != "b" || p.From != "w" {
		t.Fatalf("then sent %+v", p)
	}
	if r := <-done; r.err != nil || r.res.State != "queued" || r.res.To != "b" {
		t.Fatalf("the page heard %+v", r)
	}
	// The next goes where the user chose, unasked.
	done = send()
	if p := next(); p.To != "b" {
		t.Errorf("the next went to %q", p.To)
	}
	<-done

	// A page an agent opened leaves the choice to kerod.
	s.Panes[2].Opener = "a"
	opened.Store(true)
	done = send()
	if p := next(); p.To != "" {
		t.Errorf("a page with an opener sent to %q", p.To)
	}
	<-done
}

// The page script makes window.kero in local pages alone; the example
// page's Send sends with it, says queued, then sent once the agent has
// it, even when kerod tells it before the call returns; and copies where
// there is no window.kero.
func TestBridgeScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node")
	}
	page, err := os.ReadFile("testdata/send-to-agent.html")
	if err != nil {
		t.Fatal(err)
	}
	example := string(page)
	example = example[strings.Index(example, "<script>")+len("<script>") : strings.Index(example, "</script>")]
	harness := `
const results = {};
const run = async (protocol, early) => {
  const listeners = {}, calls = [];
  const els = { send: { textContent: "Send" }, status: { textContent: "" } };
  const g = {
    location: { protocol },
    navigator: { clipboard: { writeText: async (t) => { results.copied = t; } } },
    document: { getElementById: (id) => els[id], querySelector: () => ({ value: "B" }) },
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init.detail; } },
    dispatchEvent: (e) => { (results.events ||= []).push(e.detail.state); },
    mygo: {
      on: (name, fn) => { listeners[name] = fn; },
      call: async (method, arg) => {
        calls.push([method, arg]);
        if (early) listeners["kero:delivery"]({ id: 9, from: "w", state: "delivered" });
        return { id: 9, state: "queued", to: "b" };
      },
    },
  };
  g.window = g;
  const f = new Function(...Object.keys(g), BRIDGE + "\n" + EXAMPLE + "\nreturn { kero: window.kero, send };");
  const { kero, send } = f(...Object.values(g));
  return { kero, send, els, calls, deliver: (d) => listeners["kero:delivery"](d) };
};
(async () => {
  const web = await run("https:", false);
  results.https = web.kero === undefined;
  await web.send.onclick();
  results.httpsStatus = web.els.status.textContent;

  const p = await run("file:", false);
  results.version = p.kero.version;
  const clicked = p.send.onclick();
  await new Promise((r) => setTimeout(r, 10));
  results.call = p.calls[0];
  results.queued = p.els.status.textContent;
  p.deliver({ id: 9, from: "w", to: "b", state: "delivered" });
  await clicked;
  results.sent = p.els.status.textContent;

  const e = await run("file:", true);
  await e.send.onclick();
  results.early = e.els.status.textContent;

  const f = await run("file:", false);
  const failed = f.send.onclick();
  await new Promise((r) => setTimeout(r, 10));
  f.deliver({ id: 9, from: "w", state: "failed", error: "the agent's pane closed" });
  await failed;
  results.failed = f.els.status.textContent;
  console.log(JSON.stringify(results));
})();
`
	src := "const BRIDGE = " + jsString(bridgeScript) + ";\nconst EXAMPLE = " + jsString(example) + ";\n" + harness
	out, err := exec.Command(node, "-e", src).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var got struct {
		HTTPS       bool     `json:"https"`
		HTTPSStatus string   `json:"httpsStatus"`
		Copied      string   `json:"copied"`
		Version     int      `json:"version"`
		Call        []any    `json:"call"`
		Queued      string   `json:"queued"`
		Sent        string   `json:"sent"`
		Early       string   `json:"early"`
		Failed      string   `json:"failed"`
		Events      []string `json:"events"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !got.HTTPS || got.Copied != "D1: B" || !strings.HasPrefix(got.HTTPSStatus, "Copied") {
		t.Errorf("a page of the web: kero hidden %v, copied %q, said %q", got.HTTPS, got.Copied, got.HTTPSStatus)
	}
	if got.Version != 1 || len(got.Call) != 2 || got.Call[0] != "Kero.Send" {
		t.Fatalf("window.kero %d called %v", got.Version, got.Call)
	}
	if arg, _ := got.Call[1].(map[string]any); arg["text"] != "D1: B" || arg["label"] != "database pick" {
		t.Errorf("sent %v", got.Call[1])
	}
	if got.Queued != "Queued: the agent is working" || got.Sent != "Sent" {
		t.Errorf("said %q, then %q", got.Queued, got.Sent)
	}
	if got.Early != "Sent" {
		t.Errorf("told before the call returned, said %q", got.Early)
	}
	if got.Failed != "Not sent: the agent's pane closed" {
		t.Errorf("failed, said %q", got.Failed)
	}
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
