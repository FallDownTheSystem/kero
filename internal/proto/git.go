package proto

// What the Changes and History views ask kerod to do with a pane's
// worktree, beyond showing its changes: staging and commits, branches,
// stashes, the log, and fetching, pulling and pushing.

type (
	// GitIndex is a worktree's uncommitted changes: staged, and not, with
	// untracked files ("untracked") and conflicts ("conflict"). Message
	// is what git prepared for the commit that ends a merge, a revert or
	// a cherry-pick in progress; Ours and Theirs name its sides, HEAD's
	// and what comes into it, as a branch or a short hash.
	GitIndex struct {
		Root     string    `json:"root"`
		Staged   []GitFile `json:"staged"`
		Unstaged []GitFile `json:"unstaged"`
		Message  string    `json:"message,omitempty"`
		Ours     string    `json:"ours,omitempty"`
		Theirs   string    `json:"theirs,omitempty"`
	}

	// GitOpParams do an operation of git's in a pane's worktree (git.op).
	// ID, set by the client, follows a long one's progress (git.progress)
	// and cancels it (git.cancel). The other fields are the operation's,
	// by Op:
	//
	//	stage, unstage      Paths, or all
	//	discard             Paths tracked, Untracked
	//	setIndex            Path, Known (the index's hash), Text
	//	commit              Message, Amend, Signoff, NoVerify
	//	lastMessage, undo, continue, abort
	//	cherryPick, revert, checkout   Rev
	//	merge               Rev, Mode: "" or no-ff
	//	rebase              Rev, what to rebase onto, Autostash
	//	rebaseInteractive   Rev, Steps, Backup (a branch to make at HEAD first, or ""), Autostash
	//	resolve             Path, Mode: ours or theirs
	//	reset               Rev, Mode: soft, mixed or hard
	//	switch              Name, Remote, Rev (the commit it is to be at, or "")
	//	createBranch        Name, Rev ("" HEAD), Checkout
	//	deleteBranch        Name, Force
	//	renameBranch        Name, To
	//	stash               Message, IncludeUntracked, Staged
	//	stashApply, stashPop, stashDrop   Index, Hash
	//	fetch               All, Prune
	//	pull                Mode: "", merge, rebase or ff-only
	//	push                Force, Tags
	GitOpParams struct {
		Pane string `json:"pane"`
		Op   string `json:"op"`
		ID   string `json:"id,omitempty"`
		// Root is the worktree the user chose the operation in, which the
		// pane must still work in: else it is refused, rather than done in
		// the one the pane went to since, as a queued one could be.
		Root string `json:"root,omitempty"`

		Paths     []string `json:"paths,omitempty"`
		Untracked []string `json:"untracked,omitempty"`
		Path      string   `json:"path,omitempty"`
		Known     string   `json:"known,omitempty"`
		Text      string   `json:"text,omitempty"`

		Message  string `json:"message,omitempty"`
		Amend    bool   `json:"amend,omitempty"`
		Signoff  bool   `json:"signoff,omitempty"`
		NoVerify bool   `json:"noVerify,omitempty"`

		Rev      string `json:"rev,omitempty"`
		Mode     string `json:"mode,omitempty"`
		Name     string `json:"name,omitempty"`
		To       string `json:"to,omitempty"`
		Remote   bool   `json:"remote,omitempty"`
		Checkout bool   `json:"checkout,omitempty"`
		Force    bool   `json:"force,omitempty"`

		Autostash bool `json:"autostash,omitempty"`
		// Steps and Backup are an interactive rebase's.
		Steps  []GitRebaseStep `json:"steps,omitempty"`
		Backup string          `json:"backup,omitempty"`

		IncludeUntracked bool   `json:"includeUntracked,omitempty"`
		Staged           bool   `json:"staged,omitempty"`
		Index            int    `json:"index,omitempty"`
		Hash             string `json:"hash,omitempty"`

		All   bool `json:"all,omitempty"`
		Prune bool `json:"prune,omitempty"`
		Tags  bool `json:"tags,omitempty"`
	}
	// GitRebaseStep is a commit of an interactive rebase: Action pick,
	// reword (with Message), edit, squash, fixup or drop.
	GitRebaseStep struct {
		Action  string `json:"action"`
		Hash    string `json:"hash"`
		Message string `json:"message,omitempty"`
	}
	// GitRebasePlan is what an interactive rebase onto a commit replays:
	// HEAD's branch's commits since it, the oldest first (git.rebasePlan,
	// of GitShowParams' Rev).
	GitRebasePlan struct {
		Branch  string      `json:"branch,omitempty"` // "" when detached
		Onto    GitCommit   `json:"onto"`
		Commits []GitCommit `json:"commits"`
	}

	// GitOpResult is how an operation went: Error, the line of git's that
	// says what went wrong, "" when it went through; Output all git and the
	// hooks printed. Command is the command of one that reaches a remote,
	// to run again in a terminal, where git can ask for a password.
	GitOpResult struct {
		Error   string   `json:"error,omitempty"`
		Output  string   `json:"output,omitempty"`
		Command []string `json:"command,omitempty"`
	}
	// GitProgress is an operation's progress: the line git printed last.
	GitProgress struct {
		Line string `json:"line,omitempty"`
	}

	// GitBranches are a worktree's repository's branches, local ones
	// first, and its remotes.
	GitBranches struct {
		Branches []GitBranch `json:"branches"`
		Remotes  []string    `json:"remotes,omitempty"`
	}
	GitBranch struct {
		Name     string `json:"name"`
		Remote   bool   `json:"remote,omitempty"`
		Current  bool   `json:"current,omitempty"`
		Head     string `json:"head"`
		Upstream string `json:"upstream,omitempty"`
		Ahead    int    `json:"ahead,omitempty"`
		Behind   int    `json:"behind,omitempty"`
		Gone     bool   `json:"gone,omitempty"`
		Time     int64  `json:"time,omitempty"` // its commit's, in seconds
		Subject  string `json:"subject,omitempty"`
		// Worktree is the worktree that has it checked out.
		Worktree string `json:"worktree,omitempty"`
	}

	GitStashes struct {
		Stashes []GitStash `json:"stashes"`
	}
	// GitStash is an entry of the stash: stash@{Index}, as long as it is
	// still Hash.
	GitStash struct {
		Index   int    `json:"index"`
		Hash    string `json:"hash"`
		Time    int64  `json:"time,omitempty"`
		Message string `json:"message"`
	}

	// GitLogParams ask for a page of a pane's worktree's history: of its
	// branch, its upstream and the default branch, or All, every branch's
	// and tag's. Query finds commits by their message, author or hash, and
	// Path those that changed a file, both in the order of their time, as
	// no graph draws them.
	GitLogParams struct {
		Pane        string `json:"pane"`
		All         bool   `json:"all,omitempty"`
		FirstParent bool   `json:"firstParent,omitempty"`
		Skip        int    `json:"skip,omitempty"`
		Max         int    `json:"max,omitempty"`
		Query       string `json:"query,omitempty"`
		Path        string `json:"path,omitempty"`
	}
	// GitLog is a page of history. More: there are more after it.
	// Unpushed are its branch's commits its upstream hasn't, or no
	// remote's branch, and Remotes the repository's remotes, which its
	// commits' refs name branches of, with the first page.
	GitLog struct {
		Commits  []GitCommit `json:"commits"`
		More     bool        `json:"more,omitempty"`
		Unpushed []string    `json:"unpushed,omitempty"`
		Remotes  []string    `json:"remotes,omitempty"`
	}
	GitCommit struct {
		Hash    string   `json:"hash"`
		Parents []string `json:"parents,omitempty"`
		Author  string   `json:"author"`
		Email   string   `json:"email,omitempty"`
		Time    int64    `json:"time"` // the author's, in seconds
		Refs    string   `json:"refs,omitempty"`
		Subject string   `json:"subject"`
	}
	// GitShowParams ask for a commit, or a stash's.
	GitShowParams struct {
		Pane string `json:"pane"`
		Rev  string `json:"rev"`
	}
	// GitCommitInfo is a commit whole: its message, and its files, each
	// with the sides git.file shows it between.
	GitCommitInfo struct {
		GitCommit
		Committer string    `json:"committer,omitempty"`
		Message   string    `json:"message"`
		Files     []GitFile `json:"files"`
	}

	// GitMessageParams ask for a commit message of what a pane's worktree
	// has staged, written by Command, given what to write on its stdin;
	// "" for a fork of the conversation of the pane's agent, else a new
	// one of an agent's.
	GitMessageParams struct {
		Pane    string `json:"pane"`
		ID      string `json:"id,omitempty"`
		Command string `json:"command,omitempty"`
	}
	GitMessage struct {
		Message string `json:"message"`
	}
)
