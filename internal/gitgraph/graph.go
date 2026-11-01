// Package gitgraph lays out a history's graph as git log --graph draws
// it: each commit a node in a lane, lines down from it to its parents'
// lanes, and the lanes of other commits' parents passing beside it. A
// Layout takes the commits a page at a time, children before parents, and
// gives each its row.
package gitgraph

// Commit is what the layout takes of a commit.
type Commit struct {
	Hash    string
	Parents []string
}

// Line is a line of a row: from a lane at the row's top or its node, to a
// lane at its bottom or its node; Color is its lane's.
type Line struct {
	From, To int
	Color    int
}

// Row is a commit's row of the graph. Lanes count from 0 at the left.
type Row struct {
	Node  int // the commit's lane
	Color int // its color, as its lane's
	// Pass are the lanes going by the node, top to bottom: From and To
	// are the same lane. In come from the top into the node, From their
	// lane; Out go from the node to the bottom, To their lane.
	Pass []Line
	In   []Line
	Out  []Line
	// Width is the lanes the row takes: one past the rightmost it uses.
	Width int
}

// lane is an open lane: the commit it leads down to, and its color.
type lane struct {
	hash  string
	color int
	open  bool
}

// Layout lays out a history a commit at a time; its zero value is ready
// for the first.
type Layout struct {
	lanes  []lane
	colors int // the colors given out, which the next lane's follows
}

// Colors is how many colors lanes take turns at.
const Colors = 8

// Add lays out the commit that comes next and returns its row.
func (l *Layout) Add(c Commit) Row {
	var r Row
	// The lanes leading down to this commit: its children's. The first
	// takes it; the others end in it.
	r.Node = -1
	for i, ln := range l.lanes {
		if !ln.open || ln.hash != c.Hash {
			continue
		}
		r.In = append(r.In, Line{From: i, Color: ln.color})
		if r.Node < 0 {
			r.Node, r.Color = i, ln.color
		} else {
			l.lanes[i].open = false
		}
	}
	if r.Node < 0 {
		// A branch's tip: no child laid out leads to it.
		r.Node = l.free(-1)
		r.Color = l.nextColor()
		l.lanes[r.Node] = lane{color: r.Color, open: true}
	}
	for i := range r.In {
		r.In[i].To = r.Node
	}
	for i, ln := range l.lanes {
		if ln.open && i != r.Node && ln.hash != c.Hash {
			r.Pass = append(r.Pass, Line{From: i, To: i, Color: ln.color})
		}
	}

	if len(c.Parents) == 0 {
		// A root: its lane ends here.
		l.lanes[r.Node].open = false
	} else {
		l.lanes[r.Node].hash = c.Parents[0]
		r.Out = append(r.Out, Line{From: r.Node, To: r.Node, Color: r.Color})
		for _, p := range c.Parents[1:] {
			// A parent another lane leads down to already is joined
			// there; another gets a lane of its own.
			j := -1
			for i, ln := range l.lanes {
				if ln.open && ln.hash == p && i != r.Node {
					j = i
					break
				}
			}
			if j < 0 {
				j = l.free(r.Node)
				l.lanes[j] = lane{hash: p, color: l.nextColor(), open: true}
			}
			r.Out = append(r.Out, Line{From: r.Node, To: j, Color: l.lanes[j].color})
		}
	}

	r.Width = r.Node + 1
	for _, ls := range [][]Line{r.Pass, r.In, r.Out} {
		for _, ln := range ls {
			r.Width = max(r.Width, ln.From+1, ln.To+1)
		}
	}
	for len(l.lanes) > 0 && !l.lanes[len(l.lanes)-1].open {
		l.lanes = l.lanes[:len(l.lanes)-1]
	}
	return r
}

// free returns the leftmost lane that is free, but not, adding one when
// none is.
func (l *Layout) free(not int) int {
	for i, ln := range l.lanes {
		if !ln.open && i != not {
			return i
		}
	}
	l.lanes = append(l.lanes, lane{})
	return len(l.lanes) - 1
}

func (l *Layout) nextColor() int {
	c := l.colors % Colors
	l.colors++
	return c
}
