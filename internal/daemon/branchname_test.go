package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kero/internal/proto"
)

// A new worktree's branch is named by the user's command, given the task,
// with the kind of work it picks as the prefix, and a number after a name
// a branch has already.
func TestGitBranchName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for the model")
	}
	d := testDaemon(t)
	repo := gitRepo(t)
	runGit(t, repo, "branch", "fix/login-redirect")
	bin := t.TempDir()
	got := filepath.Join(bin, "prompt.txt")
	model := filepath.Join(bin, "model.sh")
	os.WriteFile(model, []byte("#!/bin/sh\ncat > "+got+"\nprintf 'Sure.\\n<branch-name>\\nfix/login-redirect\\n</branch-name>\\n'\n"), 0o755)
	res, err := d.gitBranchName(proto.GitBranchNameParams{Dir: repo, Task: "The login page redirects forever", Command: model, Prefix: "{type}/"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "fix/login-redirect-2" {
		t.Errorf("named %q", res.Name)
	}
	prompt, _ := os.ReadFile(got)
	if !strings.Contains(string(prompt), "The login page redirects forever") || !strings.Contains(string(prompt), "fix/login-redirect") {
		t.Errorf("the prompt lacks the task or the recent branches:\n%s", prompt)
	}
	if _, err := d.gitBranchName(proto.GitBranchNameParams{Dir: repo, Task: " ", Command: model}); err == nil {
		t.Error("named a branch with no task")
	}
}
