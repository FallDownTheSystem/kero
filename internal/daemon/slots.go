package daemon

import (
	"errors"
	"slices"
	"sync"
	"time"

	"kero/internal/proto"
	"kero/internal/settings"
)

// slots are heavy-slots' (settings.HeavySlots): how many builds and test
// runs agents run at once, over the machine. kero hook asks for one as an
// agent's tool call starts a build (package heavy), and waits, the tool
// call with it, while all are held; the call's end gives it back, or the
// turn's, or the pane's, or slotMaxHold, should no end come. kero lock
// heavy holds one while its connection stays.
//
// Nothing waits for ever: the hook asks for a while short of its own
// timeout, and runs the command anyway after it.
type slots struct {
	mu    sync.Mutex
	held  []*slot
	queue []*slot // the oldest first

	// markMu keeps the panes' SlotWait, as last set, in step: marked.
	markMu sync.Mutex
	marked map[string]int
}

type slot struct {
	pane, key string
	conn      bool // held while its connection stays
	since     time.Time
	got       chan struct{} // closed as it is held
}

// slotMaxHold is the longest a slot is held: a build whose end kero never
// hears of, as one an agent sent to the background, gives its slot back
// after it.
var slotMaxHold = 30 * time.Minute

// slotTick is how often a waiter looks again, for a slot that went for
// being held too long, or a setting that gives more.
var slotTick = 15 * time.Second

// takeSlot waits for a slot of heavy-slots for a pane's build, as p asks.
func (d *Daemon) takeSlot(c *controlConn, p proto.SlotParams) (proto.SlotResult, error) {
	pn := d.slotPane(p)
	if settings.Load().HeavySlots <= 0 || pn == nil {
		return proto.SlotResult{Off: true}, nil
	}
	start := time.Now()
	s := &d.slots
	if p.Conn {
		// kero lock inside a build the pane's hook holds, as a script's:
		// it builds under that build's slot, or it would wait on itself.
		s.mu.Lock()
		inside := slices.ContainsFunc(s.held, func(o *slot) bool { return o.pane == pn.info.ID && !o.conn })
		s.mu.Unlock()
		if inside {
			return proto.SlotResult{Got: true}, nil
		}
	}
	sl := &slot{pane: pn.info.ID, key: p.Key, conn: p.Conn, got: make(chan struct{})}
	s.mu.Lock()
	s.queue = append(s.queue, sl)
	s.grantLocked(start)
	s.mu.Unlock()
	d.markSlots()

	var timeout <-chan time.Time
	if p.Wait > 0 {
		t := time.NewTimer(time.Duration(p.Wait) * time.Millisecond)
		defer t.Stop()
		timeout = t.C
	}
	tick := time.NewTicker(slotTick)
	defer tick.Stop()
	for {
		select {
		case <-sl.got:
			if sl.conn {
				go func() {
					select {
					case <-c.done:
					case <-d.quit:
					}
					d.dropSlots(false, func(o *slot) bool { return o == sl })
				}()
			}
			return proto.SlotResult{Got: true, Waited: time.Since(start).Milliseconds()}, nil
		case <-tick.C:
			s.mu.Lock()
			s.grantLocked(time.Now())
			s.mu.Unlock()
			d.markSlots()
		case <-timeout:
			// Held as the wait ran out: it is the caller's all the same.
			if d.dropWaiting(sl) {
				return proto.SlotResult{Got: true, Waited: time.Since(start).Milliseconds()}, nil
			}
			return proto.SlotResult{Waited: time.Since(start).Milliseconds()}, nil
		case <-c.done:
			d.dropSlots(true, func(o *slot) bool { return o == sl })
			return proto.SlotResult{}, errors.New("canceled")
		case <-d.quit:
			return proto.SlotResult{}, errors.New("kerod exited")
		}
	}
}

// slotPane is the pane whose agent asks for a slot: the one that runs
// its session, else the one its hook ran in, else as pane.agentSession
// finds it.
func (d *Daemon) slotPane(p proto.SlotParams) *pane {
	if p.Agent != "" && p.Session != "" {
		if ids, ok := d.runningConversations()[proto.AgentSession{Agent: p.Agent, ID: p.Session}]; ok {
			if pn := d.findPane(ids[1]); pn != nil {
				return pn
			}
		}
	}
	if p.Pane != "" {
		if pn := d.findPane(p.Pane); pn != nil {
			return pn
		}
	}
	if p.PID != 0 && p.Agent != "" {
		pn, _ := d.agentPane(p.PID, p.Agent, p.Dir)
		return pn
	}
	return nil
}

// grantLocked gives the slots free, those held too long first freed, to
// those waiting longest; s.mu is held.
func (s *slots) grantLocked(now time.Time) {
	s.held = slices.DeleteFunc(s.held, func(o *slot) bool { return now.Sub(o.since) > slotMaxHold })
	limit := settings.Load().HeavySlots
	for len(s.queue) > 0 && (limit <= 0 || len(s.held) < limit) {
		sl := s.queue[0]
		s.queue = s.queue[1:]
		sl.since = now
		s.held = append(s.held, sl)
		close(sl.got)
	}
}

// dropWaiting takes sl out of the queue, as its wait runs out, reporting
// whether it was held already.
func (d *Daemon) dropWaiting(sl *slot) (held bool) {
	s := &d.slots
	s.mu.Lock()
	held = slices.Contains(s.held, sl)
	s.queue = slices.DeleteFunc(s.queue, func(o *slot) bool { return o == sl })
	s.mu.Unlock()
	d.markSlots()
	return held
}

// dropSlots gives back the slots held that drop says, and the waits for
// one too when waits, and hands them on. A turn's end leaves the waits:
// a tool call still waiting, as a subagent's, goes as its hook does.
func (d *Daemon) dropSlots(waits bool, drop func(*slot) bool) {
	s := &d.slots
	s.mu.Lock()
	s.held = slices.DeleteFunc(s.held, drop)
	if waits {
		s.queue = slices.DeleteFunc(s.queue, drop)
	}
	s.grantLocked(time.Now())
	s.mu.Unlock()
	d.markSlots()
}

// releaseSlot gives back the slot of an agent's tool call, as it ends.
func (d *Daemon) releaseSlot(key string) {
	if key != "" {
		d.dropSlots(false, func(o *slot) bool { return o.key == key && !o.conn })
	}
}

// releaseTurnSlots gives back the slots a pane's agent's tool calls
// held, as its turn ends or the agent exits: what runs on after it, as a
// build sent to the background, runs without one. kero lock's stay.
func (d *Daemon) releaseTurnSlots(pane string) {
	d.dropSlots(false, func(o *slot) bool { return o.pane == pane && !o.conn })
}

// releasePaneSlots gives back all of a pane's slots, as it closes or its
// shell exits.
func (d *Daemon) releasePaneSlots(pane string) {
	d.dropSlots(true, func(o *slot) bool { return o.pane == pane })
}

// markSlots sets each pane's place in the queue (proto.Pane.SlotWait),
// for the sidebar.
func (d *Daemon) markSlots() {
	s := &d.slots
	s.markMu.Lock()
	defer s.markMu.Unlock()
	s.mu.Lock()
	places := map[string]int{}
	for i, sl := range s.queue {
		if places[sl.pane] == 0 {
			places[sl.pane] = i + 1
		}
	}
	s.mu.Unlock()
	changed := false
	for id := range s.marked {
		if places[id] == 0 {
			places[id] = 0 // to clear
		}
	}
	for id, n := range places {
		if s.marked[id] == n {
			continue
		}
		if pn := d.findPane(id); pn != nil {
			pn.mu.Lock()
			pn.info.SlotWait = n
			pn.mu.Unlock()
			changed = true
		}
	}
	for id, n := range places {
		if n == 0 {
			delete(places, id)
		}
	}
	s.marked = places
	if changed {
		d.changed()
	}
}
