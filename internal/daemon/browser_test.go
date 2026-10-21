package daemon

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"kero/internal/proto"
)

func testDaemon(t *testing.T) *Daemon {
	t.Helper()
	t.Setenv("KERO_CONFIG", filepath.Join(t.TempDir(), "config"))
	d := &Daemon{dir: t.TempDir(), log: log.New(io.Discard, "", 0), panes: map[string]*pane{},
		changeCh: make(chan struct{}, 1), saveCh: make(chan struct{}, 1)}
	d.viewed.Store(new(string))
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: t.TempDir()}}
	return d
}

// A browser pane runs no shell: it keeps its page, as the app tells it,
// which a restore brings back, and the terminal's calls leave it be.
func TestBrowserPane(t *testing.T) {
	d := testDaemon(t)
	s, err := d.create(proto.CreateParams{URL: "localhost:3000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Panes) != 1 || s.Panes[0].URL != "http://localhost:3000" {
		t.Fatalf("panes %+v", s.Panes)
	}
	p := d.findPane(s.Panes[0].ID)
	if p.term != nil || p.proc != nil {
		t.Fatal("a browser pane has a terminal")
	}
	// What a terminal's pane takes, a browser's ignores.
	p.resize(80, 24, 8, 16)
	p.clear(proto.ClearScreen)
	if p.tick(time.Now()) {
		t.Error("a browser pane ticked a change")
	}
	if _, _, _, ok := p.attach(newAttachment(nil)); ok {
		t.Error("attached to a browser pane")
	}
	if p.restart() == nil {
		t.Error("restarted a browser pane")
	}
	if h := p.history(true); h != nil {
		t.Error("a browser pane has a history")
	}

	if err := p.setPage("http://localhost:3000/docs", "Docs"); err != nil {
		t.Fatal(err)
	}
	if got := p.readText(false, 0); got != "Docs\nhttp://localhost:3000/docs\n" {
		t.Errorf("read %q", got)
	}
	d.saveState()

	r := testDaemon(t)
	r.dir = d.dir
	r.restore()
	if len(r.sessions) != 1 || len(r.sessions[0].panes) != 1 {
		t.Fatalf("restored %d sessions", len(r.sessions))
	}
	info := r.sessions[0].panes[0].info
	if info.URL != "http://localhost:3000/docs" || info.Title != "Docs" || !info.Restored {
		t.Errorf("restored %+v", info)
	}
	if err := r.removeSession(r.sessions[0].id); err != nil {
		t.Fatal(err)
	}

	// A terminal's pane has no page to set.
	tp := testPane(t)
	if tp.setPage("https://kero.sh", "") == nil {
		t.Error("set a terminal's page")
	}
}
