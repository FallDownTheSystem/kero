package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"kero/internal/proto"
)

// A browser pane's console, network and emulation, for kero browser
// console, network and emulate. WebView2 (Windows) tells of them through
// the DevTools Protocol, from the moment the pane's web view is made, so
// what a page logs and requests as it first loads is kept; WebKit (macOS,
// Linux) has no such protocol, so there a script the page runs first
// (consoleScript) keeps what its document logs, and the network and
// emulation are Windows' alone.

// devLog is what a browser pane's page told of its console and network,
// while its web view lives, and how emulate shows it.
type devLog struct {
	// cdp tells that the page speaks the DevTools Protocol.
	cdp     bool
	console []proto.ConsoleEntry
	network []*netEntry
	// live are the requests under way, by the protocol's id of each.
	live   map[string]*netEntry
	nextID int
	emu    proto.Emulation
}

// netEntry is a request of the network's log, with what the protocol
// tells it by: its id, and when it was sent, in its monotonic seconds.
type netEntry struct {
	proto.NetworkRequest
	reqID string
	start float64
}

// How much a pane's logs keep: the oldest go first.
const (
	maxConsole = 1000
	maxNetwork = 1000
	maxLogText = 8000
)

// errNoDevTools is why network and emulate fail where the page speaks no
// DevTools Protocol.
var errNoDevTools = errors.New("this needs the DevTools Protocol, which browser panes have on Windows only")

// watchDevTools has the page of a browser pane tell its console and
// network, then calls start, which loads its first page: the domains are
// on before it requests anything. Where the page speaks no DevTools
// Protocol, consoleScript keeps its console, and start runs at once.
func (a *App) watchDevTools(id string, e *webEntry, start func()) {
	d := &devLog{live: map[string]*netEntry{}}
	e.dev = d
	page := e.view.Page()
	for event, fn := range map[string]func([]byte){
		"Runtime.consoleAPICalled":       d.consoleAPICalled,
		"Runtime.exceptionThrown":        d.exceptionThrown,
		"Log.entryAdded":                 d.logEntry,
		"Network.requestWillBeSent":      d.requestWillBeSent,
		"Network.responseReceived":       d.responseReceived,
		"Network.requestServedFromCache": d.servedFromCache,
		"Network.loadingFinished":        d.loadingFinished,
		"Network.loadingFailed":          d.loadingFailed,
	} {
		if _, err := page.OnDevToolsEvent(event, fn); err != nil {
			start()
			return
		}
	}
	d.cdp = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// A domain that fails to turn on loses its log, not the page. The
		// bodies kept for network body are kept to a pane's 20 MB.
		for _, m := range []struct {
			method string
			params any
		}{
			{"Runtime.enable", nil},
			{"Log.enable", nil},
			{"Network.enable", map[string]int{"maxTotalBufferSize": 20e6, "maxResourceBufferSize": 5e6}},
		} {
			if _, err := page.CallDevTools(ctx, m.method, m.params); err != nil {
				break
			}
		}
		a.update(func() {
			if a.webs[id] == e {
				start()
			}
		})
	}()
}

// devtoolsOp does kero browser console, network and emulate, off the
// main thread once the page loaded (do): what reads or changes the log
// does so on the main thread, where its events come.
func (e *webEntry) devtoolsOp(r *proto.BrowserRequest, do func(func() (*proto.BrowserResult, error))) {
	d, page := e.dev, e.view.Page()
	onMain := mygo.RunOnMain
	switch {
	case r.Op == "console" && !d.cdp:
		// The page's own script kept its document's console.
		do(func() (*proto.BrowserResult, error) {
			res, err := evalPage(page, consoleRead(r.Clear), browseTimeout)
			if err != nil {
				return nil, err
			}
			var out []proto.ConsoleEntry
			if res.Text != "" {
				if err := json.Unmarshal([]byte(res.Text), &out); err != nil {
					return nil, err
				}
			}
			return &proto.BrowserResult{Console: out}, nil
		})
	case !d.cdp:
		do(func() (*proto.BrowserResult, error) {
			return nil, fmt.Errorf("kero browser %s: %w", r.Op, errNoDevTools)
		})
	case r.Op == "console":
		do(func() (*proto.BrowserResult, error) {
			var out []proto.ConsoleEntry
			onMain(func() {
				out = append(out, d.console...)
				if r.Clear {
					d.console = nil
				}
			})
			return &proto.BrowserResult{Console: out}, nil
		})
	case r.Op == "network" && r.Text == "body":
		do(func() (*proto.BrowserResult, error) {
			var req proto.NetworkRequest
			var id string
			onMain(func() {
				for _, n := range d.network {
					if n.ID == r.Request {
						req, id = n.NetworkRequest, n.reqID
					}
				}
			})
			if id == "" {
				return nil, fmt.Errorf("no request #%d in the log", r.Request)
			}
			return responseBody(page, req, id)
		})
	case r.Op == "network":
		do(func() (*proto.BrowserResult, error) {
			var out []proto.NetworkRequest
			onMain(func() {
				for _, n := range d.network {
					out = append(out, n.NetworkRequest)
				}
				if r.Clear {
					d.network, d.live = nil, map[string]*netEntry{}
				}
			})
			return &proto.BrowserResult{Network: out}, nil
		})
	case r.Op == "emulate":
		do(func() (*proto.BrowserResult, error) {
			var was proto.Emulation
			onMain(func() { was = d.emu })
			ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
			defer cancel()
			call := func(method string, params any) error {
				_, err := page.CallDevTools(ctx, method, params)
				return err
			}
			now, err := emulate(call, r.Text, was, r.Emulate)
			onMain(func() { d.emu = now })
			if err != nil {
				return nil, err
			}
			return &proto.BrowserResult{Emulation: &now}, nil
		})
	}
}

// responseBody reads the body of a response the page had, which the
// browser keeps a while.
func responseBody(page *mygo.Page, n proto.NetworkRequest, reqID string) (*proto.BrowserResult, error) {
	if n.Pending {
		return nil, fmt.Errorf("request #%d is still loading", n.ID)
	}
	if n.Error != "" {
		return nil, fmt.Errorf("request #%d failed: %s", n.ID, n.Error)
	}
	ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
	defer cancel()
	raw, err := page.CallDevTools(ctx, "Network.getResponseBody", map[string]string{"requestId": reqID})
	if err != nil {
		return nil, fmt.Errorf("request #%d's body: %w", n.ID, err)
	}
	var b struct {
		Body          string `json:"body"`
		Base64Encoded bool   `json:"base64Encoded"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	res := &proto.BrowserResult{Network: []proto.NetworkRequest{n}, Body: []byte(b.Body)}
	if b.Base64Encoded {
		if res.Body, err = base64.StdEncoding.DecodeString(b.Body); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// emulate changes what the page is shown as, from was, with the page's
// DevTools Protocol (call): the device or its viewport, the user agent,
// the network, or all back as they are (reset); none shows what is. It
// returns what is now, which a failure leaves part changed.
func emulate(call func(method string, params any) error, what string, was proto.Emulation, to *proto.Emulation) (proto.Emulation, error) {
	if to == nil {
		to = &proto.Emulation{}
	}
	now := was
	viewport := func() error {
		var err error
		if to.Width == 0 && to.Height == 0 {
			err = call("Emulation.clearDeviceMetricsOverride", nil)
		} else {
			err = call("Emulation.setDeviceMetricsOverride", map[string]any{
				"width": to.Width, "height": to.Height, "deviceScaleFactor": to.Scale, "mobile": to.Mobile,
			})
		}
		if err != nil {
			return err
		}
		now.Device, now.Width, now.Height, now.Scale, now.Mobile = to.Device, to.Width, to.Height, to.Scale, to.Mobile
		touch := map[string]any{"enabled": to.Touch}
		if to.Touch {
			touch["maxTouchPoints"] = 5
		}
		if err := call("Emulation.setTouchEmulationEnabled", touch); err != nil {
			return err
		}
		if err := call("Emulation.setEmitTouchEventsForMouse", map[string]any{"enabled": to.Touch, "configuration": "mobile"}); err != nil {
			return err
		}
		now.Touch = to.Touch
		return nil
	}
	userAgent := func() error {
		// An empty user agent is the browser's own again.
		if err := call("Emulation.setUserAgentOverride", map[string]string{"userAgent": to.UserAgent}); err != nil {
			return err
		}
		now.UserAgent = to.UserAgent
		return nil
	}
	network := func() error {
		down, up := to.Download, to.Upload
		if down == 0 {
			down = -1
		}
		if up == 0 {
			up = -1
		}
		if err := call("Network.emulateNetworkConditions", map[string]any{
			"offline": to.Network == "offline", "latency": to.Latency, "downloadThroughput": down, "uploadThroughput": up,
		}); err != nil {
			return err
		}
		now.Network, now.Latency, now.Download, now.Upload = to.Network, to.Latency, to.Download, to.Upload
		if now.Network == "none" {
			now.Network = ""
		}
		return nil
	}
	var err error
	switch what {
	case "":
	case "viewport":
		err = viewport()
	case "device":
		if err = viewport(); err == nil {
			err = userAgent()
		}
	case "user-agent":
		err = userAgent()
	case "network":
		err = network()
	case "reset":
		*to = proto.Emulation{}
		err = errors.Join(viewport(), userAgent(), network())
	default:
		err = fmt.Errorf("emulate: no %q", what)
	}
	return now, err
}

// The protocol's events, as the log takes them. Each runs on the main
// thread; one the log cannot read changes nothing.

// remoteObject is a value the protocol tells of the page's.
type remoteObject struct {
	Type                string          `json:"type"`
	Subtype             string          `json:"subtype"`
	Value               json.RawMessage `json:"value"`
	UnserializableValue string          `json:"unserializableValue"`
	Description         string          `json:"description"`
	Preview             *struct {
		Overflow   bool `json:"overflow"`
		Properties []struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"properties"`
	} `json:"preview"`
}

type stackTrace struct {
	CallFrames []struct {
		FunctionName string `json:"functionName"`
		URL          string `json:"url"`
		LineNumber   int    `json:"lineNumber"`
		ColumnNumber int    `json:"columnNumber"`
	} `json:"callFrames"`
}

// text is how a console shows a value: a string as it is, an object by
// its preview, else as the page describes it.
func (o remoteObject) text() string {
	switch {
	case o.Type == "string":
		var s string
		json.Unmarshal(o.Value, &s)
		return s
	case o.Type == "undefined":
		return "undefined"
	case o.UnserializableValue != "":
		return o.UnserializableValue
	case o.Preview != nil && (o.Subtype == "array" || o.Type == "object" && o.Subtype == ""):
		var parts []string
		for _, p := range o.Preview.Properties {
			v := p.Value
			if p.Type == "string" {
				v = strconv.Quote(v)
			}
			if o.Subtype == "array" {
				parts = append(parts, v)
			} else {
				parts = append(parts, p.Name+": "+v)
			}
		}
		if o.Preview.Overflow {
			parts = append(parts, "…")
		}
		if o.Subtype == "array" {
			return "[" + strings.Join(parts, ", ") + "]"
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case len(o.Value) > 0 && o.Description == "":
		return string(o.Value)
	}
	return o.Description
}

// consoleText joins what console.* was given as a console does,
// substituting the first's %s, %d, %i, %f, %o and %O; %c, a style, shows
// nothing.
func consoleText(args []remoteObject) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	rest := args[1:]
	if args[0].Type == "string" && strings.Contains(args[0].text(), "%") {
		var b strings.Builder
		f := args[0].text()
		for i := 0; i < len(f); i++ {
			if f[i] != '%' || i+1 == len(f) {
				b.WriteByte(f[i])
				continue
			}
			switch c := f[i+1]; {
			case c == '%':
				b.WriteByte('%')
			case strings.IndexByte("sdifoOc", c) >= 0 && len(rest) > 0:
				if c != 'c' {
					b.WriteString(rest[0].text())
				}
				rest = rest[1:]
			default:
				b.WriteByte('%')
				continue
			}
			i++
		}
		parts = append(parts, b.String())
	} else {
		parts = append(parts, args[0].text())
	}
	for _, a := range rest {
		parts = append(parts, a.text())
	}
	return strings.Join(parts, " ")
}

// at sets where an entry came from, from a stack's top frame, and with
// all, the stack.
func (st *stackTrace) at(c *proto.ConsoleEntry, all bool) {
	if st == nil || len(st.CallFrames) == 0 {
		return
	}
	if f := st.CallFrames[0]; c.URL == "" {
		c.URL, c.Line, c.Column = f.URL, f.LineNumber+1, f.ColumnNumber+1
	}
	if !all {
		return
	}
	for i, f := range st.CallFrames {
		if i == 10 {
			c.Stack = append(c.Stack, fmt.Sprintf("… %d more", len(st.CallFrames)-i))
			break
		}
		fn := f.FunctionName
		if fn == "" {
			fn = "(anonymous)"
		}
		c.Stack = append(c.Stack, fmt.Sprintf("%s (%s:%d:%d)", fn, f.URL, f.LineNumber+1, f.ColumnNumber+1))
	}
}

func (d *devLog) add(c proto.ConsoleEntry) {
	if len(c.Text) > maxLogText {
		c.Text = c.Text[:maxLogText] + "…"
	}
	if len(d.console) == maxConsole {
		d.console = append(d.console[:0], d.console[1:]...)
	}
	d.console = append(d.console, c)
}

func (d *devLog) consoleAPICalled(b []byte) {
	var ev struct {
		Type       string         `json:"type"`
		Args       []remoteObject `json:"args"`
		Timestamp  float64        `json:"timestamp"`
		StackTrace *stackTrace    `json:"stackTrace"`
	}
	if json.Unmarshal(b, &ev) != nil {
		return
	}
	c := proto.ConsoleEntry{Time: int64(ev.Timestamp), Level: "info", Source: "console", Text: consoleText(ev.Args)}
	switch ev.Type {
	case "endGroup", "clear":
		return
	case "error", "assert":
		c.Level = "error"
	case "warning":
		c.Level = "warning"
	case "debug":
		c.Level = "debug"
	}
	if ev.Type == "assert" && c.Text == "" {
		c.Text = "Assertion failed"
	}
	// An error logged tells its own stack, where it was made, which the
	// call's would only repeat.
	stack := c.Level == "error" || ev.Type == "trace"
	for _, a := range ev.Args {
		stack = stack && a.Subtype != "error"
	}
	ev.StackTrace.at(&c, stack)
	d.add(c)
}

func (d *devLog) exceptionThrown(b []byte) {
	var ev struct {
		Timestamp float64 `json:"timestamp"`
		Details   struct {
			Text         string        `json:"text"`
			URL          string        `json:"url"`
			LineNumber   int           `json:"lineNumber"`
			ColumnNumber int           `json:"columnNumber"`
			StackTrace   *stackTrace   `json:"stackTrace"`
			Exception    *remoteObject `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if json.Unmarshal(b, &ev) != nil {
		return
	}
	x := ev.Details
	c := proto.ConsoleEntry{Time: int64(ev.Timestamp), Level: "error", Source: "exception", Text: x.Text}
	if strings.Contains(x.Text, "(in promise)") {
		c.Source = "rejection"
	}
	// The exception's description is its message, then its stack.
	if o := x.Exception; o != nil {
		what, _, _ := strings.Cut(o.text(), "\n")
		if what != "" && !strings.Contains(c.Text, what) {
			c.Text = strings.TrimSpace(c.Text + " " + what)
		}
	}
	if x.URL != "" {
		c.URL, c.Line, c.Column = x.URL, x.LineNumber+1, x.ColumnNumber+1
	}
	x.StackTrace.at(&c, true)
	d.add(c)
}

func (d *devLog) logEntry(b []byte) {
	var ev struct {
		Entry struct {
			Source     string      `json:"source"`
			Level      string      `json:"level"`
			Text       string      `json:"text"`
			URL        string      `json:"url"`
			LineNumber *int        `json:"lineNumber"`
			Timestamp  float64     `json:"timestamp"`
			StackTrace *stackTrace `json:"stackTrace"`
		} `json:"entry"`
	}
	if json.Unmarshal(b, &ev) != nil {
		return
	}
	x := ev.Entry
	c := proto.ConsoleEntry{Time: int64(x.Timestamp), Level: x.Level, Source: x.Source, Text: x.Text, URL: x.URL}
	if c.Level == "verbose" {
		c.Level = "debug"
	}
	if x.LineNumber != nil {
		c.Line = *x.LineNumber + 1
	}
	x.StackTrace.at(&c, false)
	d.add(c)
}

type netResponse struct {
	Status        int     `json:"status"`
	StatusText    string  `json:"statusText"`
	MIMEType      string  `json:"mimeType"`
	FromDiskCache bool    `json:"fromDiskCache"`
	FromSW        bool    `json:"fromServiceWorker"`
	Encoded       float64 `json:"encodedDataLength"`
}

// ended ends a request at the protocol's time.
func (n *netEntry) ended(at float64) {
	n.Pending = false
	if at > n.start {
		n.Duration = math.Round((at-n.start)*10000) / 10
	}
}

func (d *devLog) requestWillBeSent(b []byte) {
	var ev struct {
		RequestID string `json:"requestId"`
		Request   struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		} `json:"request"`
		Timestamp        float64      `json:"timestamp"`
		WallTime         float64      `json:"wallTime"`
		Type             string       `json:"type"`
		RedirectResponse *netResponse `json:"redirectResponse"`
	}
	if json.Unmarshal(b, &ev) != nil || strings.HasPrefix(ev.Request.URL, "data:") {
		return
	}
	// A redirect goes on with the same id: the hop before ends with it.
	if n := d.live[ev.RequestID]; n != nil && ev.RedirectResponse != nil {
		n.respond(ev.RedirectResponse)
		n.ended(ev.Timestamp)
	}
	d.nextID++
	n := &netEntry{reqID: ev.RequestID, start: ev.Timestamp, NetworkRequest: proto.NetworkRequest{
		ID: d.nextID, Method: ev.Request.Method, URL: ev.Request.URL, Type: ev.Type,
		Time: int64(ev.WallTime * 1000), Pending: true,
	}}
	if len(d.network) == maxNetwork {
		if old := d.network[0]; d.live[old.reqID] == old {
			delete(d.live, old.reqID)
		}
		d.network = append(d.network[:0], d.network[1:]...)
	}
	d.network = append(d.network, n)
	d.live[ev.RequestID] = n
}

func (n *netEntry) respond(r *netResponse) {
	n.Status, n.StatusText, n.MIMEType = r.Status, r.StatusText, r.MIMEType
	n.FromCache = n.FromCache || r.FromDiskCache || r.FromSW
	n.Size = int64(r.Encoded)
}

func (d *devLog) responseReceived(b []byte) {
	var ev struct {
		RequestID string      `json:"requestId"`
		Type      string      `json:"type"`
		Response  netResponse `json:"response"`
	}
	if json.Unmarshal(b, &ev) != nil {
		return
	}
	if n := d.live[ev.RequestID]; n != nil {
		n.respond(&ev.Response)
		if n.Type == "" {
			n.Type = ev.Type
		}
	}
}

func (d *devLog) servedFromCache(b []byte) {
	var ev struct {
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(b, &ev) == nil && d.live[ev.RequestID] != nil {
		d.live[ev.RequestID].FromCache = true
	}
}

func (d *devLog) loadingFinished(b []byte) {
	var ev struct {
		RequestID string  `json:"requestId"`
		Timestamp float64 `json:"timestamp"`
		Encoded   float64 `json:"encodedDataLength"`
	}
	if json.Unmarshal(b, &ev) != nil {
		return
	}
	if n := d.live[ev.RequestID]; n != nil {
		n.ended(ev.Timestamp)
		n.Size = int64(ev.Encoded)
	}
}

func (d *devLog) loadingFailed(b []byte) {
	var ev struct {
		RequestID     string  `json:"requestId"`
		Timestamp     float64 `json:"timestamp"`
		Type          string  `json:"type"`
		ErrorText     string  `json:"errorText"`
		Canceled      bool    `json:"canceled"`
		BlockedReason string  `json:"blockedReason"`
		CORS          *struct {
			CORSError       string `json:"corsError"`
			FailedParameter string `json:"failedParameter"`
		} `json:"corsErrorStatus"`
	}
	if json.Unmarshal(b, &ev) != nil {
		return
	}
	n := d.live[ev.RequestID]
	if n == nil {
		return
	}
	n.ended(ev.Timestamp)
	why := []string{}
	switch {
	case ev.BlockedReason != "":
		why = append(why, "blocked: "+ev.BlockedReason)
	case ev.CORS != nil && ev.CORS.CORSError != "":
		cors := "CORS " + ev.CORS.CORSError
		if ev.CORS.FailedParameter != "" {
			cors += " (" + ev.CORS.FailedParameter + ")"
		}
		why = append(why, cors)
	case ev.Canceled:
		why = append(why, "canceled")
	}
	n.Error = ev.ErrorText
	if len(why) > 0 {
		n.Error = strings.TrimSpace(n.Error + " " + strings.Join(why, ", "))
	}
	if n.Error == "" {
		n.Error = "failed"
	}
}

// consoleScript keeps what a document logs where the page speaks no
// DevTools Protocol, before the page's own scripts run: console.*,
// uncaught errors and promises none caught, with where each came from,
// in window.__keroConsole, which consoleRead reads. WebView2's pages tell
// the protocol instead, so they run none of it.
const consoleScript = `(() => {
  if (window.__keroConsole) return;
  const log = [];
  Object.defineProperty(window, "__keroConsole", { value: log });
  const show = (v) => {
    if (typeof v === "string") return v;
    if (v instanceof Error) return v.stack || String(v);
    try { const s = JSON.stringify(v); if (s !== undefined) return s; } catch {}
    return String(v);
  };
  // As console.* joins its arguments, with the first's %s, %d, ... in
  // place, and %c, a style, showing nothing.
  const join = (args) => {
    if (!args.length) return "";
    let [first, ...rest] = args;
    if (typeof first === "string" && first.includes("%")) {
      first = first.replace(/%[sdifoOc%]/g, (m) => m === "%%" ? "%" : !rest.length ? m : m === "%c" ? (rest.shift(), "") : show(rest.shift()));
    } else first = show(first);
    return [first, ...rest.map(show)].join(" ");
  };
  const keep = (e) => {
    if (log.length >= 1000) log.shift();
    e.time = Date.now();
    if (e.text.length > 8000) e.text = e.text.slice(0, 8000) + "…";
    log.push(e);
  };
  // Where a stack's frame skip came from: 1, past this script's own, is
  // console.*'s caller.
  const where = (stack, skip) => {
    const lines = String(stack || "").split("\n").filter((l) => /:\d+:\d+/.test(l));
    const m = /(\S+?):(\d+):(\d+)\)?\s*$/.exec(lines[skip] || "");
    return m ? { url: m[1].replace(/^\(|^.*@/, ""), line: +m[2], column: +m[3] } : {};
  };
  const levels = { log: "info", info: "info", debug: "debug", warn: "warning", error: "error", trace: "info", assert: "error" };
  for (const [name, level] of Object.entries(levels)) {
    const orig = console[name];
    if (typeof orig !== "function") continue;
    console[name] = function (...args) {
      let shown = args;
      if (name === "assert") {
        if (args[0]) return orig.apply(this, args);
        shown = args.length > 1 ? args.slice(1) : ["Assertion failed"];
      }
      keep({ level, source: "console", text: join(shown), ...where(new Error().stack, 1) });
      return orig.apply(this, args);
    };
  }
  addEventListener("error", (e) => {
    if (!(e instanceof ErrorEvent)) return;
    const stack = e.error && e.error.stack ? String(e.error.stack).split("\n").slice(1).map((l) => l.trim().replace(/^at /, "")) : undefined;
    keep({ level: "error", source: "exception", text: "Uncaught " + (e.message || "").replace(/^Uncaught /, ""), url: e.filename, line: e.lineno, column: e.colno, stack });
  });
  addEventListener("unhandledrejection", (e) => {
    const r = e.reason;
    keep({ level: "error", source: "rejection", text: "Uncaught (in promise) " + (r instanceof Error ? String(r) : show(r)), ...(r instanceof Error ? where(r.stack, 0) : {}) });
  });
})();`

// preloadConsole is consoleScript, for the page to run first, where it
// speaks no DevTools Protocol.
func preloadConsole() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return "\n" + consoleScript
}

// consoleRead is the script that reads what consoleScript kept, as JSON,
// and with clear, forgets it.
func consoleRead(clear bool) string {
	s := `JSON.stringify(window.__keroConsole || [])`
	if clear {
		s = `(() => { const l = window.__keroConsole || []; const s = JSON.stringify(l); l.length = 0; return s })()`
	}
	return s
}
