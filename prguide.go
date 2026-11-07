package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/proto"

	"kero/locales"
)

// A pull request's review guide, in its page's Guide tab: a tour of it an
// agent writes with kero pr guide, which kerod keeps apart from the draft
// review. Its summary, then its stops in reading order, each on lines of
// the diff with a note: the page shows one stop at a time, an excerpt of
// its lines, or the file's whole diff, where the user comments on them in
// the draft review as on the Files tab. Nothing of it goes to GitHub.

// prGuideTab is the page's Guide tab, beside prConversation and prFiles.
const prGuideTab = "guide"

// DefaultGuidePrompt is what the agent that writes a guide is asked when
// the pr-guide-prompt setting has no prompt.
const DefaultGuidePrompt = "Write a review guide of pull request #{number}, \"{title}\" ({url}), of {branch} into {base}: a tour that lets a maintainer understand it without reading it line by line.\n\n" +
	"First read it in full: gh pr view {number}, gh pr diff {number}, and the code around what it changes.\n\n" +
	"Then write the tour with kero pr guide (see kero's skill). Start with a short summary of what the change does and why, with kero pr guide summary. " +
	"Then add stops in reading order with kero pr guide stop, each on the lines worth reading, with a title and a note that explains them and what to check: " +
	"start where the change starts, its entry point, new types or core logic, and follow it outward. Mark with --closely a stop that needs a call only a maintainer can make. " +
	"Group mechanical changes, such as renames, generated files, snapshots and formatting, into stops at the end. Every file the pull request changes should be on some stop.\n\n" +
	"This is a guide, not a review: don't write review comments, and don't post anything to GitHub."

// prGuideState is a page's guide, as kerod last had it.
type prGuideState struct {
	guide   *proto.PRGuide // nil for none
	fetched bool
	err     string
	at      int          // the stop shown, of guide.Stops
	full    map[int]bool // the stops that show their file's whole diff, by id
	reveal  bool         // the stop shown comes into view
	view    ui.Rect      // the tab's scroll, as last laid out
}

// guidePrompt is what the agent writing a guide is asked: the setting's,
// else Kero's own.
func (a *App) guidePrompt() string {
	if strings.TrimSpace(a.set.GuidePrompt) != "" {
		return a.set.GuidePrompt
	}
	return DefaultGuidePrompt
}

// fetchGuide asks for the page's guide.
func (a *App) fetchGuide(g *prPage) { a.callGuide(g, proto.PRGuideParams{Op: "get"}) }

// callGuide calls pr.guide with p, of the page's pull request, then shows
// the guide that comes of it, else why none did.
func (a *App) callGuide(g *prPage, p proto.PRGuideParams) {
	cl := a.hostOf(g.pane).cl
	if cl == nil {
		return
	}
	p.Pane, p.Remote, p.Number = g.pane, g.remote, g.pr.Number
	go func() {
		var gd *proto.PRGuide
		err := cl.Call("pr.guide", p, &gd)
		a.update(func() {
			st := &g.guide
			st.fetched = true
			if err != nil {
				st.err = err.Error()
				return
			}
			st.err, st.guide = "", gd
			if gd != nil {
				st.at = min(st.at, max(len(gd.Stops)-1, 0))
			}
		})
	}()
}

// askGuide opens the dialog that has an agent write the page's guide, in
// a worktree of the pull request, as a review's does, with the guide's
// prompt.
func (a *App) askGuide(s *proto.Session, pane *proto.Pane, g *prPage) {
	a.askReview(s, pane, g.remote, g.slug, g.pr, reviewWorktree)
	w := a.prReview
	tpl := a.guidePrompt()
	w.guide, w.template = true, tpl
	w.asksTicket = strings.Contains(tpl, "{ticket}") && w.ticket == ""
	w.prompt = fillReview(tpl, g.pr, w.ticket)
}

// guideAgent is the session of the agent that writes the guide, and its
// pane; nil once it closed.
func (a *App) guideAgent(gd *proto.PRGuide) (*proto.Session, *proto.Pane) {
	if gd == nil || gd.Agent == "" && gd.Session == "" {
		return nil, nil
	}
	ref := proto.PRReviewDraft{Agent: gd.Agent, Session: gd.Session}
	return a.draftSession(ref), a.draftAgent(ref)
}

// prGuidePane is the Guide tab: the guide, else how to have one written.
func (a *App) prGuidePane(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, g *prPage, width float32) {
	st := &g.guide
	if st.full == nil {
		st.full = map[int]bool{}
	}
	gd := st.guide
	if gd == nil && !st.fetched {
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	}
	sc := ui.Scroll(c.Key("guide")).Grow(1).MinHeight(0)
	st.view = sc.Bounds()
	sc.Children(func() {
		ui.Column(c).Padding(4, 12, 20, 12).Gap(12).AlignItems(ui.Stretch).Children(func() {
			if gd == nil {
				a.guideEmpty(c, pal, s, pane, g)
				return
			}
			a.guideHead(c, pal, s, pane, g)
			if gd.Summary != "" {
				a.prMarkdown(c, g, "guide-summary", gd.Summary)
			}
			if len(gd.Stops) == 0 {
				say := locales.PrsGuideHint(g.pr.Number)
				if ss, _ := a.guideAgent(gd); ss != nil {
					say = locales.PrsGuideWriting()
				}
				ui.Text(c, say).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.4).Selectable()
				return
			}
			a.guideOutline(c, pal, g)
			a.guideStop(c, pal, s, g, width-24)
		})
	})
}

// guideEmpty is the Guide tab of a pull request with no guide: what one is,
// and Write Guide.
func (a *App) guideEmpty(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, g *prPage) {
	ui.Column(c.Key("guide-empty")).Padding(28, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, sparklesIcon).Size(22, 22).TextColor(pal.accent)
		ui.Text(c, locales.PrsGuideEmpty()).FontSize(14).FontWeight(650).TextColor(pal.text)
		ui.Text(c, locales.PrsGuideEmptyDetail()).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.4).TextAlign(ui.Center).MaxWidth(420)
		if sizedButton(c, "write-guide", locales.PrsWriteGuide(), "", true, 28, 8, pal) {
			a.askGuide(s, pane, g)
		}
		ui.Text(c, locales.PrsGuideHint(g.pr.Number)).FontSize(11.5).TextColor(pal.textFaint).TextAlign(ui.Center).Selectable()
		if g.guide.err != "" {
			_, removed, _ := a.diffColors(c, pal)
			ui.Text(c, g.guide.err).FontSize(12).TextColor(removed).LineHeight(1.35).Selectable()
		}
	})
}

// guideHead is the guide's title row, with its agent while it writes,
// and its menu: Rewrite Guide, Discard Guide; then whether it is of an
// older head than the pull request's.
func (a *App) guideHead(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, g *prPage) {
	st := &g.guide
	gd := st.guide
	ui.Row(c.Key("guide-head")).Height(24).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, sparklesIcon).Size(12, 12).TextColor(pal.accent).Shrink(0)
		ui.Text(c, locales.PrsGuideStops(len(gd.Stops))).FontSize(12.5).FontWeight(600).TextColor(pal.text).SingleLine().MinWidth(0)
		ui.Spacer(c)
		if ss, _ := a.guideAgent(gd); ss != nil {
			a.agentButton(c, pal, ss)
		}
		b := ui.ButtonBase(c.Key("guide-menu")).Size(24, 24).Radius(6).Center().TextColor(pal.textMuted).Label(locales.PrsGuideOptions()).Tooltip(locales.PrsGuideOptions())
		if b.Hovered() {
			b.Background(pal.rowHover)
		}
		b.Children(func() { ui.Icon(c, moreIcon).Size(14, 14) })
		b.Menu(func(m *ui.Menu) {
			if m.Item(locales.PrsRewriteGuide()).Chosen() {
				a.askGuide(s, pane, g)
			}
			if m.Item(locales.PrsGuideDiscard()).Chosen() {
				a.confirm = &confirmation{open: true, title: locales.PrsGuideDiscardTitle(), detail: locales.PrsGuideDiscardDetail(), action: locales.PrsDiscard(), destroy: true,
					run: func() { a.callGuide(g, proto.PRGuideParams{Op: "discard"}) }}
			}
		})
	})
	if gd.Head != "" && g.res.HeadOID != "" && gd.Head != g.res.HeadOID {
		ui.Row(c.Key("guide-stale")).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.PrsGuideStale()).FontSize(12).TextColor(pal.attention).LineHeight(1.35).Grow(1).MinWidth(0)
			if sizedButton(c, "rewrite", locales.PrsRewriteGuide(), "", false, 24, 6, pal) {
				a.askGuide(s, pane, g)
			}
		})
	}
	if st.err != "" {
		_, removed, _ := a.diffColors(c, pal)
		ui.Text(c, st.err).FontSize(12).TextColor(removed).LineHeight(1.35).Selectable()
	}
}

// guideOutline lists the stops, the one shown marked, which a click shows.
func (a *App) guideOutline(c *ui.Context, pal *palette, g *prPage) {
	st := &g.guide
	ui.Column(c.Key("guide-outline")).Gap(1).AlignItems(ui.Stretch).Children(func() {
		for i, sp := range st.guide.Stops {
			row := ui.Row(c.Key(sp.ID)).Height(26).Padding(0, 6).Gap(8).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip(guidePlace(sp))
			switch {
			case i == st.at:
				row.Background(pal.accent.Alpha(0.12))
			case row.Hovered():
				row.Background(pal.rowHover)
			}
			if row.Clicked() {
				st.at, st.reveal = i, true
			}
			row.Children(func() {
				fg := pal.textFaint
				if i == st.at {
					fg = pal.accent
				}
				ui.Text(c, itoa(i+1)).FontSize(11.5).FontWeight(650).FontFeatures("tnum").TextColor(fg).Width(18).TextAlign(ui.End).Shrink(0)
				ui.Text(c, sp.Title).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				if sp.Closely {
					closelyBadge(c, pal)
				}
				ui.Text(c, guidePlace(sp)).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Grow(1).Shrink(1000).MinWidth(0).TextAlign(ui.End)
			})
		}
	})
}

// closelyBadge marks a stop with a call only a maintainer can make.
func closelyBadge(c *ui.Context, pal *palette) {
	ui.Row(c.Key("closely")).Height(18).Padding(0, 6).Gap(4).Radius(9).AlignItems(ui.Center).Shrink(0).Background(pal.attention.Alpha(0.14)).Children(func() {
		ui.Icon(c, warningIcon).Size(10, 10).TextColor(pal.attention)
		ui.Text(c, locales.PrsGuideClosely()).FontSize(11).FontWeight(600).TextColor(pal.attention).SingleLine()
	})
}

// guidePlace is where a stop is, as a draft comment's place says it.
func guidePlace(sp proto.PRGuideStop) string {
	if sp.Line == 0 {
		return locales.ReviewOnFile(sp.Path)
	}
	side := editor.DiffNew
	if sp.Side == "LEFT" {
		side = editor.DiffOld
	}
	start := sp.Start
	if start == 0 {
		start = sp.Line
	}
	return commentPlace(&reviewComment{path: sp.Path, side: side, start: start - 1, end: sp.Line - 1})
}

// guideStop is the stop shown: where it is, Previous Stop and Next Stop,
// its title and note, then its lines, as an excerpt or the file's whole
// diff; Comment, Ask Agent, and the toggle between them.
func (a *App) guideStop(c *ui.Context, pal *palette, s *proto.Session, g *prPage, width float32) {
	st := &g.guide
	stops := st.guide.Stops
	st.at = min(max(st.at, 0), len(stops)-1)
	i := st.at
	sp := stops[i]
	step := func(by int) {
		st.at, st.reveal = min(max(i+by, 0), len(stops)-1), true
	}
	card := ui.Column(c.Key("guide-stop")).Padding(10, 12, 12, 12).Gap(8).Radius(10).AlignItems(ui.Stretch).
		InsetShadow(0, 0, 0, 1, pal.text.Alpha(0.1))
	card.Children(func() {
		if st.reveal {
			st.reveal = false
			ui.Box(c.Key("reveal")).Absolute().Top(-8).Left(0).Width(1).Height(max(st.view.H-16, 40)).ScrollIntoView()
		}
		ui.Row(c).Height(24).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.PrsGuideStopOf(i+1, len(stops))).FontSize(11.5).FontWeight(600).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine()
			if sp.Closely {
				closelyBadge(c, pal)
			}
			ui.Spacer(c)
			if i > 0 {
				iconButton(c, "previous", chevronLeftIcon, locales.PrsGuidePrevious(), pal, func() { step(-1) })
			}
			if i < len(stops)-1 {
				iconButton(c, "next", chevronIcon, locales.PrsGuideNext(), pal, func() { step(1) })
			}
		})
		ui.Text(c, sp.Title).FontSize(15).FontWeight(650).TextColor(pal.text).LineHeight(1.3).Selectable()
		ui.Text(c, guidePlace(sp)).FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…")
		if sp.Note != "" {
			a.prMarkdown(c, g, "guide-note-"+itoa(sp.ID), sp.Note)
		}
		a.guideLines(c, pal, s, g, sp, width-24)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(2, 0, 0, 0).Children(func() {
			if sizedButton(c, "comment", locales.PrsGuideComment(), "", false, 26, 6, pal) {
				a.commentOnStop(g, sp)
			}
			if _, p := a.guideAgent(st.guide); p != nil && sizedButton(c, "ask", locales.PrsGuideAsk(), "", false, 26, 6, pal) {
				a.askAboutStop(g, i)
			}
			ui.Spacer(c)
			label := locales.PrsGuideFullDiff()
			if st.full[sp.ID] {
				label = locales.PrsGuideExcerpt()
			}
			if sizedButton(c, "full", label, "", false, 26, 6, pal) {
				st.full[sp.ID] = !st.full[sp.ID]
			}
			if i < len(stops)-1 && sizedButton(c, "next-stop", locales.PrsGuideNext(), "", true, 26, 6, pal) {
				step(1)
			}
		})
		if i == len(stops)-1 {
			ui.Text(c, locales.PrsGuideEnd()).FontSize(12).TextColor(pal.textFaint).LineHeight(1.35)
		}
	})
}

// guideFile returns the page's file of a stop, its text asked for as it
// is first shown; nil for a file the pull request no longer changes.
func (a *App) guideFile(g *prPage, path string) *sideFile {
	v := g.res
	for _, fl := range v.Files {
		if fl.Path != path {
			continue
		}
		sf := g.files[path]
		if sf == nil {
			sf = &sideFile{pane: g.pane, base: baseRev, f: fl, pr: g}
			g.files[path] = sf
		}
		if !sf.busy && sf.d == nil && !sf.text.Binary && !sf.text.Large && sf.err == "" {
			a.fetchFile(sf, "")
		}
		return sf
	}
	return nil
}

// guideLines are a stop's lines: an excerpt of the diff, else the file's
// whole diff, with the comments on it, of the draft review too.
func (a *App) guideLines(c *ui.Context, pal *palette, s *proto.Session, g *prPage, sp proto.PRGuideStop, width float32) {
	sf := a.guideFile(g, sp.Path)
	switch {
	case sf == nil:
		a.sectionNote(c, pal, locales.PrsGuideNoFile())
		return
	case g.guide.full[sp.ID]:
		ui.Column(c.Key("full-" + itoa(sp.ID))).AlignItems(ui.Stretch).Children(func() {
			a.prFileNotes(c, pal, g, sp.Path)
			a.fileDiff(c, pal, s, sf, width)
		})
		return
	case sf.err != "" && sf.d == nil:
		a.sectionNote(c, pal, locales.ChangesCantRead()+": "+sf.err)
		return
	case sf.text.Binary:
		a.sectionNote(c, pal, locales.ChangesBinary())
		return
	case sf.patch || sf.text.Large && sf.d == nil:
		a.sectionNote(c, pal, locales.ChangesTooLarge())
		return
	case sf.d == nil:
		ui.Column(c.Key("loading")).Height(48).Center().Children(func() { ui.Spinner(c) })
		return
	}
	rows := stopExcerpt(sf.d, sp)
	more := 0
	if len(rows) > guideExcerptMax {
		more, rows = len(rows)-guideExcerptMax, rows[:guideExcerptMax]
	}
	added, removed, _ := a.diffColors(c, pal)
	fam := a.termFamily()
	ui.Column(c.Key("excerpt-"+itoa(sp.ID))).Padding(6, 0).Radius(8).Background(pal.chip).AlignItems(ui.Stretch).Children(func() {
		for j, r := range rows {
			row := ui.Row(c.Key(j)).Padding(1, 10, 1, 0).Gap(8).AlignItems(ui.Start)
			sign, fg := " ", pal.textFaint
			switch r.kind {
			case '+':
				row.Background(added.Alpha(0.14))
				sign, fg = "+", added
			case '-':
				row.Background(removed.Alpha(0.14))
				sign, fg = "-", removed
			}
			row.Children(func() {
				if r.kind == '~' {
					ui.Text(c, "⋯").FontSize(12).TextColor(pal.textFaint).Padding(0, 0, 0, 58)
					return
				}
				num := ""
				if r.line > 0 {
					num = itoa(r.line)
				}
				ui.Text(c, num).Font(fam).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).Width(40).TextAlign(ui.End).Shrink(0)
				ui.Text(c, sign).Font(fam).FontSize(12).TextColor(fg).Width(10).Shrink(0)
				text := strings.ReplaceAll(r.text, "\t", "    ")
				if text == "" {
					text = " "
				}
				ui.Text(c, text).Font(fam).FontSize(12).TextColor(pal.text).LineHeight(1.45).Grow(1).MinWidth(0).Selectable()
			})
		}
		if more > 0 {
			ui.Text(c, locales.PrsGuideMoreLines(more)).FontSize(11.5).TextColor(pal.textFaint).Padding(4, 10, 0, 58)
		}
	})
}

// guideExcerptMax is how many lines an excerpt shows at most, the rest
// in the file's whole diff.
const guideExcerptMax = 80

// excerptRow is a line of a stop's excerpt: kind '+' added, '-' removed,
// ' ' unchanged, '~' a gap between changes; line its number, from 1, of
// the side it is of.
type excerptRow struct {
	kind byte
	line int
	text string
}

// stopExcerpt is a stop's lines of a diff, as a unified diff shows them:
// of the new side, each change's removed lines before its added ones; of
// the old side (LEFT), its lines alone; of the whole file, every change,
// gaps between them.
func stopExcerpt(d *editor.DiffState, sp proto.PRGuideStop) []excerptRow {
	hunks := d.Hunks()
	var rows []excerptRow
	removed := func(l int) excerptRow { return excerptRow{kind: '-', line: l + 1, text: d.Old.Line(l)} }
	if sp.Line == 0 {
		for i, h := range hunks {
			if i > 0 {
				rows = append(rows, excerptRow{kind: '~'})
			}
			for l := h.OldStart; l < h.OldEnd; l++ {
				rows = append(rows, removed(l))
			}
			for l := h.NewStart; l < h.NewEnd; l++ {
				rows = append(rows, excerptRow{kind: '+', line: l + 1, text: d.New.Line(l)})
			}
		}
		return rows
	}
	start := sp.Start
	if start == 0 {
		start = sp.Line
	}
	start, end := start-1, sp.Line-1
	if sp.Side == "LEFT" {
		for l := start; l <= min(end, d.Old.LineCount()-1); l++ {
			r := excerptRow{kind: ' ', line: l + 1, text: d.Old.Line(l)}
			for _, h := range hunks {
				if h.OldStart <= l && l < h.OldEnd {
					r.kind = '-'
				}
			}
			rows = append(rows, r)
		}
		return rows
	}
	for l := start; l <= min(end, d.New.LineCount()-1); l++ {
		r := excerptRow{kind: ' ', line: l + 1, text: d.New.Line(l)}
		for _, h := range hunks {
			if h.NewStart == l {
				for o := h.OldStart; o < h.OldEnd; o++ {
					rows = append(rows, removed(o))
				}
			}
			if h.NewStart <= l && l < h.NewEnd {
				r.kind = '+'
			}
		}
		rows = append(rows, r)
	}
	return rows
}

// commentOnStop opens a draft comment of the review on a stop's lines, or
// its file, in the file's whole diff, which it shows.
func (a *App) commentOnStop(g *prPage, sp proto.PRGuideStop) {
	st := &g.guide
	if sp.Line == 0 {
		st.full[sp.ID] = true
		a.startFileComment(g, sp.Path)
		return
	}
	sf := g.files[sp.Path]
	if sf == nil || sf.d == nil || sf.patch {
		return
	}
	side := editor.DiffNew
	if sp.Side == "LEFT" {
		side = editor.DiffOld
	}
	start := sp.Start
	if start == 0 {
		start = sp.Line
	}
	st.full[sp.ID] = true
	a.startComment(sf, side, start-1, sp.Line-1)
}

// askAboutStop types a question about a stop into the pane of the agent
// that wrote the guide, for the user to finish, and goes there.
func (a *App) askAboutStop(g *prPage, i int) {
	gd := g.guide.guide
	s, p := a.guideAgent(gd)
	if s == nil || p == nil || i >= len(gd.Stops) {
		a.toast(locales.ReviewNoAgent())
		return
	}
	cl := a.hostOf(p.ID).cl
	if cl == nil {
		return
	}
	text := locales.PrsGuideAskText(i+1, guidePlace(gd.Stops[i])) + " "
	agent := p.ID
	go cl.Call("send", proto.SendParams{ID: agent, Data: text, Paste: true}, nil)
	if a.focus == nil {
		a.focus = map[string]string{}
	}
	a.focus[s.ID] = agent
	a.selectSession(s.ID)
	a.focusTerminal()
}
