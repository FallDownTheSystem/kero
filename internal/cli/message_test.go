package cli

import (
	"testing"

	"kero/internal/proto"
)

// A message names whom it is from as its reader stands to it: its
// orchestrator, its child, another session, or no session.
func TestMessageLabel(t *testing.T) {
	boss := proto.Session{ID: "o1", Name: "boss", Orchestrator: true}
	kid := proto.Session{ID: "k1", Name: "login", Parent: "o1"}
	other := proto.Session{ID: "x1", Name: "solo"}
	for _, c := range []struct {
		from *proto.Session
		to   proto.Session
		want string
	}{
		{&boss, kid, `your orchestrator, session o1 "boss"`},
		{&kid, boss, `your child session k1 "login"`},
		{&other, kid, `session x1 "solo"`},
		{nil, kid, "a terminal outside Kero's sessions"},
	} {
		if got := messageLabel(c.from, c.to); got != c.want {
			t.Errorf("%v to %s: %q, want %q", c.from, c.to.ID, got, c.want)
		}
	}
}
