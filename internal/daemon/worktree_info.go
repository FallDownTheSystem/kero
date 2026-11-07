package daemon

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
	"kero/internal/settings"
)

// What an agent asks of worktrees (kero worktree list and status), after
// t3code's t3_worktree_list and t3_worktree_status: what git worktree
// list doesn't know. It changes nothing; the cleanup's verdict is what it
// would make of each worktree now.

// worktreeList returns the worktrees of a pane's repository, or a
// folder's: as the History tab lists them, with what removing each would
// lose, and the sessions in each, whether kero made it, and what the
// automatic cleanup makes of it.
func (d *Daemon) worktreeList(p proto.WorktreeListParams) (proto.WorktreeList, error) {
	gw, err := d.gitWorktrees(proto.GitWorktreesParams{Pane: p.Pane, Dir: p.Dir, State: true})
	if err != nil {
		return proto.WorktreeList{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := d.paramsRepo(ctx, proto.GitParams{Pane: p.Pane, Dir: p.Dir})
	if err != nil {
		return proto.WorktreeList{}, err
	}
	main := r.Main()
	res := proto.WorktreeList{Main: main, Default: gw.Default, Worktrees: make([]proto.WorktreeInfo, len(gw.Worktrees))}
	for i, w := range gw.Worktrees {
		res.Worktrees[i] = proto.WorktreeInfo{GitWorktree: w}
	}
	d.worktreeSessions(res.Worktrees)

	set := settings.Load()
	roots := []string{}
	if main != "" {
		roots = cleanupRoots(main, set.WorktreeDir)
	}
	for i := range res.Worktrees {
		w := &res.Worktrees[i]
		w.Kero = w.Session != "" || (!w.Main && inRoots(w.Path, roots))
	}
	repos, _ := d.cleanupRepos(set)
	var cr *cleanupRepo
	for m, c := range repos {
		if main != "" && sameDir(m, main) {
			cr = c
		}
	}
	switch {
	case cr == nil:
		res.Cleanup = "no project"
	case cr.keep:
		res.Cleanup = "off for a project of it"
	case !cr.rules.Removes():
		res.Cleanup = "off"
	default:
		res.Cleanup = "on"
		seen, now, base := d.loadCleanupSeen(), time.Now(), cr.r.DefaultRef()
		gh := &cleanupGitHub{}
		for i := range res.Worktrees {
			w := &res.Worktrees[i]
			co := git.Checkout{Path: w.Path, Head: w.Head, Branch: w.Branch, Prunable: w.Prunable, Locked: w.Locked}
			w.Removes, w.Keeps = d.cleanupVerdict(ctx, cr, co, roots, base, seen, now, gh)
		}
	}
	return res, nil
}

// inRoots reports whether the worktree at path is in one of roots, kero's
// worktree folders.
func inRoots(path string, roots []string) bool {
	for _, root := range roots {
		if sameDir(filepath.Dir(path), root) {
			return true
		}
	}
	return false
}

// worktreeSessions sets the sessions working in each of wts: a pane in
// it, of the deepest that holds it, as the main checkout holds the
// others; and the session each was made for, with its branch's base.
func (d *Daemon) worktreeSessions(wts []proto.WorktreeInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.sessions {
		in := map[int]bool{}
		for _, pn := range s.panes {
			pn.mu.Lock()
			dir := pn.info.WorkDir()
			pn.mu.Unlock()
			at := -1
			for i := range wts {
				if proto.Within(dir, wts[i].Path) && (at < 0 || len(wts[i].Path) > len(wts[at].Path)) {
					at = i
				}
			}
			if at >= 0 {
				in[at] = true
			}
		}
		for i := range wts {
			if s.worktree != nil && sameDir(s.worktree.Path, wts[i].Path) {
				wts[i].Session, wts[i].Base = s.id, s.worktree.Base
				in[i] = true
			}
			if in[i] {
				wts[i].Sessions = append(wts[i].Sessions, s.id)
			}
		}
	}
}

// worktreeStatus returns how a pane's session stands with its worktree:
// where it works, the worktree kero made for it and its setup, its pull
// request, the settings a new worktree goes by, and whether kero worktree
// handoff would take it.
func (d *Daemon) worktreeStatus(p proto.WorktreeStatusParams) (proto.WorktreeStatus, error) {
	pn := d.findPane(p.Pane)
	if pn == nil {
		return proto.WorktreeStatus{}, fmt.Errorf("no pane %q", p.Pane)
	}
	d.mu.Lock()
	s := d.sessionOfLocked(pn)
	if s == nil {
		d.mu.Unlock()
		return proto.WorktreeStatus{}, errors.New("the session closed")
	}
	info := d.sessionInfo(s)
	setups := d.setupsLocked()
	pn.mu.Lock()
	dir, terminal := pn.info.WorkDir(), pn.info.Terminal()
	var as proto.AgentSession
	if pn.info.AgentSession != nil {
		as = *pn.info.AgentSession
	}
	id := pn.info.ID
	pn.mu.Unlock()
	d.mu.Unlock()

	set := settings.Load()
	st := proto.WorktreeStatus{Session: info.ID, Pane: id, Dir: dir, PR: info.PR,
		FromOrigin: set.WorktreeFromOrigin, WorktreeBase: set.WorktreeBase, WorktreeDir: set.WorktreeDir}
	if info.Worktree != nil {
		own := *info.Worktree
		st.Own = &own
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if r, err := git.Open(ctx, dir); err == nil {
		st.Root, st.Linked, st.Main, st.Default = r.Root, r.Linked(), r.Main(), r.DefaultBranch()
		if gs, err := r.Status(ctx); err == nil {
			st.Branch = gs.Branch
		}
	}
	// The setup of the session's worktree, else of the one it works in:
	// the last to start.
	for i := range setups {
		su := &setups[i]
		if (st.Own != nil && sameDir(su.Path, st.Own.Path)) || (st.Root != "" && sameDir(su.Path, st.Root)) {
			if st.Setup == nil || su.Started > st.Setup.Started {
				st.Setup = su
			}
		}
	}
	// As handoff refuses.
	switch {
	case st.Own != nil:
		st.Handoff = "the session works in a worktree of its own already"
	case st.Linked:
		st.Handoff = "the pane works in a linked worktree already"
	case st.Root == "":
		st.Handoff = "the pane works in no git repository"
	case !terminal || !resumable(&as):
		st.Handoff = "the pane runs no agent's conversation that kero resumes"
	}
	return st, nil
}
