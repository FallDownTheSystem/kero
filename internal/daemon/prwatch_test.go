package daemon

import (
	"strings"
	"testing"
	"time"

	"kero/internal/github"
	"kero/internal/proto"
)

// The watch tells the agent of failing checks at once, even on its first
// look, once per head; of passing checks, conflicts and others' comments
// as pr-watch-wake says; and of comments alone ten times at most.
func TestPRWatched(t *testing.T) {
	link := &proto.SessionPR{Slug: "acme/web", Number: 7}
	now := time.Unix(1000, 0)
	pr := func(head string, checks ...string) github.Watched {
		w := github.Watched{Number: 7, State: "OPEN", HeadOID: head, Base: "main", Mergeable: "MERGEABLE", URL: "https://github.com/acme/web/pull/7"}
		for i, st := range checks {
			w.Checks = append(w.Checks, github.Check{Name: "job" + string(rune('a'+i)), State: st})
		}
		return w
	}

	// Still running: nothing yet.
	_, wt, msg := watched(link, prWatch{}, pr("h1", "success", "pending"), nil, "me", "all", now)
	if msg != "" {
		t.Fatalf("pending checks told: %q", msg)
	}
	// One failed: told, with its name; not again for the same head.
	next, wt, msg := watched(link, wt, pr("h1", "success", "failure"), nil, "me", "failures", now)
	if !strings.Contains(msg, "checks failed") || !strings.Contains(msg, "jobb") || strings.Contains(msg, "joba") {
		t.Fatalf("failure told as %q", msg)
	}
	if next.Checks != "failure" || len(next.Failed) != 1 {
		t.Fatalf("link after failure: %+v", next)
	}
	if _, _, msg = watched(link, wt, pr("h1", "success", "failure"), nil, "me", "all", now); msg != "" {
		t.Fatalf("same failure told again: %q", msg)
	}
	// A new head passes: told with all, not with failures.
	if _, _, msg = watched(link, wt, pr("h2", "success"), nil, "me", "failures", now); msg != "" {
		t.Fatalf("pass told with failures: %q", msg)
	}
	_, wt2, msg := watched(link, wt, pr("h2", "success"), nil, "me", "all", now)
	if !strings.Contains(msg, "passed") {
		t.Fatalf("pass not told: %q", msg)
	}
	// A failure on the very first look is told: the PR was just made.
	if _, _, msg = watched(link, prWatch{}, pr("h1", "failure"), nil, "me", "all", now); !strings.Contains(msg, "checks failed") {
		t.Fatalf("first-look failure not told: %q", msg)
	}
	// But a pass on the first look, and the comments already there, aren't.
	old := pr("h2", "success")
	old.Comments = []github.Comment{{Author: "bob", Body: "old", Created: time.Unix(500, 0)}}
	_, first, msg := watched(link, prWatch{}, old, nil, "me", "all", now)
	if msg != "" {
		t.Fatalf("first look told %q", msg)
	}

	// A conflict: told once, until it merges again.
	conflict := pr("h2", "success")
	conflict.Mergeable = "CONFLICTING"
	_, wt3, msg := watched(link, wt2, conflict, nil, "me", "failures", now)
	if !strings.Contains(msg, "merge conflicts") {
		t.Fatalf("conflict not told: %q", msg)
	}
	if _, _, msg = watched(link, wt3, conflict, nil, "me", "failures", now); msg != "" {
		t.Fatalf("conflict told again: %q", msg)
	}

	// Comments: others' are told, the user's own aren't, nor any with
	// failures; then ten wakes at most for comments alone.
	wt = first
	for i := 1; i <= 12; i++ {
		got := pr("h2", "success")
		at := time.Unix(int64(500+i*10), 0)
		got.Comments = []github.Comment{{Author: "me", Body: "mine", Created: at}}
		lines := []github.LineComment{{Author: "bot", Path: "a.go", Line: 3, Body: "nit " + string(rune('A'+i)), Created: at}}
		if i == 1 {
			if _, _, m := watched(link, wt, got, lines, "me", "failures", now); m != "" {
				t.Fatalf("comment told with failures: %q", m)
			}
		}
		_, wt, msg = watched(link, wt, got, lines, "me", "all", now)
		switch {
		case i <= prWatchMaxWakes && (!strings.Contains(msg, "bot on a.go:3") || strings.Contains(msg, "mine")):
			t.Fatalf("comment %d told as %q", i, msg)
		case i > prWatchMaxWakes && msg != "":
			t.Fatalf("comment %d told past the cap: %q", i, msg)
		}
	}
	// Past the cap, a failure still wakes, with the comments along.
	got := pr("h3", "failure")
	got.Comments = []github.Comment{{Author: "bob", Body: "late", Created: time.Unix(900, 0)}}
	if _, _, msg = watched(link, wt, got, nil, "me", "all", now); !strings.Contains(msg, "checks failed") || !strings.Contains(msg, "bob commented: late") {
		t.Fatalf("failure past the cap told as %q", msg)
	}
}

func TestParsePRURL(t *testing.T) {
	r, n, ok := github.ParsePRURL("https://github.com/acme/web/pull/42/files")
	if !ok || n != 42 || r.Slug() != "acme/web" {
		t.Fatalf("got %v %d %v", r, n, ok)
	}
	for _, bad := range []string{"https://github.com/acme/web/issues/42", "https://gitlab.com/acme/web/pull/42", "42"} {
		if _, _, ok := github.ParsePRURL(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}
