package daemon

import (
	"errors"
	"slices"
	"time"

	"kero/internal/proto"
)

// An agent's own way in: a message typed into its prompt takes along what
// the user was typing there, and an Enter pressed for it sends the user's
// half-written text, so a message goes where the agent takes messages from
// outside, when it has such a place, and is typed only when not. Claude
// Code's monitor of Kero's plugin and the extension Kero installs in Pi
// and OMP run `kero inbox`, which waits here for the messages of its pane
// (agent.inbox) and gives each to the agent as a turn of its own; Codex
// and OpenCode take them from kerod (native.go).

// inboxGrace is how long a listener may be away between two waits, as it
// gives a message to its agent and asks for the next, before the messages
// go the other way.
const inboxGrace = 3 * time.Second

// agentInbox waits for the messages for pane pn's agent (agent.inbox), and
// takes them, as one prompt; turns, for a listener that gives them as the
// user's turns (InboxParams.Turns).
func (d *Daemon) agentInbox(c *controlConn, pn *pane, timeout time.Duration, turns bool) (proto.InboxMessage, error) {
	pn.mu.Lock()
	id, closed := pn.info.ID, pn.closed
	pn.mu.Unlock()
	if closed {
		return proto.InboxMessage{}, errors.New("pane closed")
	}
	mb := &d.mail
	ch := make(chan string, 1)
	mb.mu.Lock()
	mb.initLocked()
	mb.inbox[id] = append(mb.inbox[id], inboxListener{ch, turns})
	mb.mu.Unlock()
	// The pane may have closed, and let go of its listeners, since.
	pn.mu.Lock()
	closed = pn.closed
	pn.mu.Unlock()
	if closed {
		d.mailGone(id)
	}
	// What waits for the agent goes now, as it may.
	select {
	case mb.wake <- struct{}{}:
	default:
	}
	var expired <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		expired = t.C
	}
	var done <-chan struct{}
	if c != nil {
		done = c.done
	}
	var why error
	var text string
	got, open := false, true
	select {
	case text, open = <-ch:
		got = true
	case <-expired:
	case <-done:
		why = errors.New("canceled")
	case <-d.quit:
		why = errors.New("kerod exited")
	}
	if !open {
		return proto.InboxMessage{}, errors.New("pane closed")
	}
	// No longer waiting. A message handed over meanwhile is the listener's
	// still, while it is there to take it; else it waits for the next way.
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.heard[id] = time.Now()
	if got {
		return proto.InboxMessage{Text: text}, nil
	}
	mb.setInboxLocked(id, slices.DeleteFunc(mb.inbox[id], func(w inboxListener) bool { return w.ch == ch }))
	select {
	case text = <-ch:
	default:
	}
	if text != "" && why != nil {
		mb.out[id] = append([]outMsg{{text: text}}, mb.out[id]...)
		text = ""
	}
	return proto.InboxMessage{Text: text}, why
}

// inboxBack puts text, which a listener of pane pn's agent took and could
// not give it, as its agent had gone, first among the pane's messages.
// That listener has stopped, so they go to another or are typed in.
func (d *Daemon) inboxBack(pn *pane, text string) {
	pn.mu.Lock()
	id := pn.info.ID
	pn.mu.Unlock()
	mb := &d.mail
	mb.mu.Lock()
	mb.initLocked()
	mb.out[id] = append([]outMsg{{text: text}}, mb.out[id]...)
	delete(mb.heard, id)
	mb.mu.Unlock()
	select {
	case mb.wake <- struct{}{}:
	default:
	}
}

// setInboxLocked sets the listeners waiting for pane id's messages; mb.mu
// is held.
func (mb *mailbox) setInboxLocked(id string, ws []inboxListener) {
	if len(ws) == 0 {
		delete(mb.inbox, id)
	} else {
		mb.inbox[id] = ws
	}
}

// listenerLocked tells whether a listener of the agent of pane id waits,
// or is away between two waits, which it is back from soon; mb.mu is
// held.
func (mb *mailbox) listenerLocked(id string, now time.Time) (waiting, between bool) {
	if len(mb.inbox[id]) > 0 {
		return true, false
	}
	heard, ok := mb.heard[id]
	return false, ok && now.Sub(heard) < inboxGrace
}
