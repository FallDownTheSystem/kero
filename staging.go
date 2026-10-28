package main

import (
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// Uncommitted's sections, staged and not, and their files' buttons, which
// stage, unstage and discard them; their changes' own, and those of the
// lines selected of a change, are in the change's bar (diffOptions).

// sectionColor is the color a section's title and the stripe along its
// files are in: the added lines' for what's staged, the modified ones'
// for what isn't.
func (a *App) sectionColor(c *ui.Context, pal *palette, section string) ui.Color {
	added, _, modified := a.diffColors(c, pal)
	if section == baseStaged {
		return added
	}
	return modified
}

// indexTitle is a section's title, with the buttons for all of its files.
func (a *App) indexTitle(c *ui.Context, pal *palette, pane *proto.Pane, ix *proto.GitIndex, section string) {
	list, title, icon := ix.Unstaged, locales.GitChanges(), diffIcon
	if section == baseStaged {
		list, title, icon = ix.Staged, locales.GitStagedChanges(), checkIcon
	}
	col := a.sectionColor(c, pal, section)
	top := float32(4)
	if section == baseUnstaged && len(ix.Staged) > 0 {
		top = 14 // apart from the staged files above it
	}
	band := ui.Row(c.Key("title")).Height(34).Margin(top, 0, 4, 0).Padding(0, 6, 0, 12).Gap(7).AlignItems(ui.Center).Background(col.Alpha(0.1))
	band.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(ui.Rect{X: r.X, Y: r.Y, W: 3, H: r.H}, col, 0) })
	band.Children(func() {
		scope := changesScope(pane, git.BaseHead)
		keys := make([]string, len(list))
		for i, f := range list {
			keys[i] = fileKey(scope, section, f.Path)
		}
		a.foldAllButton(c, pal, keys)
		ui.Icon(c, icon).Size(13, 13).TextColor(col).Shrink(0)
		ui.Text(c, title).FontSize(12.5).FontWeight(650).TextColor(pal.text).SingleLine().Shrink(0)
		ui.Box(c).Height(16).Padding(0, 6).Radius(8).Center().Background(col.Alpha(0.16)).Children(func() {
			ui.Text(c, itoa(len(list))).FontSize(11).FontFeatures("tnum").TextColor(pal.textMuted).SingleLine()
		})
		ui.Spacer(c)
		a.indexListTools(c, pal, pane, section, list, "")
	})
}

// indexListTools are the buttons of a section's files: all of them, in
// its title's, or those in folder dir of the tree, in its row's.
func (a *App) indexListTools(c *ui.Context, pal *palette, pane *proto.Pane, section string, list []proto.GitFile, dir string) {
	id, root := pane.ID, pane.Git.Root
	if dir != "" {
		var in []proto.GitFile
		for _, f := range list {
			if strings.HasPrefix(f.Path, dir+"/") {
				in = append(in, f)
			}
		}
		list = in
	}
	if len(list) == 0 {
		return
	}
	// A folder's name its files' paths, a renamed file's old one too, as
	// its own button does; the title's none, which is all of them.
	paths := func(files []proto.GitFile) []string {
		if dir == "" {
			return nil
		}
		var out []string
		for _, f := range files {
			out = append(out, f.Path)
			if f.OldPath != "" {
				out = append(out, f.OldPath)
			}
		}
		return out
	}
	unstage, discard, stage := locales.GitUnstageAll(), locales.GitDiscardAll(), locales.GitStageAll()
	if dir != "" {
		unstage, discard, stage = locales.GitUnstage(), locales.GitDiscard(), locales.GitStage()
	}
	if section == baseStaged {
		iconButton(c, "unstage-all", minusIcon, unstage, pal, func() {
			a.moveFiles(pane, section, list)
			a.runGit(id, root, proto.GitOpParams{Op: "unstage", Paths: paths(list)}, locales.GitUnstage(), nil)
		})
		return
	}
	iconButton(c, "discard-all", revertIcon, discard, pal, func() {
		title, action := locales.GitDiscardAllTitle(), locales.GitDiscardAll()
		if dir != "" {
			title, action = locales.GitDiscardTitle(dir+"/"), locales.GitDiscardChanges()
		}
		a.discard(id, root, list, title, locales.GitDiscardAllDetail(len(list)), action)
	})
	iconButton(c, "stage-all", plusIcon, stage, pal, func() {
		// Staging a conflict marks it resolved, its markers and all: with
		// conflicts, all stages the other files shown, each marked
		// resolved by its own button.
		files := list
		if slices.ContainsFunc(list, func(f proto.GitFile) bool { return f.Status == "conflict" }) {
			files = slices.DeleteFunc(slices.Clone(list), func(f proto.GitFile) bool { return f.Status == "conflict" })
			if len(files) == 0 {
				return
			}
		}
		p := proto.GitOpParams{Op: "stage", Paths: paths(files)}
		if dir == "" && len(files) < len(list) {
			for _, f := range files {
				p.Paths = append(p.Paths, f.Path)
			}
		}
		a.moveFiles(pane, section, files)
		a.runGit(id, root, p, locales.GitStage(), nil)
	})
}

// moveFiles is what of files the Changes view keeps as they move from a
// section to the other, staged or unstaged: each folded or not, as it was,
// rather than as a file first shown is.
func (a *App) moveFiles(pane *proto.Pane, from string, files []proto.GitFile) {
	ch := &a.side.changes
	scope := changesScope(pane, git.BaseHead)
	to := baseStaged
	if from == baseStaged {
		to = baseUnstaged
	}
	if ch.collapsed == nil {
		ch.collapsed = map[string]bool{}
	}
	for _, f := range files {
		ch.collapsed[fileKey(scope, to, f.Path)] = a.fileCollapsed(fileKey(scope, from, f.Path))
	}
}

// indexTools are a file's buttons in its section's list: a conflict's,
// the menu that resolves it by a side's version or an agent, and Mark
// Resolved, once it is by hand.
func (a *App) indexTools(c *ui.Context, pal *palette, pane *proto.Pane, ix *proto.GitIndex, section string, f proto.GitFile) {
	id, root := pane.ID, pane.Git.Root
	paths := []string{f.Path}
	if f.OldPath != "" {
		paths = append(paths, f.OldPath)
	}
	name := path.Base(f.Path)
	switch {
	case section == baseStaged:
		iconButton(c, "unstage", minusIcon, locales.GitUnstage(), pal, func() {
			a.moveFiles(pane, section, []proto.GitFile{f})
			a.runGit(id, root, proto.GitOpParams{Op: "unstage", Paths: paths}, locales.GitUnstage(), nil)
		})
	case f.Status == "conflict":
		ours, theirs := sideNames(ix)
		b := ui.ButtonBase(c.Key("resolve")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.GitResolve()).Tooltip(locales.GitResolve())
		if b.Hovered() || b.Pressed() {
			b.Background(pal.rowHover).TextColor(pal.text)
		}
		b.Children(func() { ui.Icon(c, mergeIcon).Size(15, 15) })
		b.Menu(func(m *ui.Menu) {
			if m.Item(locales.GitOpenMergeEditor()).Chosen() {
				a.openMerge(pane, f)
			}
			m.Separator()
			if m.Item(locales.GitUseSide(ours)).Chosen() {
				a.resolveSide(pane, f, git.SideOurs, ours)
			}
			if m.Item(locales.GitUseSide(theirs)).Chosen() {
				a.resolveSide(pane, f, git.SideTheirs, theirs)
			}
			m.Separator()
			if m.Item(locales.GitResolveWithAgent()).Disabled(a.agentPane(pane) == nil).Chosen() {
				a.resolveWithAgent(pane, ix, []string{f.Path})
			}
		})
		iconButton(c, "resolved", checkIcon, locales.GitMarkResolved(), pal, func() {
			a.moveFiles(pane, section, []proto.GitFile{f})
			a.runGit(id, root, proto.GitOpParams{Op: "stage", Paths: paths}, locales.GitStage(), nil)
		})
	default:
		title, detail, action := locales.GitDiscardTitle(name), locales.GitDiscardDetail(), locales.GitDiscardChanges()
		switch f.Status {
		case "untracked":
			title, detail, action = locales.GitDeleteTitle(name), locales.GitDeleteDetail(), locales.GitDeleteFile()
		case "deleted":
			title, detail, action = locales.GitRestoreTitle(name), locales.GitRestoreDetail(), locales.GitRestoreFile()
		}
		iconButton(c, "discard", revertIcon, locales.GitDiscard(), pal, func() {
			a.discard(id, root, []proto.GitFile{f}, title, detail, action)
		})
		iconButton(c, "stage", plusIcon, locales.GitStage(), pal, func() {
			a.moveFiles(pane, section, []proto.GitFile{f})
			a.runGit(id, root, proto.GitOpParams{Op: "stage", Paths: paths}, locales.GitStage(), nil)
		})
	}
}

// ignore adds a file, or a folder by its path ending in a slash, to its
// worktree's .gitignore: at once while git tracks nothing of it, else as
// the user says, who may take what it tracks out of the repository too,
// as git goes on tracking what .gitignore lists.
func (a *App) ignore(pane *proto.Pane, p string) {
	id, root := pane.ID, pane.Git.Root
	plain := strings.TrimSuffix(p, "/")
	run := func(untrack bool) {
		a.runGit(id, root, proto.GitOpParams{Op: "ignore", Paths: []string{p}, Untrack: untrack}, locales.GitIgnore(), nil)
	}
	a.runGit(id, root, proto.GitOpParams{Op: "tracked", Paths: []string{plain}}, locales.GitIgnore(), func(out string) {
		if out == "" {
			run(false)
			return
		}
		a.confirm = &confirmation{open: true, title: locales.GitIgnoreTrackedTitle(path.Base(plain)), detail: locales.GitIgnoreTrackedDetail(strings.Count(out, "\n") + 1),
			action: locales.GitIgnoreUntrack(), run: func() { run(true) },
			alt: locales.GitIgnoreOnly(), altRun: func() { run(false) }}
	})
}

// discard drops the changes of files not staged once the user says so,
// and as the agents in the worktree allow: tracked files go back to the
// index's, untracked ones go. action is the confirming button's label,
// which says what happens to the files.
func (a *App) discard(pane, root string, files []proto.GitFile, title, detail, action string) {
	var p proto.GitOpParams
	p.Op = "discard"
	for _, f := range files {
		switch f.Status {
		case "conflict":
		case "untracked":
			p.Untracked = append(p.Untracked, f.Path)
		default:
			p.Paths = append(p.Paths, f.Path)
		}
	}
	if len(p.Paths) == 0 && len(p.Untracked) == 0 {
		return
	}
	a.confirm = &confirmation{open: true, title: title, detail: detail, action: action, destroy: true, run: func() {
		a.guarded(pane, root, locales.GitDiscard(), func() { a.runGit(pane, root, p, locales.GitDiscard(), nil) })
	}}
}

// lineActions are, in the bar of a change of a file's diff, the actions
// of the lines selected by their numbers, while some of them are of that
// change: Stage or Unstage, Discard, and Clear, in place of the change's
// own, so that there is one place to stage from. False while none are.
func (a *App) lineActions(c *ui.Context, pal *palette, section string, sf *sideFile, h editor.DiffHunk) bool {
	if sf == nil || sf.d == nil || !sf.d.LineSelection || sf.writing {
		return false
	}
	d := sf.d
	sel := d.SelectedRanges()
	if len(sel) == 0 {
		return false
	}
	hunks := d.Hunks()
	chosen, n := chosenLines(hunks, sel)
	if n == 0 || !touches(chosen, h) {
		return false
	}
	if hunkButton(c, "clear-lines", closeIcon, locales.GitClearLines(), locales.GitClearLines(), pal) {
		d.SetSelectedRanges(nil)
	}
	if section == baseStaged {
		// The index's text, the new side, without the lines.
		if hunkButton(c, "unstage-lines", minusIcon, locales.GitUnstageLines(n), locales.GitUnstageLinesTip(), pal) {
			text := applyLines(d.Old, d.New, hunks, not(chosen), d.New.LineEnding())
			d.SetSelectedRanges(nil)
			a.writeIndex(sf, text)
		}
		return true
	}
	if hunkButton(c, "discard-lines", revertIcon, locales.GitDiscardLines(n), locales.GitDiscardLinesTip(), pal) {
		a.discardLines(sf, hunks, chosen)
	}
	// The index's text, the old side, with the lines.
	if hunkButton(c, "stage-lines", plusIcon, locales.GitStageLines(n), locales.GitStageLinesTip(), pal) {
		text := applyLines(d.Old, d.New, hunks, withEnding(d.New, chosen), indexEnding(sf))
		d.SetSelectedRanges(nil)
		a.writeIndex(sf, text)
	}
	return true
}

// discardLines puts the old lines of the lines selected back in the file:
// as the user says while an agent works there, so long as the file is
// still the one they chose them in. Its text, refreshed meanwhile, can
// differ in the same lines, as the hunks alone wouldn't tell.
func (a *App) discardLines(sf *sideFile, hunks []editor.DiffHunk, chosen func(editor.DiffSide, int) bool) {
	g := a.paneGit(sf.pane)
	if g == nil {
		return
	}
	d, hash := sf.d, sf.hash
	anyway := len(a.agentsAt(sf.pane, g.Root)) > 0
	a.guardedNow(sf.pane, g.Root, locales.GitDiscard(), func() {
		if sf.d != d || sf.hash != hash || !slices.Equal(d.Hunks(), hunks) {
			a.toast(locales.GitChangeMoved())
			return
		}
		text := applyLines(d.Old, d.New, hunks, not(chosen), d.New.LineEnding())
		d.SetSelectedRanges(nil)
		a.writeBack(sf, text, anyway)
	})
}

// indexEnding is the line ending of the text a file's index is to have,
// as a staged change makes it: the index's own, but for a file the index
// hasn't yet, whose text goes as the working tree has it, for git to
// clean as it adds it (SetIndex).
func indexEnding(sf *sideFile) string {
	if sf.f.Status == "untracked" {
		return sf.d.New.LineEnding()
	}
	return sf.d.Old.LineEnding()
}

// touches tells whether a change has lines chosen.
func touches(chosen func(editor.DiffSide, int) bool, h editor.DiffHunk) bool {
	for l := h.OldStart; l < h.OldEnd; l++ {
		if chosen(editor.DiffOld, l) {
			return true
		}
	}
	for l := h.NewStart; l < h.NewEnd; l++ {
		if chosen(editor.DiffNew, l) {
			return true
		}
	}
	return false
}

// chosenLines tells which changed lines of a diff the lines selected
// choose, and how many of them are: each line selected alone, as git add
// -p stages them, a removed line's removal and an added line's addition.
// A line edited stages as an edit with both its lines selected, as a
// drag over them selects them in a unified view.
func chosenLines(hunks []editor.DiffHunk, sel []editor.DiffLineRange) (func(editor.DiffSide, int) bool, int) {
	in := func(side editor.DiffSide, l int) bool {
		return slices.ContainsFunc(sel, func(r editor.DiffLineRange) bool { return r.Side == side && l >= r.Start && l <= r.End })
	}
	n := 0
	for _, h := range hunks {
		for l := h.OldStart; l < h.OldEnd; l++ {
			if in(editor.DiffOld, l) {
				n++
			}
		}
		for l := h.NewStart; l < h.NewEnd; l++ {
			if in(editor.DiffNew, l) {
				n++
			}
		}
	}
	return in, n
}

// applyLines is the old text with the changes take chooses made: of its
// lines, those removed it takes go, and of the new ones, those added it
// takes come, in their change's place, each after the old line at its
// place in the change, which it would replace; with ending's line endings.
// A text that ends in a line ending has a last line that is empty, which
// the diff compares as any other: taking it or not keeps the ending or
// drops it. An empty text has no line, though its buffer holds an empty
// one, which would put an ending after the lines taken.
func applyLines(old, new *editor.Buffer, hunks []editor.DiffHunk, take func(editor.DiffSide, int) bool, ending string) string {
	var out []string
	pos := 0
	n := old.LineCount()
	if n == 1 && old.Line(0) == "" {
		n = 0
	}
	for _, h := range hunks {
		for l := pos; l < h.OldStart; l++ {
			out = append(out, old.Line(l))
		}
		for i := 0; i < max(h.OldEnd-h.OldStart, h.NewEnd-h.NewStart); i++ {
			if l := h.OldStart + i; l < h.OldEnd && !take(editor.DiffOld, l) {
				out = append(out, old.Line(l))
			}
			if l := h.NewStart + i; l < h.NewEnd && take(editor.DiffNew, l) {
				out = append(out, new.Line(l))
			}
		}
		pos = h.OldEnd
	}
	for l := pos; l < n; l++ {
		out = append(out, old.Line(l))
	}
	return strings.Join(out, ending)
}

// inHunk takes the lines of a change alone, both its old and its new: as
// staging it does, or, negated, discarding or unstaging it.
func inHunk(h editor.DiffHunk) func(editor.DiffSide, int) bool {
	return func(side editor.DiffSide, l int) bool {
		if side == editor.DiffOld {
			return l >= h.OldStart && l < h.OldEnd
		}
		return l >= h.NewStart && l < h.NewEnd
	}
}

// withEnding takes, with the last line of a text, the empty line after it
// that its line ending is: so a new file's line staged alone keeps its
// ending, as git add -p stages it. The empty line selected alone still
// unstages or discards the ending alone.
func withEnding(new *editor.Buffer, take func(editor.DiffSide, int) bool) func(editor.DiffSide, int) bool {
	last := new.LineCount() - 1
	if last < 1 || new.Line(last) != "" {
		return take
	}
	return func(side editor.DiffSide, l int) bool {
		return take(side, l) || side == editor.DiffNew && l == last && take(side, last-1)
	}
}

// not negates what lines take.
func not(take func(editor.DiffSide, int) bool) func(editor.DiffSide, int) bool {
	return func(side editor.DiffSide, l int) bool { return !take(side, l) }
}
