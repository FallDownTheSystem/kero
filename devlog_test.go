package main

import (
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"kero/internal/proto"
)

// The console keeps what the page logged, threw and the browser told of,
// as the DevTools Protocol tells them: the text as a console shows it,
// where it came from, from 1, and an error's stack.
func TestDevLogConsole(t *testing.T) {
	d := &devLog{live: map[string]*netEntry{}}
	d.consoleAPICalled([]byte(`{"type":"log","timestamp":1700000000123.5,
		"args":[{"type":"string","value":"%s items, %d left%c"},{"type":"number","value":3,"description":"3"},{"type":"number","value":1,"description":"1"},{"type":"string","value":"color:red"},
			{"type":"object","className":"Object","description":"Object","preview":{"type":"object","overflow":false,"properties":[{"name":"done","type":"boolean","value":"true"},{"name":"title","type":"string","value":"milk"}]}}],
		"stackTrace":{"callFrames":[{"functionName":"add","url":"http://localhost:3000/src/app.js","lineNumber":11,"columnNumber":4},{"functionName":"","url":"http://localhost:3000/src/main.js","lineNumber":0,"columnNumber":0}]}}`))
	d.consoleAPICalled([]byte(`{"type":"error","timestamp":1,"args":[{"type":"object","subtype":"array","description":"Array(2)","preview":{"overflow":true,"properties":[{"name":"0","type":"number","value":"1"},{"name":"1","type":"string","value":"b"}]}}],
		"stackTrace":{"callFrames":[{"functionName":"save","url":"http://x.dev/a.js","lineNumber":0,"columnNumber":9},{"functionName":"","url":"http://x.dev/b.js","lineNumber":4,"columnNumber":1}]}}`))
	d.consoleAPICalled([]byte(`{"type":"endGroup","timestamp":2,"args":[]}`))
	// As Chromium tells console.error(new Error("boom")).
	d.consoleAPICalled([]byte(`{"type":"error","timestamp":2,"args":[{"type":"object","subtype":"error","className":"Error","description":"Error: boom\n    at save (http://x.dev/:4:33)"}],
		"stackTrace":{"callFrames":[{"functionName":"save","url":"http://x.dev/","lineNumber":3,"columnNumber":26}]}}`))
	d.exceptionThrown([]byte(`{"timestamp":3,"exceptionDetails":{"text":"Uncaught","lineNumber":13,"columnNumber":21,"url":"http://localhost:3000/src/List.tsx",
		"exception":{"type":"object","subtype":"error","className":"TypeError","description":"TypeError: Cannot read properties of undefined (reading 'map')\n    at List (http://localhost:3000/src/List.tsx:14:22)"},
		"stackTrace":{"callFrames":[{"functionName":"List","url":"http://localhost:3000/src/List.tsx","lineNumber":13,"columnNumber":21}]}}}`))
	d.exceptionThrown([]byte(`{"timestamp":4,"exceptionDetails":{"text":"Uncaught (in promise)","lineNumber":0,"columnNumber":0,"exception":{"type":"string","value":"nope"}}}`))
	d.logEntry([]byte(`{"entry":{"source":"network","level":"error","text":"Failed to load resource: the server responded with a status of 404 (Not Found)","url":"http://localhost:3000/api/todos","timestamp":5}}`))
	d.logEntry([]byte(`{"entry":{"source":"violation","level":"verbose","text":"Forced reflow","lineNumber":0,"url":"http://x.dev/c.js","timestamp":6}}`))

	want := []proto.ConsoleEntry{
		{Time: 1700000000123, Level: "info", Source: "console", Text: "3 items, 1 left {done: true, title: \"milk\"}", URL: "http://localhost:3000/src/app.js", Line: 12, Column: 5},
		{Time: 1, Level: "error", Source: "console", Text: `[1, "b", …]`, URL: "http://x.dev/a.js", Line: 1, Column: 10,
			Stack: []string{"save (http://x.dev/a.js:1:10)", "(anonymous) (http://x.dev/b.js:5:2)"}},
		{Time: 2, Level: "error", Source: "console", Text: "Error: boom\n    at save (http://x.dev/:4:33)", URL: "http://x.dev/", Line: 4, Column: 27},
		{Time: 3, Level: "error", Source: "exception", Text: "Uncaught TypeError: Cannot read properties of undefined (reading 'map')",
			URL: "http://localhost:3000/src/List.tsx", Line: 14, Column: 22, Stack: []string{"List (http://localhost:3000/src/List.tsx:14:22)"}},
		{Time: 4, Level: "error", Source: "rejection", Text: "Uncaught (in promise) nope", Line: 0},
		{Time: 5, Level: "error", Source: "network", Text: "Failed to load resource: the server responded with a status of 404 (Not Found)", URL: "http://localhost:3000/api/todos"},
		{Time: 6, Level: "debug", Source: "violation", Text: "Forced reflow", URL: "http://x.dev/c.js", Line: 1},
	}
	if !reflect.DeepEqual(d.console, want) {
		for i := range max(len(d.console), len(want)) {
			var got, w proto.ConsoleEntry
			if i < len(d.console) {
				got = d.console[i]
			}
			if i < len(want) {
				w = want[i]
			}
			if !reflect.DeepEqual(got, w) {
				t.Errorf("entry %d:\n got %+v\nwant %+v", i, got, w)
			}
		}
	}

	// The oldest go first, past what the log keeps.
	for range maxConsole {
		d.consoleAPICalled([]byte(`{"type":"log","args":[{"type":"string","value":"x"}]}`))
	}
	if len(d.console) != maxConsole || d.console[0].Text != "x" {
		t.Errorf("%d entries kept, the first %q", len(d.console), d.console[0].Text)
	}
}

// The network's log has a line for each request and each hop of a
// redirect, with its status, how long it took and its size, and why one
// failed: a CORS error, a block, a network error.
func TestDevLogNetwork(t *testing.T) {
	d := &devLog{live: map[string]*netEntry{}}
	ev := func(fn func([]byte), s string) { fn([]byte(s)) }
	ev(d.requestWillBeSent, `{"requestId":"A","request":{"url":"http://localhost:3000/","method":"GET"},"timestamp":100.0,"wallTime":1700000000.5,"type":"Document"}`)
	ev(d.requestWillBeSent, `{"requestId":"A","request":{"url":"http://localhost:3000/login","method":"GET"},"timestamp":100.25,"wallTime":1700000000.75,"type":"Document",
		"redirectResponse":{"status":302,"statusText":"Found","mimeType":"text/html","encodedDataLength":120}}`)
	ev(d.responseReceived, `{"requestId":"A","type":"Document","response":{"status":200,"statusText":"OK","mimeType":"text/html","encodedDataLength":300}}`)
	ev(d.loadingFinished, `{"requestId":"A","timestamp":100.5,"encodedDataLength":4321}`)
	ev(d.requestWillBeSent, `{"requestId":"B","request":{"url":"http://localhost:3000/api/todos","method":"POST"},"timestamp":101.0,"wallTime":1700000001,"type":"Fetch"}`)
	ev(d.responseReceived, `{"requestId":"B","type":"Fetch","response":{"status":500,"statusText":"Internal Server Error","mimeType":"application/json"}}`)
	ev(d.loadingFinished, `{"requestId":"B","timestamp":101.0123,"encodedDataLength":88}`)
	ev(d.requestWillBeSent, `{"requestId":"C","request":{"url":"https://api.other.dev/x","method":"GET"},"timestamp":102,"wallTime":1700000002,"type":"XHR"}`)
	ev(d.loadingFailed, `{"requestId":"C","timestamp":102.5,"type":"XHR","errorText":"net::ERR_FAILED","canceled":false,"corsErrorStatus":{"corsError":"MissingAllowOriginHeader","failedParameter":""}}`)
	ev(d.requestWillBeSent, `{"requestId":"D","request":{"url":"https://ads.dev/a.js","method":"GET"},"timestamp":103,"wallTime":1700000003,"type":"Script"}`)
	ev(d.loadingFailed, `{"requestId":"D","timestamp":103,"type":"Script","errorText":"net::ERR_BLOCKED_BY_CLIENT","blockedReason":"inspector"}`)
	ev(d.requestWillBeSent, `{"requestId":"E","request":{"url":"data:image/png;base64,AAAA","method":"GET"},"timestamp":104,"wallTime":1700000004,"type":"Image"}`)
	ev(d.requestWillBeSent, `{"requestId":"F","request":{"url":"http://localhost:3000/logo.png","method":"GET"},"timestamp":105,"wallTime":1700000005,"type":"Image"}`)
	ev(d.servedFromCache, `{"requestId":"F"}`)
	ev(d.requestWillBeSent, `{"requestId":"G","request":{"url":"http://localhost:3000/slow","method":"GET"},"timestamp":106,"wallTime":1700000006,"type":"Fetch"}`)

	got := make([]proto.NetworkRequest, len(d.network))
	for i, n := range d.network {
		got[i] = n.NetworkRequest
	}
	want := []proto.NetworkRequest{
		{ID: 1, Method: "GET", URL: "http://localhost:3000/", Type: "Document", Status: 302, StatusText: "Found", MIMEType: "text/html", Time: 1700000000500, Duration: 250, Size: 120},
		{ID: 2, Method: "GET", URL: "http://localhost:3000/login", Type: "Document", Status: 200, StatusText: "OK", MIMEType: "text/html", Time: 1700000000750, Duration: 250, Size: 4321},
		{ID: 3, Method: "POST", URL: "http://localhost:3000/api/todos", Type: "Fetch", Status: 500, StatusText: "Internal Server Error", MIMEType: "application/json", Time: 1700000001000, Duration: 12.3, Size: 88},
		{ID: 4, Method: "GET", URL: "https://api.other.dev/x", Type: "XHR", Error: "net::ERR_FAILED CORS MissingAllowOriginHeader", Time: 1700000002000, Duration: 500},
		{ID: 5, Method: "GET", URL: "https://ads.dev/a.js", Type: "Script", Error: "net::ERR_BLOCKED_BY_CLIENT blocked: inspector", Time: 1700000003000},
		{ID: 6, Method: "GET", URL: "http://localhost:3000/logo.png", Type: "Image", Time: 1700000005000, FromCache: true, Pending: true},
		{ID: 7, Method: "GET", URL: "http://localhost:3000/slow", Type: "Fetch", Time: 1700000006000, Pending: true},
	}
	if len(got) != len(want) {
		t.Fatalf("%d requests: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d:\n got %+v\nwant %+v", i+1, got[i], want[i])
		}
	}
}

// Where the page speaks no DevTools Protocol, consoleScript keeps what
// its document logs and throws, with where from, and consoleRead reads
// it, then with clear, forgets it.
func TestConsoleScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node")
	}
	harness := `
globalThis.window = globalThis;
class ErrorEvent { constructor(o) { Object.assign(this, o) } }
globalThis.ErrorEvent = ErrorEvent;
const listeners = {};
globalThis.addEventListener = (type, fn) => { (listeners[type] ||= []).push(fn) };
const printed = [];
for (const k of ["log", "info", "debug", "warn", "error", "trace", "assert"]) console[k] = (...a) => printed.push(k === "assert" ? k + ":" + a[0] : k);
eval(CONSOLE);
eval(CONSOLE); // a second run keeps the first's
function logIt() { console.log("%s items", 3, { done: true }) }
logIt();
console.warn("careful");
console.assert(true, "fine");
console.assert(false, "broken");
const err = new TypeError("x is not a function");
listeners.error[0](new ErrorEvent({ message: "Uncaught TypeError: x is not a function", filename: "http://localhost:3000/app.js", lineno: 12, colno: 5, error: err }));
listeners.unhandledrejection[0]({ reason: "nope" });
const first = JSON.parse(eval(READ_CLEAR));
const after = JSON.parse(eval(READ));
process.stdout.write(JSON.stringify({ first, after, printed }));
`
	src := "const CONSOLE = " + jsString(consoleScript) + ";\nconst READ = " + jsString(consoleRead(false)) +
		";\nconst READ_CLEAR = " + jsString(consoleRead(true)) + ";\n" + harness
	out, err := exec.Command(node, "-e", src).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var got struct {
		First   []proto.ConsoleEntry `json:"first"`
		After   []proto.ConsoleEntry `json:"after"`
		Printed []string             `json:"printed"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(got.First) != 5 || len(got.After) != 0 {
		t.Fatalf("kept %+v, then %+v", got.First, got.After)
	}
	log, warn, assert, thrown, rejected := got.First[0], got.First[1], got.First[2], got.First[3], got.First[4]
	// The page's console still prints, once each, as the page called it:
	// the assert that held too.
	if strings.Join(got.Printed, " ") != "log warn assert:true assert:false" {
		t.Errorf("printed %q", got.Printed)
	}
	// Where console.log was called: logIt's line of the script node ran,
	// past the three constants, not consoleScript's own.
	if log.Level != "info" || log.Text != `3 items {"done":true}` || log.URL != "[eval]" || log.Line != 14 || log.Time == 0 {
		t.Errorf("console.log: %+v", log)
	}
	if warn.Level != "warning" || warn.Text != "careful" {
		t.Errorf("console.warn: %+v", warn)
	}
	if assert.Level != "error" || assert.Text != "broken" {
		t.Errorf("console.assert: %+v", assert)
	}
	if thrown.Source != "exception" || thrown.Text != "Uncaught TypeError: x is not a function" || thrown.URL != "http://localhost:3000/app.js" || thrown.Line != 12 || thrown.Column != 5 || len(thrown.Stack) == 0 {
		t.Errorf("uncaught: %+v", thrown)
	}
	if rejected.Source != "rejection" || rejected.Text != "Uncaught (in promise) nope" {
		t.Errorf("rejection: %+v", rejected)
	}
}
