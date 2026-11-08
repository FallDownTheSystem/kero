package cli

import (
	"errors"
	"slices"
	"testing"

	"kero/internal/proto"
)

// kero ls and kero close pick sessions by project, by its name or id,
// and by statuses, any of them.
func TestSessionFilter(t *testing.T) {
	st := proto.State{
		Projects: []proto.Project{{ID: "p1", Name: "api"}, {ID: "p2", Name: "web"}},
		Sessions: []proto.Session{
			{ID: "a", Project: "p1", Pane: proto.Pane{Status: proto.Idle}},
			{ID: "b", Project: "p1", Pane: proto.Pane{Status: proto.Waiting}},
			{ID: "c", Project: "p2", Pane: proto.Pane{Status: proto.Idle}},
			{ID: "d", Project: "p1", Pane: proto.Pane{Status: proto.Exited}},
		},
	}
	ids := func(ss []proto.Session) []string {
		var out []string
		for _, s := range ss {
			out = append(out, s.ID)
		}
		return out
	}
	for _, tc := range []struct {
		project, status string
		want            []string
	}{
		{"api", "", []string{"a", "b", "d"}},
		{"p2", "", []string{"c"}},
		{"", "idle", []string{"a", "c"}},
		{"API", "idle,exited", []string{"a", "d"}},
	} {
		sf, err := newFilter(st, &flags{vals: map[string]string{"project": tc.project, "status": tc.status}})
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(sf.pick(st)); !slices.Equal(got, tc.want) {
			t.Errorf("-p %q --status %q: %q, want %q", tc.project, tc.status, got, tc.want)
		}
	}
	if _, err := newFilter(st, &flags{vals: map[string]string{"status": "done"}}); !errors.Is(err, errUsage) {
		t.Errorf("status done: %v", err)
	}
	if _, err := newFilter(st, &flags{vals: map[string]string{"project": "nope"}}); !errors.Is(err, errNotFound) {
		t.Errorf("no project: %v", err)
	}
}
