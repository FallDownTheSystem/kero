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
	// PRFetched is the branch to check the pull request out on, made from
	// Base when the repository has none of its name (CreateParams).
	PRFetched struct {
		Branch string `json:"branch"`
		Base   string `json:"base"`
	}

	// PRMessageParams ask for a pull request's title and description of
	// what a pane's branch brings into Base of Remote's repository,
	// written as GitMessageParams' commit message is: by Command, after
	// Prompt, the user's words of how to write them, "" for kerod's own.
	PRMessageParams struct {
		Pane    string `json:"pane"`
		ID      string `json:"id,omitempty"`
		Remote  string `json:"remote,omitempty"`
		Base    string `json:"base"`
		Command string `json:"command,omitempty"`
		Prompt  string `json:"prompt,omitempty"`
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
	// PRReviewRef names a pull request: its repository's slug, as
	// GitHubRepo's, and its number.
	PRReviewRef struct {
		Slug   string `json:"slug"`
		Number int    `json:"number"`
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
	// Start 0 for Line alone. By is "agent" or "user".
	PRDraftComment struct {
		ID      int    `json:"id,omitempty"`
		Path    string `json:"path"`
		Side    string `json:"side"`
		Start   int    `json:"start,omitempty"`
		Line    int    `json:"line"`
		Body    string `json:"body"`
		By      string `json:"by,omitempty"`
		Created int64  `json:"created,omitempty"`
	}
	// PRReviewDraft is a pull request's review being written, at Head, of
	// the changes since MergeBase: kept by kerod until it is posted or
	// discarded. Agent is the pane of the agent that writes it.
	PRReviewDraft struct {
		Repo      string           `json:"repo"` // its slug
		Remote    string           `json:"remote"`
		Number    int              `json:"number"`
		Title     string           `json:"title,omitempty"`
		Head      string           `json:"head"`
		MergeBase string           `json:"mergeBase"`
		Agent     string           `json:"agent,omitempty"`
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
	}
)
