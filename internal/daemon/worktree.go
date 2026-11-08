package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
	"kero/internal/settings"
)

// Sessions in a worktree of their own (plans/git.md, Phase 3): kerod adds
// a linked worktree on a new branch as such a session starts, and removes
// it as the session closes, when the user asks and nothing would be lost
// that they didn't agree to.

// worktreeDir is the folder kero puts a repository's worktrees in, by
// default: inside the repository, which its info/exclude ignores.
const worktreeDir = ".kero/worktrees"

// addWorktree makes a session's own worktree, of the repository holding
// dir, on branch from base ("" the default branch, "HEAD" dir's).
func (d *Daemon) addWorktree(dir string, p proto.CreateParams) (*proto.Worktree, error) {
	branch, base := p.Worktree, p.Base
	if dir == "" {
		return nil, errors.New("a worktree needs a directory in a git repository")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := git.Open(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("%s is in no git repository", proto.ShortDir(dir))
	}
	main := r.Main()
	if main == "" {
		return nil, errors.New("kero makes worktrees of a repository with a main checkout, not a bare one's")
	}
	branch = strings.TrimSpace(branch)
	if err := git.CheckBranch(ctx, main, branch); err != nil {
		return nil, err
	}
	path := worktreePath(main, branch, settings.Load().WorktreeDir)
	if taken(path) {
		return nil, fmt.Errorf("%s is there already", proto.ShortDir(path))
	}
	from, untrack, fetch := "", false, originFetch{}
	if r.HasBranch(branch) {
		// Git checks a branch out in one worktree alone.
		cos, err := r.Checkouts(ctx)
		if err != nil {
			return nil, err
		}
		for _, co := range cos {
			if co.Branch == branch {
				return nil, fmt.Errorf("%s is checked out in %s", branch, proto.ShortDir(co.Path))
			}
		}
	} else {
		switch base {
		case "":
			def := r.DefaultBranch()
			switch {
			case def == "":
				from = "HEAD"
			case r.HasBranch(def):
				from = def
			default:
				from = "origin/" + def
			}
		case "HEAD":
			// dir's own branch, by name when it has one, so that whether
			// the work is merged reads against it.
			if st, err := r.Status(ctx); err == nil && st.Branch != "" {
				from = st.Branch
			} else {
				from = "HEAD"
			}
		default:
			from = base
		}
		// A pull request's worktree starts from the branch fetched for it.
		if p.PushBranch == "" && settings.Load().WorktreeFromOrigin {
			from, untrack, fetch = d.fromOrigin(ctx, r, from)
		}
	}
	if proto.Within(path, main) {
		if err := r.Exclude("/" + strings.SplitN(worktreeDir, "/", 2)[0] + "/"); err != nil {
			d.log.Printf("excluding %s: %v", worktreeDir, err)
		}
	}
	if err := r.AddWorktree(ctx, path, branch, from); err != nil {
		return nil, err
	}
	if untrack {
		if err := r.UnsetUpstream(ctx, branch); err != nil {
			d.log.Printf("untracking %s: %v", branch, err)
		}
	}
	d.log.Printf("worktree %s added on %s from %q", path, branch, from)
	if from != "" && p.PushBranch != "" {
		if err := r.SetPushTarget(ctx, branch, p.PushRemote, p.PushBranch); err != nil {
			d.log.Printf("pushing %s to %s/%s: %v", branch, p.PushRemote, p.PushBranch, err)
		}
	}
	// Other worktrees in the repository are no source of its files.
	n, err := r.CopyIncluded(ctx, path, worktreeDir, ".claude/worktrees")
	if err != nil {
		d.log.Printf("copying .worktreeinclude's files to %s: %v", path, err)
	}
	if n > 0 {
		d.log.Printf("copied %d files of .worktreeinclude to %s", n, path)
	}
	d.setUpWorktree(main, path, branch, fetch)
	return &proto.Worktree{Path: path, Branch: branch, Base: from, Main: main}, nil
}

// countWorktreeSessions sets how many sessions have a pane working in
// each of wts.
func (d *Daemon) countWorktreeSessions(wts []proto.GitWorktree) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range wts {
		wt := &wts[i]
		for _, s := range d.sessions {
			for _, pn := range s.panes {
				pn.mu.Lock()
				in := proto.Within(pn.info.WorkDir(), wt.Path)
				pn.mu.Unlock()
				if in {
					wt.Sessions++
					break
				}
			}
		}
	}
}

// sameDir reports whether a and b are one folder, as git and the system
// may spell it apart in case on Windows and macOS.
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}

// worktreePath is where a worktree for branch goes: in the repository's
// .kero/worktrees, else in dir (the worktree-dir setting) under the
// repository's name; a branch's slashes become dashes.
func worktreePath(main, branch, dir string) string {
	name := strings.ReplaceAll(branch, "/", "-")
	if dir = strings.TrimSpace(dir); dir != "" {
		dir = expandHome(dir)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(main, dir)
		}
		return filepath.Join(dir, filepath.Base(main), name)
	}
	return filepath.Join(main, filepath.FromSlash(worktreeDir), name)
}

// checkWorktree tells what removing a session's worktree would lose.
func (d *Daemon) checkWorktree(ref string) (proto.WorktreeCheck, error) {
	d.mu.Lock()
	s := d.findSessionLocked(ref)
	if s == nil {
		d.mu.Unlock()
		return proto.WorktreeCheck{}, fmt.Errorf("no session %q", ref)
	}
	if s.worktree == nil {
		d.mu.Unlock()
		return proto.WorktreeCheck{}, fmt.Errorf("session %s has no worktree of its own", s.id)
	}
	wt := *s.worktree
	shared := d.worktreeSharedLocked(s, wt.Path)
	d.mu.Unlock()
	wc := proto.WorktreeCheck{Worktree: wt, Shared: shared}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := git.Open(ctx, wt.Path)
	if err != nil {
		return wc, nil // gone already: nothing to lose
	}
	st, err := r.State(ctx, wt.Branch, wt.Base)
	if err != nil {
		return wc, err
	}
	wc.Files, wc.Unpushed, wc.Merged = st.Files, st.Unpushed, st.Merged
	return wc, nil
}

// worktreeSharedLocked reports whether a session besides s works in the
// worktree at path; d.mu is held.
func (d *Daemon) worktreeSharedLocked(s *session, path string) bool {
	for _, o := range d.sessions {
		if o == s {
			continue
		}
		if o.worktree != nil && o.worktree.Path == path {
			return true
		}
		for _, p := range o.panes {
			p.mu.Lock()
			in := proto.Within(p.info.WorkDir(), path)
			p.mu.Unlock()
			if in {
				return true
			}
		}
	}
	return false
}

// closeSession closes a session, and with p.RemoveWorktree its worktree,
// unless another session works in it. Without p.Force it refuses, keeping
// the session, when removing it would lose changes or commits.
func (d *Daemon) closeSession(p proto.CloseParams) error {
	d.quietClose(p)
	if !p.RemoveWorktree {
		return d.removeSession(p.ID)
	}
	wc, err := d.checkWorktree(p.ID)
	if err != nil {
		return err
	}
	// Its branch stays unless merged, with its commits: only what isn't
	// committed would be lost.
	if !wc.Shared && !p.Force && wc.Files > 0 {
		return fmt.Errorf("the worktree has %s: --force removes it anyway", wc.Losses())
	}
	// From before its panes close, which leaves the worktree to no
	// session, until it is gone: the cleanup leaves it be meanwhile.
	defer d.markRemoving(wc.Path)()
	panes, err := d.closePanes(p.ID)
	if err != nil {
		return err
	}
	if wc.Shared {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := d.removeWorktree(ctx, wc.Main, wc.Path, wc.Branch, wc.Base, panes); err != nil {
		return fmt.Errorf("the session closed, but its worktree stays: %w", err)
	}
	d.log.Printf("worktree %s removed", wc.Path)
	return nil
}

// markRemoving tells that the worktree at path is being removed, until
// done (worktreeInUse).
func (d *Daemon) markRemoving(path string) (done func()) {
	d.mu.Lock()
	if d.removing == nil {
		d.removing = map[string]int{}
	}
	d.removing[path]++
	d.mu.Unlock()
	return func() {
		d.mu.Lock()
		if d.removing[path]--; d.removing[path] <= 0 {
			delete(d.removing, path)
		}
		d.mu.Unlock()
	}
}

// treeWait is how long removing a worktree waits for the programs of the
// panes that closed in it, and whatever those started, to exit, before
// it kills those left; an agent's MCP servers, node and git may take a
// while.
var treeWait = 5 * time.Second

// awaitTrees tells whether removing a worktree waits for its programs:
// on Windows alone, as Unix deletes a folder in use, and kills nothing
// that outlives its pane on purpose, as a nohup job.
var awaitTrees = runtime.GOOS == "windows"

// removeWorktree removes the linked worktree at path of the repository
// whose main checkout is main, and its branch when base has every commit
// of it (git.RemoveWorktree), once the programs of panes, closed, have
// exited, as Windows deletes no folder that a process works in; Kero's
// own git status stays out of it meanwhile.
func (d *Daemon) removeWorktree(ctx context.Context, main, path, branch, base string, panes []*pane) error {
	awaitExit(panes)
	defer d.git.hold(path)()
	return git.RemoveWorktree(ctx, main, path, branch, base)
}

// awaitExit waits for the programs of panes, closed, and the processes
// they started, to exit, those of a program that exited before too: for
// up to treeWait, then it kills those left, and waits a second more for
// them.
func awaitExit(panes []*pane) {
	if !awaitTrees {
		return
	}
	var procs []*ptyProc
	for _, p := range panes {
		p.mu.Lock()
		for _, proc := range []*ptyProc{p.proc, p.ended} {
			if proc != nil {
				procs = append(procs, proc)
			}
		}
		p.mu.Unlock()
	}
	deadline, killed := time.Now().Add(treeWait), false
	for {
		procs = slices.DeleteFunc(procs, func(p *ptyProc) bool { return !p.running() })
		if len(procs) == 0 {
			return
		}
		if time.Now().After(deadline) {
			if killed {
				return
			}
			for _, p := range procs {
				p.killTree()
			}
			deadline, killed = time.Now().Add(time.Second), true
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// reviewBranch is a branch for a review of pull request number of its
// own, review/pr-<number>, or -2, -3, ... after it, of a name no branch
// or worktree folder has yet: a pull request may have several reviews.
func reviewBranch(r git.Repo, number int) string {
	base := fmt.Sprintf("review/pr-%d", number)
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		if r.HasBranch(name) {
			continue
		}
		if main := r.Main(); main != "" && taken(worktreePath(main, name, settings.Load().WorktreeDir)) {
			continue
		}
		return name
	}
}

// taken reports whether a new worktree can't go at path: something is
// there, but an empty folder, which git adds a worktree in, as a removal
// that failed on Windows used to leave.
func taken(path string) bool {
	if _, err := os.Lstat(path); err != nil {
		return false
	}
	entries, err := os.ReadDir(path)
	return err != nil || len(entries) > 0
}
