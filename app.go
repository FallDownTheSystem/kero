package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"kero/internal/client"
	"kero/internal/proto"
)

// connState is how the app stands with kerod.
type connState int

const (
	connConnecting connState = iota
	connReady
	connVersion // kerod of another version runs
	connFailed
)

// prefs are what the app remembers between runs; the sessions themselves
// are kerod's.
type prefs struct {
	Selected      string          `json:"selected,omitempty"`
	SidebarWidth  float32         `json:"sidebarWidth,omitempty"`
	SidebarHidden bool            `json:"sidebarHidden,omitempty"`
	Collapsed     map[string]bool `json:"collapsed,omitempty"`
	FontSize      float32         `json:"fontSize,omitempty"`
}

const (
	defaultSidebar = 268
	minSidebar     = 200
	maxSidebar     = 440
	defaultFont    = 13
	maxTerms       = 12 // terminals kept attached, most recently shown
)

// termEntry is a session's terminal in the app: attached to kerod, which
// streams the session to it.
type termEntry struct {
	term    *terminal.Terminal
	stream  *client.Stream
	err     string
	started time.Time
}

// App is the app's state. It lives on the main thread: other goroutines
// change it through Window.Update.
type App struct {
	win *mygo.Window

	cl      *client.Client
	conn    connState
	connErr string
	retries int
	st      proto.State
	loaded  bool // a state came

	prefs       prefs
	selected    string
	terms       map[string]*termEntry
	lru         []string
	refs        map[string]*ui.Handle // panes' terminals, and splits, by id
	focus       map[string]string     // the pane the user is in, by session
	layoutDirty string                // a session whose divider is dragged
	viewSent    *string

	active   bool
	focused  bool
	statuses map[string]proto.Status // as last seen, for notifications

	renaming  string
	renameBuf string

	renameFrames int
	chrome       chrome // where the sidebar's animation puts things

	pal          paletteState
	confirm      *confirmation
	sideDrag     float32 // the width as a drag of the sidebar's edge began
	pendingToast string
	// later are actions that change what the view is building, run once
	// it is built.
	later []func()

	// Panes dragged: the zone last under the pointer, the pane whose
	// drop preview shows and since when, and the panes just moved, which
	// spring in.
	lastZone     dropZone
	previewKey   string
	previewSince time.Time
	previewShown bool
	arrived      map[string]bool
	copyText     string
}

// confirmation is a question the app asks before something it cannot undo.
type confirmation struct {
	open    bool
	title   string
	detail  string
	action  string
	destroy bool
	run     func()
}

func newApp() *App {
	a := &App{terms: map[string]*termEntry{}, statuses: map[string]proto.Status{}, active: true, focused: true,
		refs: map[string]*ui.Handle{}, focus: map[string]string{}, arrived: map[string]bool{}}
	a.loadPrefs()
	a.selected = a.prefs.Selected
	return a
}

func prefsPath() string { return filepath.Join(proto.Dir(), "app.json") }

func (a *App) loadPrefs() {
	if b, err := os.ReadFile(prefsPath()); err == nil {
		json.Unmarshal(b, &a.prefs)
	}
	if a.prefs.SidebarWidth == 0 {
		a.prefs.SidebarWidth = defaultSidebar
	}
	a.prefs.SidebarWidth = min(max(a.prefs.SidebarWidth, minSidebar), maxSidebar)
	if a.prefs.FontSize == 0 {
		a.prefs.FontSize = defaultFont
	}
	if a.prefs.Collapsed == nil {
		a.prefs.Collapsed = map[string]bool{}
	}
}

func (a *App) savePrefs() {
	a.prefs.Selected = a.selected
	b, err := json.MarshalIndent(a.prefs, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(proto.Dir(), 0o700)
	tmp := prefsPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, prefsPath())
	}
}

// update runs fn on the main thread, and draws.
func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
	}
}

// connect connects to kerod, starting it, and follows its state.
func (a *App) connect() {
	go func() {
		cl, err := client.Connect(false)
		a.update(func() {
			if err != nil {
				a.cl = nil
				if errors.Is(err, client.ErrVersion) {
					a.conn = connVersion
					return
				}
				a.conn, a.connErr = connFailed, err.Error()
				a.retries++
				if a.retries < 4 {
					// It may be starting, or restarting: try again.
					time.AfterFunc(time.Duration(a.retries)*400*time.Millisecond, func() { a.update(a.connect) })
				}
				return
			}
			a.cl, a.conn, a.connErr, a.retries = cl, connReady, "", 0
			cl.OnEvent(func(m proto.Message) { a.update(func() { a.event(cl, m) }) })
			cl.Go("subscribe", nil, nil)
			a.viewSent = nil
			a.reportView()
			go func() {
				<-cl.Done()
				a.update(func() { a.disconnected(cl) })
			}()
		})
	}()
}

// disconnected follows kerod going away, as when it was restarted: the
// terminals attached to it are done, and the app connects again.
func (a *App) disconnected(cl *client.Client) {
	if a.cl != cl {
		return
	}
	a.cl = nil
	a.conn = connConnecting
	for id := range a.terms {
		a.dropTerm(id)
	}
	time.AfterFunc(300*time.Millisecond, func() { a.update(a.connect) })
}

func (a *App) event(cl *client.Client, m proto.Message) {
	if a.cl != cl {
		return
	}
	switch m.Event {
	case "state":
		if m.State != nil {
			a.setState(*m.State)
		}
	case "select":
		a.selectSession(m.Select)
		if a.win != nil {
			a.win.Show()
			a.win.Focus()
			mygo.App.Focus()
		}
	}
}

func (a *App) setState(st proto.State) {
	first := !a.loaded
	a.st, a.loaded = st, true
	// Terminals of panes gone go too.
	live := map[string]bool{}
	for _, s := range a.st.Sessions {
		for _, p := range s.Panes {
			live[p.ID] = true
		}
	}
	for id := range a.terms {
		if !live[id] {
			a.dropTerm(id)
		}
	}
	if a.session(a.selected) == nil {
		ids := a.visibleOrder()
		if len(ids) == 0 {
			a.selected = ""
		} else {
			a.selectSession(ids[0])
		}
	} else if first {
		a.selectSession(a.selected)
	}
	a.notify(first)
	a.updateTitle()
}

// notify tells about sessions that came to need the user, or finished,
// while the user was elsewhere: a notification, and the Dock's badge.
func (a *App) notify(first bool) {
	waiting := 0
	for i := range a.st.Sessions {
		s := &a.st.Sessions[i]
		prev, seen := a.statuses[s.ID]
		a.statuses[s.ID] = s.Status
		if s.Status == proto.Waiting {
			waiting++
		}
		if first || !seen || prev == s.Status || s.ID == a.selected && a.active {
			continue
		}
		var title, body string
		switch {
		case s.Status == proto.Waiting:
			title, body = s.DisplayName()+" needs you", s.Message
			if body == "" {
				body = "Waiting for your answer"
			}
		case s.Status == proto.Idle && prev == proto.Working && s.Unread && s.Agent != "":
			title, body = s.DisplayName()+" is done", fmt.Sprintf("%s finished in %s", s.Agent, proto.ShortDir(s.Dir))
		default:
			continue
		}
		if !mygo.NotificationsSupported() {
			continue
		}
		id := s.ID
		n := mygo.NewNotification(mygo.NotificationOptions{ID: "session/" + id, Group: id, Title: title, Body: body})
		n.OnClick(func() {
			a.update(func() {
				a.selectSession(id)
				a.win.Show()
				a.win.Focus()
			})
			n.Close()
		})
		go n.Show()
	}
	for id := range a.statuses {
		if a.session(id) == nil {
			delete(a.statuses, id)
		}
	}
	mygo.App.SetBadgeCount(waiting)
}

func (a *App) updateTitle() {
	if a.win == nil {
		return
	}
	title := "kero"
	if s := a.session(a.selected); s != nil {
		title = s.DisplayName()
	}
	a.win.SetTitle(title)
}

func (a *App) session(id string) *proto.Session {
	if id == "" {
		return nil
	}
	for i := range a.st.Sessions {
		if a.st.Sessions[i].ID == id {
			return &a.st.Sessions[i]
		}
	}
	return nil
}

func (a *App) project(id string) *proto.Project {
	for i := range a.st.Projects {
		if a.st.Projects[i].ID == id {
			return &a.st.Projects[i]
		}
	}
	return nil
}

// group is a project with its sessions, as the sidebar lists them.
type group struct {
	project  proto.Project
	sessions []*proto.Session
}

func (a *App) groups() []group {
	var gs []group
	known := map[string]bool{}
	for _, p := range a.st.Projects {
		known[p.ID] = true
		g := group{project: p}
		for i := range a.st.Sessions {
			if a.st.Sessions[i].Project == p.ID {
				g.sessions = append(g.sessions, &a.st.Sessions[i])
			}
		}
		gs = append(gs, g)
	}
	other := group{project: proto.Project{ID: "", Name: "Other"}}
	for i := range a.st.Sessions {
		if !known[a.st.Sessions[i].Project] {
			other.sessions = append(other.sessions, &a.st.Sessions[i])
		}
	}
	if len(other.sessions) > 0 {
		gs = append(gs, other)
	}
	return gs
}

// visibleOrder returns the sessions in the sidebar's order, those of
// collapsed projects left out unless selected.
func (a *App) visibleOrder() []string {
	var ids []string
	for _, g := range a.groups() {
		for _, s := range g.sessions {
			if !a.prefs.Collapsed[g.project.ID] || s.ID == a.selected {
				ids = append(ids, s.ID)
			}
		}
	}
	return ids
}

func (a *App) selectSession(id string) {
	if a.session(id) == nil {
		return
	}
	changed := a.selected != id
	a.selected = id
	s := a.session(id)
	for _, p := range s.Panes {
		a.ensureTerm(p.ID)
	}
	if pane := a.focusOf(s); pane != "" {
		a.touch(pane)
	}
	a.focusTerminal()
	a.reportView()
	a.updateTitle()
	if changed {
		a.savePrefs()
	}
}

// reportView tells kerod which session the user looks at, which it then
// never marks unread.
func (a *App) reportView() {
	if a.cl == nil {
		return
	}
	id := ""
	if a.active && a.focused {
		id = a.selected
	}
	if a.viewSent != nil && *a.viewSent == id {
		return
	}
	a.viewSent = &id
	a.cl.Go("view", proto.ViewParams{ID: id}, nil)
}

// touch moves a terminal to the front of those kept attached, dropping
// the one shown longest ago past the limit.
func (a *App) touch(id string) {
	a.lru = slices.DeleteFunc(a.lru, func(x string) bool { return x == id })
	a.lru = append(a.lru, id)
	for len(a.lru) > maxTerms {
		a.dropTerm(a.lru[0])
	}
}

func (a *App) dropTerm(id string) {
	if e := a.terms[id]; e != nil {
		if e.term != nil {
			e.term.Close()
		}
		if e.stream != nil {
			e.stream.Close()
		}
		delete(a.terms, id)
	}
	a.lru = slices.DeleteFunc(a.lru, func(x string) bool { return x == id })
}

// gated holds back the stream's output until the terminal takes the size
// of the snapshot, which then shows as it was.
type gated struct {
	*client.Stream
	ready chan struct{}
}

func (g *gated) Read(p []byte) (int, error) {
	<-g.ready
	return g.Stream.Read(p)
}

// ensureTerm attaches a terminal to a session, unless one is.
func (a *App) ensureTerm(id string) {
	if a.terms[id] != nil || a.conn != connReady {
		return
	}
	e := &termEntry{started: time.Now()}
	a.terms[id] = e
	go func() {
		stream, err := client.Attach(id)
		if err != nil {
			a.update(func() {
				if a.terms[id] == e {
					e.err = err.Error()
				}
			})
			return
		}
		g := &gated{Stream: stream, ready: make(chan struct{})}
		term, err := terminal.New(a.termOptions(id, g))
		if err != nil {
			stream.Close()
			a.update(func() {
				if a.terms[id] == e {
					e.err = err.Error()
				}
			})
			return
		}
		term.Resize(stream.Welcome.Cols, stream.Welcome.Rows)
		close(g.ready)
		a.update(func() {
			if a.terms[id] != e {
				term.Close()
				stream.Close()
				return
			}
			e.term, e.stream = term, stream
			if a.selected == id {
				a.focusTerminal()
			}
		})
	}()
}

func (a *App) termFont() terminal.Font {
	return terminal.Font{Size: a.prefs.FontSize, LineHeight: 1.22}
}

func (a *App) termOptions(id string, conn *gated) terminal.Options {
	return terminal.Options{
		Conn:          conn,
		Font:          a.termFont(),
		Theme:         lightTermTheme,
		DarkTheme:     darkTermTheme,
		ClipboardRead: terminal.ClipboardAsk,
		// The pane's card shows under it, dimmed when the user is in
		// another pane.
		Transparent: true,
		// Every terminal zooms together, and the size is remembered.
		OnZoom: func(z terminal.Zoom) bool {
			a.zoom(z.Steps, z.Reset)
			return true
		},
		OpenLink: func(link string) bool { return a.openLink(id, link) },
	}
}

func (a *App) zoom(steps int, reset bool) {
	size := a.prefs.FontSize + float32(steps)
	if reset {
		size = defaultFont
	}
	size = min(max(size, 8), 32)
	if size == a.prefs.FontSize {
		return
	}
	a.prefs.FontSize = size
	for _, e := range a.terms {
		if e.term != nil {
			e.term.SetFont(a.termFont())
		}
	}
	a.savePrefs()
}

var lineSuffix = regexp.MustCompile(`(:\d+){1,2}$`)

// openLink opens what a Command+click opens: URLs with the system, and
// file paths printed in the session, relative to its directory, in the
// app that opens them.
func (a *App) openLink(id, link string) bool {
	if strings.Contains(link, "://") {
		return false
	}
	path := lineSuffix.ReplaceAllString(link, "")
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(proto.Home(), path[2:])
	}
	if !filepath.IsAbs(path) {
		if s := a.session(id); s != nil && s.Dir != "" {
			path = filepath.Join(s.Dir, path)
		}
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	exec.Command("/usr/bin/open", path).Start()
	return true
}

// --- actions ---

func (a *App) call(method string, params any, done func(err error)) {
	if a.cl == nil {
		return
	}
	a.cl.Go(method, params, func(err error) {
		if done != nil || err != nil {
			a.update(func() {
				if done != nil {
					done(err)
				} else {
					a.toast(err.Error())
				}
			})
		}
	})
}

func (a *App) toast(msg string) { a.pendingToast = msg }

// newSession starts a session in a project: in the directory of the
// session shown when it is the project's, as a terminal's new tab does,
// else the project's.
func (a *App) newSession(projectID string) {
	if a.cl == nil {
		return
	}
	p := proto.CreateParams{Project: projectID}
	if cur := a.session(a.selected); cur != nil {
		if projectID == "" || cur.Project == projectID {
			p.Project, p.Dir, p.After = cur.Project, cur.Dir, cur.ID
		}
		p.Cols, p.Rows = cur.Cols, cur.Rows
	}
	if p.Project == "" && len(a.st.Projects) == 0 {
		p.Dir = proto.Home()
	}
	cl := a.cl
	go func() {
		var s proto.Session
		err := cl.Call("create", p, &s)
		a.update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			// The state with it may come after the answer.
			a.addPending(s)
			a.selectSession(s.ID)
		})
	}()
}

// addPending puts a session just created into the state until the
// daemon's comes with it.
func (a *App) addPending(s proto.Session) {
	if a.session(s.ID) != nil {
		return
	}
	a.st.Sessions = append(a.st.Sessions, s)
	if a.project(s.Project) == nil {
		a.st.Projects = append(a.st.Projects, proto.Project{ID: s.Project, Name: filepath.Base(s.Dir), Dir: s.Dir})
	}
}

// openProject asks for a folder, and starts a session there, in its
// project.
func (a *App) openProject() {
	if a.cl == nil {
		return
	}
	cl, win := a.cl, a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: win, Title: "Open Project", Directory: true, ButtonLabel: "Open",
			Message: "Choose a folder: its sessions are grouped under its name.",
		})
		if err != nil || len(paths) == 0 {
			return
		}
		var p proto.Project
		if err := cl.Call("project.create", proto.ProjectParams{Dir: paths[0]}, &p); err != nil {
			a.update(func() { a.toast(err.Error()) })
			return
		}
		var s proto.Session
		err = cl.Call("create", proto.CreateParams{Project: p.ID, Dir: paths[0]}, &s)
		a.update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			delete(a.prefs.Collapsed, p.ID)
			a.addPending(s)
			a.selectSession(s.ID)
		})
	}()
}

// busy reports whether closing a session would end a program the user may
// care about: anything but the shell at its prompt.
func busy(s *proto.Session) bool {
	return s.Status != proto.Exited && s.Process != ""
}

func (a *App) closeSession(id string, ask bool) {
	s := a.session(id)
	if s == nil {
		return
	}
	if ask && busy(s) {
		name := s.DisplayName()
		a.confirm = &confirmation{
			open:    true,
			title:   fmt.Sprintf("Close “%s”?", name),
			detail:  fmt.Sprintf("%s is still running in it. Closing the session ends it.", s.Process),
			action:  "Close Session",
			destroy: true,
			run:     func() { a.closeSession(id, false) },
		}
		return
	}
	if id == a.selected {
		// The next session takes its place, else the one before.
		ids := a.visibleOrder()
		i := slices.Index(ids, id)
		next := ""
		if i >= 0 && i+1 < len(ids) {
			next = ids[i+1]
		} else if i > 0 {
			next = ids[i-1]
		}
		a.selected = next
		if next != "" {
			a.selectSession(next)
		}
	}
	for _, p := range s.Panes {
		a.dropTerm(p.ID)
	}
	a.st.Sessions = slices.DeleteFunc(a.st.Sessions, func(x proto.Session) bool { return x.ID == id })
	a.call("close", proto.IDParams{ID: id}, nil)
}

func (a *App) restartSession(id string) {
	a.call("restart", proto.IDParams{ID: id}, nil)
}

func (a *App) startRename(id string) {
	s := a.session(id)
	if s == nil {
		return
	}
	a.renaming, a.renameBuf, a.renameFrames = id, s.DisplayName(), 0
}

func (a *App) commitRename() {
	id, name := a.renaming, strings.TrimSpace(a.renameBuf)
	a.renaming = ""
	s := a.session(id)
	if s == nil || name == s.DisplayName() {
		a.focusTerminal()
		return
	}
	s.Name = name // at once; the daemon's state follows
	a.call("rename", proto.RenameParams{ID: id, Name: name}, nil)
	a.focusTerminal()
	a.updateTitle()
}

func (a *App) moveSession(id, project string) {
	if s := a.session(id); s != nil {
		s.Project = project
	}
	a.call("move", proto.MoveParams{ID: id, Project: project, Index: -1}, nil)
}

func (a *App) selectOffset(d int) {
	ids := a.visibleOrder()
	if len(ids) == 0 {
		return
	}
	i := slices.Index(ids, a.selected)
	if i < 0 {
		i = 0
	} else {
		i = (i + d + len(ids)) % len(ids)
	}
	a.selectSession(ids[i])
}

// jumpToAttention goes to the next session that needs the user, after the
// one shown, else to the next with something unseen.
func (a *App) jumpToAttention() {
	var ids []string
	for _, g := range a.groups() {
		for _, s := range g.sessions {
			ids = append(ids, s.ID)
		}
	}
	i := slices.Index(ids, a.selected)
	for _, want := range []func(*proto.Session) bool{
		func(s *proto.Session) bool { return s.Status == proto.Waiting },
		func(s *proto.Session) bool { return s.Unread },
	} {
		for k := 1; k <= len(ids); k++ {
			id := ids[(i+k+len(ids))%len(ids)]
			if s := a.session(id); s != nil && id != a.selected && want(s) {
				delete(a.prefs.Collapsed, s.Project)
				a.selectSession(id)
				return
			}
		}
	}
	a.toast("Nothing needs you")
}

func (a *App) toggleSidebar() {
	a.prefs.SidebarHidden = !a.prefs.SidebarHidden
	a.savePrefs()
}

func (a *App) toggleProject(id string) {
	a.prefs.Collapsed[id] = !a.prefs.Collapsed[id]
	if !a.prefs.Collapsed[id] {
		delete(a.prefs.Collapsed, id)
	}
	a.savePrefs()
}

// quitAndEnd ends every session and kerod, then quits.
func (a *App) quitAndEnd() {
	n := len(a.st.Sessions)
	a.confirm = &confirmation{
		open:    true,
		title:   "Quit and end all sessions?",
		detail:  fmt.Sprintf("kero keeps sessions running when it quits. This ends all %d of them, with what runs in them.", n),
		action:  "End Sessions and Quit",
		destroy: true,
		run: func() {
			cl := a.cl
			go func() {
				if cl != nil {
					cl.Call("shutdown", nil, nil)
				}
				mygo.App.Quit()
			}()
		},
	}
}

// restartDaemon restarts kerod, as when it is of another version: its
// sessions start again where they were, with their scrollback.
func (a *App) restartDaemon() {
	a.conn = connConnecting
	go func() {
		err := stopDaemon()
		a.update(func() {
			if err != nil {
				a.conn, a.connErr = connFailed, err.Error()
				return
			}
			a.connect()
		})
	}()
}
