package proto

// WorktreeCleanupResult is what a cleanup of kero's worktrees removed
// (worktree.cleanup), as kerod runs it at its start, hourly, and when the
// user asks.
type WorktreeCleanupResult struct {
	Removed []CleanedWorktree `json:"removed,omitempty"`
	// Failed are the removals that went wrong, each its error.
	Failed []string `json:"failed,omitempty"`
}

// CleanedWorktree is a worktree a cleanup removed, and the rule that did:
// "inactive", "merged", "unchanged" or "closed".
type CleanedWorktree struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Reason string `json:"reason"`
}
