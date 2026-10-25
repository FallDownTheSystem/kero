package daemon

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"kero/internal/commitmsg"
	"kero/internal/git"
	"kero/internal/proto"
)

// The git of the Changes and History views beyond their diffs: the index,
// operations on the worktree at the user's word, branches, stashes, the
// log, and commit messages an agent writes.

// gitRuns are the operations under way that the client follows by an id
// of its own: what git printed last, and how to cancel them.
var gitRuns = struct {
	sync.Mutex
	m map[string]*gitRun
}{m: map[string]*gitRun{}}

type gitRun struct {
	cancel context.CancelFunc
	line   string
}

// follow keeps an operation's run by id while it goes on: its context,
// which git.cancel cancels, and a function for its progress. done ends it.
func follow(id string, timeout time.Duration) (ctx context.Context, progress func(string), done func()) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	if id == "" {
		return ctx, nil, cancel
	}
	run := &gitRun{cancel: cancel}
	gitRuns.Lock()
	gitRuns.m[id] = run
	gitRuns.Unlock()
	progress = func(line string) {
		gitRuns.Lock()
		run.line = line
		gitRuns.Unlock()
	}
	return ctx, progress, func() {
		cancel()
		gitRuns.Lock()
		if gitRuns.m[id] == run {
			delete(gitRuns.m, id)
		}
		gitRuns.Unlock()
	}
}

func gitProgress(id string) proto.GitProgress {
	gitRuns.Lock()
	defer gitRuns.Unlock()
	if r := gitRuns.m[id]; r != nil {
		return proto.GitProgress{Line: r.line}
	}
	return proto.GitProgress{}
}

func gitCancel(id string) bool {
	gitRuns.Lock()
	r := gitRuns.m[id]
	gitRuns.Unlock()
	if r != nil {
		r.cancel()
	}
	return r != nil
}

// repoLocks hold, by a repository's git directory, the operation that
// changes it.
var repoLocks = struct {
	sync.Mutex
	m map[string]chan struct{}
}{m: map[string]chan struct{}{}}

// lockRepo waits for the repository's other operations, as long as ctx
// lets it, and returns what lets the next go.
func lockRepo(ctx context.Context, common string) (func(), error) {
	repoLocks.Lock()
	ch := repoLocks.m[common]
	if ch == nil {
		ch = make(chan struct{}, 1)
		repoLocks.m[common] = ch
	}
	repoLocks.Unlock()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// paneRepo returns the worktree a pane works in, with its agent's
// conversation and its folder.
func (d *Daemon) paneRepo(ctx context.Context, ref string) (git.Repo, *proto.AgentSession, string, error) {
	pn := d.findPane(ref)
	if pn == nil {
		return git.Repo{}, nil, "", fmt.Errorf("no session or pane %q", ref)
	}
	pn.mu.Lock()
	dir, as := pn.info.WorkDir(), pn.info.AgentSession
	pn.mu.Unlock()
	r, err := git.Open(ctx, dir)
	return r, as, dir, err
}

// gitIndex returns a pane's worktree's changes, staged and not.
func (d *Daemon) gitIndex(p proto.GitParams) (proto.GitIndex, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitIndex{}, err
	}
	staged, unstaged, err := r.Index(ctx)
	if err != nil {
		return proto.GitIndex{}, err
	}
	res := proto.GitIndex{Root: r.Root, Staged: gitFiles(staged), Unstaged: gitFiles(unstaged)}
	if st, err := r.Status(ctx); err == nil && st.State != "" {
		res.Message = r.MergeMessage()
		res.Ours, res.Theirs = r.ConflictSides(ctx)
	}
	return res, nil
}

func gitFiles(files []git.File) []proto.GitFile {
	out := make([]proto.GitFile, 0, len(files))
	for _, f := range files {
		out = append(out, proto.GitFile{Path: f.Path, OldPath: f.OldPath, Status: f.Status, Added: f.Added, Removed: f.Removed, Binary: f.Binary})
	}
	return out
}

// Timeouts of the operations: a commit's hooks may run a project's tests,
// and a fetch a large repository's; the user cancels either sooner.
const (
	gitOpTimeout     = 2 * time.Minute
	gitLongTimeout   = 30 * time.Minute
	gitMessageExpiry = 5 * time.Minute
)

// gitOp does an operation in a pane's worktree. How it went is the result,
// git's words with it, not the call's error, which is for a call that
// went wrong.
func (d *Daemon) gitOp(p proto.GitOpParams) (proto.GitOpResult, error) {
	timeout := gitOpTimeout
	switch p.Op {
	case "commit", "fetch", "pull", "push", "continue", "cherryPick", "revert", "merge", "rebase", "rebaseInteractive":
		timeout = gitLongTimeout
	}
	ctx, progress, done := follow(p.ID, timeout)
	defer done()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitOpResult{}, err
	}
	if p.Root != "" && !(proto.Within(r.Root, p.Root) && proto.Within(p.Root, r.Root)) {
		return proto.GitOpResult{Error: "the pane works in another worktree now: " + r.Root}, nil
	}
	// One at a time in a repository, of those that check and then change:
	// two drops of the stash both found at stash@{0} would drop two. A
	// fetch or a push changes no more than the remotes' branches, and can
	// take long, which nothing else is to wait for.
	switch p.Op {
	case "fetch", "push", "lastMessage":
	default:
		unlock, err := lockRepo(ctx, r.Common)
		if err != nil {
			return proto.GitOpResult{Error: "canceled"}, nil
		}
		defer unlock()
	}
	cmd, err := opCommand(ctx, r, p)
	var out string
	if err == nil {
		out, err = d.runOp(ctx, r, p, cmd, progress)
	}
	d.git.poke(r.Root)
	res := proto.GitOpResult{Output: out}
	if cmd != nil {
		res.Command = append([]string{"git"}, cmd...)
	}
	if err != nil {
		res.Error = err.Error()
		if errors.Is(err, context.Canceled) {
			res.Error = "canceled"
		}
		var ge *git.Error
		if errors.As(err, &ge) && res.Output == "" {
			res.Output = ge.Output
		}
	}
	return res, nil
}

func (d *Daemon) runOp(ctx context.Context, r git.Repo, p proto.GitOpParams, cmd []string, progress func(string)) (string, error) {
	switch p.Op {
	case "stage":
		return "", r.Stage(ctx, p.Paths)
	case "unstage":
		return "", r.Unstage(ctx, p.Paths)
	case "discard":
		return "", r.Discard(ctx, p.Paths, p.Untracked)
	case "setIndex":
		return "", r.SetIndex(ctx, p.Path, p.Known, p.Text)
	case "commit":
		return r.Commit(ctx, git.CommitOptions{Message: p.Message, Amend: p.Amend, Signoff: p.Signoff, NoVerify: p.NoVerify})
	case "lastMessage":
		return r.LastMessage(ctx)
	case "undo":
		return "", r.UndoCommit(ctx)
	case "continue":
		return r.Continue(ctx)
	case "abort":
		return r.Abort(ctx)
	case "cherryPick":
		return r.CherryPick(ctx, p.Rev)
	case "revert":
		return r.RevertCommit(ctx, p.Rev)
	case "merge":
		if p.Mode != "" && p.Mode != "no-ff" {
			return "", fmt.Errorf("no merge %q", p.Mode)
		}
		return r.Merge(ctx, p.Rev, p.Mode == "no-ff")
	case "rebase":
		return r.Rebase(ctx, p.Rev, p.Autostash)
	case "rebaseInteractive":
		steps := make([]git.RebaseStep, len(p.Steps))
		for i, s := range p.Steps {
			steps[i] = git.RebaseStep{Action: s.Action, Hash: s.Hash, Message: s.Message}
		}
		return r.RebaseInteractive(ctx, p.Rev, steps, p.Backup, p.Autostash)
	case "resolve":
		return "", r.ResolveSide(ctx, p.Path, p.Mode)
	case "reset":
		return "", r.Reset(ctx, p.Rev, p.Mode)
	case "checkout":
		return "", r.Checkout(ctx, p.Rev)
	case "switch":
		return r.Switch(ctx, p.Name, p.Remote, p.Rev)
	case "createBranch":
		return r.CreateBranch(ctx, p.Name, p.Rev, p.Checkout)
	case "deleteBranch":
		return r.DeleteBranch(ctx, p.Name, p.Force)
	case "renameBranch":
		return r.RenameBranch(ctx, p.Name, p.To)
	case "stash":
		return r.StashPush(ctx, git.StashOptions{Message: p.Message, Untracked: p.IncludeUntracked, Staged: p.Staged})
	case "stashApply", "stashPop":
		return r.StashApply(ctx, p.Index, p.Hash, p.Op == "stashPop")
	case "stashDrop":
		return r.StashDrop(ctx, p.Index, p.Hash)
	case "fetch":
		return r.Fetch(ctx, git.FetchOptions{All: p.All, Prune: p.Prune}, progress)
	case "pull":
		return r.Pull(ctx, p.Mode, progress)
	case "push":
		return r.Push(ctx, cmd, progress)
	}
	return "", fmt.Errorf("no git operation %q", p.Op)
}

// opCommand is the git command of an operation that reaches a remote,
// which may need what the user alone can give, as a password: the one
// that runs, and that the client offers to run again in a terminal.
func opCommand(ctx context.Context, r git.Repo, p proto.GitOpParams) ([]string, error) {
	switch p.Op {
	case "fetch":
		return git.FetchArgs(git.FetchOptions{All: p.All, Prune: p.Prune}), nil
	case "pull":
		return git.PullArgs(p.Mode)
	case "push":
		st, err := r.Status(ctx)
		if err != nil {
			return nil, err
		}
		return r.PushArgs(ctx, st.Branch, git.PushOptions{Force: p.Force, Tags: p.Tags})
	}
	return nil, nil
}

// gitBranches returns a pane's repository's branches.
func (d *Daemon) gitBranches(p proto.GitParams) (proto.GitBranches, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitBranches{}, err
	}
	list, err := r.Branches(ctx)
	if err != nil {
		return proto.GitBranches{}, err
	}
	res := proto.GitBranches{Branches: make([]proto.GitBranch, 0, len(list))}
	for _, b := range list {
		res.Branches = append(res.Branches, proto.GitBranch{Name: b.Name, Remote: b.Remote, Current: b.Current, Head: b.Head,
			Upstream: b.Upstream, Ahead: b.Ahead, Behind: b.Behind, Gone: b.Gone, Time: b.Time, Subject: b.Subject, Worktree: b.Worktree})
	}
	res.Remotes, _ = r.Remotes(ctx)
	return res, nil
}

// gitStashes returns a pane's repository's stash.
func (d *Daemon) gitStashes(p proto.GitParams) (proto.GitStashes, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitStashes{}, err
	}
	list, err := r.Stashes(ctx)
	if err != nil {
		return proto.GitStashes{}, err
	}
	res := proto.GitStashes{Stashes: make([]proto.GitStash, 0, len(list))}
	for _, s := range list {
		res.Stashes = append(res.Stashes, proto.GitStash{Index: s.Index, Hash: s.Hash, Time: s.Time, Message: s.Message})
	}
	return res, nil
}

// maxFoundCommits is how many commits a search of the log finds.
const maxFoundCommits = 300

var hexRe = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// gitLog returns a page of a pane's worktree's history.
func (d *Daemon) gitLog(p proto.GitLogParams) (proto.GitLog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitLog{}, err
	}
	st, err := r.Status(ctx)
	if err != nil {
		return proto.GitLog{}, err
	}
	o := git.LogOptions{All: p.All, FirstParent: p.FirstParent}
	if !p.All {
		o.Revs = r.ShownRevs(ctx, st.Upstream != "")
	}
	var res proto.GitLog
	query := strings.TrimSpace(p.Query)
	switch {
	case query != "" || p.Path != "":
		// Found commits, in the order of their time: the message's, the
		// author's and the hash's together.
		o.Path, o.Max = p.Path, maxFoundCommits+1
		var found []git.Commit
		if query == "" {
			if found, err = r.Log(ctx, o); err != nil {
				return proto.GitLog{}, err
			}
		} else {
			seen := map[string]bool{}
			add := func(list []git.Commit) {
				for _, c := range list {
					if !seen[c.Hash] {
						seen[c.Hash] = true
						found = append(found, c)
					}
				}
			}
			if hexRe.MatchString(query) {
				if c, err := r.Show(ctx, query+"^{commit}"); err == nil {
					add([]git.Commit{c.Commit})
				}
			}
			for _, by := range []git.LogOptions{{Grep: query}, {Author: query}} {
				q := o
				q.Grep, q.Author = by.Grep, by.Author
				list, err := r.Log(ctx, q)
				if err != nil {
					return proto.GitLog{}, err
				}
				add(list)
			}
			slices.SortStableFunc(found, func(a, b git.Commit) int { return cmp.Compare(b.Time, a.Time) })
		}
		if len(found) > maxFoundCommits {
			found, res.More = found[:maxFoundCommits], true
		}
		res.Commits = logCommits(found)
	default:
		o.Skip, o.Max = p.Skip, max(p.Max, 1)+1
		list, err := r.Log(ctx, o)
		if err != nil {
			return proto.GitLog{}, err
		}
		if len(list) > o.Max-1 {
			list, res.More = list[:o.Max-1], true
		}
		res.Commits = logCommits(list)
	}
	if p.Skip == 0 {
		res.Unpushed, _ = r.Unpushed(ctx, st.Upstream != "", 1000)
		res.Remotes, _ = r.Remotes(ctx)
	}
	return res, nil
}

func logCommits(list []git.Commit) []proto.GitCommit {
	out := make([]proto.GitCommit, 0, len(list))
	for _, c := range list {
		out = append(out, proto.GitCommit{Hash: c.Hash, Parents: c.Parents, Author: c.Author, Email: c.Email, Time: c.Time, Refs: c.Refs, Subject: c.Subject})
	}
	return out
}

// gitShow returns a commit, with the sides each of its files shows
// between.
func (d *Daemon) gitShow(p proto.GitShowParams) (proto.GitCommitInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitCommitInfo{}, err
	}
	ci, err := r.Show(ctx, p.Rev)
	if err != nil {
		return proto.GitCommitInfo{}, err
	}
	from, err := r.Parent(ctx, ci.Commit)
	if err != nil {
		return proto.GitCommitInfo{}, err
	}
	c := ci.Commit
	res := proto.GitCommitInfo{GitCommit: proto.GitCommit{Hash: c.Hash, Parents: c.Parents, Author: c.Author, Email: c.Email, Time: c.Time, Refs: c.Refs, Subject: c.Subject},
		Committer: ci.Committer, Message: ci.Message}
	for _, f := range gitFiles(ci.Files) {
		f.From, f.To = from, c.Hash
		res.Files = append(res.Files, f)
	}
	for _, f := range gitFiles(ci.Untracked) {
		f.From, f.To = git.SideNone, c.Parents[2]
		res.Files = append(res.Files, f)
	}
	return res, nil
}

// gitRebasePlan returns what an interactive rebase of a pane's worktree's
// HEAD onto a commit replays.
func (d *Daemon) gitRebasePlan(p proto.GitShowParams) (proto.GitRebasePlan, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitRebasePlan{}, err
	}
	list, err := r.RebasePlan(ctx, p.Rev)
	if err != nil {
		return proto.GitRebasePlan{}, err
	}
	onto, err := r.Log(ctx, git.LogOptions{Revs: []string{p.Rev}, Max: 1})
	if err != nil || len(onto) == 0 {
		return proto.GitRebasePlan{}, fmt.Errorf("no commit %s", p.Rev)
	}
	st, err := r.Status(ctx)
	if err != nil {
		return proto.GitRebasePlan{}, err
	}
	commit := func(c git.Commit) proto.GitCommit {
		return proto.GitCommit{Hash: c.Hash, Parents: c.Parents, Author: c.Author, Email: c.Email, Time: c.Time, Refs: c.Refs, Subject: c.Subject}
	}
	res := proto.GitRebasePlan{Branch: st.Branch, Onto: commit(onto[0]), Commits: make([]proto.GitCommit, 0, len(list))}
	for _, c := range list {
		res.Commits = append(res.Commits, commit(c))
	}
	return res, nil
}

// gitMessage has a commit message written of what a pane's worktree has
// staged.
func (d *Daemon) gitMessage(p proto.GitMessageParams) (proto.GitMessage, error) {
	ctx, _, done := follow(p.ID, gitMessageExpiry)
	defer done()
	r, as, dir, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.GitMessage{}, err
	}
	stat, patch, err := r.Staged(ctx, 60_000)
	if err != nil {
		return proto.GitMessage{}, err
	}
	if stat == "" {
		return proto.GitMessage{}, errors.New("nothing is staged")
	}
	var agent *commitmsg.Agent
	if as != nil {
		agent = &commitmsg.Agent{Name: as.Agent, ID: as.ID}
	}
	argv, forked, err := commitmsg.Command(p.Command, agent, userShell())
	if err != nil {
		return proto.GitMessage{}, err
	}
	recent, _ := r.Log(ctx, git.LogOptions{Revs: []string{"HEAD"}, Max: 10, FirstParent: true})
	var subjects []string
	for _, c := range recent {
		subjects = append(subjects, c.Subject)
	}
	// A fork runs where its conversation did, which its agent finds it
	// by.
	at := r.Root
	if forked {
		at = dir
	}
	msg, err := commitmsg.Write(ctx, at, argv, commitmsg.Prompt(forked, strings.Join(subjects, "\n"), stat, patch))
	if errors.Is(err, context.Canceled) {
		return proto.GitMessage{}, errors.New("canceled")
	}
	return proto.GitMessage{Message: msg}, err
}
