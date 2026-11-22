package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"kero/internal/assistant"
	"kero/internal/orchestrator"
	"kero/internal/proto"
)

func TestSkillLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links need Developer Mode on Windows: a copy then")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KERO_HOME", filepath.Join(home, "kero"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	adopt := adoptCopies
	adoptCopies = func() bool { return true }
	defer func() { adoptCopies = adopt }()
	claude := skillDirs("claude")["claude"]
	agents := skillDirs("agents")["agents"]

	// An older kero's install: a copy, of another skill.
	os.MkdirAll(claude, 0o755)
	os.WriteFile(filepath.Join(claude, "SKILL.md"), []byte("old"), 0o644)
	SyncSkill()
	if t1, err := os.Readlink(filepath.Join(claude, "SKILL.md")); err != nil || t1 != skillFile() {
		t.Fatalf("the copy is no link to Kero's own: %q %v", t1, err)
	}
	if skillState(claude) != "current" {
		t.Errorf("state %s", skillState(claude))
	}
	// SyncSkill installs nothing that isn't.
	if _, err := os.Stat(agents); err == nil {
		t.Errorf("SyncSkill installed %s", agents)
	}

	// Kero updates: its own copy follows, and the install with it.
	os.WriteFile(skillFile(), []byte("of an older Kero"), 0o644)
	if skillState(claude) != "outdated" {
		t.Errorf("state %s", skillState(claude))
	}
	SyncSkill()
	if b, _ := os.ReadFile(filepath.Join(claude, "SKILL.md")); string(b) != skillGuide {
		t.Errorf("the install reads %q", b)
	}

	if err := InstallSkill("agents"); err != nil {
		t.Fatal(err)
	}
	if t1, _ := os.Readlink(filepath.Join(agents, "SKILL.md")); t1 != skillFile() {
		t.Errorf("installed as %q", t1)
	}
	if err := RemoveSkill("agents"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skillFile()); err != nil {
		t.Errorf("removing an install took Kero's own copy: %v", err)
	}

	// Another Kero's link, as the installed Kero's to a development
	// build: SyncSkill leaves it, and so does a build that takes no copies.
	other := filepath.Join(home, "other", "SKILL.md")
	os.MkdirAll(filepath.Dir(other), 0o755)
	os.WriteFile(other, []byte("another Kero's"), 0o644)
	os.Remove(filepath.Join(claude, "SKILL.md"))
	os.Symlink(other, filepath.Join(claude, "SKILL.md"))
	os.MkdirAll(agents, 0o755)
	os.WriteFile(filepath.Join(agents, "SKILL.md"), []byte("a copy"), 0o644)
	adoptCopies = func() bool { return false }
	SyncSkill()
	if t1, _ := os.Readlink(filepath.Join(claude, "SKILL.md")); t1 != other {
		t.Errorf("another Kero's link taken: %q", t1)
	}
	if b, _ := os.ReadFile(filepath.Join(agents, "SKILL.md")); string(b) != "a copy" {
		t.Errorf("a build that takes no copies took one: %q", b)
	}

	// A plugin of an older kero is made current; another Kero's, or one
	// not there, stays as it is.
	pi := filepath.Join(home, ".pi", "agent")
	t.Setenv("PI_CODING_AGENT_DIR", pi)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	ts := filepath.Join(pi, "extensions", "kero.ts")
	os.MkdirAll(filepath.Dir(ts), 0o755)
	os.WriteFile(ts, []byte(`run("/elsewhere/kero-dev/bin/kero")`), 0o644)
	SyncSkill()
	if b, _ := os.ReadFile(ts); string(b) != `run("/elsewhere/kero-dev/bin/kero")` {
		t.Errorf("another Kero's plugin taken: %q", b)
	}
	os.WriteFile(ts, append([]byte("// older\nrun("), append(marshal(kero()), ')')...), 0o644)
	SyncSkill()
	for _, it := range agentHooks("pi") {
		if it.state() != "current" {
			t.Errorf("%s: %s", it.name(), it.state())
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode")); err == nil {
		t.Error("SyncSkill installed OpenCode's plugin")
	}
}

// OMP's extension goes under PI_CONFIG_DIR, which moves its config root,
// and its hook names OMP, so Kero resumes it with `omp --session`.
func TestOMPHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KERO_HOME", filepath.Join(home, "kero"))
	t.Setenv("PI_CONFIG_DIR", filepath.Join(home, "omp"))
	ts := filepath.Join(home, "omp", "agent", "extensions", "kero.ts")
	os.MkdirAll(filepath.Dir(ts), 0o755)
	var plugin agentPlugin
	for _, it := range agentHooks("omp") {
		if p, ok := it.(agentPlugin); ok && p.agent == "omp" {
			plugin = p
		}
	}
	if plugin.dir != filepath.Dir(ts) {
		t.Fatalf("dir %q, want %q", plugin.dir, filepath.Dir(ts))
	}
	if err := plugin.install(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ts); !strings.Contains(got, `const AGENT = "omp"`) || !strings.Contains(got, `["hook", AGENT]`) {
		t.Errorf("no OMP hook in:\n%s", got)
	}
	if plugin.state() != "current" {
		t.Errorf("state %s", plugin.state())
	}
	if dir := skillDirs("omp")["agents"]; dir != filepath.Join(home, ".agents", "skills", "kero") {
		t.Errorf("skill %q", dir)
	}
}

// CodeBuddy Code's hook goes in the settings file of its own config
// root, CODEBUDDY_CONFIG_DIR (default ~/.codebuddy), as Claude Code's
// does, and its skill is left out when CodeBuddy Code is not installed.
func TestCodeBuddyHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KERO_HOME", filepath.Join(home, "kero"))
	t.Setenv("CODEBUDDY_CONFIG_DIR", filepath.Join(home, ".codebuddy"))
	settings := filepath.Join(home, ".codebuddy", "settings.json")
	os.MkdirAll(filepath.Dir(settings), 0o755)

	var hook agentHook
	for _, it := range agentHooks("codebuddy") {
		if h, ok := it.(agentHook); ok && h.agent == "codebuddy" {
			hook = h
		}
	}
	if hook.file != settings {
		t.Fatalf("file %q, want %q", hook.file, settings)
	}
	if err := hook.install(); err != nil {
		t.Fatal(err)
	}
	got := read(t, settings)
	if !strings.Contains(got, "hook codebuddy") {
		t.Errorf("no CodeBuddy Code hook in:\n%s", got)
	}
	if hook.state() != "current" {
		t.Errorf("state %s", hook.state())
	}
	if dir := skillDirs("codebuddy")["codebuddy"]; dir != filepath.Join(home, ".codebuddy", "skills", "kero") {
		t.Errorf("skill %q", dir)
	}
	// Its skill goes nowhere when CodeBuddy Code is not installed.
	if _, ok := skillDirs("")["codebuddy"]; !ok {
		t.Error("no skill for CodeBuddy Code though it is installed")
	}
	os.RemoveAll(filepath.Join(home, ".codebuddy"))
	if _, ok := skillDirs("")["codebuddy"]; ok {
		t.Error("a skill dir for CodeBuddy Code it is not installed")
	}
}

// PiG's extension goes under its own config root, PIG_HOME (default
// ~/.pig), and its hook names PiG, so Kero resumes it with `pig --session`.
func TestPiGHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KERO_HOME", filepath.Join(home, "kero"))
	t.Setenv("PIG_HOME", filepath.Join(home, "pig"))
	ts := filepath.Join(home, "pig", "agent", "extensions", "kero.ts")
	os.MkdirAll(filepath.Dir(ts), 0o755)
	var plugin agentPlugin
	for _, it := range agentHooks("pig") {
		if p, ok := it.(agentPlugin); ok && p.agent == "pig" {
			plugin = p
		}
	}
	if plugin.dir != filepath.Dir(ts) {
		t.Fatalf("dir %q, want %q", plugin.dir, filepath.Dir(ts))
	}
	if err := plugin.install(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ts); !strings.Contains(got, `const AGENT = "pig"`) || !strings.Contains(got, `["hook", AGENT]`) {
		t.Errorf("no PiG hook in:\n%s", got)
	}
	if plugin.state() != "current" {
		t.Errorf("state %s", plugin.state())
	}
	if dir := skillDirs("pig")["agents"]; dir != filepath.Join(home, ".agents", "skills", "kero") {
		t.Errorf("skill %q", dir)
	}
	// PiG reads neither Pi's directory nor Pi's environment.
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "pi", "agent"))
	if dir := agentHooks("pig")[0].(agentPlugin).dir; dir != filepath.Dir(ts) {
		t.Errorf("dir %q, want %q", dir, filepath.Dir(ts))
	}
}

// The skill is SKILL.md and the reference files beside it: setup installs
// each, SyncSkill adds those an older install lacks, and both remove what
// kero installed that it has no more, but never the user's own files.
func TestSkillFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KERO_HOME", filepath.Join(home, "kero"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	claude := skillDirs("claude")["claude"]
	if _, ok := skillFiles["reference/orchestrating.md"]; !ok || len(skillFiles) < 5 {
		t.Fatalf("skill files: %v", sortedKeys(skillFiles))
	}

	// An older kero's install: SKILL.md alone, linked.
	os.MkdirAll(claude, 0o755)
	if err := syncSkillFiles(); err != nil {
		t.Fatal(err)
	}
	if os.Symlink(skillFile(), filepath.Join(claude, "SKILL.md")) != nil {
		os.WriteFile(filepath.Join(claude, "SKILL.md"), []byte(skillGuide), 0o644)
	}
	if skillState(claude) != "outdated" {
		t.Errorf("state %s, with no reference files", skillState(claude))
	}
	if err := writeSkill(claude); err != nil { // kero skill setup's part
		t.Fatal(err)
	}
	for rel, text := range skillFiles {
		if b, err := os.ReadFile(filepath.Join(claude, filepath.FromSlash(rel))); err != nil || string(b) != text {
			t.Errorf("%s: %v", rel, err)
		}
	}
	if skillState(claude) != "current" {
		t.Errorf("state %s", skillState(claude))
	}

	// What a later kero drops: Kero's own copy, a link to it and a copy of
	// a reference file go; the user's notes, in the skill or beside the
	// reference files, stay.
	own := filepath.Join(skillRoot(), "reference", "dropped.md")
	os.WriteFile(own, []byte(skillMark+" -->\n"), 0o644)
	linked := filepath.Join(claude, "reference", "dropped.md")
	if os.Symlink(own, linked) != nil {
		os.WriteFile(linked, []byte(skillMark+" -->\n"), 0o644)
	}
	copied := filepath.Join(claude, "reference", "renamed.md")
	os.WriteFile(copied, []byte(skillMark+" -->\n# Old\n"), 0o644)
	mine := []string{filepath.Join(claude, "NOTES.md"), filepath.Join(claude, "reference", "mine.md")}
	for _, p := range mine {
		os.WriteFile(p, []byte("# My notes\n"), 0o644)
	}
	if skillState(claude) != "outdated" {
		t.Errorf("state %s, with files kero dropped", skillState(claude))
	}
	SyncSkill()
	for _, p := range []string{own, linked, copied} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s stays", p)
		}
	}
	for _, p := range mine {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("the user's %s: %v", p, err)
		}
	}
	if skillState(claude) != "current" {
		t.Errorf("state %s, the user's notes there", skillState(claude))
	}

	// Removing it takes kero's files alone.
	if err := removeSkill(claude); err != nil {
		t.Fatal(err)
	}
	for rel := range skillFiles {
		if _, err := os.Lstat(filepath.Join(claude, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s stays", rel)
		}
	}
	for _, p := range mine {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("removing took the user's %s", p)
		}
	}
}

// SKILL.md is a directory of the reference files: it names every one, and
// every file that it, the files, the role prompt and the hook name is
// there, for kero skill show to print too.
func TestSkillReferences(t *testing.T) {
	name := regexp.MustCompile(`reference/([a-z-]+)\.md`)
	texts := map[string]string{"role prompt": orchestrator.DefaultPrompt, "hook": hookOrchestrator + hookChild + hookAssistant,
		"assistant's prompt": assistant.Prompt(proto.Assistant{Name: "A", Role: "B"})}
	for rel, text := range skillFiles {
		texts[rel] = text
	}
	for from, text := range texts {
		for _, m := range name.FindAllStringSubmatch(text, -1) {
			if _, ok := skillFiles[m[0]]; !ok {
				t.Errorf("%s names %s, which isn't there", from, m[0])
			}
			if got, ok := skillShow(m[1]); !ok || got != skillFiles[m[0]] {
				t.Errorf("kero skill show %s", m[1])
			}
		}
	}
	for rel, text := range skillFiles {
		if rel == "SKILL.md" {
			continue
		}
		if !strings.Contains(skillGuide, "- "+rel+":") {
			t.Errorf("SKILL.md doesn't list %s", rel)
		}
		if !strings.HasPrefix(text, skillMark) {
			t.Errorf("%s doesn't start with the mark", rel)
		}
	}
	if _, ok := skillShow("nothing"); ok {
		t.Error("kero skill show nothing")
	}
	// A description of triggers, not of every feature.
	desc, _, _ := strings.Cut(strings.SplitN(skillGuide, "description: ", 2)[1], "\n")
	if len(desc) > 600 || !strings.Contains(desc, "Use when") {
		t.Errorf("description of %d bytes: %s", len(desc), desc)
	}
	for _, want := range []string{"Result", "Not done", "Where", "Next", "Waiting on you:", "kero report working"} {
		if !strings.Contains(skillFiles["reference/reporting.md"], want) || !strings.Contains(hookChild, want) {
			t.Errorf("the report's %q is missing", want)
		}
	}
}
