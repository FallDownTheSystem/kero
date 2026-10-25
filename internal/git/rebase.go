package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Interactive rebases: the commits of HEAD's branch since a commit,
// replayed on it in the order the user lists them, each picked, reworded,
// stopped at to edit, squashed or fixed up into the one before it, or
// dropped. git's own editor of the list is a file the user's list is
// copied over, and a commit's new message is put by a command after it,
// so that git asks nothing.

// The actions of a rebase's steps, as git's list has them.
const (
	RebasePick   = "pick"
	RebaseReword = "reword"
	RebaseEdit   = "edit"
	RebaseSquash = "squash"
	RebaseFixup  = "fixup"
	RebaseDrop   = "drop"
)

// RebaseStep is a commit of an interactive rebase, and what is done with
// it. Message is a reworded commit's new message; empty, it keeps its own.
type RebaseStep struct {
	Action, Hash, Message string
}

// ErrRebaseMerges: the commits include merges, which a rebase of the list
// flattens.
var ErrRebaseMerges = errors.New("the commits include merges, which an interactive rebase would flatten")

// RebasePlan returns the commits a rebase of HEAD onto onto replays, the
// oldest first: those HEAD has that onto doesn't.
func (r Repo) RebasePlan(ctx context.Context, onto string) ([]Commit, error) {
	if err := checkRev(onto); err != nil || onto == "" {
		return nil, errors.New("not a revision: " + onto)
	}
	list, err := r.Log(ctx, LogOptions{Revs: []string{onto + "..HEAD"}})
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		if len(c.Parents) > 1 {
			return nil, ErrRebaseMerges
		}
	}
	slices.Reverse(list)
	return list, nil
}

var hashPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// RebaseInteractive rebases HEAD onto onto by steps, after a branch named
// backup, when set, keeps where HEAD was. The changes not committed are
// stashed for it (autostash). It stops as git does: at a commit to edit,
// or a conflict, which Continue goes on from.
func (r Repo) RebaseInteractive(ctx context.Context, onto string, steps []RebaseStep, backup string, autostash bool) (string, error) {
	if err := checkRev(onto); err != nil || onto == "" {
		return "", errors.New("not a revision: " + onto)
	}
	if len(steps) == 0 {
		return "", errors.New("no commits to rebase")
	}
	// The files the list and its messages are in stay while the rebase
	// stops, for the commands after its commits to read as it goes on.
	dir := filepath.Join(r.GitDir, "kero-rebase")
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var todo strings.Builder
	for i, s := range steps {
		if !hashPattern.MatchString(s.Hash) {
			return "", errors.New("not a commit: " + s.Hash)
		}
		switch s.Action {
		case RebaseSquash, RebaseFixup:
			if i == 0 {
				return "", errors.New("the first commit has none before it to " + s.Action + " into")
			}
		case RebaseReword:
			if strings.TrimSpace(s.Message) == "" {
				fmt.Fprintf(&todo, "pick %s\n", s.Hash)
				continue
			}
			msg := filepath.Join(dir, fmt.Sprintf("message-%d.txt", i))
			if err := os.WriteFile(msg, []byte(s.Message), 0o644); err != nil {
				return "", err
			}
			// Hooks are for what a commit changes, which this doesn't.
			fmt.Fprintf(&todo, "pick %s\nexec git commit --amend --only --allow-empty --no-verify -F %s\n", s.Hash, shQuote(msg))
			continue
		case RebasePick, RebaseEdit, RebaseDrop:
		default:
			return "", errors.New("not an action of a rebase: " + s.Action)
		}
		fmt.Fprintf(&todo, "%s %s\n", s.Action, s.Hash)
	}
	list := filepath.Join(dir, "todo")
	if err := os.WriteFile(list, []byte(todo.String()), 0o644); err != nil {
		return "", err
	}
	if backup != "" {
		if _, err := r.CreateBranch(ctx, backup, "HEAD", false); err != nil {
			return "", err
		}
	}
	args := []string{"rebase", "-i"}
	if autostash {
		args = append(args, "--autostash")
	}
	// git runs its editor of the list by the shell, its own on Windows,
	// with the list's path after it.
	out, err := r.writeWith(ctx, []string{"GIT_SEQUENCE_EDITOR=cp " + shQuote(list)}, "", nil, append(args, onto)...)
	if err == nil && r.operation() == "" {
		os.RemoveAll(dir)
	}
	return out, err
}

// shQuote quotes a path for the shell git runs its editors and commands
// by, with forward slashes, which Windows' takes too.
func shQuote(path string) string {
	return "'" + strings.ReplaceAll(filepath.ToSlash(path), "'", `'\''`) + "'"
}
