package main

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"kero/internal/orchestrator"
	"kero/internal/proto"

	"kero/locales"
)

// Orchestrator sessions in the app (internal/daemon/orchestrator.go has
// the links and reports): an orchestrator's row carries its mark and a
// chevron that folds its children, which show right under it, each with
// a ↳; folded, the row counts those that need the user. Children don't
// notify, unless notify-children says to: their orchestrator hears of
// them.

// orchestratorIcon marks an orchestrator: a node and the three it starts.
var orchestratorIcon = icon(`<circle cx="8" cy="3.5" r="1.75"/><circle cx="3.25" cy="12.5" r="1.5"/><circle cx="8" cy="12.5" r="1.5"/><circle cx="12.75" cy="12.5" r="1.5"/><path d="M8 5.25V11M8 8H4.5a1.25 1.25 0 0 0-1.25 1.25V11M8 8h3.5a1.25 1.25 0 0 1 1.25 1.25V11"/>`)

// childrenOf are an orchestrator's children, in the sidebar's order.
func (a *App) childrenOf(id string) []*proto.Session {
	var kids []*proto.Session
	for i := range a.st.Sessions {
		if s := &a.st.Sessions[i]; s.Parent == id {
			kids = append(kids, s)
		}
	}
	return kids
}

// folded reports whether a session is a child its orchestrator's folded
// row hides; the session shown stays in view.
func (a *App) folded(s *proto.Session) bool {
	return s.Parent != "" && s.ID != a.selected && a.prefs.Folded[s.Parent] && a.session(s.Parent) != nil
}

// toggleChildren folds or unfolds an orchestrator's children, forgetting
// the orchestrators that are gone.
func (a *App) toggleChildren(id string) {
	if a.prefs.Folded == nil {
		a.prefs.Folded = map[string]bool{}
	}
	for k := range a.prefs.Folded {
		if a.session(k) == nil {
			delete(a.prefs.Folded, k)
		}
	}
	if a.prefs.Folded[id] {
		delete(a.prefs.Folded, id)
	} else {
		a.prefs.Folded[id] = true
	}
	a.savePrefs()
}

// mutedChild reports whether a session's notifications are left to its
// orchestrator (notify-children off).
func (a *App) mutedChild(s *proto.Session) bool {
	return s.Parent != "" && !a.set.NotifyChildren && a.session(s.Parent) != nil
}

// treeTip is what a session's tooltip says of its part in a tree.
func (a *App) treeTip(s *proto.Session) string {
	switch {
	case s.Orchestrator && s.Goal != "":
		return locales.SidebarOrchestratorTip() + "\n" + locales.SidebarGoal(s.Goal)
	case s.Orchestrator:
		return locales.SidebarOrchestratorTip()
	}
	if o := a.session(s.Parent); o != nil {
		return locales.SidebarChildOf(o.DisplayName(a.set.SessionName))
	}
	return ""
}

// orchestratorMark is an orchestrator's mark in its row, as a worktree's
// is: then, with children, their count and a chevron that folds them,
// which, folded, counts in the attention color those that need the user.
func (a *App) orchestratorMark(c *ui.Context, pal *palette, s *proto.Session) {
	ui.Icon(c, orchestratorIcon).Size(12, 12).TextColor(pal.accent).Shrink(0)
	kids := a.childrenOf(s.ID)
	if len(kids) == 0 {
		return
	}
	folded := a.prefs.Folded[s.ID]
	label := locales.SidebarHideChildren()
	if folded {
		label = locales.SidebarShowChildren()
	}
	needs := 0
	for _, k := range kids {
		if folded && (k.Status == proto.Waiting || k.Status == proto.Failed) {
			needs++
		}
	}
	b := ui.ButtonBase(c.Key("children")).Height(18).Padding(0, 4).Gap(3).Radius(5).AlignItems(ui.Center).Shrink(0).
		TextColor(pal.textFaint).Label(label).Tooltip(label)
	if needs > 0 {
		b.TextColor(pal.attention).Tooltip(label + "\n" + locales.SidebarNeedsYou(needs))
	}
	if b.Hovered() {
		b.Background(pal.chip)
		if needs == 0 {
			b.TextColor(pal.text)
		}
	}
	if b.Clicked() {
		id := s.ID
		a.defer_(func() { a.toggleChildren(id) })
	}
	b.Children(func() {
		if needs > 0 {
			ui.Box(c).Size(6, 6).Radius(3).Background(pal.attention).Shrink(0)
			ui.Textf(c, "%d", needs).FontSize(12).FontWeight(600).FontFeatures("tnum").SingleLine()
		} else {
			ui.Textf(c, "%d", len(kids)).FontSize(12).FontFeatures("tnum").SingleLine()
		}
		chev := ui.Icon(c, chevronIcon).Size(9, 9)
		chev.Rotate(chev.AnimateTo("turn", 90*(1-b2f(folded)), ui.Snappy))
	})
}

// childMark is the ↳ before a child's row, under its orchestrator.
func childMark(c *ui.Context, pal *palette) {
	ui.Text(c, "↳").FontSize(12.5).TextColor(pal.textFaint).Shrink(0).Margin(0, -4, 0, 0)
}

// askCloseOrchestrator asks what closing an orchestrator does with its
// children: close them too, or keep them, as top-level sessions. A
// child in a worktree of its own keeps it, for the cleanup, as kero
// close leaves it. The answer is the confirmation the orchestrator's
// close asks for, but for its own worktree's, which comes next; its
// children close once it has, which tells it nothing of them.
func (a *App) askCloseOrchestrator(s *proto.Session) bool {
	kids := a.childrenOf(s.ID)
	if len(kids) == 0 {
		return false
	}
	id, ids := s.ID, make([]string, len(kids))
	for i, k := range kids {
		ids[i] = k.ID
	}
	a.confirm = &confirmation{
		open:    true,
		title:   locales.ConfirmCloseOrchestratorTitle(s.DisplayName(a.set.SessionName)),
		detail:  locales.ConfirmCloseOrchestratorDetail(len(kids)),
		action:  locales.ConfirmCloseChildren(),
		destroy: true,
		run: func() {
			a.closeConfirmed(id, func() {
				for _, k := range ids {
					a.endSession(k, proto.CloseParams{ID: k}, nil)
				}
			})
		},
		alt:    locales.ConfirmKeepChildren(),
		altRun: func() { a.closeConfirmed(id, nil) },
	}
	return true
}

// closeConfirmed closes a session the user confirmed closing, asking
// still what of its own worktree; closed, unless nil, runs once it did.
func (a *App) closeConfirmed(id string, closed func()) {
	if s := a.session(id); s != nil && s.Worktree != nil && a.hostOf(id).cl != nil {
		a.askCloseWorktree(id, closed)
		return
	}
	a.endSession(id, proto.CloseParams{ID: id}, closed)
}

// orchestratorMenu is a session's menu items of orchestrators: make it
// one, or an ordinary session again; a child is none.
func (a *App) orchestratorMenu(m *ui.Menu, s *proto.Session) {
	if s.Parent != "" && !s.Orchestrator {
		return
	}
	id, label, off := s.ID, locales.SessionMakeOrchestrator(), s.Orchestrator
	if off {
		label = locales.SessionMakeOrdinary()
	}
	if m.Item(label).Chosen() {
		a.call(id, "orchestrator", proto.OrchestratorParams{ID: id, Off: off}, nil)
	}
}

// goalField is the New Session dialog's Goal of an orchestrator, which
// ⌘↩ starts.
func (a *App) goalField(c *ui.Context, pal *palette, w *newSessionAsk, start *bool) {
	ui.Text(c, locales.NewSessionGoal()).FontSize(13).TextColor(pal.text)
	frame := ui.Row(c.Key("goal")).AlignItems(ui.Start).Cursor(ui.CursorText)
	var in ui.Element
	frame.Children(func() {
		in = ui.TextAreaBase(c.Key("in"), &w.goal).Grow(1).MinWidth(0).MinHeight(64).MaxHeight(200).Padding(8).FontSize(13).
			Placeholder(locales.NewSessionGoalPlaceholder()).Label(locales.NewSessionGoal())
		in.OnShortcut(ui.Cmd, ui.KeyEnter, func() { *start = true })
	})
	fieldLook(c, frame, in.Focused(), pal).Padding(0)
}

// orchestratorSettings are Settings › Agents' of orchestrators: the role
// prompt, and how many children may run at once.
func (a *App) orchestratorSettings(c *ui.Context, pal *palette) {
	s := &a.set
	settingsGroup(c, pal, locales.SetOrchestrators(), locales.SetOrchestratorsDetail(), func() {
		settingRow(c, pal, locales.SetOrchestratorMax(), locales.SetOrchestratorMaxDetail(), func() {
			most := strconv.Itoa(s.OrchestratorMaxChildren)
			opts := []string{"0", "1", "2", "3", "4", "5", "6", "8", "10"}
			if !slices.Contains(opts, most) {
				opts = append(opts, most)
			}
			popup(c, "orch-max", &most, opts, func(v string) string {
				if v == "0" {
					return locales.SetOrchestratorMaxAny()
				}
				return v
			}, 140, pal)
			if n, err := strconv.Atoi(most); err == nil {
				s.OrchestratorMaxChildren = n
			}
		})
		promptSetting(c, pal, "orch-prompt", locales.SetOrchestratorPrompt(), locales.SetOrchestratorPromptDetail("{goal}", "{agent}"), &s.OrchestratorPrompt, orchestrator.DefaultPrompt, &a.ss.prompts[4])
	})
}
