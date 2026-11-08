package cli

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kero/internal/proto"
)

// kero browser drives a browser pane: the one named, or a session's,
// which with no target is the session the command runs in, though its
// pane is the terminal it runs in.
func TestBrowserTarget(t *testing.T) {
	t.Setenv("KERO_CONFIG", filepath.Join(t.TempDir(), "config"))
	st := proto.State{Sessions: []proto.Session{
		{ID: "s1", Focus: "t1", Panes: []proto.Pane{{ID: "t1"}, {ID: "b1", URL: "http://localhost:3000"}}},
		{ID: "s2", Focus: "b3", Panes: []proto.Pane{{ID: "b2", URL: "https://kero.sh"}, {ID: "b3", URL: "https://go.dev"}}},
		{ID: "s3", Focus: "t3", Panes: []proto.Pane{{ID: "t3"}, {ID: "b4", URL: "https://a.dev"}, {ID: "b5", URL: "https://b.dev"}}},
		{ID: "s4", Focus: "t4", Panes: []proto.Pane{{ID: "t4"}}},
	}}
	for _, tc := range []struct {
		ref, want string
		err       error
	}{
		{"s1", "b1", nil},       // its one browser, though a terminal has the focus
		{"b1", "b1", nil},       // a pane named
		{"s2", "b3", nil},       // the focused browser of several
		{"b2", "b2", nil},       // the other, named
		{"s3", "", errNotFound}, // several, none focused: name one
		{"s4", "", errNotFound}, // none
		{"t1", "", errNotFound}, // a terminal named
	} {
		tg, err := find(st, tc.ref)
		if err != nil {
			t.Fatal(err)
		}
		got, err := browserOf(tg)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("%s: %q, %v; want %q, %v", tc.ref, got, err, tc.want, tc.err)
		}
	}

	// Run in a terminal pane, a command names no target: its session's
	// browser is meant. Bad usage is found before kerod is asked.
	t.Setenv("KERO_SESSION", "s1")
	t.Setenv("KERO_PANE", "t1")
	for _, args := range [][]string{
		{},
		{"s1"},
		{"bogus"},
		{"click"},
		{"type"},
		{"open"},
		{"text", "h1", "h2"},
		{"reload", "now"},
		{"text", "-o", "x.png"},
		{"click", "a", "--enter"},
		{"s1", "eval"},
		{"cookies"},
		{"cookies", "import"},
		{"cookies", "export", "chrome"},
		{"console", "now"},
		{"console", "--failed"},
		{"text", "--clear"},
		{"screenshot", "--touch"},
		{"network", "body"},
		{"network", "body", "x"},
		{"network", "headers", "3"},
		{"network", "-o", "x.txt"},
		{"emulate", "phone"},
		{"emulate", "device", "nokia-3310"},
		{"emulate", "viewport", "big"},
		{"emulate", "viewport", "390x"},
		{"emulate", "network", "5g"},
		{"emulate", "reset", "now"},
		{"emulate", "network", "3g", "--scale", "2"},
		{"emulate", "viewport", "390x844", "--scale", "0"},
	} {
		err := browserCmd(nil, st, args)
		if !errors.Is(err, errUsage) && !(len(args) == 1 && args[0] == "bogus" && errors.Is(err, errNotFound)) {
			t.Errorf("kero browser %q: %v", args, err)
		}
	}
	t.Setenv("KERO_SESSION", "s4")
	t.Setenv("KERO_PANE", "t4")
	if err := browserCmd(nil, st, []string{"text"}); !errors.Is(err, errNotFound) {
		t.Errorf("a session with no browser: %v", err)
	}
}

// Cookies come from a storage state, a list of its cookies, or a
// cookies.txt, and go back to one as they came.
func TestCookieFiles(t *testing.T) {
	txt := "# Netscape HTTP Cookie File\n" +
		"#HttpOnly_localhost\tFALSE\t/\tFALSE\t0\tsess\tabc\n" +
		"example.com\tTRUE\t/app\tTRUE\t2000000000\tpref\tdark=1\n" +
		"\n# a comment\n" +
		"empty.dev\tFALSE\t/\tFALSE\t0\tblank\n"
	want := []proto.Cookie{
		{Name: "sess", Value: "abc", Domain: "localhost", Path: "/", Expires: -1, HTTPOnly: true, SameSite: "Lax"},
		{Name: "pref", Value: "dark=1", Domain: ".example.com", Path: "/app", Expires: 2000000000, Secure: true, SameSite: "Lax"},
		{Name: "blank", Domain: "empty.dev", Path: "/", Expires: -1, SameSite: "Lax"},
	}
	got, err := parseCookies([]byte(txt))
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("cookies.txt: %+v, %v", got, err)
	}
	if back, err := parseCookies(netscapeCookies(want)); err != nil || !slices.Equal(back, want) {
		t.Errorf("cookies.txt again: %+v, %v", back, err)
	}
	state := `{"cookies":[{"name":"a","value":"1","domain":".x.dev","path":"/","expires":-1,"httpOnly":true,"secure":true,"sameSite":"None"}],"origins":[]}`
	got, err = parseCookies([]byte(state))
	if err != nil || len(got) != 1 || got[0].Domain != ".x.dev" || got[0].SameSite != "None" || !got[0].HTTPOnly {
		t.Errorf("storage state: %+v, %v", got, err)
	}
	if got, err = parseCookies([]byte(`[{"name":"a","value":"1","domain":"x.dev"}]`)); err != nil || len(got) != 1 {
		t.Errorf("a list: %+v, %v", got, err)
	}
	if _, err := parseCookies([]byte("not\ta cookie\n")); err == nil {
		t.Error("a line of no cookie read")
	}
}

// The console prints a line an entry, as a console shows it; the
// network a line a request, with why one failed.
func TestDevtoolsOutput(t *testing.T) {
	var b strings.Builder
	printConsole(&b, []proto.ConsoleEntry{
		{Level: "info", Source: "console", Text: "ready", URL: "http://localhost:3000/main.js", Line: 3, Column: 9},
		{Level: "error", Source: "exception", Text: "Uncaught TypeError: x is not a function", URL: "http://localhost:3000/app.js", Line: 12, Column: 5,
			Stack: []string{"go (http://localhost:3000/app.js:12:5)"}},
		{Level: "warning", Source: "deprecation", Text: "two\nlines", URL: "http://x.dev/"},
	})
	want := "[info] ready  (http://localhost:3000/main.js:3:9)\n" +
		"[exception] Uncaught TypeError: x is not a function  (http://localhost:3000/app.js:12:5)\n" +
		"    at go (http://localhost:3000/app.js:12:5)\n" +
		"[warning deprecation] two  (http://x.dev/)\n    lines\n"
	if b.String() != want {
		t.Errorf("console:\n%s\nwant:\n%s", b.String(), want)
	}

	b.Reset()
	printNetwork(&b, []proto.NetworkRequest{
		{ID: 1, Method: "GET", URL: "http://localhost:3000/", Type: "Document", Status: 200, Duration: 85.4, Size: 12345},
		{ID: 12, Method: "POST", URL: "https://api.dev/x", Type: "Fetch", Error: "net::ERR_FAILED CORS MissingAllowOriginHeader", Duration: 1500},
		{ID: 13, Method: "GET", URL: "http://localhost:3000/slow", Type: "XHR", Pending: true},
	})
	want = "#1   GET   200     Document  12.3 kB  85 ms    http://localhost:3000/\n" +
		"#12  POST  failed  Fetch              1.50 s   https://api.dev/x  net::ERR_FAILED CORS MissingAllowOriginHeader\n" +
		"#13  GET   …       XHR                pending  http://localhost:3000/slow\n"
	if b.String() != want {
		t.Errorf("network:\n%q\nwant:\n%q", b.String(), want)
	}

	for s, want := range map[string][2]int{"390x844": {390, 844}, "1280×800": {1280, 800}, "390X844": {390, 844}, "x844": {}, "0x10": {}, "390": {}} {
		w, h, ok := parseViewport(s)
		if [2]int{w, h} != want || ok != (want != [2]int{}) {
			t.Errorf("viewport %q: %d, %d, %v", s, w, h, ok)
		}
	}
}
