package localbrowser

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"kero/internal/proto"
)

// Many single-page apps keep their sign-in in localStorage or IndexedDB,
// not in cookies. The browser tells a site's storage only to a page of
// the site (DOMStorage and IndexedDB fail with no tab of it open), and its
// IndexedDB commands hand values back as handles of objects in the page,
// so a script reads both, in a world of its own in a tab of the site,
// which the page's own scripts can't reach; the app's page of the site
// writes them back (WriteScript).

var (
	//go:embed read.js
	readJS string
	//go:embed write.js
	writeJS string
)

const (
	// siteLimit bounds a site's databases, as JSON: a mail or chat app
	// keeps hundreds of MB there, which no sign-in needs.
	siteLimit = 8 << 20
	// totalLimit bounds every site's, under a control line's bound
	// (proto.MaxLine), which carries them to the app.
	totalLimit = 24 << 20
	// siteWait is how long a tab may take to answer, or to load.
	siteWait = 15 * time.Second
)

// cdp is a connection to the browser's DevTools.
type cdp struct {
	ws   *websocket.Conn
	last int
}

// cdpError is an error the browser answered a call with.
type cdpError struct{ Message string }

func (e *cdpError) Error() string { return e.Message }

// call calls method, of the target attached as session, or of the browser
// for "", and decodes its result into out, unless nil.
func (c *cdp) call(ctx context.Context, session, method string, params, out any) error {
	c.last++
	id := c.last
	req, err := json.Marshal(struct {
		ID        int    `json:"id"`
		Method    string `json:"method"`
		Params    any    `json:"params,omitempty"`
		SessionID string `json:"sessionId,omitempty"`
	}{id, method, params, session})
	if err != nil {
		return err
	}
	if err := c.ws.Write(ctx, websocket.MessageText, req); err != nil {
		return err
	}
	for {
		_, b, err := c.ws.Read(ctx)
		if err != nil {
			return err
		}
		var res struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *cdpError       `json:"error"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return err
		}
		// Events of the browser come between, and answers of calls that
		// ran out of time.
		if res.ID != id {
			continue
		}
		if res.Error != nil {
			return res.Error
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(res.Result, out)
	}
}

// readSites reads the storage of the sites of the browser's open tabs, of
// domain alone unless "", a tab of each: with no tab of domain open, it
// opens one, unseen, for the time it reads. It tells what it left out.
func readSites(ctx context.Context, c *cdp, domain string) (sites []proto.SiteStorage, notes []string) {
	var targets struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			URL      string `json:"url"`
		} `json:"targetInfos"`
	}
	if err := c.call(ctx, "", "Target.getTargets", nil, &targets); err != nil {
		return nil, []string{"site storage: " + err.Error()}
	}
	seen := map[string]bool{}
	type tab struct{ id, url string }
	var tabs []tab
	for _, t := range targets.TargetInfos {
		origin, host, ok := siteOf(t.URL)
		if t.Type != "page" || !ok || !OfDomain(host, domain) || seen[origin] {
			continue
		}
		seen[origin] = true
		tabs = append(tabs, tab{t.TargetID, origin})
	}
	if len(tabs) == 0 && domain != "" {
		id, err := openTab(ctx, c, "https://"+domain+"/")
		if err != nil {
			return nil, []string{fmt.Sprintf("site storage: opening %s: %v", domain, err)}
		}
		defer func() {
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			c.call(cctx, "", "Target.closeTarget", map[string]any{"targetId": id}, nil)
			cancel()
		}()
		tabs = append(tabs, tab{id, "https://" + domain})
	}
	total := 0
	for _, t := range tabs {
		tctx, cancel := context.WithTimeout(ctx, siteWait)
		site, more, err := readTab(tctx, c, t.id, min(siteLimit, totalLimit-total))
		cancel()
		if ctx.Err() != nil {
			return sites, append(notes, "site storage: "+ctx.Err().Error())
		}
		if err != nil {
			notes = append(notes, t.url+": "+err.Error())
			continue
		}
		for _, n := range more {
			notes = append(notes, site.Origin+": "+n)
		}
		if len(site.LocalStorage) == 0 && len(site.IndexedDB) == 0 {
			continue
		}
		total += len(site.IndexedDB)
		sites = append(sites, *site)
	}
	return sites, notes
}

// readTab reads the storage of the site of a tab, its databases up to
// limit bytes, once it shows a page of a site. It returns what it left
// out.
func readTab(ctx context.Context, c *cdp, target string, limit int) (*proto.SiteStorage, []string, error) {
	var att struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": target, "flatten": true}, &att); err != nil {
		return nil, nil, err
	}
	s := att.SessionID
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		c.call(cctx, "", "Target.detachFromTarget", map[string]any{"sessionId": s}, nil)
		cancel()
	}()
	// eval evaluates expr in a world of Kero's own of the tab's page, which
	// its own scripts don't see; a page's main frame is its target's id.
	eval := func(expr string, out any) error {
		var w struct {
			ID int `json:"executionContextId"`
		}
		if err := c.call(ctx, s, "Page.createIsolatedWorld", map[string]any{"frameId": target, "worldName": "kero-import"}, &w); err != nil {
			return err
		}
		var r struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			ExceptionDetails *struct {
				Text      string `json:"text"`
				Exception *struct {
					Description string `json:"description"`
				} `json:"exception"`
			} `json:"exceptionDetails"`
		}
		err := c.call(ctx, s, "Runtime.evaluate", map[string]any{"expression": expr, "contextId": w.ID, "awaitPromise": true, "returnByValue": true}, &r)
		if err != nil {
			return err
		}
		if e := r.ExceptionDetails; e != nil {
			if e.Exception != nil && e.Exception.Description != "" {
				return fmt.Errorf("%s", e.Exception.Description)
			}
			return fmt.Errorf("%s", e.Text)
		}
		return json.Unmarshal(r.Result.Value, out)
	}
	// A tab opened for the import loads first.
	for {
		var at string
		err := eval(`location.origin + " " + document.readyState`, &at)
		if origin, state, _ := strings.Cut(at, " "); err == nil && state != "loading" && strings.HasPrefix(origin, "http") {
			break
		}
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("the tab didn't answer in %s", siteWait)
		case <-time.After(200 * time.Millisecond):
		}
	}
	var raw string
	if err := eval(readJS+"("+strconv.Itoa(limit)+")", &raw); err != nil {
		return nil, nil, err
	}
	var got struct {
		proto.SiteStorage
		Notes []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return nil, nil, err
	}
	if string(got.IndexedDB) == "[]" {
		got.IndexedDB = nil
	}
	return &got.SiteStorage, got.Notes, nil
}

// openTab opens a tab of url in the browser, in the background, unseen
// where the browser can.
func openTab(ctx context.Context, c *cdp, url string) (string, error) {
	var t struct {
		TargetID string `json:"targetId"`
	}
	err := c.call(ctx, "", "Target.createTarget", map[string]any{"url": url, "background": true, "hidden": true}, &t)
	if _, ok := err.(*cdpError); ok {
		err = c.call(ctx, "", "Target.createTarget", map[string]any{"url": url, "background": true}, &t)
	}
	return t.TargetID, err
}

// siteOf is the origin and host of a page's URL, of a site on the web.
func siteOf(raw string) (origin, host string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", false
	}
	return u.Scheme + "://" + u.Host, strings.ToLower(u.Hostname()), true
}

// OfDomain reports whether host is domain or a subdomain of it, as every
// host is of "".
func OfDomain(host, domain string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), ".")
	return domain == "" || host == domain || strings.HasSuffix(host, "."+domain)
}

// WriteScript is the script that writes a site's storage in a page of the
// site's origin, whose value tells what it wrote, as JSON (Wrote).
func WriteScript(site proto.SiteStorage) (string, error) {
	b, err := json.Marshal(site)
	if err != nil {
		return "", err
	}
	return writeJS + "(" + string(b) + ")", nil
}

// Wrote is what WriteScript's script tells it wrote.
type Wrote struct {
	Items     int      `json:"items"`
	Databases int      `json:"databases"`
	Records   int      `json:"records"`
	Notes     []string `json:"notes"`
}
