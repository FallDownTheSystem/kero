package main

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"kero/internal/ama"
	"kero/internal/cli"
	"kero/internal/proto"

	"kero/locales"
)

// Ama, after amagaeru, the Japanese tree frog, is the app's helper agent:
// an agent session of this machine's kerod, of no project, in a folder of
// its own (internal/ama), that starts and manages the user's sessions for
// them. The sidebar lists it apart: the frog button at its bottom opens a
// popover with its terminal, which it keeps running as it closes, as the
// quick terminal keeps its session.

const amaAction = "ama.toggle"

// amaFrog is Ama's icon, as given: a frog's face, in currentColor.
var amaFrog = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="0.833"
stroke-linecap="round" stroke-linejoin="round">
  <g>
    <path d="M3 6.75 C2.25 7.5 2 8.5 2 9.5 C2 12 4.5 13.5 8 13.5 C11.5 13.5 14 12 14 9.5 C14 8.5 13.75 7.5 13 6.75"/>
    <path d="M7 5.6 C7.65 5.4 8.35 5.4 9 5.6"/>
    <circle cx="4.75" cy="5.25" r="2.25"/>
    <circle cx="11.25" cy="5.25" r="2.25"/>
    <circle cx="4.75" cy="5.25" r="0.55" fill="currentColor" stroke="none"/>
    <circle cx="11.25" cy="5.25" r="0.55" fill="currentColor" stroke="none"/>
    <path d="M5.25 10.25 C6.75 11.5 9.25 11.5 10.75 10.25"/>
  </g>
</svg>`))

// amaState is Ama's popover's, on the main thread.
type amaState struct {
	open     bool
	creating bool   // its session is being made
	err      string // why it has none
	// focus asks for the keyboard in its terminal once it shows; claimed
	// is the pane whose size the popover took last.
	focus   bool
	claimed string
	// history are Ama's past conversations, as History lists them.
	history []proto.Conversation
}

// The popover's size, which its terminal takes.
const (
	amaW = 620
	amaH = 460
)

// amaSession is Ama's session, of this machine's kerod; nil while it has
// none.
func (a *App) amaSession() *proto.Session {
	for i := range a.st.Sessions {
		if s := &a.st.Sessions[i]; s.Ama && a.hostOf(s.ID) == a.local() {
			return s
		}
	}
	return nil
}

// isAma reports whether a session is Ama's, which the sidebar, Needs You
// and notifications leave out.
func isAma(s *proto.Session) bool { return s != nil && s.Ama }

func (a *App) toggleAma() {
	if a.ama.open {
		a.closeAma()
	} else {
		a.openAma()
	}
}

// openAma opens Ama's popover, starting Ama first if it has no session.
func (a *App) openAma() {
	m := &a.ama
	m.open, m.focus, m.err, m.claimed = true, true, "", ""
	if a.prefs.SidebarHidden {
		a.prefs.SidebarHidden = false
		a.savePrefs()
	}
	if a.amaSession() == nil {
		a.startAma()
	}
	a.loadAmaHistory()
	a.reportView()
	if a.win != nil {
		a.showWindow()
	}
}

// closeAma closes the popover: Ama runs on, for the next time.
func (a *App) closeAma() {
	a.ama.open = false
	a.focusTerminal()
	a.reportView()
}

// startAma starts a session of Ama, which replaces the one there is: kerod
// keeps one. Kero's skill is installed for Ama's agent first, as kero
// skill setup installs it, so that Ama has it from its first turn.
func (a *App) startAma() {
	m := &a.ama
	h := a.local()
	if m.creating {
		return
	}
	if h.cl == nil || h.conn != connReady {
		m.err = locales.StatusDisconnected()
		return
	}
	m.creating, m.err = true, ""
	cl, command := h.cl, ama.Command(a.set)
	go func() {
		installAmaSkill(command)
		var s proto.Session
		err := cl.Call("create", proto.CreateParams{Ama: true, Name: "Ama", Command: command, Cols: 80, Rows: 24}, &s)
		a.update(func() {
			m.creating = false
			if err != nil {
				m.err = err.Error()
				return
			}
			a.addPending(h, s)
			m.focus, m.claimed = true, ""
			a.loadAmaHistory()
		})
	}()
}

// installAmaSkill installs Kero's skill where Ama's agent reads it, unless
// it is there and current: Claude Code's, the other agents', or both for
// a command it can't tell, as an alias.
func installAmaSkill(command []string) {
	kind := ama.SkillAgent(command)
	for _, st := range cli.SkillStates() {
		if (kind == "" || kind == st.Agent) && st.State != "current" {
			cli.InstallSkill(st.Agent)
		}
	}
}

// loadAmaHistory asks for the conversations of Ama's folder, the last to
// run first, for History.
func (a *App) loadAmaHistory() {
	h := a.local()
	if h.cl == nil {
		return
	}
	cl := h.cl
	go func() {
		var list []proto.Conversation
		if cl.Call("conversations", nil, &list) != nil {
			return
		}
		list = slices.DeleteFunc(list, func(cv proto.Conversation) bool { return !ama.Within(cv.Dir) })
		a.update(func() { a.ama.history = list })
	}()
}

// resumeAma goes on with one of Ama's conversations, as Ama's session.
func (a *App) resumeAma(cv proto.Conversation) {
	h := a.local()
	if h.cl == nil {
		return
	}
	cl := h.cl
	go func() {
		installAmaSkill(ama.Command(a.set))
		var s proto.Session
		err := cl.Call("conversation.resume", proto.ConversationParams{Agent: cv.Agent, ID: cv.ID, Ama: true, Cols: 80, Rows: 24}, &s)
		a.update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			a.addPending(h, s)
			a.ama.focus, a.ama.claimed = true, ""
		})
	}()
}

// clearAma clears the screen of Ama's terminal, as ⌘K does a pane's.
func (a *App) clearAma() {
	if s := a.amaSession(); s != nil {
		if p := s.FocusPane(); p != nil {
			if e := a.terms[p.ID]; e != nil && e.stream != nil {
				st := e.stream
				go st.Clear(proto.ClearScreen)
			}
		}
	}
}

// amaButton is the frog at the bottom of the sessions sidebar, which opens
// and closes Ama's popover, with a dot while Ama waits for the user; the
// popover, while open, is above it.
func (a *App) amaButton(c *ui.Context, pal *palette) {
	s := a.amaSession()
	tip := a.tip(locales.AmaTitle(), amaAction)
	b := ui.ButtonBase(c.Key("ama")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.AmaTitle())
	pointerTooltip(c, b, "ama-tip", tip+" · "+locales.AmaAbout())
	switch {
	case b.Pressed():
		b.Background(pal.rowHover.Alpha(0.12))
	case b.Hovered() || a.ama.open:
		b.Background(pal.rowHover)
		b.TextColor(pal.text)
	}
	waiting := s != nil && s.Status == proto.Waiting
	unread := s != nil && s.Unread && !a.ama.open
	b.Children(func() {
		ui.Icon(c, amaFrog).Size(17, 17)
		if waiting || unread {
			dot := pal.attention
			if !waiting {
				dot = pal.textMuted
			}
			ui.Box(c.Key("dot")).Absolute().Top(3).Right(3).Size(6, 6).Radius(3).Background(dot).PassThrough()
		}
	})
	if waiting {
		b.Description(locales.AmaWaiting())
	}
	if b.Clicked() {
		a.toggleAma()
	}
	if a.ama.open {
		a.amaPopover(c, pal, b)
	}
}

// amaPopover is Ama's popover, above the frog: a header with New Chat,
// History, Clear and Close, and Ama's terminal. It closes as the frog, its
// Close or the shortcut asks, not as the user presses Escape, which is the
// agent's.
func (a *App) amaPopover(c *ui.Context, pal *palette, anchor ui.Element) {
	s := a.amaSession()
	ui.Overlay(c, func() {
		at, self := ui.AnchorTopLeft, ui.AnchorBottomLeft
		if a.sideLeft() {
			at, self = ui.AnchorTopRight, ui.AnchorBottomRight
		}
		panel := ui.Column(c.Key("ama-popover")).AttachTo(anchor, at, self).Margin(0, 0, 6, 0).Size(amaW, amaH).Radius(12).
			Background(pal.popover).Clip().Label(locales.AmaTitle())
		floating(panel, pal)
		panel.Children(func() {
			ui.Row(c.Key("ama-header")).Height(38).Shrink(0).Padding(0, 6, 0, 12).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, amaFrog).Size(16, 16).TextColor(pal.textMuted)
				ui.Text(c, locales.AmaTitle()).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine()
				if s != nil && s.Status != proto.Idle {
					statusIcon(c, pal, s.Status, false, 12)
				}
				ui.Spacer(c)
				iconButton(c, "ama-new", plusIcon, locales.AmaNewChat(), pal, a.startAma)
				a.amaHistoryButton(c, pal)
				iconButton(c, "ama-clear", eraserIcon, locales.AmaClear(), pal, a.clearAma)
				iconButton(c, "ama-close", closeIcon, a.tip(locales.CommonClose(), amaAction), pal, a.closeAma)
			})
			ui.Box(c).Height(1).Shrink(0).Background(pal.text.Alpha(0.07))
			ui.Box(c.Key("ama-body")).Grow(1).MinHeight(0).Padding(6, 4, 6, 12).Background(lookFor(c).term.Background).Children(func() { a.amaBody(c, pal, s) })
		})
	})
}

// amaBody is Ama's terminal, or what keeps it.
func (a *App) amaBody(c *ui.Context, pal *palette, s *proto.Session) {
	m := &a.ama
	var p *proto.Pane
	if s != nil {
		p = s.FocusPane()
	}
	var e *termEntry
	if p != nil {
		if e = a.terms[p.ID]; e == nil {
			a.ensureTerm(p.ID)
			e = a.terms[p.ID]
		}
	}
	switch {
	case e != nil && e.term != nil:
		ref := a.paneRef(p.ID)
		termAccess(terminal.View(c.Key("ama-term"), e.term).Fill().Bind(ref), e.term, ref.Focused(c))
		a.touch(p.ID)
		// It takes the popover's size, which the window's panes don't
		// give it, once a pane of it shows.
		if m.claimed != p.ID && e.stream != nil {
			m.claimed = p.ID
			go e.stream.Claim()
		}
		if m.focus {
			m.focus = false
			ref.Focus()
		}
	case m.err != "" || e != nil && e.err != "":
		detail := m.err
		if e != nil && e.err != "" {
			detail = e.err
		}
		a.message(c, pal, locales.AmaCantStart(), detail, locales.CommonTryAgain(), func() {
			m.err = ""
			if p != nil {
				a.dropTerm(p.ID)
				a.ensureTerm(p.ID)
			} else {
				a.startAma()
			}
		})
	default:
		ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
		c.After(100 * time.Millisecond)
	}
}

// amaHistoryButton lists Ama's past conversations, the last first, to go
// on with one: the one running is checked.
func (a *App) amaHistoryButton(c *ui.Context, pal *palette) {
	b := ui.ButtonBase(c.Key("ama-history")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.AmaHistory()).Tooltip(locales.AmaHistory())
	if b.Hovered() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, clockIcon).Size(16, 16) })
	b.Menu(func(m *ui.Menu) {
		if len(a.ama.history) == 0 {
			m.Item(locales.AmaNoHistory()).Disabled(true)
			return
		}
		now := time.Now()
		for i, cv := range a.ama.history {
			if i >= 30 {
				break
			}
			label := cv.Label()
			if label == "" {
				label = cv.ID
			}
			label = menuName(strings.TrimSpace(label)) + " · " + proto.Ago(cv.Updated, now)
			running := cv.Session != ""
			if m.Item(label).Checked(running).Disabled(running).Chosen() {
				a.resumeAma(cv)
			}
		}
	})
	if b.Clicked() {
		a.loadAmaHistory()
	}
}

// reviewFromAgent starts an agent's review of a pull request, as kero pr
// review asks (prReviewAction): in a new session, in a worktree of its
// branch, of the repository of args' dir, with the prompt and the agent
// Settings › Reviews give it, as the Review tab's Review does; with
// guide, its review guide instead.
func (a *App) reviewFromAgent(h *host, args map[string]string) {
	cl := h.cl
	n, _ := strconv.Atoi(args["number"])
	dir, remote, guide := args["dir"], args["remote"], args["guide"] == "true"
	if cl == nil || n <= 0 || dir == "" {
		return
	}
	go func() {
		var res proto.PRList
		err := cl.Call("pr.list", proto.PRListParams{Dir: dir, Remote: remote}, &res)
		if err == nil && res.Error != "" {
			err = errorString(res.Error)
		}
		var pr proto.PullRequest
		if err == nil {
			i := slices.IndexFunc(res.PRs, func(pr proto.PullRequest) bool { return pr.Number == n })
			if i < 0 {
				err = errorString(locales.AmaNoPR(n))
			} else {
				pr = res.PRs[i]
			}
		}
		slug := ""
		for _, r := range res.Repos {
			if r.Remote == res.Remote {
				slug = r.Slug
			}
		}
		var got proto.PRFetched
		if err == nil {
			err = cl.Call("pr.fetch", proto.PRFetchParams{Dir: dir, Remote: res.Remote, Number: n, Head: pr.Head, CrossRepo: pr.CrossRepo}, &got)
		}
		a.update(func() {
			if err != nil {
				a.toast(locales.PrsFetchFailed(err.Error()))
				return
			}
			setup := a.reviewSetup(slug)
			tpl := setup.Prompt
			if guide {
				tpl = a.guidePrompt()
			}
			ticket := prTicket(pr)
			prompt := strings.TrimSpace(fillReview(tpl, pr, ticket))
			p := proto.CreateParams{Dir: dir, Name: "#" + strconv.Itoa(n), Command: reviewArgs(setup.Command, prompt, pr, ticket),
				Review: &proto.PRReviewRef{Slug: slug, Number: n}, Worktree: got.Branch, Base: got.Base, PushRemote: got.PushRemote, PushBranch: got.PushBranch}
			a.createThen(h, p, "", false, func(ns proto.Session) {
				if len(ns.Panes) == 0 {
					return
				}
				pane := ns.Panes[0].ID
				var params any = proto.PRReviewParams{Op: "start", Pane: pane, Remote: res.Remote, Number: n, Agent: pane}
				method := "pr.review"
				if guide {
					params, method = proto.PRGuideParams{Op: "start", Pane: pane, Remote: res.Remote, Number: n, Agent: pane}, "pr.guide"
				}
				go cl.Call(method, params, nil)
				if guide {
					a.toast(locales.PrsGuideStarted(n))
				} else {
					a.toast(locales.PrsReviewStarted(n))
				}
			})
		})
	}()
}

// errorString is an error of a message.
type errorString string

func (e errorString) Error() string { return string(e) }
