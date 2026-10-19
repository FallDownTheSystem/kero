package daemon

import "time"

const (
	// workFor: output is work once it has kept coming this long, as a
	// spinner's, or an agent's streaming answer.
	workFor = time.Second
	// beatEvery: writes this far apart are separate redraws; one
	// redraw read in pieces is one beat.
	beatEvery = 100 * time.Millisecond
)

// workMeter tells a program working from a program redrawing once: a
// toast going away, a selection, a clock. Work is output that keeps
// coming, more than a few bytes, in separate redraws over workFor, with
// no pause of quietAfter.
type workMeter struct {
	start, last, beat time.Time
	bytes, beats      int
}

// add counts n bytes written at now.
func (w *workMeter) add(now time.Time, n int) {
	if now.Sub(w.last) > quietAfter {
		*w = workMeter{start: now}
	}
	w.last = now
	w.bytes += n
	if now.Sub(w.beat) >= beatEvery {
		w.beats++
		w.beat = now
	}
}

// working reports whether what was counted is work.
func (w *workMeter) working() bool {
	return w.bytes > 48 && w.beats >= 3 && w.last.Sub(w.start) >= workFor
}

// reset forgets what was counted.
func (w *workMeter) reset() { *w = workMeter{} }
