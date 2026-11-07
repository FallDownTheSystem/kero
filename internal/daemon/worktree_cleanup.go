package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"kero/internal/git"
	"kero/internal/github"
	"kero/internal/proto"
	"kero/internal/settings"
)

// The automatic cleanup of the worktrees kero made (settings'
// worktree-cleanup-*): a minute after kerod starts, hourly, and when the
// user asks (worktree.cleanup), it removes those its rules say are done
// with. Never one a session works in, one git keeps locked, or one that
// holds anything of its own: a change, an untracked file, or an ignored
// file no build makes again (git.Repo.LocalFiles). A branch goes with its
// worktree only when the default branch has it all, as on closing a
// session; else it stays, with its commits.

// cleanupMu keeps one cleanup at a time.
var cleanupMu sync.Mutex

// cleanupGrace is how long a new worktree is safe from the cleanup: kerod
// adds a session's before the session that works in it.
const cleanupGrace = 10 * time.Minute

// worktreeCleanupLoop runs the cleanup a minute after kerod starts, once
// it restored its sessions, then hourly.
func (d *Daemon) worktreeCleanupLoop() {
	t := time.NewTimer(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			d.cleanWorktrees()
			t.Reset(time.Hour)
		case <-d.quit:
			return
		}
	}
}

// cleanupRepo is a repository the cleanup goes through: its main
// checkout, and the rules of its projects.
type cleanupRepo struct {
	r     git.Repo
	rules settings.WorktreeCleanup
	// keep: a project of it turned the cleanup off.
	keep bool
}

// cleanWorktrees removes the worktrees of the projects' repositories that
// their rules say are done with.
func (d *Daemon) cleanWorktrees() proto.WorktreeCleanupResult {
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	var res proto.WorktreeCleanupResult
	set := settings.Load()
	d.mu.Lock()
	projects := slices.Clone(d.projects)
	d.mu.Unlock()
	// A repository's projects share its worktrees: one turned off keeps
	// them, else the first whose rules remove any decides.
	repos := map[string]*cleanupRepo{}
	var order []string
	for _, p := range projects {
		if p.Dir == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		r, err := git.Open(ctx, p.Dir)
		cancel()
		main := r.Main()
		if err != nil || main == "" {
			continue
		}
		cr := repos[main]
		if cr == nil {
			mr, err := git.Open(context.Background(), main)
			if err != nil {
				continue
			}
			cr = &cleanupRepo{r: mr}
			repos[main] = cr
			order = append(order, main)
		}
		w := set.CleanupFor(p.Dir)
		if _, own := set.ProjectCleanup[settings.CleanupKey(p.Dir)]; own && !w.On {
			cr.keep = true
		} else if !cr.rules.Removes() && w.Removes() {
			cr.rules = w
		}
	}
	seen := d.loadCleanupSeen()
	now := time.Now()
	for _, main := range order {
		if cr := repos[main]; !cr.keep && cr.rules.Removes() {
			d.cleanRepo(cr, set.WorktreeDir, seen, now, &res)
		}
	}
	for path := range seen {
		if _, err := os.Stat(path); err != nil {
			delete(seen, path)
		}
	}
	d.saveCleanupSeen(seen)
	if len(res.Removed) > 0 || len(res.Failed) > 0 {
		d.log.Printf("worktree cleanup: %d removed, %d failed", len(res.Removed), len(res.Failed))
	}
	return res
}

// cleanRepo removes the worktrees of a repository its rules say are done
// with, and notes in seen when a session last worked in each.
func (d *Daemon) cleanRepo(cr *cleanupRepo, dir string, seen map[string]int64, now time.Time, res *proto.WorktreeCleanupResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	main := cr.r.Main()
	cos, err := cr.r.Checkouts(ctx)
	if err != nil {
		d.log.Printf("worktree cleanup of %s: %v", main, err)
		return
	}
	// Kero's worktrees, of either folder: its own, and the setting's.
	roots := []string{filepath.Dir(worktreePath(main, "x", "")), filepath.Dir(worktreePath(main, "x", dir))}
	base := cr.r.DefaultRef()
	var ghRepos []github.Repo
	ghAsked := false
	for _, co := range cos {
		if co.Prunable || co.Locked || sameDir(co.Path, main) ||
			!slices.ContainsFunc(roots, func(root string) bool { return sameDir(filepath.Dir(co.Path), root) }) {
			continue
		}
		if d.worktreeInUse(co.Path) {
			seen[co.Path] = now.UnixMilli()
			continue
		}
		used := max(git.Used(co.Path), seen[co.Path])
		if used == 0 || now.Sub(time.UnixMilli(used)) < cleanupGrace {
			continue
		}
		wr, err := git.Open(ctx, co.Path)
		if err != nil {
			continue
		}
		st, err := wr.State(ctx, co.Branch, base)
		if err != nil || st.Files > 0 {
			continue
		}
		if f, err := wr.LocalFiles(ctx, main); err != nil || f != "" {
			continue
		}
		rules, reason := cr.rules, ""
		switch {
		case rules.Closed:
			reason = "closed"
		case rules.Unchanged && co.Branch != "" && base != "" && st.Merged:
			reason = "unchanged"
		case rules.InactiveDays > 0 && now.Sub(time.UnixMilli(used)) >= time.Duration(rules.InactiveDays)*24*time.Hour:
			reason = "inactive"
		case rules.Merged && co.Branch != "":
			if !ghAsked {
				ghAsked = true
				if _, byName, err := gitHubRepos(ctx, cr.r); err == nil {
					for _, gr := range byName {
						ghRepos = append(ghRepos, gr)
					}
				}
			}
			number := 0
			if m := reviewBranchRe.FindStringSubmatch(co.Branch); m != nil {
				number, _ = strconv.Atoi(m[1])
			}
			for _, gr := range ghRepos {
				merged, err := github.MergedAt(ctx, main, gr, number, co.Branch, co.Head)
				if err != nil {
					d.log.Printf("worktree cleanup: pull request of %s in %s: %v", co.Branch, gr.Slug(), err)
				}
				if merged {
					reason = "merged"
					break
				}
			}
		}
		// A session may have started in it while this looked.
		if reason == "" || d.worktreeInUse(co.Path) {
			continue
		}
		if err := git.RemoveWorktree(ctx, main, co.Path, co.Branch, base); err != nil {
			res.Failed = append(res.Failed, proto.ShortDir(co.Path)+": "+err.Error())
			d.log.Printf("worktree cleanup: removing %s: %v", co.Path, err)
			continue
		}
		delete(seen, co.Path)
		res.Removed = append(res.Removed, proto.CleanedWorktree{Path: co.Path, Branch: co.Branch, Reason: reason})
		d.log.Printf("worktree cleanup: removed %s on %q (%s)", co.Path, co.Branch, reason)
	}
}

// worktreeInUse reports whether a session works in the worktree at path,
// or was made for it.
func (d *Daemon) worktreeInUse(path string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.worktreeSharedLocked(nil, path)
}

// cleanupSeenPath keeps when a session last worked in each worktree, as
// of the cleanups, for the inactive rule: a session may work in one for
// days without a commit.
func (d *Daemon) cleanupSeenPath() string { return filepath.Join(d.dir, "worktree-cleanup.json") }

func (d *Daemon) loadCleanupSeen() map[string]int64 {
	seen := map[string]int64{}
	if b, err := os.ReadFile(d.cleanupSeenPath()); err == nil {
		json.Unmarshal(b, &seen)
	}
	return seen
}

func (d *Daemon) saveCleanupSeen(seen map[string]int64) {
	if b, err := json.Marshal(seen); err == nil {
		writeAtomic(d.cleanupSeenPath(), b)
	}
}
