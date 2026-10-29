package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// The bar atop the Changes and History views: the worktree's branch, which
// opens the branch switcher; how far it is from its upstream, which a
// click syncs; and the menu of the rest, fetching, pulling, pushing and
// stashing with their options.

// branchMenu is the branch switcher's: open, what is typed, the row the
// arrows are on, and the branches as kerod last listed them.
type branchMenu struct {
	open   bool
	query  string
	hi     int
	reveal bool
	pane   string
	root   string
	list   *proto.GitBranches
	busy   bool
	err    string
	at     ui.Rect
}

// gitBar is a worktree's bar, of the pane the view is of.
func (a *App) gitBar(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane) {
	g := pane.Git
	ui.Row(c.Key("git-bar")).Height(32).Shrink(0).Padding(2, 8, 2, 8).Gap(2).AlignItems(ui.Center).Children(func() {
		trig := ui.ButtonBase(c.Key("branch")).Height(26).Padding(0, 8).Gap(6).Radius(7).AlignItems(ui.Center).MinWidth(0).Shrink(1).
			Label(locales.GitSwitchBranch()).Tooltip(locales.GitSwitchBranch())
		m := &a.side.git.menu
		if trig.Hovered() || m.open {
			trig.Background(pal.rowHover)
		}
		trig.Children(func() {
			ui.Icon(c, branchIcon).Size(12, 12).TextColor(pal.textFaint).Shrink(0)
			ui.Text(c, branchName(g)).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
			ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(pal.textFaint).Shrink(0)
		})
		if trig.Clicked() {
			if m.open {
				m.open = false
			} else {
				a.openBranches(pane.ID, g.Root, trig.Bounds())
			}
		}
		a.branchPopover(c, pal, s, pane, trig)
		if st := gitState(g.State); st != "" {
			ui.Text(c, st).FontSize(12).TextColor(pal.attention).SingleLine().Shrink(0).Margin(0, 0, 0, 4)
		}
		ui.Spacer(c)
		a.gitRunning(c, pal, g.Root)
		a.syncButton(c, pal, pane)
		more := ui.ButtonBase(c.Key("more")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.GitMore()).Tooltip(locales.GitMore())
		if more.Hovered() || more.Pressed() {
			more.Background(pal.rowHover).TextColor(pal.text)
		}
		more.Children(func() { ui.Icon(c, moreIcon).Size(16, 16) })
		more.Menu(func(mm *ui.Menu) { a.gitMenu(mm, s, pane) })
	})
	a.gitStrip(c, pal, g.Root)
}

// syncButton syncs the branch with its upstream: pulls what it's behind,
// then pushes what it's ahead, or fetches when it's neither; a branch
// with no upstream it publishes.
func (a *App) syncButton(c *ui.Context, pal *palette, pane *proto.Pane) {
	g := pane.Git
	busy := a.gitBusy(g.Root)
	label, tip := "", ""
	svg := syncIcon
	switch {
	case g.Branch != "" && g.Upstream == "":
		svg, label, tip = pushIcon, locales.GitPublish(), locales.GitPublishTip()
	case g.Ahead > 0 || g.Behind > 0:
		if g.Ahead > 0 {
			label = "↑" + itoa(g.Ahead)
		}
		if g.Behind > 0 {
			label = strings.TrimSpace(label + " ↓" + itoa(g.Behind))
		}
		tip = locales.GitSyncTip(g.Upstream)
	default:
		tip = locales.GitFetchTip(g.Upstream)
	}
	b := ui.ButtonBase(c.Key("sync")).Height(26).Padding(0, 7).Gap(5).Radius(6).AlignItems(ui.Center).Shrink(0).
		TextColor(pal.textMuted).Label(tip).Tooltip(tip)
	// While another operation runs, which the bar shows beside it
	// (gitRunning), it waits, faded.
	if busy {
		b.Disabled(true).Opacity(0.45)
	} else if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() {
		ui.Icon(c, svg).Size(13, 13)
		if label != "" {
			ui.Text(c, label).FontSize(12.5).FontFeatures("tnum").SingleLine()
		}
	})
	if b.Clicked() && !busy {
		a.defer_(func() { a.sync(pane) })
	}
}

// sync is the sync button's click.
func (a *App) sync(pane *proto.Pane) {
	g := pane.Git
	switch {
	case g.Branch != "" && g.Upstream == "":
		a.push(pane.ID, g.Root, proto.GitOpParams{})
	case g.Behind > 0:
		ahead := g.Ahead > 0
		a.pull(pane.ID, g.Root, git.PullDefault, func() {
			if ahead {
				a.push(pane.ID, g.Root, proto.GitOpParams{})
			}
		})
	case g.Ahead > 0:
		a.push(pane.ID, g.Root, proto.GitOpParams{})
	default:
		a.runGit(pane.ID, g.Root, proto.GitOpParams{Op: "fetch"}, locales.GitFetch(), nil)
	}
}

// pull pulls into a worktree's branch, which changes its files: it waits
// for the agents there, or goes ahead as the user says.
func (a *App) pull(pane, root, mode string, then func()) {
	a.guarded(pane, root, locales.GitPull(), func() {
		a.runGit(pane, root, proto.GitOpParams{Op: "pull", Mode: mode}, locales.GitPull(), func(string) {
			if then != nil {
				then()
			}
		})
	})
}

func (a *App) push(pane, root string, p proto.GitOpParams) {
	p.Op = "push"
	a.runGit(pane, root, p, locales.GitPush(), nil)
}

// gitMenu is the bar's menu.
func (a *App) gitMenu(m *ui.Menu, s *proto.Session, pane *proto.Pane) {
	g := pane.Git
	id, root := pane.ID, g.Root
	fetch := func(p proto.GitOpParams) {
		p.Op = "fetch"
		a.runGit(id, root, p, locales.GitFetch(), nil)
	}
	if m.Item(locales.GitFetch()).Chosen() {
		fetch(proto.GitOpParams{})
	}
	if m.Item(locales.GitFetchAll()).Chosen() {
		fetch(proto.GitOpParams{All: true})
	}
	if m.Item(locales.GitFetchPrune()).Chosen() {
		fetch(proto.GitOpParams{All: true, Prune: true})
	}
	m.Separator()
	noUpstream := g.Upstream == ""
	for _, pm := range []struct {
		mode  string
		label string
	}{{git.PullDefault, locales.GitPull()}, {git.PullMerge, locales.GitPullMerge()}, {git.PullRebase, locales.GitPullRebase()}, {git.PullFF, locales.GitPullFF()}} {
		if m.Item(pm.label).Disabled(noUpstream).Chosen() {
			a.pull(id, root, pm.mode, nil)
		}
	}
	m.Separator()
	if m.Item(locales.GitPush()).Disabled(g.Branch == "").Chosen() {
		a.push(id, root, proto.GitOpParams{})
	}
	if m.Item(locales.GitPushForce()).Disabled(g.Branch == "" || noUpstream).Chosen() {
		a.confirm = &confirmation{open: true, title: locales.GitPushForceTitle(g.Branch), detail: locales.GitPushForceDetail(g.Upstream),
			action: locales.GitPushForce(), destroy: true, run: func() { a.push(id, root, proto.GitOpParams{Force: true}) }}
	}
	if m.Item(locales.GitPushTags()).Disabled(g.Branch == "").Chosen() {
		a.push(id, root, proto.GitOpParams{Tags: true})
	}
	m.Separator()
	stash := func(p proto.GitOpParams) {
		p.Op = "stash"
		a.guarded(id, root, locales.GitStash(), func() { a.runGit(id, root, p, locales.GitStash(), nil) })
	}
	none := g.Files == 0
	if m.Item(locales.GitStashAll()).Disabled(none).Chosen() {
		stash(proto.GitOpParams{IncludeUntracked: true})
	}
	if m.Item(locales.GitStashStaged()).Disabled(g.Staged == 0).Chosen() {
		stash(proto.GitOpParams{Staged: true})
	}
	if m.Item(locales.GitStashMessage()).Disabled(none).Chosen() {
		a.askName(locales.GitStashMessageTitle(), "", locales.GitStashMessagePlaceholder(), "", locales.GitStash(), func(msg string) {
			stash(proto.GitOpParams{IncludeUntracked: true, Message: msg})
		})
	}
	m.Separator()
	if m.Item(locales.GitNewBranch()).Chosen() {
		a.askNewBranch(id, root, "", "")
	}
	if m.Item(locales.GitUndoCommit()).Disabled(g.Head == "").Chosen() {
		a.confirm = &confirmation{open: true, title: locales.GitUndoCommitTitle(), detail: locales.GitUndoCommitDetail(),
			action: locales.GitUndoCommit(), run: func() { a.runGit(id, root, proto.GitOpParams{Op: "undo"}, locales.GitUndoCommit(), nil) }}
	}
	if m.Item(locales.GitRefresh()).Chosen() {
		a.side.git.done++
	}
}

// askNewBranch asks a new branch's name, to make at start ("" HEAD), named
// as at, and check it out.
func (a *App) askNewBranch(pane, root, start, at string) {
	detail := locales.GitNewBranchDetail()
	if at != "" {
		detail = locales.GitNewBranchAt(at)
	}
	a.askName(locales.GitNewBranchTitle(), detail, locales.GitBranchName(), "", locales.GitCreateBranch(), func(name string) {
		a.createBranch(pane, root, name, start)
	})
}

// createBranch makes a branch at start and checks it out: from HEAD, as
// its files are, which waits for no agent; from another commit, which
// changes them, as a switch does.
func (a *App) createBranch(pane, root, name, start string) {
	run := func() {
		a.runGit(pane, root, proto.GitOpParams{Op: "createBranch", Name: name, Rev: start, Checkout: true}, locales.GitCreateNamed(name), nil)
	}
	if start == "" {
		run()
		return
	}
	a.guarded(pane, root, locales.GitSwitchTo(name), run)
}

// openBranches opens the branch switcher, listing the branches anew.
func (a *App) openBranches(pane, root string, at ui.Rect) {
	m := &a.side.git.menu
	if m.root != root {
		m.list = nil
	}
	m.open, m.query, m.hi, m.reveal, m.pane, m.root, m.at, m.err = true, "", 0, true, pane, root, at, ""
	cl := a.hostOf(pane).cl
	if cl == nil || m.busy {
		return
	}
	m.busy = true
	go func() {
		var res proto.GitBranches
		err := cl.Call("git.branches", proto.GitParams{Pane: pane}, &res)
		a.update(func() {
			m.busy = false
			if m.root != root {
				return
			}
			if err != nil {
				m.err = err.Error()
				return
			}
			m.list = &res
		})
	}()
}

// branchRow is a row of the branch switcher: a branch, or the branch the
// query names, to make.
type branchRow struct {
	b      *proto.GitBranch
	create string
	label  string // the section it starts, "" for none
}

// branchRows are the switcher's rows, as the query filters them: local
// branches, then the remotes' that no local branch has the name of, then
// the tags.
func branchRows(list *proto.GitBranches, query string) []branchRow {
	q := strings.ToLower(strings.TrimSpace(query))
	var rows []branchRow
	exact := false
	local := map[string]bool{}
	for i := range list.Branches {
		b := &list.Branches[i]
		if !b.Remote && !b.Tag {
			local[b.Name] = true
			exact = exact || strings.ToLower(b.Name) == q
		}
	}
	if q != "" && !exact {
		rows = append(rows, branchRow{create: strings.TrimSpace(query)})
	}
	sections := []struct {
		label       string
		remote, tag bool
	}{{locales.GitBranches(), false, false}, {locales.GitRemoteBranches(), true, false}, {locales.GitTags(), false, true}}
	for _, sec := range sections {
		remote := sec.remote
		first := true
		for i := range list.Branches {
			b := &list.Branches[i]
			if b.Remote != remote || b.Tag != sec.tag || q != "" && !strings.Contains(strings.ToLower(b.Name), q) {
				continue
			}
			if remote {
				if _, name, ok := strings.Cut(b.Name, "/"); ok && local[name] {
					continue
				}
			}
			r := branchRow{b: b}
			if first {
				r.label, first = sec.label, false
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// branchPopover is the branch switcher, below its button.
func (a *App) branchPopover(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, trig ui.Element) {
	m := &a.side.git.menu
	if m.open && m.root != pane.Git.Root {
		m.open = false
	}
	if b := trig.Bounds(); m.open && (abs(b.X-m.at.X) > 0.5 || abs(b.Y-m.at.Y) > 0.5) {
		m.open = false
	}
	if m.open {
		ui.Overlay(c, func() {
			back := ui.ButtonBase(c.Key("branch-back")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Label(locales.CloseTheMenu())
			if back.Clicked() {
				m.open = false
			}
		})
	}
	_, h := c.Size()
	listH := max(menuItemH*4, min(380, h-trig.Bounds().Y-trig.Bounds().H-120))
	ui.PopoverBase(c.Key("branch-menu"), trig, &m.open, func(panel ui.Element) {
		menuPanel(panel, pal).Margin(4, 0, 0, 0).Width(330)
		var rows []branchRow
		if m.list != nil {
			rows = branchRows(m.list, m.query)
		}
		m.hi = min(max(m.hi, 0), max(len(rows)-1, 0))
		pick := func(i int) {
			if i < 0 || i >= len(rows) {
				return
			}
			m.open = false
			r := rows[i]
			if r.create != "" {
				name := r.create
				a.defer_(func() { a.createBranch(pane.ID, pane.Git.Root, name, "") })
				return
			}
			b := *r.b
			a.defer_(func() { a.switchBranch(s, pane, b, "") })
		}
		var in ui.Element
		frame := ui.Row(c.Key("search")).Margin(3, 3, menuPad+3, 3).Cursor(ui.CursorText)
		frame.Children(func() {
			ui.Icon(c, searchIcon).Size(12, 12).TextColor(pal.textFaint).Shrink(0)
			in = ui.TextInputBase(c.Key("q"), &m.query).AutoFocus().Grow(1).MinWidth(0).AlignSelf(ui.Stretch).FontSize(13).
				Placeholder(locales.GitBranchFilter()).Label(locales.GitBranchFilter())
		})
		field(c, frame, in.Focused(), pal)
		in.OnChange(func() { m.hi, m.reveal = 0, true })
		in.OnShortcut(0, ui.KeyDown, func() { m.hi, m.reveal = min(m.hi+1, len(rows)-1), true })
		in.OnShortcut(0, ui.KeyUp, func() { m.hi, m.reveal = max(m.hi-1, 0), true })
		in.OnShortcut(0, ui.KeyEscape, func() { m.open = false })
		in.OnSubmit(func() { pick(m.hi) })
		switch {
		case m.list == nil && m.err != "":
			ui.Text(c, m.err).FontSize(12.5).TextColor(pal.textMuted).Padding(6, 12)
			return
		case m.list == nil:
			ui.Column(c).Height(60).Center().Children(func() { ui.Spinner(c) })
			return
		case len(rows) == 0:
			ui.Text(c, locales.CommonNoResults()).FontSize(13).TextColor(pal.textFaint).Padding(6, 12)
			return
		}
		reveal := m.reveal
		m.reveal = false
		now := time.Now()
		ui.Scroll(c.Key("rows")).MaxHeight(listH).Children(func() {
			for i, r := range rows {
				if r.label != "" {
					ui.Text(c, r.label).FontSize(11.5).FontWeight(600).TextColor(pal.textFaint).Padding(6, 10, 2, 10)
				}
				key, label := "new", locales.GitCreateNamed(r.create)
				if r.b != nil {
					key, label = r.b.Name, r.b.Name
				}
				row := ui.Row(c.Key(key)).Label(label)
				if row.Hovered() && m.hi != i {
					m.hi = i
					c.After(0)
				}
				if row.Clicked() {
					pick(i)
				}
				hi := i == m.hi
				row.Height(menuItemH+2).Padding(0, 10, 0, 6).Gap(6).Radius(7).AlignItems(ui.Center)
				fg, faint := pal.text, pal.textFaint
				if hi {
					row.Background(pal.accent)
					fg, faint = ui.Hex("#ffffff"), ui.Hex("#ffffff").Alpha(0.75)
				}
				if r.b != nil && r.b.Worktree != "" && !r.b.Current {
					// Checked out in another worktree, where git won't let
					// it be checked out again.
					row.Tooltip(locales.GitBranchInWorktree(a.hostOf(pane.ID).shortDir(r.b.Worktree)))
				}
				b := r.b
				if b != nil {
					row.ContextMenu(func(mm *ui.Menu) { a.branchMenuItems(mm, s, pane, *b) })
				}
				row.TextColor(fg).Children(func() {
					switch {
					case b == nil:
						ui.Icon(c, plusIcon).Size(12, 12).Shrink(0)
					case b.Current:
						ui.Icon(c, checkIcon).Size(12, 12).Shrink(0)
					default:
						ui.Box(c).Size(12, 12).Shrink(0)
					}
					ui.Text(c, label).FontSize(13).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
					if b == nil {
						return
					}
					ui.Spacer(c)
					var bits []string
					if b.Ahead > 0 {
						bits = append(bits, "↑"+itoa(b.Ahead))
					}
					if b.Behind > 0 {
						bits = append(bits, "↓"+itoa(b.Behind))
					}
					if b.Gone {
						bits = append(bits, locales.GitGone())
					}
					if b.Worktree != "" && !b.Current {
						bits = append(bits, locales.GitInWorktree())
					}
					bits = append(bits, ago(b.Time*1000, now))
					ui.Text(c, strings.Join(bits, " · ")).FontSize(11.5).FontFeatures("tnum").TextColor(faint).SingleLine().Shrink(0)
				})
				if reveal && hi {
					row.ScrollIntoView()
				}
			}
		})
	})
}

// branchMenuItems are a branch's menu, in the switcher.
func (a *App) branchMenuItems(m *ui.Menu, s *proto.Session, pane *proto.Pane, b proto.GitBranch) {
	id, root := pane.ID, pane.Git.Root
	if m.Item(locales.GitSwitch()).Disabled(b.Current).Chosen() {
		a.side.git.menu.open = false
		a.switchBranch(s, pane, b, "")
	}
	if m.Item(locales.GitNewBranchFrom()).Chosen() {
		a.side.git.menu.open = false
		a.askNewBranch(id, root, b.Name, b.Name)
	}
	if m.Item(locales.GitOpenInWorktree()).Disabled(b.Current || b.Worktree != "" || b.Tag).Chosen() {
		a.side.git.menu.open = false
		a.branchSession(s, pane, b)
	}
	m.Separator()
	a.mergeItems(m, pane, b.Name, b.Name, b.Current)
	m.Separator()
	if m.Item(locales.GitRenameBranch()).Disabled(b.Remote || b.Tag).Chosen() {
		a.side.git.menu.open = false
		a.askName(locales.GitRenameBranchTitle(b.Name), "", locales.GitBranchName(), b.Name, locales.GitRename(), func(to string) {
			a.runGit(id, root, proto.GitOpParams{Op: "renameBranch", Name: b.Name, To: to}, locales.GitRenameNamed(b.Name), nil)
		})
	}
	if m.Item(locales.GitDeleteBranch()).Disabled(b.Remote || b.Tag || b.Current || b.Worktree != "").Chosen() {
		a.side.git.menu.open = false
		a.deleteBranch(id, root, b.Name, false)
	}
}

// mergeItems are a branch's or a commit's items that bring it into the
// worktree's branch: merged, as a fast forward where it can be or always
// as a commit, or the branch rebased onto it. head: it is HEAD's own.
func (a *App) mergeItems(m *ui.Menu, pane *proto.Pane, rev, name string, head bool) {
	g := pane.Git
	branch := menuName(branchName(g))
	label := menuName(name)
	off := head || !canMergeInto(g)
	if m.Item(locales.GitMergeInto(label, branch)).Disabled(off).Chosen() {
		a.side.git.menu.open = false
		a.mergeInto(pane, rev, name, false)
	}
	if m.Item(locales.GitMergeIntoNoFF(label, branch)).Disabled(off).Chosen() {
		a.side.git.menu.open = false
		a.mergeInto(pane, rev, name, true)
	}
	if m.Item(locales.GitRebaseOnto(branch, label)).Disabled(off || g.Branch == "").Chosen() {
		a.side.git.menu.open = false
		a.rebaseOnto(pane, rev, name)
	}
	if m.Item(locales.GitRebaseInteractive(branch, label)).Disabled(off || g.Branch == "").Chosen() {
		a.side.git.menu.open = false
		a.openRebase(pane, rev)
	}
}

// menuName is a branch's name as a menu's item says it: a long one cut in
// its middle, keeping its start, as a ticket's key, and its end, so that
// the menu doesn't grow across the window.
func menuName(name string) string {
	const head, tail = 22, 12
	r := []rune(name)
	if len(r) <= head+tail+1 {
		return name
	}
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

// deleteBranch deletes a local branch once the user says so; one whose
// commits no other branch has asks again, to delete them with it.
func (a *App) deleteBranch(pane, root, name string, force bool) {
	title, detail, label := locales.GitDeleteBranchTitle(name), locales.GitDeleteBranchDetail(), locales.GitDeleteBranch()
	if force {
		title, detail, label = locales.GitDeleteUnmergedTitle(name), locales.GitDeleteUnmergedDetail(), locales.GitDeleteAnyway()
	}
	a.confirm = &confirmation{open: true, title: title, detail: detail, action: label, destroy: true, run: func() {
		a.runGitFor(pane, root, proto.GitOpParams{Op: "deleteBranch", Name: name, Force: force}, locales.GitDeleteNamed(name), nil,
			func(summary string) bool {
				if !force && strings.Contains(summary, "not fully merged") {
					a.deleteBranch(pane, root, name, true)
					return true
				}
				return false
			})
	}}
}

// switchBranch checks out a branch, which changes the worktree's files: it
// waits for the agents there, or goes ahead as the user says, and asks
// what becomes of the changes not committed, stashed or taken along. at,
// when set, is the commit the branch is chosen at, in the history.
func (a *App) switchBranch(s *proto.Session, pane *proto.Pane, b proto.GitBranch, at string) {
	if b.Current {
		return
	}
	g := pane.Git
	id, root := pane.ID, g.Root
	if b.Worktree != "" {
		a.toast(locales.GitBranchInWorktree(a.hostOf(id).shortDir(b.Worktree)))
		return
	}
	name := b.Name
	op := proto.GitOpParams{Op: "switch", Name: name, Remote: b.Remote, Rev: at}
	if b.Tag {
		// A tag is checked out as its commit, HEAD detached.
		op = proto.GitOpParams{Op: "checkout", Rev: b.Head}
	}
	sw := func() {
		a.runGit(id, root, op, locales.GitSwitchTo(name), nil)
	}
	a.guarded(id, root, locales.GitSwitchTo(name), func() {
		if g.Files == 0 {
			sw()
			return
		}
		a.confirm = &confirmation{open: true, title: locales.GitSwitchDirtyTitle(), detail: locales.GitSwitchDirtyDetail(g.Files, name),
			action: locales.GitStashAndSwitch(), run: func() {
				a.runGit(id, root, proto.GitOpParams{Op: "stash", IncludeUntracked: true, Message: locales.GitStashSwitchMessage(name)}, locales.GitStash(), func(string) { sw() })
			},
			alt: locales.GitTakeChanges(), altRun: sw}
	})
}

// branchSession starts a session in a worktree of its own on a branch: a
// remote's as a local branch of its name.
func (a *App) branchSession(s *proto.Session, pane *proto.Pane, b proto.GitBranch) {
	h := a.hostOf(pane.ID)
	p := proto.CreateParams{Project: s.Project, After: s.ID, Dir: pane.Git.Root, Cols: s.Cols, Rows: s.Rows, Worktree: b.Name}
	if b.Remote {
		if _, name, ok := strings.Cut(b.Name, "/"); ok {
			p.Worktree, p.Base = name, b.Name
		}
	}
	a.create(h, p, s.Project)
}
