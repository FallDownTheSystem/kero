// Package localbrowser reads the cookies of the user's own browser, Chrome
// or Edge, as it runs: browser panes are a web view of their own, which
// the user's sign-ins are not in, and can be given them.
//
// The browser's cookie files can't be read: from Chrome 127 (Edge too) they
// are encrypted with a key only the browser itself may unlock, on Windows.
// Nor will it start with --remote-debugging-port on its own profile, from
// Chrome 136. What it does allow, from Chrome 144, is remote debugging the
// user turns on as it runs (chrome://inspect/#remote-debugging), which then
// asks the user to allow each program that connects. Cookies connects so,
// and asks the browser for its cookies over the DevTools protocol.
package localbrowser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/coder/websocket"

	"kero/internal/proto"
)

// Names are the browsers Dir knows by name.
var Names = []string{"chrome", "edge"}

// Dir is the user data directory of the browser named, chrome or edge, on
// this system, or the directory given, which a browser started with
// --user-data-dir has.
func Dir(name string) (string, error) {
	home, _ := os.UserHomeDir()
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = filepath.Join(home, "AppData", "Local")
	}
	var dir string
	switch lower := strings.ToLower(name); {
	case lower == "chrome" && runtime.GOOS == "windows":
		dir = filepath.Join(local, "Google", "Chrome", "User Data")
	case lower == "edge" && runtime.GOOS == "windows":
		dir = filepath.Join(local, "Microsoft", "Edge", "User Data")
	case lower == "chrome" && runtime.GOOS == "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	case lower == "edge" && runtime.GOOS == "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "Microsoft Edge")
	case lower == "chrome":
		dir = filepath.Join(configDir(home), "google-chrome")
	case lower == "edge":
		dir = filepath.Join(configDir(home), "microsoft-edge")
	default:
		// A directory of its own.
		if st, err := os.Stat(name); err != nil || !st.IsDir() {
			return "", fmt.Errorf("no browser %q: chrome, edge, or a browser's user data directory", name)
		}
		return name, nil
	}
	return dir, nil
}

func configDir(home string) string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	return filepath.Join(home, ".config")
}

// Title is how a browser of Dir is named to the user.
func Title(name string) string {
	switch strings.ToLower(name) {
	case "chrome":
		return "Chrome"
	case "edge":
		return "Edge"
	}
	return name
}

// inspectPage is the page that turns on remote debugging in the browser.
func inspectPage(name string) string {
	if strings.ToLower(name) == "edge" {
		return "edge://inspect/#remote-debugging"
	}
	return "chrome://inspect/#remote-debugging"
}

// endpoint is the browser's DevTools address, which it writes to
// DevToolsActivePort in its user data directory while remote debugging is
// on: the port, then the browser's path.
func endpoint(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "DevToolsActivePort"))
	if err != nil {
		return "", err
	}
	lines := strings.Fields(string(b))
	if len(lines) < 2 {
		return "", fmt.Errorf("DevToolsActivePort holds no address: %q", b)
	}
	port, err := strconv.Atoi(lines[0])
	if err != nil || port <= 0 || port > 65535 {
		return "", fmt.Errorf("DevToolsActivePort holds no port: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "/devtools/browser/") {
		return "", fmt.Errorf("DevToolsActivePort holds no browser path: %q", lines[1])
	}
	return "ws://127.0.0.1:" + strconv.Itoa(port) + lines[1], nil
}

// cdpCookie is a cookie as the DevTools protocol has it (Network.Cookie).
type cdpCookie struct {
	Name         string          `json:"name"`
	Value        string          `json:"value"`
	Domain       string          `json:"domain"`
	Path         string          `json:"path"`
	Expires      float64         `json:"expires"`
	HTTPOnly     bool            `json:"httpOnly"`
	Secure       bool            `json:"secure"`
	Session      bool            `json:"session"`
	SameSite     string          `json:"sameSite"`
	PartitionKey json.RawMessage `json:"partitionKey"`
}

// Import is what Read took of the user's browser.
type Import struct {
	Cookies []proto.Cookie
	// Sites are the storage of the sites of the browser's open tabs.
	Sites []proto.SiteStorage
	// Notes tell what was left out, and why.
	Notes []string
}

// Read connects to the browser of the user data directory dir, which asks
// the user to allow it, and returns its cookies, those of the profile the
// browser deems the default, and the storage of the sites of its open tabs
// (readSites): of domain and its subdomains alone, unless domain is "".
// name is the browser's, for the error that tells how to turn remote
// debugging on. A cookie of a partition (CHIPS), which a pane would send
// to every site, is left out.
func Read(ctx context.Context, name, dir, domain string) (*Import, error) {
	off := func(err error) error {
		return fmt.Errorf("%s doesn't let Kero connect (%v): open %s as it runs, turn on remote debugging there, and try again", Title(name), err, inspectPage(name))
	}
	url, err := endpoint(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, off(errors.New("its remote debugging is off"))
	}
	if err != nil {
		return nil, err
	}
	// The browser holds the connection until the user allows it: ctx is
	// how long they may take.
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s did not let Kero connect in time: allow it in the browser's dialog, which may show in another of its windows, an installed app's (PWA) too", Title(name))
		}
		return nil, off(err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(256 << 20) // every site's cookies come in one message
	c := &cdp{ws: ws}
	var res struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	if err := c.call(ctx, "", "Storage.getCookies", nil, &res); err != nil {
		var ce *cdpError
		if errors.As(err, &ce) {
			return nil, fmt.Errorf("%s: %w", Title(name), err)
		}
		return nil, browserGone(name, ctx, err)
	}
	imp := &Import{Cookies: convert(res.Cookies)}
	imp.Sites, imp.Notes = readSites(ctx, c, domain)
	ws.Close(websocket.StatusNormalClosure, "")
	return imp, nil
}

// browserGone tells why the connection ended: the user denied it, which
// closes it, or ctx ran out.
func browserGone(name string, ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%s did not answer in time: allow Kero in the browser's dialog, which may show in another of its windows, an installed app's (PWA) too", Title(name))
	}
	return fmt.Errorf("%s closed the connection (%v): allow Kero in the browser's dialog", Title(name), err)
}

// convert turns the browser's cookies into the panes', as Playwright's
// storage state has them.
func convert(in []cdpCookie) []proto.Cookie {
	out := make([]proto.Cookie, 0, len(in))
	for _, c := range in {
		if len(c.PartitionKey) > 0 && string(c.PartitionKey) != "null" {
			continue
		}
		pc := proto.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Expires: c.Expires,
			HTTPOnly: c.HTTPOnly, Secure: c.Secure, SameSite: c.SameSite}
		if c.Session || pc.Expires <= 0 {
			pc.Expires = -1
		}
		if pc.Path == "" {
			pc.Path = "/"
		}
		// A cookie set with no SameSite the browser treats as Lax.
		if pc.SameSite != "Strict" && pc.SameSite != "None" {
			pc.SameSite = "Lax"
		}
		out = append(out, pc)
	}
	return out
}
