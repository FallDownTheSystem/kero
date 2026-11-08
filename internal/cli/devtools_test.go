package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kero/internal/proto"
)

// kero browser devtools finds the port that answers, and gives each
// browser pane the page of the port that shows it: by its URL and title,
// so that two panes of one URL get a page each, and a pane whose title is
// stale takes no page of another's title. A pane with no page yet, and
// the browser's other pages, get none.
func TestDevToolsPanes(t *testing.T) {
	targets := []map[string]string{
		{"id": "T-app", "type": "page", "url": "http://localhost:5173/", "title": "Vite App"},
		{"id": "T-a", "type": "page", "url": "https://kero.sh/", "title": "Kero"},
		{"id": "T-b", "type": "page", "url": "https://kero.sh/", "title": "Docs"},
		{"id": "T-worker", "type": "service_worker", "url": "https://go.dev/", "title": ""},
		{"id": "T-other", "type": "page", "url": "https://example.com/", "title": "Other"},
	}
	for _, tg := range targets {
		tg["webSocketDebuggerUrl"] = "ws://127.0.0.1/devtools/page/" + tg["id"]
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			json.NewEncoder(w).Encode(map[string]string{"Browser": "Edg/142.0", "webSocketDebuggerUrl": "ws://127.0.0.1/devtools/browser/B"})
		case "/json/list":
			json.NewEncoder(w).Encode(targets)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	// A port nothing listens on, as a stale DevToolsActivePort tells.
	gone := httptest.NewServer(http.NotFoundHandler())
	dead, _ := strconv.Atoi(gone.URL[strings.LastIndex(gone.URL, ":")+1:])
	gone.Close()

	st := proto.State{Sessions: []proto.Session{
		{ID: "s1", Panes: []proto.Pane{
			{ID: "t1"},
			// Stale title: listed first, it must not take T-a, the first page of
			// its URL, which b3's title is.
			{ID: "b1", URL: "https://kero.sh/", Title: "Kero — old"},
			{ID: "b2", URL: "http://localhost:5173/", Title: "Vite App"},
		}},
		{ID: "s2", Panes: []proto.Pane{
			{ID: "b3", URL: "https://kero.sh/", Title: "Kero"},
			{ID: "b4", URL: "https://go.dev/", Title: "Go"},
		}},
	}}
	dt, err := findDevTools(st, []int{dead, port})
	if err != nil {
		t.Fatal(err)
	}
	if dt.Port != port || dt.Endpoint != "http://127.0.0.1:"+strconv.Itoa(port) || dt.Browser != "ws://127.0.0.1/devtools/browser/B" {
		t.Errorf("endpoint %q, port %d, browser %q", dt.Endpoint, dt.Port, dt.Browser)
	}
	want := map[string]string{"b1": "T-b", "b2": "T-app", "b3": "T-a", "b4": ""}
	if len(dt.Panes) != len(want) {
		t.Fatalf("panes %+v", dt.Panes)
	}
	for _, p := range dt.Panes {
		if p.Target != want[p.Pane] {
			t.Errorf("pane %s: target %q, want %q", p.Pane, p.Target, want[p.Pane])
		}
		if p.Target != "" && p.WebSocket != "ws://127.0.0.1/devtools/page/"+p.Target {
			t.Errorf("pane %s: websocket %q", p.Pane, p.WebSocket)
		}
		if p.Pane == "b3" && p.Session != "s2" {
			t.Errorf("pane b3: session %q", p.Session)
		}
	}

	if _, err := findDevTools(st, []int{dead}); err == nil {
		t.Error("a port nothing listens on answered")
	}
}

// The port WebView2's browser opened, as it writes it in its data folder,
// which tells the port it picked for 0.
func TestActivePort(t *testing.T) {
	dir := t.TempDir()
	if p := activePort(dir); p != 0 {
		t.Errorf("no file: %d", p)
	}
	os.MkdirAll(filepath.Join(dir, "EBWebView"), 0o755)
	os.WriteFile(filepath.Join(dir, "EBWebView", "DevToolsActivePort"), []byte("53127\r\n/devtools/browser/0b1c\r\n"), 0o644)
	if p := activePort(dir); p != 53127 {
		t.Errorf("port %d, want 53127", p)
	}
}
