package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/client"
	"kero/internal/proto"

	"kero/locales"
)

// An editor pane shows a file of its host in the editor the Files view
// opens files in: kerod keeps its place and its file, the app its text,
// which it reads through kerod as the pane first shows, and again as the
// session's worktrees change while it has no edits.

// editorOf returns an editor pane's file, to be read as it first shows.
func (a *App) editorOf(p *proto.Pane) *fileView {
	if a.edits == nil {
		a.edits = map[string]*fileView{}
	}
	f := a.edits[p.ID]
	if f == nil || f.path != p.File {
		// Its file may be one to make, as kero split --file names it.
		f = &fileView{pane: p.ID, path: p.File, rel: a.filesRel(p.ID, p.File), fresh: true}
		a.edits[p.ID] = f
		key := gotoKey(a.hostOf(p.ID), p.File)
		if pos, ok := a.gotos[key]; ok {
			f.goTo = &pos
			delete(a.gotos, key)
		}
	}
	return f
}

// sessionStamp is what of a session's worktrees the files its editor
// panes show follow, and whether an agent works in it.
func sessionStamp(s *proto.Session) (stamp string, working bool) {
	var b strings.Builder
	for i := range s.Panes {
		p := &s.Panes[i]
		b.WriteString(gitStamp(p.Git))
		b.WriteByte('|')
		working = working || p.Status == proto.Working
	}
	return b.String(), working
}

// paneEditor is an editor pane's body: its file in the editor, which
// takes the pane's keyboard.
func (a *App) paneEditor(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane) {
	f := a.editorOf(p)
	if p.Preview != f.told {
		f.told, f.preview = p.Preview, p.Preview
	}
	stamp, working := sessionStamp(s)
	switch cl := a.hostOf(p.ID).cl; {
	case cl == nil:
	case f.stale(stamp, working):
		a.loadFile(cl, f, stamp)
	case f.due():
		a.checkFile(cl, f)
	}
	c.After(fileCheck)
	ref := a.paneRef(p.ID)
	if pm := a.mergeOf(s, p, f); pm != nil && pm.m != nil && !pm.text {
		a.mergeBody(c, pal, f, pm, ref)
	} else {
		a.fileBody(c, pal, f, ref)
	}
	if a.focusOf(s) == p.ID || ref.Focused(c) {
		// The pane a file opened in a pane replaces the file of, as the
		// user chooses (openInPane).
		if a.lastEdit == nil {
			a.lastEdit = map[string]string{}
		}
		a.lastEdit[s.ID] = p.ID
	}
	// Focus within the pane, as on the preview's text, which a click
	// selects in, takes the pane: the card sees no press of it.
	if ref.FocusWithin(c) && a.focusOf(s) != p.ID {
		a.setFocus(s, p.ID)
	}
}

// editorTools are an editor pane's buttons in its header, as the viewer
// has them in its own: what the file is, then its actions; and while it is
// a new file, not yet saved or typed in, of this machine, one that opens a
// file of it there instead, which the system's dialog chooses. A file in
// conflict has the merge editor's (mergeTools), or shown as text, the one
// that shows the merge editor again.
func (a *App) editorTools(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane) {
	f := a.editorOf(p)
	pm := a.mergeOf(s, p, f)
	if pm != nil && pm.m != nil && !pm.text {
		// How many conflicts are left, in the room the buttons leave.
		ui.Text(c, locales.MergeConflictsLeft(pm.m.Unresolved())).FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).Margin(0, 6, 0, 0)
	}
	ui.Row(c.Key("file-tools")).Gap(0).AlignItems(ui.Center).Shrink(0).Bind(a.paneRef("tools-" + p.ID)).Children(func() {
		if pm != nil && pm.m != nil && !pm.text {
			a.mergeTools(c, pal, s, p, f, pm)
			return
		}
		a.fileStatus(c, pal, f)
		if pm != nil && pm.m != nil {
			a.mergeTextBar(c, pal, pm)
		}
		sid, pid := s.ID, p.ID
		choose := func() { a.chooseFile(func(path string) { a.replaceFile(sid, pid, path) }) }
		canChoose := f.absent && f.st != nil && !f.modified() && !a.hostOf(p.ID).remote()
		if f.toolsFold {
			var more func(m *ui.Menu)
			if canChoose {
				more = func(m *ui.Menu) {
					if m.Item(locales.FilesOpenFile()).Chosen() {
						choose()
					}
				}
			}
			a.fileActionsButton(c, pal, f, more)
			return
		}
		if canChoose {
			iconButton(c, "open-file", folderIcon, locales.FilesOpenFile(), pal, choose)
		}
		a.fileActions(c, pal, f)
	})
}

// fileCheck is how often an editor pane looks whether its file changed on
// disk: an agent rewriting the plan it shows the user, in a folder git
// ignores, changes no worktree's status, and works no more once it wrote.
const fileCheck = 2 * time.Second

// due tells that the file is to be checked against the disk (checkFile):
// a while since it last was, and never while it has edits, which a save
// checks against the disk instead.
func (fv *fileView) due() bool {
	return !fv.busy && !fv.edits.saving && !fv.checking && !fv.modified() && fv.st != nil && time.Since(fv.checked) >= fileCheck
}

// checkFile asks kerod about an editor pane's file, and reads it again
// when it changed on disk since it was read, or came to be.
func (a *App) checkFile(cl *client.Client, fv *fileView) {
	fv.checking, fv.checked = true, time.Now()
	path := fv.path
	go func() {
		var st proto.FSStat
		err := cl.Call("fs.stat", proto.FSStatParams{Path: path}, &st)
		a.update(func() {
			fv.checking = false
			if err == nil && fv.changedOnDisk(st) && !fv.busy && !fv.modified() && a.edits[fv.pane] == fv {
				a.loadFile(cl, fv, fv.stamp)
			}
		})
	}()
}

// changedOnDisk tells a file that is not as it was read: made since, or
// written. One gone stays as it shows, as a save tells.
func (fv *fileView) changedOnDisk(st proto.FSStat) bool {
	return st.Exists && !st.Dir && (fv.absent || st.Mod != fv.mod || st.Size != fv.size)
}

// previewFile shows a Markdown file as it reads, or its text again; for
// an editor pane, it tells kerod, which keeps it for kero do pane.preview
// and a restore. A kerod of before it keeps none: the pane previews still.
func (a *App) previewFile(fv *fileView, on bool) {
	fv.preview = on
	if a.edits[fv.pane] != fv {
		return
	}
	fv.told = on
	if s := a.sessionOfPane(fv.pane); s != nil {
		if p := s.PaneByID(fv.pane); p != nil {
			p.Preview = on
		}
	}
	a.call(fv.pane, "pane.preview", proto.PreviewParams{ID: fv.pane, Preview: on}, func(error) {})
}

// togglePreview previews the Markdown file of the editor pane the user is
// in, or shows its text again, as its header's button does.
func (a *App) togglePreview(s *proto.Session) {
	if s == nil {
		return
	}
	if p := s.PaneByID(a.focusOf(s)); p != nil && p.Editor() && markdownFile(p.File) {
		f := a.editorOf(p)
		a.previewFile(f, !f.preview)
	}
}

// editorUnsaved returns the file of an editor pane while it has edits not
// saved, else nil.
func (a *App) editorUnsaved(pane string) *fileView {
	if f := a.edits[pane]; f != nil && f.modified() {
		return f
	}
	return nil
}
