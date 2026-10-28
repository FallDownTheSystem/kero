package main

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/client"
	"kero/internal/commitmsg"
	"kero/internal/github"
	"kero/internal/proto"
	"kero/internal/settings"

	"kero/locales"
)

// The Review view: the open pull requests of the repository of the pane
// the user is in, as the GitHub CLI lists them on its host, each of which
// an agent reviews (startReview), and New Pull Request, a form of one of
// the pane's branch, with what it would bring.

// sideReview is the Review view's id, as prefs.Side holds it.
const sideReview = "review"

// pullsState is the Review view's.
type pullsState struct {
	// key is the repository and the list asked for (pullsKey), res what
	// came of it and err why nothing did; gen tells the fetch asked for
	// last from those still under way.
	key     string
	res     *proto.PRList
	err     string
	busy    bool
	fetched time.Time
	gen     int
	filter  string // github.Filter*
	remote  string // the remote asked for, "" for the branch's push remote
	list    ui.ListState
	// drafts are the draft reviews of the list's repository, by their
	// pull requests' numbers, of draftsKey's host and repository.
	drafts    map[int]proto.PRReviewDraft
	draftsKey string
	// form is the New Pull Request form, and page a pull request's page,
	// shown in the list's place while set.
	form *prForm
	page *prPage
}

// prForm is a pull request being made, of a pane's branch.
type prForm struct {
	pane, root   string
	remote, base string // as asked; "" for the draft's own
	draft        *proto.PRDraft
	err          string
	busy         bool
	gen          int
	title, body  string
	// branches are the base's remote's branches, which the base picker
	// chooses the base from.
	branches []baseBranch
	picker   basePicker
	// writing is the id of the run writing the title and the description,
	// "" for none; writeErr why the last wrote none.
	writing  string
	writeErr string
	creating bool
	files    map[string]*sideFile
	scroll   ui.ScrollState
}

// pullsRefresh is how long a list stays before the view asks for it anew.
const pullsRefresh = 2 * time.Minute

func pullsKey(h *host, root, remote, filter string) string {
	return h.name + "\x00" + root + "\x00" + remote + "\x00" + filter
}

func (a *App) pullsView(c *ui.Context, pal *palette, s *proto.Session, width float32) {
	ps := &a.side.pulls
	pane, _ := a.changesPanes(s)
	if pane == nil || pane.Git == nil {
		a.sideMessage(c, pal, locales.PrsNoRepo(), locales.PrsNoRepoDetail())
		return
	}
	if f := ps.form; f != nil && f.pane == pane.ID && f.root == pane.Git.Root {
		a.prFormView(c, pal, s, pane, width)
		return
	}
	ps.form = nil
	if g := ps.page; g != nil && g.pane == pane.ID && g.root == pane.Git.Root {
		a.prPageView(c, pal, s, pane, width)
		return
	}
	ps.page = nil
	h := a.hostOf(pane.ID)
	key := pullsKey(h, pane.Git.Root, ps.remote, ps.filter)
	switch {
	case ps.busy && ps.key == key:
	case ps.key != key, c.Now().Sub(ps.fetched) > pullsRefresh:
		a.fetchPulls(pane, key)
	}
	res := ps.res
	if ps.key != key {
		res = nil
	}

	ui.Column(c.Key("pulls-bar")).Shrink(0).Padding(2, 12, 8, 12).Gap(8).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Height(28).Gap(4).AlignItems(ui.Center).Children(func() {
			a.repoButton(c, pal, res)
			ui.Spacer(c)
			if ps.busy {
				ui.Spinner(c).Size(12, 12).Margin(0, 4, 0, 0)
			}
			iconButton(c, "refresh", reloadIcon, locales.PrsRefresh(), pal, func() { a.fetchPulls(pane, key) })
			iconButton(c, "setup", gearIcon, locales.PrsSetup(), pal, func() {
				if slug := pullsSlug(res); slug != "" {
					a.openReviewSetup(slug)
				}
			})
			if sizedButton(c, "new", locales.PrsNew(), "", true, 26, 6, pal) {
				a.openPRForm(pane)
			}
		})
		ui.Row(c).Children(func() {
			segmented(c, "filter", &ps.filter, []string{github.FilterAll, github.FilterMine, github.FilterReview},
				[]string{locales.PrsAll(), locales.PrsMine(), locales.PrsReviewRequested()})
		})
	})

	switch {
	case ps.err != "" && res == nil:
		a.sideMessage(c, pal, locales.PrsCantList(), ps.err)
		return
	case res == nil:
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	case res.Error != "":
		a.sideMessage(c, pal, locales.PrsCantList(), res.Error)
		return
	case len(res.PRs) == 0:
		a.sideMessage(c, pal, locales.PrsNone(), locales.PrsNoneDetail(pullsSlug(res)))
		return
	}
	ls := &ps.list
	ls.Key = func(i int) any { return res.PRs[i].Number }
	ui.List(c.Key("prs"), ls, len(res.PRs), func(i int) {
		a.pullRow(c, pal, s, pane, res, res.PRs[i])
	}).Grow(1).MinHeight(0).Padding(0, 6, 16, 6)
}

// pullsSlug is the repository a list is of, as OWNER/NAME.
func pullsSlug(res *proto.PRList) string {
	if res == nil {
		return ""
	}
	for _, r := range res.Repos {
		if r.Remote == res.Remote {
			return r.Slug
		}
	}
	return ""
}

// repoButton is the repository the list is of, whose menu picks another
// of the repository's remotes on GitHub.
func (a *App) repoButton(c *ui.Context, pal *palette, res *proto.PRList) {
	ps := &a.side.pulls
	label := pullsSlug(res)
	if label == "" {
		label = locales.PrsRepo()
	}
	b := ui.ButtonBase(c.Key("repo")).Height(26).Padding(0, 8).Gap(6).Radius(6).AlignItems(ui.Center).MinWidth(0).Shrink(1).
		TextColor(pal.text).Label(locales.PrsRepo()).Tooltip(locales.PrsRepo())
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover)
	}
	b.Children(func() {
		ui.Icon(c, prIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, label).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…").MinWidth(0)
		ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(pal.textFaint).Shrink(0)
	})
	b.Menu(func(m *ui.Menu) {
		if res == nil {
			return
		}
		for _, r := range res.Repos {
			if m.Item(r.Slug + " (" + r.Remote + ")").Checked(r.Remote == res.Remote).Chosen() {
				ps.remote = r.Remote
			}
		}
		m.Separator()
		if m.Item(locales.PrsOpenRepo()).Chosen() {
			for _, r := range res.Repos {
				if r.Remote == res.Remote {
					openURL(r.URL + "/pulls")
				}
			}
		}
	})
}

// fetchPulls asks the pane's host for the list key names.
func (a *App) fetchPulls(pane *proto.Pane, key string) {
	ps := &a.side.pulls
	cl := a.hostOf(pane.ID).cl
	if cl == nil {
		return
	}
	ps.gen++
	gen := ps.gen
	if ps.key != key {
		ps.res, ps.err = nil, ""
	}
	ps.key, ps.busy, ps.fetched = key, true, time.Now()
	p := proto.PRListParams{Pane: pane.ID, Remote: ps.remote, Filter: ps.filter}
	go func() {
		var res proto.PRList
		err := cl.Call("pr.list", p, &res)
		a.update(func() {
			if ps.gen != gen {
				return
			}
			ps.busy = false
			if err != nil {
				ps.res, ps.err = nil, err.Error()
				return
			}
			ps.res, ps.err = &res, ""
			a.fetchDrafts(a.hostOf(p.Pane), pullsSlug(&res))
		})
	}()
}

// fetchDrafts asks a host for the draft reviews of a repository's pull
// requests, which the list marks. A kerod older than pr.drafts has none
// to show.
func (a *App) fetchDrafts(h *host, slug string) {
	ps := &a.side.pulls
	if h.cl == nil || slug == "" {
		return
	}
	key := h.name + "\x00" + strings.ToLower(slug)
	cl := h.cl
	go func() {
		var drs []proto.PRReviewDraft
		err := cl.Call("pr.drafts", proto.PRDraftsParams{Slug: slug}, &drs)
		a.update(func() {
			if err != nil {
				return
			}
			ps.drafts, ps.draftsKey = map[int]proto.PRReviewDraft{}, key
			for _, dr := range drs {
				ps.drafts[dr.Number] = dr
			}
		})
	}()
}

// listDraft returns the draft review of a pull request of the list, which
// h shows, and whether the list marks it: while it has comments or a
// summary, or its agent reviews; false for none. A draft whose comments
// were all deleted marks nothing.
func (a *App) listDraft(h *host, res *proto.PRList, number int) (proto.PRReviewDraft, bool) {
	ps := &a.side.pulls
	if ps.draftsKey != h.name+"\x00"+strings.ToLower(pullsSlug(res)) {
		return proto.PRReviewDraft{}, false
	}
	dr, ok := ps.drafts[number]
	return dr, ok && (len(dr.Comments) > 0 || strings.TrimSpace(dr.Summary) != "" || a.reviewing(dr))
}

// reviewing tells a draft whose agent works on it: busy, or not yet sent
// its prompt.
func (a *App) reviewing(dr proto.PRReviewDraft) bool {
	p := a.paneByID(dr.Agent)
	if p == nil {
		return false
	}
	return p.Status == proto.Working || slices.ContainsFunc(a.launches, func(l *agentLaunch) bool { return l.pane == dr.Agent })
}

// pullRow is an open pull request: its number and title, its author, its
// branches, its checks and its review, which a click opens on GitHub, with
// Review, which has an agent review it; or, while it has a draft review,
// the review's pill under them, and the row marked.
func (a *App) pullRow(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, res *proto.PRList, pr proto.PullRequest) {
	dr, drafted := a.listDraft(a.hostOf(pane.ID), res, pr.Number)
	row := ui.Row(c).FillWidth().Padding(6, 6, 6, 6).Gap(8).AlignItems(ui.Center).Radius(6).Cursor(ui.CursorPointer).Tooltip(pr.URL)
	switch {
	case row.Hovered():
		row.Background(pal.rowHover.Alpha(0.6))
	case drafted:
		row.Background(pal.accent.Alpha(0.07))
	}
	if drafted {
		row.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + 6, W: 3, H: r.H - 12}, pal.accent, 1.5)
		})
	}
	slug := pullsSlug(res)
	launch := func(mode string) { a.askReview(s, pane, res.Remote, slug, pr, mode) }
	row.ContextMenu(func(m *ui.Menu) {
		reviewItems(m, launch)
		m.Separator()
		if m.Item(locales.PrsOpen()).Chosen() {
			openURL(pr.URL)
		}
		if m.Item(locales.PrsCopyLink()).Chosen() {
			c.WriteClipboard(pr.URL)
		}
	})
	if row.Clicked() {
		a.openPRPage(pane, res.Remote, slug, pr)
	}
	row.Children(func() {
		icon, col := prIcon, pal.accent
		if pr.Draft {
			icon, col = draftPRIcon, pal.textFaint
		}
		ui.Icon(c, icon).Size(14, 14).TextColor(col).Shrink(0).AlignSelf(ui.Start).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "#"+strconv.Itoa(pr.Number)).FontSize(12.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Shrink(0)
				ui.Text(c, pr.Title).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				meta := pr.Author + " · " + pr.Head + " → " + pr.Base
				if pr.CrossRepo && pr.HeadOwner != "" {
					meta = pr.Author + " · " + pr.HeadOwner + ":" + pr.Head + " → " + pr.Base
				}
				ui.Text(c, meta).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				a.prBadges(c, pal, pr)
			})
			// A line of its own, which takes nothing of the title's.
			if drafted {
				ui.Row(c).Margin(3, 0, 1, 0).Children(func() { a.draftPill(c, pal, dr) })
			}
		})
		if row.Hovered() {
			iconButton(c, "open", linkIcon, locales.PrsOpen(), pal, func() { openURL(pr.URL) })
		}
		if !drafted {
			a.reviewButton(c, pal, launch)
		}
	})
}

// draftPill is a pull request's draft review, in the list: Reviewing while
// its agent works, Review Ready once it is done, In Review for the user's
// own; and how many comments it has so far. A click on it goes to the row,
// which opens the pull request.
func (a *App) draftPill(c *ui.Context, pal *palette, dr proto.PRReviewDraft) {
	working := a.reviewing(dr)
	n := len(dr.Comments)
	pill := ui.Row(c.Key("draft")).Height(22).Padding(0, 9, 0, 8).Gap(6).Radius(11).AlignItems(ui.Center).MinWidth(0)
	label, fg := locales.PrsReviewReady(), ui.Hex("#ffffff")
	switch {
	case working:
		label, fg = locales.PrsReviewing(), pal.accent
		pill.Background(pal.accent.Alpha(0.16)).InsetShadow(0, 0, 0, 1, pal.accent.Alpha(0.45))
	case dr.Agent == "":
		label = locales.PrsInReview()
		pill.Background(pal.accent)
	default:
		pill.Background(pal.accent)
	}
	pill.TextColor(fg).Children(func() {
		switch {
		case working:
			ui.Spinner(c).Size(10, 10)
		case dr.Agent != "":
			ui.Text(c, "✓").FontSize(11.5).FontWeight(700).SingleLine()
		}
		ui.Text(c, label).FontSize(11.5).FontWeight(650).SingleLine()
		ui.Box(c).Size(1, 11).Background(fg.Alpha(0.4))
		ui.Icon(c, commentIcon).Size(11, 11).Shrink(0)
		ui.Text(c, locales.PrsDraftComments(n)).FontSize(11.5).FontWeight(600).FontFeatures("tnum").SingleLine().Ellipsis("…").MinWidth(0)
	})
}

// prBadges are a pull request's draft, checks and review, as words.
func (a *App) prBadges(c *ui.Context, pal *palette, pr proto.PullRequest) {
	added, removed, _ := a.diffColors(c, pal)
	badge := func(text string, col ui.Color) {
		ui.Text(c, text).FontSize(11).FontWeight(550).TextColor(col).SingleLine().Shrink(0)
	}
	if pr.Draft {
		badge(locales.PrsDraft(), pal.textFaint)
	}
	switch pr.Checks {
	case "success":
		badge("✓ "+locales.PrsChecksPassed(), added)
	case "failure":
		badge("✕ "+locales.PrsChecksFailed(), removed)
	case "pending":
		badge("● "+locales.PrsChecksPending(), pal.attention)
	}
	switch pr.Review {
	case "APPROVED":
		badge(locales.PrsApproved(), added)
	case "CHANGES_REQUESTED":
		badge(locales.PrsChangesRequested(), removed)
	}
}

// Where startReview has the agent review: in a new session, in a worktree
// of the pull request's branch; in the agent of the pane's worktree; or
// in a new session in the pane's worktree.
const (
	reviewWorktree = "worktree"
	reviewHere     = "here"
	reviewSession  = "session"
)

// reviewItems are the items of a menu that has an agent review a pull
// request, in each place.
func reviewItems(m *ui.Menu, launch func(mode string)) {
	if m.Item(locales.PrsReviewWorktree()).Chosen() {
		launch(reviewWorktree)
	}
	if m.Item(locales.PrsReviewHere()).Chosen() {
		launch(reviewHere)
	}
	if m.Item(locales.PrsReviewSession()).Chosen() {
		launch(reviewSession)
	}
}

// DefaultReviewPrompt is what a review agent is asked when its
// repository's setup has no prompt.
const DefaultReviewPrompt = "Review pull request #{number}, \"{title}\" ({url}), of {branch} into {base}.\n\n" +
	"First review it in full: read it with gh pr view {number} and gh pr diff {number}, and the code around what it changes. Look for bugs, risky changes and missing tests.\n\n" +
	"Then write the comments you would leave for its author. Check each one against the code, and drop any that isn't true or that this change doesn't cause. " +
	"Only then add them to the draft review with kero pr comment, each on the lines of the pull request's head it is about, and a short summary with kero pr summary (see kero's skill). " +
	"Don't post anything: I read the draft, and post it myself."

// defaultReviewCommand starts the review agent when the repository's
// setup has no command.
const defaultReviewCommand = "claude"

// reviewSetup is how an agent reviews a repository's pull requests, the
// defaults in what the user left empty.
func (a *App) reviewSetup(slug string) settings.ReviewSetup {
	r := a.set.Reviews[strings.ToLower(slug)]
	if strings.TrimSpace(r.Prompt) == "" {
		r.Prompt = DefaultReviewPrompt
	}
	if strings.TrimSpace(r.Command) == "" {
		r.Command = defaultReviewCommand
	}
	return r
}

// ticketPattern is an issue's key, as Jira's: PROJ-123.
var ticketPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,9}-[0-9]+\b`)

// prTicket is the issue a pull request is of, as the first key its title,
// its description or its branch names; "" for none.
func prTicket(pr proto.PullRequest) string {
	for _, s := range []string{pr.Title, pr.Body, strings.ToUpper(pr.Head)} {
		if k := ticketPattern.FindString(s); k != "" {
			return k
		}
	}
	return ""
}

// fillReview fills a review prompt's placeholders in with a pull
// request's.
func fillReview(tpl string, pr proto.PullRequest, ticket string) string {
	head := pr.Head
	if pr.CrossRepo && pr.HeadOwner != "" {
		head = pr.HeadOwner + ":" + pr.Head
	}
	return strings.NewReplacer(
		"{number}", strconv.Itoa(pr.Number), "{url}", pr.URL, "{title}", pr.Title, "{branch}", head,
		"{base}", pr.Base, "{author}", pr.Author, "{ticket}", ticket,
	).Replace(tpl)
}

// prReview is a review being asked for: the prompt, filled in for the
// pull request, which the user edits before it starts.
type prReview struct {
	open         bool
	mode         string
	session      string // the session the view is of, beside which a new one goes
	pane         string
	remote, slug string
	pr           proto.PullRequest
	template     string
	ticket       string
	asksTicket   bool // the prompt has {ticket}, which nothing named
	prompt       string
	edited       bool // the user changed the prompt, which the ticket no longer fills in
}

// askReview opens the review's prompt, filled in, for the user to start.
func (a *App) askReview(s *proto.Session, pane *proto.Pane, remote, slug string, pr proto.PullRequest, mode string) {
	setup := a.reviewSetup(slug)
	ticket := prTicket(pr)
	a.prReview = &prReview{open: true, mode: mode, session: s.ID, pane: pane.ID, remote: remote, slug: slug, pr: pr,
		template: setup.Prompt, ticket: ticket, asksTicket: strings.Contains(setup.Prompt, "{ticket}") && ticket == "",
		prompt: fillReview(setup.Prompt, pr, ticket)}
}

// prReviewView is the review's prompt, before it starts.
func (a *App) prReviewView(c *ui.Context, pal *palette) {
	w := a.prReview
	if w == nil {
		return
	}
	start := false
	ui.DialogBase(c.Key("pr-review"), &w.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.PrsReviewTitle(w.pr.Number)
		floating(panel.Width(520).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).Label(title)
		ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(pal.text)
		where := map[string]string{reviewWorktree: locales.PrsReviewWorktreeDetail(), reviewHere: locales.PrsReviewHereDetail(), reviewSession: locales.PrsReviewSessionDetail()}[w.mode]
		ui.Text(c, w.pr.Title).FontSize(13).FontWeight(550).TextColor(pal.textMuted).SingleLine().Ellipsis("…")
		ui.Text(c, where).FontSize(12.5).TextColor(pal.textFaint).LineHeight(1.4)
		if w.asksTicket {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(6, 0, 0, 0).Children(func() {
				ui.Text(c, locales.PrsTicket()).FontSize(12.5).TextColor(pal.textMuted).Shrink(0)
				in := textField(c, "ticket", &w.ticket, "PROJ-123", 0, pal)
				in.AutoFocus()
				if in.Changed() && !w.edited {
					w.prompt = fillReview(w.template, w.pr, strings.TrimSpace(w.ticket))
				}
			})
		}
		frame := ui.Row(c.Key("prompt")).AlignItems(ui.Start).Cursor(ui.CursorText).Margin(6, 0, 0, 0)
		var in ui.Element
		frame.Children(func() {
			in = ui.TextAreaBase(c.Key("in"), &w.prompt).Grow(1).MinWidth(0).MinHeight(120).MaxHeight(320).Padding(8).FontSize(13).
				Label(locales.PrsReviewPrompt())
			if in.Changed() {
				w.edited = true
			}
			in.OnShortcut(ui.Cmd, ui.KeyEnter, func() { start = true })
			if !w.asksTicket {
				in.AutoFocus()
			}
		})
		fieldLook(c, frame, in.Focused(), pal).Padding(0)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(14, 0, 0, 0).Children(func() {
			setup := ui.ButtonBase(c.Key("setup")).Height(26).Padding(0, 6).Radius(6).AlignItems(ui.Center).TextColor(pal.textMuted).Label(locales.PrsSetup())
			if setup.Hovered() {
				setup.TextColor(pal.text)
			}
			setup.Children(func() { ui.Text(c, locales.PrsSetup()).FontSize(12.5).SingleLine() })
			if setup.Clicked() {
				w.open = false
				a.openReviewSetup(w.slug)
			}
			ui.Spacer(c)
			if button(c, "cancel", locales.CommonCancel(), "esc", false, pal) {
				w.open = false
			}
			if button(c, "ok", locales.PrsStart(), commitKeys(), true, pal) {
				start = true
			}
		})
	})
	if start && strings.TrimSpace(w.prompt) != "" {
		w.open = false
		a.startReview(w)
	}
	if !w.open {
		a.prReview = nil
	}
}

// startReview has an agent review the pull request, where w.mode says.
func (a *App) startReview(w *prReview) {
	s := a.session(w.session)
	pane := a.paneByID(w.pane)
	if s == nil || pane == nil || pane.Git == nil {
		return
	}
	h := a.hostOf(pane.ID)
	prompt := strings.TrimSpace(w.prompt)
	// The draft the agent writes its comments to, which kero pr in its
	// pane finds by the pane.
	draft := func(agent string, then func()) {
		cl := h.cl
		if cl == nil {
			return
		}
		p := proto.PRReviewParams{Op: "start", Pane: pane.ID, Remote: w.remote, Number: w.pr.Number, Agent: agent}
		go func() {
			err := cl.Call("pr.review", p, nil)
			a.update(func() {
				if err != nil {
					a.toast(locales.PrsDraftFailed(err.Error()))
					return
				}
				if then != nil {
					then()
				}
			})
		}()
	}
	if w.mode == reviewHere {
		ap := a.agentPane(pane)
		if ap == nil {
			a.toast(locales.ReviewNoAgent())
			return
		}
		draft(ap.ID, func() { a.sendPrompt(ap.ID, prompt) })
		return
	}
	setup := a.reviewSetup(w.slug)
	p := proto.CreateParams{Project: s.Project, After: s.ID, Dir: pane.Git.Root, Cols: s.Cols, Rows: s.Rows,
		Name: "#" + strconv.Itoa(w.pr.Number), Command: commitmsg.Words(setup.Command)}
	// The new session works in the background, the user where they were:
	// the list marks the pull request while its draft review is written,
	// and once it is ready.
	then := func(ns proto.Session) {
		if len(ns.Panes) == 0 {
			return
		}
		agent := ns.Panes[0].ID
		draft(agent, nil)
		a.watchLaunch(h, agent, prompt)
		a.toast(locales.PrsReviewStarted(w.pr.Number))
	}
	if w.mode == reviewSession {
		a.createThen(h, p, s.Project, false, then)
		return
	}
	cl := h.cl
	if cl == nil {
		return
	}
	fp := proto.PRFetchParams{Pane: pane.ID, Remote: w.remote, Number: w.pr.Number, Head: w.pr.Head, CrossRepo: w.pr.CrossRepo}
	a.toast(locales.PrsFetching(w.pr.Number))
	go func() {
		var got proto.PRFetched
		err := cl.Call("pr.fetch", fp, &got)
		a.update(func() {
			if err != nil {
				a.toast(locales.PrsFetchFailed(err.Error()))
				return
			}
			p.Worktree, p.Base, p.PushRemote, p.PushBranch = got.Branch, got.Base, got.PushRemote, got.PushBranch
			a.createThen(h, p, s.Project, false, then)
		})
	}()
}

// paneByID returns a pane of the state, nil for none.
func (a *App) paneByID(id string) *proto.Pane {
	if s := a.sessionOfPane(id); s != nil {
		return s.PaneByID(id)
	}
	return nil
}

// agentLaunch is a prompt waiting for the agent a new session starts:
// pasted in once the agent is up and quiet.
type agentLaunch struct {
	host     *host
	pane     string
	prompt   string
	started  time.Time
	deadline time.Time
	late     bool // past its deadline, its prompt for the clipboard
}

// launchWait is how long a launch waits for its agent before it gives up,
// and leaves the prompt on the clipboard.
const launchWait = 90 * time.Second

func (a *App) watchLaunch(h *host, pane, prompt string) {
	now := time.Now()
	a.launches = append(a.launches, &agentLaunch{host: h, pane: pane, prompt: prompt, started: now, deadline: now.Add(launchWait)})
	if a.launchStop == nil {
		stop := make(chan struct{})
		a.launchStop = stop
		go func() {
			for {
				select {
				case <-stop:
					return
				case <-time.After(time.Second):
				}
				a.update(func() {
					a.checkLaunches(time.Now())
					if len(a.launches) == 0 && a.launchStop == stop {
						close(stop)
						a.launchStop = nil
					}
				})
			}
		}()
	}
}

// pollLaunches checks the launches as the view draws, and gives a late
// one's prompt to the clipboard. A ticker of watchLaunch's checks them
// too, while the window draws nothing, hidden or minimized: the review
// goes on in the background.
func (a *App) pollLaunches(c *ui.Context) {
	if len(a.launches) == 0 {
		return
	}
	a.checkLaunches(c.Now())
	keep := a.launches[:0]
	for _, l := range a.launches {
		if !l.late {
			keep = append(keep, l)
			continue
		}
		c.WriteClipboard(l.prompt)
		a.toast(locales.PrsAgentLate())
	}
	a.launches = keep
	if len(keep) > 0 {
		c.After(300 * time.Millisecond)
	}
}

// checkLaunches pastes each launch's prompt once its agent is ready: the
// pane runs an agent Kero knows, which is idle and has drawn nothing for
// a moment; else, of an agent Kero doesn't know, once the program it runs
// has drawn and gone quiet a while. One past its deadline is late.
func (a *App) checkLaunches(now time.Time) {
	keep := a.launches[:0]
	for _, l := range a.launches {
		p := a.paneByID(l.pane)
		var quiet time.Duration
		drew := false
		if p != nil && p.Activity > 0 {
			last := time.UnixMilli(p.Activity)
			quiet, drew = now.Sub(last), last.After(l.started)
		}
		switch {
		case l.late:
			keep = append(keep, l)
		case p == nil && now.After(l.deadline):
		case p != nil && p.Agent != "" && p.Status == proto.Idle && drew && quiet > 1200*time.Millisecond,
			p != nil && p.Agent == "" && now.Sub(l.started) > 8*time.Second && drew && quiet > 2*time.Second:
			a.sendPrompt(l.pane, l.prompt)
		case now.After(l.deadline):
			l.late = true
			keep = append(keep, l)
		default:
			keep = append(keep, l)
		}
	}
	a.launches = keep
}

// sendPrompt pastes a prompt in an agent's pane, then Enter, as Send to
// Agent does.
func (a *App) sendPrompt(pane, prompt string) {
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	go func() {
		if err := a.submit(cl, pane, prompt); err != nil {
			a.update(func() { a.toast(locales.ReviewSendFailed(err.Error())) })
		}
	}()
}

// How long submit leaves a program to take a paste before Enter, and to
// start working on it before Enter again. Claude Code takes what comes
// soon after a paste for more of it: an Enter too soon is a new line, and
// the prompt stays unsent.
const (
	pasteSettle = 500 * time.Millisecond
	enterWait   = 2 * time.Second
)

// submit pastes text in a pane, then Enter, apart from the paste, and
// returns, its caller waiting on a goroutine of its own. To an agent Kero
// knows that stays idle after, it sends Enter again, twice at most, as an
// Enter it took into the paste leaves the prompt unsent. It pastes rather
// than types the text, whose newlines would send it line by line.
func (a *App) submit(cl *client.Client, pane, text string) error {
	var res any
	if err := cl.Call("send", proto.SendParams{ID: pane, Data: text, Paste: true, Submit: true}, &res); err != nil {
		return err
	}
	if res == proto.Submitted {
		return nil // kerod presses Enter, the window shown or not
	}
	// An older kerod's, which pastes alone.
	time.Sleep(pasteSettle)
	if err := cl.Call("send", proto.SendParams{ID: pane, Data: "\r"}, nil); err != nil {
		return err
	}
	go func() {
		for range 2 {
			time.Sleep(enterWait)
			idle := make(chan bool, 1)
			a.update(func() {
				p := a.paneByID(pane)
				idle <- p != nil && p.Agent != "" && p.Status == proto.Idle
			})
			select {
			case still := <-idle:
				if !still {
					return
				}
			case <-time.After(5 * time.Second):
				return
			}
			if cl.Call("send", proto.SendParams{ID: pane, Data: "\r"}, nil) != nil {
				return
			}
		}
	}()
	return nil
}

// prSetup is a repository's review setup being edited.
type prSetup struct {
	open            bool
	slug            string
	command, prompt string
}

// openReviewSetup edits how an agent reviews a repository's pull requests.
func (a *App) openReviewSetup(slug string) {
	r := a.set.Reviews[strings.ToLower(slug)]
	prompt := r.Prompt
	if strings.TrimSpace(prompt) == "" {
		prompt = DefaultReviewPrompt
	}
	a.prSetup = &prSetup{open: true, slug: slug, command: r.Command, prompt: prompt}
}

func (a *App) prSetupView(c *ui.Context, pal *palette) {
	w := a.prSetup
	if w == nil {
		return
	}
	save := false
	ui.DialogBase(c.Key("pr-setup"), &w.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.PrsSetupTitle(w.slug)
		floating(panel.Width(560).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).Label(title)
		ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(pal.text)
		ui.Text(c, locales.PrsSetupDetail()).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.4)
		ui.Text(c, locales.PrsSetupCommand()).FontSize(12.5).FontWeight(600).TextColor(pal.text).Margin(8, 0, 0, 0)
		ui.Row(c).Children(func() {
			textField(c, "command", &w.command, defaultReviewCommand, 0, pal).Font("monospace").Grow(1)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
			ui.Text(c, locales.PrsSetupPrompt()).FontSize(12.5).FontWeight(600).TextColor(pal.text)
			ui.Spacer(c)
			if strings.TrimSpace(w.prompt) != DefaultReviewPrompt {
				iconButton(c, "default", returnIcon, locales.PrsUseDefault(), pal, func() { w.prompt = DefaultReviewPrompt })
			}
		})
		frame := ui.Row(c.Key("prompt")).AlignItems(ui.Start).Cursor(ui.CursorText)
		var in ui.Element
		frame.Children(func() {
			in = ui.TextAreaBase(c.Key("in"), &w.prompt).Grow(1).MinWidth(0).MinHeight(140).MaxHeight(320).Padding(8).FontSize(13).
				Label(locales.PrsSetupPrompt())
			in.OnShortcut(ui.Cmd, ui.KeyEnter, func() { save = true })
		})
		fieldLook(c, frame, in.Focused(), pal).Padding(0)
		ui.Text(c, locales.PrsSetupPlaceholders()).FontSize(12).TextColor(pal.textFaint).LineHeight(1.4)
		ui.Row(c).Gap(8).Justify(ui.End).Margin(14, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.CommonCancel(), "esc", false, pal) {
				w.open = false
			}
			if button(c, "ok", locales.CommonSave(), commitKeys(), true, pal) {
				save = true
			}
		})
	})
	if save {
		w.open = false
		set := a.set.Clone()
		if set.Reviews == nil {
			set.Reviews = map[string]settings.ReviewSetup{}
		}
		r := settings.ReviewSetup{Command: strings.TrimSpace(w.command), Prompt: strings.TrimSpace(w.prompt)}
		// The default's own words keep following Kero's default.
		if r.Prompt == DefaultReviewPrompt {
			r.Prompt = ""
		}
		key := strings.ToLower(w.slug)
		if r == (settings.ReviewSetup{}) {
			delete(set.Reviews, key)
		} else {
			set.Reviews[key] = r
		}
		a.applySettings(set)
	}
	if !w.open {
		a.prSetup = nil
	}
}
