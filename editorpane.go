package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
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
		// Of no stamp a session has: read as it shows.
		f = &openFile{path: p.File, stamp: "\x00"}
		a.edits[p.ID] = f
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
	a.fileBody(c, pal, p.ID, f, ref)
	if ref.Focused(c) && a.focusOf(s) != p.ID {
		a.setFocus(s, p.ID)
	}
}

// editorChips are an editor pane's header's buttons on its file.
func (a *App) editorChips(c *ui.Context, pal *palette, p *proto.Pane) {
	a.fileButtons(c, pal, p.ID, a.editorOf(p))
}

// editorUnsaved returns the file of an editor pane while it has edits not
// saved, else nil.
func (a *App) editorUnsaved(pane string) *openFile {
	if f := a.edits[pane]; f != nil && f.modified() {
		return f
	}
	return nil
}
