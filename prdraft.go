package main

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"kero/internal/github"
	"kero/internal/proto"

	"kero/locales"
)

// A pull request's draft review: the comments the agent that reviews it
// writes with kero pr, and the user's own, which kerod keeps and the
// page's diffs show under their lines, as the Changes view's review
// comments; with its summary, it goes to GitHub as one review when the
// user posts it, never before.

// prDraftState is a page's draft review, as kerod last had it.
type prDraftState struct {
	draft *proto.PRReviewDraft
	// summary is the summary as typed, kept as its field loses the focus.
	summary     string
	summaryEdit bool
	event       string // a github.Event*
	posting     bool
	err         string
}

// newPageReview is the review a page's diffs show the draft's comments
// in, each change of which goes to kerod.
func (a *App) newPageReview(g *prPage) *review {
	return &review{
		sync: func(cm *reviewComment, deleted bool) { a.syncDraftComment(g, cm, deleted) },
		ask:  func(cm *reviewComment) { a.askAboutComment(g, cm) },
	}
}

// reviewParams are the params that name the page's pull request.
func (g *prPage) reviewParams(op string) proto.PRReviewParams {
	return proto.PRReviewParams{Op: op, Pane: g.pane, Remote: g.remote, Number: g.pr.Number}
}

// callReview calls pr.review with p, then shows the draft that comes of
// it, else why none did; then runs then, when set, with the draft.
func (a *App) callReview(g *prPage, p proto.PRReviewParams, then func(dr *proto.PRReviewDraft)) {
	cl := a.hostOf(g.pane).cl
	if cl == nil {
		return
	}
	go func() {
		var dr *proto.PRReviewDraft
		err := cl.Call("pr.review", p, &dr)
		a.update(func() {
			if err != nil {
				g.review.err, g.review.posting = err.Error(), false
				// What the user did that kerod didn't keep goes.
				a.applyDraft(g, g.review.draft)
				return
			}
			g.review.err = ""
			if p.Op != "post" {
				a.applyDraft(g, dr)
			}
			if then != nil {
				then(dr)
			}
		})
	}()
}

// fetchDraftReview asks for the page's draft review.
func (a *App) fetchDraftReview(g *prPage) { a.callReview(g, g.reviewParams("get"), nil) }

// applyDraft shows dr, nil for none: its comments in the page's review,
// those being edited kept as they are.
func (a *App) applyDraft(g *prPage, dr *proto.PRReviewDraft) {
	g.review.draft = dr
	if !g.review.summaryEdit {
		g.review.summary = ""
		if dr != nil {
			g.review.summary = dr.Summary
		}
	}
	rv := g.rv
	var edited int
	if rv.edits != nil {
		edited = rv.edits.id
	}
	rv.comments, rv.edits = nil, nil
	if dr == nil {
		return
	}
	for _, c := range dr.Comments {
		side := editor.DiffNew
		if c.Side == "LEFT" {
			side = editor.DiffOld
		}
		start := c.Start
		if start == 0 {
			start = c.Line
		}
		cm := &reviewComment{id: c.ID, path: c.Path, diff: baseRev + "|" + c.Path, side: side, start: start - 1, end: c.Line - 1, text: c.Body, agent: c.By == "agent"}
		rv.comments = append(rv.comments, cm)
		rv.last = max(rv.last, c.ID)
		if c.ID == edited {
			rv.edits = cm
		}
	}
}

// syncDraftComment keeps what the user did to a comment in kerod's draft,
// which it starts first when there is none.
func (a *App) syncDraftComment(g *prPage, cm *reviewComment, deleted bool) {
	if deleted {
		p := g.reviewParams("delete")
		p.ID = cm.id
		a.callReview(g, p, nil)
		return
	}
	p := g.reviewParams("comment")
	p.Comment = proto.PRDraftComment{Path: cm.path, Side: "RIGHT", Start: cm.start + 1, Line: cm.end + 1, Body: cm.text, By: "user"}
	if cm.side == editor.DiffOld {
		p.Comment.Side = "LEFT"
	}
	if dr := g.review.draft; dr != nil {
		for _, c := range dr.Comments {
			if c.ID == cm.id {
				// An edit keeps the comment's place, and who wrote it.
				p.Comment = proto.PRDraftComment{ID: cm.id, Body: cm.text}
			}
		}
		a.callReview(g, p, nil)
		return
	}
	a.callReview(g, g.reviewParams("start"), func(*proto.PRReviewDraft) { a.callReview(g, p, nil) })
}

// askAboutComment types a question about an agent's comment into the
// agent's pane, for the user to finish, and goes there.
func (a *App) askAboutComment(g *prPage, cm *reviewComment) {
	dr := g.review.draft
	if dr == nil || dr.Agent == "" {
		a.toast(locales.ReviewNoAgent())
		return
	}
	s := a.sessionOfPane(dr.Agent)
	if s == nil {
		a.toast(locales.ReviewNoAgent())
		return
	}
	cl := a.hostOf(dr.Agent).cl
	if cl == nil {
		return
	}
	text := locales.ReviewAskText(cm.id, commentPlace(cm)) + " "
	go cl.Call("send", proto.SendParams{ID: dr.Agent, Data: text, Paste: true}, nil)
	if a.focus == nil {
		a.focus = map[string]string{}
	}
	a.focus[s.ID] = dr.Agent
	a.selectSession(s.ID)
	a.focusTerminal()
}

// prDraftBar is the draft review, below the page: how many comments, the
// summary, the review's verdict, and Post Review; Discard in its menu.
func (a *App) prDraftBar(c *ui.Context, pal *palette, g *prPage) {
	st := &g.review
	dr := st.draft
	if dr == nil && st.err == "" {
		return
	}
	box := ui.Column(c.Key("draft-bar")).Shrink(0).Padding(8, 12, 12, 12).Gap(6).AlignItems(ui.Stretch)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, pal.text.Alpha(0.08), 0)
	})
	box.Children(func() {
		if st.err != "" {
			_, removed, _ := a.diffColors(c, pal)
			ui.Text(c, st.err).FontSize(12).TextColor(removed).LineHeight(1.35).Selectable()
		}
		if dr == nil {
			return
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, commentIcon).Size(12, 12).TextColor(pal.accent).Shrink(0)
			ui.Text(c, locales.PrsDraftReview(len(dr.Comments))).FontSize(12.5).FontWeight(600).TextColor(pal.text).SingleLine().MinWidth(0)
			ui.Spacer(c)
			b := ui.ButtonBase(c.Key("draft-menu")).Size(24, 24).Radius(6).Center().TextColor(pal.textMuted).Label(locales.PrsDraftOptions()).Tooltip(locales.PrsDraftOptions())
			if b.Hovered() {
				b.Background(pal.rowHover)
			}
			b.Children(func() { ui.Icon(c, moreIcon).Size(14, 14) })
			b.Menu(func(m *ui.Menu) {
				if m.Item(locales.PrsDiscardReview()).Chosen() {
					a.confirm = &confirmation{open: true, title: locales.PrsDiscardTitle(), detail: locales.PrsDiscardDetail(), action: locales.PrsDiscard(), destroy: true,
						run: func() { a.callReview(g, g.reviewParams("discard"), nil) }}
				}
			})
		})
		if g.res != nil && dr.Head != "" && g.res.HeadOID != "" && dr.Head != g.res.HeadOID {
			ui.Text(c, locales.PrsDraftOld()).FontSize(12).TextColor(pal.attention).LineHeight(1.35)
		}
		frame := ui.Row(c.Key("summary")).AlignItems(ui.Start).Cursor(ui.CursorText)
		var in ui.Element
		frame.Children(func() {
			in = ui.TextAreaBase(c.Key("in"), &st.summary).Grow(1).MinWidth(0).MinHeight(44).MaxHeight(140).Padding(6, 8).FontSize(13).
				Placeholder(locales.PrsSummary()).Label(locales.PrsSummary())
		})
		fieldLook(c, frame, in.Focused(), pal).Padding(0)
		if in.Focused() {
			a.side.editing = true
		}
		// Kept as the field loses the focus.
		if st.summaryEdit && !in.Focused() && strings.TrimSpace(st.summary) != dr.Summary {
			p := g.reviewParams("summary")
			p.Summary = st.summary
			a.callReview(g, p, nil)
		}
		st.summaryEdit = in.Focused()
		ids := []string{github.EventComment, github.EventApprove, github.EventRequestChanges}
		labels := []string{locales.PrsEventComment(), locales.PrsApprove(), locales.PrsRequestChanges()}
		// GitHub takes no verdict on one's own pull request.
		if g.res != nil && g.res.Viewer != "" && strings.EqualFold(g.res.Viewer, g.res.Author) {
			ids, labels = ids[:1], labels[:1]
		}
		if !slices.Contains(ids, st.event) {
			st.event = github.EventComment
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			segmented(c, "event", &st.event, ids, labels)
			ui.Spacer(c)
			can := !st.posting && (len(dr.Comments) > 0 || strings.TrimSpace(st.summary) != "" || st.event == github.EventApprove)
			if sizedButton(c, "post", locales.PrsPost(), "", can, 28, 8, pal) && can {
				a.confirmPost(g)
			}
		})
	})
}

// confirmPost asks before the review goes to GitHub, where everyone sees
// it, then posts it with the summary as typed.
func (a *App) confirmPost(g *prPage) {
	st := &g.review
	dr := st.draft
	a.confirm = &confirmation{open: true, title: locales.PrsPostTitle(g.repoSlug() + "#" + strconv.Itoa(g.pr.Number)), detail: locales.PrsPostDetail(len(dr.Comments)),
		action: locales.PrsPost(), run: func() {
			st.posting = true
			post := func() {
				p := g.reviewParams("post")
				p.Event = st.event
				a.callReview(g, p, func(done *proto.PRReviewDraft) {
					st.posting = false
					a.applyDraft(g, nil)
					a.toast(locales.PrsPosted())
					a.fetchPRPage(g)
				})
			}
			if strings.TrimSpace(st.summary) == dr.Summary {
				post()
				return
			}
			p := g.reviewParams("summary")
			p.Summary = st.summary
			a.callReview(g, p, func(*proto.PRReviewDraft) { post() })
		}}
}
