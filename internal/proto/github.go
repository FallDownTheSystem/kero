package proto

// Pull requests on GitHub, which kerod reads and makes through the GitHub
// CLI (gh) on the host the repository is on.

type (
	// GitHubRepo is a remote of a pane's repository that is on GitHub:
	// Slug is OWNER/NAME as gh takes it, Default its default branch, as
	// last fetched.
	GitHubRepo struct {
		Remote  string `json:"remote"`
		Slug    string `json:"slug"`
		Owner   string `json:"owner"`
		URL     string `json:"url"`
		Default string `json:"default,omitempty"`
	}

	// PRListParams ask for the open pull requests of a pane's repository's
	// remote, "" for the one its branch pushes to, as Filter picks them:
	// "" all, "mine", or "review" (the user's review asked for).
	PRListParams struct {
		Pane   string `json:"pane"`
		Remote string `json:"remote,omitempty"`
		Filter string `json:"filter,omitempty"`
		// Dir names the repository in Pane's place, as a project's folder;
		// Viewer asks who gh is signed in as, too.
		Dir    string `json:"dir,omitempty"`
		Viewer bool   `json:"viewer,omitempty"`
	}
	// PRList is the pull requests of Remote, one of Repos, the pane's
	// repository's remotes on GitHub. Error says why there are none to
	// show, as gh missing or signed out, which leaves Repos to choose from.
	PRList struct {
		Repos  []GitHubRepo  `json:"repos"`
		Remote string        `json:"remote,omitempty"`
		PRs    []PullRequest `json:"prs"`
		Error  string        `json:"error,omitempty"`
		Viewer string        `json:"viewer,omitempty"`
	}
	// PullRequest is an open pull request. Head is its branch, of
	// HeadOwner's repository, another's than the base's when CrossRepo.
	// Checks sums its checks up: "success", "failure", "pending", or "";
	// Review is GitHub's review decision.
	PullRequest struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body,omitempty"`
		URL       string `json:"url"`
		Author    string `json:"author"`
		Head      string `json:"head"`
		HeadOwner string `json:"headOwner,omitempty"`
		CrossRepo bool   `json:"crossRepo,omitempty"`
		Base      string `json:"base"`
		Draft     bool   `json:"draft,omitempty"`
		Checks    string `json:"checks,omitempty"`
		Review    string `json:"review,omitempty"`
		Updated   int64  `json:"updated"` // unix seconds
	}

	// PRDraftParams ask what a pull request of a pane's branch into Base,
	// of the remote's repository (Remote, "" for the branch's push
	// remote's; Base "" for its default branch), would bring.
	PRDraftParams struct {
		Pane   string `json:"pane"`
		Remote string `json:"remote,omitempty"`
		Base   string `json:"base,omitempty"`
	}
	// PRDraft is a pull request to make: Head, the pane's branch, as
	// HeadOwner's repository (the push remote's) has it, into Base of
	// Remote's. Commits and Files are what it brings: since MergeBase,
	// the commits that Base, as last fetched, hasn't, and their files,
	// each with the sides git.file shows it between. Unpushed counts the
	// commits the push remote hasn't, which go first, and Uncommitted the
	// files changed in the worktree, which it leaves out. Existing is the
	// open pull request of Head already, nil for none.
	PRDraft struct {
		Repos       []GitHubRepo `json:"repos"`
		Remote      string       `json:"remote"`
		Base        string       `json:"base"`
		Head        string       `json:"head"`
		HeadOwner   string       `json:"headOwner"`
		PushRemote  string       `json:"pushRemote"`
		MergeBase   string       `json:"mergeBase,omitempty"`
		Commits     []GitCommit  `json:"commits"`
		Files       []GitFile    `json:"files"`
		Unpushed    int          `json:"unpushed,omitempty"`
		Uncommitted int          `json:"uncommitted,omitempty"`
		Existing    *PullRequest `json:"existing,omitempty"`
		// Error says why the existing pull request isn't known, as gh
		// missing, which leaves the rest to show and Show on GitHub.
		Error string `json:"error,omitempty"`
	}

	// PRCreateParams make a pull request of a pane's branch, pushed
	// already, into Base of Remote's repository; or, with Show, only the
	// URL of GitHub's page that makes it, its fields filled in.
	PRCreateParams struct {
		Pane   string `json:"pane"`
		Remote string `json:"remote,omitempty"`
		Base   string `json:"base"`
		Title  string `json:"title"`
		Body   string `json:"body,omitempty"`
		Draft  bool   `json:"draft,omitempty"`
		Show   bool   `json:"show,omitempty"`
	}
	// PRCreated is the pull request made, or the page to make it on.
	PRCreated struct {
		URL string `json:"url"`
	}

	// PRFetchParams fetch an open pull request's branch, Head (of another
	// repository when CrossRepo), from Remote, for a session to check out
	// in a worktree of its own.
	PRFetchParams struct {
		Pane      string `json:"pane"`
		Remote    string `json:"remote"`
		Number    int    `json:"number"`
		Head      string `json:"head"`
		CrossRepo bool   `json:"crossRepo,omitempty"`
	}
	// PRFetched is the review's own branch to check the pull request out
	// on, made from Base, and where it pushes (CreateParams).
	PRFetched struct {
		Branch     string `json:"branch"`
		Base       string `json:"base"`
		PushRemote string `json:"pushRemote,omitempty"`
		PushBranch string `json:"pushBranch,omitempty"`
	}

	// PRCheckoutParams check an open pull request out (pr.checkout) on a
	// branch of its own name, Head, or pr-<number> of another repository's
	// (CrossRepo): in Pane's worktree when Here, which refuses one with
	// changes, else for a session in a new worktree of its own.
	PRCheckoutParams struct {
		Pane      string `json:"pane"`
		Remote    string `json:"remote"`
		Number    int    `json:"number"`
		Head      string `json:"head"`
		CrossRepo bool   `json:"crossRepo,omitempty"`
		Here      bool   `json:"here,omitempty"`
	}
	// PRCheckedOut is where the pull request is checked out: Worktree, a
	// worktree of the repository's that has it already, which nothing
	// changed; else on Branch, in the pane's worktree (Here), or to check
	// out in a new worktree, from Base, pushing where PRFetched's does.
	PRCheckedOut struct {
		Branch     string `json:"branch"`
		Worktree   string `json:"worktree,omitempty"`
		Base       string `json:"base,omitempty"`
		PushRemote string `json:"pushRemote,omitempty"`
		PushBranch string `json:"pushBranch,omitempty"`
	}

	// PRMessageParams ask for a pull request's title and description of
	// what a pane's branch brings into Base of Remote's repository,
	// written as GitMessageParams' commit message is: by Command, after
	// Prompt, the user's words of how to write them, "" for kerod's own.
	// Repos are prompts of repositories, as GitMessageParams' are.
	PRMessageParams struct {
		Pane    string            `json:"pane"`
		ID      string            `json:"id,omitempty"`
		Remote  string            `json:"remote,omitempty"`
		Base    string            `json:"base"`
		Command string            `json:"command,omitempty"`
		Prompt  string            `json:"prompt,omitempty"`
		Repos   map[string]string `json:"repos,omitempty"`
	}
	PRMessage struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}

	// PRViewParams ask for an open pull request of a remote of a pane's
	// repository, with its changes.
	PRViewParams struct {
		Pane   string `json:"pane"`
		Remote string `json:"remote,omitempty"`
		Number int    `json:"number"`
	}
	// PRView is a pull request as its page shows it. Its changes are those
	// from MergeBase, where its branch left the base, to HeadOID, both
	// fetched; FetchError says why they aren't, which leaves Files empty.
	// Viewer is the user gh is signed in as.
	PRView struct {
		PullRequest
		Repo         GitHubRepo      `json:"repo"`
		State        string          `json:"state"` // OPEN, CLOSED or MERGED
		HeadOID      string          `json:"headOid"`
		Additions    int             `json:"additions"`
		Deletions    int             `json:"deletions"`
		ChangedFiles int             `json:"changedFiles"`
		Labels       []string        `json:"labels,omitempty"`
		Created      int64           `json:"created"` // unix seconds
		Requested    []string        `json:"requested,omitempty"`
		Reviews      []PRReview      `json:"reviews,omitempty"`
		Comments     []PRComment     `json:"comments,omitempty"`
		CheckRuns    []PRCheck       `json:"checkRuns,omitempty"`
		LineComments []PRLineComment `json:"lineComments,omitempty"`
		MergeBase    string          `json:"mergeBase,omitempty"`
		Files        []GitFile       `json:"files"`
		FetchError   string          `json:"fetchError,omitempty"`
		Viewer       string          `json:"viewer,omitempty"`
		// Checkout is a worktree of the repository's that has HeadOID
		// checked out, whose files are the pull request's; "" for none.
		Checkout string `json:"checkout,omitempty"`
	}
	// PRReview is a review given: State is APPROVED, CHANGES_REQUESTED,
	// COMMENTED, DISMISSED or PENDING.
	PRReview struct {
		Author    string `json:"author"`
		State     string `json:"state"`
		Body      string `json:"body,omitempty"`
		Submitted int64  `json:"submitted"`
	}
	// PRComment is a comment of a pull request's conversation.
	PRComment struct {
		Author  string `json:"author"`
		Body    string `json:"body"`
		Created int64  `json:"created"`
		URL     string `json:"url,omitempty"`
	}
	// PRDraftsParams ask for the draft reviews of the pull requests of
	// Slug's repository, which pr.drafts answers with, as []PRReviewDraft.
	PRDraftsParams struct {
		Slug string `json:"slug"`
	}
	// PRReviewRef names a pull request: its repository's slug, as
	// GitHubRepo's, and its number.
	PRReviewRef struct {
		Slug   string `json:"slug"`
		Number int    `json:"number"`
	}
	// PRWorktreesParams ask for the worktrees reviews checked pull
	// requests out in, of Pane's repository, which pr.worktrees answers
	// with, as []ReviewWorktree.
	PRWorktreesParams struct {
		Pane string `json:"pane"`
	}
	// ReviewWorktree is a worktree a review checked pull request Number
	// out in, on Branch, a review/pr-<number> one: Files are its changed
	// files, untracked ones too, which removing it would lose.
	ReviewWorktree struct {
		Number int    `json:"number"`
		Path   string `json:"path"`
		Branch string `json:"branch"`
		Files  int    `json:"files"`
	}
	// PRWorktreeRemoveParams remove a review's worktree at Path, of Pane's
	// repository, that no session works in; one with changed files only
	// with Force.
	PRWorktreeRemoveParams struct {
		Pane  string `json:"pane"`
		Path  string `json:"path"`
		Force bool   `json:"force,omitempty"`
	}

	// PRReviewParams act on a pull request's draft review, by Op:
	//
	//   - "get": the draft, if any.
	//   - "start": a draft for Agent, the pane of the agent that reviews,
	//     at the pull request's head, which kerod fetches.
	//   - "comment": Comment added, or, with its ID, edited.
	//   - "delete": the comment ID deleted.
	//   - "summary": the review's Summary.
	//   - "post": the review posted on GitHub with Event, as one review,
	//     then the draft gone.
	//   - "discard": the draft gone.
	//
	// The pull request is Number of Remote's repository of Pane's; with no
	// Number, the one whose draft Pane's agent writes.
	PRReviewParams struct {
		Op      string         `json:"op"`
		Pane    string         `json:"pane"`
		Remote  string         `json:"remote,omitempty"`
		Number  int            `json:"number,omitempty"`
		Agent   string         `json:"agent,omitempty"`
		Comment PRDraftComment `json:"comment,omitempty"`
		ID      int            `json:"id,omitempty"`
		Summary string         `json:"summary,omitempty"`
		Event   string         `json:"event,omitempty"`
	}
	// PRDraftComment is a comment of a draft review on lines Start to Line,
	// from 1, of Side's file: RIGHT the pull request's, LEFT the base's;
	// Start 0 for Line alone. File makes it a comment on the file, with no
	// lines, Side "". By is "agent" or "user".
	PRDraftComment struct {
		ID      int    `json:"id,omitempty"`
		Path    string `json:"path"`
		Side    string `json:"side"`
		Start   int    `json:"start,omitempty"`
		Line    int    `json:"line"`
		File    bool   `json:"file,omitempty"`
		Body    string `json:"body"`
		By      string `json:"by,omitempty"`
		Created int64  `json:"created,omitempty"`
	}
	// PRReviewDraft is a pull request's review being written, at Head, of
	// the changes since MergeBase: kept by kerod until it is posted or
	// discarded. Agent is the pane of the agent that writes it, and
	// Session that pane's session, which outlives the pane: drafts kept
	// from before kerod named it have Agent alone.
	PRReviewDraft struct {
		Repo      string           `json:"repo"` // its slug
		Remote    string           `json:"remote"`
		Number    int              `json:"number"`
		Title     string           `json:"title,omitempty"`
		Head      string           `json:"head"`
		MergeBase string           `json:"mergeBase"`
		Agent     string           `json:"agent,omitempty"`
		Session   string           `json:"session,omitempty"`
		Summary   string           `json:"summary,omitempty"`
		Comments  []PRDraftComment `json:"comments"`
		Updated   int64            `json:"updated"`
		// Posted is the review's URL, once posted, in post's answer.
		Posted string `json:"posted,omitempty"`
	}

	// PRCheck is a check of the pull request's head: State is "success",
	// "failure", "pending", "skipped" or "neutral".
	PRCheck struct {
		Name     string `json:"name"`
		Workflow string `json:"workflow,omitempty"`
		State    string `json:"state"`
		URL      string `json:"url,omitempty"`
	}
	// PRLineComment is a comment of a review on lines of a file: Line and
	// StartLine, from 1, of Side's file, LEFT the base's or RIGHT the
	// head's; Line 0 when the lines changed since, which leaves it
	// outdated, at OriginalLine. ReplyTo is the comment it answers, 0 for
	// one that starts a thread.
	PRLineComment struct {
		ID           int64  `json:"id"`
		ReplyTo      int64  `json:"replyTo,omitempty"`
		Path         string `json:"path"`
		Body         string `json:"body"`
		Author       string `json:"author"`
		Created      int64  `json:"created"`
		URL          string `json:"url,omitempty"`
		Side         string `json:"side"`
		Line         int    `json:"line,omitempty"`
		StartLine    int    `json:"startLine,omitempty"`
		OriginalLine int    `json:"originalLine,omitempty"`
		// File is a comment on the file, not on lines, which has none.
		File bool `json:"file,omitempty"`
		// Thread is the GraphQL id of the thread a comment starts, which
		// pr.thread resolves, "" when unknown; Resolved, whether it is.
		Thread   string `json:"thread,omitempty"`
		Resolved bool   `json:"resolved,omitempty"`
	}
	// PRThreadParams resolve a thread of comments of pull request Number
	// of Remote's repository of Pane's, by its id (PRLineComment.Thread),
	// or, unless Resolve, make it unresolved again.
	PRThreadParams struct {
		Pane    string `json:"pane"`
		Remote  string `json:"remote,omitempty"`
		Number  int    `json:"number"`
		Thread  string `json:"thread"`
		Resolve bool   `json:"resolve,omitempty"`
	}
	// PRActionParams do Op (PRAct*) to pull request Number of Remote's
	// repository of Pane's, or, with Op "" (PRActState), only read what
	// its actions depend on. Method is a merge's (merge, squash or
	// rebase), or with PRActUpdateBranch, rebase to rebase the branch;
	// DeleteBranch deletes the branch once merged; Head is the commit the
	// user saw it at, which a merge merges at alone, and runs are approved
	// of alone.
	PRActionParams struct {
		Pane         string `json:"pane"`
		Remote       string `json:"remote,omitempty"`
		Number       int    `json:"number"`
		Op           string `json:"op,omitempty"`
		Method       string `json:"method,omitempty"`
		DeleteBranch bool   `json:"deleteBranch,omitempty"`
		Head         string `json:"head,omitempty"`
	}
	// PRActionResult is what came of an action: the URL of the pull
	// request a revert opened, how many runs were approved, and, read
	// after it, what the pull request's actions depend on now; nil when
	// it couldn't be read.
	PRActionResult struct {
		URL      string     `json:"url,omitempty"`
		Approved int        `json:"approved,omitempty"`
		State    *PRActions `json:"state,omitempty"`
	}
	// PRActions is what a pull request's actions depend on: its state
	// (OPEN, CLOSED or MERGED), whether it merges as it is (Mergeable:
	// MERGEABLE, CONFLICTING or UNKNOWN; MergeState GitHub's
	// mergeStateStatus, as BEHIND, BLOCKED or CLEAN), the method it
	// auto-merges with, "" for none, the merge methods the repository
	// allows, and what the user may do. PendingRuns counts the workflow
	// runs of a fork's head that wait on a maintainer's approval.
	PRActions struct {
		State            string   `json:"state"`
		Draft            bool     `json:"draft,omitempty"`
		Head             string   `json:"head"`
		HeadOID          string   `json:"headOid"`
		Base             string   `json:"base"`
		CrossRepo        bool     `json:"crossRepo,omitempty"`
		HeadExists       bool     `json:"headExists,omitempty"`
		Mergeable        string   `json:"mergeable,omitempty"`
		MergeState       string   `json:"mergeState,omitempty"`
		AutoMerge        string   `json:"autoMerge,omitempty"`
		Methods          []string `json:"methods"`
		AutoMergeAllowed bool     `json:"autoMergeAllowed,omitempty"`
		DeleteOnMerge    bool     `json:"deleteOnMerge,omitempty"`
		CanWrite         bool     `json:"canWrite,omitempty"`
		CanUpdate        bool     `json:"canUpdate,omitempty"`
		CanUpdateBranch  bool     `json:"canUpdateBranch,omitempty"`
		CanDeleteBranch  bool     `json:"canDeleteBranch,omitempty"`
		PendingRuns      int      `json:"pendingRuns,omitempty"`
	}
)

// pr.action's ops, as PRActionParams.Op names them.
const (
	PRActState            = ""
	PRActMerge            = "merge"
	PRActAutoMerge        = "auto-merge"
	PRActDisableAutoMerge = "disable-auto-merge"
	PRActReady            = "ready"
	PRActDraft            = "draft"
	PRActClose            = "close"
	PRActReopen           = "reopen"
	PRActDeleteBranch     = "delete-branch"
	PRActUpdateBranch     = "update-branch"
	PRActRevert           = "revert"
	PRActApproveRuns      = "approve-runs"
)

type (
	// PRGuideParams act on a pull request's review guide, the tour of it
	// the Guide tab shows, by Op:
	//
	//   - "get": the guide, if any.
	//   - "start": a guide anew, for Agent, the pane of the agent that
	//     writes it, at the pull request's head, which kerod fetches. A
	//     stop or summary with no guide yet starts one, with no Agent;
	//     one of an older head than the pull request's starts anew.
	//   - "stop": Stop added, at At (from 1; 0 last), or, with its ID,
	//     edited, and moved to At when given.
	//   - "delete": the stop ID deleted.
	//   - "summary": the guide's Summary.
	//   - "clear": its stops and summary gone, to write it anew.
	//   - "discard": the guide gone.
	//
	// The pull request is Number of Remote's repository of Pane's; with no
	// Number, the one whose guide, else draft review, Pane's agent writes.
	PRGuideParams struct {
		Op      string      `json:"op"`
		Pane    string      `json:"pane"`
		Remote  string      `json:"remote,omitempty"`
		Number  int         `json:"number,omitempty"`
		Agent   string      `json:"agent,omitempty"`
		Stop    PRGuideStop `json:"stop,omitempty"`
		At      int         `json:"at,omitempty"`
		ID      int         `json:"id,omitempty"`
		Summary string      `json:"summary,omitempty"`
	}
	// PRGuideStop is a stop of a guide: lines Start to Line, from 1, of
	// Side's file, RIGHT the pull request's, LEFT the base's; Line 0 for
	// the whole file, Side "". Closely marks a stop with a call only a
	// maintainer can make ("Look closely").
	PRGuideStop struct {
		ID      int    `json:"id,omitempty"`
		Path    string `json:"path"`
		Side    string `json:"side,omitempty"`
		Start   int    `json:"start,omitempty"`
		Line    int    `json:"line,omitempty"`
		Title   string `json:"title"`
		Note    string `json:"note,omitempty"`
		Closely bool   `json:"closely,omitempty"`
	}
	// PRGuide is a pull request's review guide, written at Head, of the
	// changes since MergeBase: its summary, then its stops in reading
	// order. kerod keeps it apart from the draft review, until discarded;
	// nothing of it goes to GitHub. Agent and Session are the agent's that
	// writes it, as PRReviewDraft's.
	PRGuide struct {
		Repo      string        `json:"repo"` // its slug
		Remote    string        `json:"remote"`
		Number    int           `json:"number"`
		Title     string        `json:"title,omitempty"`
		Head      string        `json:"head"`
		MergeBase string        `json:"mergeBase"`
		Agent     string        `json:"agent,omitempty"`
		Session   string        `json:"session,omitempty"`
		Summary   string        `json:"summary,omitempty"`
		Stops     []PRGuideStop `json:"stops"`
		Updated   int64         `json:"updated"`
	}
)
