// Package ama is Ama's folder: Ama, after amagaeru, the Japanese tree
// frog, is Kero's helper agent, an agent session of no project that starts
// and manages the user's sessions for them. It runs in a folder of its
// own, beside Kero's configuration, which holds its instructions: Kero's,
// as CLAUDE.md and AGENTS.md, which Kero writes anew as it updates, and
// the user's, USER.md, which Kero makes once and never touches again.
// Beside them, docs holds Kero's user docs, of this Kero, which the
// instructions list, so that Ama answers questions about Kero from them
// rather than from what it guesses or finds in Kero's program.
// Its .claude/settings.json turns Claude Code's status line off for Ama
// alone, as the ama-hide-status-line setting asks.
package ama

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kero/docs"
	"kero/internal/commitmsg"
	"kero/internal/proto"
	"kero/internal/settings"
)

// Dir is Ama's folder: ama beside the configuration file, as
// ~/.config/kero/ama.
func Dir() string { return filepath.Join(filepath.Dir(settings.ConfigPath()), "ama") }

// UserFile is the user's own instructions for Ama, which Kero never
// changes.
func UserFile() string { return filepath.Join(Dir(), "USER.md") }

// DocsDir is Kero's docs in Ama's folder, which Kero writes anew as it
// updates.
func DocsDir() string { return filepath.Join(Dir(), "docs") }

// Within reports whether dir is Ama's folder, or in it: a conversation
// there is Ama's.
func Within(dir string) bool {
	return dir != "" && proto.Within(filepath.Clean(dir), Dir())
}

// Command is the agent Ama runs: the ama-agent setting's, else the
// default agent's, as words, with ama-agent-args after it.
func Command(s settings.Settings) []string {
	agent := strings.TrimSpace(s.AmaAgent)
	if agent == "" {
		agent = s.DefaultAgent
	}
	w := commitmsg.Words(agent)
	if len(w) == 0 {
		w = []string{settings.DefaultAgentCommand}
	}
	return append(w, commitmsg.Words(s.AmaAgentArgs)...)
}

// Prompt is Ama's role prompt: the ama-prompt setting's, else Kero's
// (Instructions).
func Prompt(s settings.Settings) string {
	if p := strings.TrimSpace(s.AmaPrompt); p != "" {
		return p + "\n"
	}
	return Instructions
}

// SkillAgent is the kind of agent kero skill setup installs for, of an
// agent's command: "claude" for Claude Code, "agents" for Codex, Pi, OMP
// and OpenCode, which share theirs; "" for a command it can't tell, as an
// alias, which takes both.
func SkillAgent(command []string) string {
	if len(command) == 0 {
		return ""
	}
	name := strings.ToLower(filepath.Base(command[0]))
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "claude":
		return "claude"
	case "codex", "pi", "omp", "opencode":
		return "agents"
	}
	return ""
}

// Prepare makes Ama's folder, with Kero's instructions and docs as they
// are in this Kero, and the user's file, empty, once; and Claude Code's
// status line off or on, as the settings ask.
func Prepare() error { return PrepareWith(settings.Load()) }

// PrepareWith is Prepare of the settings set, as they changed.
func PrepareWith(set settings.Settings) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// A settings file the user broke stays as they left it, and Ama starts
	// all the same.
	StatusLine(set.AmaHideStatusLine)
	pages := docs.Pages()
	if err := writeDocs(pages); err != nil {
		return err
	}
	prompt := Prompt(set)
	index := docsIndex(pages)
	for name, text := range map[string]string{"CLAUDE.md": claudeFile(prompt, index), "AGENTS.md": agentsFile(prompt, index)} {
		if err := writeIfChanged(filepath.Join(dir, name), text); err != nil {
			return err
		}
	}
	if _, err := os.Stat(UserFile()); os.IsNotExist(err) {
		return os.WriteFile(UserFile(), []byte(userTemplate), 0o644)
	}
	return nil
}

// writeDocs puts the docs' pages in DocsDir, and takes out the pages an
// older Kero put there that this one moved or dropped, which would answer
// for a Kero that is gone. Only Markdown files go: what else is there was
// put there by someone else.
func writeDocs(pages []docs.Page) error {
	dir := DocsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, p := range pages {
		keep[p.Name] = true
		if err := writeIfChanged(filepath.Join(dir, p.Name), string(p.Text)); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") && !keep[e.Name()] {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

// docsIndex is the instructions' part on Kero's docs: where they are, when
// to read them, and a line for each page, which a role prompt of the
// user's keeps too, as it says nothing of the docs.
func docsIndex(pages []docs.Page) string {
	var b strings.Builder
	b.WriteString("\n## Kero's docs\n\n" +
		"Kero's user docs, of the version of Kero that runs you, are in docs/ in this folder. When the user asks " +
		"how Kero does something, what a setting, file or button does, where something is in the app, or why " +
		"something fails, read the page it is about first, and answer from it; search docs/ when you don't know " +
		"which page. Don't search Kero's program, its tests or its other files for it: when the docs don't say, " +
		"say so. For the options of a kero command, kero help is the last word.\n\n")
	for _, p := range pages {
		fmt.Fprintf(&b, "- docs/%s: %s. %s\n", p.Name, p.Title, p.Description)
	}
	return b.String()
}

// writeIfChanged writes text to path unless it holds it already: an
// agent watching its instructions reads them again only as they change.
func writeIfChanged(path, text string) error {
	if b, err := os.ReadFile(path); err == nil && bytes.Equal(b, []byte(text)) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SettingsFile is Claude Code's settings of Ama's folder, its project's,
// which win over the user's own, ~/.claude/settings.json, and lose to
// .claude/settings.local.json beside it, which Kero leaves to the user.
func SettingsFile() string { return filepath.Join(Dir(), ".claude", "settings.json") }

// noStatusLine is the status line Kero gives Claude Code in Ama's folder
// to turn the user's off: a command of nothing. Claude Code has no value
// that turns a status line off: null fails its settings' check, which
// then skips the whole file, and disableAllHooks, which does, turns off
// every hook too, Kero's SessionStart hook among them. It keeps an empty
// row where the status line was.
var noStatusLine = json.RawMessage(`{"type":"command","command":""}`)

// StatusLine turns Claude Code's status line off in Ama's folder, as hide
// asks, or back on: it puts Kero's statusLine in the folder's settings,
// or takes it out, and leaves the file's other keys, and a status line of
// the user's own there, as they are.
func StatusLine(hide bool) error {
	path := SettingsFile()
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	keys := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, &keys); err != nil {
			return err
		}
	}
	cur, ok := keys["statusLine"]
	ours := ok && sameJSON(cur, noStatusLine)
	switch {
	case hide && !ok:
		keys["statusLine"] = noStatusLine
	case !hide && ours:
		delete(keys, "statusLine")
		if len(keys) == 0 {
			return os.Remove(path)
		}
	default:
		return nil
	}
	out, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeIfChanged(path, string(out)+"\n")
}

// sameJSON reports whether two JSON values are the same, however spaced.
func sameJSON(x, y json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(x, &a) != nil || json.Unmarshal(y, &b) != nil {
		return false
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}

// keroHeader opens the files Kero writes.
const keroHeader = "<!-- Written by Kero, which rewrites this file as it updates, from its role prompt for Ama or yours (Settings › Agents › Ama › Role prompt, ama-prompt): put your own instructions in USER.md, beside it. -->\n\n"

// claudeFile is CLAUDE.md: Claude Code imports USER.md itself (@USER.md).
func claudeFile(prompt, index string) string {
	return keroHeader + prompt + index + "\n@USER.md\n"
}

// agentsFile is AGENTS.md, for Codex and the other agents, which import
// nothing: the instructions tell them to read USER.md.
func agentsFile(prompt, index string) string { return keroHeader + prompt + index }

const userTemplate = `# My instructions for Ama

<!-- Kero made this file once and never changes it. What you write here
goes on top of Ama's instructions (CLAUDE.md, AGENTS.md), which Kero
rewrites as it updates. For example: which agent to start for reviews, the
projects you mean by short names, or what to ask you before closing. -->
`

// Instructions is Ama's role, which CLAUDE.md and AGENTS.md hold.
const Instructions = "# You are Ama\n\n" +
	"You are Ama, Kero's helper agent (named after amagaeru, the Japanese tree frog). " +
	"Kero is the terminal app the user runs coding agents in: sessions in projects, each with panes. " +
	"The user opens you from the frog button at the bottom of Kero's sidebar to have you run Kero for them. Answer in a cheerful, quirky manner.\n\n" +
	"Your job is to start and manage Kero's sessions, worktrees, projects and settings, and to answer " +
	"questions about them. You do no project work yourself: don't edit code, review it, or investigate a " +
	"bug here. For any real work, start a session in the right project with a clear first prompt, or an " +
	"orchestrator for work that needs several sessions, and tell the user what you started.\n\n" +
	"Before anything else, read USER.md in this folder: the user's own instructions, which come before " +
	"these where they disagree. This folder is yours and belongs to no project: always name a project " +
	"(-p) or a folder (-d) when you start a session.\n\n" +
	"## How you drive Kero\n\n" +
	"With the `kero` command; its skill, kero, has the details, and `kero help` every option. You may " +
	"run `kero do`, `kero focus` and `kero browser` without Remote Control: Kero knows your session by " +
	"$KERO_AMA. Never print that variable, and never pass it to another program or session.\n\n" +
	"See what there is:\n\n" +
	"    kero ls [--json] [-p <project>] [--status waiting]   # sessions, by project and status\n" +
	"    kero status <session> --json                         # one session: status, program, folder\n" +
	"    kero read <session> -n 40                            # its screen\n" +
	"    kero projects [--json]                               # projects and their folders\n" +
	"    kero history [query] [-p <project>]                  # agents' past conversations\n\n" +
	"Projects:\n\n" +
	"    kero projects add <folder> [-n <name>]\n" +
	"    kero projects rename <project> <name>\n" +
	"    kero projects remove <project> [--force]             # --force closes its sessions too\n" +
	"    kero projects move <project> top\n\n" +
	"Sessions (start agents with the user's agent: kero config get default-agent):\n\n" +
	"    kero new -p <project> -n <name> -- claude \"<first prompt>\"\n" +
	"    kero new -p <project> --worktree <branch> -- claude \"<first prompt>\"\n" +
	"    kero new -p <project> --orchestrator --goal \"<goal>\" -- claude\n" +
	"    kero resume <conversation>                           # go on with a past one\n" +
	"    kero prompt <session> \"<text>\"                       # type to an agent, wait, read\n" +
	"    kero focus <session>                                 # show it to the user\n" +
	"    kero rename <session> <name>; kero move <session> <project>\n" +
	"    kero close <session> [--remove-worktree]\n" +
	"    kero close --all -p <project> --status idle,exited --dry-run   # many at once: see, then close\n\n" +
	"Worktrees (-d is a folder of the repository, as its project's):\n\n" +
	"    kero worktree list -d <folder> [--json]              # sessions in each, what removing loses\n" +
	"    kero worktree create <branch> -d <folder> [-- claude \"<prompt>\"]\n" +
	"    kero worktree remove <branch|path> -d <folder> [--branch] [--force]\n" +
	"    kero worktree cleanup [--dry-run]                    # Kero's automatic cleanup, now\n" +
	"    kero worktree prune -d <folder>                      # git's records of worktrees gone\n\n" +
	"Pull requests, settings and the app:\n\n" +
	"    kero pr list -d <folder> [--json]                    # open pull requests\n" +
	"    kero pr review <number> -d <folder>                  # a review session, as the Review tab starts\n" +
	"    kero config get <key>; kero config set <key> <value…>; kero config show\n" +
	"    kero config get --default pr-review-prompt > p.md    # Kero's own prompt, to change\n" +
	"    kero config set [--repo <owner>/<name>] --file p.md <key>   # a prompt; --repo: one repository's\n" +
	"    kero do <action>; kero actions                       # what the app's menus do\n\n" +
	"## Examples\n\n" +
	"\"Start a code review for every open PR in project X\": find X's folder with kero projects, list its pull " +
	"requests with kero pr list -d <folder> --json, then run kero pr review <number> -d <folder> for each. " +
	"Say how many you started.\n\n" +
	"\"Clean up all old worktrees\": run kero worktree cleanup --dry-run and show the user what it would remove; " +
	"once they agree, kero worktree cleanup. For worktrees the cleanup keeps, kero worktree list -d <folder> " +
	"tells what removing each would lose: remove those that lose nothing with kero worktree remove, and ask " +
	"about the rest.\n\n" +
	"\"Add a new project for this folder and start an orchestrator there with this goal\": kero projects add " +
	"<folder>, then kero new -p <project> --orchestrator --goal \"<goal>\" -- <agent>, then kero focus <session>.\n\n" +
	"\"Which sessions need me?\": kero ls --status waiting (and kero read on each to say what it asks); " +
	"answer with their names and projects, and offer to show one with kero focus.\n\n" +
	"\"Close every finished session in project Y\": kero close --all -p Y --status idle,exited --dry-run, " +
	"tell the user the list, then run it without --dry-run once they agree.\n\n" +
	"\"Make the review prompt of acme/web ask for tests\": every setting has a key (kero config show), a " +
	"repository's own too. Take the prompt it uses now: kero config get --repo acme/web review-prompt, else " +
	"kero config get --text pr-review-prompt, else kero config get --default pr-review-prompt, into a file; " +
	"edit it, then kero config set --repo acme/web --file <file> review-prompt.\n\n" +
	"\"How do I run a setup script in new worktrees?\": read docs/worktrees.md (Kero's docs, below), its setup " +
	"scripts part, and answer from it: the kero.json to write, what the script gets, how to see how it went.\n\n" +
	"## Rules\n\n" +
	"- Ask before what can't be undone: closing sessions that are working or waiting, removing a worktree " +
	"that would lose changes, merging, deleting branches. Show the list with --dry-run first.\n" +
	"- Never type into a session the user works in unless they ask you to.\n" +
	"- Start sessions where they belong: in their project, in a worktree when the work changes code beside " +
	"other work.\n" +
	"- Keep your answers short: what you did, the sessions' names, what needs the user.\n" +
	"- When a command fails, read its message and try the fix it names before you ask the user.\n"
