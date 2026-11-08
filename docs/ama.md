---
title: Ama
description: Kero's helper agent, which starts and manages your sessions, worktrees, projects and settings, and answers questions about Kero.
---

# Ama

Ama is an agent that runs Kero for you. Its name comes from *amagaeru*, the Japanese tree frog. Click the frog at the bottom of the sessions sidebar, or press **Ctrl+Shift+A** (**⇧⌘A** on macOS), or choose **View › Ama**, and ask it things such as:

- "Start a code review for every open PR in project X."
- "Clean up all old worktrees."
- "Add a new project for this folder and start an orchestrator there with this goal."
- "Which sessions need me?"
- "Close every finished session in project Y."
- "How do I run a setup script in new worktrees?"

Ama is an ordinary agent session in a real terminal. It runs your default agent, or the one you pick in **Settings › Agents › Ama** (`ama-agent`). It doesn't do project work itself: for real work it starts a session, or an [orchestrator](./orchestrators.md), in the right project. It belongs to no project, and there is one Ama. The sidebar doesn't list it, Needs You doesn't count it, and it sends no notifications. The frog shows a dot while Ama waits for you.

Ama manages Kero with the commands in [Managing Kero](./cli.md#managing-kero), and asks before what can't be undone, such as closing sessions that work or removing a worktree with changes.

## Its window

Ama opens in a window of its own, above Kero's. Drag its header to move it, even to another display, and drag its edges to resize it. Kero remembers its size and its place relative to Kero's window: the window follows Kero's as you move or resize it, and stays on a display when Kero's window nears a screen edge or a display goes away. Kero stores the size and place in DIPs, so the window keeps its size across displays with different scaling. Ama keeps running when you close its window, and the next time you open it you're back in the same conversation. In its header:

- **New Chat** starts a fresh conversation.
- **History** goes back to one of Ama's earlier conversations.
- **Clear** clears the screen.

To put the window back above the frog, at its first size, right-click the frog and choose **Reset Window Position**, or click **Reset** in **Settings › Agents › Ama**.

Ama's text is a point smaller than the terminals' (**Text size** in **Settings › Agents › Ama**, `ama-font-size`; `0` is automatic). Claude Code's status line is hidden in Ama's window (`ama-hide-status-line`, on by default). Kero puts an empty status line in `.claude/settings.json` in Ama's folder and leaves your own Claude Code settings alone. Claude Code has no setting that removes the status line entirely without also turning off hooks, so one blank row stays where it was. If that file already has a status line of your own, Kero leaves it, and `.claude/settings.local.json` beside it overrides both.

## Its folder

Ama runs in a folder of its own, `ama` beside Kero's configuration file (`~/.config/kero/ama`, or as `kero config path` says). Kero writes it before Ama starts:

| File | What it is |
| --- | --- |
| `CLAUDE.md`, `AGENTS.md` | Ama's instructions: its role prompt and the list of Kero's docs. Kero rewrites them as it updates, and as you change the role prompt |
| `USER.md` | Your own instructions for Ama, which Kero makes once and never changes |
| `docs/` | Kero's docs, these pages, as this version of Kero has them. Kero rewrites them as it updates |
| `.claude/settings.json` | The empty status line, while `ama-hide-status-line` is on |

## What Ama knows

Ama learns Kero from three places:

- **Its role prompt**, in `CLAUDE.md` and `AGENTS.md`: what it is for, the commands it runs, and its rules.
- **Kero's docs**, in its folder's `docs/`, the pages of this site as your version of Kero has them, in English. Its instructions list each page with what it covers, and have Ama read the page a question is about before it answers, rather than guess or search Kero's program.
- **Kero's skill**, which Kero installs for Ama's agent before Ama starts, as `kero skill setup` does, and `kero help`: every command and option.

So Ama answers questions about Kero, such as how a setting works or where a button is, from the docs of the version you run.

## Your instructions

Kero writes Ama's instructions into its folder as `CLAUDE.md` and `AGENTS.md`, and rewrites them as Kero updates. They are Kero's role prompt for Ama, unless you replace it in **Settings › Agents › Ama › Role prompt** (`ama-prompt`); **Reset to Default** brings Kero's back. Ama's agent reads the prompt from its next chat. A prompt of your own replaces Kero's role prompt alone: the list of the docs still follows it.

Your own instructions, on top of the role prompt, go in `USER.md` beside them, such as which agent to start for reviews, the projects you mean by short names, or what to ask you before closing. Kero creates that file once and never changes it, and Ama reads it first. **Settings › Agents › Ama** opens it.

**Arguments** (`ama-agent-args`) go after Ama's agent command, such as `--model sonnet`, so you needn't add another agent for Ama: Kero adds them when Ama starts and when it resumes.

## Without Remote Control

Ama doesn't need [Remote Control](./agents.md#remote-control): kerod gives Ama's panes a secret, `$KERO_AMA`, which is a hash of Ama's session with a key that kerod makes anew each time it starts. `kero do`, `kero focus` and `kero browser` pass it on, and the app takes the action without Remote Control only when kerod confirms the secret is Ama's. No file holds it, other panes never get it, and it means nothing to another machine's kerod, so Remote Control still decides for every other agent.
