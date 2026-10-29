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
func (a *App) editorOf(p *proto.Pane) *openFile {
	if a.edits == nil {
		a.edits = map[string]*openFile{}
	}
	f := a.edits[p.ID]
	if f == nil || f.path != p.File {
		// Of no stamp a session has: read as it shows. Its file may be one
		// to make, as kero split --file names it.
		f = &openFile{path: p.File, stamp: "\x00", fresh: true}
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
	if a.hostOf(p.ID).cl != nil && f.stale(stamp, working) {
		a.readFile(p.ID, stamp, f)
	}
	if working {
		c.After(5 * time.Second)
	}
	ref := a.paneRef(p.ID)
	if pm := a.mergeOf(s, p, f); pm != nil && pm.m != nil && !pm.text {
		a.mergeBody(c, pal, p.ID, f, pm, ref)
		a.mergeBar(c, pal, s, p, f, pm)
	} else {
		a.fileBody(c, pal, p.ID, f, ref)
		a.editorBar(c, pal, s, p, f, pm)
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

// editorBar is an editor pane's bar below its file, as the Files view's
// tree has one: what the file is, then its actions; and while it is a new
// file, not yet saved or typed in, of this machine, one that opens a file
// of it there instead, which the system's dialog chooses. pm, when not nil,
// is the merge editor of a file in conflict shown as text.
func (a *App) editorBar(c *ui.Context, pal *palette, s *proto.Session, p *proto.Pane, f *openFile, pm *paneMerge) {
	bottomBar(c, "bar", pal).Padding(0, 6, 0, 12).Children(func() {
		a.fileStatus(c, pal, f)
		ui.Spacer(c)
		if pm != nil && pm.m != nil {
			a.mergeTextBar(c, pal, pm)
		}
		if f.absent && f.st != nil && !f.modified() && !a.hostOf(p.ID).remote() {
			sid, pid := s.ID, p.ID
			iconButton(c, "open-file", folderIcon, locales.FilesOpenFile(), pal, func() {
				a.chooseFile(func(path string) { a.replaceFile(sid, pid, path) })
			})
		}
		a.fileActions(c, pal, p.ID, f)
	})
}

// editorUnsaved returns the file of an editor pane while it has edits not
// saved, else nil.
func (a *App) editorUnsaved(pane string) *openFile {
	if f := a.edits[pane]; f != nil && f.modified() {
		return f
	}
	return nil
}
