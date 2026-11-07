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
	// folds are the groups and folders the user opened or folded, by
	// pickRow.key; older the groups showing the branches hidden as old;
	// scrollHi that the row the arrows are on is to scroll into view.
	folds    map[string]bool
	older    map[string]bool
	scrollHi bool
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
	// The stash, which History lists as it shows.
	a.fetchStashes(pane, a.side.git.hist.stashAt)
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

// The branch switcher lists, under a filter, the repository's worktrees,
// its branches in their folders, its remotes' and its tags, and the
// stash, each a group that folds. A click shows the commit in History's
// graph, chosen; a double click, or Return, checks a branch out, or opens
// a worktree's session. Local and remote branches with no commit in
// settings' BranchMaxWeeks hide until asked for, or a filter finds them.

// Kinds of the switcher's rows.
const (
	pickGroup = iota
	pickFolder
	pickBranch
	pickWorktree
	pickStash
	pickCreate
	pickOlder
)

// pickRow is a row of the switcher: a group's title, a folder of
// branches, a branch, a worktree, an entry of the stash, the branch the
// filter names, to make, or the row that shows the branches hidden as
// old, of group.
type pickRow struct {
	kind   int
	group  string // pickGroups' id
	key    string // what folds a group or folder, by group and folder
	name   string
	depth  int
	open   bool
	count  int // a group's rows, or the branches hidden as old
	b      *proto.GitBranch
	st     *proto.GitStash
	remote bool // a folder that is a remote
}

// The switcher's groups, in their order.
const (
	groupWorktrees = "worktrees"
	groupLocal     = "local"
	groupRemote    = "remote"
	groupTags      = "tags"
	groupStash     = "stash"
)

// groupOpen tells that a group of the switcher shows its rows: as the
// user left it, else Tags and the stash folded, the rest open.
func (m *branchMenu) groupOpen(group string) bool {
	if v, ok := m.folds[group]; ok {
		return v
	}
	return group != groupTags && group != groupStash
}

// pickRows are the switcher's rows, as the filter, the groups' and the
// folders' folds and the age the branches shown have at most (weeks, 0
// for any) have them.
func pickRows(m *branchMenu, stashes []proto.GitStash, weeks int, now time.Time) []pickRow {
	list := m.list
	q := strings.ToLower(strings.TrimSpace(m.query))
	match := func(s ...string) bool {
		if q == "" {
			return true
		}
		for _, x := range s {
			if strings.Contains(strings.ToLower(x), q) {
				return true
			}
		}
		return false
	}
	var rows []pickRow
	exact := false
	for i := range list.Branches {
		b := &list.Branches[i]
		exact = exact || !b.Remote && !b.Tag && strings.ToLower(b.Name) == q
	}
	if q != "" && !exact {
		rows = append(rows, pickRow{kind: pickCreate, name: strings.TrimSpace(m.query)})
	}
	// A branch too old to list, unless the filter looks for it, or it's
	// checked out.
	old := func(b *proto.GitBranch) bool {
		return weeks > 0 && q == "" && !b.Current && b.Worktree == "" && b.Time > 0 &&
			now.Sub(time.Unix(b.Time, 0)) > time.Duration(weeks)*7*24*time.Hour
	}
	// group adds a group of n entries, its rows items while open.
	group := func(id, title string, n int, items []pickRow) {
		if len(items) == 0 {
			return
		}
		open := q != "" || m.groupOpen(id)
		rows = append(rows, pickRow{kind: pickGroup, group: id, key: id, name: title, open: open, count: n})
		if open {
			rows = append(rows, items...)
		}
	}
	// branches lays branches out in their folders, split at slashes, as
	// the user left them folded, all open while the filter finds them.
	branches := func(id string, list []*proto.GitBranch, remote bool) []pickRow {
		files := make([]proto.GitFile, len(list))
		for i, b := range list {
			files[i].Path = b.Name
		}
		var out []pickRow
		closed := func(dir string) bool { return q == "" && !m.folds[id+"/"+dir] }
		for _, it := range treeItems(fileTree(files), closed) {
			r := pickRow{group: id, key: id + "/" + it.path, name: it.name, depth: it.depth + 1, open: it.open}
			if it.file < 0 {
				r.kind, r.remote = pickFolder, remote && it.depth == 0
			} else {
				r.kind, r.b = pickBranch, list[it.file]
			}
			out = append(out, r)
		}
		return out
	}

	var wts, local, remote, tags []*proto.GitBranch
	hidden := map[string]int{}
	for i := range list.Branches {
		b := &list.Branches[i]
		switch {
		case b.Tag:
			if match(b.Name) {
				tags = append(tags, b)
			}
			continue
		case !b.Remote && b.Worktree != "" && match(b.Name, proto.DirName(b.Worktree)):
			wts = append(wts, b)
		}
		if !match(b.Name) || b.Remote && strings.HasSuffix(b.Name, "/HEAD") {
			continue
		}
		id := groupLocal
		if b.Remote {
			id = groupRemote
		}
		if old(b) && !m.older[id] {
			hidden[id]++
			continue
		}
		if b.Remote {
			remote = append(remote, b)
		} else {
			local = append(local, b)
		}
	}
	var items []pickRow
	for _, b := range wts {
		items = append(items, pickRow{kind: pickWorktree, group: groupWorktrees, name: proto.DirName(b.Worktree), depth: 1, b: b})
	}
	group(groupWorktrees, locales.GitPickWorktrees(), len(wts), items)
	for _, g := range []struct {
		id, title string
		list      []*proto.GitBranch
	}{{groupLocal, locales.GitBranches(), local}, {groupRemote, locales.GitPickRemotes(), remote}} {
		items := branches(g.id, g.list, g.id == groupRemote)
		if n := hidden[g.id]; n > 0 {
			items = append(items, pickRow{kind: pickOlder, group: g.id, name: locales.GitPickOlder(n, weeks), depth: 1, count: n})
		}
		group(g.id, g.title, len(g.list)+hidden[g.id], items)
	}
	items = nil
	for _, b := range tags {
		items = append(items, pickRow{kind: pickBranch, group: groupTags, name: b.Name, depth: 1, b: b})
	}
	group(groupTags, locales.GitTags(), len(items), items)
	items = nil
	for i := range stashes {
		if st := &stashes[i]; match(st.Message) {
			items = append(items, pickRow{kind: pickStash, group: groupStash, name: st.Message, depth: 1, st: st})
		}
	}
	group(groupStash, locales.GitPickStashes(), len(items), items)
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
	listH := max(menuItemH*4, min(520, h-trig.Bounds().Y-trig.Bounds().H-120))
	ui.PopoverBase(c.Key("branch-menu"), trig, &m.open, func(panel ui.Element) {
		menuPanel(panel, pal).Margin(4, 0, 0, 0).Width(380)
		var rows []pickRow
		if m.list != nil {
			rows = pickRows(m, a.side.git.hist.stashes, a.set.BranchMaxWeeks, time.Now())
		}
		m.hi = min(max(m.hi, 0), max(len(rows)-1, 0))
		// act does what a row does: a click (choose), or a double click
		// and Return, which check a branch out.
		act := func(i int, choose bool) {
			if i < 0 || i >= len(rows) {
				return
			}
			r := rows[i]
			switch r.kind {
			case pickGroup, pickFolder:
				if m.folds == nil {
					m.folds = map[string]bool{}
				}
				m.folds[r.key] = !r.open
			case pickOlder:
				if m.older == nil {
					m.older = map[string]bool{}
				}
				m.older[r.group] = true
			case pickCreate:
				m.open = false
				name := r.name
				a.defer_(func() { a.createBranch(pane.ID, pane.Git.Root, name, "") })
			// A click shows the commit, the switcher staying open to show
			// another; what acts closes it.
			case pickStash:
				a.revealCommit(pane.Git.Root, r.st.Hash, true)
				m.open = choose && m.open
			case pickWorktree:
				if choose {
					a.revealCommit(pane.Git.Root, r.b.Head, false)
				} else {
					m.open = false
					a.openWorktree(pane.ID, r.b.Worktree)
				}
			case pickBranch:
				b := *r.b
				switch {
				case choose:
					a.revealCommit(pane.Git.Root, b.Head, false)
				case b.Tag:
					m.open = false
					a.revealCommit(pane.Git.Root, b.Head, false)
				default:
					m.open = false
					a.defer_(func() { a.switchBranch(s, pane, b, "") })
				}
			}
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
		in.OnSubmit(func() { act(m.hi, false) })
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
			m.scrollHi = reveal
			for i, r := range rows {
				a.pickRowView(c, pal, s, pane, m, r, i, now, act)
			}
		})
	})
}

// pickRowView is a row of the switcher; act does what it does, chosen by
// a click, else by a double click.
func (a *App) pickRowView(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, m *branchMenu, r pickRow, i int, now time.Time, act func(int, bool)) {
	key := r.key
	switch {
	case r.b != nil:
		key = r.group + "|" + r.b.Name
	case r.st != nil:
		key = "stash|" + r.st.Hash
	case key == "":
		key = r.group + "|" + itoa(r.kind)
	}
	// The filter keeps the keyboard as rows are clicked.
	row := ui.Row(c.Key(key)).Label(r.name).KeepFocus()
	if row.Hovered() && m.hi != i {
		m.hi = i
		c.After(0)
	}
	switch {
	case row.DoubleClicked():
		if r.kind == pickBranch || r.kind == pickWorktree {
			act(i, false)
		}
	case row.Clicked():
		act(i, true)
	}
	hi := i == m.hi
	row.Height(menuItemH+2).Padding(0, 10, 0, 6+14*float32(r.depth)).Gap(6).Radius(7).AlignItems(ui.Center)
	fg, faint := pal.text, pal.textFaint
	if hi {
		fg, faint = highlightRow(row, pal)
	}
	b := r.b
	if b != nil && b.Worktree != "" && !b.Current {
		// Checked out in another worktree, where git won't let it be
		// checked out again.
		row.Tooltip(locales.GitBranchInWorktree(a.hostOf(pane.ID).shortDir(b.Worktree)))
	}
	if b != nil && !b.Tag {
		row.ContextMenu(func(mm *ui.Menu) { a.branchMenuItems(mm, s, pane, *b) })
	}
	if r.st != nil {
		st := *r.st
		row.ContextMenu(func(mm *ui.Menu) { a.stashMenu(mm, pane, st) })
	}
	if m.scrollHi && hi {
		m.scrollHi = false
		row.ScrollIntoView()
	}
	row.TextColor(fg).Children(func() {
		chevron := func(open bool) {
			chev := ui.Icon(c, chevronIcon).Size(9, 9).TextColor(faint).Shrink(0)
			chev.Rotate(chev.AnimateTo("turn", 90*b2f(open), chevronTurn))
		}
		var icon *ui.SVG
		weight := 400
		switch r.kind {
		case pickGroup:
			chevron(r.open)
			weight = 650
		case pickFolder:
			chevron(r.open)
			icon = folderIcon
			if r.remote {
				icon = globeIcon
			}
		case pickBranch:
			icon = branchIcon
			if b.Tag {
				icon = tagIcon
			}
		case pickWorktree:
			icon = folderIcon
		case pickStash:
			icon = archiveIcon
		case pickCreate:
			icon = plusIcon
		}
		if icon != nil {
			ui.Icon(c, icon).Size(12, 12).TextColor(faint).Shrink(0)
		}
		label := r.name
		if r.kind == pickCreate {
			label = locales.GitCreateNamed(r.name)
		}
		if b != nil && b.Current {
			weight = 700
		}
		col := fg
		if r.kind == pickOlder {
			col = faint
		}
		ui.Text(c, label).FontSize(13).FontWeight(weight).TextColor(col).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		ui.Spacer(c)
		var bits []string
		switch {
		case r.kind == pickGroup:
			bits = append(bits, itoa(r.count))
		case r.kind == pickWorktree:
			bits = append(bits, b.Name)
		case r.kind == pickStash:
			bits = append(bits, ago(r.st.Time*1000, now))
		case b != nil && !b.Tag:
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
			if b.Time > 0 {
				bits = append(bits, ago(b.Time*1000, now))
			}
		}
		if len(bits) > 0 {
			ui.Text(c, strings.Join(bits, " · ")).FontSize(11.5).FontFeatures("tnum").TextColor(faint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0)
		}
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
