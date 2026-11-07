package daemon

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"kero/internal/detect"
	"kero/internal/git"
	"kero/internal/github"
	"kero/internal/proto"
	"kero/internal/settings"
)

// A session's pull request (proto.Session.PR): kerod finds the open one
// of the session's branch, once the branch is pushed, and links it; or
// the user, or an agent, links one by hand (pr.link), which no change of
// branch replaces. While the session is open and its pull request is,
// kerod watches it, and wakes the session's agent, by typing into its
// pane as kero prompt does, when its checks fail, and, as pr-watch-wake
// says, when they pass, when comments or reviews come, and when it can't
// merge for a conflict. Comments alone wake the agent prWatchMaxWakes
// times at most, so that a bot that answers the agent's answers doesn't
// keep it going. The agent hears once it's at its prompt; while it works,
// or asks the user something, what it is to hear waits. A session with
// no agent has the user notified instead.

const (
	prWatchTick     = 5 * time.Second  // how often the watch looks at what is due
	prWatchStep     = time.Minute      // between looks at an open pull request
	prFindStep      = 3 * time.Minute  // between looks for a branch's pull request
	prFindBurstStep = 30 * time.Second // the same, just after a push
	prFindBurst     = 4                // looks that soon after a push
	prWatchMaxWakes = 10               // wakes for comments alone
	prWatchTimeout  = 45 * time.Second
	prExcerpt       = 280 // a comment's text the agent is told, in runes
)

// prState is a session's pull request, kept with it: Link, as the state
// tells it; Branch, the local branch kerod found it by, "" for one linked
// by hand; Off, the pull request the user unlinked ("slug#number"), which
// isn't linked again by its branch; Watch, what the agent was told.
type prState struct {
	Link   *proto.SessionPR `json:"link,omitempty"`
	Branch string           `json:"branch,omitempty"`
	Off    string           `json:"off,omitempty"`
	Watch  prWatch          `json:"watch"`

	// When kerod last looked for the branch's pull request, for which
	// branch and pushed head, and how many quick looks are left.
	foundAt       time.Time
	foundFor, sig string
	burst         int
	// When the watch last looked, and that it is to look at once.
	watchedAt time.Time
	poked     bool
}

// prWatch is what the watch told the agent of its pull request: Head's
// checks (Told: "failure" or "success"); a conflict, until it merges
// again; and Since, unix seconds, the latest comment it has seen. Wakes
// counts its wakes for comments alone; Pending is what waits for the
// agent to be at its prompt. Started: it looked once, before which what
// is there is no news.
type prWatch struct {
	Started  bool     `json:"started,omitempty"`
	Head     string   `json:"head,omitempty"`
	Told     string   `json:"told,omitempty"`
	Conflict bool     `json:"conflict,omitempty"`
	Since    int64    `json:"since,omitempty"`
	Wakes    int      `json:"wakes,omitempty"`
	Pending  []string `json:"pending,omitempty"`
}

// link is the session's pull request, as the state tells it, nil for
// none. A link is never changed once made, only replaced.
func (ps *prState) link() *proto.SessionPR {
	if ps == nil {
		return nil
	}
	return ps.Link
}

func prKey(slug string, number int) string {
	return strings.ToLower(slug) + "#" + strconv.Itoa(number)
}

// prWatcher runs the watch: a loop that looks every prWatchTick at what
// is due, and that poke wakes.
type prWatcher struct {
	d    *Daemon
	wake chan struct{}

	mu      sync.Mutex
	viewers map[string]viewer // by host: who gh is signed in as
}

type viewer struct {
	login string
	at    time.Time
}

func newPRWatcher(d *Daemon) *prWatcher {
	return &prWatcher{d: d, wake: make(chan struct{}, 1), viewers: map[string]viewer{}}
}

// poke has the watch look at once.
func (w *prWatcher) poke() {
	if w == nil {
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *prWatcher) loop() {
	t := time.NewTicker(prWatchTick)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-w.wake:
		case <-w.d.quit:
			return
		}
		w.look(time.Now())
	}
}

// prSession is what the watch takes of a session, under d.mu.
type prSession struct {
	id    string
	ps    *prState
	git   *proto.GitInfo // of the pane the agent is in, else the focused one
	agent *pane          // the agent's pane, nil for none
	ready bool           // the agent is at its prompt
}

// prSessionLocked takes what the watch needs of a session; d.mu is held.
func prSessionLocked(s *session) prSession {
	ps := prSession{id: s.id, ps: s.pr}
	order := slices.Clone(s.panes)
	// The focused pane first, as the one the user means.
	slices.SortStableFunc(order, func(a, b *pane) int {
		return boolInt(b.info.ID == s.focus) - boolInt(a.info.ID == s.focus)
	})
	var focusGit *proto.GitInfo
	for _, p := range order {
		p.mu.Lock()
		if focusGit == nil && p.info.Git != nil {
			focusGit = p.info.Git
		}
		if ps.agent == nil && p.term != nil && !p.closed && p.info.Agent != "" {
			ps.agent = p
			ps.git = p.info.Git
			// At its prompt, or done and waiting for the user to look; not
			// asking for a permission or an answer, which typing would give.
			st := p.info.Status
			ps.ready = st == proto.Idle || st == proto.Waiting && p.seen != detect.Blocked && !p.declared() && p.info.Kind == ""
		}
		p.mu.Unlock()
	}
	if ps.git == nil {
		ps.git = focusGit
	}
	return ps
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// look does what is due: tells the agents what waits for them, looks for
// the branches' pull requests, and watches those linked.
func (w *prWatcher) look(now time.Time) {
	wake := settings.Load().PRWatchWake
	d := w.d
	d.mu.Lock()
	list := make([]prSession, 0, len(d.sessions))
	for _, s := range d.sessions {
		list = append(list, prSessionLocked(s))
	}
	d.mu.Unlock()
	for _, s := range list {
		w.deliver(s)
		w.find(s, now)
		if wake != "off" {
			w.watch(s, wake, now)
		}
	}
}

// update changes a session's pull request under d.mu. fn makes the
// state, given the session's, nil for none; it reports whether it changed
// what is kept of it, which is then saved and told.
func (w *prWatcher) update(sid string, fn func(ps *prState) (*prState, bool)) {
	d := w.d
	d.mu.Lock()
	s := d.findSessionLocked(sid)
	if s == nil || s.id != sid {
		d.mu.Unlock()
		return
	}
	var changed bool
	s.pr, changed = fn(s.pr)
	d.mu.Unlock()
	if changed {
		d.save()
		d.changed()
	}
}

// find looks for the open pull request of the session's branch, when it
// has none linked by hand, as often as is due; and lets one go that was
// found by a branch the session left.
func (w *prWatcher) find(s prSession, now time.Time) {
	g := s.git
	var cur prState
	if s.ps != nil {
		w.d.mu.Lock()
		cur = *s.ps
		w.d.mu.Unlock()
	}
	if cur.Link != nil && cur.Link.Manual {
		return
	}
	branch := ""
	if g != nil && g.Branch != g.Default {
		branch = g.Branch
	}
	// One merged or closed is let go once the branch has another.
	if cur.Link != nil && cur.Branch == branch && cur.Link.Open() {
		return
	}
	if branch == "" {
		if cur.Link != nil {
			w.update(s.id, func(ps *prState) (*prState, bool) {
				if ps == nil || ps.Link == nil || ps.Link.Manual {
					return ps, false
				}
				ps.Link, ps.Branch, ps.Watch = nil, "", prWatch{}
				return ps, true
			})
		}
		return
	}
	// The head as pushed, which a push changes.
	sig := g.Upstream + "|"
	if g.Upstream != "" && g.Ahead == 0 {
		sig += g.Head
	}
	step := prFindStep
	if cur.burst > 0 {
		step = prFindBurstStep
	}
	pushed := sig != cur.sig && g.Upstream != ""
	// A branch never pushed has no pull request.
	due := g.Upstream != "" && (branch != cur.foundFor || pushed || now.Sub(cur.foundAt) >= step)
	if !due {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), prWatchTimeout)
	defer cancel()
	found, err := branchPR(ctx, g.Root, branch)
	if err != nil && !errors.Is(err, errNoPR) {
		w.d.log.Printf("pull request of %s: %v", branch, err)
	}
	w.update(s.id, func(ps *prState) (*prState, bool) {
		if ps == nil {
			ps = &prState{}
		}
		if ps.Link != nil && ps.Link.Manual {
			return ps, false
		}
		ps.foundAt, ps.foundFor = now, branch
		switch {
		case pushed:
			ps.sig, ps.burst = sig, prFindBurst
		case ps.burst > 0:
			ps.burst--
		}
		told := false
		if ps.Link != nil && ps.Branch != branch {
			ps.Link, ps.Branch, ps.Watch = nil, "", prWatch{}
			told = true
		}
		if found != nil && prKey(found.Slug, found.Number) != ps.Off {
			ps.Link, ps.Branch, ps.Watch, ps.poked = found, branch, prWatch{}, true
			told = true
			w.poke()
		}
		return ps, told
	})
}

var errNoPR = errors.New("no open pull request")

// branchPR returns the open pull request of a branch of the repository
// at root, as the branch's push remote has it, into the remote prList's
// would be of; errNoPR for none.
func branchPR(ctx context.Context, root, branch string) (*proto.SessionPR, error) {
	r, err := git.Open(ctx, root)
	if err != nil {
		return nil, err
	}
	repos, byName, err := gitHubRepos(ctx, r)
	if err != nil {
		return nil, err
	}
	remote, err := pickRemote(ctx, r, byName, repos, "")
	if err != nil {
		return nil, err
	}
	pushRemote, pushName, err := r.PushRemote(ctx, branch)
	if err != nil {
		return nil, err
	}
	push, ok := byName[pushRemote]
	if !ok {
		return nil, errNoPR
	}
	// The pull request may be into another remote's repository than the
	// one prList picks, as a fork's into its upstream: each is asked.
	order := []string{remote}
	for _, g := range repos {
		if g.Remote != remote {
			order = append(order, g.Remote)
		}
	}
	for _, name := range order {
		pr, err := github.ForBranch(ctx, r.Root, byName[name], push.Owner, pushName)
		if err != nil {
			return nil, err
		}
		if pr != nil {
			return &proto.SessionPR{Slug: byName[name].Slug(), Number: pr.Number, URL: pr.URL, Title: pr.Title, Remote: name,
				Head: pr.Head, Base: pr.Base, State: "OPEN", Checks: pr.Checks}, nil
		}
	}
	return nil, errNoPR
}

// watch looks at the session's open pull request, when it is due, and
// has the agent told what is new.
func (w *prWatcher) watch(s prSession, wake string, now time.Time) {
	var cur prState
	w.d.mu.Lock()
	if s.ps != nil {
		cur = *s.ps
		cur.Watch.Pending = slices.Clone(cur.Watch.Pending)
	}
	w.d.mu.Unlock()
	link := cur.Link
	if link == nil || !link.Open() || !cur.poked && now.Sub(cur.watchedAt) < prWatchStep {
		return
	}
	repo, ok := github.ParseSlug(link.Slug)
	if !ok {
		return
	}
	dir := ""
	if s.git != nil {
		dir = s.git.Root
	}
	ctx, cancel := context.WithTimeout(context.Background(), prWatchTimeout)
	defer cancel()
	got, err := github.Watch(ctx, dir, repo, link.Number)
	var lines []github.LineComment
	if err == nil && wake == "all" {
		lines, err = github.LineComments(ctx, dir, repo, link.Number)
	}
	if err != nil {
		w.d.log.Printf("watch of %s#%d: %v", link.Slug, link.Number, err)
		w.update(s.id, func(ps *prState) (*prState, bool) {
			if ps != nil && ps.Link == link {
				ps.watchedAt, ps.poked = now, false
			}
			return ps, false
		})
		return
	}
	me := w.viewer(ctx, dir, repo)
	next, watch, msg := watched(link, cur.Watch, got, lines, me, wake, now)
	w.update(s.id, func(ps *prState) (*prState, bool) {
		// Unlinked, or linked anew, while gh was asked.
		if ps == nil || ps.Link != link {
			return ps, false
		}
		ps.watchedAt, ps.poked = now, false
		pending := ps.Watch.Pending
		ps.Link, ps.Watch = next, watch
		ps.Watch.Pending = pending
		if msg != "" {
			ps.Watch.Pending = append(ps.Watch.Pending, msg)
		}
		return ps, true
	})
	if msg != "" {
		w.poke()
	}
}

// viewer is who gh is signed in as on the repository's host, asked once
// an hour; "" when unknown.
func (w *prWatcher) viewer(ctx context.Context, dir string, r github.Repo) string {
	w.mu.Lock()
	v, ok := w.viewers[r.Host]
	w.mu.Unlock()
	if ok && time.Since(v.at) < time.Hour {
		return v.login
	}
	login, err := github.Viewer(ctx, dir, r)
	if err != nil {
		return v.login
	}
	w.mu.Lock()
	w.viewers[r.Host] = viewer{login: login, at: time.Now()}
	w.mu.Unlock()
	return login
}

// watched returns the pull request as got found it, the watch's state
// after it, and what the agent is to hear of it, "" for nothing: of its
// checks, once all of its head's have finished, failing (always) or
// passing (with wake "all", unless they passed already when the watch
// started); of a conflict; and of comments and reviews since the last
// look but the user's own (me), which the agent may have written, with
// wake "all" and prWatchMaxWakes times at most when they are all there
// is to tell.
func watched(link *proto.SessionPR, wt prWatch, got github.Watched, lines []github.LineComment, me, wake string, now time.Time) (*proto.SessionPR, prWatch, string) {
	next := *link
	next.Title, next.URL, next.State, next.Base, next.Mergeable, next.Checked = got.Title, got.URL, got.State, got.Base, got.Mergeable, now.UnixMilli()
	if got.Head != "" {
		next.Head = got.Head
	}
	next.Checks, next.Failed = "", nil
	pending := false
	for _, c := range got.Checks {
		switch c.State {
		case "failure":
			next.Failed = append(next.Failed, c.Name)
		case "pending":
			pending = true
		}
	}
	switch {
	case len(next.Failed) > 0:
		next.Checks = "failure"
	case pending:
		next.Checks = "pending"
	case len(got.Checks) > 0:
		next.Checks = "success"
	}
	ref := fmt.Sprintf("pull request #%d (%s, %s)", got.Number, link.Slug, got.URL)
	var parts []string
	first := !wt.Started
	wt.Started = true
	if got.State != "OPEN" {
		return &next, wt, ""
	}

	// Its checks, once its head's have all finished.
	if got.HeadOID != wt.Head {
		wt.Head, wt.Told = got.HeadOID, ""
	}
	if !pending && len(got.Checks) > 0 {
		switch {
		case len(next.Failed) > 0 && wt.Told != "failure":
			wt.Told = "failure"
			parts = append(parts, fmt.Sprintf("Kero PR watch: checks failed on %s: %s. Look at what failed (gh pr checks %d --repo %s; gh run view <run-id> --log-failed) and, if this branch's changes caused it, fix it and push.",
				ref, strings.Join(next.Failed, ", "), got.Number, link.Slug))
		case len(next.Failed) == 0 && wt.Told != "success":
			wt.Told = "success"
			if wake == "all" && !first {
				parts = append(parts, fmt.Sprintf("Kero PR watch: all checks passed on %s.", ref))
			}
		}
	}

	// A conflict with its base.
	switch got.Mergeable {
	case "CONFLICTING":
		if !wt.Conflict {
			wt.Conflict = true
			parts = append(parts, fmt.Sprintf("Kero PR watch: %s has merge conflicts with %s. Fetch, bring %s in (rebase or merge, as the branch has been), resolve the conflicts, and push.",
				ref, got.Base, got.Base))
		}
	case "MERGEABLE":
		wt.Conflict = false
	}

	// Comments and reviews since the last look.
	type note struct {
		at   time.Time
		text string
	}
	var notes []note
	since := wt.Since
	saw := func(at time.Time, author, text string) {
		if at.Unix() > wt.Since {
			since = max(since, at.Unix())
			if !first && (me == "" || !strings.EqualFold(author, me)) {
				notes = append(notes, note{at, text})
			}
		}
	}
	for _, c := range got.Comments {
		saw(c.Created, c.Author, fmt.Sprintf("- %s commented: %s", c.Author, excerpt(c.Body)))
	}
	for _, v := range got.Reviews {
		// A review that only holds comments on lines says nothing of its
		// own; its comments come as lines'.
		if strings.TrimSpace(v.Body) == "" && v.State != "APPROVED" && v.State != "CHANGES_REQUESTED" {
			continue
		}
		text := fmt.Sprintf("- %s reviewed (%s)", v.Author, strings.ToLower(strings.ReplaceAll(v.State, "_", " ")))
		if strings.TrimSpace(v.Body) != "" {
			text += ": " + excerpt(v.Body)
		}
		saw(v.Submitted, v.Author, text)
	}
	for _, c := range lines {
		place := c.Path
		if c.Line > 0 {
			place += ":" + strconv.Itoa(c.Line)
		}
		saw(c.Created, c.Author, fmt.Sprintf("- %s on %s: %s", c.Author, place, excerpt(c.Body)))
	}
	wt.Since = since
	if len(notes) > 0 && wake == "all" && (len(parts) > 0 || wt.Wakes < prWatchMaxWakes) {
		slices.SortStableFunc(notes, func(a, b note) int { return a.at.Compare(b.at) })
		var b strings.Builder
		fmt.Fprintf(&b, "Kero PR watch: new comments on %s:\n", ref)
		for _, n := range notes {
			b.WriteString(n.text + "\n")
		}
		repo, _ := github.ParseSlug(link.Slug)
		fmt.Fprintf(&b, "Read them in full (gh pr view %d --repo %s --comments; gh api repos/%s/%s/pulls/%d/comments), and address those that ask for changes.",
			got.Number, link.Slug, repo.Owner, repo.Name, got.Number)
		if len(parts) == 0 {
			wt.Wakes++
			if wt.Wakes == prWatchMaxWakes {
				b.WriteString(" Kero won't wake you for comments alone on this pull request again.")
			}
		}
		parts = append(parts, b.String())
	}
	return &next, wt, strings.Join(parts, "\n\n")
}

// excerpt is a comment's text on one line, cut to prExcerpt.
func excerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > prExcerpt {
		s = string(r[:prExcerpt]) + "…"
	}
	return s
}

// deliver tells the session's agent what waits for it, once it is at
// its prompt; with no agent in the session, it notifies the user.
func (w *prWatcher) deliver(s prSession) {
	if s.ps == nil || s.agent != nil && !s.ready {
		return
	}
	w.d.mu.Lock()
	waits := len(s.ps.Watch.Pending) > 0
	w.d.mu.Unlock()
	if !waits {
		return
	}
	var msg string
	var link *proto.SessionPR
	w.update(s.id, func(ps *prState) (*prState, bool) {
		if ps != nil && len(ps.Watch.Pending) > 0 {
			msg, link = strings.Join(ps.Watch.Pending, "\n\n"), ps.Link
			ps.Watch.Pending = nil
			return ps, true
		}
		return ps, false
	})
	if msg == "" {
		return
	}
	if s.agent == nil {
		title := "Pull request"
		if link != nil {
			title = fmt.Sprintf("Pull request #%d", link.Number)
		}
		body, _, _ := strings.Cut(strings.TrimPrefix(msg, "Kero PR watch: "), "\n")
		w.d.notice(proto.Notice{Session: s.id, Title: title, Body: body})
		return
	}
	s.agent.paste(msg)
	go s.agent.submit(time.Now())
}

// prLink links a session to a pull request, or unlinks it (pr.link).
func (d *Daemon) prLink(p proto.PRLinkParams) (*proto.SessionPR, error) {
	d.mu.Lock()
	s := d.findSessionLocked(p.Session)
	var ps prSession
	if s != nil {
		ps = prSessionLocked(s)
	}
	d.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("no session %q", p.Session)
	}
	if p.Unlink {
		d.prw.update(ps.id, func(cur *prState) (*prState, bool) {
			if cur == nil || cur.Link == nil {
				return cur, false
			}
			cur.Off = prKey(cur.Link.Slug, cur.Link.Number)
			cur.Link, cur.Branch, cur.Watch = nil, "", prWatch{}
			return cur, true
		})
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), prWatchTimeout)
	defer cancel()
	root, branch := "", ""
	if ps.git != nil {
		root, branch = ps.git.Root, ps.git.Branch
	}
	var link *proto.SessionPR
	switch {
	case p.URL == "" && p.Number == 0:
		if branch == "" {
			return nil, errors.New("the session is on no branch: give the pull request's number or URL")
		}
		found, err := branchPR(ctx, root, branch)
		if errors.Is(err, errNoPR) {
			return nil, fmt.Errorf("%s has no open pull request", branch)
		}
		if err != nil {
			return nil, err
		}
		link = found
	default:
		var err error
		if link, err = prOfLink(ctx, root, p); err != nil {
			return nil, err
		}
		link.Manual = true
		branch = ""
	}
	d.prw.update(ps.id, func(cur *prState) (*prState, bool) {
		if cur == nil {
			cur = &prState{}
		}
		cur.Link, cur.Branch, cur.Off, cur.Watch, cur.poked = link, branch, "", prWatch{}, true
		return cur, true
	})
	d.prw.poke()
	return link, nil
}

// prOfLink returns the pull request a pr.link names, by its URL or its
// number of a remote of the repository at root.
func prOfLink(ctx context.Context, root string, p proto.PRLinkParams) (*proto.SessionPR, error) {
	var repo github.Repo
	number, remote := p.Number, ""
	if p.URL != "" {
		var ok bool
		if repo, number, ok = github.ParsePRURL(p.URL); !ok {
			return nil, fmt.Errorf("%q is no pull request's URL", p.URL)
		}
	}
	if root != "" {
		if r, err := git.Open(ctx, root); err == nil {
			if repos, byName, err := gitHubRepos(ctx, r); err == nil {
				if p.URL == "" {
					if remote, err = pickRemote(ctx, r, byName, repos, p.Remote); err != nil {
						return nil, err
					}
					repo = byName[remote]
				} else {
					for _, g := range repos {
						if strings.EqualFold(g.Slug, repo.Slug()) {
							remote = g.Remote
						}
					}
				}
			}
		}
	}
	if repo.Name == "" {
		return nil, errors.New("the session's repository has no remote on GitHub: give the pull request's URL")
	}
	got, err := github.Watch(ctx, root, repo, number)
	if err != nil {
		return nil, err
	}
	return &proto.SessionPR{Slug: repo.Slug(), Number: got.Number, URL: got.URL, Title: got.Title, Remote: remote, Head: got.Head, Base: got.Base, State: got.State}, nil
}

// prCreated links the session of the pane a pull request was made of to
// it, as soon as it is made.
func (d *Daemon) prCreated(pane string, t prTarget, url string) {
	_, number, ok := github.ParsePRURL(url)
	if !ok {
		return
	}
	d.mu.Lock()
	sid := ""
	if pn := d.findPaneLocked(pane); pn != nil {
		if s := d.sessionOfLocked(pn); s != nil {
			sid = s.id
		}
	}
	d.mu.Unlock()
	if sid == "" {
		return
	}
	link := &proto.SessionPR{Slug: t.repo.Slug(), Number: number, URL: url, Remote: t.remote, Head: t.pushName, Base: t.base, State: "OPEN"}
	d.prw.update(sid, func(cur *prState) (*prState, bool) {
		if cur == nil {
			cur = &prState{}
		}
		if cur.Link != nil && cur.Link.Manual {
			return cur, false
		}
		cur.Link, cur.Branch, cur.Off, cur.Watch, cur.poked = link, t.st.Branch, "", prWatch{}, true
		return cur, true
	})
	d.prw.poke()
}
