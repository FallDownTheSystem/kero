package daemon

import (
	"context"
	"os"
	"testing"
	"time"

	"kero/internal/git"
	"kero/internal/proto"
)

// Closing a session with its worktree waits for whatever the pane's
// program started to exit, as Windows deletes no folder that a process
// works in, killing what outlives treeWait: here a job that ignores the
// hangup, as an agent's MCP server may linger, in the pane's session but
// a process group of its own. Windows alone waits (awaitTrees); Linux
// lists a session's processes, as Windows does a job's.
func TestWorktreeCloseWaitsForPrograms(t *testing.T) {
	setupShell(t)
	defer func(w time.Duration, on bool) { treeWait, awaitTrees = w, on }(treeWait, awaitTrees)
	treeWait, awaitTrees = 500*time.Millisecond, true
	d := testDaemon(t)
	repo := gitRepo(t)
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	s, err := d.create(proto.CreateParams{Dir: repo, Worktree: "linger", Command: []string{"(trap '' HUP; sleep 30) & sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	p := d.findPane(s.Panes[0].ID)
	p.mu.Lock()
	pid := p.proc.PID()
	p.mu.Unlock()
	for start := time.Now(); len(sessionProcs(pid)) < 3; time.Sleep(20 * time.Millisecond) {
		if time.Since(start) > 5*time.Second {
			t.Fatalf("the pane's programs didn't start: %v", sessionProcs(pid))
		}
	}
	if err := d.closeSession(proto.CloseParams{ID: s.ID, RemoveWorktree: true}); err != nil {
		t.Fatal(err)
	}
	if left := sessionProcs(pid); len(left) > 0 {
		t.Errorf("processes of the pane outlive the removal: %v", left)
	}
	if _, err := os.Stat(s.Worktree.Path); !os.IsNotExist(err) {
		t.Errorf("the worktree stays: %v", err)
	}
	if r, err := git.Open(context.Background(), repo); err != nil || r.HasBranch("linger") {
		t.Errorf("kept the merged branch: %v", err)
	}
}
