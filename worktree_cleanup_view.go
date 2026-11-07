package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
	"kero/internal/settings"

	"kero/locales"
)

// Settings › Git's worktree cleanup: when kerod removes the worktrees
// kero made, for all projects and for one of its own, and Delete Now,
// which runs it at once.

// cleanupDraft is the project cleanup's controls' own state: the project
// chosen, by its folder, and its mode and rules as the controls have
// them, which applied says the settings have.
type cleanupDraft struct {
	dir           string
	mode, applied string // "inherit", "off" or "custom"
	rules         settings.WorktreeCleanup
	rulesApplied  settings.WorktreeCleanup
	running       bool // Delete Now asked kerod
}

// cleanupDays are the inactive rule's choices, 0 for none.
var cleanupDays = []int{0, 1, 3, 7, 14, 30, 60, 90}

// worktreeCleanupGroups are the cleanup's groups of Settings › Git.
func (a *App) worktreeCleanupGroups(c *ui.Context, pal *palette) {
	s := &a.set
	settingsGroup(c, pal, locales.SetWorktreeCleanup(), "", func() {
		settingRow(c, pal, locales.SetWorktreeCleanupOn(), locales.SetWorktreeCleanupOnDetail(), func() {
			toggle(c.Key("worktree-cleanup"), &s.WorktreeCleanup.On, pal)
		})
		if s.WorktreeCleanup.On {
			cleanupRuleRows(c, pal, "worktree-cleanup", &s.WorktreeCleanup)
		}
	})
	// A group of its own: its rules' rows are keyed as those above.
	if dirs, names := a.cleanupProjects(); len(dirs) > 0 {
		settingsGroup(c, pal, "", "", func() { a.cleanupProjectRows(c, pal, dirs, names) })
	}
	settingsGroup(c, pal, "", locales.SetWorktreeCleanupNote(), func() {
		settingRow(c, pal, locales.SetWorktreeCleanupNow(), locales.SetWorktreeCleanupNowDetail(), func() {
			label := locales.SetWorktreeCleanupNowButton()
			if a.ss.cleanup.running {
				label = locales.SetWorktreeCleanupRunning()
			}
			if button(c, "worktree-cleanup-now", label, "", false, pal) && !a.ss.cleanup.running {
				a.cleanWorktreesNow()
			}
		})
	})
}

// cleanupRuleRows are the rules of a cleanup, w, whose controls' keys
// key starts.
func cleanupRuleRows(c *ui.Context, pal *palette, key string, w *settings.WorktreeCleanup) {
	settingRow(c, pal, locales.SetWorktreeCleanupInactive(), locales.SetWorktreeCleanupInactiveDetail(), func() {
		days := cleanupDays
		if !slices.Contains(days, w.InactiveDays) {
			days = append(slices.Clone(days), w.InactiveDays)
		}
		popup(c, key+"-inactive", &w.InactiveDays, days, func(n int) string {
			if n == 0 {
				return locales.SetWorktreeCleanupNever()
			}
			return locales.SetWorktreeCleanupDays(n)
		}, 140, pal)
	})
	settingRow(c, pal, locales.SetWorktreeCleanupMerged(), locales.SetWorktreeCleanupMergedDetail(), func() {
		toggle(c.Key(key+"-merged"), &w.Merged, pal)
	})
	settingRow(c, pal, locales.SetWorktreeCleanupUnchanged(), locales.SetWorktreeCleanupUnchangedDetail(), func() {
		toggle(c.Key(key+"-unchanged"), &w.Unchanged, pal)
	})
	settingRow(c, pal, locales.SetWorktreeCleanupClosed(), locales.SetWorktreeCleanupClosedDetail(), func() {
		toggle(c.Key(key+"-closed"), &w.Closed, pal)
	})
}

// cleanupProjects returns this machine's projects' folders, as
// ProjectCleanup has them, and their names.
func (a *App) cleanupProjects() ([]string, map[string]string) {
	var dirs []string
	names := map[string]string{}
	for _, p := range a.st.Projects {
		if p.Dir == "" || a.hostOf(p.ID).remote() {
			continue
		}
		if dir := settings.CleanupKey(p.Dir); !slices.Contains(dirs, dir) {
			dirs, names[dir] = append(dirs, dir), p.Name
		}
	}
	return dirs, names
}

// cleanupProjectRows choose a project of this machine, one of dirs, and
// whether its worktrees follow the rules above, none, or its own.
func (a *App) cleanupProjectRows(c *ui.Context, pal *palette, dirs []string, names map[string]string) {
	s, d := &a.set, &a.ss.cleanup
	if !slices.Contains(dirs, d.dir) {
		d.dir = dirs[0]
	}
	// What the settings have, unless the controls changed it.
	own, ok := s.ProjectCleanup[d.dir]
	mode := "inherit"
	if ok {
		mode = "custom"
		if !own.On {
			mode = "off"
		}
	}
	if mode != d.applied || own != d.rulesApplied {
		d.mode, d.applied, d.rules, d.rulesApplied = mode, mode, own, own
	}
	settingRow(c, pal, locales.SetWorktreeCleanupProject(), locales.SetWorktreeCleanupProjectDetail(), func() {
		popup(c, "worktree-cleanup-project", &d.dir, dirs, func(dir string) string {
			if n := names[dir]; n != "" {
				return n
			}
			return proto.ShortDir(dir)
		}, 200, pal).Tooltip(proto.ShortDir(d.dir))
	})
	settingRow(c, pal, locales.SetWorktreeCleanupProjectMode(), "", func() {
		segmented(c, "worktree-cleanup-mode", &d.mode, []string{"inherit", "off", "custom"},
			[]string{locales.SetWorktreeCleanupInherit(), locales.SetWorktreeCleanupOff(), locales.SetWorktreeCleanupCustom()})
	})
	if d.mode == "custom" {
		cleanupRuleRows(c, pal, "worktree-cleanup-own", &d.rules)
	}
	// The controls' changes, into the settings.
	if d.mode == d.applied && d.rules == d.rulesApplied {
		return
	}
	if s.ProjectCleanup == nil {
		s.ProjectCleanup = map[string]settings.WorktreeCleanup{}
	}
	switch d.mode {
	case "inherit":
		delete(s.ProjectCleanup, d.dir)
	case "off":
		s.ProjectCleanup[d.dir] = settings.WorktreeCleanup{}
	case "custom":
		if d.applied != "custom" {
			// Its own rules start as those above.
			d.rules = s.WorktreeCleanup
		}
		d.rules.On = true
		s.ProjectCleanup[d.dir] = d.rules
	}
	d.applied, d.rulesApplied = d.mode, s.ProjectCleanup[d.dir]
	if d.mode == "inherit" {
		d.rulesApplied = settings.WorktreeCleanup{}
	}
}

// cleanWorktreesNow runs this machine's cleanup at once, and tells what
// it removed.
func (a *App) cleanWorktreesNow() {
	cl := a.local().cl
	if cl == nil {
		a.ss.toast = locales.SetWorktreeCleanupNoKerod()
		return
	}
	a.ss.cleanup.running = true
	go func() {
		var res proto.WorktreeCleanupResult
		err := cl.Call("worktree.cleanup", nil, &res)
		a.update(func() {
			a.ss.cleanup.running = false
			switch {
			case err != nil:
				a.ss.toast = locales.SetWorktreeCleanupFailed(err.Error())
			case len(res.Failed) > 0:
				a.ss.toast = locales.SetWorktreeCleanupSome(len(res.Removed), strings.Join(res.Failed, "; "))
			case len(res.Removed) == 0:
				a.ss.toast = locales.SetWorktreeCleanupNone()
			default:
				a.ss.toast = locales.SetWorktreeCleanupRemoved(len(res.Removed))
			}
		})
	}()
}
