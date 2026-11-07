package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"kero/internal/commitmsg"
	"kero/internal/proto"
)

// branchNameExpiry is how long naming a branch may take.
const branchNameExpiry = 2 * time.Minute

// gitBranchName has the branch of a new worktree of a repository named
// from the task its session starts with, by the command that writes
// commit messages, a new run of it: the name with the prefix the user
// set, and a number after it while a branch, a remote's too, has it.
func (d *Daemon) gitBranchName(p proto.GitBranchNameParams) (proto.GitBranchName, error) {
	task := strings.TrimSpace(p.Task)
	if task == "" {
		return proto.GitBranchName{}, errors.New("no task to name the branch from")
	}
	ctx, _, done := follow(p.ID, branchNameExpiry)
	defer done()
	r, err := d.paramsRepo(ctx, proto.GitParams{Dir: p.Dir})
	if err != nil {
		return proto.GitBranchName{}, err
	}
	list, _ := r.Branches(ctx)
	taken := map[string]bool{}
	var recent []string
	def := r.DefaultBranch()
	for _, b := range list {
		switch {
		case b.Tag:
		case b.Remote:
			if _, name, ok := strings.Cut(b.Name, "/"); ok {
				taken[name] = true
			}
		default:
			taken[b.Name] = true
			if b.Name != def && len(recent) < 12 {
				recent = append(recent, b.Name)
			}
		}
	}
	argv, _, err := commitmsg.Command(p.Command, nil, userShell())
	if err != nil {
		return proto.GitBranchName{}, err
	}
	// Kero's own run of Claude Code names it with its quickest model:
	// a few words want no more.
	if p.Command == "" && argv[0] == "claude" {
		argv = append(argv, "--model", "haiku")
	}
	name, err := commitmsg.WriteBranch(ctx, r.Root, argv, commitmsg.BranchPrompt(p.Prompt, p.Prefix, task, recent), p.Prefix)
	if errors.Is(err, context.Canceled) {
		return proto.GitBranchName{}, errors.New("canceled")
	}
	if err != nil {
		return proto.GitBranchName{}, err
	}
	return proto.GitBranchName{Name: commitmsg.UniqueBranch(name, func(s string) bool { return taken[s] })}, nil
}
