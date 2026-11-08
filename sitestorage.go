package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"kero/internal/localbrowser"
	"kero/internal/proto"
)

// siteWait is how long a page of a site takes to show, and to write the
// site's storage.
const siteWait = 10 * time.Second

// storagePage is the document of the page that writes a site's storage:
// the app hands it the page at the site's URL (Page.LoadHTML), so it has
// the site's origin, and its storage, and runs nothing of the site's.
const storagePage = "<!doctype html><title>Kero</title>"

// loadStorage gives the browser panes the storage of sites, which kero
// browser cookies import took of the user's browser: a page of each
// site's origin, which no one sees, writes it, in the panes' session. Off
// the main thread, as it waits on the page. It returns a line of what it
// wrote, and a line of each thing it left out.
func (a *App) loadStorage(sites []proto.SiteStorage) []string {
	if len(sites) == 0 {
		return nil
	}
	made := make(chan *mygo.WebView, 1)
	a.update(func() {
		var v *mygo.WebView
		if a.win != nil {
			v, _ = a.win.NewWebView(mygo.WebViewOptions{})
		}
		made <- v
	})
	var view *mygo.WebView
	select {
	case view = <-made:
	case <-time.After(siteWait):
	}
	if view == nil {
		return []string{"site storage: this window shows no pages"}
	}
	defer a.update(view.Destroy)
	page := view.Page()
	var n localbrowser.Wrote
	var wrote int
	var notes []string
	for _, s := range sites {
		w, err := writeSite(page, s)
		if err != nil {
			notes = append(notes, s.Origin+": "+err.Error())
			continue
		}
		wrote++
		n.Items += w.Items
		n.Databases += w.Databases
		n.Records += w.Records
		for _, note := range w.Notes {
			notes = append(notes, s.Origin+": "+note)
		}
	}
	line := fmt.Sprintf("loaded the storage of %d of %d sites: %d localStorage items, %d IndexedDB databases with %d records",
		wrote, len(sites), n.Items, n.Databases, n.Records)
	return append([]string{line}, notes...)
}

// writeSite has the page go to the site's origin and write its storage.
func writeSite(page *mygo.Page, s proto.SiteStorage) (*localbrowser.Wrote, error) {
	script, err := localbrowser.WriteScript(s)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), siteWait)
	defer cancel()
	page.LoadHTML(storagePage, s.Origin+"/")
	// The page is at the origin once it committed there.
	for {
		if at, err := page.EvalContext(ctx, "location.origin"); err == nil && at == s.Origin {
			break
		}
		select {
		case <-ctx.Done():
			return nil, errors.New("no page of the site showed")
		case <-time.After(100 * time.Millisecond):
		}
	}
	v, err := page.EvalContext(ctx, script)
	if err != nil {
		var js *mygo.EvalError
		if errors.As(err, &js) {
			return nil, errors.New(strings.TrimPrefix(js.Message, "Error: "))
		}
		return nil, err
	}
	text, _ := v.(string)
	var w localbrowser.Wrote
	if err := json.Unmarshal([]byte(text), &w); err != nil {
		return nil, fmt.Errorf("the page answered %q", text)
	}
	return &w, nil
}
