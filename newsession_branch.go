package main

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/commitmsg"
	"kero/internal/proto"

	"kero/locales"
)

// The New Session dialog's task: what an agent's session in a new worktree
// starts on, which names the worktree's new branch (git.branchName), as
// t3code names its worktrees' branches from the first message, unless the
// user names it. The agent is given it as its first prompt.

// taskShown reports whether the dialog asks for a task: for an agent's
// session on a new branch of its own worktree.
func (w *newSessionAsk) taskShown() bool {
	return w.kind == kindAgent && w.worktree && w.fresh && w.gitErr == "" && w.branches.list != nil
}

// namesBranch reports whether starting names the new branch first: the
// user left it unnamed, and there is a task to name it from.
func (w *newSessionAsk) namesBranch() bool {
	return w.taskShown() && strings.TrimSpace(w.newBranch) == "" && strings.TrimSpace(w.task) != ""
}

// branchPrefix is what goes before a named branch, as the settings say:
// the kind of the work, the user's prefix, or what their prompt says.
func (a *App) branchPrefix() string {
	switch a.set.WorktreeBranchPrefix {
	case "static":
		return a.set.WorktreeBranchStatic
	case "custom":
		return ""
	}
	return commitmsg.DefaultBranchPrefix
}

// nameBranch has the new branch named from the dialog's task, in its name
// field, then, unless nil, runs then. A name the user types meanwhile
// stays. When the model can't, the field has one of the task's first
// words, which the user sees before starting.
func (a *App) nameBranch(w *newSessionAsk, then func()) {
	cl := w.host.cl
	if cl == nil || w.naming != "" {
		return
	}
	id := opID()
	task, prefix := strings.TrimSpace(w.task), a.branchPrefix()
	w.naming, w.nameErr = id, ""
	p := proto.GitBranchNameParams{Dir: w.dir, ID: id, Task: task, Command: a.set.CommitMessageCommand,
		Prompt: a.set.WorktreeBranchPrompt, Prefix: prefix}
	go func() {
		var res proto.GitBranchName
		err := cl.Call("git.branchName", p, &res)
		a.update(func() {
			if w.naming != id {
				return // stopped
			}
			w.naming = ""
			if strings.TrimSpace(w.newBranch) != "" {
				return // the user named it
			}
			if err != nil || res.Name == "" {
				if err != nil && err.Error() == "canceled" {
					return
				}
				msg := "no name"
				if err != nil {
					msg = err.Error()
				}
				w.newBranch = commitmsg.UniqueBranch(commitmsg.BranchFallback(task, prefix), func(s string) bool { return hasLocalBranch(w.branches.list, s) })
				w.nameErr = locales.NewSessionNameFailed(msg, w.newBranch)
				return
			}
			w.newBranch = res.Name
			if then != nil && w.open && a.newAsk == w {
				then()
			}
		})
	}()
}

// stopNaming stops the naming of the dialog's branch under way.
func (a *App) stopNaming(w *newSessionAsk) {
	if w.naming == "" {
		return
	}
	if cl := w.host.cl; cl != nil {
		go cl.Call("git.cancel", proto.IDParams{ID: w.naming}, nil)
	}
	w.naming = ""
}

// taskField is the dialog's task, which Cmd+Return starts the session of.
func (a *App) taskField(c *ui.Context, pal *palette, w *newSessionAsk, start *bool) {
	ui.Text(c, locales.NewSessionTask()).FontSize(13).TextColor(pal.text)
	frame := ui.Row(c.Key("task")).AlignItems(ui.Start).Cursor(ui.CursorText)
	var in ui.Element
	frame.Children(func() {
		in = ui.TextAreaBase(c.Key("in"), &w.task).Grow(1).MinWidth(0).MinHeight(64).MaxHeight(200).Padding(8).FontSize(13).
			Placeholder(locales.NewSessionTaskPlaceholder()).Label(locales.NewSessionTask())
		in.OnShortcut(ui.Cmd, ui.KeyEnter, func() { *start = true })
	})
	fieldLook(c, frame, in.Focused(), pal).Padding(0)
}

// nameButton names the new branch from the task, for the user to see or
// change before starting, or stops it while it names it.
func (a *App) nameButton(c *ui.Context, pal *palette, w *newSessionAsk) {
	has := strings.TrimSpace(w.task) != ""
	label := locales.NewSessionNameBranch()
	if w.naming != "" {
		label = locales.NewSessionStopNaming()
	}
	b := ui.ButtonBase(c.Key("name-branch")).Size(26, 26).Radius(6).Center().Shrink(0).TextColor(pal.textMuted).
		Label(label).Tooltip(label).Opacity(1 - 0.5*b2f(!has && w.naming == ""))
	if has && (b.Hovered() || b.Pressed()) {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() {
		if w.naming != "" {
			ui.Spinner(c).Size(13, 13)
		} else {
			ui.Icon(c, sparklesIcon).Size(14, 14)
		}
	})
	switch {
	case !b.Clicked():
	case w.naming != "":
		a.stopNaming(w)
	case has:
		w.newBranch = ""
		a.nameBranch(w, nil)
	}
}

// agentArgs is an agent's command given the task as its first prompt: in
// place of {prompt} in it, else after it, as OpenCode's --prompt.
func agentArgs(command []string, task string) []string {
	task = strings.TrimSpace(task)
	if task == "" || len(command) == 0 {
		return command
	}
	out := slices.Clone(command)
	placed := false
	for i, word := range out {
		if strings.Contains(word, promptPlaceholder) {
			out[i], placed = strings.ReplaceAll(word, promptPlaceholder, task), true
		}
	}
	switch {
	case placed:
	case strings.TrimSuffix(filepath.Base(strings.ReplaceAll(out[0], `\`, "/")), ".exe") == "opencode":
		out = append(out, "--prompt", task)
	default:
		out = append(out, task)
	}
	return out
}
