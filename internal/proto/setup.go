package proto

// Setting up a worktree kero just made (plans/git.md): after its checkout,
// its submodules, then the repository's setup script, as kero.json or
// t3.json name it, in a terminal of the session that works in it, before
// or beside its agent. The app shows each as a progress card, which can
// cancel it.

// The stages of a worktree's setup, in their order.
const (
	SetupFetch      = "fetch"
	SetupCheckout   = "checkout"
	SetupSubmodules = "submodules"
	SetupScript     = "setup-script"
	SetupAgent      = "agent"
)

// A setup stage's status, and a setup's phase: running, done, failed or
// cancelled. A stage may be pending, skipped, or done with a warning too.
const (
	SetupPending   = "pending"
	SetupRunning   = "running"
	SetupDone      = "done"
	SetupSkipped   = "skipped"
	SetupWarning   = "warning"
	SetupFailed    = "failed"
	SetupCancelled = "cancelled"
)

// WorktreeSetup is the setup of the worktree at Path, on Branch, of the
// repository whose main checkout is Main, for Session once it works
// there. Script is the setup script's name, Command what it runs, in the
// terminal Pane. Error tells why it failed.
type WorktreeSetup struct {
	ID      string       `json:"id"`
	Path    string       `json:"path"`
	Branch  string       `json:"branch,omitempty"`
	Main    string       `json:"main,omitempty"`
	Session string       `json:"session,omitempty"`
	Script  string       `json:"script,omitempty"`
	Command string       `json:"command,omitempty"`
	Pane    string       `json:"pane,omitempty"`
	Phase   string       `json:"phase"`
	Stages  []SetupStage `json:"stages"`
	Error   string       `json:"error,omitempty"`
	Started int64        `json:"started"` // unix ms
	Ended   int64        `json:"ended,omitempty"`
}

// SetupStage is a stage of a worktree's setup: one of SetupFetch...,
// with its status and what it says of it, as an exit status.
type SetupStage struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Running tells that the setup goes on, which cancelling stops.
func (s WorktreeSetup) Running() bool { return s.Phase == SetupRunning }
