package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"kero/internal/proto"
	"kero/locales"
)

// The progress cards of the setups of worktrees kero made (the daemon's
// worktree_setup.go), at the window's bottom right, one a setup, until a
// while after it ends: its stages, and Cancel while it runs, or Show
// Terminal for its script's.

var pendingIcon = icon(`<circle cx="8" cy="8" r="3.5"/>`)

func (a *App) worktreeSetupCards(c *ui.Context, pal *palette) {
	type hostSetup struct {
		h *host
		s proto.WorktreeSetup
	}
	var all []hostSetup
	for _, h := range a.hosts {
		for _, s := range h.setups {
			all = append(all, hostSetup{h, s})
		}
	}
	if len(all) == 0 {
		return
	}
	ui.Overlay(c, func() {
		ui.Column(c.Key("worktree-setups")).Absolute().Right(16).Bottom(16).Width(320).Gap(8).AlignItems(ui.Stretch).Children(func() {
			for _, hs := range all {
				a.worktreeSetupCard(c.Key("setup-"+hs.s.ID), pal, hs.h, hs.s)
			}
		})
	})
}

func (a *App) worktreeSetupCard(c *ui.Context, pal *palette, h *host, s proto.WorktreeSetup) {
	name := s.Branch
	if name == "" {
		name = proto.DirName(s.Path)
	}
	title := locales.WorktreeSetupRunning(name)
	switch s.Phase {
	case proto.SetupDone:
		title = locales.WorktreeSetupDone(name)
	case proto.SetupFailed:
		title = locales.WorktreeSetupFailed(name)
	case proto.SetupCancelled:
		title = locales.WorktreeSetupCancelled(name)
	}
	card := ui.Column(c).Padding(12, 14).Gap(6).Radius(10).Background(pal.popover).AlignItems(ui.Stretch).Label(title)
	floating(card, pal).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, worktreeIcon).Size(14, 14).Shrink(0).TextColor(pal.textMuted)
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine().Grow(1).Shrink(1)
			if !s.Running() {
				iconButton(c, "close", closeIcon, locales.CommonClose(), pal, func() { a.cancelSetup(h, s.ID) })
			}
		})
		for _, st := range s.Stages {
			// A stage that never came, as the fetch kero doesn't make.
			if st.Status == proto.SetupSkipped && st.Detail == "" {
				continue
			}
			setupStageRow(c.Key("stage-"+st.ID), pal, s, st)
		}
		if s.Error != "" {
			ui.Text(c, s.Error).FontSize(12).TextColor(pal.danger).LineHeight(1.35)
		}
		showTerm := s.Pane != "" && s.Session != "" && a.session(s.Session) != nil && paneIn(a.session(s.Session), s.Pane)
		if !s.Running() && !showTerm {
			return
		}
		ui.Row(c).Gap(6).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if showTerm && smallButton(c, "terminal", locales.WorktreeSetupShowTerminal(), false, pal) {
				a.defer_(func() { a.showSetupTerminal(s.Session, s.Pane) })
			}
			if s.Running() && smallButton(c, "cancel", locales.CommonCancel(), false, pal) {
				a.cancelSetup(h, s.ID)
			}
		})
	})
}

func setupStageRow(c *ui.Context, pal *palette, s proto.WorktreeSetup, st proto.SetupStage) {
	label := ""
	switch st.ID {
	case proto.SetupFetch:
		label = locales.WorktreeSetupStageFetch()
	case proto.SetupCheckout:
		label = locales.WorktreeSetupStageCheckout()
	case proto.SetupSubmodules:
		label = locales.WorktreeSetupStageSubmodules()
	case proto.SetupScript:
		label = locales.WorktreeSetupStageScript(s.Script)
	case proto.SetupAgent:
		label = locales.WorktreeSetupStageAgent()
	default:
		label = st.ID
	}
	detail := st.Detail
	if detail == proto.SetupCancelled {
		detail = locales.WorktreeSetupStageCancelled()
	}
	color := pal.text
	if st.Status == proto.SetupPending || st.Status == proto.SetupSkipped {
		color = pal.textMuted
	}
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Size(14, 16).Shrink(0).Center().Children(func() {
			switch st.Status {
			case proto.SetupRunning:
				ui.Spinner(c).Size(12, 12)
			case proto.SetupDone:
				ui.Icon(c, checkIcon).Size(12, 12).TextColor(pal.done)
			case proto.SetupWarning:
				ui.Icon(c, warningIcon).Size(12, 12).TextColor(pal.attention)
			case proto.SetupFailed:
				ui.Icon(c, closeIcon).Size(12, 12).TextColor(pal.danger)
			case proto.SetupSkipped:
				ui.Icon(c, minusIcon).Size(12, 12).TextColor(pal.textFaint)
			default:
				ui.Icon(c, pendingIcon).Size(12, 12).TextColor(pal.textFaint)
			}
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(color).SingleLine()
			if detail != "" {
				d := ui.Text(c, detail).FontSize(11.5).TextColor(pal.textFaint).SingleLine()
				if st.ID == proto.SetupScript && st.Status == proto.SetupRunning {
					d.Font("monospace")
				}
			}
		})
	})
}

func paneIn(s *proto.Session, id string) bool {
	for _, p := range s.Panes {
		if p.ID == id {
			return true
		}
	}
	return false
}

// showSetupTerminal shows the terminal a setup script runs in.
func (a *App) showSetupTerminal(sid, pane string) {
	a.selectSession(sid)
	if s := a.session(sid); s != nil && paneIn(s, pane) {
		a.setFocus(s, pane)
		a.paneRef(pane).Focus()
	}
}

// cancelSetup cancels a setup that runs on h, or closes the card of one
// that ended.
func (a *App) cancelSetup(h *host, id string) {
	cl := h.cl
	if cl == nil {
		return
	}
	go func() {
		err := cl.Call("worktree.setup.cancel", proto.IDParams{ID: id}, nil)
		if err != nil {
			a.update(func() { a.toast(fmt.Sprint(err)) })
		}
	}()
}
