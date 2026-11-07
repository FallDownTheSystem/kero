package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// setupDone waits for the setup of the worktree at path to end, and
// returns it.
func setupDone(t *testing.T, d *Daemon, path string) proto.WorktreeSetup {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range d.state().Setups {
			if s.Path == path && !s.Running() {
				return s
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the setup of %s didn't end: %+v", path, d.state().Setups)
	return proto.WorktreeSetup{}
}

func stageOf(s proto.WorktreeSetup, id string) proto.SetupStage {
	for _, st := range s.Stages {
		if st.ID == id {
			return st
		}
	}
	return proto.SetupStage{}
}

// sessionNow is a session's info as it stands, ok false once it closed.
func sessionNow(d *Daemon, id string) (proto.Session, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.findSessionLocked(id)
	if s == nil {
		return proto.Session{}, false
	}
	return d.sessionInfo(s), true
}

func setupShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a POSIX shell runs the scripts here")
	}
	t.Setenv("SHELL", "/bin/sh")
}

// A worktree's setup script, of its kero.json, runs in a terminal below
// the session's own, told where the worktree is; it closes as it
// succeeds. A repository without one sets nothing up.
func TestWorktreeSetupScript(t *testing.T) {
	setupShell(t)
	d := testDaemon(t)
	repo := gitRepo(t)
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	plain, err := d.create(proto.CreateParams{Dir: repo, Worktree: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if st := d.state().Setups; len(st) != 0 {
		t.Errorf("a worktree with nothing to set up has setups %+v", st)
	}
	d.removeSession(plain.ID)

	os.WriteFile(filepath.Join(repo, "kero.json"), []byte(`{"scripts": [
		{"name": "Dev", "command": "echo dev > dev.txt"},
		{"name": "Install", "command": "echo \"$KERO_WORKTREE_PATH\" > setup.txt", "runOnWorktreeCreate": true}
	]}`), 0o644)
	// kero.json comes first.
	os.WriteFile(filepath.Join(repo, "t3.json"), []byte(`{"scripts": [{"name": "T3", "command": "echo t3 > t3.txt", "runOnWorktreeCreate": true}]}`), 0o644)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "setup")
	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "feat"})
	if err != nil {
		t.Fatal(err)
	}
	wt := s.Worktree.Path
	got := setupDone(t, d, wt)
	if got.Phase != proto.SetupDone || got.Script != "Install" || got.Session != s.ID || got.Pane == "" {
		t.Fatalf("setup %+v", got)
	}
	if st := stageOf(got, proto.SetupScript); st.Status != proto.SetupDone {
		t.Errorf("script stage %+v", st)
	}
	if st := stageOf(got, proto.SetupSubmodules); st.Status != proto.SetupSkipped {
		t.Errorf("submodules stage %+v, of a repository with none", st)
	}
	if b, err := os.ReadFile(filepath.Join(wt, "setup.txt")); strings.TrimSpace(string(b)) != wt {
		t.Errorf("setup.txt: %q, %v", b, err)
	}
	for _, f := range []string{"dev.txt", "t3.txt"} {
		if _, err := os.Stat(filepath.Join(wt, f)); err == nil {
			t.Errorf("ran the script that makes %s", f)
		}
	}
	if d.findPane(got.Pane) != nil {
		t.Error("the script's terminal stays after it succeeded")
	}
	if info, ok := sessionNow(d, s.ID); !ok || len(info.Panes) != 1 || info.Panes[0].ID != s.Panes[0].ID {
		t.Fatalf("the session lost its own pane: %+v", info.Panes)
	}
	if err := d.cancelSetup(got.ID); err != nil || len(d.state().Setups) != 0 {
		t.Errorf("a setup that ended stays after its card closed: %v", err)
	}
	d.removeSession(s.ID)
}

// A setup script that holds the agent ("async": false), here the main
// checkout's own t3.json, starts in the session's terminal; the agent
// starts above it once it ends, failed too, which leaves its terminal.
func TestWorktreeSetupHolds(t *testing.T) {
	setupShell(t)
	d := testDaemon(t)
	repo := gitRepo(t)
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	os.WriteFile(filepath.Join(repo, "t3.json"), []byte(`{"scripts": [{"name": "Setup", "command": "sleep 1; false", "runOnWorktreeCreate": true, "async": false}]}`), 0o644)
	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "held", Command: []string{"echo agent; sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Panes) != 1 || strings.Join(s.Panes[0].Command, " ") != "sleep 1; false" {
		t.Fatalf("the session starts with %+v, not the script", s.Panes)
	}
	got := setupDone(t, d, s.Worktree.Path)
	if got.Phase != proto.SetupFailed || stageOf(got, proto.SetupScript).Detail != "exit 1" || stageOf(got, proto.SetupAgent).Status != proto.SetupDone {
		t.Fatalf("setup %+v", got)
	}
	info, _ := sessionNow(d, s.ID)
	if len(info.Panes) != 2 || strings.Join(info.Panes[0].Command, " ") != "echo agent; sleep 30" || info.Panes[1].ID != got.Pane || info.Focus != info.Panes[0].ID {
		t.Fatalf("panes %+v, focus %s", info.Panes, info.Focus)
	}
	d.removeSession(s.ID)
}

// Cancelling a setup stops its script, closing its terminal; the session
// goes on.
func TestWorktreeSetupCancel(t *testing.T) {
	setupShell(t)
	d := testDaemon(t)
	repo := gitRepo(t)
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	os.WriteFile(filepath.Join(repo, "kero.json"), []byte(`{"scripts": [{"command": "sleep 30", "runOnWorktreeCreate": true}]}`), 0o644)
	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "slow"})
	if err != nil {
		t.Fatal(err)
	}
	var id, pane string
	for deadline := time.Now().Add(10 * time.Second); pane == "" && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		for _, st := range d.state().Setups {
			if stageOf(st, proto.SetupScript).Status == proto.SetupRunning {
				id, pane = st.ID, st.Pane
			}
		}
	}
	if pane == "" {
		t.Fatalf("the script didn't start: %+v", d.state().Setups)
	}
	if err := d.cancelSetup(id); err != nil {
		t.Fatal(err)
	}
	got := setupDone(t, d, s.Worktree.Path)
	if got.Phase != proto.SetupCancelled || got.Script != "kero.json" {
		t.Fatalf("setup %+v", got)
	}
	if d.findPane(pane) != nil {
		t.Error("the script's terminal stays after cancelling")
	}
	if info, ok := sessionNow(d, s.ID); !ok || len(info.Panes) != 1 {
		t.Fatal("the session went with the setup")
	}
	d.removeSession(s.ID)
}

// A setup's fetch stage tells what the worktree started from: the remote's
// branch, fetched, or the fetch that failed, which leaves the local one.
func TestWorktreeSetupFetch(t *testing.T) {
	setupShell(t)
	d := testDaemon(t)
	up := gitRepo(t)
	repo := filepath.Join(t.TempDir(), "clone")
	runGit(t, up, "clone", "-q", up, repo)
	os.WriteFile(filepath.Join(repo, "kero.json"), []byte(`{"scripts": [{"command": "true", "runOnWorktreeCreate": true}]}`), 0o644)
	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "fetched"})
	if err != nil {
		t.Fatal(err)
	}
	if st := stageOf(setupDone(t, d, s.Worktree.Path), proto.SetupFetch); st.Status != proto.SetupDone || st.Detail != "origin/main" {
		t.Errorf("fetch stage %+v", st)
	}
	runGit(t, repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
	s, err = d.create(proto.CreateParams{Dir: repo, Worktree: "offline"})
	if err != nil {
		t.Fatal(err)
	}
	if st := stageOf(setupDone(t, d, s.Worktree.Path), proto.SetupFetch); st.Status != proto.SetupWarning || st.Detail == "" {
		t.Errorf("fetch stage offline %+v", st)
	}
}
