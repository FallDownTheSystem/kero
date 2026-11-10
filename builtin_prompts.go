package main

import (
	"kero/internal/ama"
	"kero/internal/commitmsg"
	"kero/internal/orchestrator"
	"kero/internal/settings"
)

// Kero's own prompts, which an empty prompt setting stands for, for kero
// config get --default to give an agent that changes one; the Settings
// window shows the same ones (promptSetting).
func init() {
	for name, text := range map[string]string{
		"orchestrator-prompt":    orchestrator.DefaultPrompt,
		"ama-prompt":             ama.Instructions,
		"worktree-branch-prompt": commitmsg.DefaultBranchPrompt,
		"commit-message-prompt":  commitmsg.DefaultCommitPrompt,
		"pr-message-prompt":      commitmsg.DefaultPRPrompt,
		"pr-review-prompt":       DefaultReviewPrompt,
		"pr-guide-prompt":        DefaultGuidePrompt,
	} {
		settings.SetBuiltin(name, func() string { return text })
	}
}
