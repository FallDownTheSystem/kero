package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// An editor pane whose file its worktree's index has in conflict shows it
// in the merge editor: the versions merged, git's stages of it, side by
// side, each conflict's lines taken by a checkbox, over the file's own
// text, which keeps the edits and saves as the editor pane's does. It
// shows as long as the index has the conflict: marked resolved, it is the
// editor pane it was.

// paneMerge is what the merge editor of an editor pane knows: the versions
// of its file, read as of stamp.
type paneMerge struct {
	path  string
	stamp string
	busy  bool
	// m is nil while the file is in no conflict.
	m *editor.MergeState
	// text shows the file as text instead, as the user chose.
	text bool
	// at is the conflict gone to last (stepConflict).
	at int
}

// conflictRepo returns the terminal pane of a session whose worktree has
// file, while a merge, rebase, cherry-pick or revert is under way there,
// and the file's path in it, slash-separated; nil for none.
func conflictRepo(s *proto.Session, file string) (*proto.Pane, string) {
	for i := range s.Panes {
		q := &s.Panes[i]
		if !q.Terminal() || q.Git == nil || q.Git.State == "" || !proto.Within(file, q.Git.Root) {
			continue
		}
		rel := strings.TrimLeft(strings.TrimPrefix(file, q.Git.Root), `/\`)
		if rel == "" {
			continue
		}
		return q, strings.ReplaceAll(rel, `\`, "/")
	}
	return nil, ""
}

// mergeOf returns the merge editor of an editor pane while its file is in
// conflict, else nil: it reads the conflict's versions as the worktree
// changes, through the terminal pane of the same worktree.
func (a *App) mergeOf(s *proto.Session, p *proto.Pane, f *openFile) *paneMerge {
	repo, rel := conflictRepo(s, p.File)
	if repo == nil || f.st == nil {
		if a.merges != nil {
			delete(a.merges, p.ID)
		}
		return nil
	}
	if a.merges == nil {
		a.merges = map[string]*paneMerge{}
	}
	pm := a.merges[p.ID]
	if pm == nil || pm.path != p.File {
		pm = &paneMerge{path: p.File}
		a.merges[p.ID] = pm
	}
	if stamp := gitStamp(repo.Git); pm.stamp != stamp && !pm.busy {
		a.readMerge(pm, repo.ID, rel, stamp, f)
	}
	if pm.m != nil && pm.m.Result != f.st.Buffer {
		pm.m = nil // read anew, as another file
		pm.stamp = ""
	}
	return pm
}

// readMerge reads whether the index has the file in conflict, and its
// versions then (git.file of its stages 2 and 3) with the names of the
// operation's sides.
func (a *App) readMerge(pm *paneMerge, pane, rel, stamp string, f *openFile) {
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	pm.busy = true
	go func() {
		var ix proto.GitIndex
		var tx proto.GitText
		err := cl.Call("git.index", proto.GitParams{Pane: pane}, &ix)
		conflict := false
		for _, g := range ix.Unstaged {
			conflict = conflict || g.Path == rel && g.Status == "conflict"
		}
		if err == nil && conflict {
			err = cl.Call("git.file", proto.GitParams{Pane: pane, Base: baseRev, Path: rel, From: git.SideOursStage, To: git.SideTheirsStage}, &tx)
		}
		a.update(func() {
			pm.busy, pm.stamp = false, stamp
			if err != nil || !conflict || tx.Binary || tx.Large || f.st == nil {
				pm.m = nil
				return
			}
			ours, theirs := sideNames(&ix)
			if pm.m == nil || pm.m.Result != f.st.Buffer {
				pm.m = &editor.MergeState{Ours: editor.NewBuffer(tx.Old), Theirs: editor.NewBuffer(tx.New), Result: f.st.Buffer}
			} else {
				resetText(pm.m.Ours, tx.Old)
				resetText(pm.m.Theirs, tx.New)
			}
			pm.m.OursLabel, pm.m.TheirsLabel = ours, theirs
		})
	}()
}

// resetText sets a version's text, when it changed.
func resetText(b *editor.Buffer, text string) {
	if b.String() != strings.ReplaceAll(text, "\r\n", "\n") {
		b.Reset(text)
	}
}

// mergeBody is an editor pane's file in the merge editor, whose result
// takes the pane's keyboard, by ref; and what a save found changed on
// disk.
func (a *App) mergeBody(c *ui.Context, pal *palette, pane string, f *openFile, pm *paneMerge, ref *ui.Handle) {
	m := pm.m
	m.Font = ui.Font{Family: a.termFamily(), Size: max(a.set.FontSize-1, 10)}
	m.Theme = a.codeTheme(c, pal)
	m.Wrap = a.prefs.Wrap
	m.ResultHandle = ref
	col := ui.Column(c.Key("merge")).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	col.Children(func() {
		if f.changed {
			a.fileChanged(c, pal, pane, f)
		}
		editor.MergeView(c.Key("view"), m).Grow(1).MinHeight(0)
	})
	if col.Shortcut(ui.Cmd, ui.KeyS) && f.modified() && !f.saving {
		a.saveFile(pane, f, false)
	}
}

// mergeBar is an editor pane's bar while it shows the merge editor: how
// many conflicts are left and the way between them, the version to take
// in all of them, the text instead, the save, and Mark Resolved, which
// saves first, once none is left.
func (a *App) mergeBar(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane, f *openFile, pm *paneMerge) {
	m := pm.m
	left := m.Unresolved()
	bottomBar(c, "bar", pal).Padding(0, 6, 0, 12).Children(func() {
		a.fileStatus(c, pal, f)
		ui.Text(c, locales.MergeConflictsLeft(left)).FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		ui.Spacer(c)
		if n := len(m.Conflicts()); n > 1 {
			iconButton(c, "prev", chevronUpIcon, locales.MergePrevious(), pal, func() { a.stepConflict(pm, -1) })
			iconButton(c, "next", chevronDownIcon, locales.MergeNext(), pal, func() { a.stepConflict(pm, 1) })
		}
		ours, theirs := m.Labels()
		b := ui.ButtonBase(c.Key("all")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.GitResolve()).Tooltip(locales.GitResolve())
		if b.Hovered() || b.Pressed() {
			b.Background(pal.rowHover).TextColor(pal.text)
		}
		b.Children(func() { ui.Icon(c, mergeIcon).Size(15, 15) })
		b.Menu(func(mn *ui.Menu) {
			if mn.Item(locales.MergeUseAll(ours)).Chosen() {
				m.ChooseAll(editor.MergeOurs)
			}
			if mn.Item(locales.MergeUseAll(theirs)).Chosen() {
				m.ChooseAll(editor.MergeTheirs)
			}
			mn.Separator()
			if mn.Item(locales.MergeShowText()).Chosen() {
				pm.text = true
			}
		})
		a.fileActions(c, pal, p.ID, f)
		label := locales.GitMarkResolved()
		if f.modified() {
			label = locales.GitSaveAndMarkResolved()
		}
		tip := label
		if left > 0 {
			tip = locales.GitResolveFirst(left)
		}
		mark := ui.ButtonBase(c.Key("resolved")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).Tooltip(tip).Disabled(left > 0 || f.saving)
		if (mark.Hovered() || mark.Pressed()) && left == 0 {
			mark.Background(pal.rowHover).TextColor(pal.text)
		}
		mark.Children(func() { ui.Icon(c, checkIcon).Size(16, 16) })
		if mark.Clicked() {
			a.markMerged(s, p, f)
		}
	})
}

// stepConflict goes to the conflict before or after the one gone to last,
// round from the last to the first.
func (a *App) stepConflict(pm *paneMerge, dir int) {
	n := len(pm.m.Conflicts())
	if n == 0 {
		return
	}
	pm.at = ((pm.at+dir)%n + n) % n
	pm.m.GoTo(pm.at)
}

// markMerged saves the merged file when it has edits, then stages it,
// which tells git its conflict is resolved.
func (a *App) markMerged(s *proto.Session, p *proto.Pane, f *openFile) {
	repo, rel := conflictRepo(s, p.File)
	if repo == nil {
		return
	}
	id, root := repo.ID, repo.Git.Root
	stage := func() {
		a.runGit(id, root, proto.GitOpParams{Op: "stage", Paths: []string{rel}}, locales.GitStage(), nil)
	}
	if !f.modified() {
		stage()
		return
	}
	a.saveFileThen(p.ID, f, false, stage)
}

// mergeTextBar is the button of an editor pane that shows its file in
// conflict as text, which shows the merge editor again.
func (a *App) mergeTextBar(c *ui.Context, pal *palette, pm *paneMerge) {
	iconButton(c, "merge", mergeIcon, locales.MergeShowEditor(), pal, func() { pm.text = false })
}

// openMerge opens a file in conflict in an editor pane, which shows it in
// the merge editor.
func (a *App) openMerge(pane *proto.Pane, f proto.GitFile) {
	root := pane.Git.Root
	rel := f.Path
	if a.hostOf(pane.ID).windows() {
		rel = strings.ReplaceAll(rel, "/", `\`)
	}
	a.openInPane(pane.ID, a.hostOf(pane.ID).joinPath(root, rel))
}
