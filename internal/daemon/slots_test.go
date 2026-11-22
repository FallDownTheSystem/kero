package daemon

import (
	"testing"
	"time"

	"kero/internal/proto"
	"kero/internal/settings"
)

// slotDaemon is a daemon with heavy-slots at slots, and panes a, b and c.
func slotDaemon(t *testing.T, slots int) *Daemon {
	t.Helper()
	d := testDaemon(t)
	set := settings.Default()
	set.HeavySlots = slots
	if err := settings.Save(set); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		p, err := newPane(d, "s", proto.Pane{ID: id, Status: proto.Working, Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.term.Close() })
		d.panes[id] = p
	}
	return d
}

type slotTake struct {
	res proto.SlotResult
	err error
}

// take asks for a slot for pane, as a tool call key, in the background.
func take(d *Daemon, c *controlConn, pane, key string, wait time.Duration) chan slotTake {
	ch := make(chan slotTake, 1)
	go func() {
		res, err := d.takeSlot(c, proto.SlotParams{Pane: pane, Key: key, Wait: wait.Milliseconds()})
		ch <- slotTake{res, err}
	}()
	return ch
}

func got(t *testing.T, ch chan slotTake) proto.SlotResult {
	t.Helper()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.res
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting for a slot")
	}
	return proto.SlotResult{}
}

func waiting(t *testing.T, ch chan slotTake) {
	t.Helper()
	select {
	case r := <-ch:
		t.Fatalf("got %+v, want it still waiting", r)
	case <-time.After(100 * time.Millisecond):
	}
}

func slotWait(d *Daemon, pane string) int {
	p := d.findPane(pane)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info.SlotWait
}

// With two slots, a third build waits, its pane marked as next, and gets
// the slot the first one's end gives back, not the second's.
func TestSlotsQueue(t *testing.T) {
	d := slotDaemon(t, 2)
	c := &controlConn{done: make(chan struct{})}
	if r := got(t, take(d, c, "a", "k1", 0)); !r.Got {
		t.Fatalf("first: %+v", r)
	}
	got(t, take(d, c, "b", "k2", 0))
	third := take(d, c, "c", "k3", 0)
	waiting(t, third)
	if n := slotWait(d, "c"); n != 1 {
		t.Fatalf("c's place %d, want 1", n)
	}
	fourth := take(d, c, "a", "k4", 0)
	waiting(t, fourth)
	if n := slotWait(d, "a"); n != 2 {
		t.Fatalf("a's place %d, want 2", n)
	}
	d.releaseSlot("k1")
	if r := got(t, third); !r.Got || r.Waited < 100 {
		t.Fatalf("third: %+v", r)
	}
	waiting(t, fourth)
	if slotWait(d, "c") != 0 || slotWait(d, "a") != 1 {
		t.Fatalf("places c %d a %d, want 0 and 1", slotWait(d, "c"), slotWait(d, "a"))
	}
	d.releaseSlot("nobody's") // gives back nothing
	waiting(t, fourth)
	d.releaseSlot("k2")
	got(t, fourth)
}

// A wait that runs out leaves the queue, and the next one in it moves up;
// a waiter whose connection closes, as a hook the agent stopped, too.
func TestSlotsWaitEnds(t *testing.T) {
	d := slotDaemon(t, 1)
	c := &controlConn{done: make(chan struct{})}
	got(t, take(d, c, "a", "k1", 0))
	if r := got(t, take(d, c, "b", "k2", 150*time.Millisecond)); r.Got || r.Off || r.Waited < 150 {
		t.Fatalf("ran out: %+v", r)
	}
	gone := &controlConn{done: make(chan struct{})}
	stopped := take(d, gone, "b", "k3", 0)
	later := take(d, c, "c", "k4", 0)
	waiting(t, later)
	close(gone.done)
	if r := <-stopped; r.err == nil {
		t.Fatalf("a closed connection's wait: %+v", r.res)
	}
	if n := slotWait(d, "c"); n != 1 {
		t.Fatalf("c's place %d, want 1 once b's left", n)
	}
	d.releaseSlot("k1")
	got(t, later)
}

// A turn's end gives back its tool calls' slots, kero lock's stays until
// its connection closes, and a pane's close gives back all of its own.
func TestSlotsTurnAndConn(t *testing.T) {
	d := slotDaemon(t, 1)
	c := &controlConn{done: make(chan struct{})}
	got(t, take(d, c, "a", "k1", 0))
	next := take(d, c, "b", "k2", 0)
	waiting(t, next)
	d.releaseTurnSlots("b") // another pane's turn
	waiting(t, next)
	d.releaseTurnSlots("a")
	got(t, next)

	lock := &controlConn{done: make(chan struct{})}
	d.releaseSlot("k2")
	res, err := d.takeSlot(lock, proto.SlotParams{Pane: "c", Conn: true})
	if err != nil || !res.Got {
		t.Fatal(res, err)
	}
	after := take(d, c, "a", "k5", 0)
	d.releaseTurnSlots("c")
	waiting(t, after)
	close(lock.done)
	got(t, after)

	again := take(d, c, "b", "k6", 0)
	waiting(t, again)
	d.releasePaneSlots("a")
	got(t, again)
}

// A slot held past slotMaxHold, as by a build whose end never came, goes
// to the next that waits.
func TestSlotsMaxHold(t *testing.T) {
	d := slotDaemon(t, 1)
	defer func(h, tk time.Duration) { slotMaxHold, slotTick = h, tk }(slotMaxHold, slotTick)
	slotMaxHold, slotTick = 200*time.Millisecond, 50*time.Millisecond
	c := &controlConn{done: make(chan struct{})}
	got(t, take(d, c, "a", "k1", 0))
	if r := got(t, take(d, c, "b", "k2", 0)); r.Waited < 150 {
		t.Fatalf("got the slot after %dms, before the first was held too long", r.Waited)
	}
}

// heavy-slots = 0, or a pane Kero doesn't know, holds nothing.
func TestSlotsOff(t *testing.T) {
	d := slotDaemon(t, 0)
	c := &controlConn{done: make(chan struct{})}
	if r := got(t, take(d, c, "a", "k1", 0)); !r.Off {
		t.Fatalf("off: %+v", r)
	}
	d = slotDaemon(t, 1)
	if r := got(t, take(d, c, "zz", "k1", 0)); !r.Off {
		t.Fatalf("no pane: %+v", r)
	}
}
