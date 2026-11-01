package gitgraph

import (
	"fmt"
	"strings"
	"testing"
)

// draw lays out commits, "hash:parent,parent", and prints each row as
// its node's lane, then its lines in, by, and out.
func draw(commits ...string) string {
	var l Layout
	var b strings.Builder
	for _, s := range commits {
		hash, parents, _ := strings.Cut(s, ":")
		c := Commit{Hash: hash}
		if parents != "" {
			c.Parents = strings.Split(parents, ",")
		}
		r := l.Add(c)
		fmt.Fprintf(&b, "%s@%d in%s by%s out%s w%d\n", hash, r.Node, lanes(r.In, true), lanes(r.Pass, true), lanes(r.Out, false), r.Width)
	}
	return b.String()
}

func lanes(ls []Line, from bool) string {
	var s []string
	for _, l := range ls {
		if from {
			s = append(s, fmt.Sprint(l.From))
		} else {
			s = append(s, fmt.Sprint(l.To))
		}
	}
	return "[" + strings.Join(s, " ") + "]"
}

func TestLinear(t *testing.T) {
	got := draw("c:b", "b:a", "a:")
	want := "c@0 in[] by[] out[0] w1\nb@0 in[0] by[] out[0] w1\na@0 in[0] by[] out[] w1\n"
	if got != want {
		t.Errorf("got\n%swant\n%s", got, want)
	}
}

func TestMerge(t *testing.T) {
	// m merges s into the main line; s and x both come from a.
	//   m
	//   |\
	//   x s
	//   |/
	//   a
	got := draw("m:x,s", "x:a", "s:a", "a:")
	want := "m@0 in[] by[] out[0 1] w2\n" +
		"x@0 in[0] by[1] out[0] w2\n" +
		"s@1 in[1] by[0] out[1] w2\n" +
		"a@0 in[0 1] by[] out[] w2\n"
	if got != want {
		t.Errorf("got\n%swant\n%s", got, want)
	}
}

func TestTips(t *testing.T) {
	// Two branches' tips, f and g, on b; the lane g's ends in frees for d,
	// a tip later on.
	got := draw("f:b", "g:b", "b:a", "d:a", "a:")
	want := "f@0 in[] by[] out[0] w1\n" +
		"g@1 in[] by[0] out[1] w2\n" +
		"b@0 in[0 1] by[] out[0] w2\n" +
		"d@1 in[] by[0] out[1] w2\n" +
		"a@0 in[0 1] by[] out[] w2\n"
	if got != want {
		t.Errorf("got\n%swant\n%s", got, want)
	}
}

func TestColors(t *testing.T) {
	var l Layout
	a := l.Add(Commit{Hash: "m", Parents: []string{"x", "s"}})
	if a.Out[0].Color == a.Out[1].Color {
		t.Error("a merge's two lanes have one color")
	}
	b := l.Add(Commit{Hash: "x", Parents: []string{"r"}})
	if b.Color != a.Color || b.Pass[0].Color != a.Out[1].Color {
		t.Errorf("lanes changed colors: %+v then %+v", a, b)
	}
}
