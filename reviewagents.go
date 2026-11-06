package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// Review agents: the sessions started to review a repository's pull
// requests (proto.Session.Review), which the Review view lists above the
// pull requests, with the worktrees of those that closed and left theirs.
// A row shows its agent's status and goes to its session; the agent's
// terminal stays in the session, as the panel showing it would resize it.

// reviewAgent is a review of a pull request: its session, nil once it
// closed, leaving its worktree at path on branch, with files changed.
type reviewAgent struct {
	number  int
	session *proto.Session
	path    string
	branch  string
	files   int
}

// reviewAgents lists a repository's reviews on h: its review sessions,
// the newest first, then the review worktrees no session works in.
func (a *App) reviewAgents(h *host, slug string) []reviewAgent {
	var out []reviewAgent
	for i := range a.st.Sessions {
		s := &a.st.Sessions[i]
		if s.Review == nil || a.hostOf(s.ID) != h || !strings.EqualFold(s.Review.Slug, slug) {
			continue
		}
		ra := reviewAgent{number: s.Review.Number, session: s}
		if s.Worktree != nil {
			ra.path, ra.branch = s.Worktree.Path, s.Worktree.Branch
		}
		out = append(out, ra)
	}
	slices.SortStableFunc(out, func(x, y reviewAgent) int { return cmp.Compare(y.session.Created, x.session.Created) })
	ps := &a.side.pulls
	if ps.worktreesKey == h.name+"\x00"+strings.ToLower(slug) {
		for _, wt := range ps.worktrees {
			if !a.worktreeInUse(h, wt.Path) {
				out = append(out, reviewAgent{number: wt.Number, path: wt.Path, branch: wt.Branch, files: wt.Files})
			}
		}
	}
	return out
}

// prAgent returns the review of a pull request: its session while one
// is open, else a worktree a closed one left; false for none.
func (a *App) prAgent(h *host, slug string, number int) (reviewAgent, bool) {
	var found reviewAgent
	ok := false
	for _, ra := range a.reviewAgents(h, slug) {
		if ra.number != number {
			continue
		}
		if ra.session != nil {
			return ra, true
		}
		if !ok {
			found, ok = ra, true
		}
	}
	return found, ok
}

// worktreeInUse tells a worktree a session of h works in.
func (a *App) worktreeInUse(h *host, path string) bool {
	for i := range a.st.Sessions {
		s := &a.st.Sessions[i]
		if a.hostOf(s.ID) != h {
			continue
		}
		if s.Worktree != nil && samePath(s.Worktree.Path, path) {
			return true
		}
		for _, p := range s.Panes {
			if proto.Within(p.WorkDir(), path) {
				return true
			}
		}
	}
	return false
}

// fetchReviewWorktrees asks a host for the worktrees reviews left in the
// repository of a pane, which the Review view lists with its sessions. A
// kerod older than pr.worktrees has none to show.
func (a *App) fetchReviewWorktrees(h *host, pane, slug string) {
	ps := &a.side.pulls
	cl := h.cl
	if cl == nil || slug == "" {
		return
	}
	key := h.name + "\x00" + strings.ToLower(slug)
	ps.worktreesPane = pane
	go func() {
		var wts []proto.ReviewWorktree
		err := cl.Call("pr.worktrees", proto.PRWorktreesParams{Pane: pane}, &wts)
		a.update(func() {
			if err != nil {
				return
			}
			ps.worktrees, ps.worktreesKey = wts, key
		})
	}()
}

// refetchReviewWorktrees asks for the review worktrees the view lists
// anew, as a review session closed with its worktree or without.
func (a *App) refetchReviewWorktrees() {
	ps := &a.side.pulls
	key, pane := ps.worktreesKey, ps.worktreesPane
	if key == "" || pane == "" {
		return
	}
	h := a.hostOf(pane)
	if !strings.HasPrefix(key, h.name+"\x00") {
		return
	}
	a.fetchReviewWorktrees(h, pane, strings.TrimPrefix(key, h.name+"\x00"))
}

// listedPR returns a pull request of the list by its number.
func listedPR(res *proto.PRList, number int) (proto.PullRequest, bool) {
	if res != nil {
		for _, pr := range res.PRs {
			if pr.Number == number {
				return pr, true
			}
		}
	}
	return proto.PullRequest{}, false
}

// reviewAgentsView lists the repository's reviews, above its pull
// requests; nothing while it has none.
func (a *App) reviewAgentsView(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, res *proto.PRList) {
	slug := pullsSlug(res)
	if slug == "" {
		return
	}
	agents := a.reviewAgents(a.hostOf(pane.ID), slug)
	if len(agents) == 0 {
		return
	}
	ui.Column(c.Key("agents")).Shrink(0).Padding(0, 6, 6, 6).Gap(1).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Height(22).Padding(0, 6).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.PrsAgents()).FontSize(11.5).FontWeight(600).TextColor(pal.textFaint).SingleLine()
			ui.Text(c, strconv.Itoa(len(agents))).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine()
		})
		for _, ra := range agents {
			a.reviewAgentRow(c, pal, s, pane, res, ra)
		}
		ui.Box(c).Height(1).Margin(5, 6, 0, 6).Background(pal.text.Alpha(0.08))
	})
}

// reviewAgentRow is a review: its status, as the sidebar's, the pull
// request's number and title, and its agent's message, else where it
// works. A click goes to its session; a closed one's opens the pull
// request.
func (a *App) reviewAgentRow(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, res *proto.PRList, ra reviewAgent) {
	key := ra.path
	if ra.session != nil {
		key = ra.session.ID
	}
	slug := pullsSlug(res)
	pr, listed := listedPR(res, ra.number)
	title := pr.Title
	if !listed {
		if dr, ok := a.side.pulls.drafts[ra.number]; ok && dr.Title != "" {
			title = dr.Title
		} else if ra.session != nil {
			title = ra.session.DisplayName(a.set.SessionName)
		}
	}
	row := ui.Row(c.Key("agent-"+key)).FillWidth().Padding(5, 6, 5, 6).Gap(8).AlignItems(ui.Center).Radius(6).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.6))
	}
	menu := func(m *ui.Menu) { a.reviewAgentMenu(m, s, pane, res, ra) }
	row.ContextMenu(menu)
	if row.Clicked() {
		switch {
		case ra.session != nil:
			a.selectSession(ra.session.ID)
		case listed:
			a.openPRPage(pane, res.Remote, slug, pr)
		}
	}
	detail, strong := "", false
	switch {
	case ra.session != nil && ra.session.Message != "":
		detail, strong = strings.Join(strings.Fields(ra.session.Message), " "), true
	case ra.session != nil && ra.branch != "":
		detail = ra.branch
	case ra.session != nil:
		detail = a.hostOf(ra.session.ID).shortDir(ra.session.WorkDir())
	default:
		detail = locales.PrsAgentClosed(ra.branch)
		if ra.files > 0 {
			detail += " · " + locales.PrsAgentChanged(ra.files)
		}
	}
	row.Tooltip(detail)
	row.Children(func() {
		if ra.session != nil {
			statusIconP(c, pal, ra.session.Status, ra.session.Unread, ra.session.Progress, 14)
		} else {
			ui.Icon(c, folderIcon).Size(13, 13).TextColor(pal.textFaint).Shrink(0)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "#"+strconv.Itoa(ra.number)).FontSize(12).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Shrink(0)
				name := ui.Text(c, title).FontSize(12.5).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
				if ra.session == nil {
					name.TextColor(pal.textMuted)
				}
				ui.Spacer(c)
				if ra.session != nil {
					ui.Text(c, ago(ra.session.Since, c.Now())).FontSize(11.5).TextColor(pal.textFaint).FontFeatures("tnum").SingleLine().Shrink(0)
				}
			})
			fg := pal.textFaint
			if strong {
				fg = pal.textMuted
			}
			ui.Text(c, detail).FontSize(11.5).TextColor(fg).SingleLine().Ellipsis("…").MinWidth(0)
		})
		if row.Hovered() {
			b := ui.ButtonBase(c.Key("menu")).Size(22, 22).Radius(6).Center().TextColor(pal.textMuted).Shrink(0).
				Label(locales.PrsAgentOptions()).Tooltip(locales.PrsAgentOptions())
			if b.Hovered() {
				b.Background(pal.rowHover)
			}
			b.Children(func() { ui.Icon(c, moreIcon).Size(13, 13) })
			b.Menu(menu)
		}
	})
}

// reviewAgentMenu is a review's menu: its session's, to go to and to
// close, which asks what goes with its worktree; a closed one's, to
// review again in its worktree, or remove it.
func (a *App) reviewAgentMenu(m *ui.Menu, s *proto.Session, pane *proto.Pane, res *proto.PRList, ra reviewAgent) {
	slug := pullsSlug(res)
	pr, listed := listedPR(res, ra.number)
	if ra.session != nil {
		id := ra.session.ID
		if m.Item(locales.PrsOpenAgent()).Chosen() {
			a.selectSession(id)
		}
	}
	if m.Item(locales.PrsOpenPage()).Disabled(!listed).Chosen() {
		a.openPRPage(pane, res.Remote, slug, pr)
	}
	if m.Item(locales.PrsOpen()).Disabled(!listed).Chosen() {
		openURL(pr.URL)
	}
	m.Separator()
	if ra.session != nil {
		id := ra.session.ID
		if m.Item(locales.ActionCloseSession()).Chosen() {
			a.defer_(func() { a.closeSession(id, true) })
		}
		return
	}
	if m.Item(locales.PrsReviewAgain()).Disabled(!listed).Chosen() {
		a.askReview(s, pane, res.Remote, slug, pr, reviewWorktree)
	}
	if m.Item(locales.PrsRemoveWorktreeItem()).Chosen() {
		a.askRemoveReviewWorktree(pane.ID, ra)
	}
}

// askRemoveReviewWorktree removes a closed review's worktree once the
// user says so, warned of the changed files it would lose.
func (a *App) askRemoveReviewWorktree(pane string, ra reviewAgent) {
	h := a.hostOf(pane)
	path := h.shortDir(ra.path)
	remove := func(force bool) func() {
		return func() {
			p := proto.PRWorktreeRemoveParams{Pane: pane, Path: ra.path, Force: force}
			a.callOn(h, "pr.worktree.remove", p, func(err error) {
				if err != nil {
					a.toast(locales.PrsRemoveWorktreeFailed(err.Error()))
				}
				a.refetchReviewWorktrees()
			})
		}
	}
	cf := &confirmation{open: true, title: locales.PrsRemoveWorktreeTitle(ra.number), detail: locales.PrsRemoveWorktreeDetail(path, ra.branch),
		action: locales.WorktreeRemove(), destroy: true, run: remove(false)}
	if ra.files > 0 {
		cf.detail = locales.PrsRemoveWorktreeDirty(ra.files, path) + " " + cf.detail
		cf.action, cf.run = locales.WorktreeRemoveAnyway(), remove(true)
	}
	a.confirm = cf
}

// agentButton goes to the session of a pull request's review agent, its
// status beside its label.
func (a *App) agentButton(c *ui.Context, pal *palette, s *proto.Session) {
	b := ui.ButtonBase(c.Key("agent")).Height(24).Padding(0, 8, 0, 6).Gap(5).Radius(6).AlignItems(ui.Center).Shrink(0).
		TextColor(pal.textMuted).Label(locales.PrsOpenAgent()).Tooltip(locales.PrsOpenAgent())
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() {
		statusIconP(c, pal, s.Status, s.Unread, s.Progress, 12)
		ui.Text(c, locales.PrsAgent()).FontSize(12).FontWeight(500).SingleLine()
	})
	if b.Clicked() {
		a.selectSession(s.ID)
	}
}

// reviewBadge marks a review session's row in the sidebar.
func reviewBadge(c *ui.Context, pal *palette, s *proto.Session) {
	if s.Review == nil {
		return
	}
	ui.Box(c.Key("review-badge")).Height(16).Padding(0, 5).Radius(4).Center().Shrink(0).Background(pal.accent.Alpha(0.14)).
		Tooltip(locales.SessionReviewTip(s.Review.Number, s.Review.Slug)).Children(func() {
		ui.Text(c, locales.SessionReview()).FontSize(10.5).FontWeight(600).TextColor(pal.accent).SingleLine()
	})
}
