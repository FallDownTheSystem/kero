package main

import (
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
	"kero/internal/settings"

	"kero/locales"
)

// The commit box, under the Uncommitted list: the message, which an agent
// can write (git.message), and the button that commits what's staged,
// with the options of a commit in its menu. While a merge, a rebase, a
// cherry-pick or a revert is in progress, the box ends it: Continue once
// its conflicts are resolved and staged, or Abort.

// commitDraft is a worktree's commit being written: its message and
// options, and the run writing the message, while one does.
type commitDraft struct {
	message                  string
	amend, signoff, noVerify bool
	// seeded is the message git prepared that the draft took, which it
	// takes once.
	seeded string
	// writing is the id of the run writing the message, "" for none, and
	// writePane its pane; err why the last run wrote none.
	writing   string
	writePane string
	err       string
	// kept is the message as it was before Amend put HEAD's in its place,
	// which turning Amend off puts back.
	kept string
}

// draft returns a worktree's draft, made as first asked for.
func (a *App) draft(root string) *commitDraft {
	g := &a.side.git
	if g.drafts == nil {
		g.drafts = map[string]*commitDraft{}
	}
	d := g.drafts[root]
	if d == nil {
		d = &commitDraft{}
		g.drafts[root] = d
	}
	return d
}

// commitBox is the box of the worktree of pane, whose index is ix.
func (a *App) commitBox(c *ui.Context, pal *palette, pane *proto.Pane, ix *proto.GitIndex) {
	g := pane.Git
	root := g.Root
	d := a.draft(root)
	if ix.Message != "" && ix.Message != d.seeded {
		d.seeded = ix.Message
		if strings.TrimSpace(d.message) == "" {
			d.message = ix.Message
		}
	}
	conflicts := 0
	for _, f := range ix.Unstaged {
		if f.Status == "conflict" {
			conflicts++
		}
	}
	busy := a.gitBusy(root)
	commit := func(push bool) {
		if !a.canCommit(d, ix, conflicts, busy) {
			return
		}
		a.commit(pane, d, push)
	}
	box := ui.Column(c.Key("commit")).Shrink(0).Padding(8, 12, 12, 12).Gap(6).AlignItems(ui.Stretch)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, pal.text.Alpha(0.08), 0)
	})
	box.Children(func() {
		if g.State != "" {
			a.operationRow(c, pal, pane, ix, conflicts)
		}
		var in ui.Element
		frame := ui.Row(c.Key("msg")).AlignItems(ui.Start).Cursor(ui.CursorText)
		frame.Children(func() {
			in = ui.TextAreaBase(c.Key("in"), &d.message).Grow(1).MinWidth(0).MinHeight(52).MaxHeight(150).Padding(6, 0, 6, 8).FontSize(13).
				Placeholder(locales.GitMessagePlaceholder(commitKeys())).Label(locales.GitMessage())
			in.OnShortcut(ui.Cmd, ui.KeyEnter, func() { commit(false) })
			a.writeButton(c, pal, pane, d)
		})
		fieldLook(c, frame, in.Focused(), pal).Padding(0)
		if d.err != "" {
			_, removed, _ := a.diffColors(c, pal)
			ui.Text(c, d.err).FontSize(12).TextColor(removed).LineHeight(1.35).Selectable()
		}
		ui.Row(c.Key("buttons")).Gap(1).AlignItems(ui.Stretch).Children(func() {
			label := locales.GitCommitFiles(len(ix.Staged))
			switch {
			case len(ix.Staged) == 0 && !d.amend:
				label = locales.GitCommit()
			case d.amend:
				label = locales.GitAmend()
			case g.State == "merge":
				label = locales.GitCommitMerge()
			}
			can := a.canCommit(d, ix, conflicts, busy)
			tip := locales.GitCommitTip()
			switch {
			case conflicts > 0:
				tip = locales.GitResolveFirst(conflicts)
			case len(ix.Staged) == 0 && !d.amend:
				tip = locales.GitStageFirst()
			case strings.TrimSpace(d.message) == "":
				tip = locales.GitWriteFirst()
			}
			b := ui.ButtonBase(c.Key("commit-btn")).Height(28).Grow(1).Padding(0, 12).Gap(6).Center().Label(label).Tooltip(tip)
			commitFace(b, pal, can, can, 8, 2)
			b.Children(func() {
				ui.Icon(c, checkIcon).Size(12, 12)
				ui.Text(c, label).FontSize(13).FontWeight(550).SingleLine().Ellipsis("…").MinWidth(0)
			})
			if b.Clicked() {
				commit(false)
			}
			opts := ui.ButtonBase(c.Key("commit-opts")).Width(28).Height(28).Radius(8).Center().Label(locales.GitCommitOptions()).Tooltip(locales.GitCommitOptions())
			commitFace(opts, pal, can, !busy, 2, 8)
			opts.Children(func() { ui.Icon(c, chevronDownIcon).Size(10, 10) })
			opts.Menu(func(m *ui.Menu) {
				if m.Item(locales.GitCommitAndPush()).Disabled(!can || g.Branch == "").Chosen() {
					commit(true)
				}
				m.Separator()
				if m.Item(locales.GitAmend()).Checked(d.amend).Disabled(g.Head == "").Chosen() {
					a.setAmend(pane, d, !d.amend)
				}
				if m.Item(locales.GitSignoff()).Checked(d.signoff).Chosen() {
					d.signoff = !d.signoff
				}
				if m.Item(locales.GitSkipHooks()).Checked(d.noVerify).Chosen() {
					d.noVerify = !d.noVerify
				}
			})
		})
	})
}

// commitKeys are the keys that commit from the message, as the platform
// writes them.
func commitKeys() string {
	if runtime.GOOS == "darwin" {
		return "⌘↩"
	}
	return "Ctrl+Enter"
}

// commitFace styles a part of the split commit button: the accent's while
// the box can commit, faint while it can't, as both parts look alike;
// enabled tells whether this part takes clicks. rl and rr are its
// corners at the left and the right.
func commitFace(b ui.Element, pal *palette, on, enabled bool, rl, rr float32) {
	bg, fg := pal.accent, ui.Hex("#ffffff")
	if !on {
		bg, fg = pal.text.Alpha(0.08), pal.textFaint
	}
	if !enabled {
		b.Disabled(true)
	}
	switch {
	case !enabled:
	case !on && (b.Hovered() || b.Pressed()):
		bg, fg = pal.text.Alpha(0.12), pal.textMuted
	case !on:
	case b.Pressed():
		bg = bg.Mix(ui.Hex("#000000"), 0.08)
	case b.Hovered():
		bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
	}
	b.Radius(rl, rr, rr, rl).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 90 * time.Millisecond})).TextColor(fg)
}

// canCommit tells whether the box can commit now: with a message, what's
// staged or Amend, no conflicts, and nothing else running there.
func (a *App) canCommit(d *commitDraft, ix *proto.GitIndex, conflicts int, busy bool) bool {
	return !busy && conflicts == 0 && strings.TrimSpace(d.message) != "" && (len(ix.Staged) > 0 || d.amend)
}

// commit commits what's staged, with the draft's message and options, and
// pushes after when push is set.
func (a *App) commit(pane *proto.Pane, d *commitDraft, push bool) {
	root := pane.Git.Root
	p := proto.GitOpParams{Op: "commit", Message: d.message, Amend: d.amend, Signoff: d.signoff, NoVerify: d.noVerify}
	label := locales.GitCommit()
	if d.amend {
		label = locales.GitAmend()
	}
	a.runGit(pane.ID, root, p, label, func(string) {
		d.message, d.amend, d.kept, d.err = "", false, "", ""
		if push {
			a.push(pane.ID, root, proto.GitOpParams{})
		}
	})
}

// setAmend turns Amend on, taking HEAD's message into an empty draft, or
// off, giving back the draft it replaced.
func (a *App) setAmend(pane *proto.Pane, d *commitDraft, on bool) {
	d.amend = on
	if !on {
		if d.kept != "" || d.message != "" {
			d.message, d.kept = d.kept, ""
		}
		return
	}
	d.kept = d.message
	a.runGit(pane.ID, pane.Git.Root, proto.GitOpParams{Op: "lastMessage"}, locales.GitAmend(), func(out string) {
		if d.amend && d.message == d.kept {
			d.message = out
		}
	})
}

// writeButton has an agent write the message of what's staged, or stops
// it while it writes.
func (a *App) writeButton(c *ui.Context, pal *palette, pane *proto.Pane, d *commitDraft) {
	tip := locales.GitWriteMessageTip()
	if agent := pane.AgentSession; agent != nil && a.set.CommitMessageCommand == "" {
		if name := proto.AgentNames[agent.Agent]; name != "" {
			tip = locales.GitWriteMessageFork(name)
		}
	}
	if a.set.CommitMessageCommand != "" {
		tip = locales.GitWriteMessageCommand(a.set.CommitMessageCommand)
	}
	b := ui.ButtonBase(c.Key("write")).Size(26, 26).Margin(3, 3, 0, 0).Radius(6).Center().Shrink(0).TextColor(pal.textMuted)
	if d.writing != "" {
		b.Label(locales.GitStopWriting()).Tooltip(locales.GitStopWriting())
	} else {
		b.Label(locales.GitWriteMessage()).Tooltip(tip)
	}
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() {
		if d.writing != "" {
			ui.Spinner(c).Size(13, 13)
		} else {
			ui.Icon(c, sparklesIcon).Size(14, 14)
		}
	})
	if !b.Clicked() {
		return
	}
	if d.writing != "" {
		if cl := a.hostOf(d.writePane).cl; cl != nil {
			go cl.Call("git.cancel", proto.IDParams{ID: d.writing}, nil)
		}
		d.writing = ""
		return
	}
	a.writeMessage(pane, d)
}

// writeMessage has the message written of what's staged in pane's
// worktree, in the draft's place.
func (a *App) writeMessage(pane *proto.Pane, d *commitDraft) {
	cl := a.hostOf(pane.ID).cl
	if cl == nil {
		return
	}
	id := opID()
	d.writing, d.writePane, d.err = id, pane.ID, ""
	p := proto.GitMessageParams{Pane: pane.ID, ID: id, Command: a.set.CommitMessageCommand, Prompt: a.set.CommitMessagePrompt,
		Repos: a.repoPrompts(func(r settings.ReviewSetup) string { return r.CommitPrompt })}
	go func() {
		var res proto.GitMessage
		err := cl.Call("git.message", p, &res)
		a.update(func() {
			if d.writing != id {
				return // stopped, or written over by another
			}
			d.writing = ""
			if err != nil {
				if err.Error() != "canceled" {
					d.err = locales.GitWriteFailed(err.Error())
				}
				return
			}
			d.message = res.Message
		})
	}()
}

// operationRow says the merge, rebase, cherry-pick or revert in progress,
// with Continue, once its conflicts are resolved and staged, and Abort;
// while they aren't, the agent working there can resolve them all.
func (a *App) operationRow(c *ui.Context, pal *palette, pane *proto.Pane, ix *proto.GitIndex, conflicts int) {
	g := pane.Git
	id, root := pane.ID, g.Root
	op := gitOpName(g.State)
	ui.Row(c.Key("op")).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, warningIcon).Size(12, 12).TextColor(pal.attention).Shrink(0)
		text := locales.GitOperationReady(op)
		if conflicts > 0 {
			text = locales.GitOperationConflicts(conflicts, op)
		}
		ui.Text(c, text).FontSize(12.5).TextColor(pal.textMuted).Grow(1).MinWidth(0).LineHeight(1.35)
		if conflicts > 0 && a.agentPane(pane) != nil && hunkButton(c, "agent", sparklesIcon, locales.GitResolveWithAgent(), locales.GitResolveWithAgentTip(), pal) {
			a.resolveWithAgent(pane, ix, conflictPaths(ix))
		}
		// A merge ends in a commit of the box's own.
		if conflicts == 0 && g.State != "merge" && hunkButton(c, "continue", checkIcon, locales.GitContinue(), locales.GitContinueTip(op), pal) {
			a.runGit(id, root, proto.GitOpParams{Op: "continue"}, locales.GitContinueOp(op), nil)
		}
		// Aborting drops the conflicts resolved so far: it asks first.
		if hunkButton(c, "abort", closeIcon, locales.GitAbort(), locales.GitAbortTip(op), pal) {
			a.confirm = &confirmation{open: true, title: locales.GitAbortOp(op), detail: locales.GitAbortTip(op), action: locales.GitAbort(), destroy: true, run: func() {
				a.guarded(id, root, locales.GitAbortOp(op), func() {
					a.runGit(id, root, proto.GitOpParams{Op: "abort"}, locales.GitAbortOp(op), nil)
				})
			}}
		}
	})
}

// gitOpName names an operation in progress, as a sentence takes it: "the
// rebase".
func gitOpName(st string) string {
	switch st {
	case "merge":
		return locales.GitOpMerge()
	case "rebase":
		return locales.GitOpRebase()
	case "cherry-pick":
		return locales.GitOpCherryPick()
	}
	return locales.GitOpRevert()
}
