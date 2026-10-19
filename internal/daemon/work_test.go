package daemon

import (
	"testing"
	"time"
)

func TestWorkMeter(t *testing.T) {
	t0 := time.Unix(1000, 0)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	cases := []struct {
		name   string
		writes [][2]int // ms, bytes
		want   bool
	}{
		// A TUI's toast going away: one redraw, read in pieces.
		{"one redraw", [][2]int{{0, 4096}, {2, 4096}, {5, 900}}, false},
		// A toast shown, then gone three seconds later.
		{"toast in and out", [][2]int{{0, 600}, {3000, 600}}, false},
		// A clock in a prompt, a few bytes a second.
		{"clock", [][2]int{{0, 8}, {1000, 8}, {2000, 8}, {3000, 8}}, false},
		// A spinner, ten frames a second.
		{"spinner", [][2]int{{0, 30}, {100, 30}, {200, 30}, {300, 30}, {400, 30}, {500, 30}, {600, 30}, {700, 30}, {800, 30}, {900, 30}, {1000, 30}}, true},
		// An agent streaming its answer.
		{"streaming", [][2]int{{0, 200}, {400, 300}, {800, 250}, {1200, 400}}, true},
		// Two redraws apart, too few to be work.
		{"two redraws", [][2]int{{0, 500}, {1500, 500}}, false},
	}
	for _, c := range cases {
		var w workMeter
		for _, wr := range c.writes {
			w.add(at(wr[0]), wr[1])
		}
		if got := w.working(); got != c.want {
			t.Errorf("%s: working %v, want %v", c.name, got, c.want)
		}
	}
}
