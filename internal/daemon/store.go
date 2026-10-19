package daemon

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"kero/internal/proto"
	"kero/internal/settings"
)

// The daemon keeps its projects and sessions in state.json, and each
// session's screen and scrollback in sessions/<id>.vt, so that after the
// daemon itself ends, as when the Mac restarts, the next one starts every
// session again where it was, with what it showed.

type savedState struct {
	Version  int             `json:"version"`
	Projects []proto.Project `json:"projects"`
	Sessions []savedSession  `json:"sessions"`
	Saved    int64           `json:"saved"`
}

type savedSession struct {
	ID      string        `json:"id"`
	Project string        `json:"project"`
	Name    string        `json:"name,omitempty"`
	Created int64         `json:"created"`
	Layout  *proto.Layout `json:"layout,omitempty"`
	Focus   string        `json:"focus,omitempty"`
	Panes   []savedPane   `json:"panes"`
}

type savedPane struct {
	ID      string   `json:"id"`
	Dir     string   `json:"dir,omitempty"`
	Command []string `json:"command,omitempty"`
	Created int64    `json:"created"`
	Cols    int      `json:"cols"`
	Rows    int      `json:"rows"`
}

func (d *Daemon) statePath() string             { return filepath.Join(d.dir, "state.json") }
func (d *Daemon) snapshotPath(id string) string { return filepath.Join(d.dir, "sessions", id+".vt") }

func (d *Daemon) saveLoop() {
	snaps := time.NewTicker(10 * time.Second)
	defer snaps.Stop()
	for {
		select {
		case <-d.saveCh:
			time.Sleep(300 * time.Millisecond)
			d.saveState()
		case <-snaps.C:
			d.saveSnapshots(false)
		case <-d.quit:
			return
		}
	}
}

func (d *Daemon) saveState() {
	d.mu.Lock()
	st := savedState{Version: 2, Projects: d.projects, Saved: time.Now().UnixMilli()}
	for _, s := range d.sessions {
		ss := savedSession{ID: s.id, Project: s.project, Name: s.name, Created: s.created, Layout: s.layout, Focus: s.focus}
		for _, p := range s.panes {
			p.mu.Lock()
			ss.Panes = append(ss.Panes, savedPane{ID: p.info.ID, Dir: p.info.Dir, Command: p.info.Command,
				Created: p.info.Created, Cols: p.info.Cols, Rows: p.info.Rows})
			p.mu.Unlock()
		}
		st.Sessions = append(st.Sessions, ss)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	d.mu.Unlock()
	if err != nil {
		return
	}
	writeAtomic(d.statePath(), data)
}

func (d *Daemon) saveSnapshots(force bool) {
	d.mu.Lock()
	panes := make([]*pane, 0, len(d.panes))
	for _, p := range d.panes {
		panes = append(panes, p)
	}
	d.mu.Unlock()
	for _, p := range panes {
		if snap := p.snapshot(force); snap != nil {
			writeAtomic(d.snapshotPath(p.info.ID), snap)
		}
	}
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// restoreReset leaves what a program set up in the snapshot, as an
// agent's full-screen interface with the mouse, so that the new shell
// starts on the main screen with the modes it expects.
const restoreReset = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1006l\x1b[?1015l" + // no mouse, focus reports
	"\x1b[?2004l\x1b[?2026l\x1b[=0;1u\x1b[>4;0m" + // no bracketed paste, sync, kitty keys, modifyOtherKeys
	"\x1b[!p\x1b[?25h\x1b[0m" // a soft reset, a visible cursor

// restore starts again the sessions of the last daemon, which ended
// without closing them.
func (d *Daemon) restore() {
	data, err := os.ReadFile(d.statePath())
	if err != nil {
		return
	}
	var st savedState
	if json.Unmarshal(data, &st) != nil {
		return
	}
	if !settings.Load().RestoreSessions {
		// The projects stay; the sessions of the last daemon do not.
		d.projects = st.Projects
		return
	}
	d.projects = st.Projects
	when := time.UnixMilli(st.Saved).Format("Mon Jan 2 15:04")
	for _, ss := range st.Sessions {
		s := &session{id: ss.ID, project: ss.Project, name: ss.Name, created: ss.Created, restored: true, focus: ss.Focus}
		for _, sp := range ss.Panes {
			if p := d.restorePane(s.id, sp, when); p != nil {
				s.panes = append(s.panes, p)
				d.panes[p.info.ID] = p
			}
		}
		if len(s.panes) == 0 {
			continue
		}
		if sameLeaves(ss.Layout, s.panes) {
			s.layout = ss.Layout
		} else {
			for _, p := range s.panes[1:] {
				if s.layout == nil {
					s.layout = &proto.Layout{Pane: s.panes[0].info.ID}
				}
				s.layout = s.layout.Split(s.panes[0].info.ID, p.info.ID, "right", 0.5)
			}
		}
		d.sessions = append(d.sessions, s)
		d.log.Printf("session %s restored, %d panes", s.id, len(s.panes))
	}
}

// restorePane starts a pane of the last daemon again, under its
// scrollback, in its directory.
func (d *Daemon) restorePane(sid string, sp savedPane, when string) *pane {
	now := time.Now().UnixMilli()
	p, err := newPane(d, sid, proto.Pane{
		ID: sp.ID, Dir: sp.Dir, Command: sp.Command, Status: proto.Idle, Since: now,
		Created: sp.Created, Cols: sp.Cols, Rows: sp.Rows, Restored: true,
	})
	if err != nil {
		d.log.Printf("restoring %s: %v", sp.ID, err)
		return nil
	}
	if snap, err := os.ReadFile(d.snapshotPath(sp.ID)); err == nil && len(snap) > 0 {
		p.term.Feed(snap)
		if bytes.Contains(snap, []byte("\x1b[?1049h")) || bytes.Contains(snap, []byte("\x1b[?1047h")) {
			// Back to the main screen, and its cursor, from a
			// full-screen program's.
			p.term.Feed([]byte("\x1b[?1049l"))
		}
		p.term.Feed([]byte(restoreReset + "\r\n\x1b[2m── restored · session ended " + when + " ──\x1b[0m\r\n"))
	}
	// The shell alone: what ran in it, as an agent, may not want to run
	// again unasked.
	if err := p.start(nil, nil); err != nil {
		d.log.Printf("restoring %s: %v", sp.ID, err)
		p.term.Close()
		return nil
	}
	p.dirty = true
	return p
}
