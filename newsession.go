package main

import (
	"cmp"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/commitmsg"
	"kero/internal/proto"

	"kero/locales"
)

// The New Session dialog, which a project's + opens: an agent session, a
// terminal, a new file or a browser, in the project's folder; an agent or
// a terminal also in a git worktree, a new one on the branch the user
// picks, or one the repository has already.

// The kinds of session the dialog starts.
const (
	kindAgent    = "agent"
	kindTerminal = "terminal"
	kindFile     = "file"
	kindBrowser  = "browser"
)

var sessionKinds = []string{kindAgent, kindTerminal, kindFile, kindBrowser}

// newSessionAsk is the New Session dialog: the host, and the session as
// newSessionOn would start it, which the dialog changes; dir, the
// project's folder, whose repository the worktree options are of.
type newSessionAsk struct {
	open     bool
	host     *host
	p        proto.CreateParams
	dir      string
	kind     string
	agent    string
	worktree bool
	// base is what a branch the user names starts from: "" the default
	// branch, "HEAD" dir's (the worktree-base setting).
	base string
	// branches are the repository's, but its tags, which the branch list
	// shows; sel the one chosen.
	branches branchMenu
	sel      branchSel
	// choice is what a branch checked out in another worktree does, as git
	// checks a branch out in one alone: "open" starts the session in that
	// worktree, "new" makes newBranch from the branch in a new one.
	choice    string
	newBranch string
	trees     *proto.GitWorktrees
	// loads counts the loads of the branches and worktrees, of which the
	// last alone fills them in, as a removal loads them anew.
	loads int
	// gitErr is why the folder has no worktree options: no repository.
	gitErr string
}

// branchSel is the branch a new worktree checks out: name, a local branch
// as it is, unless create makes it from the dialog's base, or from names
// what it starts at, a remote's branch for its local one. worktree is
// where the local branch is checked out already, "" for nowhere.
type branchSel struct {
	name     string
	from     string
	create   bool
	worktree string
}

// is reports whether b is the branch chosen, as the branch list lists it.
func (s branchSel) is(b *proto.GitBranch) bool {
	if b.Remote {
		return s.from == b.Name
	}
	return !s.create && s.from == "" && s.name == b.Name
}

// newWorktreeSession opens the New Session dialog with its worktree
// option on, for the project of the session the user is in.
func (a *App) newWorktreeSession() { a.askNewSession(a.selectedHost(), "", "", true) }

// askNewSession opens the New Session dialog for a session on h, in
// project, "" for the shown session's, its worktree option on with
// worktree. Its worktrees are of the repository holding dir, else of the
// project's folder.
func (a *App) askNewSession(h *host, project, dir string, worktree bool) {
	if h.cl == nil {
		a.toast(locales.HostNotConnected(h.label()))
		return
	}
	p := a.newSessionParams(h, project)
	if dir == "" {
		dir = p.Dir
		if pr := a.project(p.Project); pr != nil {
			dir = pr.Dir
		}
	}
	w := &newSessionAsk{open: true, host: h, p: p, dir: dir, kind: kindAgent, agent: a.set.DefaultAgent, worktree: worktree}
	if a.set.WorktreeBase == "current" {
		w.base = "HEAD"
	}
	a.newAsk = w
	a.loadNewSession(w)
}

// loadNewSession asks w's host for the branches and the worktrees of the
// repository of w's folder, choosing the folder's branch.
func (a *App) loadNewSession(w *newSessionAsk) {
	if w.dir == "" {
		w.gitErr = locales.FilesNoFolder()
		return
	}
	cl, dir := w.host.cl, w.dir
	w.loads++
	n := w.loads
	go func() {
		var bs proto.GitBranches
		var trees proto.GitWorktrees
		err := cl.Call("git.branches", proto.GitParams{Dir: dir}, &bs)
		if err == nil {
			err = cl.Call("git.worktrees", proto.GitWorktreesParams{Dir: dir}, &trees)
		}
		a.update(func() {
			if w.loads != n {
				return
			}
			if err != nil {
				w.gitErr = err.Error()
				return
			}
			bs.Branches = slices.DeleteFunc(bs.Branches, func(b proto.GitBranch) bool { return b.Tag })
			w.branches.list, w.trees = &bs, dialogTrees(trees)
			for i := range bs.Branches {
				if b := &bs.Branches[i]; b.Current && w.sel.name == "" {
					w.pickBranch(branchRow{b: b})
				}
			}
		})
		if err != nil {
			return
		}
		// What removing each would lose, for its menu's Remove to ask by,
		// after the list, which reading it would hold up.
		var st proto.GitWorktrees
		if cl.Call("git.worktrees", proto.GitWorktreesParams{Dir: dir, State: true}, &st) == nil {
			a.update(func() {
				if w.loads == n {
					w.trees = dialogTrees(st)
				}
			})
		}
	}()
}

// dialogTrees are the linked worktrees a session may start in, the one
// used last first: not the main checkout, which is the project's folder,
// nor one whose folder is gone.
func dialogTrees(trees proto.GitWorktrees) *proto.GitWorktrees {
	trees.Worktrees = slices.DeleteFunc(trees.Worktrees, func(t proto.GitWorktree) bool { return t.Main || t.Prunable })
	slices.SortStableFunc(trees.Worktrees, func(x, y proto.GitWorktree) int { return cmp.Compare(y.Used, x.Used) })
	return &trees
}

// removeDialogWorktree removes one of the dialog's worktrees, as
// askRemoveWorktree asks, in the dialog's folder's repository, then lists
// them anew.
func (a *App) removeDialogWorktree(w *newSessionAsk, t proto.GitWorktree, branch bool) {
	h := w.host
	run := func(p proto.GitOpParams) {
		cl := h.cl
		if cl == nil {
			a.toast(locales.HostNotConnected(h.label()))
			return
		}
		p.Dir = w.dir
		go func() {
			var res proto.GitOpResult
			err := cl.Call("git.op", p, &res)
			a.update(func() {
				if err == nil && res.Error != "" {
					err = errors.New(res.Error)
				}
				if err != nil {
					a.toast(locales.WorktreeRemoveFailed(err.Error()))
				}
				if a.newAsk == w {
					a.loadNewSession(w)
				}
			})
		}()
	}
	a.askRemoveWorktree(h, t, branch, run, nil)
}

// pickBranch chooses a row of the branch list.
func (w *newSessionAsk) pickBranch(r branchRow) {
	w.choice = ""
	switch {
	case r.create != "":
		w.sel = branchSel{name: r.create, create: true}
	case r.b.Remote:
		_, name, _ := strings.Cut(r.b.Name, "/")
		w.sel = branchSel{name: name, from: r.b.Name}
	default:
		w.sel = branchSel{name: r.b.Name, worktree: r.b.Worktree}
		if r.b.Worktree != "" {
			w.newBranch = freeBranch(w.branches.list, r.b.Name)
		}
	}
}

// freeBranch is a name for a new branch from branch that no local branch
// has: branch-2, else branch-3 and on.
func freeBranch(list *proto.GitBranches, branch string) string {
	for i := 2; ; i++ {
		if name := branch + "-" + strconv.Itoa(i); !hasLocalBranch(list, name) {
			return name
		}
	}
}

func hasLocalBranch(list *proto.GitBranches, name string) bool {
	if list == nil {
		return false
	}
	for _, b := range list.Branches {
		if !b.Remote && !b.Tag && b.Name == name {
			return true
		}
	}
	return false
}

// worktreeKind reports whether the dialog's kind of session may go in a
// worktree: an agent's and a terminal's.
func (w *newSessionAsk) worktreeKind() bool { return w.kind == kindAgent || w.kind == kindTerminal }

// params are the session the dialog starts as it stands, in there when
// it isn't "", one of the repository's worktrees; ok, whether it may
// start: not while the branch it would check out is unknown, or is
// checked out in another worktree and the user didn't say what to do. A
// new file's path is found as it starts.
func (w *newSessionAsk) params(there string) (p proto.CreateParams, ok bool) {
	p = w.p
	switch w.kind {
	case kindAgent:
		if p.Command = commitmsg.Words(w.agent); len(p.Command) == 0 {
			return p, false
		}
	case kindBrowser:
		p.URL = proto.BlankURL
	}
	if there != "" {
		p.Dir = there
		return p, true
	}
	if !w.worktree || !w.worktreeKind() || w.gitErr != "" {
		return p, true
	}
	if w.branches.list == nil {
		return p, false
	}
	p.Dir = w.dir
	s := w.sel
	switch {
	case s.name == "":
		return p, false
	case s.create:
		p.Worktree, p.Base = s.name, w.base
	case s.worktree != "":
		switch w.choice {
		case "open":
			p.Dir = s.worktree
		case "new":
			name := strings.TrimSpace(w.newBranch)
			if name == "" || hasLocalBranch(w.branches.list, name) {
				return p, false
			}
			p.Worktree, p.Base = name, s.name
		default:
			return p, false
		}
	default:
		p.Worktree, p.Base = s.name, s.from
	}
	return p, true
}

// startNewSession starts the dialog's session, in there when it isn't "",
// and closes the dialog; while the session may not start, it stays.
func (a *App) startNewSession(w *newSessionAsk, there string) {
	p, ok := w.params(there)
	if !ok {
		return
	}
	w.open = false
	h := w.host
	if w.kind != kindFile {
		a.create(h, p, p.Project)
		return
	}
	dir := p.Dir
	if dir == "" {
		dir = w.dir
	}
	a.untitledIn(h, dir, func(path string) {
		p.File = path
		a.create(h, p, p.Project)
	})
}

func (a *App) newSessionView(c *ui.Context, pal *palette) {
	w := a.newAsk
	if w == nil {
		return
	}
	start, there := false, ""
	ui.DialogBase(c.Key("new-session"), &w.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.ActionNewSession()
		if pr := a.project(w.p.Project); pr != nil {
			title = locales.NewSessionIn(pr.Name)
		}
		// No taller than the window, which the fields scroll in.
		_, winH := c.Size()
		floating(panel.Width(480).MaxHeight(winH-40).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).Label(title)
		// Return starts the session from wherever the focus is in the
		// dialog, but in a field, which takes it itself.
		if panel.Shortcut(0, ui.KeyEnter) {
			start = true
		}
		ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(pal.text)
		ui.Scroll(c.Key("fields")).Shrink(1).MinHeight(0).Margin(0, -6).Padding(4, 6, 2, 6).Children(func() {
			ui.Column(c).Shrink(0).Gap(10).AlignItems(ui.Stretch).Children(func() {
				a.kindPicker(c, pal, w)
				ui.Text(c, a.kindDetail(w)).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.4)
				if w.kind == kindAgent {
					formRow(c, pal, "agent", locales.NewSessionAgentCommand(), func() {
						popup(c, "agent", &w.agent, a.set.AgentCommands(), func(cmd string) string { return cmd }, 260, pal)
					})
				}
				if w.worktreeKind() {
					a.worktreeFields(c, pal, w, &start, &there)
				}
			})
		})
		_, ok := w.params("")
		dialogButtons(c, func() {
			if button(c, "cancel", locales.CommonCancel(), "esc", false, pal) {
				w.open = false
			}
			// Faded while the session may not start, as a choice is wanting.
			ui.Row(c.Key("start")).Opacity(1 - 0.5*b2f(!ok)).Children(func() {
				if button(c, "ok", locales.NewSessionStart(), "↩", true, pal) {
					start = true
				}
			})
		})
	})
	if w.open && (start || there != "") {
		a.startNewSession(w, there)
	}
	if !w.open {
		a.newAsk = nil
		a.focusTerminal()
	}
}

// formRow is a row of a dialog's form: a label, and its control at the end.
func formRow(c *ui.Context, pal *palette, key, label string, control func()) {
	ui.Row(c.Key(key)).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(pal.text).Grow(1).MinWidth(0)
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

var kindIcons = map[string]*ui.SVG{kindAgent: sparklesIcon, kindTerminal: terminalIcon, kindFile: newFileIcon, kindBrowser: globeIcon}

func kindLabel(kind string) string {
	switch kind {
	case kindAgent:
		return locales.NewSessionAgent()
	case kindTerminal:
		return locales.NewSessionTerminal()
	case kindFile:
		return locales.NewSessionFile()
	}
	return locales.NewSessionBrowser()
}

// kindPicker chooses the kind of session, a card each, which the arrows
// move between; the chosen one has the focus as the dialog opens.
func (a *App) kindPicker(c *ui.Context, pal *palette, w *newSessionAsk) {
	ui.RadioGroup(c.Key("kinds"), func() {
		for _, k := range sessionKinds {
			label := kindLabel(k)
			card := ui.RadioBase(c.Key(k), &w.kind, k).Column().Basis(0).Grow(1).MinWidth(0).Height(68).Gap(7).Radius(10).Center().Label(label)
			if k == w.kind {
				card.AutoFocus()
			}
			on := k == w.kind
			bg, edge, fg, weight := ui.Transparent, pal.text.Alpha(0.1), pal.textMuted, 500
			switch {
			case on:
				bg, edge, fg, weight = pal.accent.Alpha(0.1), pal.accent, pal.accent, 600
			case card.Hovered():
				bg = pal.rowHover
			}
			card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond})).InsetShadow(0, 0, 0, 1+0.5*b2f(on), edge)
			card.Children(func() {
				ui.Icon(c, kindIcons[k]).Size(18, 18).TextColor(fg)
				ui.Text(c, label).FontSize(12.5).FontWeight(weight).TextColor(pal.text).SingleLine().Ellipsis("…")
			})
		}
	}).Row().Gap(8).Label(locales.NewSessionKind())
}

// kindDetail says what the chosen kind of session starts.
func (a *App) kindDetail(w *newSessionAsk) string {
	switch w.kind {
	case kindAgent:
		return locales.NewSessionAgentDetail()
	case kindTerminal:
		return locales.NewSessionTerminalDetail()
	case kindFile:
		dir := w.p.Dir
		if dir == "" {
			dir = w.dir
		}
		return locales.NewSessionFileDetail(w.host.shortDir(dir))
	}
	return locales.NewSessionBrowserDetail()
}

// worktreeFields are the worktree option of an agent's or a terminal's
// session: the switch; then the branch, with what checking it out does,
// and the repository's worktrees, a click on one starting the session
// there.
func (a *App) worktreeFields(c *ui.Context, pal *palette, w *newSessionAsk, start *bool, there *string) {
	formRow(c, pal, "worktree", locales.NewSessionWorktree(), func() { toggle(c, &w.worktree, pal) })
	if !w.worktree {
		return
	}
	switch {
	case w.gitErr != "":
		ui.Text(c, w.gitErr).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.4)
		return
	case w.branches.list == nil:
		ui.Row(c).Height(40).Center().Children(func() { ui.Spinner(c) })
		return
	}
	formRow(c, pal, "branch", locales.NewSessionBranch(), func() {
		m := &w.branches
		label := w.sel.name
		switch {
		case w.sel.from != "":
			label = w.sel.from
		case label == "":
			label = locales.NewSessionPickBranch()
		}
		trig := ui.ButtonBase(c.Key("branch")).MaxWidth(260).Label(locales.NewSessionBranch())
		popupFace(c, trig, label, m.open, pal)
		if trig.Clicked() {
			if m.open {
				m.open = false
			} else {
				m.open, m.query, m.hi, m.reveal, m.at = true, "", 0, true, trig.Bounds()
				for i, r := range branchRows(m.list, "") {
					if r.b != nil && w.sel.is(r.b) {
						m.hi = i
					}
				}
			}
		}
		a.branchList(c, pal, m, trig, branchListOpts{key: "wt-branch", host: w.host, checked: w.sel.is, pick: w.pickBranch})
	})
	a.branchNote(c, pal, w, start)
	if w.trees == nil || len(w.trees.Worktrees) == 0 {
		return
	}
	ui.Text(c, locales.NewSessionWorktrees()).FontSize(12).FontWeight(600).TextColor(pal.textFaint).Margin(6, 0, 0, 0)
	now := time.Now()
	ui.Column(c.Key("trees")).Gap(2).AlignItems(ui.Stretch).Children(func() {
		for _, t := range w.trees.Worktrees {
			name := t.Branch
			if name == "" {
				name = locales.NewSessionDetached(shortHash(t.Head))
			}
			row := ui.ButtonBase(c.Key("tree-"+t.Path)).Padding(6, 8).Gap(8).Radius(8).AlignItems(ui.Center).Label(locales.NewSessionOpenIn(name)).Tooltip(locales.NewSessionOpenIn(name))
			if row.Hovered() || row.Pressed() {
				row.Background(pal.rowHover)
			}
			row.Children(func() {
				ui.Icon(c, branchIcon).Size(12, 12).TextColor(pal.textFaint).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…")
					ui.Text(c, w.host.shortDir(t.Path)).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…")
				})
				if t.Sessions > 0 {
					ui.Text(c, locales.NewSessionRunning(t.Sessions)).FontSize(11.5).FontWeight(500).TextColor(pal.accent).SingleLine().Shrink(0)
				}
				ui.Text(c, ago(t.Used, now)).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Shrink(0)
			})
			if row.Clicked() {
				*there = t.Path
			}
			row.ContextMenu(func(m *ui.Menu) {
				removeWorktreeItems(m, t, func(branch bool) { a.removeDialogWorktree(w, t, branch) })
			})
		}
	})
}

// branchNote says what the branch chosen does: made, from what; checked
// out as it is; or, checked out in another worktree, the choice of that
// worktree or a new branch from it.
func (a *App) branchNote(c *ui.Context, pal *palette, w *newSessionAsk, start *bool) {
	s := w.sel
	note := func(text string) { ui.Text(c, text).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.4) }
	switch {
	case s.name == "":
		return
	case s.create:
		base := locales.NewSessionDefaultBranch()
		if w.base == "HEAD" {
			base = "HEAD"
			for _, b := range w.branches.list.Branches {
				if b.Current {
					base = b.Name
				}
			}
		}
		note(locales.NewSessionBranchNew(s.name, base))
		return
	case s.from != "":
		note(locales.NewSessionBranchNew(s.name, s.from))
		return
	case s.worktree == "":
		note(locales.NewSessionBranchAsIs(s.name))
		return
	}
	box := ui.Column(c.Key("in-use")).Padding(10, 12).Gap(8).Radius(8).AlignItems(ui.Stretch).Background(pal.attention.Alpha(0.1))
	box.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, warningIcon).Size(13, 13).TextColor(pal.attention).Shrink(0).Margin(2, 0, 0, 0)
			ui.Text(c, locales.NewSessionBranchInUse(s.name, w.host.shortDir(s.worktree))).FontSize(12.5).TextColor(pal.text).LineHeight(1.4).Grow(1).MinWidth(0)
		})
		ui.RadioGroup(c.Key("choice"), func() {
			choiceRow(c, pal, &w.choice, "open", locales.NewSessionOpenThere())
			choiceRow(c, pal, &w.choice, "new", locales.NewSessionNewBranchFrom(s.name))
		}).Gap(4).Label(locales.NewSessionBranchInUseChoice())
		if w.choice != "new" {
			return
		}
		ui.Row(c).Margin(0, 0, 0, 22).Children(func() {
			in := textField(c, "new-branch", &w.newBranch, locales.GitBranchName(), 0, pal)
			if in.Submitted() {
				*start = true
			}
		})
		if name := strings.TrimSpace(w.newBranch); hasLocalBranch(w.branches.list, name) {
			ui.Text(c, locales.NewSessionBranchExists(name)).FontSize(12).TextColor(pal.attention).Margin(0, 0, 0, 22)
		}
	})
}

// branchRow is a row of the dialog's branch list: a branch, or the branch
// the query names, to make.
type branchRow struct {
	b      *proto.GitBranch
	create string
	label  string // the section it starts, "" for none
}

// branchRows are the branch list's rows, as the query filters them: local
// branches, then the remotes' that no local branch has the name of.
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
		label  string
		remote bool
	}{{locales.GitBranches(), false}, {locales.GitRemoteBranches(), true}}
	for _, sec := range sections {
		first := true
		for i := range list.Branches {
			b := &list.Branches[i]
			if b.Tag || b.Remote != sec.remote || q != "" && !strings.Contains(strings.ToLower(b.Name), q) {
				continue
			}
			if sec.remote {
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

// branchListOpts are what a branch list does besides listing m's branches:
// its elements' key; the host whose folders it names; the branch it marks
// as chosen; and what picking a row does, once the list closed.
type branchListOpts struct {
	key     string
	host    *host
	checked func(b *proto.GitBranch) bool
	pick    func(r branchRow)
}

// branchList is a popover below trig of m's branches: a field that filters
// them, or names a branch to make, and the rows the arrows and Return
// choose from.
func (a *App) branchList(c *ui.Context, pal *palette, m *branchMenu, trig ui.Element, o branchListOpts) {
	if b := trig.Bounds(); m.open && (abs(b.X-m.at.X) > 0.5 || abs(b.Y-m.at.Y) > 0.5) {
		m.open = false
	}
	if m.open {
		ui.Overlay(c, func() {
			back := ui.ButtonBase(c.Key(o.key+"-back")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Label(locales.CloseTheMenu())
			if back.Clicked() {
				m.open = false
			}
		})
	}
	_, h := c.Size()
	listH := max(menuItemH*4, min(380, h-trig.Bounds().Y-trig.Bounds().H-120))
	ui.PopoverBase(c.Key(o.key+"-menu"), trig, &m.open, func(panel ui.Element) {
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
			o.pick(rows[i])
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
					fg, faint = highlightRow(row, pal)
				}
				if r.b != nil && r.b.Worktree != "" && !r.b.Current {
					// Checked out in another worktree, where git won't let
					// it be checked out again.
					row.Tooltip(locales.GitBranchInWorktree(o.host.shortDir(r.b.Worktree)))
				}
				b := r.b
				row.TextColor(fg).Children(func() {
					switch {
					case b == nil:
						ui.Icon(c, plusIcon).Size(12, 12).Shrink(0)
					case o.checked(b):
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

// choiceRow is a radio button of value, with its label, in a radio group.
func choiceRow(c *ui.Context, pal *palette, choice *string, value, label string) {
	r := ui.RadioBase(c.Key(value), choice, value).Gap(8).Padding(3, 0).AlignItems(ui.Center).Label(label)
	on := *choice == value
	r.Children(func() {
		dot := ui.Box(c).Size(14, 14).Radius(7).Center().Shrink(0)
		if on {
			dot.Background(pal.accentBg).Children(func() { ui.Box(c).Size(6, 6).Radius(3).Background(pal.onAccent) })
		} else {
			dot.InsetShadow(0, 0, 0, 1, pal.text.Alpha(0.3))
		}
		ui.Text(c, label).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
	})
}
