package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// The interactive rebase, as Fork's: the commits of the worktree's branch
// since the one it is rebased onto, the newest on top, each with what is
// done with it, which the user drags into the order they are to be in. A
// commit reworded has its new message written in a panel under it, which
// opens as it is chosen; below the list, the commit chosen's message and
// files.

// rebaseRowH is the height of a commit's row, which a drag moves it by.
const rebaseRowH = 34

// rebaseView is the interactive rebase asked for: of the worktree of a
// terminal pane, onto onto.
type rebaseView struct {
	open       bool
	pane, root string
	onto       string
	plan       *proto.GitRebasePlan
	err        string
	busy       bool
	// rows are the commits, the oldest first, as the rebase takes them.
	rows   []rebaseRow
	sel    int
	backup bool
	tab    string
	// infos are the commits' messages and files, as git.show read them.
	infos   map[string]*proto.GitCommitInfo
	reading map[string]bool
	// editing is the commit whose new message the panel edits, in subject
	// and body; shut, one whose panel the user closed, which it is until
	// another commit is chosen.
	editing, shut string
	subject, body string
	// dragging is the commit whose row is dragged, and drag how far it
	// moved since it last took a place; shift, the places it is to move
	// by once the rows are built.
	dragging string
	drag     float32
	shift    int
}

// rebaseRow is a commit of the rebase, and what is done with it; message
// is its new message, when reworded.
type rebaseRow struct {
	c       proto.GitCommit
	action  string
	message string
}

// rebaseActions are the actions a commit can have, with their keys.
var rebaseActions = []struct {
	action string
	key    ui.Key
}{
	{git.RebasePick, ui.KeyP}, {git.RebaseReword, ui.KeyR}, {git.RebaseEdit, ui.KeyE},
	{git.RebaseSquash, ui.KeyS}, {git.RebaseFixup, ui.KeyF}, {git.RebaseDrop, ui.KeyD},
}

func rebaseLabel(action string) string {
	switch action {
	case git.RebaseReword:
		return locales.RebaseReword()
	case git.RebaseEdit:
		return locales.RebaseEdit()
	case git.RebaseSquash:
		return locales.RebaseSquash()
	case git.RebaseFixup:
		return locales.RebaseFixup()
	case git.RebaseDrop:
		return locales.RebaseDrop()
	}
	return locales.RebasePick()
}

func rebaseTip(action string) string {
	switch action {
	case git.RebaseReword:
		return locales.RebaseRewordTip()
	case git.RebaseEdit:
		return locales.RebaseEditTip()
	case git.RebaseSquash:
		return locales.RebaseSquashTip()
	case git.RebaseFixup:
		return locales.RebaseFixupTip()
	case git.RebaseDrop:
		return locales.RebaseDropTip()
	}
	return locales.RebasePickTip()
}

// rebaseColor is an action's color, of the terminal's palette.
func (a *App) rebaseColor(c *ui.Context, pal *palette, action string) ui.Color {
	p := a.codeOf(c, pal).Palette
	switch action {
	case git.RebaseReword:
		return p[2]
	case git.RebaseEdit:
		return p[3]
	case git.RebaseSquash:
		return p[5]
	case git.RebaseFixup:
		return p[6]
	case git.RebaseDrop:
		return p[1]
	}
	return p[4]
}

// openRebase opens the interactive rebase of a pane's worktree's branch
// onto rev, and reads its commits.
func (a *App) openRebase(pane *proto.Pane, rev string) {
	rv := &rebaseView{open: true, pane: pane.ID, root: pane.Git.Root, onto: rev, busy: true, tab: "commit",
		infos: map[string]*proto.GitCommitInfo{}, reading: map[string]bool{}}
	a.rebase = rv
	cl := a.hostOf(pane.ID).cl
	if cl == nil {
		rv.busy, rv.err = false, locales.HostNotConnected(a.hostOf(pane.ID).label())
		return
	}
	go func() {
		var plan proto.GitRebasePlan
		err := cl.Call("git.rebasePlan", proto.GitShowParams{Pane: rv.pane, Rev: rev}, &plan)
		a.update(func() {
			rv.busy = false
			if err != nil {
				rv.err = locales.RebaseLoadFailed(err.Error())
				return
			}
			rv.plan = &plan
			for _, c := range plan.Commits {
				rv.rows = append(rv.rows, rebaseRow{c: c, action: git.RebasePick})
			}
			rv.sel = len(rv.rows) - 1 // the newest, on top
		})
	}()
}

// readCommit reads a commit's message and files, once.
func (a *App) readCommit(rv *rebaseView, hash string) {
	cl := a.hostOf(rv.pane).cl
	if cl == nil || rv.infos[hash] != nil || rv.reading[hash] {
		return
	}
	rv.reading[hash] = true
	go func() {
		var ci proto.GitCommitInfo
		err := cl.Call("git.show", proto.GitShowParams{Pane: rv.pane, Rev: hash}, &ci)
		a.update(func() {
			delete(rv.reading, hash)
			if err == nil {
				rv.infos[hash] = &ci
			}
		})
	}()
}

// problem is what keeps the rebase from going as the rows are, "" for
// nothing: a squash or a fixup with no commit kept below it, or no commit
// kept at all.
func (rv *rebaseView) problem() string {
	kept := false
	for _, r := range rv.rows {
		switch r.action {
		case git.RebaseDrop:
		case git.RebaseSquash, git.RebaseFixup:
			if !kept {
				return locales.RebaseSquashFirst(rebaseLabel(r.action))
			}
		default:
			kept = true
		}
	}
	if !kept {
		return locales.RebaseAllDropped()
	}
	return ""
}

// setAction gives the chosen commit an action; Reword opens its panel.
func (rv *rebaseView) setAction(i int, action string) {
	if i < 0 || i >= len(rv.rows) {
		return
	}
	rv.rows[i].action = action
	if action == git.RebaseReword {
		rv.shut = ""
	}
}

// move moves the chosen commit by one place, up toward the newest (+1) or
// down (-1), with the choice.
func (rv *rebaseView) move(dir int) bool {
	j := rv.sel + dir
	if rv.sel < 0 || j < 0 || j >= len(rv.rows) {
		return false
	}
	rv.rows[rv.sel], rv.rows[j] = rv.rows[j], rv.rows[rv.sel]
	rv.sel = j
	return true
}

// choose makes a commit the chosen one.
func (rv *rebaseView) choose(i int) {
	if i != rv.sel {
		rv.shut = ""
	}
	rv.sel = i
}

func (a *App) rebaseView(c *ui.Context, pal *palette) {
	rv := a.rebase
	if rv == nil {
		return
	}
	start := false
	ui.DialogBase(c.Key("rebase"), &rv.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(760).Height(620).Padding(20).Gap(10).Radius(14).Background(pal.popover), pal).Label(locales.RebaseTitle())
		ui.Text(c, locales.RebaseTitle()).FontSize(15).FontWeight(650).TextColor(pal.text)
		switch {
		case rv.busy:
			ui.Column(c.Key("loading")).Grow(1).Center().Children(func() { ui.Spinner(c) })
		case rv.err != "":
			ui.Text(c, rv.err).FontSize(13).TextColor(pal.attention).LineHeight(1.4).Grow(1).Selectable()
		case len(rv.rows) == 0:
			ui.Text(c, locales.RebaseNothing(rebaseBranch(rv), rebaseOntoName(rv))).FontSize(13).TextColor(pal.textMuted).Grow(1)
		default:
			ui.Text(c, locales.RebaseOnto(rebaseBranch(rv), rebaseOntoName(rv))).FontSize(13).TextColor(pal.textMuted).SingleLine().Ellipsis("…")
			a.rebaseList(c, pal, rv)
			a.rebaseDetail(c, pal, rv)
			ui.Text(c, locales.RebaseHelp()).FontSize(11.5).TextColor(pal.textFaint).LineHeight(1.35)
		}
		problem := rv.problem()
		ui.Row(c.Key("foot")).Gap(8).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			if len(rv.rows) > 0 {
				ui.Checkbox(c.Key("backup"), &rv.backup, locales.RebaseBackup()).Shrink(1).MinWidth(0)
			}
			ui.Spacer(c)
			if problem != "" && len(rv.rows) > 0 {
				ui.Text(c, problem).FontSize(12).TextColor(pal.attention).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			}
			if button(c, "cancel", locales.CommonCancel(), "esc", false, pal) {
				rv.open = false
			}
			if len(rv.rows) > 0 && problem == "" && button(c, "ok", locales.GitRebase(), "", true, pal) {
				start = true
			}
		})
	})
	if start {
		rv.open = false
		a.runRebase(rv)
	}
	if !rv.open {
		a.rebase = nil
	}
}

// rebaseBranch and rebaseOntoName name the branch rebased and the commit
// it goes onto, by its short hash and subject.
func rebaseBranch(rv *rebaseView) string {
	if rv.plan == nil || rv.plan.Branch == "" {
		return "HEAD"
	}
	return rv.plan.Branch
}

func rebaseOntoName(rv *rebaseView) string {
	if rv.plan == nil {
		return rv.onto
	}
	return shortHash(rv.plan.Onto.Hash) + " " + rv.plan.Onto.Subject
}

// rebaseList is the commits, the newest on top, each a row, with the panel
// of the chosen one's new message under it while it is reworded. The keys
// of the actions, and Alt with the arrows, act on the chosen one.
func (a *App) rebaseList(c *ui.Context, pal *palette, rv *rebaseView) {
	list := ui.Scroll(c.Key("list")).Grow(1).Basis(0).MinHeight(120).Focusable().FocusRing(false).Radius(8).
		Background(pal.text.Alpha(0.035)).Label(locales.RebaseTitle())
	list.Children(func() {
		for i := len(rv.rows) - 1; i >= 0; i-- {
			a.rebaseRow(c, pal, rv, i)
			if i == rv.sel && rv.rows[i].action == git.RebaseReword && rv.shut != rv.rows[i].c.Hash {
				a.rewordPanel(c, pal, rv, i)
			}
		}
	})
	for ; rv.shift > 0 && rv.move(1); rv.shift-- {
	}
	for ; rv.shift < 0 && rv.move(-1); rv.shift++ {
	}
	rv.shift = 0
	for _, k := range rebaseActions {
		if list.Shortcut(0, k.key) {
			rv.setAction(rv.sel, k.action)
		}
	}
	switch {
	case list.Shortcut(ui.Alt, ui.KeyUp):
		rv.move(1)
	case list.Shortcut(ui.Alt, ui.KeyDown):
		rv.move(-1)
	case list.Shortcut(0, ui.KeyUp):
		rv.choose(min(rv.sel+1, len(rv.rows)-1))
	case list.Shortcut(0, ui.KeyDown):
		rv.choose(max(rv.sel-1, 0))
	}
}

// rebaseRow is a commit's row: its action, which its button's menu
// changes, its subject, author, hash and date. It is dragged by itself,
// moving a place as the pointer goes past half of a row.
func (a *App) rebaseRow(c *ui.Context, pal *palette, rv *rebaseView, i int) {
	r := rv.rows[i]
	col := a.rebaseColor(c, pal, r.action)
	row := ui.Row(c.Key("row-"+r.c.Hash)).Height(rebaseRowH).Shrink(0).Padding(0, 10).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorGrab)
	switch {
	case i == rv.sel:
		// The list's own tint is that of a selected row elsewhere.
		row.Background(pal.accent.Alpha(0.14))
	case row.Hovered():
		row.Background(pal.rowHover)
	}
	if row.Pressed() {
		rv.choose(i)
	}
	if _, dy, ok := row.Dragged(); ok && i == rv.sel {
		// Up the screen is toward the newest, the rows' end.
		rv.dragging = r.c.Hash
		rv.drag -= dy
		for ; rv.drag > rebaseRowH/2; rv.drag -= rebaseRowH {
			rv.shift++
		}
		for ; rv.drag < -rebaseRowH/2; rv.drag += rebaseRowH {
			rv.shift--
		}
	} else if rv.dragging == r.c.Hash {
		rv.dragging, rv.drag = "", 0
	}
	dropped := r.action == git.RebaseDrop
	row.Children(func() {
		b := ui.ButtonBase(c.Key("action")).Width(96).Height(24).Padding(0, 8).Gap(7).Radius(6).AlignItems(ui.Center).Shrink(0).
			Background(col.Alpha(0.14)).TextColor(pal.text).Label(rebaseLabel(r.action)).Tooltip(rebaseTip(r.action))
		b.Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(col).Shrink(0)
			ui.Text(c, rebaseLabel(r.action)).FontSize(12.5).FontWeight(550).SingleLine().Grow(1)
			ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(pal.textMuted)
		})
		b.Menu(func(m *ui.Menu) {
			for _, k := range rebaseActions {
				if m.Item(rebaseLabel(k.action)).Checked(r.action == k.action).Chosen() {
					rv.choose(i)
					rv.setAction(i, k.action)
				}
			}
		})
		subject := r.c.Subject
		if r.action == git.RebaseReword && r.message != "" {
			subject, _, _ = strings.Cut(r.message, "\n")
		}
		fg := pal.text
		if dropped {
			fg = pal.textFaint
		}
		s := ui.Text(c, subject).FontSize(13).TextColor(fg).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if dropped {
			s.Strikethrough()
		}
		ui.Text(c, r.c.Author).FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MaxWidth(140).Shrink(1)
		ui.Text(c, shortHash(r.c.Hash)).FontSize(12).Font(a.termFamily()).TextColor(pal.textFaint).SingleLine().Shrink(0)
		ui.Text(c, ago(r.c.Time*1000, time.Now())).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Shrink(0).
			Tooltip(time.Unix(r.c.Time, 0).Format("2006-01-02 15:04"))
	})
}

// rewordPanel edits a reworded commit's new message: its subject and its
// description, from its message as it is, until OK keeps them.
func (a *App) rewordPanel(c *ui.Context, pal *palette, rv *rebaseView, i int) {
	r := &rv.rows[i]
	hash := r.c.Hash
	info := rv.infos[hash]
	if info == nil && r.message == "" {
		a.readCommit(rv, hash)
	}
	if rv.editing != hash && (r.message != "" || info != nil) {
		msg := r.message
		if msg == "" {
			msg = info.Message
		}
		rv.editing = hash
		subject, body, _ := strings.Cut(strings.TrimSpace(msg), "\n")
		rv.subject, rv.body = strings.TrimSpace(subject), strings.TrimSpace(body)
	}
	panel := ui.Column(c.Key("reword-"+hash)).Shrink(0).Margin(2, 10, 8, 10).Padding(12).Gap(8).Radius(10).Background(pal.popover)
	floating(panel, pal).Label(locales.RebaseNewMessage())
	panel.Children(func() {
		if rv.editing != hash {
			ui.Spinner(c).Size(14, 14)
			return
		}
		textField(c, "subject", &rv.subject, locales.RebaseSubject(), 0, pal).AutoFocus()
		frame := ui.Row(c.Key("body")).AlignItems(ui.Start).Cursor(ui.CursorText)
		var body ui.Element
		frame.Children(func() {
			body = ui.TextAreaBase(c.Key("in"), &rv.body).Grow(1).MinWidth(0).MinHeight(64).MaxHeight(140).Padding(6, 8).FontSize(13).
				Placeholder(locales.RebaseBody()).Label(locales.RebaseBody())
		})
		fieldLook(c, frame, body.Focused(), pal).Padding(0)
		ui.Row(c.Key("buttons")).Gap(8).Justify(ui.End).Children(func() {
			if button(c, "cancel", locales.CommonCancel(), "", false, pal) {
				if r.message == "" {
					r.action = git.RebasePick
				}
				rv.shut, rv.editing = hash, ""
			}
			if button(c, "ok", locales.RebaseKeepMessage(), "", true, pal) && strings.TrimSpace(rv.subject) != "" {
				msg := strings.TrimSpace(rv.subject)
				if b := strings.TrimSpace(rv.body); b != "" {
					msg += "\n\n" + b
				}
				r.message = msg + "\n"
				rv.shut, rv.editing = hash, ""
			}
		})
	})
}

// rebaseDetail is the chosen commit's message, or its files, as its tabs
// choose.
func (a *App) rebaseDetail(c *ui.Context, pal *palette, rv *rebaseView) {
	if rv.sel < 0 || rv.sel >= len(rv.rows) {
		return
	}
	hash := rv.rows[rv.sel].c.Hash
	a.readCommit(rv, hash)
	info := rv.infos[hash]
	ui.Column(c.Key("detail")).Height(170).Shrink(0).Gap(6).Children(func() {
		ui.Row(c.Key("tabs")).Gap(2).Children(func() {
			for _, t := range []struct{ key, label string }{{"commit", locales.RebaseCommitTab()}, {"changes", locales.RebaseChangesTab()}} {
				b := ui.ButtonBase(c.Key("tab-"+t.key)).Height(26).Padding(0, 10).Radius(7).Center().TextColor(pal.textMuted).Label(t.label)
				switch {
				case rv.tab == t.key:
					b.Background(pal.rowHover).TextColor(pal.text)
				case b.Hovered():
					b.Background(pal.rowHover.Alpha(0.5))
				}
				b.Children(func() { ui.Text(c, t.label).FontSize(12.5).FontWeight(550).SingleLine() })
				if b.Clicked() {
					rv.tab = t.key
				}
			}
		})
		body := ui.Scroll(c.Key("body")).Grow(1).Basis(0).Radius(8).Padding(10).Background(pal.text.Alpha(0.035))
		body.Children(func() {
			if info == nil {
				ui.Spinner(c).Size(14, 14)
				return
			}
			if rv.tab == "changes" {
				for _, f := range info.Files {
					ui.Row(c.Key("f-" + f.Path)).Gap(8).Height(22).AlignItems(ui.Center).Children(func() {
						ui.Text(c, f.Path).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
						added, removed, _ := a.diffColors(c, pal)
						if f.Added > 0 {
							ui.Text(c, "+"+itoa(f.Added)).FontSize(11.5).FontFeatures("tnum").TextColor(added).Shrink(0)
						}
						if f.Removed > 0 {
							ui.Text(c, "−"+itoa(f.Removed)).FontSize(11.5).FontFeatures("tnum").TextColor(removed).Shrink(0)
						}
					})
				}
				return
			}
			ui.Text(c, info.Author+" · "+time.Unix(info.Time, 0).Format("2006-01-02 15:04")+" · "+shortHash(info.Hash)).
				FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…")
			ui.Text(c, strings.TrimSpace(info.Message)).FontSize(13).TextColor(pal.text).LineHeight(1.4).Selectable().Margin(6, 0, 0, 0)
		})
	})
}

// runRebase runs the rebase as its rows are, the changes not committed
// stashed for it, after a branch keeps where the branch was, as the user
// chose; as the agents in the worktree allow.
func (a *App) runRebase(rv *rebaseView) {
	p := proto.GitOpParams{Op: "rebaseInteractive", Rev: rv.onto, Autostash: true}
	for _, r := range rv.rows {
		s := proto.GitRebaseStep{Action: r.action, Hash: r.c.Hash}
		if r.action == git.RebaseReword {
			s.Message = r.message
		}
		p.Steps = append(p.Steps, s)
	}
	if rv.backup {
		p.Backup = rebaseBranch(rv) + "-backup-" + time.Now().Format("20060102-150405")
	}
	id, root := rv.pane, rv.root
	a.guarded(id, root, locales.RebaseOp(), func() {
		a.runGit(id, root, p, locales.RebaseOp(), nil)
	})
}
