package daemon

import (
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// turnRun follows one child, "s1" of orchestrator "o", through the
// statuses and turn ends a test gives it, at the times it gives.
type turnRun struct {
	t   *testing.T
	r   *reports
	now time.Time
}

func newTurnRun(t *testing.T) *turnRun {
	r := &reports{}
	r.initLocked()
	tr := &turnRun{t: t, r: r, now: time.Unix(1000, 0)}
	tr.screen(proto.Idle, 0)
	return tr
}

// screen shows status for d, looked at every reportTick.
func (tr *turnRun) screen(status proto.Status, d time.Duration) {
	end := tr.now.Add(d)
	for {
		tr.r.followLocked("s1", childNow{parent: "o", name: "alpha", status: status}, tr.now)
		if !tr.now.Before(end) {
			return
		}
		tr.now = tr.now.Add(reportTick)
	}
}

func (tr *turnRun) end(p proto.TurnParams) {
	tr.r.endTurnLocked("s1", p, tr.now)
}

// lines takes the lines reported so far.
func (tr *turnRun) lines() []string {
	rp := tr.r.out["o"]
	delete(tr.r.out, "o")
	if rp == nil {
		return nil
	}
	var out []string
	for _, l := range rp.lines {
		out = append(out, l.text)
	}
	return out
}

func (tr *turnRun) want(n int, has ...string) {
	tr.t.Helper()
	got := tr.lines()
	if len(got) != n {
		tr.t.Fatalf("%d lines, want %d: %q", len(got), n, got)
	}
	for _, h := range has {
		if !strings.Contains(strings.Join(got, "\n"), h) {
			tr.t.Fatalf("no %q in %q", h, got)
		}
	}
}

var done = proto.TurnParams{End: proto.TurnDone, Reply: "Done. Push it?", HasReply: true}

// The end of a turn its agent tells goes once, with the reply the agent
// told, whether the screen shows it before the hook, after it, or later.
func TestTurnEndReported(t *testing.T) {
	tr := newTurnRun(t)
	tr.screen(proto.Working, 3*time.Second)
	tr.end(done)
	tr.screen(proto.Idle, 12*time.Second)
	tr.want(1, "finished its turn: it is idle. Its reply:\n  Done. Push it?")

	// The screen lags: still working as the hook tells the end.
	tr.screen(proto.Working, 3*time.Second)
	tr.end(proto.TurnParams{End: proto.TurnDone, Reply: "Second.", HasReply: true})
	tr.screen(proto.Working, 3*time.Second)
	tr.want(1, "Second.")
	tr.screen(proto.Idle, 12*time.Second)
	tr.want(0)

	// Background work as the agent told it.
	tr.screen(proto.Working, 3*time.Second)
	tr.end(proto.TurnParams{End: proto.TurnDone, Reply: "Watching.", HasReply: true, Background: "1 shell", HasBackground: true})
	tr.screen(proto.Idle, 3*time.Second)
	tr.want(1, "it is idle, but 1 shell still running. Its reply:")
}

// A stop hook of the agent's that goes on with the turn: the first end is
// none; the last goes.
func TestTurnEndContinued(t *testing.T) {
	tr := newTurnRun(t)
	tr.screen(proto.Working, 3*time.Second)
	tr.end(proto.TurnParams{End: proto.TurnDone, Reply: "Not yet.", HasReply: true})
	tr.screen(proto.Idle, time.Second)
	tr.screen(proto.Working, 5*time.Second)
	tr.end(done)
	tr.screen(proto.Idle, 12*time.Second)
	tr.want(1, "Push it?")
}

// A turn interrupted, which the agent tells no end of: the screen tells
// it, once the hook had time to.
func TestTurnEndInterrupted(t *testing.T) {
	tr := newTurnRun(t)
	tr.screen(proto.Working, 3*time.Second)
	tr.end(done)
	tr.screen(proto.Idle, 3*time.Second)
	tr.want(1)
	tr.screen(proto.Working, 3*time.Second)
	tr.screen(proto.Idle, replyWait-reportTick)
	tr.want(0)
	tr.screen(proto.Idle, time.Second)
	tr.want(1, "finished its turn: it is idle.")

	tr.screen(proto.Working, 3*time.Second)
	tr.end(proto.TurnParams{End: proto.TurnInterrupted})
	tr.screen(proto.Idle, 12*time.Second)
	tr.want(1, "its turn was interrupted")
}

// A turn that failed tells the error, once, though the screen fails too.
func TestTurnEndFailed(t *testing.T) {
	tr := newTurnRun(t)
	tr.screen(proto.Working, 3*time.Second)
	tr.end(proto.TurnParams{End: proto.TurnFailed, Error: "API Error: Rate limit reached"})
	tr.screen(proto.Failed, 12*time.Second)
	tr.want(1, `s1 "alpha" failed: API Error: Rate limit reached`)
}

// A turn with no words, as a compaction's, is no news.
func TestTurnEndNoReply(t *testing.T) {
	tr := newTurnRun(t)
	tr.screen(proto.Working, 3*time.Second)
	tr.end(proto.TurnParams{End: proto.TurnDone, HasReply: true})
	tr.screen(proto.Idle, 12*time.Second)
	tr.want(0)
}

// Of an agent that tells no turn ends, the screen tells them, as before.
func TestTurnEndScreenOnly(t *testing.T) {
	tr := newTurnRun(t)
	tr.screen(proto.Working, 3*time.Second)
	tr.screen(proto.Idle, 3*time.Second)
	tr.want(1, "finished its turn: it is idle.")
	// A hook late after the screen told the end: that turn's.
	tr.end(done)
	tr.screen(proto.Idle, 3*time.Second)
	tr.want(0)
}
