package commitmsg

import (
	"strings"
	"testing"
)

func TestBranchName(t *testing.T) {
	for _, tc := range []struct{ reply, prefix, want string }{
		// Semantic: the model's kind of work goes in {type}.
		{"fix/login-redirect-loop", DefaultBranchPrefix, "fix/login-redirect-loop"},
		{"Docs/readme-typos", "fdts/{type}/", "fdts/docs/readme-typos"},
		// No kind given: feat, and the whole reply is the name.
		{"add-dark-mode", DefaultBranchPrefix, "feat/add-dark-mode"},
		// Static: the model's own copy of the prefix goes.
		{"kero/add-dark-mode", "kero/", "kero/add-dark-mode"},
		{"add dark mode", "kero/", "kero/add-dark-mode"},
		// None: the reply as the user's instructions had it.
		{"JIRA-12/Add Dark Mode", "", "JIRA-12/Add-Dark-Mode"},
		// What a model wraps it in, and what git refuses.
		{"`feat/a..b~c:d.lock`\nbecause it adds", DefaultBranchPrefix, "feat/a.bcd"},
		{"", "kero/", ""},
	} {
		if got := BranchName(tc.reply, tc.prefix); got != tc.want {
			t.Errorf("BranchName(%q, %q) = %q, want %q", tc.reply, tc.prefix, got, tc.want)
		}
	}
}

func TestBranchFallback(t *testing.T) {
	if got := BranchFallback("Fix the crash when saving, then add tests for it", DefaultBranchPrefix); got != "fix/fix-the-crash-when-saving" {
		t.Errorf("got %q", got)
	}
	if got := BranchFallback("  ", "kero/"); got != "kero/task" {
		t.Errorf("got %q", got)
	}
}

func TestUniqueBranch(t *testing.T) {
	taken := map[string]bool{"feat/x": true, "feat/x-2": true}
	if got := UniqueBranch("feat/x", func(s string) bool { return taken[s] }); got != "feat/x-3" {
		t.Errorf("got %q", got)
	}
}

func TestBranchPrompt(t *testing.T) {
	p := BranchPrompt("", "kero/", "add dark mode", []string{"kero/old-one"})
	for _, want := range []string{DefaultBranchPrompt, "Kero puts kero/ before the name", "kero/old-one", "Task:\nadd dark mode"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
	if p := BranchPrompt("Mine.", "", "x", nil); strings.Contains(p, DefaultBranchPrompt) || strings.Contains(p, "prefix") {
		t.Errorf("prompt with the user's instructions and no prefix:\n%s", p)
	}
}
