package proto

import (
	"slices"
	"testing"
)

func TestLayoutSplitRemove(t *testing.T) {
	var l *Layout
	l = l.Split("a", "b", "right", 0)
	l = l.Split("b", "c", "down", 0)
	l = l.Split("a", "d", "left", 0)
	if got := l.Panes(); !slices.Equal(got, []string{"d", "a", "b", "c"}) {
		t.Fatalf("panes %v", got)
	}
	if n := l.Neighbor("a", "right"); n != "b" && n != "c" {
		t.Errorf("right of a: %q", n)
	}
	if n := l.Neighbor("b", "down"); n != "c" {
		t.Errorf("below b: %q", n)
	}
	if n := l.Neighbor("d", "left"); n != "" {
		t.Errorf("left of d: %q", n)
	}
	l = l.Remove("b")
	if got := l.Panes(); !slices.Equal(got, []string{"d", "a", "c"}) {
		t.Fatalf("after removing b: %v", got)
	}
	l = l.Remove("d").Remove("c")
	if !l.Leaf() || l.Pane != "a" {
		t.Fatalf("last pane: %+v", l)
	}
	if l.Remove("a") != nil {
		t.Fatal("no pane left, a layout remains")
	}
}

func TestLead(t *testing.T) {
	panes := []Pane{{ID: "a", Status: Working}, {ID: "b", Status: Idle, Unread: true}, {ID: "c", Status: Waiting}}
	if i := Lead(panes, "a"); panes[i].ID != "c" {
		t.Fatalf("lead %s, want the one waiting", panes[i].ID)
	}
	panes[2].Status = Idle
	if i := Lead(panes, "a"); panes[i].ID != "b" {
		t.Fatalf("lead %s, want the unread one", panes[i].ID)
	}
}

func TestLayoutMoveSwap(t *testing.T) {
	var l *Layout
	l = l.Split("a", "b", "right", 0).Split("b", "c", "down", 0) // a | (b / c)
	m := l.Move("c", "a", "left")                                // c | a | b
	if got := m.Panes(); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Fatalf("move: %v", got)
	}
	if n := m.Neighbor("a", "left"); n != "c" {
		t.Errorf("left of a after the move: %q", n)
	}
	if got := l.Swap("a", "c").Panes(); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("swap: %v", got)
	}
	if got := l.Panes(); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("swap changed the original: %v", got)
	}
	if l.Move("a", "a", "up") != l {
		t.Error("a pane moved onto itself")
	}
}
