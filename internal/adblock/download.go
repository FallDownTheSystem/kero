package adblock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Source is a filter list kero downloads.
type Source struct {
	Name, URL string
}

// Sources are the lists browser panes block what they say: EasyList's
// ads, and EasyPrivacy's trackers.
var Sources = []Source{
	{"easylist", "https://easylist.to/easylist/easylist.txt"},
	{"easyprivacy", "https://easylist.to/easylist/easyprivacy.txt"},
}

// Refresh is how old a list downloaded gets before it is downloaded again.
const Refresh = 7 * 24 * time.Hour

// Load returns the filters of the sources, downloaded into dir: a list
// is downloaded when dir has none, or one older than Refresh, and a list
// that cannot be downloaded is the one dir has, if any.
func Load(ctx context.Context, dir string, sources []Source) ([]string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	var lists []string
	var errs []error
	for _, s := range sources {
		path := filepath.Join(dir, s.Name+".txt")
		info, statErr := os.Stat(path)
		if statErr != nil || time.Since(info.ModTime()) > Refresh {
			var since time.Time
			if statErr == nil {
				since = info.ModTime()
			}
			if err := download(ctx, s.URL, path, since); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", s.Name, err))
			}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue // not downloaded, which errs tells
		}
		lists = append(lists, string(b))
	}
	if len(lists) == 0 {
		return nil, errors.Join(errs...)
	}
	return lists, nil
}

// download saves url's body at path, unless it did not change since a
// copy of that time, which then counts as downloaded now.
func download(ctx context.Context, url, path string, since time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if !since.IsZero() {
		req.Header.Set("If-Modified-Since", since.UTC().Format(http.TimeFormat))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	now := time.Now()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return os.Chtimes(path, now, now)
	case http.StatusOK:
	default:
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	// Not a captive portal's page, say.
	if !strings.HasPrefix(string(body), "[Adblock Plus") {
		return fmt.Errorf("%s is no filter list", url)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
