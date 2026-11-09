package daemon

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"kero/internal/detect"
	"kero/internal/proto"
)

// Kero's one way of giving a coding agent a message, labelled with where
// it comes from (agent.send): Send to Session… of a terminal's text,
// review's Send to Agent, PR watch, orchestrators' reports and the page
// bridge (window.kero.send) all send through it. A message is pushed once
// the agent is at its prompt (agentReady), or kept for an agent waiting
// for its page's messages (kero browser wait-message). A pushed message
// goes through the agent's own way in, where it has one, which leaves
// what the user is typing in its prompt alone (inbox.go); else it is
// typed into the prompt, as kero prompt does. The app hears when one from
// a pane reached its agent, or never will (proto.Delivery), to tell the
// page it came from.

const (
	deliverTick = 500 * time.Millisecond
	// deliverHold is how long a pane typed into is left alone, unless its
	// agent worked and stopped since: its Enter may not have gone yet.
	deliverHold = 10 * time.Second
	// agentGone is how long a pane's agent may be gone, as it quit, before
	// what waits for it fails rather than go to the next agent there.
	agentGone = 30 * time.Second
)

// mailbox holds the messages on their way; its own lock, never held while
// taking d.mu or a pane's.
type mailbox struct {
	// flush makes one flushOutbox at a time, which a tick, a send and
	// another send may start together: two would type into one pane.
	flush sync.Mutex
	mu    sync.Mutex
	last  int64
	// out are the messages to type in, by the agent's pane, in order;
	// typed is when kerod last typed into a pane, and since the pane's
	// status's Since just after, which kerod's own paste changes; noAgent
	// is since when a pane with messages waiting has had no agent.
	out     map[string][]outMsg
	typed   map[string]time.Time
	since   map[string]int64
	noAgent map[string]time.Time
	// kept are the messages for wait-message, by the pane they come from;
	// waiters wait for them, by the same pane.
	kept    map[string][]proto.AgentMessage
	waiters map[string][]chan proto.AgentMessage
	// inbox are the agents' own listeners waiting for their messages, by
	// the agent's pane (kero inbox), and heard when one last stopped
	// waiting, to come back for the next.
	inbox map[string][]chan string
	heard map[string]time.Time
	// pending are the panes whose messages are on their way through the
	// agent's own way in (native.go); noNative, by pane, the agent's
	// session whose way in failed, which gets its messages typed in.
	pending  map[string]bool
	noNative map[string]string
	wake     chan struct{}
}

type outMsg struct {
	id         int64
	from, text string
}

func (mb *mailbox) initLocked() {
	if mb.out == nil {
		mb.out, mb.typed, mb.since, mb.noAgent = map[string][]outMsg{}, map[string]time.Time{}, map[string]int64{}, map[string]time.Time{}
		mb.kept, mb.waiters = map[string][]proto.AgentMessage{}, map[string][]chan proto.AgentMessage{}
		mb.inbox, mb.heard = map[string][]chan string{}, map[string]time.Time{}
		mb.pending, mb.noNative = map[string]bool{}, map[string]string{}
		mb.wake = make(chan struct{}, 1)
	}
}

// agentReady tells that a pane's agent is at its prompt, or done and
// waiting for the user to look; not working, nor asking for a permission
// or an answer, which typing would give. p.mu is held.
func (p *pane) agentReady() bool {
	st := p.info.Status
	return p.term != nil && !p.closed && p.info.Agent != "" &&
		(st == proto.Idle || st == proto.Waiting && p.seen != detect.Blocked && !p.declared() && p.info.Kind == "")
}

// pageLabel is how the agent is told of a browser pane's page: its title
// and URL, and the pane.
func pageLabel(id, url, title string) string {
	if title != "" && title != url {
		return fmt.Sprintf("the page %s (%s) in browser pane %s", proto.Quoted(title), url, id)
	}
	return fmt.Sprintf("the page %s in browser pane %s", url, id)
}

// agentSend sends a message to an agent (agent.send).
func (d *Daemon) agentSend(p proto.AgentSendParams) (proto.AgentSendResult, error) {
	if strings.TrimSpace(p.Text) == "" {
		return proto.AgentSendResult{}, errors.New("agent.send: no text")
	}
	switch p.Mode {
	case "", "push", "wait":
	default:
		return proto.AgentSendResult{}, fmt.Errorf("agent.send: no mode %q (push or wait)", p.Mode)
	}
	if p.ToParent {
		// With its orchestrator's next report of its children.
		return d.reportToParent(p)
	}
	d.mu.Lock()
	var from *pane
	var info proto.Pane
	if p.From != "" {
		if from = d.findPaneLocked(p.From); from == nil {
			d.mu.Unlock()
			return proto.AgentSendResult{}, fmt.Errorf("no pane %q", p.From)
		}
		from.mu.Lock()
		info = from.info
		from.mu.Unlock()
	}
	d.mu.Unlock()
	msg := proto.AgentMessage{From: info.ID, Label: p.Label, URL: p.URL, Title: p.Title, Text: p.Text, At: time.Now().UnixMilli()}
	if from != nil && info.Browser() {
		if msg.URL == "" {
			msg.URL, msg.Title = info.URL, info.Title
		}
		page := pageLabel(info.ID, msg.URL, msg.Title)
		if p.Label != "" {
			page = p.Label + ", from " + page
		}
		msg.Label = page
	} else if from != nil && msg.Label == "" {
		msg.Label = "terminal pane " + info.ID
	}

	mb := &d.mail
	mb.mu.Lock()
	mb.initLocked()
	mb.last++
	msg.ID = mb.last
	// An agent waiting for the pane's messages takes it, unless it is to
	// be pushed.
	if p.Mode != "push" && msg.From != "" && len(mb.waiters[msg.From]) > 0 {
		ws := mb.waiters[msg.From]
		w := ws[0]
		mb.waiters[msg.From] = ws[1:]
		w <- msg
		mb.mu.Unlock()
		// Delivered once the waiter has it (waitMessage), which may time
		// out at this very moment, and keep it for the next.
		return proto.AgentSendResult{ID: msg.ID, State: "waiting"}, nil
	}
	if p.Mode == "wait" {
		if msg.From == "" {
			mb.mu.Unlock()
			return proto.AgentSendResult{}, errors.New("agent.send: wait keeps a pane's messages: which pane?")
		}
		mb.kept[msg.From] = append(mb.kept[msg.From], msg)
		mb.mu.Unlock()
		return proto.AgentSendResult{ID: msg.ID, State: "waiting"}, nil
	}
	mb.mu.Unlock()

	to, err := d.agentTarget(p.To, info.Opener)
	if err != nil {
		return proto.AgentSendResult{}, err
	}
	if to == "" {
		// Nowhere to go: the app asks the user which agent.
		return proto.AgentSendResult{ID: msg.ID, State: "nowhere"}, nil
	}
	mb.mu.Lock()
	mb.out[to] = append(mb.out[to], outMsg{id: msg.ID, from: msg.From, text: msg.Prompt()})
	mb.mu.Unlock()
	d.flushOutbox(time.Now())
	mb.mu.Lock()
	queued := slices.ContainsFunc(mb.out[to], func(m outMsg) bool { return m.id == msg.ID })
	mb.mu.Unlock()
	state := "delivered"
	if queued {
		state = "queued"
		select {
		case mb.wake <- struct{}{}:
		default:
		}
	}
	return proto.AgentSendResult{ID: msg.ID, To: to, State: state}, nil
}

// agentTarget is the agent's pane a message goes to: to, a pane or a
// session, which stands for the pane of its agent; else the opener's,
// when it still is, or its session's agent's. "" for none.
func (d *Daemon) agentTarget(to, opener string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if to != "" {
		pn := d.findPaneLocked(to)
		if pn == nil {
			s := d.findSessionLocked(to)
			if s == nil {
				return "", fmt.Errorf("no session or pane %q", to)
			}
			if pn = prSessionLocked(s).agent; pn == nil {
				return "", fmt.Errorf("session %s runs no agent Kero knows", s.id)
			}
		}
		pn.mu.Lock()
		id, agent := pn.info.ID, pn.term != nil && pn.info.Agent != ""
		pn.mu.Unlock()
		if !agent {
			return "", fmt.Errorf("pane %s runs no agent Kero knows", id)
		}
		return id, nil
	}
	pn := d.findPaneLocked(opener)
	if pn == nil {
		return "", nil
	}
	pn.mu.Lock()
	agent := pn.term != nil && pn.info.Agent != ""
	pn.mu.Unlock()
	if agent {
		return opener, nil
	}
	if s := d.sessionOfLocked(pn); s != nil {
		if a := prSessionLocked(s).agent; a != nil {
			return a.info.ID, nil
		}
	}
	return "", nil
}

// deliverLoop types the messages in as their agents get to their prompts.
func (d *Daemon) deliverLoop() {
	d.mail.mu.Lock()
	d.mail.initLocked()
	wake := d.mail.wake
	d.mail.mu.Unlock()
	t := time.NewTicker(deliverTick)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-wake:
		case <-d.quit:
			return
		}
		d.flushOutbox(time.Now())
	}
}

// flushOutbox types into each pane whose agent is ready what waits for
// it, as one prompt; and lets go of what waits for a pane that closed.
func (d *Daemon) flushOutbox(now time.Time) {
	mb := &d.mail
	mb.flush.Lock()
	defer mb.flush.Unlock()
	mb.mu.Lock()
	targets := make([]string, 0, len(mb.out))
	for id, msgs := range mb.out {
		if len(msgs) > 0 {
			targets = append(targets, id)
		}
	}
	mb.mu.Unlock()
	for _, id := range targets {
		d.mu.Lock()
		pn := d.findPaneLocked(id)
		d.mu.Unlock()
		mb.mu.Lock()
		typed, since, lost := mb.typed[id], mb.since[id], mb.noAgent[id]
		mb.mu.Unlock()
		ready, gone, why := false, pn == nil, "the agent's pane closed"
		if pn != nil {
			pn.mu.Lock()
			// Left alone after typing, until its agent has worked on it
			// (its status changed since) or deliverHold passed.
			ready = pn.agentReady() && (typed.IsZero() || pn.info.Since != since || now.Sub(typed) >= deliverHold)
			gone = pn.closed
			noAgent := pn.info.Agent == "" || pn.info.Status == proto.Exited
			pn.mu.Unlock()
			// An agent that quit gets none of it, nor does the next one
			// started there, once it has been gone a while.
			switch {
			case !noAgent:
				lost = time.Time{}
			case lost.IsZero():
				lost = now
			case now.Sub(lost) >= agentGone:
				gone, why = true, "the agent is no longer running"
			}
			mb.mu.Lock()
			if lost.IsZero() {
				delete(mb.noAgent, id)
			} else {
				mb.noAgent[id] = lost
			}
			mb.mu.Unlock()
		}
		if !ready && !gone {
			continue
		}
		var as proto.AgentSession
		native := false
		if !gone {
			_, as, native = nativeSession(pn)
		}
		// Whether a listener of the agent takes them, or one between two
		// waits will, is told under the same hold of mb.mu as they are
		// taken, so that one stopping meanwhile can't make them typed in.
		mb.mu.Lock()
		waiting, between := mb.listenerLocked(id, now)
		if !gone && (between || mb.pending[id]) {
			mb.mu.Unlock()
			continue
		}
		native = native && !waiting && mb.noNative[id] != as.ID
		msgs := mb.out[id]
		delete(mb.out, id)
		delete(mb.noAgent, id)
		if gone {
			delete(mb.typed, id)
			delete(mb.since, id)
		}
		texts := make([]string, len(msgs))
		for i, m := range msgs {
			texts[i] = m.text
		}
		text := strings.Join(texts, "\n\n")
		switch {
		case gone || len(msgs) == 0:
		case waiting:
			// The newest listener, as one of an agent restarted in the pane
			// replaces the old one's, which may outlive its agent.
			ws := mb.inbox[id]
			last := len(ws) - 1
			ws[last] <- text // its room for one, which nothing else fills
			mb.setInboxLocked(id, ws[:last])
		case native:
			mb.pending[id] = true
		}
		mb.mu.Unlock()
		if len(msgs) == 0 {
			continue
		}
		if gone {
			for _, m := range msgs {
				d.tellDelivery(proto.Delivery{ID: m.id, From: m.from, To: id, State: "failed", Error: why})
			}
			continue
		}
		if native && !waiting {
			// A program's run or a request, which may take seconds: the
			// other agents' messages don't wait for it.
			go d.deliverNative(pn, id, msgs, text)
			continue
		}
		if !waiting {
			pn.paste(text)
			go pn.submit(now)
		}
		d.delivered(pn, id, msgs, now)
	}
}

// delivered records that msgs reached the agent of pane pn, id, at now:
// the agent is left alone till it worked on them (deliverHold), and the
// apps hear of it.
func (d *Daemon) delivered(pn *pane, id string, msgs []outMsg, now time.Time) {
	pn.mu.Lock()
	after := pn.info.Since
	pn.mu.Unlock()
	mb := &d.mail
	mb.mu.Lock()
	mb.typed[id], mb.since[id] = now, after
	mb.mu.Unlock()
	for _, m := range msgs {
		d.tellDelivery(proto.Delivery{ID: m.id, From: m.from, To: id, State: "delivered"})
	}
}

// deliverNative gives msgs to the agent of pane pn, id, through its own way
// in (native.go). When that fails, they wait again, first, and the agent's
// session gets its messages typed in from then on, by the next flush, as
// the agent is still at its prompt then.
func (d *Daemon) deliverNative(pn *pane, id string, msgs []outMsg, text string) {
	_, as, _ := nativeSession(pn)
	ok := d.sendNative(pn, text)
	mb := &d.mail
	mb.mu.Lock()
	delete(mb.pending, id)
	if !ok {
		mb.out[id] = append(slices.Clone(msgs), mb.out[id]...)
		mb.noNative[id] = as.ID
	}
	mb.mu.Unlock()
	if ok {
		d.delivered(pn, id, msgs, time.Now())
		return
	}
	select {
	case mb.wake <- struct{}{}:
	default:
	}
}

// tellDelivery tells the apps how a message went: one from no pane only
// when it failed, for the user to hear of. Best effort: an app far
// behind drops events, as it does notices.
func (d *Daemon) tellDelivery(dl proto.Delivery) {
	if dl.From == "" && dl.State != "failed" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for c := range d.subs {
		c.event(proto.Message{Event: "delivery", Delivery: &dl})
	}
}

// waitMessage waits for the next message from a browser pane's page, as
// kero browser wait-message does, and takes it.
func (d *Daemon) waitMessage(c *controlConn, pn *pane, timeout time.Duration) (proto.AgentMessage, error) {
	pn.mu.Lock()
	id, browser, closed := pn.info.ID, pn.info.Browser(), pn.closed
	pn.mu.Unlock()
	if !browser {
		return proto.AgentMessage{}, fmt.Errorf("pane %s is a terminal, not a browser", id)
	}
	if closed {
		return proto.AgentMessage{}, errors.New("pane closed")
	}
	mb := &d.mail
	mb.mu.Lock()
	mb.initLocked()
	if kept := mb.kept[id]; len(kept) > 0 {
		msg := kept[0]
		mb.kept[id] = kept[1:]
		mb.mu.Unlock()
		d.tellDelivery(proto.Delivery{ID: msg.ID, From: id, State: "delivered"})
		return msg, nil
	}
	ch := make(chan proto.AgentMessage, 1)
	mb.waiters[id] = append(mb.waiters[id], ch)
	mb.mu.Unlock()
	// The pane may have closed, and let go of its waiters, since.
	pn.mu.Lock()
	closed = pn.closed
	pn.mu.Unlock()
	if closed {
		d.mailGone(id)
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
	select {
	case msg, ok := <-ch:
		if !ok {
			return proto.AgentMessage{}, errors.New("pane closed")
		}
		d.tellDelivery(proto.Delivery{ID: msg.ID, From: id, State: "delivered"})
		return msg, nil
	case <-expired:
		why = errors.New("timed out")
	case <-done:
		why = errors.New("canceled")
	case <-d.quit:
		why = errors.New("kerod exited")
	}
	// No longer waiting; a message handed over meanwhile goes back first.
	mb.mu.Lock()
	mb.waiters[id] = slices.DeleteFunc(mb.waiters[id], func(w chan proto.AgentMessage) bool { return w == ch })
	select {
	case msg, ok := <-ch:
		if ok {
			mb.kept[id] = append([]proto.AgentMessage{msg}, mb.kept[id]...)
		}
	default:
	}
	mb.mu.Unlock()
	return proto.AgentMessage{}, why
}

// mailGone lets go of a closed pane's messages: those it was to take fail,
// those of its page go, and agents waiting for them hear it closed.
func (d *Daemon) mailGone(id string) {
	mb := &d.mail
	mb.mu.Lock()
	out := mb.out[id]
	delete(mb.out, id)
	delete(mb.typed, id)
	delete(mb.since, id)
	delete(mb.noAgent, id)
	delete(mb.kept, id)
	for _, w := range mb.waiters[id] {
		close(w)
	}
	delete(mb.waiters, id)
	for _, w := range mb.inbox[id] {
		close(w)
	}
	delete(mb.inbox, id)
	delete(mb.heard, id)
	delete(mb.pending, id)
	delete(mb.noNative, id)
	mb.mu.Unlock()
	for _, m := range out {
		d.tellDelivery(proto.Delivery{ID: m.id, From: m.from, To: id, State: "failed", Error: "the agent's pane closed"})
	}
}
