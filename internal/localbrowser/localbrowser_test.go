package localbrowser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kero/internal/proto"
)

// fakeBrowser serves the DevTools protocol's browser endpoint as Chrome
// does, at the path a DevToolsActivePort in a new user data directory
// names, and answers Storage.getCookies with cookies after an event.
func fakeBrowser(t *testing.T, cookies string) string {
	t.Helper()
	const path = "/devtools/browser/0b5e-41"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		// Chrome refuses a connection from a page, which has an Origin.
		if r.Header.Get("Origin") != "" {
			http.Error(w, "origin", http.StatusForbidden)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		_, b, err := c.Read(r.Context())
		if err != nil {
			return
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.Unmarshal(b, &req)
		c.Write(r.Context(), websocket.MessageText, []byte(`{"method":"Target.targetCreated","params":{}}`))
		if req.Method != "Storage.getCookies" {
			c.Write(r.Context(), websocket.MessageText, []byte(`{"id":`+itoa(req.ID)+`,"error":{"message":"no method"}}`))
			return
		}
		c.Write(r.Context(), websocket.MessageText, []byte(`{"id":`+itoa(req.ID)+`,"result":{"cookies":`+cookies+`}}`))
		c.Read(r.Context())
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	if err := os.WriteFile(filepath.Join(dir, "DevToolsActivePort"), []byte(port+"\n"+path), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// The browser's cookies come as the panes take them: a session cookie
// lasts until the app quits, one with no SameSite goes as Lax, and one of
// a partition, which a pane would send to every site, stays behind.
func TestCookies(t *testing.T) {
	dir := fakeBrowser(t, `[
		{"name":"sid","value":"a=b","domain":".github.com","path":"/","expires":1893456000.5,"httpOnly":true,"secure":true,"session":false,"sameSite":"None"},
		{"name":"pref","value":"dark","domain":"docs.github.com","path":"","expires":-1,"session":true},
		{"name":"csrf","value":"x","domain":"app.dev","path":"/api","expires":1893456000,"secure":true,"session":false,"sameSite":"Strict"},
		{"name":"embed","value":"y","domain":"widget.dev","path":"/","expires":1893456000,"secure":true,"session":false,"sameSite":"None","partitionKey":{"topLevelSite":"https://app.dev","hasCrossSiteAncestor":true}}
	]`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := Cookies(ctx, "chrome", dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []proto.Cookie{
		{Name: "sid", Value: "a=b", Domain: ".github.com", Path: "/", Expires: 1893456000.5, HTTPOnly: true, Secure: true, SameSite: "None"},
		{Name: "pref", Value: "dark", Domain: "docs.github.com", Path: "/", Expires: -1, SameSite: "Lax"},
		{Name: "csrf", Value: "x", Domain: "app.dev", Path: "/api", Expires: 1893456000, Secure: true, SameSite: "Strict"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d cookies, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cookie %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// With remote debugging off there is no DevToolsActivePort, and with the
// browser closed since, nothing listens at the port it names: either way
// the error tells where to turn it on, Edge's page for Edge.
func TestCookiesOff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Cookies(ctx, "edge", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "edge://inspect/#remote-debugging") {
		t.Errorf("no file: %v", err)
	}
	stale := t.TempDir()
	l := httptest.NewServer(http.NotFoundHandler())
	port := l.URL[strings.LastIndex(l.URL, ":")+1:]
	l.Close()
	os.WriteFile(filepath.Join(stale, "DevToolsActivePort"), []byte(port+"\n/devtools/browser/x\n"), 0o644)
	_, err = Cookies(ctx, "chrome", stale)
	if err == nil || !strings.Contains(err.Error(), "chrome://inspect/#remote-debugging") {
		t.Errorf("stale file: %v", err)
	}
}

// A user data directory of its own is taken as it is; a name that is
// neither a browser nor a directory is refused.
func TestDir(t *testing.T) {
	own := t.TempDir()
	if d, err := Dir(own); err != nil || d != own {
		t.Errorf("Dir(%q) = %q, %v", own, d, err)
	}
	if _, err := Dir("netscape"); err == nil {
		t.Error("Dir(netscape) found a browser")
	}
	for _, n := range Names {
		if d, err := Dir(n); err != nil || d == "" {
			t.Errorf("Dir(%s) = %q, %v", n, d, err)
		}
	}
}
