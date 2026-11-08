package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"kero/internal/proto"
	"kero/internal/settings"
)

// devtoolsCmd prints the DevTools port of the app's browser panes, which
// browser-devtools opens on Windows, and each pane's page there: the
// endpoint is what Playwright's connectOverCDP and chrome-devtools-mcp's
// --browserUrl take, and a pane's target what a tool picks it by, as
// tools see every pane as a page of the one browser.
func devtoolsCmd(st proto.State, args []string) error {
	f, err := parse(args, nil, map[string]string{"json": "json"})
	if err != nil {
		return err
	}
	if len(f.pos) > 0 || len(f.rest) > 0 {
		return usageErr("kero browser devtools [--json]")
	}
	if runtime.GOOS != "windows" {
		return errors.New("the DevTools port is Windows only: WebView2 opens one, and the system's web view elsewhere does not")
	}
	set := settings.Load()
	var ports []int
	if p := activePort(webViewData()); p > 0 {
		ports = append(ports, p)
	}
	if set.DevTools && set.DevToolsPort > 0 {
		ports = append(ports, set.DevToolsPort)
	}
	dt, err := findDevTools(st, ports)
	if err != nil {
		if !set.DevTools {
			return errors.New("the DevTools port is off: turn on DevTools port in Kero's Settings › Browser, or set browser-devtools = true, then quit Kero and start it again")
		}
		return fmt.Errorf("no DevTools port answers (%v): quit Kero and start it again, as the port opens as Kero starts; with browser-devtools-port taken by another program, choose another, or 0", err)
	}
	if f.set["json"] {
		return printJSON(dt)
	}
	fmt.Println(dt.Endpoint)
	for _, p := range dt.Panes {
		target := p.Target
		if target == "" {
			target = "(no page yet: show the pane)"
		}
		title := p.Title
		if title == "" {
			title = "-"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", p.Pane, target, p.URL, title)
	}
	return nil
}

// webViewData is the folder of the WebView2 browser's data, as MyGo makes
// it: WEBVIEW2_USER_DATA_FOLDER, else %LOCALAPPDATA%\<app>\WebView2.
func webViewData() string {
	if d := os.Getenv("WEBVIEW2_USER_DATA_FOLDER"); d != "" {
		return d
	}
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, mygo.App.Name(), "WebView2")
}

// activePort is the port the browser of a WebView2 data folder listens
// on, as it writes it in DevToolsActivePort as it opens it, which tells
// the port it picked for 0; 0 for none.
func activePort(dir string) int {
	for _, p := range []string{filepath.Join(dir, "EBWebView", "DevToolsActivePort"), filepath.Join(dir, "DevToolsActivePort")} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		var n int
		if sc.Scan() {
			n, _ = strconv.Atoi(strings.TrimSpace(sc.Text()))
		}
		f.Close()
		if n > 0 && n <= 65535 {
			return n
		}
	}
	return 0
}

// cdpTarget is a target as a DevTools port lists it at /json/list.
type cdpTarget struct {
	ID, Type, Title, URL string
	WebSocket            string `json:"webSocketDebuggerUrl"`
}

// findDevTools asks each port in turn for its browser and its pages, and
// gives the first that answers, with the browser panes of st matched to
// its pages.
func findDevTools(st proto.State, ports []int) (*proto.DevTools, error) {
	if len(ports) == 0 {
		return nil, errors.New("no port")
	}
	hc := &http.Client{Timeout: 2 * time.Second}
	get := func(url string, v any) error {
		res, err := hc.Get(url)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: %s", url, res.Status)
		}
		return json.NewDecoder(res.Body).Decode(v)
	}
	var first error
	for _, port := range ports {
		dt := &proto.DevTools{Endpoint: "http://127.0.0.1:" + strconv.Itoa(port), Port: port}
		var version struct {
			WebSocket string `json:"webSocketDebuggerUrl"`
		}
		var targets []cdpTarget
		err := get(dt.Endpoint+"/json/version", &version)
		if err == nil {
			err = get(dt.Endpoint+"/json/list", &targets)
		}
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		dt.Browser = version.WebSocket
		dt.Panes = devtoolsPanes(st, targets)
		return dt, nil
	}
	return nil, first
}

// devtoolsPanes are the browser panes of st, each with the page of the
// port that shows it: the one of its URL and title, else of its URL, each
// page given once, in the order the port lists them. kerod knows a
// pane's page as the app last told it, so a page that just went elsewhere
// matches once the app tells.
func devtoolsPanes(st proto.State, targets []cdpTarget) []proto.DevToolsPane {
	panes := []proto.DevToolsPane{}
	for _, s := range st.Sessions {
		for _, p := range s.Panes {
			if p.Browser() {
				panes = append(panes, proto.DevToolsPane{Pane: p.ID, Session: s.ID, URL: p.URL, Title: p.Title})
			}
		}
	}
	taken := make([]bool, len(targets))
	// The panes of a page's URL and title first, so that a pane whose
	// title is stale takes none of theirs.
	for _, byTitle := range []bool{true, false} {
		for i := range panes {
			p := &panes[i]
			if p.Target != "" {
				continue
			}
			for j, t := range targets {
				if !taken[j] && t.Type == "page" && t.URL == p.URL && (!byTitle || t.Title == p.Title) {
					taken[j] = true
					p.Target, p.WebSocket = t.ID, t.WebSocket
					break
				}
			}
		}
	}
	return panes
}
