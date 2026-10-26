package main

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"kero/internal/github"
	"kero/internal/proto"

	"kero/locales"
)

// New pull requests: every pr-poll-interval minutes, the app asks each
// host for the open pull requests of its projects' repositories on GitHub
// (those the user's review is asked of, or all, as pr-notify says), and
// notifies of each it didn't see before. What it saw is kept in the
// prefs, by host and repository, so that a restart notifies of none
// again; the first poll of a repository only learns what is there.

// prPoll is the polling's: when it last asked, and that it is asking.
type prPoll struct {
	last time.Time
	busy bool
}

// watchPulls polls for new pull requests as often as the settings say.
func (a *App) watchPulls() {
	for range time.Tick(30 * time.Second) {
		a.update(a.pollPulls)
	}
}

// polled is a repository's open pull requests, as one of a project's
// folders on a host listed them.
type polled struct {
	h       *host
	project string
	res     proto.PRList
}

// pollPulls asks for the projects' pull requests, when it is time.
func (a *App) pollPulls() {
	if a.set.PRNotify == "off" || a.poll.busy || time.Since(a.poll.last) < time.Duration(a.set.PRPollMinutes)*time.Minute {
		return
	}
	type job struct {
		h            *host
		project, dir string
	}
	var jobs []job
	asked := map[string]bool{}
	for _, p := range a.st.Projects {
		h := a.hostOf(p.ID)
		if h == nil || h.cl == nil || p.Dir == "" || asked[h.name+"|"+p.Dir] {
			continue
		}
		asked[h.name+"|"+p.Dir] = true
		jobs = append(jobs, job{h, p.ID, p.Dir})
	}
	if len(jobs) == 0 {
		return
	}
	a.poll.busy, a.poll.last = true, time.Now()
	filter, all := github.FilterReview, a.set.PRNotify == "all"
	if all {
		filter = github.FilterAll
	}
	go func() {
		var got []polled
		for _, j := range jobs {
			var res proto.PRList
			if err := j.h.cl.Call("pr.list", proto.PRListParams{Dir: j.dir, Filter: filter, Viewer: all}, &res); err != nil || res.Error != "" {
				continue
			}
			got = append(got, polled{j.h, j.project, res})
		}
		a.update(func() {
			a.poll.busy = false
			a.pullsPolled(got, all)
		})
	}()
}

// pullsPolled notifies of the pull requests not seen before, but the
// user's own, and returns them; it keeps those seen.
func (a *App) pullsPolled(got []polled, all bool) (fresh []proto.PullRequest) {
	if a.prefs.PRSeen == nil {
		a.prefs.PRSeen = map[string][]int{}
	}
	done := map[string]bool{}
	for _, g := range got {
		slug := pullsSlug(&g.res)
		key := g.h.name + "|" + strings.ToLower(slug)
		if slug == "" || done[key] {
			continue
		}
		done[key] = true
		seen, known := a.prefs.PRSeen[key]
		var now []int
		for _, pr := range g.res.PRs {
			now = append(now, pr.Number)
			if !known || slices.Contains(seen, pr.Number) || all && strings.EqualFold(pr.Author, g.res.Viewer) {
				continue
			}
			fresh = append(fresh, pr)
			a.notifyPull(g, slug, pr, all)
		}
		a.prefs.PRSeen[key] = now
	}
	a.savePrefs()
	return fresh
}

// notifyPull notifies of a new pull request, which a click shows in the
// Review view of a session of its project.
func (a *App) notifyPull(g polled, slug string, pr proto.PullRequest, all bool) {
	title := locales.PrsNotifyReview(slug)
	if all {
		title = locales.PrsNotifyNew(slug)
	}
	if g.h.remote() {
		title = g.h.name + ": " + title
	}
	body := "#" + strconv.Itoa(pr.Number) + " " + pr.Title + " · " + pr.Author
	remote := g.res.Remote
	showNote(mygo.NotificationOptions{ID: "pr/" + g.h.name + "/" + slug + "/" + strconv.Itoa(pr.Number), Title: title, Body: body, Silent: !a.set.NotifySound}, func() {
		a.update(func() {
			a.showPull(g.project, remote, slug, pr)
			a.showWindow()
		})
	})
}

// showPull shows a pull request in the Review view of a session of its
// project, the one shown last; else its page opens on GitHub.
func (a *App) showPull(project, remote, slug string, pr proto.PullRequest) {
	var s *proto.Session
	if cur := a.session(a.selected); cur != nil && cur.Project == project {
		s = cur
	}
	for i := range a.st.Sessions {
		if s == nil && a.st.Sessions[i].Project == project {
			s = &a.st.Sessions[i]
		}
	}
	if s == nil {
		openURL(pr.URL)
		return
	}
	pane := s.PaneByID(a.focusOf(s))
	if pane == nil || pane.Git == nil {
		openURL(pr.URL)
		return
	}
	a.selectSession(s.ID)
	a.prefs.Side, a.prefs.SideView = sideReview, sideReview
	a.openPRPage(pane, remote, slug, pr)
}
