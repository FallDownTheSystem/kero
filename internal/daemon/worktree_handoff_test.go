package daemon

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

// A handoff makes the session's own worktree at once, and once the
// agent's pane is quiet starts it again in the same folder of the
// worktree, with Claude Code's transcript moved to that folder's; a pane
// with no agent's conversation, or a session with a worktree already,
// can't hand off.
func TestWorktreeHandoff(t *testing.T) {
	d := testDaemon(t)
	repo := gitRepo(t)
	os.MkdirAll(filepath.Join(repo, "web"), 0o755)
	os.WriteFile(filepath.Join(repo, "web", "x.txt"), []byte("x\n"), 0o644)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "web")
	d.projects = []proto.Project{{ID: "pr", Name: "repo", Dir: repo}}
	from := filepath.Join(repo, "web")

	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	project := func(dir string) string {
		return filepath.Join(claude, "projects", regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(dir, "-"))
	}
	os.MkdirAll(filepath.Join(project(from), "conv1", "subagents"), 0o755)
	os.WriteFile(filepath.Join(project(from), "conv1.jsonl"), []byte("{}\n"), 0o644)

	shell, err := d.create(proto.CreateParams{Dir: from})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.handoff(proto.HandoffParams{Pane: shell.Panes[0].ID, Branch: "nope"}); err == nil || !strings.Contains(err.Error(), "no agent") {
		t.Errorf("a shell's pane handed off: %v", err)
	}

	s, err := d.create(proto.CreateParams{Dir: from})
	if err != nil {
		t.Fatal(err)
	}
	pn := d.findPane(s.Panes[0].ID)
	pn.mu.Lock()
	pn.info.AgentSession = &proto.AgentSession{Agent: "claude", ID: "conv1"}
	pn.mu.Unlock()
	res, err := d.handoff(proto.HandoffParams{Pane: pn.info.ID, Branch: "feat/move"})
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(repo, ".kero", "worktrees", "feat-move")
	to := filepath.Join(wt, "web")
	if res.Path != wt || res.Branch != "feat/move" || res.Base != "main" || res.Dir != to || res.Session != s.ID {
		t.Fatalf("result %+v", res)
	}
	d.mu.Lock()
	got := d.findSessionLocked(s.ID).worktree
	d.mu.Unlock()
	if got == nil || got.Path != wt {
		t.Fatalf("the session's worktree is %+v", got)
	}
	if _, err := d.handoff(proto.HandoffParams{Pane: pn.info.ID, Branch: "again"}); err == nil {
		t.Error("handed off a session with a worktree already")
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		pn.mu.Lock()
		dir, cmd, as := pn.info.Dir, pn.info.Command, pn.info.AgentSession
		pn.mu.Unlock()
		if dir == to {
			if len(cmd) != 3 || cmd[0] != "claude" || cmd[2] != "conv1" || as == nil || as.ID != "conv1" {
				t.Errorf("restarted with %q, agent %+v", cmd, as)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the pane is still in %s", dir)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(project(to), "conv1.jsonl")); err != nil {
		t.Errorf("the transcript didn't move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project(to), "conv1", "subagents")); err != nil {
		t.Errorf("its subagents didn't move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project(from), "conv1.jsonl")); !os.IsNotExist(err) {
		t.Errorf("the transcript stays where it was: %v", err)
	}
	d.removeSession(s.ID)
	d.removeSession(shell.ID)
}
