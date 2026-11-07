package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/github"
	"kero/internal/proto"
	"kero/internal/settings"

	"kero/locales"
)

// A pull request's actions, on its page and in the list's menu: merge it,
// with the repository's default method or another, mark it ready or make
// it a draft again, close or reopen it, delete or update its branch,
// auto-merge, revert it, and approve the workflow runs a fork's commits
// wait on. kerod does each with gh (pr.action), and reads after it what
// the next depend on.

// prActs is the page's pull request's actions: what they depend on, as
// last read, of the page's gen it was read for. A read that fails, as of
// a kerod older than pr.action, shows no actions: the page says what goes
// wrong with gh itself.
type prActs struct {
	st   *proto.PRActions
	of   int    // the page's gen st is of; 0 for none read
	busy string // the op under way, "" for none
	gen  int
}

// prMerge is the merge being asked for: how, and of what.
type prMerge struct {
	open         bool
	pane, remote string
	slug         string
	pr           proto.PullRequest
	page         *prPage // the page it was asked from, nil for the list
	st           *proto.PRActions
	err          string
	method       string
	deleteBranch bool
	remember     bool // make method the repository's default
}

// defaultMerge is how a pull request of slug merges unless the user says
// otherwise: the setup's method, if the repository allows it, else the
// first it allows of github.DefaultMethod's order.
func (a *App) defaultMerge(slug string, allowed []string) string {
	return github.DefaultMethod(a.set.Reviews[strings.ToLower(slug)].MergeMethod, allowed)
}

// mergeLabel is the button that merges with method.
func mergeLabel(method string) string {
	switch method {
	case github.MethodSquash:
		return locales.PrsMethodSquash()
	case github.MethodRebase:
		return locales.PrsMethodRebase()
	}
	return locales.PrsMethodMerge()
}

// prAct does op to a pull request of a pane's repository's remote, then
// shows what came of it: on the page it was done from, the state read
// after it and the pull request read anew, and the list read anew.
func (a *App) prAct(pane, remote string, pr proto.PullRequest, g *prPage, p proto.PRActionParams) {
	cl := a.hostOf(pane).cl
	if cl == nil {
		a.toast(locales.HostNotConnected(a.hostOf(pane).label()))
		return
	}
	p.Pane, p.Remote, p.Number = pane, remote, pr.Number
	if g != nil {
		if g.acts.busy != "" {
			return
		}
		g.acts.busy = p.Op
	}
	go func() {
		var res proto.PRActionResult
		err := cl.Call("pr.action", p, &res)
		a.update(func() {
			if g != nil {
				g.acts.busy = ""
			}
			if err != nil {
				a.toast(locales.PrsActionFailed(pr.Number, err.Error()))
				return
			}
			switch p.Op {
			case proto.PRActMerge:
				a.toast(locales.PrsDoneMerged(pr.Number))
			case proto.PRActAutoMerge:
				a.toast(locales.PrsDoneAutoMerge(pr.Number))
			case proto.PRActRevert:
				a.toast(locales.PrsDoneReverted(pr.Number, res.URL))
			case proto.PRActApproveRuns:
				a.toast(locales.PrsDoneApproved(res.Approved))
			case proto.PRActDeleteBranch:
				a.toast(locales.PrsDoneBranchDeleted(pr.Head))
			}
			// The list, of open pull requests, shows what changed as it
			// comes back to it.
			a.side.pulls.fetched = time.Time{}
			if g != nil && a.side.pulls.page == g {
				a.fetchPRPage(g)
				if res.State != nil {
					g.acts.st, g.acts.of = res.State, g.gen
				}
			}
		})
	}()
}

// fetchPRActs reads what the page's actions depend on.
func (a *App) fetchPRActs(g *prPage) {
	cl := a.hostOf(g.pane).cl
	if cl == nil {
		return
	}
	g.acts.of = g.gen
	g.acts.gen++
	gen := g.acts.gen
	p := proto.PRActionParams{Pane: g.pane, Remote: g.remote, Number: g.pr.Number}
	go func() {
		var res proto.PRActionResult
		err := cl.Call("pr.action", p, &res)
		a.update(func() {
			if g.acts.gen != gen {
				return
			}
			if err == nil && res.State != nil {
				g.acts.st = res.State
			}
		})
	}()
}

// askMerge opens the merge's dialog, which reads, unless st has it, what
// the merge depends on.
func (a *App) askMerge(pane, remote, slug string, pr proto.PullRequest, g *prPage, st *proto.PRActions) {
	m := &prMerge{open: true, pane: pane, remote: remote, slug: slug, pr: pr, page: g, st: st, method: a.defaultMerge(slug, nil)}
	a.prMerge = m
	if st != nil {
		m.method = a.defaultMerge(slug, st.Methods)
		return
	}
	cl := a.hostOf(pane).cl
	if cl == nil {
		m.err = locales.HostNotConnected(a.hostOf(pane).label())
		return
	}
	p := proto.PRActionParams{Pane: pane, Remote: remote, Number: pr.Number}
	go func() {
		var res proto.PRActionResult
		err := cl.Call("pr.action", p, &res)
		a.update(func() {
			if a.prMerge != m {
				return
			}
			switch {
			case err != nil:
				m.err = err.Error()
			case res.State != nil:
				m.st, m.method = res.State, a.defaultMerge(slug, res.State.Methods)
			}
		})
	}()
}

// mergeNote says whether a pull request merges as it is, and why not; ok
// is false when it can't merge at all, as with conflicts.
func mergeNote(st *proto.PRActions) (note string, ok bool) {
	switch {
	case st.State == "MERGED":
		return locales.PrsMerged(), false
	case st.State == "CLOSED":
		return locales.PrsClosed(), false
	case !st.CanWrite:
		return locales.PrsMergeNoAccess(), false
	case st.Draft:
		return locales.PrsMergeDraft(), false
	case st.Mergeable == "CONFLICTING" || st.MergeState == "DIRTY":
		return locales.PrsMergeConflicts(st.Base), false
	case st.AutoMerge != "":
		return locales.PrsAutoMergeOn(methodName(st.AutoMerge)), true
	}
	switch st.MergeState {
	case "CLEAN", "HAS_HOOKS":
		return locales.PrsMergeReady(), true
	case "BEHIND":
		return locales.PrsMergeBehind(st.Base), true
	case "BLOCKED":
		return locales.PrsMergeBlocked(), true
	case "UNSTABLE":
		return locales.PrsMergeUnstable(), true
	}
	return locales.PrsMergeUnknown(), true
}

// mergeNow tells a pull request GitHub merges at once, which auto-merge
// would merge as it is enabled: GitHub refuses that.
func mergeNow(st *proto.PRActions) bool {
	switch st.MergeState {
	case "CLEAN", "HAS_HOOKS", "UNSTABLE":
		return true
	}
	return false
}

// prMergeView is the merge's dialog: the method, of those the repository
// allows, whether its branch goes after, and Merge, or, while its
// requirements aren't met, Enable Auto-Merge beside it.
func (a *App) prMergeView(c *ui.Context, pal *palette) {
	m := a.prMerge
	if m == nil {
		return
	}
	op := ""
	ui.DialogBase(c.Key("pr-merge"), &m.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.PrsMergeTitle(m.pr.Number)
		// As wide as its buttons on one line, as confirmDialog is.
		winW, _ := c.Size()
		labels, keys := []string{locales.CommonCancel(), locales.PrsEnableAutoMerge(), mergeLabel(m.method)}, []string{"esc", "", "↩"}
		width := max(460, min(buttonsWidth(c, labels, keys)+44+1, winW-40))
		floating(panel.Width(width).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).Label(title)
		ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(pal.text)
		ui.Text(c, m.pr.Title).FontSize(13).FontWeight(550).TextColor(pal.textMuted).SingleLine().Ellipsis("…")
		st := m.st
		switch {
		case st == nil && m.err != "":
			_, removed, _ := a.diffColors(c, pal)
			ui.Text(c, m.err).FontSize(12.5).TextColor(removed).LineHeight(1.4).Selectable()
		case st == nil:
			ui.Row(c).Height(60).Center().Children(func() { ui.Spinner(c) })
		}
		ok := false
		if st != nil {
			var note string
			note, ok = mergeNote(st)
			ui.Text(c, note).FontSize(12.5).TextColor(pal.textFaint).LineHeight(1.4)
			if ok && len(st.Methods) > 1 {
				labels := make([]string, len(st.Methods))
				for i, id := range st.Methods {
					labels[i] = methodName(id)
				}
				ui.Row(c.Key("method")).Margin(6, 0, 0, 0).Children(func() {
					segmented(c, "method", &m.method, st.Methods, labels)
				})
			}
			if ok && !st.DeleteOnMerge && st.CanDeleteBranch && !st.CrossRepo {
				ui.Checkbox(c.Key("delete"), &m.deleteBranch, locales.PrsMergeDeleteBranch()).Margin(4, 0, 0, 0)
			}
			if ok && m.method != a.defaultMerge(m.slug, st.Methods) {
				ui.Checkbox(c.Key("remember"), &m.remember, locales.PrsMergeRemember()).Margin(2, 0, 0, 0)
			}
		}
		dialogButtons(c, func() {
			if button(c, "cancel", locales.CommonCancel(), "esc", false, pal) {
				m.open = false
			}
			if !ok {
				return
			}
			auto := st.AutoMergeAllowed && st.AutoMerge == "" && !mergeNow(st)
			if auto && button(c, "auto", locales.PrsEnableAutoMerge(), "", !mergeNow(st), pal) {
				op = proto.PRActAutoMerge
			}
			if defaultButton(c, "ok", mergeLabel(m.method), "↩", !auto, pal) {
				op = proto.PRActMerge
			}
		})
	})
	if op != "" {
		m.open = false
		if m.remember {
			a.setMergeDefault(m.slug, m.method)
		}
		g := m.page
		if g != nil && a.side.pulls.page != g {
			g = nil
		}
		a.prAct(m.pane, m.remote, m.pr, g, proto.PRActionParams{Op: op, Method: m.method, DeleteBranch: m.deleteBranch, Head: m.st.HeadOID})
	}
	if !m.open {
		a.prMerge = nil
	}
}

// methodName is a merge method, as the dialog's choice names it.
func methodName(method string) string {
	switch method {
	case github.MethodSquash:
		return locales.PrsMethodSquashShort()
	case github.MethodRebase:
		return locales.PrsMethodRebaseShort()
	}
	return locales.PrsMethodMergeShort()
}

// setMergeDefault makes method how slug's pull requests merge by default.
func (a *App) setMergeDefault(slug, method string) {
	set := a.set.Clone()
	if set.Reviews == nil {
		set.Reviews = map[string]settings.ReviewSetup{}
	}
	key := strings.ToLower(slug)
	r := set.Reviews[key]
	r.MergeMethod = method
	set.Reviews[key] = r
	a.applySettings(set)
}

// askPRAct asks before op, which is no undoing, or not at once.
func (a *App) askPRAct(pane, remote string, pr proto.PullRequest, g *prPage, op string, st *proto.PRActions) {
	run := func() { a.prAct(pane, remote, pr, g, proto.PRActionParams{Op: op}) }
	cf := &confirmation{open: true, run: run}
	switch op {
	case proto.PRActClose:
		cf.title, cf.detail, cf.action, cf.destroy = locales.PrsCloseTitle(pr.Number), locales.PrsCloseDetail(), locales.PrsClosePR(), true
	case proto.PRActDeleteBranch:
		cf.title, cf.detail, cf.action, cf.destroy = locales.PrsDeleteBranchTitle(pr.Head), locales.PrsDeleteBranchDetail(), locales.PrsDeleteBranch(), true
	case proto.PRActRevert:
		cf.title, cf.detail, cf.action = locales.PrsRevertTitle(pr.Number), locales.PrsRevertDetail(pr.Base), locales.PrsRevert()
	case proto.PRActApproveRuns:
		n := 0
		head := ""
		if st != nil {
			n, head = st.PendingRuns, st.HeadOID
		}
		cf.title, cf.detail, cf.action = locales.PrsApproveTitle(pr.Number), locales.PrsApproveDetail(), locales.PrsApproveRuns(n)
		// Only the runs of the commits looked at.
		cf.run = func() { a.prAct(pane, remote, pr, g, proto.PRActionParams{Op: op, Head: head}) }
	default:
		run()
		return
	}
	a.confirm = cf
}

// prActionsBar is the page's actions, under its title: whether it merges
// and why not, its main action (Merge, Ready for Review, Reopen, Revert),
// Approve Runs while a fork's runs wait, and the rest in a menu.
func (a *App) prActionsBar(c *ui.Context, pal *palette, g *prPage) {
	if g.acts.of != g.gen && !g.busy {
		a.fetchPRActs(g)
	}
	st := g.acts.st
	if st == nil {
		return
	}
	pr := g.pr
	act := func(op string) { a.askPRAct(g.pane, g.remote, pr, g, op, st) }
	ui.Row(c.Key("acts")).Gap(6).AlignItems(ui.Center).Wrap().Margin(2, 0, 0, 0).Children(func() {
		note := ""
		if st.State == "OPEN" {
			note, _ = mergeNote(st)
		}
		if st.PendingRuns > 0 && st.CanWrite {
			note = locales.PrsRunsWaiting(st.PendingRuns)
		}
		// Room to read it: the buttons wrap under it before it narrows.
		ui.Text(c, note).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35).MinWidth(min(180, float32(len(note))*6)).Shrink(1).Grow(1)
		if g.acts.busy != "" {
			ui.Spinner(c).Size(12, 12).Margin(0, 4, 0, 0)
		}
		if st.PendingRuns > 0 && st.CanWrite && st.State == "OPEN" {
			if sizedButton(c, "approve", locales.PrsApproveRuns(st.PendingRuns), "", false, 26, 6, pal) {
				act(proto.PRActApproveRuns)
			}
		}
		label, op := "", ""
		switch {
		case st.State == "OPEN" && st.Draft && st.CanUpdate:
			label, op = locales.PrsMarkReady(), proto.PRActReady
		case st.State == "OPEN" && st.AutoMerge != "" && st.CanWrite:
			label, op = locales.PrsDisableAutoMerge(), proto.PRActDisableAutoMerge
		case st.State == "OPEN" && st.CanWrite:
			label, op = mergeLabel(a.defaultMerge(g.repoSlug(), st.Methods)), proto.PRActMerge
		case st.State == "CLOSED" && st.CanUpdate:
			label, op = locales.PrsReopenPR(), proto.PRActReopen
		case st.State == "MERGED" && st.HeadExists && st.CanDeleteBranch:
			label, op = locales.PrsDeleteBranch(), proto.PRActDeleteBranch
		case st.State == "MERGED" && st.CanWrite:
			label, op = locales.PrsRevert(), proto.PRActRevert
		}
		if op != "" && sizedButton(c, "main", label, "", op == proto.PRActMerge || op == proto.PRActReady, 26, 6, pal) && g.acts.busy == "" {
			if op == proto.PRActMerge {
				a.askMerge(g.pane, g.remote, g.repoSlug(), pr, g, st)
			} else {
				act(op)
			}
		}
		more := ui.ButtonBase(c.Key("more")).Size(26, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.PrsActions()).Tooltip(locales.PrsActions())
		if more.Hovered() || more.Pressed() {
			more.Background(pal.rowHover).TextColor(pal.text)
		}
		more.Children(func() { ui.Icon(c, moreIcon).Size(13, 13) })
		more.Menu(func(m *ui.Menu) {
			if g.acts.busy != "" {
				return
			}
			a.prActionItems(m, st, op, func(o string) {
				if o == proto.PRActMerge || o == proto.PRActAutoMerge {
					a.askMerge(g.pane, g.remote, g.repoSlug(), pr, g, st)
					return
				}
				if o == proto.PRActUpdateBranch+"/rebase" {
					a.prAct(g.pane, g.remote, pr, g, proto.PRActionParams{Op: proto.PRActUpdateBranch, Method: github.MethodRebase})
					return
				}
				act(o)
			})
		})
	})
}

// prActionItems are the items of the actions' menu, but main, the bar's
// button, as st allows them; each chosen goes to act, Update Branch with
// Rebase as PRActUpdateBranch+"/rebase".
func (a *App) prActionItems(m *ui.Menu, st *proto.PRActions, main string, act func(op string)) {
	// A group's separator goes in before its first item, after another's.
	shown, sep := false, false
	group := func() { sep = shown }
	item := func(op, label string) *ui.MenuItem {
		if op == main {
			return nil
		}
		if sep {
			m.Separator()
			sep = false
		}
		shown = true
		it := m.Item(label)
		if it.Chosen() {
			act(op)
		}
		return it
	}
	switch st.State {
	case "OPEN":
		if st.CanWrite && !st.Draft {
			item(proto.PRActMerge, locales.PrsMergeMenu())
			switch {
			case st.AutoMerge != "":
				item(proto.PRActDisableAutoMerge, locales.PrsDisableAutoMerge())
			case st.AutoMergeAllowed && !mergeNow(st):
				item(proto.PRActAutoMerge, locales.PrsEnableAutoMergeMenu())
			}
		}
		if st.CanUpdateBranch {
			group()
			item(proto.PRActUpdateBranch, locales.PrsUpdateBranch())
			item(proto.PRActUpdateBranch+"/rebase", locales.PrsUpdateBranchRebase())
		}
		if st.CanUpdate {
			group()
			if st.Draft {
				item(proto.PRActReady, locales.PrsMarkReady())
			} else {
				item(proto.PRActDraft, locales.PrsConvertToDraft())
			}
			if it := item(proto.PRActClose, locales.PrsClosePRMenu()); it != nil {
				it.Danger()
			}
		}
	case "CLOSED":
		if st.CanUpdate {
			item(proto.PRActReopen, locales.PrsReopenPR())
		}
	case "MERGED":
		if st.CanWrite {
			item(proto.PRActRevert, locales.PrsRevertMenu())
		}
	}
	if st.State != "OPEN" && st.HeadExists && st.CanDeleteBranch {
		group()
		if it := item(proto.PRActDeleteBranch, locales.PrsDeleteBranchMenu()); it != nil {
			it.Danger()
		}
	}
}

// pullRowActions are a list row's actions, in its menu: of an open pull
// request, as the list knows it, which GitHub refuses of those the user
// may not do. Merge reads the rest in its dialog.
func (a *App) pullRowActions(m *ui.Menu, pane *proto.Pane, remote, slug string, pr proto.PullRequest) {
	act := func(op string) { a.askPRAct(pane.ID, remote, pr, nil, op, nil) }
	m.Separator()
	if !pr.Draft && m.Item(locales.PrsMergeMenu()).Chosen() {
		a.askMerge(pane.ID, remote, slug, pr, nil, nil)
	}
	if m.Item(locales.PrsUpdateBranch()).Chosen() {
		act(proto.PRActUpdateBranch)
	}
	if pr.Draft {
		if m.Item(locales.PrsMarkReady()).Chosen() {
			act(proto.PRActReady)
		}
	} else if m.Item(locales.PrsConvertToDraft()).Chosen() {
		act(proto.PRActDraft)
	}
	if m.Item(locales.PrsClosePRMenu()).Danger().Chosen() {
		act(proto.PRActClose)
	}
}

// mergeDefaultField is the repository setup's default merge method, ""
// for the first GitHub allows of github.DefaultMethod's order.
func mergeDefaultField(c *ui.Context, pal *palette, method *string) {
	ui.Text(c.Key("merge-label"), locales.PrsSetupMerge()).FontSize(12.5).FontWeight(600).TextColor(pal.text).Margin(8, 0, 0, 0)
	ui.Row(c.Key("merge")).Children(func() {
		segmented(c, "merge", method, []string{"", github.MethodSquash, github.MethodMerge, github.MethodRebase},
			[]string{locales.PrsSetupMergeAuto(), locales.PrsMethodSquashShort(), locales.PrsMethodMergeShort(), locales.PrsMethodRebaseShort()})
	})
	ui.Text(c.Key("merge-hint"), locales.PrsSetupMergeHint()).FontSize(12).TextColor(pal.textFaint).LineHeight(1.4)
}
