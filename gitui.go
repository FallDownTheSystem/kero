package main

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// The git of the Explorer's Changes and History views beyond their diffs:
// operations on a worktree through kerod (git.op), one at a time, followed
// as they run and told of when they fail; and the rule for those that
// change the files an agent at work edits, which ask first: to wait for
// the agent to finish, or to go ahead now.

// gitUI is the views' git: what runs, what failed, and what waits.
type gitUI struct {
	op     *gitRun
	failed *gitFailure
	queued []*gitQueued
	// next are the stagings and unstagings clicked while one ran, each
	// run as the one before it ends, rather than refused as busy.
	next []func()
	// done counts the operations ended, each of which the views' lists
	// are fetched again after, as their stamps can stay the same.
	done   int
	drafts map[string]*commitDraft // by worktree
	menu   branchMenu
	ask    *nameAsk
	hist   historyState
}

// gitRun is an operation under way: its id, which git.progress follows
// and git.cancel cancels, and the line git printed last.
type gitRun struct {
	id, pane, root, op, label string
	started                   time.Time
	line                      string
	polling, canceled         bool
	polled                    time.Time
}

// gitFailure is an operation that failed: what git said, all of it, and
// the command that runs it again in a terminal, where git can ask for a
// password, nil for none.
type gitFailure struct {
	root, pane, label string
	summary, output   string
	argv              []string
	open              bool // the output shows
}

// gitQueued is an operation that waits for the agents of its worktree to
// finish.
type gitQueued struct {
	pane, root, label string
	run               func()
}

// opID makes an operation's id, which kerod follows it by.
func opID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// gitBusy tells that an operation runs in a worktree.
func (a *App) gitBusy(root string) bool {
	op := a.side.git.op
	return op != nil && op.root == root
}

// runGit runs an operation in a pane's worktree, said as label while it
// runs and when it fails; then, when set, gets what it printed once it
// went through.
func (a *App) runGit(pane, root string, p proto.GitOpParams, label string, then func(out string)) {
	a.runGitFor(pane, root, p, label, then, nil)
}

// quickOp tells an operation of the index alone, which a click of the
// same kind waits for (gitUI.next) rather than be refused.
func quickOp(op string) bool { return op == "stage" || op == "unstage" }

// runGitFor runs an operation as runGit does; fail, when set, gets what
// went wrong first, and tells that it took care of it, which the banner
// then doesn't show. It reports whether the operation runs, or waits to.
func (a *App) runGitFor(pane, root string, p proto.GitOpParams, label string, then func(out string), fail func(summary string) bool) bool {
	g := &a.side.git
	h := a.hostOf(pane)
	cl := h.cl
	if cl == nil {
		a.toast(locales.HostNotConnected(h.label()))
		return false
	}
	if g.op != nil {
		if quickOp(p.Op) && quickOp(g.op.op) {
			g.next = append(g.next, func() { a.runGitFor(pane, root, p, label, then, fail) })
			return true
		}
		a.toast(locales.GitBusy(g.op.label))
		return false
	}
	p.Pane, p.ID, p.Root = pane, opID(), root
	run := &gitRun{id: p.ID, pane: pane, root: root, op: p.Op, label: label, started: time.Now()}
	g.op = run
	if g.failed != nil && g.failed.root == root {
		g.failed = nil
	}
	go func() {
		var res proto.GitOpResult
		err := cl.Call("git.op", p, &res)
		a.update(func() {
			if g.op == run {
				g.op = nil
			}
			g.done++
			if err != nil {
				res.Error = err.Error()
			}
			switch {
			case res.Error == "canceled" || run.canceled:
			case res.Error != "" && fail != nil && fail(res.Error):
			case res.Error != "":
				g.failed = &gitFailure{root: root, pane: pane, label: label, summary: res.Error, output: res.Output, argv: res.Command}
			case then != nil:
				then(res.Output)
			}
			if len(g.next) > 0 && g.op == nil {
				next := g.next[0]
				g.next = g.next[1:]
				next()
			}
		})
	}()
	return true
}

// paneGit returns a pane's worktree's status, nil for none.
func (a *App) paneGit(pane string) *proto.GitInfo {
	if s := a.sessionOfPane(pane); s != nil {
		if p := s.PaneByID(pane); p != nil {
			return p.Git
		}
	}
	return nil
}

// cancelGit cancels the operation under way.
func (a *App) cancelGit() {
	op := a.side.git.op
	if op == nil || op.canceled {
		return
	}
	op.canceled = true
	if cl := a.hostOf(op.pane).cl; cl != nil {
		go cl.Call("git.cancel", proto.IDParams{ID: op.id}, nil)
	}
}

// pollGit follows the operation under way's progress, a few times a
// second, while it shows.
func (a *App) pollGit(c *ui.Context) {
	op := a.side.git.op
	if op == nil {
		return
	}
	c.After(400 * time.Millisecond)
	if op.polling || time.Since(op.polled) < 400*time.Millisecond {
		return
	}
	cl := a.hostOf(op.pane).cl
	if cl == nil {
		return
	}
	op.polling = true
	go func() {
		var pr proto.GitProgress
		err := cl.Call("git.progress", proto.IDParams{ID: op.id}, &pr)
		a.update(func() {
			op.polling, op.polled = false, time.Now()
			if err == nil && pr.Line != "" {
				op.line = pr.Line
			}
		})
	}()
}

// agentsAt names the agents at work in a worktree, in the sessions of the
// host of pane.
func (a *App) agentsAt(pane, root string) []string {
	h := a.hostOf(pane)
	var names []string
	for i := range a.st.Sessions {
		s := &a.st.Sessions[i]
		if a.hostOf(s.ID) != h {
			continue
		}
		for _, p := range s.Panes {
			if p.Status != proto.Working || p.Git == nil || p.Git.Root != root {
				continue
			}
			name := proto.AgentNames[p.Agent]
			if name == "" {
				name = locales.GitAnAgent()
			}
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return names
}

// guarded runs fn, which changes the files of a worktree, at once while no
// agent works there; else it asks first: to run it once they finish, or
// now, over their edits.
func (a *App) guarded(pane, root, label string, fn func()) {
	agents := a.agentsAt(pane, root)
	if len(agents) == 0 {
		fn()
		return
	}
	who := strings.Join(agents, ", ")
	a.confirm = &confirmation{open: true, title: locales.GitAgentWorking(who), detail: locales.GitAgentWorkingDetail(label),
		action: locales.GitWhenDone(), run: func() {
			a.side.git.queued = append(a.side.git.queued, &gitQueued{pane: pane, root: root, label: label, run: fn})
		},
		alt: locales.GitNow(), altRun: fn, altDestroy: true}
}

// guardedNow runs fn, an edit of a change shown in a diff, at once while no
// agent works in the worktree; else it asks to go ahead, as the change it
// edits would not wait as it is.
func (a *App) guardedNow(pane, root, label string, fn func()) {
	agents := a.agentsAt(pane, root)
	if len(agents) == 0 {
		fn()
		return
	}
	a.confirm = &confirmation{open: true, title: locales.GitAgentWorking(strings.Join(agents, ", ")), detail: locales.GitAgentWorkingNow(label),
		action: label, destroy: true, run: fn}
}

// drainGit runs the first operation that waited for its worktree's agents,
// once they finished, when no other runs.
func (a *App) drainGit() {
	g := &a.side.git
	if len(g.queued) == 0 || g.op != nil {
		return
	}
	for i, q := range g.queued {
		if len(a.agentsAt(q.pane, q.root)) == 0 {
			g.queued = slices.Delete(g.queued, i, i+1)
			a.defer_(q.run)
			return
		}
	}
}

// gitRunning shows, in a worktree's bar, in the room it has, the operation
// running there: in place, as the bar's height stays, rather than in a row
// of its own that would push the list down and back up again. One that
// ends at once, as most staging does, shows none.
func (a *App) gitRunning(c *ui.Context, pal *palette, root string) {
	op := a.side.git.op
	if op == nil || op.root != root {
		return
	}
	a.pollGit(c)
	if wait := 300*time.Millisecond - time.Since(op.started); wait > 0 {
		c.After(wait)
		return
	}
	ui.Row(c.Key("git-op")).Gap(6).AlignItems(ui.Center).Shrink(1).MinWidth(0).Margin(0, 2, 0, 8).Tooltip(op.label).Children(func() {
		ui.Spinner(c).Size(12, 12)
		text := op.label
		line := op.line
		if line == "" && time.Since(op.started) > 3*time.Second {
			line = locales.GitStillRunning()
		}
		if line != "" {
			text += " · " + line
		}
		ui.Text(c, text).FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		if !op.canceled {
			iconButton(c, "cancel", closeIcon, locales.GitCancel(), pal, a.cancelGit)
		}
	})
}

// gitStrip shows, under a worktree's bar, the operations waiting for its
// agents, and the last that failed: until the user does something about
// them, as they stay.
func (a *App) gitStrip(c *ui.Context, pal *palette, root string) {
	g := &a.side.git
	for i, q := range g.queued {
		if q.root != root {
			continue
		}
		ui.Row(c.Key("queued-"+itoa(i))).Padding(0, 12, 6, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, clockIcon).Size(12, 12).TextColor(pal.textFaint).Shrink(0)
			ui.Text(c, locales.GitWaiting(q.label)).FontSize(12.5).TextColor(pal.textMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			iconButton(c, "drop", closeIcon, locales.GitCancel(), pal, func() {
				a.defer_(func() {
					if j := slices.Index(g.queued, q); j >= 0 {
						g.queued = slices.Delete(g.queued, j, j+1)
					}
				})
			})
		})
	}
	f := g.failed
	if f == nil || f.root != root {
		return
	}
	_, removed, _ := a.diffColors(c, pal)
	ui.Column(c.Key("git-failed")).Margin(0, 12, 8, 12).Padding(8, 10).Gap(6).Radius(8).Background(removed.Alpha(0.1)).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, warningIcon).Size(13, 13).TextColor(removed).Shrink(0).Margin(2, 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, locales.GitFailed(f.label)).FontSize(12.5).FontWeight(600).TextColor(pal.text)
				ui.Text(c, f.summary).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35).Selectable()
			})
			iconButton(c, "dismiss", closeIcon, locales.GitDismiss(), pal, func() { g.failed = nil })
		})
		if f.open && f.output != "" {
			ui.Scroll(c.Key("output")).MaxHeight(180).Children(func() {
				ui.Text(c, f.output).Font(a.termFamily()).FontSize(11.5).TextColor(pal.textMuted).Selectable().LineHeight(1.35)
			})
		}
		ui.Row(c).Gap(4).Children(func() {
			if f.output != "" && f.output != f.summary {
				label := locales.GitShowOutput()
				if f.open {
					label = locales.GitHideOutput()
				}
				if hunkButton(c, "output-btn", docIcon, label, label, pal) {
					f.open = !f.open
				}
			}
			if f.argv != nil && hunkButton(c, "terminal", terminalIcon, locales.GitRunInTerminal(), strings.Join(f.argv, " "), pal) {
				a.runInTerminal(f.pane, root, f.argv)
				g.failed = nil
			}
		})
	})
}

// runInTerminal runs a command in a new pane below the pane, in the
// worktree, as the user would type it.
func (a *App) runInTerminal(pane, dir string, argv []string) {
	s := a.sessionOfPane(pane)
	if s == nil {
		return
	}
	a.split(s, proto.CreateParams{Split: pane, Direction: "down", Ratio: 0.35, Dir: dir, Command: argv})
}

// nameAsk asks a name, of a branch: title and detail say what for, and
// run takes it.
type nameAsk struct {
	open                       bool
	title, detail, placeholder string
	action                     string
	name                       string
	run                        func(name string)
}

func (a *App) askName(title, detail, placeholder, initial, action string, run func(string)) {
	a.side.git.ask = &nameAsk{open: true, title: title, detail: detail, placeholder: placeholder, action: action, name: initial, run: run}
}

func (a *App) nameView(c *ui.Context, pal *palette) {
	w := a.side.git.ask
	if w == nil {
		return
	}
	done := false
	ui.DialogBase(c.Key("git-name"), &w.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(400).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).Label(w.title)
		ui.Text(c, w.title).FontSize(15).FontWeight(650).TextColor(pal.text)
		if w.detail != "" {
			ui.Text(c, w.detail).FontSize(13).TextColor(pal.textMuted).LineHeight(1.4)
		}
		ui.Row(c).Margin(8, 0, 0, 0).Children(func() {
			in := textField(c, "name", &w.name, w.placeholder, 0, pal)
			in.AutoFocus()
			if in.Submitted() {
				done = true
			}
		})
		ui.Row(c).Gap(8).Justify(ui.End).Margin(14, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.CommonCancel(), "esc", false, pal) {
				w.open = false
			}
			if button(c, "ok", w.action, "↩", true, pal) {
				done = true
			}
		})
	})
	if name := strings.TrimSpace(w.name); done && name != "" {
		w.open = false
		w.run(name)
	}
	if !w.open {
		a.side.git.ask = nil
	}
}
