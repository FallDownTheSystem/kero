package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// A session's pull request (proto.Session.PR), which kerod finds by the
// session's branch, or which the user links by hand, and watches for the
// session's agent (internal/daemon/prwatch.go): the session's menu shows
// and unlinks it, its tooltip says how it stands, and a pull request's
// page links it to the session it is shown in.

// prLinkTip is a line of the session's tooltip of its pull request, as
// "Pull request #42 · checks failed"; "" for none.
func prLinkTip(s *proto.Session) string {
	l := s.PR
	if l == nil {
		return ""
	}
	tip := locales.PrLinkTip(l.Number)
	switch {
	case l.Mergeable == "CONFLICTING":
		tip += " · " + locales.PrLinkConflicts()
	case l.Checks == "failure":
		tip += " · " + locales.PrLinkChecksFailed()
	case l.Checks == "pending":
		tip += " · " + locales.PrLinkChecksRunning()
	case l.Checks == "success":
		tip += " · " + locales.PrLinkChecksPassed()
	}
	return tip
}

// prLinkMenu adds the session's pull request to its menu: Show, in the
// Review view, and Unlink.
func (a *App) prLinkMenu(m *ui.Menu, s *proto.Session) {
	l := s.PR
	if l == nil {
		return
	}
	id, link := s.ID, *l
	m.Separator()
	if m.Item(locales.PrLinkShow(l.Number)).Chosen() {
		a.defer_(func() { a.showLinkedPR(id, link) })
	}
	if m.Item(locales.PrLinkUnlink(l.Number)).Chosen() {
		a.call(id, "pr.link", proto.PRLinkParams{Session: id, Unlink: true}, nil)
	}
}

// showLinkedPR shows a session's pull request in its Review view, else
// on GitHub.
func (a *App) showLinkedPR(sid string, l proto.SessionPR) {
	pr := proto.PullRequest{Number: l.Number, Title: l.Title, URL: l.URL, Head: l.Head, Base: l.Base, Checks: l.Checks}
	s := a.session(sid)
	if s == nil {
		openURL(l.URL)
		return
	}
	pane := s.PaneByID(a.focusOf(s))
	if pane == nil || pane.Git == nil || l.Remote == "" {
		openURL(l.URL)
		return
	}
	a.selectSession(sid)
	a.showTab(sideReview)
	a.openPRPage(pane, l.Remote, l.Slug, pr)
}

// prLinkButton, on a pull request's page, links it to the session it is
// shown in, or, linked already, unlinks it.
func (a *App) prLinkButton(c *ui.Context, pal *palette, s *proto.Session, g *prPage) {
	if s == nil {
		return
	}
	linked := s.PR != nil && s.PR.Number == g.pr.Number && (g.repoSlug() == "" || strings.EqualFold(s.PR.Slug, g.repoSlug()))
	tip := locales.PrLinkLink()
	if linked {
		tip = locales.PrLinkLinked()
	}
	b := ui.ButtonBase(c.Key("pr-link")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(tip).Tooltip(tip)
	if linked {
		b.TextColor(pal.accent)
	}
	if b.Pressed() {
		b.Background(pal.rowHover.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal.rowHover)
		if !linked {
			b.TextColor(pal.text)
		}
	}
	b.Children(func() { ui.Icon(c, clockIcon).Size(15, 15) })
	if b.Clicked() {
		id := s.ID
		p := proto.PRLinkParams{Session: id, Number: g.pr.Number, URL: g.pr.URL, Remote: g.remote, Unlink: linked}
		a.call(id, "pr.link", p, nil)
	}
}
