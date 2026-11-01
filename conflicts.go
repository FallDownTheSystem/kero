package main

import (
	"path"
	"strings"
	"time"

	"kero/internal/proto"

	"kero/locales"
)

// Merges and rebases of another branch into a worktree's, and the
// conflicts they stop on: resolved a file at a time by one side's version
// of it, by hand and marked resolved, or by an agent working there.

// mergeInto merges rev, named name, into the worktree's branch, which
// changes its files: it waits for the agents there, or goes ahead as the
// user says.
func (a *App) mergeInto(pane *proto.Pane, rev, name string, noFF bool) {
	id, root := pane.ID, pane.Git.Root
	p := proto.GitOpParams{Op: "merge", Rev: rev}
	if noFF {
		p.Mode = "no-ff"
	}
	a.guarded(id, root, locales.GitMergeOp(name), func() { a.runGit(id, root, p, locales.GitMergeOp(name), nil) })
}

// rebaseOnto replays the worktree's branch's commits on rev, named name,
// once the user says so: it makes them commits anew, which a branch
// pushed before then needs a force push for. The changes not committed
// are stashed for it and put back after (--autostash), which git would
// refuse to rebase over.
func (a *App) rebaseOnto(pane *proto.Pane, rev, name string) {
	g := pane.Git
	id, root := pane.ID, g.Root
	branch := branchName(g)
	detail, action := locales.GitRebaseDetail(branch, name), locales.GitRebase()
	if g.Files > 0 {
		detail += " " + locales.GitRebaseStashDetail(g.Files)
		action = locales.GitStashAndRebase()
	}
	a.confirm = &confirmation{open: true, title: locales.GitRebaseTitle(branch, name), detail: detail,
		action: action, run: func() {
			a.guarded(id, root, locales.GitRebaseOp(name), func() {
				a.runGit(id, root, proto.GitOpParams{Op: "rebase", Rev: rev, Autostash: true}, locales.GitRebaseOp(name), nil)
			})
		}}
}

// canMergeInto tells whether the worktree can take a merge or a rebase
// now: none in progress there, which it would have to end first.
func canMergeInto(g *proto.GitInfo) bool { return g != nil && g.State == "" && g.Head != "" }

// resolveSide resolves a conflicted file by one side's version of it, as
// the user says: its edits made since the conflict go with its markers.
func (a *App) resolveSide(pane *proto.Pane, f proto.GitFile, side, name string) {
	id, root := pane.ID, pane.Git.Root
	file := path.Base(f.Path)
	a.confirm = &confirmation{open: true, title: locales.GitUseSideTitle(name, file), detail: locales.GitUseSideDetail(name),
		action: locales.GitUseSide(name), destroy: true, run: func() {
			a.guardedNow(id, root, locales.GitResolveOp(file), func() {
				a.runGit(id, root, proto.GitOpParams{Op: "resolve", Path: f.Path, Mode: side}, locales.GitResolveOp(file), nil)
			})
		}}
}

// sideNames are the names of a conflict's sides, as the index tells them,
// else its sides' own.
func sideNames(ix *proto.GitIndex) (ours, theirs string) {
	ours, theirs = ix.Ours, ix.Theirs
	if ours == "" {
		ours = locales.GitOurs()
	}
	if theirs == "" {
		theirs = locales.GitTheirs()
	}
	return ours, theirs
}

// agentPane is the agent's pane that resolves a worktree's conflicts: the
// one shown, when it is an agent's, else another's working in the same
// worktree; nil for none.
func (a *App) agentPane(pane *proto.Pane) *proto.Pane {
	if pane.Agent != "" {
		return pane
	}
	h := a.hostOf(pane.ID)
	for i := range a.st.Sessions {
		s := &a.st.Sessions[i]
		if a.hostOf(s.ID) != h {
			continue
		}
		for j := range s.Panes {
			p := &s.Panes[j]
			if p.Agent != "" && p.Git != nil && p.Git.Root == pane.Git.Root {
				return p
			}
		}
	}
	return nil
}

// resolvePrompt is what an agent is asked to resolve conflicts by: each
// file's both sides kept as they mean to be, staged, and the operation
// left for the user to continue.
func resolvePrompt(op string, ix *proto.GitIndex, paths []string) string {
	ours, theirs := ix.Ours, ix.Theirs
	sides := ""
	if ours != "" && theirs != "" {
		sides = " between " + ours + " (ours) and " + theirs + " (theirs)"
	}
	return "Resolve the git " + op + " conflicts" + sides + " in " + strings.Join(paths, ", ") +
		". Keep what each side means to do, remove every conflict marker, and check that the result builds. " +
		"Then stage each resolved file with git add. Don't commit, and don't continue or abort the " + op + "."
}

// resolveWithAgent asks the worktree's agent to resolve its conflicts: by
// the prompt typed in its pane, then Enter, as kero prompt does.
func (a *App) resolveWithAgent(pane *proto.Pane, ix *proto.GitIndex, paths []string) {
	ap := a.agentPane(pane)
	cl := a.hostOf(pane.ID).cl
	if ap == nil || cl == nil || len(paths) == 0 {
		return
	}
	text := resolvePrompt(pane.Git.State, ix, paths)
	id, name := ap.ID, proto.AgentNames[ap.Agent]
	if name == "" {
		name = locales.GitAnAgent()
	}
	go func() {
		err := cl.Call("send", proto.SendParams{ID: id, Data: text}, nil)
		if err == nil {
			// Apart from the text, which a program could take for a paste
			// of a newline.
			time.Sleep(120 * time.Millisecond)
			err = cl.Call("send", proto.SendParams{ID: id, Data: "\r"}, nil)
		}
		a.update(func() {
			if err != nil {
				a.toast(locales.GitAgentSendFailed(err.Error()))
				return
			}
			a.toast(locales.GitSentToAgent(name))
		})
	}()
}

// conflictPaths are the paths of the index's conflicts.
func conflictPaths(ix *proto.GitIndex) []string {
	var out []string
	for _, f := range ix.Unstaged {
		if f.Status == "conflict" {
			out = append(out, f.Path)
		}
	}
	return out
}
