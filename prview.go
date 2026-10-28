package main

import (
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"kero/internal/proto"

	"kero/locales"
)

// A pull request's page, in the Review view, as GitHub's shows it: its
// title, its state and branches, then its conversation (its description,
// its reviewers, its checks, its comments and reviews) or its changes,
// whose diffs show its comments on lines under them.

// prPage is the pull request shown, of a pane's repository's remote.
type prPage struct {
	pane, root   string
	remote, slug string
	pr           proto.PullRequest // as the list had it, shown until res comes
	res          *proto.PRView
	err          string
	busy         bool
	gen          int
	tab          string // prConversation or prFiles
	files        map[string]*sideFile
	md           map[string]*markdown.Options // each text's, by its key
	parts        map[string]prText
	opened       map[string]bool // the <details> opened or closed, by key
	scroll       ui.ScrollState
	checksOpen   bool
	// rv shows the draft review's comments on the diffs (prdraft.go).
	rv     *review
	review prDraftState
	// at is the comment jumped to last (jumpComment), which revealSpot
	// brings into view in the frames reveal counts down, until it is; view
	// is the files' box, as last laid out.
	at     prSpot
	reveal int
	view   ui.Rect
}

// prSpot is a comment on the page's diffs, which jumpComment goes to: of
// the draft review, by its id, or a thread, by its first comment's.
type prSpot struct {
	path   string
	line   int // its last line, from 0
	draft  int
	thread int64
}

// The page's tabs.
const (
	prConversation = "conversation"
	prFiles        = "files"
)

// repoSlug is the page's repository, as OWNER/NAME.
func (g *prPage) repoSlug() string {
	if g.slug == "" && g.res != nil {
		return g.res.Repo.Slug
	}
	return g.slug
}

// openPRPage shows a pull request of the list's remote.
func (a *App) openPRPage(pane *proto.Pane, remote, slug string, pr proto.PullRequest) {
	g := &prPage{pane: pane.ID, root: pane.Git.Root, remote: remote, slug: slug, pr: pr, tab: prConversation,
		files: map[string]*sideFile{}, md: map[string]*markdown.Options{}, parts: map[string]prText{}, opened: map[string]bool{}}
	g.rv = a.newPageReview(g)
	g.rv.reveal = func(c *ui.Context, card ui.Element) { revealSpot(c, g, card) }
	a.side.pulls.page = g
	a.fetchPRPage(g)
}

// fetchPRPage asks for the pull request, and its changes.
func (a *App) fetchPRPage(g *prPage) {
	cl := a.hostOf(g.pane).cl
	if cl == nil {
		return
	}
	g.gen++
	gen := g.gen
	g.busy, g.err = true, ""
	a.fetchDraftReview(g)
	p := proto.PRViewParams{Pane: g.pane, Remote: g.remote, Number: g.pr.Number}
	go func() {
		var res proto.PRView
		err := cl.Call("pr.view", p, &res)
		// Threads' comments show as their text, but for the HTML in it.
		for i := range res.LineComments {
			res.LineComments[i].Body = inlineHTML(res.LineComments[i].Body, true)
		}
		a.update(func() {
			if g.gen != gen {
				return
			}
			g.busy = false
			if err != nil {
				g.err = err.Error()
				return
			}
			if g.res == nil || g.res.HeadOID != res.HeadOID || g.res.MergeBase != res.MergeBase {
				g.files = map[string]*sideFile{}
			}
			g.res, g.pr = &res, res.PullRequest
		})
	}()
}

func (a *App) prPageView(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, width float32) {
	ps := &a.side.pulls
	g := ps.page
	ui.Row(c.Key("page-bar")).Shrink(0).Height(32).Padding(0, 10, 4, 6).Gap(4).AlignItems(ui.Center).Children(func() {
		iconButton(c, "back", chevronLeftIcon, locales.PrsBack(), pal, func() { ps.page = nil })
		ui.Text(c, "#"+strconv.Itoa(g.pr.Number)).FontSize(13.5).FontWeight(650).FontFeatures("tnum").TextColor(pal.textMuted).SingleLine()
		ui.Spacer(c)
		if g.busy {
			ui.Spinner(c).Size(12, 12).Margin(0, 4, 0, 0)
		}
		iconButton(c, "refresh", reloadIcon, locales.PrsRefresh(), pal, func() { a.fetchPRPage(g) })
		iconButton(c, "open", linkIcon, locales.PrsOpen(), pal, func() { openURL(g.pr.URL) })
		pr := g.pr
		a.reviewButton(c, pal, func(mode string) { a.askReview(s, pane, g.remote, g.slug, pr, mode) })
	})
	if ps.page == nil {
		return
	}
	ui.Column(c.Key("page-head")).Shrink(0).Padding(0, 12, 8, 12).Gap(6).AlignItems(ui.Stretch).Children(func() {
		ui.Text(c, g.pr.Title).FontSize(15).FontWeight(650).TextColor(pal.text).LineHeight(1.3).Selectable()
		a.prMeta(c, pal, g)
		files := locales.PrsFilesTab()
		if g.res != nil {
			files = locales.PrsFilesTabCount(len(g.res.Files))
		}
		ui.Row(c).Margin(4, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			segmented(c, "tab", &g.tab, []string{prConversation, prFiles}, []string{locales.PrsConversation(), files})
			ui.Spacer(c)
			a.commentJump(c, pal, g)
		})
	})
	switch {
	case g.res == nil && g.err != "":
		a.sideMessage(c, pal, locales.PrsCantView(), g.err)
		return
	case g.res == nil:
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	}
	if g.err != "" {
		_, removed, _ := a.diffColors(c, pal)
		ui.Text(c, g.err).FontSize(12).TextColor(removed).LineHeight(1.35).Padding(0, 12, 6, 12).Selectable()
	}
	if g.tab == prFiles {
		a.prPageFiles(c, pal, s, pane, g, width)
	} else {
		ui.Scroll(c.Key("conversation")).Grow(1).MinHeight(0).TrackScroll(&g.scroll).Children(func() {
			ui.Column(c).Padding(4, 12, 20, 12).Gap(16).AlignItems(ui.Stretch).Children(func() {
				a.prDescription(c, pal, g)
				a.prReviewers(c, pal, g.res)
				a.prChecks(c, pal, g)
				a.prTimeline(c, pal, g)
			})
		})
	}
	a.prDraftBar(c, pal, g, width)
}

// reviewButton is Review, which has an agent review the pull request in a
// worktree of its own; its menu, in the other places (reviewItems).
func (a *App) reviewButton(c *ui.Context, pal *palette, launch func(mode string)) {
	b := ui.ButtonBase(c.Key("review")).Height(24).Padding(0, 8).Gap(5).Radius(6).AlignItems(ui.Center).Shrink(0).
		TextColor(pal.textMuted).Label(locales.PrsReview()).Tooltip(locales.PrsReviewTip())
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() {
		ui.Icon(c, sparklesIcon).Size(12, 12)
		ui.Text(c, locales.PrsReview()).FontSize(12).FontWeight(500).SingleLine()
	})
	b.ContextMenu(func(m *ui.Menu) { reviewItems(m, launch) })
	if b.Clicked() {
		launch(reviewWorktree)
	}
}

// prMeta is the pull request's state, its author, its branches and the
// lines it adds and removes.
func (a *App) prMeta(c *ui.Context, pal *palette, g *prPage) {
	ui.Row(c.Key("meta")).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
		state, col := locales.PrsOpenState(), pal.accent
		switch {
		case g.res != nil && g.res.State == "MERGED":
			state = locales.PrsMerged()
		case g.res != nil && g.res.State == "CLOSED":
			state, col = locales.PrsClosed(), pal.textFaint
		case g.pr.Draft:
			state, col = locales.PrsDraft(), pal.textFaint
		}
		icon := prIcon
		if g.pr.Draft {
			icon = draftPRIcon
		}
		ui.Row(c.Key("state")).Height(20).Padding(0, 7).Gap(4).Radius(10).AlignItems(ui.Center).Background(col.Alpha(0.14)).TextColor(col).Shrink(0).Children(func() {
			ui.Icon(c, icon).Size(11, 11)
			ui.Text(c, state).FontSize(11.5).FontWeight(600).SingleLine()
		})
		head := g.pr.Head
		if g.pr.CrossRepo && g.pr.HeadOwner != "" {
			head = g.pr.HeadOwner + ":" + head
		}
		ui.Text(c, locales.PrsByInto(g.pr.Author, head, g.pr.Base)).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35).MinWidth(0).Shrink(1)
		if g.res != nil {
			a.lineCounts(c, pal, g.res.Additions, g.res.Deletions, 12)
		}
		if g.res != nil {
			for _, l := range g.res.Labels {
				ui.Text(c, l).FontSize(11).FontWeight(550).TextColor(pal.textMuted).Padding(2, 7).Radius(9).Background(pal.chip).SingleLine().Shrink(0)
			}
		}
	})
}

// prText is a text's parts, kept while the text is the same.
type prText struct {
	src   string
	parts []mdPart
}

// prMarkdown shows text as markdown, its parse kept by key: links open,
// images show as their descriptions, <details> fold, and other HTML, as
// GitHub's bots write it, shows as the markdown it means (mdhtml.go).
func (a *App) prMarkdown(c *ui.Context, g *prPage, key, text string) {
	t, ok := g.parts[key]
	if !ok || t.src != text {
		t = prText{src: text, parts: mdParts(inlineHTML(text, false))}
		g.parts[key] = t
	}
	ui.Column(c.Key("md-" + key)).Gap(10).AlignItems(ui.Stretch).Children(func() { a.prParts(c, g, key, t.parts) })
}

// prParts shows parts of a text, key's.
func (a *App) prParts(c *ui.Context, g *prPage, key string, parts []mdPart) {
	pal := paletteFor(c)
	for i, p := range parts {
		k := key + "." + itoa(i)
		if !p.details {
			o := g.md[k]
			if o == nil {
				o = &markdown.Options{HTML: a.prHTML(g, k)}
				g.md[k] = o
			}
			ui.Column(c.Key(k)).AlignItems(ui.Stretch).Children(func() { markdown.View(c, p.text, o) })
			continue
		}
		open, ok := g.opened[k]
		if !ok {
			open = p.open
		}
		ui.Column(c.Key(k)).AlignItems(ui.Stretch).Children(func() {
			head := ui.Row(c.Key("head")).Gap(6).Padding(3, 0).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if head.Clicked() {
				open = !open
				g.opened[k] = open
			}
			head.Children(func() {
				chev := ui.Icon(c, chevronIcon).Size(9, 9).TextColor(pal.textFaint).Shrink(0)
				chev.Rotate(chev.AnimateTo("turn", 90*b2f(open), chevronTurn))
				summary := p.summary
				if summary == "" {
					summary = locales.PrsDetails()
				}
				ui.Text(c, summary).FontWeight(600).LineHeight(1.35).MinWidth(0).Shrink(1)
			})
			if open {
				ui.Column(c.Key("body")).Gap(10).Padding(6, 0, 4, 15).AlignItems(ui.Stretch).Children(func() {
					a.prParts(c, g, k, p.parts)
				})
			}
		})
	}
}

// prHTML shows an HTML block of the text key names as the markdown it
// means, and that markdown's own HTML blocks as their text.
func (a *App) prHTML(g *prPage, key string) func(c *ui.Context, h string) {
	key += "/html"
	done := map[string]string{}
	return func(c *ui.Context, h string) {
		md, ok := done[h]
		if !ok {
			md = htmlMarkdown(h)
			done[h] = md
		}
		o := g.md[key]
		if o == nil {
			o = &markdown.Options{HTML: func(c *ui.Context, h string) {
				if t := htmlText(h); t != "" {
					ui.Text(c, t).TextColor(paletteFor(c).textMuted).LineHeight(1.4).Selectable()
				}
			}}
			g.md[key] = o
		}
		markdown.View(c, md, o)
	}
}

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	blankLines  = regexp.MustCompile(`[ \t]*\n\s*`)
)

// htmlText is the text of HTML, without its tags and comments: a line
// where a tag was, as a block's summary and its text are apart.
func htmlText(h string) string {
	t := htmlTag.ReplaceAllString(htmlComment.ReplaceAllString(h, ""), "\n")
	return strings.TrimSpace(blankLines.ReplaceAllString(html.UnescapeString(t), "\n"))
}

// prDescription is the pull request's description.
func (a *App) prDescription(c *ui.Context, pal *palette, g *prPage) {
	if strings.TrimSpace(g.res.Body) == "" {
		ui.Text(c, locales.PrsNoDescription()).FontSize(12.5).TextColor(pal.textFaint)
		return
	}
	a.prMarkdown(c, g, "body", g.res.Body)
}

// prSection is a section's title.
func prSection(c *ui.Context, pal *palette, title string) {
	ui.Text(c, title).FontSize(12).FontWeight(650).TextColor(pal.textMuted)
}

// prReviewer is a reviewer's review as it stands: the state of the last
// review that decided (approved, changes requested, dismissed), else of
// the last that commented; pending while it is asked again, or yet.
type prReviewer struct {
	login, state string
	pending      bool
}

// prReviewersOf are those who reviewed, then those whose review is asked,
// but the pull request's author.
func prReviewersOf(v *proto.PRView) []prReviewer {
	var out []prReviewer
	at := map[string]int{}
	for _, r := range v.Reviews {
		if r.State == "PENDING" || r.Author == v.Author {
			continue
		}
		i, ok := at[r.Author]
		if !ok {
			at[r.Author] = len(out)
			out = append(out, prReviewer{login: r.Author, state: r.State})
			continue
		}
		if r.State != "COMMENTED" || out[i].state == "COMMENTED" {
			out[i].state = r.State
		}
	}
	for _, q := range v.Requested {
		if i, ok := at[q]; ok {
			out[i].pending = true
			continue
		}
		at[q] = len(out)
		out = append(out, prReviewer{login: q, pending: true})
	}
	return out
}

// prReviewers are the reviewers and how each one's review stands, and
// GitHub's decision.
func (a *App) prReviewers(c *ui.Context, pal *palette, v *proto.PRView) {
	added, removed, _ := a.diffColors(c, pal)
	ui.Column(c.Key("reviewers")).Gap(4).AlignItems(ui.Stretch).Children(func() {
		prSection(c, pal, locales.PrsReviewers())
		rs := prReviewersOf(v)
		if len(rs) == 0 {
			ui.Text(c, locales.PrsNoReviewers()).FontSize(12.5).TextColor(pal.textFaint)
		}
		for _, r := range rs {
			text, col := locales.PrsAwaiting(), pal.attention
			switch {
			case r.pending:
			case r.state == "APPROVED":
				text, col = locales.PrsApproved(), added
			case r.state == "CHANGES_REQUESTED":
				text, col = locales.PrsChangesRequested(), removed
			case r.state == "DISMISSED":
				text, col = locales.PrsDismissed(), pal.textFaint
			default:
				text, col = locales.PrsCommented(), pal.textMuted
			}
			ui.Row(c.Key("r-" + r.login)).Height(22).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Background(col).Shrink(0)
				ui.Text(c, r.login).FontSize(12.5).FontWeight(550).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
				ui.Spacer(c)
				ui.Text(c, text).FontSize(12).TextColor(col).SingleLine().Shrink(0)
			})
		}
	})
}

// prChecks sums the checks up, which a click lists, the failed and the
// pending first; each opens its page.
func (a *App) prChecks(c *ui.Context, pal *palette, g *prPage) {
	runs := g.res.CheckRuns
	if len(runs) == 0 {
		return
	}
	added, removed, _ := a.diffColors(c, pal)
	count := map[string]int{}
	for _, r := range runs {
		count[r.State]++
	}
	look := func(state string) (string, ui.Color) {
		switch state {
		case "success":
			return "✓", added
		case "failure":
			return "✕", removed
		case "pending":
			return "●", pal.attention
		}
		return "–", pal.textFaint
	}
	ui.Column(c.Key("checks")).Gap(2).AlignItems(ui.Stretch).Children(func() {
		head := ui.Row(c.Key("head")).Height(24).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if head.Clicked() {
			g.checksOpen = !g.checksOpen
		}
		head.Children(func() {
			chev := ui.Icon(c, chevronIcon).Size(9, 9).TextColor(pal.textFaint).Shrink(0)
			chev.Rotate(chev.AnimateTo("turn", 90*b2f(g.checksOpen), chevronTurn))
			prSection(c, pal, locales.PrsChecks())
			for _, st := range []string{"failure", "pending", "success"} {
				if n := count[st]; n > 0 {
					mark, col := look(st)
					ui.Text(c, mark+" "+itoa(n)).FontSize(12).FontWeight(550).FontFeatures("tnum").TextColor(col).SingleLine().Shrink(0)
				}
			}
		})
		if !g.checksOpen {
			return
		}
		order := map[string]int{"failure": 0, "pending": 1, "success": 2}
		sorted := slices.Clone(runs)
		slices.SortStableFunc(sorted, func(x, y proto.PRCheck) int {
			ox, ok := order[x.State]
			if !ok {
				ox = 3
			}
			oy, ok := order[y.State]
			if !ok {
				oy = 3
			}
			return ox - oy
		})
		for i, r := range sorted {
			mark, col := look(r.State)
			name := r.Name
			if r.Workflow != "" {
				name = r.Workflow + " / " + r.Name
			}
			row := ui.Row(c.Key(i)).Height(22).Padding(0, 6, 0, 15).Gap(8).Radius(5).AlignItems(ui.Center).Tooltip(r.URL)
			if r.URL != "" {
				row.Cursor(ui.CursorPointer)
				if row.Hovered() {
					row.Background(pal.rowHover.Alpha(0.6))
				}
				if row.Clicked() {
					openURL(r.URL)
				}
			}
			row.Children(func() {
				ui.Text(c, mark).FontSize(12).TextColor(col).Shrink(0)
				ui.Text(c, name).FontSize(12).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
			})
		}
	})
}

// prEntry is an entry of the conversation: a comment, a review, or a
// comment on lines that changed since (place says which).
type prEntry struct {
	key, author, state, body, url string
	at                            int64
	thread                        *prThread
}

// prTimeline is the conversation, the oldest first: comments, reviews
// that say something, or decide, and the threads of comments on lines and
// files, with their replies, each where it was written, which a click
// goes to.
func (a *App) prTimeline(c *ui.Context, pal *palette, g *prPage) {
	v := g.res
	var es []prEntry
	for i, cm := range v.Comments {
		es = append(es, prEntry{key: "c" + itoa(i), author: cm.Author, body: cm.Body, url: cm.URL, at: cm.Created})
	}
	for i, r := range v.Reviews {
		if r.State == "PENDING" || r.State == "COMMENTED" && strings.TrimSpace(r.Body) == "" {
			continue
		}
		es = append(es, prEntry{key: "r" + itoa(i), author: r.Author, state: r.State, body: r.Body, at: r.Submitted})
	}
	for _, t := range prThreads(v, "") {
		first := t.comments[0]
		es = append(es, prEntry{key: "l" + strconv.FormatInt(first.ID, 10), at: first.Created, thread: t})
	}
	slices.SortStableFunc(es, func(x, y prEntry) int { return int(x.at - y.at) })
	added, removed, _ := a.diffColors(c, pal)
	now := time.Now()
	ui.Column(c.Key("timeline")).Gap(10).AlignItems(ui.Stretch).Children(func() {
		prSection(c, pal, locales.PrsConversation())
		if len(es) == 0 {
			ui.Text(c, locales.PrsNoComments()).FontSize(12.5).TextColor(pal.textFaint)
		}
		for _, e := range es {
			if e.thread != nil {
				ui.Column(c.Key(e.key)).AlignItems(ui.Stretch).Children(func() { a.threadCard(c, pal, g, e.thread, true) })
				continue
			}
			ui.Column(c.Key(e.key)).Padding(8, 10).Gap(6).Radius(8).Background(pal.chip).AlignItems(ui.Stretch).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, e.author).FontSize(12.5).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
					switch e.state {
					case "APPROVED":
						ui.Text(c, locales.PrsApproved()).FontSize(11.5).FontWeight(550).TextColor(added).SingleLine().Shrink(0)
					case "CHANGES_REQUESTED":
						ui.Text(c, locales.PrsChangesRequested()).FontSize(11.5).FontWeight(550).TextColor(removed).SingleLine().Shrink(0)
					case "COMMENTED":
						ui.Text(c, locales.PrsReviewed()).FontSize(11.5).TextColor(pal.textMuted).SingleLine().Shrink(0)
					case "DISMISSED":
						ui.Text(c, locales.PrsDismissed()).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0)
					}
					ui.Spacer(c)
					ui.Text(c, ago(e.at*1000, now)).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0)
				})
				if strings.TrimSpace(e.body) != "" {
					a.prMarkdown(c, g, e.key, e.body)
				}
			})
		}
	})
}

// prSpots are the comments on the page's diffs, in the order of their
// files, then of their lines: the draft review's, and the threads that
// aren't outdated.
func prSpots(g *prPage) []prSpot {
	if g.res == nil {
		return nil
	}
	var spots []prSpot
	// A file's own comments, above its diff, before those on its lines.
	for _, cm := range g.rv.comments {
		line := cm.end
		if cm.file {
			line = -1
		}
		spots = append(spots, prSpot{path: cm.path, line: line, draft: cm.id})
	}
	for _, lc := range g.res.LineComments {
		switch {
		case lc.ReplyTo == 0 && lc.File:
			spots = append(spots, prSpot{path: lc.Path, line: -1, thread: lc.ID})
		case lc.ReplyTo == 0 && lc.Line > 0:
			spots = append(spots, prSpot{path: lc.Path, line: lc.Line - 1, thread: lc.ID})
		}
	}
	order := map[string]int{}
	for i, f := range g.res.Files {
		order[f.Path] = i
	}
	rank := func(path string) int {
		if i, ok := order[path]; ok {
			return i
		}
		return len(order)
	}
	slices.SortStableFunc(spots, func(x, y prSpot) int {
		if d := rank(x.path) - rank(y.path); d != 0 {
			return d
		}
		return x.line - y.line
	})
	return spots
}

// jumpComment goes to the next comment on the page's diffs, by (+1 or
// -1) of the one jumped to last, round: on the Files tab, its file
// unfolded, it comes into view, marked.
func (a *App) jumpComment(g *prPage, by int) {
	spots := prSpots(g)
	if len(spots) == 0 {
		return
	}
	i := slices.Index(spots, g.at)
	switch {
	case i >= 0:
		i = (i + by + len(spots)) % len(spots)
	case by > 0:
		i = 0
	default:
		i = len(spots) - 1
	}
	a.goToSpot(g, spots[i])
}

// goToSpot shows a comment of the page's diffs on the Files tab, its file
// unfolded, in view, marked.
func (a *App) goToSpot(g *prPage, at prSpot) {
	g.at, g.reveal, g.rv.at = at, revealFrames, at.draft
	g.tab = prFiles
	ch := &a.side.changes
	if ch.collapsed == nil {
		ch.collapsed = map[string]bool{}
	}
	ch.collapsed[prFileKey(g.res, at.path)] = false
}

// prFileKey is what a file of the page's changes folds by.
func prFileKey(v *proto.PRView, path string) string { return "prv|" + v.HeadOID + "|" + path }

// commentJump is how many comments the diffs have, and which was jumped
// to, with Previous Comment and Next Comment.
func (a *App) commentJump(c *ui.Context, pal *palette, g *prPage) {
	spots := prSpots(g)
	if len(spots) == 0 {
		return
	}
	count := itoa(len(spots))
	if i := slices.Index(spots, g.at); i >= 0 {
		count = itoa(i+1) + "/" + count
	}
	ui.Row(c.Key("jump")).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Icon(c, commentIcon).Size(12, 12).TextColor(pal.accent).Margin(0, 4, 0, 0)
		ui.Text(c, count).FontSize(12).FontWeight(600).FontFeatures("tnum").TextColor(pal.textMuted).SingleLine().Margin(0, 2, 0, 0)
		iconButton(c, "previous", chevronUpIcon, locales.PrsPreviousComment(), pal, func() { a.jumpComment(g, -1) })
		iconButton(c, "next", chevronDownIcon, locales.PrsNextComment(), pal, func() { a.jumpComment(g, 1) })
	})
}

// revealSpot, in the card of the comment jumped to, brings it into view,
// near the top of the files, the lines it is on above it: in each frame
// until the card, as last laid out, is in the files' upper half, as the
// diff places its cards only once it is laid out, which may move them.
func revealSpot(c *ui.Context, g *prPage, card ui.Element) {
	if g.reveal <= 0 {
		return
	}
	g.reveal--
	r, v := card.Bounds(), g.view
	if r.H > 0 && v.H > 0 && r.Y >= v.Y && r.Y <= v.Y+v.H/2 {
		g.reveal = 0
		return
	}
	h := v.H
	if h <= 0 {
		h = 400
	}
	// Scrolled as little as shows it, a box the files' height from above
	// the card shows the card at its top, whichever way it comes from.
	ui.Box(c.Key("reveal")).Absolute().Top(-revealAbove).Left(0).Width(1).Height(max(h-8, revealAbove+40)).ScrollIntoView()
}

const (
	// revealAbove is how much of the diff above a comment jumped to shows.
	revealAbove = 64
	// revealFrames is how many frames revealSpot tries for at most, which
	// leaves the user to scroll a card that never gets there.
	revealFrames = 30
)

// prPageFiles are the files the pull request changes, each of whose
// headers unfolds its diff, with the comments on its lines. A worktree
// that has the pull request checked out opens them, or drags them onto a
// pane; else they are no file on disk.
func (a *App) prPageFiles(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, g *prPage, width float32) {
	v := g.res
	if v.FetchError != "" {
		a.sideMessage(c, pal, locales.PrsCantFetch(), v.FetchError)
		return
	}
	if len(v.Files) == 0 {
		a.sideMessage(c, pal, locales.PrsNoFiles(), "")
		return
	}
	asking := 0
	for _, sf := range g.files {
		if sf.busy {
			asking++
		}
	}
	files := ui.Scroll(c.Key("files")).Grow(1).MinHeight(0)
	g.view = files.Bounds()
	files.Children(func() {
		ui.Column(c).Padding(0, 6, 16, 0).AlignItems(ui.Stretch).Children(func() {
			if v.Checkout != "" {
				ui.Text(c, locales.PrsCheckedOut(a.hostOf(pane.ID).shortDir(v.Checkout))).FontSize(11.5).TextColor(pal.textFaint).
					SingleLine().Ellipsis("…").Padding(0, 6, 6, 12)
			}
			for _, fl := range v.Files {
				key := prFileKey(v, fl.Path)
				col := ui.Column(c.Key(key)).AlignItems(ui.Stretch).Padding(0, 0, 2, 0)
				col.Children(func() {
					a.fileHeader(c, pal, key, pane.ID, v.Checkout, fl, flatList, nil, func() {
						if n := prThreadCount(v, fl.Path); n > 0 {
							ui.Row(c).Gap(3).AlignItems(ui.Center).Shrink(0).Children(func() {
								ui.Icon(c, commentIcon).Size(11, 11).TextColor(pal.textFaint)
								ui.Text(c, itoa(n)).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine()
							})
						}
						path := fl.Path
						iconButton(c, "comment-file", commentIcon, locales.PrsCommentOnFile(), pal, func() { a.startFileComment(g, path) })
					}, nil)
					if a.fileCollapsed(key) {
						return
					}
					a.prFileNotes(c, pal, g, fl.Path)
					sf := g.files[fl.Path]
					if sf == nil {
						sf = &sideFile{pane: pane.ID, base: baseRev, f: fl, pr: g}
						g.files[fl.Path] = sf
					}
					if !sf.busy && sf.d == nil && !sf.text.Binary && !sf.text.Large && sf.err == "" && asking < 4 {
						asking++
						a.fetchFile(sf, "")
					}
					a.fileDiff(c, pal, s, sf, width-6)
					// The file comes into view while the diff with the
					// comment jumped to comes.
					if g.reveal > 0 && g.at.path == fl.Path && sf.d == nil {
						col.ScrollIntoView()
					}
				})
			}
		})
	})
}

// prThreadCount is how many threads of comments a file's diff shows.
func prThreadCount(v *proto.PRView, path string) int {
	n := 0
	for _, lc := range v.LineComments {
		if lc.Path == path && lc.ReplyTo == 0 && (lc.Line > 0 || lc.File) {
			n++
		}
	}
	return n
}

// prThread is a thread of comments on lines: its first, and the replies.
type prThread struct {
	side       editor.DiffSide
	line       int // the last line, from 0
	start      int // the first, from 0
	comments   []proto.PRLineComment
	outdated   bool
	originally int  // the line it was written on, from 1, when outdated
	file       bool // on the file, which shows it above its diff
	// id resolves it on GitHub, "" when unknown; resolved, whether it is.
	id       string
	resolved bool
}

// prThreads are a file's threads, of every file's for path "", in the
// order they were started.
func prThreads(v *proto.PRView, path string) []*prThread {
	var ts []*prThread
	by := map[int64]*prThread{}
	for _, lc := range v.LineComments {
		if path != "" && lc.Path != path {
			continue
		}
		if lc.ReplyTo != 0 {
			if t := by[lc.ReplyTo]; t != nil {
				t.comments = append(t.comments, lc)
				by[lc.ID] = t
			}
			continue
		}
		t := &prThread{side: editor.DiffNew, comments: []proto.PRLineComment{lc}, outdated: lc.Line == 0 && !lc.File, originally: lc.OriginalLine, file: lc.File,
			id: lc.Thread, resolved: lc.Resolved}
		if lc.Side == "LEFT" {
			t.side = editor.DiffOld
		}
		t.line, t.start = lc.Line-1, lc.Line-1
		if lc.StartLine > 0 && lc.StartLine < lc.Line {
			t.start = lc.StartLine - 1
		}
		ts = append(ts, t)
		by[lc.ID] = t
	}
	return ts
}

// Heights of a thread's card under its lines, besides its comments' text.
const (
	threadChrome  = commentAbove + commentBelow + commentPadTop + commentPadBottom
	threadHeadH   = 20 // a comment's author and time
	threadBetween = 8  // between a thread's comments
	replyPad      = 8  // above a reply, below the rule that sets it apart
	replyIndent   = 12 // a reply's, from the card's edge
	threadFootH   = 24 // Resolve Conversation
)

// threadFolded tells a resolved thread the user hasn't opened, which shows
// as a line.
func (g *prPage) threadFolded(t *prThread) bool { return t.resolved && !g.opened["t:"+t.id] }

// threadHeight is how high threadCard is under its lines, its text
// wrapping at textW.
func threadHeight(c *ui.Context, g *prPage, t *prThread, textW float32) float32 {
	h := float32(threadChrome + threadHeadH)
	if g.threadFolded(t) {
		return h
	}
	for i, cm := range t.comments {
		w := textW
		if i > 0 {
			w -= replyIndent
			h += threadBetween + replyPad + threadHeadH
		}
		_, th := c.MeasureText(w, ui.Span{Text: cm.Body, Size: 13})
		h += th
	}
	if t.id != "" {
		h += threadBetween + threadFootH
	}
	return h
}

// prNotes sets what a pull request's diff shows under its lines: its
// threads of comments, those whose lines changed since on the
// conversation, as outdated; and the draft review's comments, which the
// gutter's button adds to.
func (a *App) prNotes(c *ui.Context, pal *palette, sf *sideFile, width float32) {
	d, v := sf.d, sf.pr.res
	if v == nil {
		return
	}
	textW := max(width-2*commentMargin-commentPadLeft-commentPadRight-60, 120)
	if d.Layout == editor.DiffSplit {
		textW = max(width/2-2*commentMargin-commentPadLeft-commentPadRight-40, 120)
	}
	threads := prThreads(v, sf.f.Path)
	byKey := map[int64]*prThread{}
	for _, t := range threads {
		if t.outdated || t.file {
			continue
		}
		buf := d.New
		if t.side == editor.DiffOld {
			buf = d.Old
		}
		if t.line < 0 || t.line >= buf.LineCount() {
			continue
		}
		h := threadHeight(c, sf.pr, t, textW)
		key := t.comments[0].ID
		byKey[key] = t
		d.Annotations = append(d.Annotations, editor.DiffAnnotation{Side: t.side, Line: t.line, Height: h, Key: key})
	}
	// The draft review's comments, and the user's to add from the gutter.
	d.OnGutterButton = func(line editor.DiffLineRange) {
		start, end := commentLines(d.SelectedRanges(), line)
		a.startComment(sf, line.Side, start, end)
	}
	drafts := a.commentNotes(c, pal, sf, sf.pr.rv, width)
	d.RenderAnnotation = func(c *ui.Context, an editor.DiffAnnotation) {
		if drafts(c, an) {
			return
		}
		id, _ := an.Key.(int64)
		if t := byKey[id]; t != nil {
			a.threadCard(c, pal, sf.pr, t, false)
		}
	}
}

// prFileNotes are the comments on a file, above its diff: the threads
// posted on it, the draft review's, and the one being written.
func (a *App) prFileNotes(c *ui.Context, pal *palette, g *prPage, path string) {
	rv := g.rv
	var threads []*prThread
	for _, t := range prThreads(g.res, path) {
		if t.file {
			threads = append(threads, t)
		}
	}
	var drafts []*reviewComment
	for _, cm := range rv.comments {
		if cm.file && cm.path == path && cm != rv.edits {
			drafts = append(drafts, cm)
		}
	}
	writing := rv.draft != nil && rv.draft.file && rv.draft.path == path
	if len(threads) == 0 && len(drafts) == 0 && !writing {
		return
	}
	ui.Column(c.Key("file-notes")).Padding(4, 0, 2, 0).AlignItems(ui.Stretch).Children(func() {
		for _, t := range threads {
			ui.Column(c.Key(t.comments[0].ID)).AlignItems(ui.Stretch).Children(func() { a.threadCard(c, pal, g, t, false) })
		}
		for _, cm := range drafts {
			ui.Column(c.Key(cm.id)).AlignItems(ui.Stretch).Children(func() { a.commentCard(c, pal, rv, cm) })
		}
		if writing {
			ui.Column(c.Key("draft")).Height(draftH).AlignItems(ui.Stretch).Children(func() { a.commentDraft(c, pal, rv) })
		}
	})
}

// threadCard is a thread of comments posted: the first, then each reply
// under a rule, in from the edge, each with its author and when; then
// Resolve Conversation. A resolved one is a line, which a click opens. In
// the conversation, at, it names where it is, which a click goes to. It is
// marked, and brought into view, when the page jumped to it.
func (a *App) threadCard(c *ui.Context, pal *palette, g *prPage, t *prThread, at bool) {
	now := time.Now()
	added, _, _ := a.diffColors(c, pal)
	jumped := g.at.thread == t.comments[0].ID
	ring := pal.text.Alpha(0.1)
	if jumped && !at {
		ring = pal.accent
	}
	card := ui.Column(c.Key("thread")).Margin(commentAbove, commentMargin, commentBelow, commentMargin).
		Padding(commentPadTop, commentPadRight, commentPadBottom, commentPadLeft).Gap(threadBetween).Radius(8).
		Background(pal.chip).InsetShadow(0, 0, 0, 1+b2f(jumped && !at), ring).AlignItems(ui.Stretch)
	if at {
		card.Margin(0)
	}
	card.Children(func() {
		if jumped && !at {
			revealSpot(c, g, card)
		}
		if at {
			a.threadPlace(c, pal, g, t)
		}
		if g.threadFolded(t) {
			first := t.comments[0]
			row := ui.Row(c.Key("folded")).Height(threadHeadH).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip(locales.PrsShowResolved())
			if row.Clicked() {
				g.opened["t:"+t.id] = true
			}
			row.Children(func() {
				ui.Text(c, "✓ "+locales.PrsResolved()).FontSize(11.5).FontWeight(600).TextColor(added).SingleLine().Shrink(0)
				ui.Text(c, first.Author).FontSize(12).FontWeight(600).TextColor(pal.textMuted).SingleLine().Shrink(0)
				ui.Text(c, firstLine(first.Body)).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0)
				ui.Spacer(c)
				ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(pal.textFaint).Shrink(0)
			})
			return
		}
		for i, cm := range t.comments {
			col := ui.Column(c.Key(cm.ID)).AlignItems(ui.Stretch)
			if i > 0 {
				// Apart from what it answers: a rule across, and in.
				col.Padding(replyPad, 0, 0, replyIndent)
				col.Draw(func(p *ui.Painter, r ui.Rect) {
					p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, pal.text.Alpha(0.1), 0)
					p.Fill(ui.Rect{X: r.X + 2, Y: r.Y + replyPad + 3, W: 2, H: r.H - replyPad - 6}, pal.text.Alpha(0.14), 1)
				})
			}
			col.Children(func() {
				ui.Row(c).Height(threadHeadH).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, cm.Author).FontSize(12).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
					ui.Text(c, ago(cm.Created*1000, now)).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0)
					ui.Spacer(c)
					if cm.URL != "" {
						url := cm.URL
						iconButton(c, "open", linkIcon, locales.PrsOpen(), pal, func() { openURL(url) })
					}
				})
				// Of the line height MeasureText measures it at, for the
				// room threadHeight leaves.
				ui.Text(c, cm.Body).FontSize(13).TextColor(pal.text).Selectable()
			})
		}
		if t.id == "" {
			return
		}
		ui.Row(c.Key("foot")).Height(threadFootH).Gap(6).AlignItems(ui.Center).Children(func() {
			label := locales.PrsResolve()
			if t.resolved {
				label = locales.PrsUnresolve()
			}
			b := ui.ButtonBase(c.Key("resolve")).Height(threadFootH).Padding(0, 8).Radius(6).AlignItems(ui.Center).
				TextColor(pal.textMuted).Label(label).InsetShadow(0, 0, 0, 1, pal.text.Alpha(0.12))
			if b.Hovered() || b.Pressed() {
				b.Background(pal.rowHover).TextColor(pal.text)
			}
			b.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(550).SingleLine() })
			if b.Clicked() {
				a.resolveThread(g, t.id, !t.resolved)
			}
			if t.resolved {
				iconButton(c, "fold", chevronUpIcon, locales.PrsHideResolved(), pal, func() { delete(g.opened, "t:"+t.id) })
			}
		})
	})
}

// threadPlace is where a thread of the conversation is: its file and
// lines, or its file, which a click goes to on the Files tab; or, of lines
// that changed since, where it was, outdated.
func (a *App) threadPlace(c *ui.Context, pal *palette, g *prPage, t *prThread) {
	first := t.comments[0]
	spot := prSpot{path: first.Path, line: t.line, thread: first.ID}
	place := first.Path + ":" + itoa(t.start+1)
	if t.line > t.start {
		place += "–" + itoa(t.line+1)
	}
	switch {
	case t.file:
		place, spot.line = locales.ReviewOnFile(first.Path), -1
	case t.outdated:
		ui.Text(c, locales.PrsOutdated(first.Path+":"+itoa(max(t.originally, 1)))).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…")
		return
	}
	row := ui.Row(c.Key("place")).Gap(5).AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip(locales.PrsShowInFiles())
	if row.Clicked() {
		a.goToSpot(g, spot)
	}
	row.Children(func() {
		ui.Icon(c, docIcon).Size(11, 11).TextColor(pal.accent).Shrink(0)
		ui.Text(c, place).FontSize(11.5).FontWeight(550).TextColor(pal.accent).SingleLine().Ellipsis("…").MinWidth(0)
	})
}

// resolveThread resolves a thread on GitHub, or makes it unresolved again,
// showing it so at once, and as it was when that fails.
func (a *App) resolveThread(g *prPage, id string, resolve bool) {
	cl := a.hostOf(g.pane).cl
	if cl == nil || g.res == nil {
		return
	}
	v := g.res
	set := func(to bool) {
		for i := range v.LineComments {
			if v.LineComments[i].Thread == id {
				v.LineComments[i].Resolved = to
			}
		}
	}
	set(resolve)
	// Resolved, it folds.
	delete(g.opened, "t:"+id)
	p := proto.PRThreadParams{Pane: g.pane, Remote: g.remote, Number: g.pr.Number, Thread: id, Resolve: resolve}
	go func() {
		err := cl.Call("pr.thread", p, nil)
		a.update(func() {
			if err != nil {
				set(!resolve)
				a.toast(locales.PrsResolveFailed(err.Error()))
			}
		})
	}()
}
