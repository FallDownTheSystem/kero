package daemon

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/plugins/terminal"

	"kero/internal/proto"
	"kero/internal/settings"
)

const (
	// echoWindow: output this soon after input is the program answering
	// the keys (echo, a redraw), not work of its own.
	echoWindow = 350 * time.Millisecond
	// quietAfter: a program working that writes nothing for this long is
	// done, or waiting for the user.
	quietAfter = 2 * time.Second
)

// pane is a shell the daemon keeps running in a pseudo-terminal, with a
// terminal emulator of its own (libghostty-vt, headless) that keeps its
// screen and scrollback for the clients that attach later.
type pane struct {
	d   *Daemon
	sid string // the session's it belongs to

	// mu guards the fields below, and orders the output the emulator and
	// the clients get with clients attaching. The emulator's callbacks
	// (OnTitle, OnBell, OnNotify) run within Feed, with mu held.
	mu       sync.Mutex
	info     proto.Pane
	proc     *ptyProc
	term     *terminal.Terminal
	clients  map[*attachment]struct{}
	shellPID int
	fgPGID   int
	fgAt     time.Time
	cwdAt    time.Time

	lastInput time.Time
	lastWork  time.Time
	workBytes int
	dirty     bool // output since the last snapshot on disk
	// osc finds OSC 7501 and 133 in the output; records are the programs'
	// status reports (OSC 7501), by id, and reported tells that one came.
	osc      oscScanner
	records  map[string]proto.Record
	reported bool
	closed   bool
	changed  chan struct{} // closed and replaced on every status change

	attached atomic.Int32
}

// headless connects the daemon's emulator to the program: it reads
// nothing (the daemon feeds it), and writes the emulator's answers to the
// program's queries only while no client is attached, whose emulator
// answers instead.
type headless struct {
	s      *pane
	closed chan struct{}
	once   sync.Once
}

func (h *headless) Read([]byte) (int, error) {
	<-h.closed
	return 0, io.EOF
}

func (h *headless) Write(p []byte) (int, error) {
	if h.s.attached.Load() == 0 {
		h.s.mu.Lock()
		proc := h.s.proc
		h.s.mu.Unlock()
		if proc != nil {
			proc.Write(p)
		}
	}
	return len(p), nil
}

func (h *headless) Close() error {
	h.once.Do(func() { close(h.closed) })
	return nil
}

func newPane(d *Daemon, sid string, info proto.Pane) (*pane, error) {
	s := &pane{d: d, sid: sid, info: info, clients: map[*attachment]struct{}{}, changed: make(chan struct{})}
	s.info.Progress = -1
	if s.info.Cols <= 0 || s.info.Rows <= 0 {
		s.info.Cols, s.info.Rows = 120, 36
	}
	sb := settings.Load().Scrollback << 20
	if sb == 0 {
		sb = -1 // none
	}
	term, err := terminal.New(terminal.Options{
		Conn:       &headless{s: s, closed: make(chan struct{})},
		Scrollback: sb,
		OnTitle:    s.onTitle,
		OnBell:     s.onBell,
		OnNotify:   s.onNotify,
	})
	if err != nil {
		return nil, err
	}
	term.Resize(s.info.Cols, s.info.Rows)
	s.term = term
	return s, nil
}

// start starts the shell, running command first when set.
func (s *pane) start(command []string, env []string) error {
	shell := userShell()
	name := filepath.Base(shell)
	args := []string{"-" + name}
	if len(command) > 0 {
		// The command runs in an interactive login shell, so that it finds
		// what the user's profile puts on PATH; the session goes on with
		// the shell once it ends.
		script := shellJoin(name, command) + "; exec " + shellQuote(name, shell) + " -l"
		args = []string{name, "-l", "-i", "-c", script}
	}
	dir := s.info.Dir
	if st, err := os.Stat(dir); dir == "" || err != nil || !st.IsDir() {
		dir = proto.Home()
	}
	proc, err := startPTY(ptyOptions{
		Path: shell, Args: args, Dir: dir,
		Env:  s.d.environment(s.sid, s.info.ID, env),
		Cols: s.info.Cols, Rows: s.info.Rows,
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.proc = proc
	s.shellPID = proc.PID()
	s.info.PID = s.shellPID
	s.info.Dir = dir
	s.info.Shell = name
	s.mu.Unlock()
	go s.pump(proc)
	return nil
}

// pump reads the program's output until it ends.
func (s *pane) pump(proc *ptyProc) {
	buf := make([]byte, 64<<10)
	for {
		n, err := proc.Read(buf)
		if n > 0 {
			s.output(buf[:n])
		}
		if err != nil {
			break
		}
	}
	select {
	case <-proc.exited:
	case <-time.After(time.Second):
		proc.Kill()
		<-proc.exited
	}
	s.exited(proc)
}

func (s *pane) output(p []byte) {
	now := time.Now()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	before := s.info.Status
	s.osc.scan(p, s.handleOSC)
	s.term.Feed(p)
	s.dirty = true
	s.info.Activity = now.UnixMilli()
	for c := range s.clients {
		c.send(proto.FrameOutput, p)
	}
	if now.Sub(s.lastInput) > echoWindow {
		s.lastWork = now
		if s.info.Status != proto.Working && !s.declared() && s.programBusy(now) {
			s.workBytes += len(p)
			// A redraw of a few bytes is not work yet: a prompt's clock,
			// a cursor moving.
			if s.workBytes > 48 || proto.TitleBusy(s.info.Title) {
				s.setStatus(proto.Working)
				s.info.Message = ""
			}
		}
	}
	changed := s.info.Status != before
	s.mu.Unlock()
	if changed {
		s.d.changed()
	}
}

// programBusy reports whether a program other than the shell is in the
// foreground, as when a command or an agent runs; s.mu is held.
func (s *pane) programBusy(now time.Time) bool {
	if s.proc == nil {
		return false
	}
	if now.Sub(s.fgAt) > 100*time.Millisecond {
		s.fgPGID = s.proc.Foreground()
		s.fgAt = now
	}
	return s.fgPGID != 0 && s.fgPGID != s.shellPID
}

func (s *pane) onTitle(title string) {
	// s.mu is held (see session.mu).
	if s.info.Title != title {
		s.info.Title = title
		if proto.TitleBusy(title) && s.info.Status != proto.Working && s.info.Status != proto.Exited && !s.declared() {
			s.setStatus(proto.Working)
			s.info.Message = ""
		}
		s.d.changed()
	}
}

func (s *pane) onBell() {
	// A shell's bell is a failed completion, not a question.
	if s.fgPGID == 0 || s.fgPGID == s.shellPID {
		return
	}
	s.needsUser("")
}

func (s *pane) onNotify(title, body string) {
	msg := strings.TrimSpace(body)
	if msg == "" {
		msg = strings.TrimSpace(title)
	}
	s.needsUser(msg)
}

// needsUser marks the session waiting for the user; s.mu is held.
func (s *pane) needsUser(msg string) {
	if s.info.Status == proto.Exited || s.declared() {
		return
	}
	s.setStatus(proto.Waiting)
	if msg != "" {
		s.info.Message = msg
	}
	s.workBytes = 0
	if !s.viewed() {
		s.info.Unread = true
	}
	s.d.changed()
}

func (s *pane) viewed() bool { return s.d.viewedID() == s.sid }

// setStatus changes the status, waking those waiting on it; s.mu is held.
func (s *pane) setStatus(st proto.Status) {
	if s.info.Status == st {
		return
	}
	s.info.Status = st
	s.info.Since = time.Now().UnixMilli()
	close(s.changed)
	s.changed = make(chan struct{})
}

// tick follows the foreground program and the shell's directory, and ends
// work that went quiet.
func (s *pane) tick(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.proc == nil || s.info.Status == proto.Exited {
		return false
	}
	changed := false
	pg := s.proc.Foreground()
	s.fgPGID, s.fgAt = pg, now
	proc, agent := "", ""
	if pg != 0 && pg != s.shellPID {
		proc = procName(pg)
		args := procArgs(pg)
		agent = detectAgent(proc, args)
		if proc == "node" || proc == "bun" || proc == "python" || proc == "python3" || proc == "deno" {
			// A script: name it by the script, as "codex", not "node".
			if len(args) > 1 {
				proc = strings.TrimSuffix(filepath.Base(args[1]), filepath.Ext(args[1]))
			}
		}
		if agent != "" {
			proc = agent
		}
	}
	if proc == "" && s.info.Process != "" {
		// The program ended: its live reports go with it.
		s.dropLive()
	}
	if proc != s.info.Process || agent != s.info.Agent {
		s.info.Process, s.info.Agent = proc, agent
		changed = true
	}
	if now.Sub(s.cwdAt) > 1500*time.Millisecond {
		s.cwdAt = now
		if dir := procCwd(s.shellPID); dir != "" && dir != s.info.Dir {
			s.info.Dir = dir
			changed = true
			s.d.save()
		}
	}
	if s.info.Status == proto.Working && now.Sub(s.lastWork) > quietAfter && !proto.TitleBusy(s.info.Title) && !s.declared() {
		s.workBytes = 0
		if s.asksQuestion() {
			s.setStatus(proto.Waiting)
		} else {
			s.setStatus(proto.Idle)
		}
		if !s.viewed() {
			s.info.Unread = true
		}
		changed = true
	}
	if s.info.Status == proto.Idle && s.workBytes > 0 && now.Sub(s.lastWork) > quietAfter {
		s.workBytes = 0
	}
	return changed
}

// questions are what the bottom of a screen shows when a program asks
// the user something: agents' permission prompts and confirmations.
var questions = []string{
	"do you want", "would you like", "(y/n)", "[y/n]", "y/n]", "(yes/no)", "[yes/no]",
	"press enter", "proceed?", "continue?", "allow ", "approve", "❯ 1.", "› 1.", "● 1.",
	"yes, allow", "waiting for your", "requires approval", "needs your",
}

// asksQuestion reports whether the screen's last lines ask the user
// something; s.mu is held.
func (s *pane) asksQuestion() bool {
	text := s.term.Text()
	lines := strings.Split(text, "\n")
	n := 0
	for i := len(lines) - 1; i >= 0 && n < 14; i-- {
		l := strings.ToLower(strings.TrimSpace(lines[i]))
		if l == "" {
			continue
		}
		n++
		for _, q := range questions {
			if strings.Contains(l, q) {
				return true
			}
		}
	}
	return false
}

// input writes what the user typed to the program.
func (s *pane) input(p []byte) {
	s.mu.Lock()
	proc := s.proc
	s.lastInput = time.Now()
	changed := false
	if len(s.records) > 0 {
		s.dismissFinished()
	}
	if s.info.Status == proto.Waiting && !s.declared() || s.info.Status == proto.Failed {
		s.setStatus(proto.Idle)
		s.info.Message = ""
		changed = true
	}
	if s.info.Unread {
		s.info.Unread = false
		changed = true
	}
	s.mu.Unlock()
	if proc != nil {
		proc.Write(p)
	}
	if changed {
		s.d.changed()
	}
}

func (s *pane) resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || cols == s.info.Cols && rows == s.info.Rows {
		return
	}
	s.info.Cols, s.info.Rows = cols, rows
	s.term.Resize(cols, rows)
	if s.proc != nil {
		s.proc.Resize(cols, rows)
	}
}

// exited ends the session as its shell exits: a shell the user exited
// closes it, as a terminal's tab closes; one that failed stays, with its
// output, until the user closes or restarts it.
func (s *pane) exited(proc *ptyProc) {
	s.mu.Lock()
	if s.closed || s.proc != proc {
		s.mu.Unlock()
		return
	}
	code := proc.code
	s.proc = nil
	s.dropLive()
	s.info.ExitCode = code
	s.info.Process, s.info.Agent, s.info.PID = "", "", 0
	if code == 0 {
		s.mu.Unlock()
		s.d.removePane(s.info.ID)
		return
	}
	msg := fmt.Sprintf("\r\n\x1b[0;2m[Process exited with code %d]\x1b[0m", code)
	s.term.Feed([]byte(msg))
	for c := range s.clients {
		c.send(proto.FrameOutput, []byte(msg))
	}
	s.setStatus(proto.Exited)
	if !s.viewed() {
		s.info.Unread = true
	}
	s.mu.Unlock()
	s.d.changed()
}

// restart starts a shell again in a session that exited.
func (s *pane) restart() error {
	s.mu.Lock()
	if s.proc != nil {
		s.mu.Unlock()
		return fmt.Errorf("session %s is running", s.info.ID)
	}
	banner := "\r\n\x1b[0;2m[Restarted]\x1b[0m\r\n"
	s.term.Feed([]byte(banner))
	for c := range s.clients {
		c.send(proto.FrameOutput, []byte(banner))
	}
	s.setStatus(proto.Idle)
	s.info.Unread, s.info.Message, s.info.ExitCode = false, "", 0
	s.mu.Unlock()
	if err := s.start(nil, nil); err != nil {
		return err
	}
	s.d.changed()
	return nil
}

// close hangs up the shell and frees the session.
func (s *pane) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.changed) // wakes those waiting on it
	s.changed = make(chan struct{})
	proc := s.proc
	clients := s.clients
	s.clients = map[*attachment]struct{}{}
	s.mu.Unlock()
	for c := range clients {
		c.send(proto.FrameClosed, nil)
		c.finish()
	}
	if proc != nil {
		proc.Close()
		go func() {
			select {
			case <-proc.exited:
			case <-time.After(2 * time.Second):
				proc.Kill()
			}
		}()
	}
	s.term.Close()
}

// attach registers a client, returning the snapshot it starts from: what
// the session shows, with the output after it going to the client.
func (s *pane) attach(c *attachment) (snap []byte, cols, rows int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, 0, 0, false
	}
	snap = s.term.Snapshot()
	s.clients[c] = struct{}{}
	s.attached.Add(1)
	return snap, s.info.Cols, s.info.Rows, true
}

func (s *pane) detach(c *attachment) {
	s.mu.Lock()
	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)
		s.attached.Add(-1)
	}
	s.mu.Unlock()
}

// snapshot returns what to save of the session, when it changed.
func (s *pane) snapshot(force bool) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.dirty && !force {
		return nil
	}
	s.dirty = false
	return s.term.Snapshot()
}

// readText returns the session's text: the screen, or with scrollback.
func (s *pane) readText(scrollback bool, lines int) string {
	s.mu.Lock()
	text := s.term.Text()
	rows := s.info.Rows
	s.mu.Unlock()
	all := strings.Split(strings.TrimRight(text, "\n "), "\n")
	if !scrollback && lines <= 0 {
		lines = rows
	}
	if lines > 0 && len(all) > lines {
		all = all[len(all)-lines:]
	}
	for i, l := range all {
		all[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(all, "\n") + "\n"
}

// attachment is a client attached to a session: its frames go out in
// order through a queue, so that a slow client never holds the session.
type attachment struct {
	conn net.Conn
	out  chan []byte
	once sync.Once
	done chan struct{}
}

func newAttachment(conn net.Conn) *attachment {
	return &attachment{conn: conn, out: make(chan []byte, 8192), done: make(chan struct{})}
}

func (a *attachment) send(kind byte, payload []byte) {
	f := make([]byte, 5+len(payload))
	f[0] = kind
	f[1], f[2], f[3], f[4] = byte(len(payload)>>24), byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
	copy(f[5:], payload)
	select {
	case a.out <- f:
	case <-a.done:
	default:
		// Too far behind: drop it, and it attaches again from a snapshot.
		a.finish()
	}
}

// finish ends the attachment: the writer flushes what is queued, as a
// closing frame, then closes the connection.
func (a *attachment) finish() {
	a.once.Do(func() { close(a.done) })
}

// write sends the queued frames until the client goes.
func (a *attachment) write() {
	defer a.conn.Close()
	for {
		select {
		case f := <-a.out:
			if _, err := a.conn.Write(f); err != nil {
				a.finish()
				return
			}
		case <-a.done:
			a.conn.SetWriteDeadline(time.Now().Add(time.Second))
			for {
				select {
				case f := <-a.out:
					if _, err := a.conn.Write(f); err != nil {
						return
					}
				default:
					return
				}
			}
		}
	}
}
