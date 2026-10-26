package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/actions"
	"kero/internal/git"
	"kero/internal/proto"

	"kero/locales"
)

// A review of what changed: comments on lines of the Changes view's diffs,
// written under the lines as a pull request's, which Send to Agent sends
// the worktree's agent as one prompt, each comment with its file, its
// lines and their text, for the agent to find them by, as they may have
// moved since.

// review is a worktree's comments, in the order they were written.
type review struct {
	comments []*reviewComment
	// draft is the comment being written, nil for none; it takes the place
	// of the comment it edits, when it edits one.
	draft *reviewComment
	edits *reviewComment
	last  int // the id given last
}

// reviewComment is a comment on lines of a diff's side.
type reviewComment struct {
	// id tells it among the diff's annotations, which keep their elements
	// by it: by a value that stays as its text changes, which would else
	// make its field anew, without the focus.
	id   int
	path string // from the worktree's root, with slashes
	// diff is the diff it is on, which shows it: of the base, and the
	// section of Uncommitted's, the side's lines are of.
	diff       string
	side       editor.DiffSide
	start, end int    // the lines, from 0, end too
	code       string // their text, as they were
	text       string
	focus      bool // the draft's field takes the keyboard, as it shows
}

// reviewKey is what a.review keeps a worktree's review by.
func reviewKey(h *host, root string) string { return gotoKey(h, root) }

// reviewOf returns a worktree's review, made as it is first asked for when
// make; nil for none.
func (a *App) reviewOf(h *host, root string, make bool) *review {
	rv := a.review[reviewKey(h, root)]
	if rv == nil && make {
		if a.review == nil {
			a.review = map[string]*review{}
		}
		rv = &review{}
		a.review[reviewKey(h, root)] = rv
	}
	return rv
}

// fileRoot is the worktree a changed file's diff is of: its pane's, or
// for a turn, the turn's.
func (a *App) fileRoot(sf *sideFile) string {
	s := a.sessionOfPane(sf.pane)
	if s == nil {
		return ""
	}
	p := s.PaneByID(sf.pane)
	switch {
	case p == nil:
		return ""
	case sf.base == git.BaseTurn && p.Turn != nil:
		return p.Turn.Root
	case p.Git != nil:
		return p.Git.Root
	}
	return ""
}

// reviewable tells a diff that takes comments: one of what changes, not
// of a commit, nor a patch, whose lines are not the file's.
func reviewable(sf *sideFile) bool { return sf.base != baseRev && !sf.patch }

// diffID is what tells a file's diff among a review's: its base, and the
// file.
func diffID(sf *sideFile) string { return sf.base + "|" + sf.f.Path }

// startComment opens a draft on lines of a diff's side, from start to end,
// from 0.
func (a *App) startComment(sf *sideFile, side editor.DiffSide, start, end int) {
	root := a.fileRoot(sf)
	if root == "" || sf.d == nil {
		return
	}
	buf := sf.d.New
	if side == editor.DiffOld {
		buf = sf.d.Old
	}
	end = min(end, buf.LineCount()-1)
	start = min(max(start, 0), end)
	var code []string
	for l := start; l <= end; l++ {
		code = append(code, buf.Line(l))
	}
	rv := a.reviewOf(a.hostOf(sf.pane), root, true)
	rv.last++
	rv.draft = &reviewComment{id: rv.last, path: sf.f.Path, diff: diffID(sf), side: side, start: start, end: end, code: strings.Join(code, "\n"), focus: true}
	rv.edits = nil
}

// commentLines are the lines a comment from the gutter's button on line is
// on: those selected around it, else the line alone.
func commentLines(sel []editor.DiffLineRange, line editor.DiffLineRange) (start, end int) {
	start, end = line.Start, line.End
	for _, r := range sel {
		if r.Side == line.Side && r.Start <= line.Start && line.End <= r.End {
			return r.Start, r.End
		}
	}
	return start, end
}

// hunkLines are the lines a comment from a change's bar is on: the lines
// selected of it, those added before those removed, else its new lines,
// else, as for lines only removed, its old ones.
func hunkLines(sel []editor.DiffLineRange, h editor.DiffHunk) (side editor.DiffSide, start, end int, ok bool) {
	for _, s := range []editor.DiffSide{editor.DiffNew, editor.DiffOld} {
		lo, hi := h.NewStart, h.NewEnd
		if s == editor.DiffOld {
			lo, hi = h.OldStart, h.OldEnd
		}
		start, end = -1, -1
		for _, r := range sel {
			if r.Side != s {
				continue
			}
			from, to := max(r.Start, lo), min(r.End, hi-1)
			if from > to {
				continue
			}
			if start < 0 || from < start {
				start = from
			}
			end = max(end, to)
		}
		if start >= 0 {
			return s, start, end, true
		}
	}
	switch {
	case h.NewEnd > h.NewStart:
		return editor.DiffNew, h.NewStart, h.NewEnd - 1, true
	case h.OldEnd > h.OldStart:
		return editor.DiffOld, h.OldStart, h.OldEnd - 1, true
	}
	return editor.DiffNew, 0, 0, false
}

// Heights of a comment's parts under its lines, which the diff leaves room
// for before it builds them.
const (
	draftH        = 128
	commentChrome = 40 // a comment's room besides its text
	commentMargin = 12 // either side of a comment's card, within the diff
)

// reviewOptions sets what of a review a file's diff shows: its comments,
// and the draft, under their lines; the gutter's button, which starts a
// comment on the line it is on, or the lines selected around it.
func (a *App) reviewOptions(c *ui.Context, pal *palette, sf *sideFile, width float32) {
	d := sf.d
	d.Annotations, d.RenderAnnotation, d.OnGutterButton = nil, nil, nil
	if !reviewable(sf) {
		return
	}
	root := a.fileRoot(sf)
	if root == "" {
		return
	}
	d.OnGutterButton = func(line editor.DiffLineRange) {
		start, end := commentLines(d.SelectedRanges(), line)
		a.startComment(sf, line.Side, start, end)
	}
	rv := a.reviewOf(a.hostOf(sf.pane), root, false)
	if rv == nil {
		return
	}
	id := diffID(sf)
	// The text wraps in the card, which a split view's side halves.
	textW := width - 2*commentMargin - 2*12 - 60
	if d.Layout == editor.DiffSplit {
		textW = width/2 - 2*commentMargin - 2*12 - 40
	}
	textW = max(textW, 120)
	for _, cm := range rv.comments {
		if cm.diff != id || cm == rv.edits {
			continue
		}
		_, h := c.MeasureText(textW, ui.Span{Text: cm.text, Size: 13})
		d.Annotations = append(d.Annotations, editor.DiffAnnotation{Side: cm.side, Line: cm.end, Height: float32(commentChrome) + h, Key: cm.id})
	}
	if dr := rv.draft; dr != nil && dr.diff == id {
		d.Annotations = append(d.Annotations, editor.DiffAnnotation{Side: dr.side, Line: dr.end, Height: draftH, Key: dr.id})
	}
	d.RenderAnnotation = func(c *ui.Context, an editor.DiffAnnotation) {
		if dr := rv.draft; dr != nil && an.Key == dr.id {
			a.commentDraft(c, pal, rv)
			return
		}
		for _, cm := range rv.comments {
			if an.Key == cm.id {
				a.commentCard(c, pal, rv, cm)
			}
		}
	}
}

// commentCard is a comment under its lines: its text, and Edit and Delete.
func (a *App) commentCard(c *ui.Context, pal *palette, rv *review, cm *reviewComment) {
	ui.Column(c.Key("comment")).Grow(1).Margin(4, commentMargin, 6, commentMargin).Padding(6, 6, 8, 12).Gap(2).Radius(8).
		Background(pal.accent.Alpha(0.08)).InsetShadow(0, 0, 0, 1, pal.accent.Alpha(0.25)).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Height(22).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, commentIcon).Size(12, 12).TextColor(pal.accent).Shrink(0)
			ui.Text(c, commentPlace(cm)).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0)
			ui.Spacer(c)
			iconButton(c, "edit", editIcon, locales.ReviewEdit(), pal, func() {
				d := *cm
				d.focus = true
				rv.draft, rv.edits = &d, cm
			})
			iconButton(c, "delete", closeIcon, locales.ReviewDelete(), pal, func() {
				rv.comments = slices.DeleteFunc(rv.comments, func(x *reviewComment) bool { return x == cm })
			})
		})
		ui.Text(c, cm.text).FontSize(13).TextColor(pal.text).LineHeight(1.35).Selectable()
	})
}

// commentDraft is the comment being written, under its lines: Comment
// keeps it, or Ctrl+Enter; Cancel or Escape drops it.
func (a *App) commentDraft(c *ui.Context, pal *palette, rv *review) {
	dr := rv.draft
	done := func(keep bool) {
		switch {
		case !keep || strings.TrimSpace(dr.text) == "":
		case rv.edits != nil:
			rv.edits.text = strings.TrimSpace(dr.text)
		default:
			dr.text = strings.TrimSpace(dr.text)
			rv.comments = append(rv.comments, dr)
		}
		rv.draft, rv.edits = nil, nil
	}
	ui.Column(c.Key("draft")).Grow(1).Margin(4, commentMargin, 6, commentMargin).Padding(8).Gap(8).Radius(8).
		Background(pal.card).InsetShadow(0, 0, 0, 1, pal.accent.Alpha(0.45)).AlignItems(ui.Stretch).Children(func() {
		frame := ui.Row(c.Key("frame")).Grow(1).MinHeight(0).Cursor(ui.CursorText)
		var in ui.Element
		frame.Children(func() {
			in = ui.TextAreaBase(c.Key("in"), &dr.text).Grow(1).MinWidth(0).AlignSelf(ui.Stretch).Padding(6, 8).FontSize(13).
				Placeholder(locales.ReviewPlaceholder(commentPlace(dr))).Label(locales.ReviewPlaceholder(commentPlace(dr)))
		})
		field(c, frame, in.Focused(), pal).Padding(0)
		// As the editor's own bars under it take the focus.
		if in.Focused() {
			a.side.drafting = rv
		}
		if dr.focus {
			dr.focus = false
			in.Focus()
		}
		// Escape is the panel's, which drops the draft (cancelDraft).
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEnter || k.Mods != ui.Cmd {
				return false
			}
			done(true)
			return true
		})
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Text(c, locales.ReviewDraftHint(actions.Glyphs("CmdOrCtrl+Enter"))).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if sizedButton(c, "cancel", locales.CommonCancel(), "", false, 26, 6, pal) {
				done(false)
			}
			label := locales.ReviewAdd()
			if rv.edits != nil {
				label = locales.ReviewSave()
			}
			if sizedButton(c, "add", label, "", true, 26, 6, pal) {
				done(true)
			}
		})
	})
}

// cancelDraft drops the draft that has the keyboard, as Escape in the
// panel does, and reports whether there was one.
func (a *App) cancelDraft() bool {
	rv := a.side.drafting
	if rv == nil || rv.draft == nil {
		return false
	}
	rv.draft, rv.edits = nil, nil
	a.paneRef("side-card").Focus()
	return true
}

// commentPlace is where a comment is: its file and lines, from 1, as
// "src/a.go:12–14", the old side's lines as removed.
func commentPlace(cm *reviewComment) string {
	lines := itoa(cm.start + 1)
	if cm.end > cm.start {
		lines += "–" + itoa(cm.end+1)
	}
	if cm.side == editor.DiffOld {
		return locales.ReviewRemovedLines(cm.path, lines)
	}
	return cm.path + ":" + lines
}

// reviewBar is the band atop the Changes view's files while the worktree
// has comments: how many, each with its place, which opens the file there,
// and Send to Agent, Copy as Prompt and Discard.
func (a *App) reviewBar(c *ui.Context, pal *palette, pane *proto.Pane, root string) {
	h := a.hostOf(pane.ID)
	rv := a.reviewOf(h, root, false)
	if rv == nil || len(rv.comments) == 0 {
		return
	}
	col := pal.accent
	band := ui.Column(c.Key("review")).Shrink(0).Margin(0, 0, 6, 0).AlignItems(ui.Stretch).Background(col.Alpha(0.08))
	band.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(ui.Rect{X: r.X, Y: r.Y, W: 3, H: r.H}, col, 0) })
	band.Children(func() {
		ui.Row(c).Height(34).Padding(0, 8, 0, 12).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, commentIcon).Size(13, 13).TextColor(col).Shrink(0)
			ui.Text(c, locales.ReviewTitle(len(rv.comments))).FontSize(12.5).FontWeight(650).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
			ui.Spacer(c)
			iconButton(c, "discard", closeIcon, locales.ReviewDiscard(), pal, func() {
				n := len(rv.comments)
				a.confirm = &confirmation{open: true, title: locales.ReviewDiscardTitle(n), detail: locales.ReviewDiscardDetail(), action: locales.ReviewDiscard(), destroy: true,
					run: func() { rv.comments, rv.draft, rv.edits = nil, nil, nil }}
			})
			iconButton(c, "copy", copyIcon, locales.ReviewCopy(), pal, func() {
				c.WriteClipboard(reviewPrompt(rv.comments))
				a.toast(locales.ReviewCopied())
			})
			agent := a.reviewAgent(pane)
			tip := locales.ReviewSendTip()
			if agent == nil {
				tip = locales.ReviewNoAgent()
			}
			// The primary button's look (sizedButton), with the icon of
			// what an agent does.
			b := ui.ButtonBase(c.Key("send")).Height(26).Padding(0, 10).Gap(6).Radius(6).AlignItems(ui.Center).
				Label(locales.ReviewSend()).Tooltip(tip).TextColor(ui.Hex("#ffffff"))
			bg := pal.accent
			switch {
			case agent == nil:
				b.Opacity(0.45)
			case b.Pressed():
				bg = bg.Mix(ui.Hex("#000000"), 0.08)
			case b.Hovered():
				bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
			}
			face(b, pal, b.AnimateColor("bg", bg, ui.Animation{Duration: 90e6}))
			b.Children(func() {
				ui.Icon(c, sparklesIcon).Size(13, 13)
				ui.Text(c, locales.ReviewSend()).FontSize(12.5).FontWeight(600).SingleLine()
			})
			if b.Clicked() && agent != nil {
				a.sendReview(pane, rv)
			}
		})
		ui.Column(c.Key("comments")).Padding(0, 8, 6, 12).AlignItems(ui.Stretch).Children(func() {
			for i, cm := range rv.comments {
				row := ui.Row(c.Key(i)).Height(24).Padding(0, 6).Gap(8).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip(cm.text)
				if row.Hovered() {
					row.Background(pal.rowHover)
				}
				if row.Clicked() {
					file, line := joinRel(root, cm.path), cm.start+1
					id := pane.ID
					a.defer_(func() { a.openAt(id, file, line) })
				}
				row.Children(func() {
					ui.Text(c, commentPlace(cm)).FontSize(12).FontFeatures("tnum").TextColor(pal.textMuted).SingleLine().Ellipsis("…").Shrink(0).MaxWidth(200)
					ui.Text(c, firstLine(cm.text)).FontSize(12).TextColor(pal.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				})
			}
		})
	})
}

// reviewAgent is the agent a worktree's review goes to: the one in the pane,
// else one working in its worktree (agentPane); nil for none.
func (a *App) reviewAgent(pane *proto.Pane) *proto.Pane {
	if pane.Agent == "" && pane.Git == nil {
		return nil
	}
	return a.agentPane(pane)
}

// firstLine is a text's first line.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// reviewPrompt is what a review asks its agent: each comment with its file,
// its lines, from 1, and their text, quoted.
func reviewPrompt(comments []*reviewComment) string {
	var b strings.Builder
	b.WriteString("Please address these review comments on the changes in this worktree. The line numbers are as the lines were when each comment was written; find each by its quoted code if they have moved.\n")
	for i, cm := range comments {
		lines := "line " + itoa(cm.start+1)
		if cm.end > cm.start {
			lines = fmt.Sprintf("lines %d-%d", cm.start+1, cm.end+1)
		}
		if cm.side == editor.DiffOld {
			lines = "removed " + lines + " of the old version"
		}
		fmt.Fprintf(&b, "\n%d. %s, %s:\n", i+1, cm.path, lines)
		for _, l := range strings.Split(cm.code, "\n") {
			b.WriteString("   > " + l + "\n")
		}
		for _, l := range strings.Split(cm.text, "\n") {
			b.WriteString("   " + l + "\n")
		}
	}
	return b.String()
}

// sendReview pastes a review's prompt in the worktree's agent's pane, then
// Enter, as kero prompt does, and lets the comments go once it is sent.
// It pastes rather than types it, whose newlines would send it line by
// line.
func (a *App) sendReview(pane *proto.Pane, rv *review) {
	ap := a.reviewAgent(pane)
	cl := a.hostOf(pane.ID).cl
	if ap == nil || cl == nil || len(rv.comments) == 0 {
		return
	}
	sent := slices.Clone(rv.comments)
	text := reviewPrompt(sent)
	id, name := ap.ID, proto.AgentNames[ap.Agent]
	if name == "" {
		name = locales.GitAnAgent()
	}
	go func() {
		err := cl.Call("send", proto.SendParams{ID: id, Data: text, Paste: true}, nil)
		if err == nil {
			// Apart from the paste, which the program takes in whole first.
			time.Sleep(120 * time.Millisecond)
			err = cl.Call("send", proto.SendParams{ID: id, Data: "\r"}, nil)
		}
		a.update(func() {
			if err != nil {
				a.toast(locales.ReviewSendFailed(err.Error()))
				return
			}
			// What was written since stays, for the next.
			rv.comments = slices.DeleteFunc(rv.comments, func(cm *reviewComment) bool { return slices.Contains(sent, cm) })
			a.toast(locales.ReviewSent(len(sent), name))
			if s := a.sessionOfPane(id); s != nil && s.ID == a.selected {
				a.setFocus(s, id)
				a.paneRef(id).Focus()
			}
		})
	}()
}
