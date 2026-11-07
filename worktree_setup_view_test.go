package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
)

// A worktree's setup, of its host's state, shows as a card of its stages
// while it runs, whose Cancel cancels it on the host, and Show Terminal
// goes to the script's terminal.
func TestWorktreeSetupCard(t *testing.T) {
	a := testApp(t)
	cancels := make(chan string, 1)
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method == "worktree.setup.cancel" {
			var p proto.IDParams
			json.Unmarshal(raw, &p)
			cancels <- p.ID
			return nil, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().conn, a.local().cl = connReady, cl
	a.selected = ""
	a.local().setups = []proto.WorktreeSetup{{
		ID: "su1", Path: "/tmp/.kero/worktrees/feat-login", Branch: "feat/login", Session: "s1", Pane: "b",
		Script: "Install", Command: "bun install", Phase: proto.SetupRunning,
		Stages: []proto.SetupStage{
			{ID: proto.SetupFetch, Status: proto.SetupSkipped},
			{ID: proto.SetupCheckout, Status: proto.SetupDone},
			{ID: proto.SetupSubmodules, Status: proto.SetupDone},
			{ID: proto.SetupScript, Status: proto.SetupRunning, Detail: "bun install"},
			{ID: proto.SetupAgent, Status: proto.SetupDone},
		},
	}}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	for _, s := range []string{"Setting up feat/login", "Check out the files", "Run Install", "bun install", "Start the session"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("Fetch the base branch") {
		t.Error("shows the fetch kero didn't make")
	}
	shot(t, tt, "worktree-setup-running")
	if err := tt.Click("Show Terminal"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != "s1" || a.focusOf(a.session("s1")) != "b" {
		t.Errorf("showed session %q, pane %q", a.selected, a.focusOf(a.session("s1")))
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-cancels:
		if id != "su1" {
			t.Errorf("cancelled %q", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel cancelled nothing")
	}

	s := &a.local().setups[0]
	s.Phase, s.Pane = proto.SetupFailed, "b"
	s.Stages[3] = proto.SetupStage{ID: proto.SetupScript, Status: proto.SetupFailed, Detail: "exit 1"}
	tt.Frame()
	if !tt.HasText("Setting up feat/login failed") || !tt.HasText("exit 1") || tt.HasText("Cancel") {
		t.Errorf("a failed setup's card: %q", tt.Texts())
	}
	shot(t, tt, "worktree-setup-failed")
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "worktree-setup-failed-dark")
}
