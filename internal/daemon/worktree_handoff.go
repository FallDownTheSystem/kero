package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
	"kero/internal/settings"
)

// A session's handoff into a worktree of its own, as its agent asks with
// kero worktree handoff (after t3code's t3_worktree_handoff): kerod makes
// the worktree as kero new --worktree does (addWorktree) and makes it the
// session's; once the agent's turn ends, it starts the agent's pane again
// in the worktree with the agent's own resume, so that the conversation
// goes on there, and types the prompt it was given to go on with.

// handoffWait is how long a handoff waits for the agent's turn to end:
// one still working after it, as its screen tells, moves all the same,
// which a misread screen would otherwise hold back for good.
const handoffWait = 15 * time.Minute

// handoffQuiet is how long the agent stays idle, and its screen still,
// for its turn to have ended: a turn's tool call can look idle a moment.
const handoffQuiet = 1500 * time.Millisecond

// handoffs are the panes a handoff is under way of, by id.
var handoffs sync.Map

// handoff makes a worktree for the session of p.Pane, and has its agent
// move into it once its turn ends (moveWhenIdle).
func (d *Daemon) handoff(p proto.HandoffParams) (proto.HandoffResult, error) {
	pn := d.findPane(p.Pane)
	if pn == nil {
		return proto.HandoffResult{}, fmt.Errorf("no session or pane %q", p.Pane)
	}
	d.mu.Lock()
	s := d.sessionOfLocked(pn)
	if s == nil {
		d.mu.Unlock()
		return proto.HandoffResult{}, errors.New("the session closed")
	}
	if s.worktree != nil {
		wt := s.worktree.Path
		d.mu.Unlock()
		return proto.HandoffResult{}, fmt.Errorf("session %s works in a worktree of its own already, %s", s.id, proto.ShortDir(wt))
	}
	sid := s.id
	pn.mu.Lock()
	var as proto.AgentSession
	if pn.info.AgentSession != nil {
		as = *pn.info.AgentSession
	}
	dir, terminal := pn.info.WorkDir(), pn.info.Terminal()
	linked := ""
	if g := pn.info.Git; g != nil && g.Linked {
		linked = g.Root
	}
	pn.mu.Unlock()
	d.mu.Unlock()
	if linked != "" {
		// As t3code's handoff: one worktree a session, which the agent
		// works on in place (kero worktree status).
		return proto.HandoffResult{}, fmt.Errorf("pane %s works in the worktree %s already: go on there, or kero new --worktree starts a session in another", pn.info.ID, proto.ShortDir(linked))
	}
	if !terminal || !resumable(&as) {
		return proto.HandoffResult{}, fmt.Errorf("pane %s runs no agent's conversation that kero resumes: kero new --worktree starts a session in a worktree instead", pn.info.ID)
	}
	if _, busy := handoffs.LoadOrStore(pn.info.ID, true); busy {
		return proto.HandoffResult{}, fmt.Errorf("pane %s is moving into a worktree already", pn.info.ID)
	}
	moving := false
	defer func() {
		if !moving {
			handoffs.Delete(pn.info.ID)
		}
	}()

	// The folder of the repository the agent works in, to start it in the
	// same of the worktree.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	rel := ""
	r, err := git.Open(ctx, dir)
	cancel()
	if err == nil {
		if rp, err := filepath.Rel(r.Root, dir); err == nil && rp != "." && !strings.HasPrefix(rp, "..") {
			rel = rp
		}
	}
	base := p.Base
	if base == "" && settings.Load().WorktreeBase == "current" {
		base = "HEAD"
	}
	wt, err := d.addWorktree(dir, proto.CreateParams{Worktree: p.Branch, Base: base})
	if err != nil {
		return proto.HandoffResult{}, err
	}
	to := wt.Path
	if rel != "" {
		if st, err := os.Stat(filepath.Join(wt.Path, rel)); err == nil && st.IsDir() {
			to = filepath.Join(wt.Path, rel)
		}
	}

	d.mu.Lock()
	gone := !slices.Contains(d.sessions, s) || !slices.Contains(s.panes, pn)
	other := s.worktree != nil
	if !gone && !other {
		s.worktree = wt
	}
	d.mu.Unlock()
	if gone || other {
		// Nothing is in it yet: it goes as it came.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := git.RemoveWorktree(ctx, wt.Main, wt.Path, wt.Branch, wt.Base); err != nil {
			d.log.Printf("removing worktree %s: %v", wt.Path, err)
		}
		if gone {
			return proto.HandoffResult{}, errors.New("the session closed")
		}
		return proto.HandoffResult{}, fmt.Errorf("session %s works in a worktree of its own already", sid)
	}
	d.changed()
	moving = true
	go d.moveWhenIdle(pn, as, *wt, to, strings.TrimSpace(p.Prompt))
	d.log.Printf("session %s hands off to worktree %s", sid, wt.Path)
	return proto.HandoffResult{Worktree: *wt, Session: sid, Pane: pn.info.ID, Dir: to}, nil
}

// moveWhenIdle starts the agent of pn again in dir, of the worktree wt,
// once its turn ended, resuming its conversation as; then types prompt,
// once it waits for one.
func (d *Daemon) moveWhenIdle(pn *pane, as proto.AgentSession, wt proto.Worktree, dir, prompt string) {
	defer handoffs.Delete(pn.info.ID)
	if !d.awaitQuiet(pn, handoffWait, false) {
		return
	}
	note := fmt.Sprintf("Moved to the worktree %s, on %s", proto.ShortDir(wt.Path), wt.Branch)
	err := pn.relaunch(dir, resumeCommand(&as), note, func() {
		// Claude Code keeps a conversation in the folder of the
		// directory it ran in, and resumes it from the one it runs in.
		if as.Agent == "claude" {
			if err := moveClaudeTranscript(as.ID, dir); err != nil {
				d.log.Printf("moving claude conversation %s to %s: %v", as.ID, dir, err)
			}
		}
	})
	if err != nil {
		d.log.Printf("pane %s moving to %s: %v", pn.info.ID, dir, err)
		return
	}
	d.changed()
	d.log.Printf("pane %s resumes %s conversation %s in %s", pn.info.ID, as.Agent, as.ID, dir)
	if prompt == "" {
		return
	}
	// The agent starts, and draws its conversation, before it takes one.
	if !d.awaitQuiet(pn, 2*time.Minute, true) {
		return
	}
	pn.paste(prompt)
	go pn.submit(time.Now())
}

// awaitQuiet waits until pn is idle, or in error, with a still screen
// (handoffQuiet), and with agent an agent's that Kero knows runs in it;
// or until timeout, which is quiet enough for a handoff. false: the pane
// closed, or kerod ends.
func (d *Daemon) awaitQuiet(pn *pane, timeout time.Duration, agent bool) bool {
	end := time.After(timeout)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
		case <-end:
			return !agent
		case <-d.quit:
			return false
		}
		pn.mu.Lock()
		info, closed := pn.info, pn.closed
		pn.mu.Unlock()
		if closed {
			return false
		}
		now := time.Now()
		still := now.Sub(time.UnixMilli(info.Activity)) >= handoffQuiet && now.Sub(time.UnixMilli(info.Since)) >= handoffQuiet
		if (info.Status == proto.Idle || info.Status == proto.Failed) && still && (!agent || info.Agent != "") {
			return true
		}
	}
}

// relaunch ends the pane's shell, with what runs in it, and starts one
// in dir that runs command, under a note in the pane's history; between
// the two, once the program ended, then runs.
func (s *pane) relaunch(dir string, command []string, note string, then func()) error {
	s.mu.Lock()
	if s.closed || s.term == nil {
		s.mu.Unlock()
		return errors.New("the pane closed")
	}
	proc := s.proc
	// Its end is the pane's no more (exited), and the agent's session
	// goes on in the next (tick).
	s.proc = nil
	s.dropLive()
	s.progress = ""
	s.info.Process, s.info.Agent, s.info.Runtime, s.info.ProcessDir = "", "", "", ""
	s.info.Dir, s.info.Command = dir, command
	s.seen, s.looked = "", false
	s.term.Feed([]byte("\r\n\x1b[0;2m[" + note + "]\x1b[0m\r\n"))
	s.frame()
	s.mu.Unlock()
	if proc != nil {
		proc.Close()
		select {
		case <-proc.exited:
		case <-time.After(3 * time.Second):
			proc.Kill()
			select {
			case <-proc.exited:
			case <-time.After(2 * time.Second):
			}
		}
	}
	if then != nil {
		then()
	}
	return s.start(command, nil)
}

// moveClaudeTranscript moves Claude Code's transcript of conversation id,
// with the folder of its subagents' and tools' beside it, to the folder
// it keeps dir's conversations in.
func moveClaudeTranscript(id, dir string) error {
	path, _, err := claudeTranscript(id)
	if err != nil {
		return err
	}
	name := claudeProject(dir)
	if len(name) > 200 {
		// Claude shortens it with a hash of its own.
		return fmt.Errorf("%s is too long a path to move a conversation to", dir)
	}
	to := filepath.Join(filepath.Dir(filepath.Dir(path)), name)
	if filepath.Clean(to) == filepath.Dir(path) {
		return nil
	}
	if err := os.MkdirAll(to, 0o700); err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(to, id+".jsonl")); err != nil {
		return err
	}
	side := strings.TrimSuffix(path, ".jsonl")
	if st, err := os.Stat(side); err == nil && st.IsDir() {
		return os.Rename(side, filepath.Join(to, id))
	}
	return nil
}

// claudeProject is the name of Claude Code's folder of the conversations
// of dir, under its projects: dir with a dash for each character but an
// ASCII letter or digit.
func claudeProject(dir string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, dir)
}
