package settings

import (
	"path/filepath"
	"testing"
)

// A prompt goes in and comes out as its lines; a repository's line is set
// and removed alone; --default gives Kero's own prompt where "" stands
// for it.
func TestValue(t *testing.T) {
	t.Setenv("KERO_CONFIG", filepath.Join(t.TempDir(), "config"))
	prompt := "Review #{number}.\n\nSay \"why\" # and C:\\notes."
	if err := SetValue("pr-review-prompt", "", prompt); err != nil {
		t.Fatal(err)
	}
	if v, _ := Value("pr-review-prompt", "", false); v != prompt {
		t.Errorf("got %q", v)
	}

	SetBuiltin("pr-guide-prompt", func() string { return "Kero's guide" })
	if v, _ := Value("pr-guide-prompt", "", true); v != "Kero's guide" {
		t.Errorf("default %q", v)
	}
	if v, _ := Value("pr-guide-prompt", "", false); v != "" {
		t.Errorf("unset %q", v)
	}

	if err := SetValue("review-prompt", "Acme/One", "first\nline"); err != nil {
		t.Fatal(err)
	}
	if err := SetValue("review-prompt", "acme/two", "second"); err != nil {
		t.Fatal(err)
	}
	if v, _ := Value("review-prompt", "acme/one", false); v != "first\nline" {
		t.Errorf("acme/one %q", v)
	}
	if err := SetValue("review-prompt", "acme/one", ""); err != nil {
		t.Fatal(err)
	}
	if v, _ := Value("review-prompt", "acme/one", false); v != "" {
		t.Errorf("acme/one not removed: %q", v)
	}
	if v, _ := Value("review-prompt", "acme/two", false); v != "second" {
		t.Errorf("acme/two lost: %q", v)
	}
	if err := SetValue("review-command", "acme/two", "cc -p"); err != nil {
		t.Fatal(err)
	}
	if v, _ := Value("review-prompt", "acme/two", false); v != "second" {
		t.Errorf("another key of the repository changed its prompt: %q", v)
	}

	if _, err := Value("pr-review-prompt", "acme/two", false); err == nil {
		t.Error("a repository taken by a setting for every repository")
	}
	if err := SetValue("font-size", "", "15"); err != nil {
		t.Fatal(err)
	}
	if v, _ := Value("font-size", "", false); v != "15" {
		t.Errorf("font-size %q", v)
	}
}
