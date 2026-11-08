---
title: Coding Agents
description: Teach Claude Code, Codex and other agents to run work beside themselves and report their progress in Kero.
---

# Coding Agents

Agents running in Kero can use the `kero` command like you do: start a dev server in a pane below them, hand a review to a second agent, wait for tests, and tell you when they're blocked.

## Install the skill

```sh
kero skill setup
```

This installs two things:

- A **skill** that teaches agents the CLI. Claude Code reads it from `~/.claude/skills/kero`; Codex, OpenCode and other agents from `~/.agents/skills/kero`.
- A **hook** that runs as an agent starts a session. It tells the agent which Kero session and pane it's in, so it doesn't spend time finding out, and has it name an unnamed session after its task, such as `fix-login-redirect`, so you don't have to. It also tells Kero the agent's own session, so that after kerod restarts, as after a reboot, the agent [picks up its conversation](./sessions.md#kerod). The same hook runs as the agent ends each turn, and tells Kero the turn's end and its reply, so that an [orchestrator](./orchestrators.md) hears of a child's turn when it ends, not when its screen looks idle. It prints nothing there, and runs in the background, so it never holds the agent up.

The hook goes where each agent looks for one, for the agents you have installed:

| Agent       | Where the hook goes                                                          |
| ----------- | ---------------------------------------------------------------------------- |
| Claude Code | `SessionStart`, `Stop` and `StopFailure` hooks in `~/.claude/settings.json`  |
| Codex       | `SessionStart`, `Stop` and `Interrupt` hooks in `~/.codex/hooks.json`        |
| Pi          | An extension, `~/.pi/agent/extensions/kero.ts`         |
| OMP         | An extension, `~/.omp/agent/extensions/kero.ts`        |
| OpenCode    | A plugin, `~/.config/opencode/plugins/kero`            |

Kero follows `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `PI_CODING_AGENT_DIR`, `PI_CONFIG_DIR` and `XDG_CONFIG_HOME` when they move an agent's files, and leaves the rest of each settings file as it was.

Pass `--agent` with `claude`, `codex`, `pi`, `omp` or `opencode` to set up one agent only. Setup is once: the skill is a link to Kero's own copy, which follows Kero as it updates, and Kero keeps the Pi, OMP and OpenCode plugins current too. `kero skill status` tells you whether each part is current. A Claude Code or Codex hook that an older Kero put in shows as outdated, as it has no turn-end hooks yet: run `kero skill setup` again to add them.

```sh
kero skill status    # what's installed, and whether it's current
kero skill show      # print the skill
kero skill remove    # uninstall the skill and the hooks
```

The hook works only in agent sessions started after setup, and agents may ask first:

- **Claude Code** runs hooks only in folders you trust.
- **Codex** asks you to approve a new hook: approve Kero's with `/hooks`.
- **OpenCode** loads plugins as its background service starts: run `opencode service restart` once, when no session is busy.

OpenCode runs an agent's commands in its background service rather than in the pane, so they don't have `KERO_SESSION` and `KERO_PANE`. Kero tells the agent its pane instead, for it to name as the target. For the same reason, `kero report` from an OpenCode agent doesn't reach its pane.

Outside Kero, the hook does nothing.

## What agents can do

Run something beside themselves:

```sh
kero split --down -- npm run dev
kero new -n review -- claude
```

Drive it and read the result:

```sh
kero prompt review "review the changes in src/auth"
kero wait tests --for idle,error --timeout 10m
kero read tests -n 60
```

Run tests beside themselves and read what failed:

```sh
pane=$(kero split --down -- go test ./...)
kero wait "$pane" --exit || kero read "$pane" -n 80
```

See the repository's worktrees, with what `git worktree list` doesn't tell: the sessions in each, which Kero made, what removing one would lose and what the automatic cleanup will do with it; check their own session's worktree, setup and pull request; and move their own session, with its conversation, into a new git worktree, to go on there once their turn ends ([more](./worktrees.md#moving-a-session-into-a-worktree)):

```sh
kero worktree list
kero worktree status
kero worktree handoff fix-login --prompt "go on with the fix"
```

Work with pull requests: write a [review guide](./pull-requests.md#review-guides), see the session's linked pull request, which Kero watches for them, typing a message starting "Kero PR watch:" into their prompt when its checks fail, and merge, update or check one out when you ask them to ([all of it](./cli.md)):

```sh
kero pr guide stop src/queue.go:10-42 --title "The new queue" "Why it exists."
kero pr status                  # the linked pull request: checks, conflicts
kero pr update-branch --rebase
kero pr merge --squash          # only when you ask: a merge can't be undone
kero pr checkout 123 --worktree -- claude
```

Tell you what's going on:

```sh
kero report working "Migrating the database" --progress 40
kero report blocked "Approve the production deploy?"
kero report done "Deployed to staging"
```

A `blocked` report turns the session to *waiting* and notifies you, so agents should use it only when they can't continue without you.

## Agents that run other agents

- An [orchestrator](./orchestrators.md) is an agent session that plans the work, starts child sessions for it and follows them, while you talk to it alone.
- [Ama](./ama.md) is Kero's helper agent, which starts and manages your sessions, worktrees, projects and settings for you, and answers questions about Kero.

## Remote Control

With **Remote Control** on (the default, in **Settings › Agents**), agents can also drive the app itself: show a session with `kero focus`, run menu actions with `kero do`, drive browser panes with `kero browser`, and start pull request reviews with `kero pr review`. Turn it off with `remote-control = false` to keep agents to their own sessions. [Ama](./ama.md#without-remote-control) doesn't need it.

## Environment

Every pane gets:

| Variable       | Value                          |
| -------------- | ------------------------------ |
| `KERO_SESSION` | The id of the pane's session   |
| `KERO_PANE`    | The id of the pane             |
| `KERO_SOCKET`  | Where kerod listens, for `kero` to reach it |
| `TERM`, `COLORTERM` | `xterm-256color` and `truecolor` |
| `TERM_PROGRAM`, `TERM_PROGRAM_VERSION` | `ghostty` and Ghostty's version, as Kero's terminal is Ghostty's, so programs that know Ghostty's keys and notifications use them |
| `LC_TERMINAL`, `LC_TERMINAL_VERSION` | `kero`, and its protocol's version |
| `SHELL`        | The pane's shell; on Windows, only when it is a POSIX shell, as bash |
| `LANG`         | `en_US.UTF-8`, when kerod had none of `LANG`, `LC_ALL` and `LC_CTYPE` |
| `PATH`         | kerod's, with Kero's own `bin` folder, which has `kero`, at its end |

The `env` setting's variables come after these, and win. A pane gets the rest of the environment kerod started with, the app's, but for what belongs to another terminal or agent, such as `TMUX`, `VSCODE_*`, `GHOSTTY_*`, `CLAUDECODE` and `CLAUDE_CODE_*`. So start Kero from a shell without `NO_COLOR` in it, or every pane loses its colors ([Troubleshooting](./troubleshooting.md#panes-without-colors)). [Ama's](./ama.md) panes also get `KERO_AMA`, and a worktree's [setup script](./worktrees.md#setup-scripts) gets `KERO_WORKTREE_PATH`, `KERO_PROJECT_ROOT` and `KERO_WORKTREE_BRANCH`.

Check `$KERO_PANE` to tell whether a script runs inside Kero.
