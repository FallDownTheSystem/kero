package proto

// What an agent asks of worktrees (kero worktree list and status, after
// t3code's t3_worktree_list and t3_worktree_status): what git worktree
// list doesn't know, the sessions in each, whether kero made it, what
// removing it would lose, and what the automatic cleanup makes of it; of
// the agent's own session, its worktree, its setup and its pull request.

type (
	// WorktreeListParams list the worktrees of the repository of Pane's,
	// else of Dir.
	WorktreeListParams struct {
		Pane string `json:"pane,omitempty"`
		Dir  string `json:"dir,omitempty"`
	}
	// WorktreeList is a repository's worktrees, the main checkout first:
	// Main is it, Default the branch Merged is of, and Cleanup whether
	// the automatic cleanup goes through the repository, "on", else why
	// not: "off", "off for a project of it" or "no project".
	WorktreeList struct {
		Main      string         `json:"main"`
		Default   string         `json:"default,omitempty"`
		Cleanup   string         `json:"cleanup"`
		Worktrees []WorktreeInfo `json:"worktrees"`
	}
	// WorktreeInfo is a worktree of the repository, as GitWorktree has it
	// (Files, Unpushed and Merged: what removing it would lose), and: Kero,
	// that kero made it, for a session or in its folder; Sessions, the ids
	// of the sessions working in it; Session and Base, the session it was
	// made for and what its branch started from. With the cleanup on,
	// Removes is why the cleanup will remove it, else Keeps why it stays.
	WorktreeInfo struct {
		GitWorktree
		Kero     bool     `json:"kero,omitempty"`
		Sessions []string `json:"sessions,omitempty"`
		Session  string   `json:"session,omitempty"`
		Base     string   `json:"base,omitempty"`
		Removes  string   `json:"removes,omitempty"`
		Keeps    string   `json:"keeps,omitempty"`
	}

	// WorktreeStatusParams ask how the session of Pane stands with its
	// worktree.
	WorktreeStatusParams struct {
		Pane string `json:"pane"`
	}
	// WorktreeStatus is where the pane works, Dir, in the worktree Root on
	// Branch, a linked one when Linked, of the repository whose main
	// checkout (the project's root) is Main, Default its default branch;
	// Own, the worktree kero made for the session, with Setup, its setup;
	// PR, the session's pull request; FromOrigin, WorktreeBase and
	// WorktreeDir the settings a new worktree goes by; Handoff, why kero
	// worktree handoff would refuse, "" when it would go.
	WorktreeStatus struct {
		Session      string         `json:"session"`
		Pane         string         `json:"pane"`
		Dir          string         `json:"dir"`
		Root         string         `json:"root,omitempty"`
		Branch       string         `json:"branch,omitempty"`
		Linked       bool           `json:"linked,omitempty"`
		Main         string         `json:"main,omitempty"`
		Default      string         `json:"default,omitempty"`
		Own          *Worktree      `json:"own,omitempty"`
		Setup        *WorktreeSetup `json:"setup,omitempty"`
		PR           *SessionPR     `json:"pr,omitempty"`
		FromOrigin   bool           `json:"fromOrigin"`
		WorktreeBase string         `json:"worktreeBase"`
		WorktreeDir  string         `json:"worktreeDir,omitempty"`
		Handoff      string         `json:"handoff,omitempty"`
	}
)
