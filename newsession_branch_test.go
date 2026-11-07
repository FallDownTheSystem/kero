package main

import (
	"encoding/json"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// An agent's session on a new branch with a task names the branch first,
// unless the user named it, and gives the agent the task as its prompt.
func TestNewSessionTask(t *testing.T) {
	w := &newSessionAsk{kind: kindAgent, agent: "claude", worktree: true, fresh: true, dir: "/src/app", task: " fix the login loop "}
	w.branches.list = &proto.GitBranches{Branches: []proto.GitBranch{{Name: "main", Current: true}}}
	if _, ok := w.params(""); ok || !w.namesBranch() {
		t.Errorf("an unnamed branch with a task: starts %v, names it %v", ok, w.namesBranch())
	}
	w.newBranch = "fix/login-loop"
	p, ok := w.params("")
	if !ok || w.namesBranch() || p.Worktree != "fix/login-loop" || !slices.Equal(p.Command, []string{"claude", "fix the login loop"}) {
		t.Errorf("a named branch: %+v %v", p, ok)
	}
	w.agent = `"C:\Tools\opencode.exe"`
	if p, _ := w.params(""); !slices.Equal(p.Command, []string{`C:\Tools\opencode.exe`, "--prompt", "fix the login loop"}) {
		t.Errorf("OpenCode's prompt: %q", p.Command)
	}
	w.agent = "pi {prompt} --yes"
	if p, _ := w.params(""); !slices.Equal(p.Command, []string{"pi", "fix the login loop", "--yes"}) {
		t.Errorf("the prompt in place of {prompt}: %q", p.Command)
	}
	// The task is the new branch's alone.
	w.agent, w.fresh = "claude", false
	if p, _ := w.params(""); !slices.Equal(p.Command, []string{"claude"}) {
		t.Errorf("an existing branch: %q", p.Command)
	}
	w.task, w.fresh, w.newBranch = "", true, ""
	if w.namesBranch() {
		t.Error("names a branch from no task")
	}
}

// Start with a task and no branch name has kerod name the branch, then
// starts the agent on it with the task; a naming that fails leaves a name
// of the task's words to look at first.
func TestNewSessionNamesBranch(t *testing.T) {
	a := testApp(t)
	a.set.Agents, a.set.DefaultAgent = []string{"claude"}, "claude"
	a.set.WorktreeBranchPrefix = "static"
	a.set.WorktreeBranchStatic = "kero/"
	created := make(chan proto.CreateParams, 1)
	named := make(chan proto.GitBranchNameParams, 2)
	var fail atomic.Bool
	cl, _ := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "git.branches":
			return proto.GitBranches{Branches: []proto.GitBranch{{Name: "main", Current: true, Worktree: "/tmp"}}}, nil
		case "git.worktrees":
			return proto.GitWorktrees{}, nil
		case "git.branchName":
			var p proto.GitBranchNameParams
			json.Unmarshal(raw, &p)
			named <- p
			if fail.Load() {
				return nil, errors.New("claude: not logged in")
			}
			return proto.GitBranchName{Name: "kero/add-dark-mode"}, nil
		case "create":
			var p proto.CreateParams
			json.Unmarshal(raw, &p)
			created <- p
			return proto.Session{ID: "new", Project: p.Project, Panes: []proto.Pane{{ID: "c", Dir: "/tmp"}}, Focus: "c"}, nil
		}
		return nil, errors.New("no " + method + " in a test")
	})
	a.local().conn, a.local().cl = connReady, cl
	tt := ui.NewTester(a.view, 1100, 760)
	tt.Frame()
	a.askNewSession(a.local(), "", "/tmp", true)
	w := a.newAsk
	frameUntil(t, tt, "the branches", func() bool { return w.branches.list != nil })
	w.task = "Add a dark mode to the settings window"
	tt.Frame()
	if !tt.HasText("Kero names the branch from the task, then creates it from main, in a new worktree.") {
		t.Fatalf("texts %q", tt.Texts())
	}
	shot(t, tt, "new-session-task")
	if err := tt.Click(locales.NewSessionStart()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if p := <-named; p.Task != w.task || p.Prefix != "kero/" || p.Dir != "/tmp" {
		t.Errorf("asked to name %+v", p)
	}
	var p proto.CreateParams
	select {
	case p = <-created:
	case <-time.After(2 * time.Second):
		t.Fatal("no session started")
	}
	if p.Worktree != "kero/add-dark-mode" || !slices.Equal(p.Command, []string{"claude", "Add a dark mode to the settings window"}) {
		t.Errorf("started %+v", p)
	}

	fail.Store(true)
	a.askNewSession(a.local(), "", "/tmp", true)
	w = a.newAsk
	frameUntil(t, tt, "the branches", func() bool { return w.branches.list != nil })
	w.task = "Fix the crash on save"
	tt.Frame()
	tt.Click(locales.NewSessionStart())
	<-named
	frameUntil(t, tt, "the fallback", func() bool { return w.nameErr != "" })
	tt.Frame()
	if w.newBranch != "kero/fix-the-crash-on-save" || a.newAsk != w {
		t.Errorf("after a failed naming: %q, the dialog %v", w.newBranch, a.newAsk != nil)
	}
	shot(t, tt, "new-session-task-failed")
}
