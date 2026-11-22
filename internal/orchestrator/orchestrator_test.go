package orchestrator

import (
	"strings"
	"testing"
)

// The role prompt has the goal where {goal} is, else after it, and says
// when there is none; {agent} is the agent children start.
func TestPrompt(t *testing.T) {
	p := Prompt("", "Ship it", "cc --yolo")
	if !strings.Contains(p, "Goal: Ship it") || !strings.Contains(p, "-- cc --yolo '<brief>'") || strings.Contains(p, "{") {
		t.Errorf("default: %q", p)
	}
	if p := Prompt("", " ", ""); !strings.Contains(p, "Goal: "+NoGoal) || !strings.Contains(p, "-- claude '<brief>'") {
		t.Errorf("no goal: %q", p)
	}
	if p := Prompt("Plan, don't code.", "Ship it", "claude"); p != "Plan, don't code.\n\nGoal: Ship it" {
		t.Errorf("a template without {goal}: %q", p)
	}
	if p := Prompt("Plan.", "", "claude"); p != "Plan." {
		t.Errorf("a template without {goal}, no goal: %q", p)
	}
	// It goes to the agent as one argument, which a double quote could
	// end early on Windows.
	if strings.Contains(DefaultPrompt, `"`) {
		t.Error("the default prompt has a double quote")
	}
	// It points at the skill's reference for the details, and keeps the
	// rules it can't do without: messages, not typing, to children; OK.
	for _, want := range []string{"kero skill show orchestrating", "Result, Not done, Where, Next", "kero message <id>", "'OK'"} {
		if !strings.Contains(DefaultPrompt, want) {
			t.Errorf("the default prompt lacks %q", want)
		}
	}
}
