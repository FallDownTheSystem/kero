package proto

type (
	// HandoffParams move the session of Pane, whose agent asks (kero
	// worktree handoff), into a new worktree of its repository on a new
	// Branch from Base, as CreateParams.Worktree and Base make one: once
	// the agent's turn ends, it starts again there, resuming its
	// conversation, and goes on with Prompt when there is one.
	HandoffParams struct {
		Pane   string `json:"pane"`
		Branch string `json:"branch"`
		Base   string `json:"base,omitempty"`
		Prompt string `json:"prompt,omitempty"`
	}
	// HandoffResult is the worktree made for the session, and Dir, the
	// folder of it the agent starts again in: the one it worked in, of
	// the worktree.
	HandoffResult struct {
		Worktree
		Session string `json:"session"`
		Pane    string `json:"pane"`
		Dir     string `json:"dir"`
	}
)
