package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"kero/internal/commitmsg"
	"kero/internal/git"
	"kero/internal/github"
	"kero/internal/proto"
)

// The Review view's pull requests: those of the repository's remotes on
// GitHub, as the GitHub CLI lists them, and one of the pane's branch to
// make, with what it would bring.

// gitHubRepos returns the repository's remotes on GitHub, in git's order,
// and each one's repository by its name.
func gitHubRepos(ctx context.Context, r git.Repo) ([]proto.GitHubRepo, map[string]github.Repo, error) {
	remotes, err := r.Remotes(ctx)
	if err != nil {
		return nil, nil, err
	}
	var list []proto.GitHubRepo
	byName := map[string]github.Repo{}
	for _, name := range remotes {
		u, err := r.RemoteURL(ctx, name)
		if err != nil {
			continue
		}
		gr, ok := github.ParseRemote(u)
		if !ok {
			continue
		}
		byName[name] = gr
		list = append(list, proto.GitHubRepo{Remote: name, Slug: gr.Slug(), Owner: gr.Owner, URL: gr.URL(), Default: r.RemoteDefault(name)})
	}
	if len(list) == 0 {
		return nil, nil, errors.New("the repository has no remote on GitHub")
	}
	return list, byName, nil
}

// pickRemote returns the remote asked for, else the one HEAD's branch
// pushes to, else the first, of those on GitHub.
func pickRemote(ctx context.Context, r git.Repo, byName map[string]github.Repo, repos []proto.GitHubRepo, asked string) (string, error) {
	if asked != "" {
		if _, ok := byName[asked]; !ok {
			return "", fmt.Errorf("the remote %q isn't on GitHub", asked)
		}
		return asked, nil
	}
	if st, err := r.Status(ctx); err == nil && st.Branch != "" {
		if push, _, err := r.PushRemote(ctx, st.Branch); err == nil {
			if _, ok := byName[push]; ok {
				return push, nil
			}
		}
	}
	return repos[0].Remote, nil
}

func prOf(p github.PR) proto.PullRequest {
	return proto.PullRequest{Number: p.Number, Title: p.Title, Body: p.Body, URL: p.URL, Author: p.Author, Head: p.Head, HeadOwner: p.HeadOwner,
		CrossRepo: p.CrossRepo, Base: p.Base, Draft: p.Draft, Checks: p.Checks, Review: p.Review, Updated: p.Updated.Unix()}
}

// prList returns the open pull requests of a remote of a pane's
// repository. What goes wrong with gh is the list's Error, which leaves
// the remotes to choose from.
func (d *Daemon) prList(p proto.PRListParams) (proto.PRList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var r git.Repo
	var err error
	if p.Pane == "" && p.Dir != "" {
		r, err = git.Open(ctx, p.Dir)
	} else {
		r, _, _, err = d.paneRepo(ctx, p.Pane)
	}
	if err != nil {
		return proto.PRList{}, err
	}
	repos, byName, err := gitHubRepos(ctx, r)
	if err != nil {
		return proto.PRList{}, err
	}
	remote, err := pickRemote(ctx, r, byName, repos, p.Remote)
	if err != nil {
		return proto.PRList{}, err
	}
	res := proto.PRList{Repos: repos, Remote: remote, PRs: []proto.PullRequest{}}
	prs, err := github.List(ctx, r.Root, byName[remote], p.Filter, 100)
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	if p.Viewer {
		res.Viewer, _ = github.Viewer(ctx, r.Root, byName[remote])
	}
	for _, pr := range prs {
		res.PRs = append(res.PRs, prOf(pr))
	}
	return res, nil
}

// prTarget is a pull request of a pane's branch, as a draft or a creation
// names it: its repository and branches.
type prTarget struct {
	r  git.Repo
	st git.Status
	// agent is the conversation of the pane's agent, and dir where it
	// works, which a fork of it runs in.
	agent                *proto.AgentSession
	dir                  string
	repos                []proto.GitHubRepo
	remote, base         string
	repo                 github.Repo
	pushRemote, pushName string // where the branch goes, and its name there
	headOwner            string
}

func (d *Daemon) prTarget(ctx context.Context, pane, remote, base string) (prTarget, error) {
	var t prTarget
	r, agent, dir, err := d.paneRepo(ctx, pane)
	if err != nil {
		return t, err
	}
	t.r, t.agent, t.dir = r, agent, dir
	if t.st, err = r.Status(ctx); err != nil {
		return t, err
	}
	if t.st.Branch == "" {
		return t, errors.New("HEAD is on no branch: check one out to open a pull request of it")
	}
	repos, byName, err := gitHubRepos(ctx, r)
	if err != nil {
		return t, err
	}
	t.repos = repos
	if t.remote, err = pickRemote(ctx, r, byName, repos, remote); err != nil {
		return t, err
	}
	t.repo = byName[t.remote]
	if t.pushRemote, t.pushName, err = r.PushRemote(ctx, t.st.Branch); err != nil {
		return t, err
	}
	push, ok := byName[t.pushRemote]
	if !ok {
		return t, fmt.Errorf("the branch pushes to %s, which isn't on GitHub", t.pushRemote)
	}
	t.headOwner = push.Owner
	t.base = base
	if t.base == "" {
		t.base = r.RemoteDefault(t.remote)
	}
	if t.base == "" {
		return t, fmt.Errorf("%s has no default branch known: fetch it, or choose a base", t.remote)
	}
	return t, nil
}

// mergeBase returns where the branch left the base, as last fetched.
func (t prTarget) mergeBase(ctx context.Context) (string, error) {
	ref := "refs/remotes/" + t.remote + "/" + t.base
	mb, err := t.r.MergeBase(ctx, "HEAD", ref)
	if err != nil {
		return "", fmt.Errorf("no %s/%s to compare with: fetch %s first", t.remote, t.base, t.remote)
	}
	return mb, nil
}

// prDraft returns what a pull request of a pane's branch would bring: the
// commits and the files since it left the base, those not pushed yet, and
// the one open already.
func (d *Daemon) prDraft(p proto.PRDraftParams) (proto.PRDraft, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	t, err := d.prTarget(ctx, p.Pane, p.Remote, p.Base)
	if err != nil {
		return proto.PRDraft{}, err
	}
	res := proto.PRDraft{Repos: t.repos, Remote: t.remote, Base: t.base, Head: t.pushName, HeadOwner: t.headOwner, PushRemote: t.pushRemote,
		Uncommitted: t.st.Files, Commits: []proto.GitCommit{}, Files: []proto.GitFile{}}
	mb, err := t.mergeBase(ctx)
	if err != nil {
		return proto.PRDraft{}, err
	}
	res.MergeBase = mb
	commits, err := t.r.Log(ctx, git.LogOptions{Revs: []string{mb + "..HEAD"}, Max: 250})
	if err != nil {
		return proto.PRDraft{}, err
	}
	for _, c := range commits {
		res.Commits = append(res.Commits, proto.GitCommit{Hash: c.Hash, Parents: c.Parents, Author: c.Author, Email: c.Email, Time: c.Time, Refs: c.Refs, Subject: c.Subject})
	}
	files, err := t.r.Changes(ctx, mb, t.st.Head)
	if err != nil {
		return proto.PRDraft{}, err
	}
	for _, f := range gitFiles(files) {
		f.From, f.To = mb, t.st.Head
		res.Files = append(res.Files, f)
	}
	// What the push remote hasn't of the branch: all of it, before its
	// first push.
	pushed := "refs/remotes/" + t.pushRemote + "/" + t.pushName
	if un, err := t.r.Log(ctx, git.LogOptions{Revs: []string{pushed + "..HEAD"}, Max: 1000}); err == nil {
		res.Unpushed = len(un)
	} else {
		res.Unpushed = max(len(commits), 1)
	}
	existing, err := github.ForBranch(ctx, t.r.Root, t.repo, t.headOwner, t.pushName)
	switch {
	case err != nil:
		res.Error = err.Error()
	case existing != nil:
		pr := prOf(*existing)
		res.Existing = &pr
	}
	return res, nil
}

// prCreate makes a pull request of a pane's branch, pushed already, or
// returns the page of GitHub's that makes it, its fields filled in.
func (d *Daemon) prCreate(p proto.PRCreateParams) (proto.PRCreated, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t, err := d.prTarget(ctx, p.Pane, p.Remote, p.Base)
	if err != nil {
		return proto.PRCreated{}, err
	}
	n := github.New{Base: t.base, Head: t.pushName, HeadOwner: t.headOwner, Title: strings.TrimSpace(p.Title), Body: strings.TrimSpace(p.Body), Draft: p.Draft}
	if p.Show {
		return proto.PRCreated{URL: github.CompareURL(t.repo, n)}, nil
	}
	url, err := github.Create(ctx, t.r.Root, t.repo, n)
	if err != nil {
		return proto.PRCreated{}, err
	}
	return proto.PRCreated{URL: url}, nil
}

// prFetch fetches a pull request's branch: of the repository's own, by
// its name, which a review's fixes push back to; of a fork's, as GitHub
// keeps every pull request's (refs/pull/N/head), as pr-N. The review
// checks it out on a branch of its own, as git checks a branch out in one
// worktree alone: the main checkout or another review may have it.
func (d *Daemon) prFetch(p proto.PRFetchParams) (proto.PRFetched, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.PRFetched{}, err
	}
	branch, err := fetchPR(ctx, r, p.Remote, p.Number, p.Head, p.CrossRepo)
	if err != nil {
		return proto.PRFetched{}, err
	}
	got := proto.PRFetched{Branch: reviewBranch(r, p.Number), Base: p.Remote + "/" + branch}
	// A fork's branch isn't the remote's to push to.
	if branch == p.Head && !p.CrossRepo {
		got.PushRemote, got.PushBranch = p.Remote, branch
	}
	return got, nil
}

// fetchPR fetches pull request number's branch, head, from remote, to
// refs/remotes/<remote>/<branch>, and returns the branch: head itself, of
// the repository's own, else pr-<number>.
func fetchPR(ctx context.Context, r git.Repo, remote string, number int, head string, cross bool) (string, error) {
	if number <= 0 {
		return "", errors.New("no pull request to fetch")
	}
	if !cross && head != "" {
		if err := r.FetchRef(ctx, remote, "refs/heads/"+head, "refs/remotes/"+remote+"/"+head); err != nil {
			return "", err
		}
		return head, nil
	}
	name := fmt.Sprintf("pr-%d", number)
	if err := r.FetchRef(ctx, remote, fmt.Sprintf("refs/pull/%d/head", number), "refs/remotes/"+remote+"/"+name); err != nil {
		return "", err
	}
	return name, nil
}

// prView returns an open pull request of a pane's repository's remote, as
// its page shows it, and its changes, fetched with its base.
func (d *Daemon) prView(p proto.PRViewParams) (proto.PRView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, _, _, err := d.paneRepo(ctx, p.Pane)
	if err != nil {
		return proto.PRView{}, err
	}
	repos, byName, err := gitHubRepos(ctx, r)
	if err != nil {
		return proto.PRView{}, err
	}
	remote, err := pickRemote(ctx, r, byName, repos, p.Remote)
	if err != nil {
		return proto.PRView{}, err
	}
	repo := byName[remote]
	dt, err := github.View(ctx, r.Root, repo, p.Number)
	if err != nil {
		return proto.PRView{}, err
	}
	res := proto.PRView{PullRequest: prOf(dt.PR), State: dt.State, HeadOID: dt.HeadOID, Additions: dt.Additions, Deletions: dt.Deletions,
		ChangedFiles: dt.ChangedFiles, Labels: dt.Labels, Created: dt.Created.Unix(), Requested: dt.Requested, Files: []proto.GitFile{}}
	for _, g := range repos {
		if g.Remote == remote {
			res.Repo = g
		}
	}
	for _, v := range dt.Reviews {
		res.Reviews = append(res.Reviews, proto.PRReview{Author: v.Author, State: v.State, Body: v.Body, Submitted: v.Submitted.Unix()})
	}
	for _, c := range dt.Comments {
		res.Comments = append(res.Comments, proto.PRComment{Author: c.Author, Body: c.Body, Created: c.Created.Unix(), URL: c.URL})
	}
	for _, c := range dt.CheckRuns {
		res.CheckRuns = append(res.CheckRuns, proto.PRCheck{Name: c.Name, Workflow: c.Workflow, State: c.State, URL: c.URL})
	}
	// The rest at once: its comments on lines, who gh is signed in as, and
	// its changes, which git fetches.
	var lines []github.LineComment
	var threads map[int64]github.Thread
	var linesErr, fetchErr error
	done := make(chan struct{}, 2)
	go func() {
		lines, linesErr = github.LineComments(ctx, r.Root, repo, p.Number)
		res.Viewer, _ = github.Viewer(ctx, r.Root, repo)
		// Without them, the threads show with no Resolve.
		threads, _ = github.Threads(ctx, r.Root, repo, p.Number)
		done <- struct{}{}
	}()
	go func() {
		fetchErr = d.prChanges(ctx, r, remote, dt, &res)
		done <- struct{}{}
	}()
	<-done
	<-done
	if linesErr != nil {
		return proto.PRView{}, linesErr
	}
	for _, c := range lines {
		res.LineComments = append(res.LineComments, proto.PRLineComment{ID: c.ID, ReplyTo: c.ReplyTo, Path: c.Path, Body: c.Body, Author: c.Author,
			Created: c.Created.Unix(), URL: c.URL, Side: c.Side, Line: c.Line, StartLine: c.StartLine, OriginalLine: c.OriginalLine, File: c.File,
			Thread: threads[c.ID].ID, Resolved: threads[c.ID].Resolved})
	}
	if fetchErr != nil {
		res.FetchError = fetchErr.Error()
	}
	if cos, err := r.Checkouts(ctx); err == nil {
		for _, co := range cos {
			if co.Head == dt.HeadOID {
				res.Checkout = co.Path
				break
			}
		}
	}
	return res, nil
}

// prChanges fetches a pull request's branch and base, and puts in res the
// files it changes since its branch left the base.
func (d *Daemon) prChanges(ctx context.Context, r git.Repo, remote string, dt github.Detail, res *proto.PRView) error {
	if _, err := fetchPR(ctx, r, remote, dt.Number, dt.Head, dt.CrossRepo); err != nil {
		return err
	}
	if err := r.FetchRef(ctx, remote, "refs/heads/"+dt.Base, "refs/remotes/"+remote+"/"+dt.Base); err != nil {
		return err
	}
	mb, err := r.MergeBase(ctx, dt.HeadOID, "refs/remotes/"+remote+"/"+dt.Base)
	if err != nil {
		return err
	}
	files, err := r.Changes(ctx, mb, dt.HeadOID)
	if err != nil {
		return err
	}
	res.MergeBase = mb
	for _, f := range gitFiles(files) {
		f.From, f.To = mb, dt.HeadOID
		res.Files = append(res.Files, f)
	}
	return nil
}

// prMessage has a pull request's title and description written, as
// gitMessage has a commit message: of the commits the pane's branch brings
// into the base, their files and their diff.
func (d *Daemon) prMessage(p proto.PRMessageParams) (proto.PRMessage, error) {
	ctx, _, done := follow(p.ID, gitMessageExpiry)
	defer done()
	t, err := d.prTarget(ctx, p.Pane, p.Remote, p.Base)
	if err != nil {
		return proto.PRMessage{}, err
	}
	mb, err := t.mergeBase(ctx)
	if err != nil {
		return proto.PRMessage{}, err
	}
	commits, err := t.r.Log(ctx, git.LogOptions{Revs: []string{mb + "..HEAD"}, Max: 100})
	if err != nil {
		return proto.PRMessage{}, err
	}
	if len(commits) == 0 {
		return proto.PRMessage{}, fmt.Errorf("the branch has no commits that %s/%s hasn't", t.remote, t.base)
	}
	var log strings.Builder
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		msg := c.Subject
		if info, err := t.r.Show(ctx, c.Hash); err == nil && strings.TrimSpace(info.Message) != "" {
			msg = strings.TrimSpace(info.Message)
		}
		fmt.Fprintf(&log, "%s %s\n\n", c.Hash[:min(len(c.Hash), 10)], msg)
	}
	files, err := t.r.Changes(ctx, mb, t.st.Head)
	if err != nil {
		return proto.PRMessage{}, err
	}
	var list strings.Builder
	for _, f := range files {
		fmt.Fprintf(&list, "%s %s +%d -%d\n", f.Status, f.Path, f.Added, f.Removed)
	}
	patch, err := t.r.Patch(ctx, mb, t.st.Head)
	if err != nil {
		return proto.PRMessage{}, err
	}
	if len(patch) > 60_000 {
		patch = patch[:60_000] + "\n[the diff goes on]\n"
	}
	var agent *commitmsg.Agent
	if t.agent != nil {
		agent = &commitmsg.Agent{Name: t.agent.Agent, ID: t.agent.ID}
	}
	argv, forked, err := commitmsg.Command(p.Command, agent, userShell())
	if err != nil {
		return proto.PRMessage{}, err
	}
	at := t.r.Root
	if forked {
		at = t.dir
	}
	title, body, err := commitmsg.WritePR(ctx, at, argv, commitmsg.PRPrompt(forked, p.Prompt, log.String(), list.String(), patch))
	if errors.Is(err, context.Canceled) {
		return proto.PRMessage{}, errors.New("canceled")
	}
	return proto.PRMessage{Title: title, Body: body}, err
}
