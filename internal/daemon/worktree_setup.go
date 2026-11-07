package daemon

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
)

// Setting up a worktree kero just made (proto.WorktreeSetup): its
// submodules, then the repository's setup script, the first of kero.json's
// scripts, else t3.json's, as t3code names them, that runs on a
// worktree's creation. It runs in a terminal of the session that works in
// the worktree, below its agent, beside it, or before it when the script
// says so ("async": false); it closes as the script succeeds, and stays
// with its output when it fails. The app shows each setup as a progress
// card, until a while after it ends, and may cancel it.

// setupFiles are the files that may name a setup script, in the order
// they are read: the worktree's first, as committed, then the main
// checkout's, which may keep one of its own.
var setupFiles = []string{"kero.json", "t3.json"}

// setupScript is a script that runs as a worktree is made.
type setupScript struct {
	Name    string
	Command string
	// Hold: the agent waits for the script to end.
	Hold bool
}

// setupFile is what kero.json, and t3.json, say: scripts, of which the
// first with runOnWorktreeCreate sets up a worktree.
type setupFile struct {
	Scripts []struct {
		Name                string `json:"name"`
		Command             string `json:"command"`
		RunOnWorktreeCreate bool   `json:"runOnWorktreeCreate"`
		Async               *bool  `json:"async"`
	} `json:"scripts"`
}

// loadSetupScript finds the setup script of a worktree in dirs, its own
// first: nil for none.
func loadSetupScript(dirs ...string) (*setupScript, error) {
	for _, dir := range dirs {
		for _, name := range setupFiles {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			var f setupFile
			if err := json.Unmarshal(b, &f); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			for _, s := range f.Scripts {
				cmd := strings.TrimSpace(s.Command)
				if !s.RunOnWorktreeCreate || cmd == "" {
					continue
				}
				return &setupScript{Name: cmp.Or(strings.TrimSpace(s.Name), name), Command: cmd, Hold: s.Async != nil && !*s.Async}, nil
			}
		}
	}
	return nil, nil
}

// worktreeSetup is a setup under way, or that ended a short while ago.
type worktreeSetup struct {
	info   proto.WorktreeSetup // d.mu
	script *setupScript
	env    []string
	ctx    context.Context
	cancel context.CancelFunc
	// held is the agent's pane, which waits for the script (Hold), as
	// create would have started it (setupFirst); d.mu.
	held *proto.CreateParams
}

// setupWait is how long a setup waits for a session to work in its
// worktree, which create starts at once.
const setupWait = time.Minute

// setUpWorktree sets the worktree at path up, which addWorktree just made
// of the repository whose main checkout is main, after fetch: its
// submodules, here, then its setup script, in the background, in the
// terminal of the session that comes to work in it. Without either, it
// does nothing.
func (d *Daemon) setUpWorktree(main, path, branch string, fetch originFetch) {
	subs := git.HasSubmodules(path)
	script, loadErr := loadSetupScript(path, main)
	if !subs && script == nil && loadErr == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &worktreeSetup{script: script, ctx: ctx, cancel: cancel}
	t.info = proto.WorktreeSetup{
		ID: newID(), Path: path, Branch: branch, Main: main,
		Phase: proto.SetupRunning, Started: time.Now().UnixMilli(),
		Stages: []proto.SetupStage{
			{ID: proto.SetupFetch, Status: proto.SetupSkipped},
			{ID: proto.SetupCheckout, Status: proto.SetupDone},
			{ID: proto.SetupSubmodules, Status: proto.SetupSkipped},
			{ID: proto.SetupScript, Status: proto.SetupSkipped},
			{ID: proto.SetupAgent, Status: proto.SetupPending},
		},
	}
	if script != nil {
		t.info.Script, t.info.Command = script.Name, script.Command
		t.env = []string{
			"KERO_PROJECT_ROOT=" + main, "KERO_WORKTREE_PATH=" + path, "KERO_WORKTREE_BRANCH=" + branch,
			// As t3code tells its scripts.
			"T3CODE_PROJECT_ROOT=" + main, "T3CODE_WORKTREE_PATH=" + path,
		}
		t.stage(proto.SetupScript, proto.SetupPending, "")
	}
	if loadErr != nil {
		t.stage(proto.SetupScript, proto.SetupFailed, loadErr.Error())
	}
	switch {
	case fetch.Err != nil:
		t.stage(proto.SetupFetch, proto.SetupWarning, fetch.Err.Error())
	case fetch.Ref != "":
		t.stage(proto.SetupFetch, proto.SetupDone, fetch.Ref)
	}
	d.mu.Lock()
	if d.setups == nil {
		d.setups = map[string]*worktreeSetup{}
	}
	d.setups[t.info.ID] = t
	d.mu.Unlock()
	d.changed()
	if subs {
		d.setStage(t, proto.SetupSubmodules, proto.SetupRunning, "")
		err := git.UpdateSubmodules(ctx, path)
		switch {
		case ctx.Err() != nil:
			d.setStage(t, proto.SetupSubmodules, proto.SetupSkipped, proto.SetupCancelled)
		case err != nil:
			d.log.Printf("submodules of %s: %v", path, err)
			d.setStage(t, proto.SetupSubmodules, proto.SetupWarning, err.Error())
		default:
			d.setStage(t, proto.SetupSubmodules, proto.SetupDone, "")
		}
	}
	go d.runSetup(t)
}

// stage sets a stage of t; d.mu is held, or t is t's alone.
func (t *worktreeSetup) stage(id, status, detail string) {
	for i := range t.info.Stages {
		if t.info.Stages[i].ID == id {
			t.info.Stages[i].Status, t.info.Stages[i].Detail = status, detail
		}
	}
}

// setStage sets a stage of t, and tells the apps.
func (d *Daemon) setStage(t *worktreeSetup, id, status, detail string) {
	d.mu.Lock()
	t.stage(id, status, detail)
	d.mu.Unlock()
	d.changed()
}

// setupFirst is the pane create starts a session in a worktree with, of
// params p: p's, but for an agent that waits for the worktree's setup
// script, whose terminal it starts instead, the agent's to follow it.
func (d *Daemon) setupFirst(path string, p proto.CreateParams) proto.CreateParams {
	if p.URL != "" || p.File != "" {
		return p
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.setups {
		if t.info.Path != path || !t.info.Running() || t.script == nil || !t.script.Hold || t.held != nil || t.ctx.Err() != nil {
			continue
		}
		held := p
		t.held = &held
		return proto.CreateParams{Command: []string{t.script.Command}, Env: t.env, Cols: p.Cols, Rows: p.Rows}
	}
	return p
}

// runSetup runs t's setup script in the session that comes to work in its
// worktree, then ends t.
func (d *Daemon) runSetup(t *worktreeSetup) {
	sid, first := d.setupSession(t)
	if sid == "" {
		d.endSetup(t, "no session works in the worktree")
		return
	}
	d.mu.Lock()
	t.info.Session = sid
	held := t.held
	d.mu.Unlock()
	// An agent that waits has the script's terminal: it starts as that
	// ends, cancelled too.
	if held == nil && (t.script == nil || t.ctx.Err() != nil) {
		d.setStage(t, proto.SetupAgent, proto.SetupDone, "")
		d.endSetup(t, "")
		return
	}
	pane := first
	if held == nil {
		d.setStage(t, proto.SetupAgent, proto.SetupDone, "")
		var err error
		if pane, err = d.splitSetup(sid, first, t); err != nil {
			d.setStage(t, proto.SetupScript, proto.SetupFailed, err.Error())
			d.endSetup(t, "")
			return
		}
	}
	d.mu.Lock()
	t.info.Pane = pane
	t.stage(proto.SetupScript, proto.SetupRunning, t.script.Command)
	d.mu.Unlock()
	d.changed()
	d.log.Printf("setup script of %s runs in pane %s: %s", t.info.Path, pane, t.script.Command)
	code, err := d.waitCommand(t.ctx, pane)
	ok := false
	switch {
	case t.ctx.Err() != nil:
		d.setStage(t, proto.SetupScript, proto.SetupSkipped, proto.SetupCancelled)
	case errors.Is(err, errUntold):
		// cmd.exe doesn't tell when the script ends: it runs on.
		d.setStage(t, proto.SetupScript, proto.SetupDone, err.Error())
	case err != nil:
		d.setStage(t, proto.SetupScript, proto.SetupFailed, err.Error())
	case code != 0:
		d.setStage(t, proto.SetupScript, proto.SetupFailed, fmt.Sprintf("exit %d", code))
	default:
		ok = true
		d.setStage(t, proto.SetupScript, proto.SetupDone, "")
	}
	if held != nil {
		// The agent starts even after a script that failed, which it
		// may see to; the script's terminal stays, with what it said.
		d.setStage(t, proto.SetupAgent, proto.SetupRunning, "")
		if err := d.startHeld(sid, pane, *held); err != nil {
			d.setStage(t, proto.SetupAgent, proto.SetupFailed, err.Error())
		} else {
			d.setStage(t, proto.SetupAgent, proto.SetupDone, "")
		}
	}
	// Cancelling stops the script, as closing its terminal does.
	if ok || t.ctx.Err() != nil && !errors.Is(err, errClosed) {
		d.removePane(pane)
	}
	d.endSetup(t, "")
}

// setupSession waits for a session to work in t's worktree, as create
// starts it right after the worktree is made, and returns its id and
// first pane's: "" for none in setupWait.
func (d *Daemon) setupSession(t *worktreeSetup) (sid, first string) {
	deadline := time.Now().Add(setupWait)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		for _, s := range d.sessions {
			in := s.worktree != nil && filepath.Clean(s.worktree.Path) == filepath.Clean(t.info.Path)
			for _, pn := range s.panes {
				pn.mu.Lock()
				in = in || pn.info.Terminal() && proto.Within(pn.info.WorkDir(), t.info.Path)
				pn.mu.Unlock()
			}
			if in && len(s.panes) > 0 {
				sid, first = s.id, s.panes[0].info.ID
				break
			}
		}
		d.mu.Unlock()
		if sid != "" {
			return sid, first
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", ""
}

// splitSetup starts t's script in a terminal below the pane at, in
// session sid, the focus staying where it was.
func (d *Daemon) splitSetup(sid, at string, t *worktreeSetup) (string, error) {
	d.mu.Lock()
	s := d.findSessionLocked(sid)
	focus := ""
	if s != nil {
		focus = s.focus
	}
	d.mu.Unlock()
	info, err := d.split(proto.CreateParams{
		Split: at, Direction: "down", Ratio: 0.3, Dir: t.info.Path,
		Command: []string{t.script.Command}, Env: t.env,
	})
	if err != nil {
		return "", err
	}
	pane := info.Focus
	d.mu.Lock()
	if s != nil && focus != "" && s.focus == pane {
		s.focus = focus
	}
	d.mu.Unlock()
	d.changed()
	return pane, nil
}

// startHeld starts the agent's pane that waited for the setup script in
// pane setup, above it, as the session's first, where it would have been.
func (d *Daemon) startHeld(sid, setup string, p proto.CreateParams) error {
	d.mu.Lock()
	s := d.findSessionLocked(sid)
	var dir string
	if at := d.panes[setup]; at != nil {
		dir = at.info.Dir
	}
	d.mu.Unlock()
	if s == nil {
		return errors.New("the session closed")
	}
	pn, err := d.startPane(sid, p, dir)
	if err != nil {
		return err
	}
	d.mu.Lock()
	if !slices.Contains(d.sessions, s) {
		d.mu.Unlock()
		pn.close()
		return errors.New("the session closed")
	}
	at := setup
	if !slices.ContainsFunc(s.panes, func(o *pane) bool { return o.info.ID == setup }) {
		at = s.panes[0].info.ID
	}
	if s.layout == nil {
		s.layout = &proto.Layout{Pane: at}
	}
	s.layout = s.layout.Split(at, pn.info.ID, "up", 0.7)
	s.panes = slices.Insert(s.panes, 0, pn)
	s.focus, s.zoomed = pn.info.ID, ""
	d.panes[pn.info.ID] = pn
	d.mu.Unlock()
	d.changed()
	return nil
}

var (
	// errUntold: the pane's shell doesn't tell when its command ends.
	errUntold = errors.New("its end isn't told by the shell")
	errClosed = errors.New("its terminal closed")
)

// waitCommand waits for the command pane id started with to end, and
// returns its exit status.
func (d *Daemon) waitCommand(ctx context.Context, id string) (int, error) {
	d.mu.Lock()
	pn := d.panes[id]
	d.mu.Unlock()
	if pn == nil {
		return 0, errClosed
	}
	for {
		pn.mu.Lock()
		info, ch, closed, awaiting := pn.info, pn.changed, pn.closed, pn.awaiting
		pn.mu.Unlock()
		switch {
		case info.CommandExit != nil:
			return *info.CommandExit, nil
		case info.Status == proto.Exited:
			return info.ExitCode, nil
		case closed:
			return 0, errClosed
		case !awaiting:
			return 0, errUntold
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

// endSetup ends t, failed with why when not "", and forgets it a while
// after: soon when all went well.
func (d *Daemon) endSetup(t *worktreeSetup, why string) {
	d.mu.Lock()
	cancelled := t.ctx.Err() != nil
	for i := range t.info.Stages {
		if st := &t.info.Stages[i]; st.Status == proto.SetupPending || st.Status == proto.SetupRunning {
			st.Status = proto.SetupSkipped
		}
	}
	phase := proto.SetupDone
	for _, st := range t.info.Stages {
		if st.Status == proto.SetupFailed {
			phase = proto.SetupFailed
		}
	}
	switch {
	case cancelled:
		phase = proto.SetupCancelled
	case why != "":
		phase, t.info.Error = proto.SetupFailed, why
	}
	t.info.Phase, t.info.Ended = phase, time.Now().UnixMilli()
	id := t.info.ID
	d.mu.Unlock()
	t.cancel()
	d.changed()
	d.log.Printf("setup of %s %s", t.info.Path, phase)
	keep := 30 * time.Second
	if phase == proto.SetupDone {
		keep = 5 * time.Second
	}
	time.AfterFunc(keep, func() { d.forgetSetup(id) })
}

// forgetSetup drops a setup that ended from the state.
func (d *Daemon) forgetSetup(id string) {
	d.mu.Lock()
	t := d.setups[id]
	if t != nil && !t.info.Running() {
		delete(d.setups, id)
	}
	d.mu.Unlock()
	if t != nil {
		d.changed()
	}
}

// cancelSetup cancels a setup that runs, which stops its script, or
// forgets one that ended, as its card closes.
func (d *Daemon) cancelSetup(id string) error {
	d.mu.Lock()
	t := d.setups[id]
	d.mu.Unlock()
	if t == nil {
		return fmt.Errorf("no setup %q", id)
	}
	if !t.info.Running() {
		d.forgetSetup(id)
		return nil
	}
	t.cancel()
	return nil
}

// setupsLocked are the setups for the state; d.mu is held.
func (d *Daemon) setupsLocked() []proto.WorktreeSetup {
	var out []proto.WorktreeSetup
	for _, t := range d.setups {
		info := t.info
		info.Stages = slices.Clone(info.Stages)
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b proto.WorktreeSetup) int { return int(a.Started - b.Started) })
	return out
}
