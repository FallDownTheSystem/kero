package main

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
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
// them. The sidebar lists it apart: the frog button at its bottom opens
// its window (ama_window.go) with its terminal, which it keeps running as
// the window closes, as the quick terminal keeps its session.

const amaAction = "ama.toggle"

// amaFrog is Ama's icon: the frog of Kero's logo, sitting with its feet in
// front of it, as one solid shape in currentColor. Lines of an outline
// frog blur together at 16 to 18 pixels; a solid one keeps its face. The
// mask cuts its eyes and smile out, keeps its belly half seen, and leaves
// a gap around each foot, so that the feet show in front of the body. The
// viewBox crops to the frog with a padding of 1 in 24 at each side, as
// the keylines of Kero's other icons have, so that it shows as large as
// they do at the same size.
var amaFrog = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="1.236 1.21 13.527 13.527" fill="none" stroke-width="0.7" stroke-linecap="round" stroke-linejoin="round">
  <mask id="ama-frog">
    <rect width="16" height="16" fill="white"/>
    <path d="M5.75 4.5a.9 .9 0 1 0-1.8 0a.9 .9 0 1 0 1.8 0Z M12.05 4.5a.9 .9 0 1 0-1.8 0a.9 .9 0 1 0 1.8 0Z" fill="black"/>
    <path d="M6.5 5.5 Q8 6.7 9.5 5.5" fill="none" stroke="black"/>
    <path d="M4.5 10.92 C5.04 9.27 6.3 8.25 8 8.25 C9.7 8.25 10.96 9.27 11.5 10.92 L11.5 13.4 H4.5 Z" fill="#8c8c8c"/>
    <path d="M2.55 13.4 C2.08 13.4 1.8 13.03 1.8 12.45 C1.8 11.45 2.65 10.85 3.55 10.85 C4.5 10.85 5.25 11.48 5.25 12.45 C5.25 13.03 4.97 13.4 4.5 13.4 Z M11.5 13.4 C11.03 13.4 10.75 13.03 10.75 12.45 C10.75 11.48 11.5 10.85 12.45 10.85 C13.35 10.85 14.2 11.45 14.2 12.45 C14.2 13.03 13.92 13.4 13.45 13.4 Z" fill="none" stroke="black"/>
    <path d="M2.55 13.4 C2.08 13.4 1.8 13.03 1.8 12.45 C1.8 11.45 2.65 10.85 3.55 10.85 C4.5 10.85 5.25 11.48 5.25 12.45 C5.25 13.03 4.97 13.4 4.5 13.4 Z M11.5 13.4 C11.03 13.4 10.75 13.03 10.75 12.45 C10.75 11.48 11.5 10.85 12.45 10.85 C13.35 10.85 14.2 11.45 14.2 12.45 C14.2 13.03 13.92 13.4 13.45 13.4 Z" fill="white"/>
  </mask>
  <path fill="currentColor" mask="url(#ama-frog)" d="M2.25 11.35 C1.95 10.4 1.8 9.6 2.05 8.3 C2.25 7.2 2.7 6.4 3.3 5.75 C2.9 5.1 3.05 4.15 3.55 3.5 C4.1 2.7 5.15 2.55 5.95 2.9 C6.38 3.1 6.7 3.5 6.88 3.9 Q8 3.55 9.12 3.9 C9.3 3.5 9.62 3.1 10.05 2.9 C10.85 2.55 11.9 2.7 12.45 3.5 C12.95 4.15 13.1 5.1 12.7 5.75 C13.3 6.4 13.75 7.2 13.95 8.3 C14.2 9.6 14.05 10.4 13.75 11.35 C13.6 12.5 13.1 13.4 12.2 13.4 H3.8 C2.9 13.4 2.4 12.5 2.25 11.35 Z M2.55 13.4 C2.08 13.4 1.8 13.03 1.8 12.45 C1.8 11.45 2.65 10.85 3.55 10.85 C4.5 10.85 5.25 11.48 5.25 12.45 C5.25 13.03 4.97 13.4 4.5 13.4 Z M11.5 13.4 C11.03 13.4 10.75 13.03 10.75 12.45 C10.75 11.48 11.5 10.85 12.45 10.85 C13.35 10.85 14.2 11.45 14.2 12.45 C14.2 13.03 13.92 13.4 13.45 13.4 Z"/>
</svg>`))

// amaState is Ama's window's, on the main thread.
type amaState struct {
	open     bool
	win      *mygo.Window   // made as it first opens
	placed   mygo.Rectangle // where the app put it last (amaMoved)
	save     *time.Timer    // saves where the user moved it
	creating bool           // its session is being made
	err      string         // why it has none
	// focus asks for the keyboard in its terminal once it shows; claimed
	// is the pane whose size the window took last.
	focus   bool
	claimed string
	// history are Ama's past conversations, as History lists them.
	history []proto.Conversation
	// confirm asks before History deletes chats, in Ama's window.
	confirm *confirmation
}

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

// openAma opens Ama's window, starting Ama first if it has no session.
func (a *App) openAma() {
	m := &a.ama
	m.open, m.focus, m.err, m.claimed = true, true, "", ""
	if a.amaSession() == nil {
		a.startAma()
	}
	a.loadAmaHistory()
	a.reportView()
	if a.win != nil {
		a.showWindow()
		a.showAmaWindow()
	}
}

// closeAma hides Ama's window: Ama runs on, for the next time, and the
// main window takes the keyboard back.
func (a *App) closeAma() {
	m := &a.ama
	if !m.open {
		return
	}
	m.open = false
	if m.win != nil {
		m.win.Hide()
		if a.winOpen {
			a.win.Focus()
		}
	}
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

// amaButton is the frog at the bottom of the sessions sidebar, which opens
// and closes Ama's window, with a dot while Ama waits for the user or has
// an answer the user hasn't read.
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
		ui.Icon(c, amaFrog).Size(16, 16)
		if waiting || unread {
			// An answer to read takes the theme's green, as the History
			// graph's lanes take the terminal's colors: a grey dot on the
			// grey frog was too easy to miss.
			dot := pal.attention
			if !waiting {
				dot = a.codeOf(c, pal).Palette[2]
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
	b.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.AmaResetWindow()).Disabled(a.prefs.AmaWindow == nil).Chosen() {
			a.resetAmaWindow()
		}
	})
}

// amaView is Ama's window: a header, which drags it, with New Chat,
// History and Close, over Ama's terminal. It closes as its Close,
// the system's close or Ama's shortcut asks, not as the user presses
// Escape, which is the agent's.
func (a *App) amaView(c *ui.Context) {
	a.looks()
	pal := paletteFor(c)
	c.SetTheme(borderless(c, pal))
	if mods, key, ok := a.actionKeys(amaAction); ok {
		c.OnShortcut(mods, key, a.closeAma)
	}
	s := a.amaSession()
	bar := c.TitleBar()
	col := ui.Column(c).Fill()
	backdrop(c, col)
	col.Children(func() {
		ui.Row(c.Key("ama-header")).Height(max(bar.Height, 38)).Shrink(0).Padding(0, 6+bar.Right, 0, 12+bar.Left).Gap(4).AlignItems(ui.Center).DragWindow().Children(func() {
			// The title keeps the icon-to-label gap of Kero's other rows; the
			// buttons after it sit closer, as a toolbar's do.
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, amaFrog).Size(16, 16).TextColor(pal.textMuted)
				ui.Text(c, locales.AmaTitle()).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine()
				if s != nil && s.Status != proto.Idle {
					statusIcon(c, pal, s.Status, false, 12)
				}
			})
			ui.Spacer(c)
			iconButton(c, "ama-new", plusIcon, locales.AmaNewChat(), pal, a.startAma)
			a.amaHistoryButton(c, pal)
			iconButton(c, "ama-close", closeIcon, a.tip(locales.CommonClose(), amaAction), pal, a.closeAma)
		})
		ui.Box(c).Height(1).Shrink(0).Background(pal.text.Alpha(0.07))
		ui.Box(c.Key("ama-body")).Grow(1).MinHeight(0).Padding(6, 4, 6, 12).Background(lookFor(c).term.Background).Children(func() { a.amaBody(c, pal, s) })
	})
	if a.ama.confirm != nil {
		confirmDialog(c, pal, &a.ama.confirm)
	}
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
		// It takes the window's size, which the main window's panes
		// don't give it, once a pane of it shows.
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
// on with one: the one running is checked. Below them, Delete Chat and
// Delete All Chats delete the others, once the user confirms.
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
			label := menuName(amaChatLabel(cv)) + " · " + proto.Ago(cv.Updated, now)
			running := cv.Session != ""
			if m.Item(label).Checked(running).Disabled(running).Chosen() {
				a.resumeAma(cv)
			}
		}
		// The chat that runs can't be deleted: its agent still writes it.
		past := slices.DeleteFunc(slices.Clone(a.ama.history), func(cv proto.Conversation) bool { return cv.Session != "" })
		m.Separator()
		m.Submenu(locales.AmaDeleteChat(), func(sm *ui.Menu) {
			for i, cv := range past {
				if i >= 30 {
					break
				}
				if sm.Item(menuName(amaChatLabel(cv)) + " · " + proto.Ago(cv.Updated, now)).Chosen() {
					a.askDeleteAmaChats([]proto.Conversation{cv})
				}
			}
		}).Disabled(len(past) == 0)
		if m.Item(locales.AmaDeleteAll()).Disabled(len(past) == 0).Chosen() {
			a.askDeleteAmaChats(past)
		}
	})
	if b.Clicked() {
		a.loadAmaHistory()
	}
}

// amaChatLabel names one of Ama's chats in History: its title, else its id.
func amaChatLabel(cv proto.Conversation) string {
	if label := strings.TrimSpace(cv.Label()); label != "" {
		return label
	}
	return cv.ID
}

// askDeleteAmaChats asks, in Ama's window, before deleting Ama's chats in
// list: one by its name, more by their count.
func (a *App) askDeleteAmaChats(list []proto.Conversation) {
	cf := &confirmation{open: true, action: locales.EndedDelete(), destroy: true, run: func() { a.deleteAmaChats(list) }}
	if len(list) == 1 {
		cf.title, cf.detail = locales.AmaDeleteChatTitle(amaChatLabel(list[0])), locales.AmaDeleteChatDetail()
	} else {
		cf.title, cf.detail = locales.AmaDeleteAllTitle(len(list)), locales.EndedDeleteAllDetail()
	}
	a.ama.confirm = cf
}

// deleteAmaChats has kerod forget Ama's chats, which leaves History, as
// the sidebar's Delete does an ended conversation; the agent keeps its
// own transcript of each.
func (a *App) deleteAmaChats(list []proto.Conversation) {
	h := a.local()
	gone := map[[2]string]bool{}
	for _, cv := range list {
		a.forgetConversation(h, cv)
		gone[[2]string{cv.Agent, cv.ID}] = true
	}
	// One pass after the calls: list may share history's array.
	a.ama.history = slices.DeleteFunc(a.ama.history, func(o proto.Conversation) bool { return gone[[2]string{o.Agent, o.ID}] })
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
			p := proto.CreateParams{Dir: dir, Name: reviewName(pr), Command: reviewArgs(setup.Command, prompt, pr, ticket),
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
