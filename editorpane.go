package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

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
	stamp, working := sessionStamp(s)
	if cl := a.hostOf(p.ID).cl; cl != nil && f.stale(stamp, working) {
		a.loadFile(cl, f, stamp)
	}
	if working {
		c.After(5 * time.Second)
	}
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

// editorUnsaved returns the file of an editor pane while it has edits not
// saved, else nil.
func (a *App) editorUnsaved(pane string) *fileView {
	if f := a.edits[pane]; f != nil && f.modified() {
		return f
	}
	return nil
}
