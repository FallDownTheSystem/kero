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
	jira := proto.Session{ID: "a1", Name: "Jira", Assistant: "jira"}
	jkid := proto.Session{ID: "k2", Name: "fix", Parent: "a1"}
	ama := proto.Session{ID: "m1", Name: "Ama", Ama: true}
	for _, c := range []struct {
		from *proto.Session
		to   proto.Session
		want string
	}{
		{&boss, kid, `your orchestrator, session o1 "boss"`},
		{&kid, boss, `your child session k1 "login"`},
		{&other, kid, `session x1 "solo"`},
		{nil, kid, "a terminal outside Kero's sessions"},
		{&jira, jkid, `your orchestrator, the assistant "Jira" (session a1)`},
		{&ama, jira, "Ama, Kero's helper agent (session m1)"},
	} {
		if got := messageLabel(c.from, c.to); got != c.want {
			t.Errorf("%v to %s: %q, want %q", c.from, c.to.ID, got, c.want)
		}
	}
}
