package cli

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kero/internal/proto"
)

// skillFS is the skill `kero skill setup` installs, from this binary, so
// that it follows kero's updates: SKILL.md, a directory of the reference
// files beside it, which agents read as a task needs one.
//
//go:embed skill/SKILL.md skill/reference/*.md
var skillFS embed.FS

// skillGuide is the skill's SKILL.md.
var skillGuide = skillText("SKILL.md")

// skillFiles are the skill's files, by their paths in its directory, with
// / between their parts.
var skillFiles = func() map[string]string {
	out := map[string]string{}
	fs.WalkDir(skillFS, "skill", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out[strings.TrimPrefix(p, "skill/")] = skillText(strings.TrimPrefix(p, "skill/"))
		}
		return err
	})
	return out
}()

func skillText(name string) string {
	b, err := skillFS.ReadFile("skill/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// skillMark starts each reference file, so that kero tells a copy of one
// it wrote, which a later kero may have dropped, from the user's own files.
const skillMark = "<!-- Part of the kero skill"

// skillShow is the file of the skill that `kero skill show <name>`
// prints: SKILL.md, or a reference file by its name, with or without
// reference/ and .md.
func skillShow(name string) (string, bool) {
	if name == "" {
		return skillGuide, true
	}
	name = strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(name), "reference/"), ".md")
	if strings.EqualFold(name, "skill") {
		return skillGuide, true
	}
	text, ok := skillFiles["reference/"+name+".md"]
	return text, ok
}

func skillNames() []string {
	var names []string
	for _, p := range sortedKeys(skillFiles) {
		if n, ok := strings.CutPrefix(p, "reference/"); ok {
			names = append(names, strings.TrimSuffix(n, ".md"))
		}
	}
	return names
}

func skillCmd(args []string) error {
	f, err := parse(args, map[string]string{"agent": "agent"}, map[string]string{"dry-run": "dry", "n": "dry"})
	if err != nil {
		return err
	}
	sub := "status"
	if len(f.pos) > 0 {
		sub = f.pos[0]
	}
	agent := strings.ToLower(f.vals["agent"])
	dirs, hooks := skillDirs(agent), hooksFor(agent)
	switch sub {
	case "show":
		name := ""
		if len(f.pos) > 1 {
			name = f.pos[1]
		}
		text, ok := skillShow(name)
		if !ok {
			return usageErr("kero skill show [" + strings.Join(skillNames(), "|") + "]")
		}
		fmt.Print(text)
		return nil
	case "status":
		for _, name := range sortedKeys(dirs) {
			fmt.Printf("%-8s skill  %-9s %s\n", name, skillState(dirs[name]), proto.ShortDir(dirs[name]))
		}
		for _, h := range hooks {
			fmt.Printf("%-8s %-6s %-9s %s\n", h.name(), h.kind(), h.state(), proto.ShortDir(h.path()))
		}
		return nil
	case "setup", "remove":
		for _, name := range sortedKeys(dirs) {
			dir := dirs[name]
			switch {
			case f.set["dry"]:
				fmt.Printf("would %s %s (%s)\n", sub, proto.ShortDir(dir), name)
			case sub == "remove":
				if err := removeSkill(dir); err != nil {
					return err
				}
				fmt.Printf("removed %s\n", proto.ShortDir(dir))
			default:
				if err := writeSkill(dir); err != nil {
					return err
				}
				fmt.Printf("installed %s (%s)\n", proto.ShortDir(dir), name)
			}
		}
		for _, h := range hooks {
			switch {
			case f.set["dry"]:
				fmt.Printf("would %s the %s in %s (%s)\n", sub, h.kind(), proto.ShortDir(h.path()), h.name())
			case sub == "remove":
				if err := h.remove(); err != nil {
					return err
				}
				fmt.Printf("removed the %s from %s\n", h.kind(), proto.ShortDir(h.path()))
			default:
				if err := h.install(); err != nil {
					return err
				}
				fmt.Printf("installed the %s in %s (%s)\n", h.kind(), proto.ShortDir(h.path()), h.name())
			}
		}
		if sub == "setup" && !f.set["dry"] && len(hooks) > 0 {
			fmt.Println("\nAgents started from now on in Kero learn which session they're in.")
			// Once each: Claude Code's hooks and its plugin share theirs.
			notes := map[string]string{
				"claude":   "Claude Code runs hooks only in folders you trust.",
				"codex":    "Codex asks you to approve a new hook: run /hooks in it.",
				"opencode": "OpenCode's background service loads plugins as it starts: run `opencode service restart` when no session is busy.",
			}
			for _, h := range hooks {
				if note := notes[h.name()]; note != "" {
					fmt.Println(note)
					delete(notes, h.name())
				}
			}
		}
		return nil
	}
	return usageErr("kero skill setup|show [<file>]|status|remove [--agent claude|codebuddy|codex|pi|omp|pig|opencode|agents] [--dry-run]")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func skillState(dir string) string {
	if _, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err != nil {
		return "missing"
	}
	for rel, text := range skillFiles {
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))); err != nil || string(b) != text {
			return "outdated"
		}
	}
	if len(staleSkillFiles(dir)) > 0 {
		return "outdated"
	}
	return "current"
}

// skillRoot is Kero's own copy of the skill, in Kero's directory, which
// each kero rewrites as it runs (SyncSkill): the agents' files link to
// it, so they read the skill of the Kero installed, as it updates.
func skillRoot() string { return filepath.Join(proto.Dir(), "skills", "kero") }

func skillFile() string { return filepath.Join(skillRoot(), "SKILL.md") }

// syncSkillFiles writes Kero's own copy of the skill, where it isn't
// current, and removes the files an older kero wrote there that this one
// has no more: that directory is Kero's alone.
func syncSkillFiles() error {
	root := skillRoot()
	for rel, text := range skillFiles {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if b, err := os.ReadFile(p); err == nil && string(b) == text {
			continue
		}
		if err := writeFileAtomic(p, []byte(text)); err != nil {
			return err
		}
	}
	for _, p := range skillExtras(root) {
		os.Remove(p)
	}
	os.Remove(filepath.Join(root, "reference")) // once empty
	return nil
}

// skillExtras lists the files in a skill's directory that aren't the
// skill's, but for the ones being written (.name.*, writeFileAtomic's).
func skillExtras(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if rel, err := filepath.Rel(dir, p); err == nil {
			if _, ok := skillFiles[filepath.ToSlash(rel)]; !ok {
				out = append(out, p)
			}
		}
		return nil
	})
	return out
}

// staleSkillFiles lists what kero installed in an agent's skill
// directory that this kero no longer has: links to Kero's own copy, and
// copies of a reference file, which start with skillMark. The user's own
// files there stay.
func staleSkillFiles(dir string) []string {
	var out []string
	for _, p := range skillExtras(dir) {
		if t, err := os.Readlink(p); err == nil {
			if rel, err := filepath.Rel(skillRoot(), t); err == nil && filepath.IsLocal(rel) {
				out = append(out, p)
			}
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		head := make([]byte, len(skillMark))
		_, err = io.ReadFull(f, head)
		f.Close()
		if err == nil && string(head) == skillMark {
			out = append(out, p)
		}
	}
	return out
}

// writeSkill installs the skill in an agent's directory: each of its
// files a link to Kero's own copy. The directories are real ones, as
// agents list theirs and may pass over links to directories.
func writeSkill(dir string) error {
	if err := syncSkillFiles(); err != nil {
		return err
	}
	return linkSkill(dir)
}

// linkSkill links an agent's skill directory to Kero's own copy, which
// is current, and removes the files kero put there that it has no more.
func linkSkill(dir string) error {
	for rel, text := range skillFiles {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		target := filepath.Join(skillRoot(), filepath.FromSlash(rel))
		if t, err := os.Readlink(p); err == nil && t == target {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		os.Remove(p) // a copy, as older keros wrote, or a link to another's
		if os.Symlink(target, p) == nil {
			continue
		}
		// Where links need a right the user lacks (Windows, out of
		// Developer Mode): a copy, which SyncSkill keeps current.
		if err := writeFileAtomic(p, []byte(text)); err != nil {
			return err
		}
	}
	for _, p := range staleSkillFiles(dir) {
		os.Remove(p)
	}
	return nil
}

// removeSkill removes the skill from an agent's directory: its files,
// and those an older kero put there, but none of the user's, and the
// directory once it is empty.
func removeSkill(dir string) error {
	for _, p := range staleSkillFiles(dir) {
		os.Remove(p)
	}
	for rel := range skillFiles {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	os.Remove(filepath.Join(dir, "reference")) // once empty
	os.Remove(dir)
	return nil
}

// SyncSkill keeps what agents load of Kero current with this kero, which
// every kero, the app and the CLI, does as it starts. It updates what is
// this Kero's alone, so that a development build, with a directory of its
// own, takes nothing from the installed Kero: its own copy of the skill,
// and the links of the installs to it, which it adds as the skill gains
// files and removes as it drops them; and the plugins of Pi, OMP, PiG and
// OpenCode written to run its kero command. The installs older keros
// wrote, copies of the skill, go to the installed Kero, which makes them
// links. It installs nothing that isn't.
func SyncSkill() {
	synced := false
	for _, dir := range skillDirs("") {
		p := filepath.Join(dir, "SKILL.md")
		if t, err := os.Readlink(p); err == nil {
			if t == skillFile() {
				if !synced {
					synced = syncSkillFiles() == nil
				}
				linkSkill(dir)
			}
			continue // or another Kero's
		}
		if _, err := os.Stat(p); err == nil && adoptCopies() && skillState(dir) != "current" {
			writeSkill(dir)
		}
	}
	for _, it := range agentHooks("") {
		switch p := it.(type) {
		case agentPlugin:
			if p.ours() && p.state() == "outdated" {
				p.install()
			}
		case claudePlugin:
			p.sync()
		}
	}
}

// adoptCopies tells the installed Kero, which takes the skill's copies,
// from a development build or one of another $KERO_HOME.
var adoptCopies = func() bool { return proto.Name() == "kero" && os.Getenv("KERO_HOME") == "" }

// hooksFor returns the hooks that go with an --agent: Claude Code's and
// CodeBuddy Code's, and Codex's, Pi's, OMP's, PiG's and OpenCode's with
// the skill they share.
func hooksFor(agent string) []integration {
	switch agent {
	case "", "claude", "codebuddy", "codex", "pi", "omp", "pig", "opencode", "agents":
		return agentHooks(agent)
	}
	return nil
}

// skillDirs returns where agents read skills: Claude Code's, CodeBuddy
// Code's, and the shared one of Codex, OpenCode and others. CodeBuddy
// Code's is left out when CodeBuddy Code is not installed, so that Kero
// makes no directory of its own for it.
func skillDirs(agent string) map[string]string {
	home := proto.Home()
	claude := os.Getenv("CLAUDE_CONFIG_DIR")
	if claude == "" {
		claude = filepath.Join(home, ".claude")
	}
	codebuddy := os.Getenv("CODEBUDDY_CONFIG_DIR")
	if codebuddy == "" {
		codebuddy = filepath.Join(home, ".codebuddy")
	}
	all := map[string]string{
		"claude":    filepath.Join(claude, "skills", "kero"),
		"codebuddy": filepath.Join(codebuddy, "skills", "kero"),
		"agents":    filepath.Join(home, ".agents", "skills", "kero"),
	}
	if agent = strings.ToLower(agent); agent != "" {
		if d, ok := all[agent]; ok {
			return map[string]string{agent: d}
		}
		if agent == "codex" || agent == "opencode" || agent == "pi" || agent == "omp" || agent == "pig" {
			return map[string]string{"agents": all["agents"]}
		}
	}
	if st, err := os.Stat(codebuddy); err != nil || !st.IsDir() {
		delete(all, "codebuddy")
	}
	return all
}

// SkillState is where a skill for agents is, and whether it is current,
// with the hooks that go with it.
type SkillState struct {
	Agent string // "claude", "codebuddy", or "agents" (Codex, OpenCode, Pi…)
	Dir   string
	State string // "current", "outdated" (or partly installed) or "missing"
}

// SkillStates returns the state of kero's skill and hooks for each kind
// of agent.
func SkillStates() []SkillState {
	var out []SkillState
	for _, agent := range []string{"claude", "codebuddy", "agents"} {
		dir := skillDirs("")[agent]
		if dir == "" {
			continue // not installed
		}
		states := []string{skillState(dir)}
		for _, h := range hooksFor(agent) {
			states = append(states, h.state())
		}
		st := "outdated"
		switch {
		case !slices.ContainsFunc(states, func(s string) bool { return s != "current" }):
			st = "current"
		case !slices.ContainsFunc(states, func(s string) bool { return s != "missing" }):
			st = "missing"
		}
		out = append(out, SkillState{Agent: agent, Dir: dir, State: st})
	}
	return out
}

// InstallSkill installs, or updates, kero's skill and hooks for a kind of
// agent.
func InstallSkill(agent string) error {
	dir := skillDirs(agent)[agent]
	if dir == "" {
		return fmt.Errorf("no agent %q", agent)
	}
	if err := writeSkill(dir); err != nil {
		return err
	}
	for _, h := range hooksFor(agent) {
		if err := h.install(); err != nil {
			return err
		}
	}
	return nil
}

// RemoveSkill removes kero's skill and hooks for a kind of agent.
func RemoveSkill(agent string) error {
	dir := skillDirs(agent)[agent]
	if dir == "" {
		return fmt.Errorf("no agent %q", agent)
	}
	for _, h := range hooksFor(agent) {
		if err := h.remove(); err != nil {
			return err
		}
	}
	return removeSkill(dir)
}
