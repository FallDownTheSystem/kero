package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// askCloseWorktree closes a session in a worktree of its own once the
// user says whether its worktree goes with it, as what kerod finds the
// removal would lose decides: Remove at hand when nothing, Keep when
// changes. A worktree another session works in stays, unasked.
func (a *App) askCloseWorktree(id string) {
	s, h := a.session(id), a.hostOf(id)
	cl := h.cl
	if s == nil || cl == nil {
		return
	}
	name := s.DisplayName(a.set.SessionName)
	go func() {
		var wc proto.WorktreeCheck
		err := cl.Call("worktree.check", proto.IDParams{ID: id}, &wc)
		a.update(func() {
			if a.session(id) == nil {
				return
			}
			keep := func() { a.endSession(id, proto.CloseParams{ID: id}) }
			if err != nil || wc.Shared {
				keep()
				return
			}
			remove := func(force bool) func() {
				return func() { a.endSession(id, proto.CloseParams{ID: id, RemoveWorktree: true, Force: force}) }
			}
			path := h.shortDir(wc.Path)
			cf := &confirmation{open: true, title: locales.WorktreeRemoveTitle(name),
				action: locales.WorktreeRemove(), run: remove(false), alt: locales.WorktreeKeep(), altRun: keep}
			switch {
			case wc.Files > 0:
				cf.detail = locales.WorktreeRemoveDirty(wc.Files, path, wc.Branch)
				cf.action, cf.run = locales.WorktreeKeep(), keep
				cf.alt, cf.altRun, cf.altDestroy = locales.WorktreeRemoveAnyway(), remove(true), true
			case wc.Merged:
				cf.detail = locales.WorktreeRemoveMerged(path, wc.Branch)
			default:
				cf.detail = locales.WorktreeRemoveUnmerged(path, wc.Branch)
			}
			a.confirm = cf
		})
	}()
}

// The History view's worktrees of the repository: each with what removing
// it would lose, to open, push, merge or remove, and New Worktree, which
// starts a session in a new one, as New Session in Worktree does.

// fetchWorktrees asks for the repository's worktrees anew, with what
// removing each would lose while their section is open, at, the
// worktree's stamp and that.
func (a *App) fetchWorktrees(pane, at string) {
	hs := &a.side.git.hist
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	hs.wtAt = at
	state := hs.wtOn
	go func() {
		var res proto.GitWorktrees
		err := cl.Call("git.worktrees", proto.GitWorktreesParams{Pane: pane, State: state}, &res)
		a.update(func() {
			if err == nil && hs.wtAt == at {
				hs.wts, hs.wtDefault = res.Worktrees, res.Default
			}
		})
	}()
}

// worktreesRow is the worktrees' title, which a click opens or closes,
// with Prune while some are gone, and New Worktree.
func (a *App) worktreesRow(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane) {
	hs := &a.side.git.hist
	row := ui.Row(c.Key("worktrees")).Height(28).Padding(0, 6, 0, 12).Gap(7).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	if row.Clicked() {
		hs.wtOn = !hs.wtOn
	}
	gone := slices.ContainsFunc(hs.wts, func(w proto.GitWorktree) bool { return w.Prunable })
	id, root := pane.ID, pane.Git.Root
	row.Children(func() {
		chev := ui.Icon(c, chevronIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		chev.Rotate(chev.AnimateTo("turn", 90*b2f(hs.wtOn), chevronTurn))
		ui.Icon(c, folderIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, locales.HistoryWorktrees(len(hs.wts))).FontSize(12.5).FontWeight(600).TextColor(pal.textMuted).SingleLine().Grow(1).MinWidth(0)
		if gone {
			iconButton(c, "prune", revertIcon, locales.HistoryWorktreesPrune(), pal, func() {
				a.runGit(id, root, proto.GitOpParams{Op: "worktreePrune"}, locales.HistoryWorktreesPrune(), nil)
			})
		}
		iconButton(c, "new", plusIcon, locales.ActionNewSessionWorktree(), pal, func() {
			a.askNewSession(a.hostOf(s.ID), s.Project, root, true)
		})
	})
}

// worktreeRow is a worktree: its branch, what of it isn't safe yet, or
// that it is, and its folder; Open, Push while it has commits on no
// remote, and Remove, with more in its menu.
func (a *App) worktreeRow(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, w proto.GitWorktree) {
	here := samePath(w.Path, pane.Git.Root)
	h := a.hostOf(pane.ID)
	name := w.Branch
	if name == "" {
		name = locales.HistoryDetached(shortHash(w.Head))
	}
	row := ui.Row(c.Key("wt")).Height(28).Padding(0, 6, 0, 30).Gap(6).AlignItems(ui.Center).Tooltip(w.Path)
	if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	if row.DoubleClicked() && !w.Prunable {
		a.openWorktree(pane.ID, w.Path)
	}
	row.ContextMenu(func(m *ui.Menu) { a.worktreeMenu(c, m, s, pane, w) })
	added, _, _ := a.diffColors(c, pal)
	row.Children(func() {
		weight := 500
		if here {
			weight = 700
		}
		// The name keeps some room, what it isn't safe for after it, then
		// the folder, which goes first as the row narrows.
		nw, _ := c.MeasureText(0, ui.Span{Text: name, Size: 12.5, Weight: weight})
		ui.Text(c, name).FontSize(12.5).FontWeight(weight).TextColor(pal.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(min(nw, 90))
		var state []string
		col := pal.textFaint
		switch {
		case w.Prunable:
			state, col = append(state, locales.HistoryWorktreeGone()), pal.attention
		case w.Main:
			state = append(state, locales.HistoryWorktreeMain())
		case w.State && (w.Files > 0 || w.Unpushed > 0):
			col = pal.attention
			if w.Files > 0 {
				state = append(state, locales.HistoryWorktreeChanged(w.Files))
			}
			if w.Unpushed > 0 {
				state = append(state, locales.HistoryWorktreeUnpushed(w.Unpushed))
			}
		case w.State && w.Merged:
			state, col = append(state, locales.HistoryWorktreeMerged()), added
		case w.State:
			state, col = append(state, locales.HistoryWorktreePushed()), added
		}
		if len(state) > 0 {
			ui.Text(c, strings.Join(state, " · ")).FontSize(11).TextColor(col).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0).Tooltip(strings.Join(state, " · "))
		}
		ui.Text(c, h.shortDir(w.Path)).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Grow(1).Shrink(1000).MinWidth(0)
		if w.Prunable {
			return
		}
		if !here {
			iconButton(c, "open", terminalIcon, locales.HistoryWorktreeOpen(), pal, func() { a.openWorktree(pane.ID, w.Path) })
		}
		if w.Branch != "" && w.Unpushed > 0 {
			iconButton(c, "push", pushIcon, locales.GitPushBranch(menuName(w.Branch)), pal, func() { a.pushWorktree(pane, w) })
		}
		if !here && !w.Main {
			iconButton(c, "remove", closeIcon, locales.WorktreeRemove(), pal, func() { a.removeWorktree(pane, w, false) })
		}
	})
}

// worktreeMenu is a worktree's menu.
func (a *App) worktreeMenu(c *ui.Context, m *ui.Menu, s *proto.Session, pane *proto.Pane, w proto.GitWorktree) {
	here := samePath(w.Path, pane.Git.Root)
	if !w.Prunable && m.Item(locales.HistoryWorktreeOpen()).Disabled(here).Chosen() {
		a.openWorktree(pane.ID, w.Path)
	}
	if m.Item(locales.FilesCopyPath()).Chosen() {
		c.WriteClipboard(w.Path)
	}
	if !w.Prunable && !a.hostOf(pane.ID).remote() && m.Item(locales.CommonShowIn(fileManager())).Chosen() {
		revealPath(w.Path)
	}
	if w.Branch != "" {
		m.Separator()
		if m.Item(locales.HistoryCopyBranch()).Chosen() {
			c.WriteClipboard(w.Branch)
		}
		if m.Item(locales.GitPushBranch(menuName(w.Branch))).Disabled(w.State && w.Unpushed == 0).Chosen() {
			a.pushWorktree(pane, w)
		}
		if !here {
			a.mergeItems(m, pane, w.Branch, w.Branch, false)
		}
	}
	if here || w.Main {
		return
	}
	m.Separator()
	if w.Prunable {
		if m.Item(locales.HistoryWorktreesPrune()).Chosen() {
			a.runGit(pane.ID, pane.Git.Root, proto.GitOpParams{Op: "worktreePrune"}, locales.HistoryWorktreesPrune(), nil)
		}
		return
	}
	if m.Item(locales.WorktreeRemove() + "…").Chosen() {
		a.removeWorktree(pane, w, false)
	}
	if w.Branch != "" && m.Item(locales.HistoryWorktreeRemoveBranch()+"…").Disabled(w.State && (w.Files > 0 || w.Unpushed > 0)).Chosen() {
		a.removeWorktree(pane, w, true)
	}
}

// pushWorktree pushes a worktree's branch, from the pane's worktree.
func (a *App) pushWorktree(pane *proto.Pane, w proto.GitWorktree) {
	a.push(pane.ID, pane.Git.Root, proto.GitOpParams{Name: w.Branch})
}

// removeWorktree removes a worktree, and with branch its branch, once the
// user says so, as what it would lose decides. A worktree a session was
// made for closes with it, as closing the session asks; one another
// session works in stays, which the user closes first.
func (a *App) removeWorktree(pane *proto.Pane, w proto.GitWorktree, branch bool) {
	h := a.hostOf(pane.ID)
	var users []string
	for _, s := range a.st.Sessions {
		if a.hostOf(s.ID) != h {
			continue
		}
		if wt := s.Worktree; wt != nil && samePath(wt.Path, w.Path) {
			a.askCloseWorktree(s.ID)
			return
		}
		for _, p := range s.Panes {
			if p.Git != nil && samePath(p.Git.Root, w.Path) {
				users = append(users, s.DisplayName(a.set.SessionName))
				break
			}
		}
	}
	if len(users) > 0 {
		a.toast(locales.HistoryWorktreeInUse(strings.Join(users, ", ")))
		return
	}
	id, root := pane.ID, pane.Git.Root
	name := w.Branch
	if name == "" {
		name = h.shortDir(w.Path)
	}
	run := func(force, withBranch bool) func() {
		p := proto.GitOpParams{Op: "worktreeRemove", Path: w.Path, Force: force}
		if withBranch {
			p.Name = w.Branch
		}
		return func() { a.runGit(id, root, p, locales.WorktreeRemove(), nil) }
	}
	path := h.shortDir(w.Path)
	cf := &confirmation{open: true, title: locales.HistoryWorktreeRemoveTitle(menuName(name)), action: locales.WorktreeRemove(), run: run(false, branch)}
	switch {
	case w.Files > 0:
		cf.detail = locales.HistoryWorktreeRemoveDirty(w.Files, path)
		cf.action, cf.destroy, cf.run = locales.WorktreeRemoveAnyway(), true, run(true, false)
	case w.Unpushed > 0:
		cf.detail = locales.HistoryWorktreeRemoveUnpushed(w.Unpushed, menuName(w.Branch))
		cf.run = run(false, false)
		cf.alt, cf.altRun = locales.GitPushBranch(menuName(w.Branch)), func() { a.pushWorktree(pane, w) }
	case branch:
		cf.detail = locales.HistoryWorktreeRemoveBranchDetail(path, menuName(w.Branch))
		cf.action = locales.HistoryWorktreeRemoveBranch()
	case w.Branch != "" && w.State:
		cf.detail = locales.HistoryWorktreeRemoveSafe(path, menuName(w.Branch))
		cf.alt, cf.altRun = locales.HistoryWorktreeRemoveBranch(), run(false, true)
	default:
		cf.detail = locales.HistoryWorktreeRemoveDetail(path)
	}
	a.confirm = cf
}

// samePath tells that two paths of a host are one, its slashes either
// way, a Windows one's letters of either case.
func samePath(a, b string) bool {
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, `\`, "/"), "/") }
	x, y := norm(a), norm(b)
	if len(x) > 1 && x[1] == ':' {
		return strings.EqualFold(x, y)
	}
	return x == y
}
