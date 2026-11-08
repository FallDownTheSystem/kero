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
- A **hook** that runs as an agent starts a session. It tells the agent which Kero session and pane it's in, so it doesn't spend time finding out, and has it name an unnamed session after its task, such as `fix-login-redirect`, so you don't have to. It also tells Kero the agent's own session, so that after kerod restarts, as after a reboot, the agent [picks up its conversation](./sessions.md#kerod).

The hook goes where each agent looks for one, for the agents you have installed:

| Agent       | Where the hook goes                                   |
| ----------- | ----------------------------------------------------- |
| Claude Code | A `SessionStart` hook in `~/.claude/settings.json`     |
| Codex       | A `SessionStart` hook in `~/.codex/hooks.json`         |
| Pi          | An extension, `~/.pi/agent/extensions/kero.ts`         |
| OMP         | An extension, `~/.omp/agent/extensions/kero.ts`        |
| OpenCode    | A plugin, `~/.config/opencode/plugins/kero`            |

Kero follows `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `PI_CODING_AGENT_DIR`, `PI_CONFIG_DIR` and `XDG_CONFIG_HOME` when they move an agent's files, and leaves the rest of each settings file as it was.

Pass `--agent` with `claude`, `codex`, `pi`, `omp` or `opencode` to set up one agent only. Setup is once: the skill is a link to Kero's own copy, which follows Kero as it updates, and Kero keeps the Pi, OMP and OpenCode plugins current too. `kero skill status` tells you whether each part is current.

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

See the repository's worktrees, with what `git worktree list` doesn't tell: the sessions in each, which Kero made, what removing one would lose and what the automatic cleanup will do with it; check their own session's worktree, setup and pull request; and move their own session, with its conversation, into a new git worktree, to go on there once their turn ends ([more](./changes.md#moving-a-session-into-a-worktree)):

```sh
kero worktree list
kero worktree status
kero worktree handoff fix-login --prompt "go on with the fix"
```

Work with pull requests: write a [review guide](./changes.md#review-guides), see the session's linked pull request, which Kero watches for them, typing a message starting "Kero PR watch:" into their prompt when its checks fail, and merge, update or check one out when you ask them to ([all of it](./cli.md)):

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

## Orchestrator sessions

An orchestrator is an agent session that plans the work, starts other sessions for it, its **children**, and follows them, while you talk to it alone. Start one from the [New Session dialog](./sessions.md#new-session-dialog) with **Orchestrator** and an optional **Goal**, or:

```sh
kero new --orchestrator --goal "Ship the new login page"
kero orchestrator <session>       # make a session one; kero orchestrator <session> off undoes it
```

Its agent, the one chosen (else `default-agent`), starts on a role prompt: plan and delegate, start children with `kero new`, answer what it can, ask you only what it must, and keep the goal in mind. Replace the prompt in **Settings › Agents › Orchestrators** (`orchestrator-prompt`), where `{goal}` is its goal and `{agent}` the default agent.

- **Children.** A session an orchestrator starts with `kero new` is its child, in its project. There is one level: a child's own `kero new` starts another child of the same orchestrator, never a grandchild. `kero new --parent <session>` names the orchestrator, `--parent none` starts a session of none. `orchestrator-max-children` limits how many run at once.
- **Reports.** Kero types into the orchestrator's prompt, once it's at it, when a child comes to wait for input, goes idle after working, fails, exits or closes: one message for what comes close together, and nothing for a state that lasts under two seconds. A child sends its own result with `kero report --to-parent "<summary>"`, which goes with the next report. A child its orchestrator closes itself isn't reported.
- **The sidebar.** The orchestrator's row has its mark, and its goal in its tooltip; its children show right under it, each with a ↳, wherever they are moved. Its chevron folds them, which Kero remembers; folded, it counts the children that need you.
- **Notifications.** Children change the sidebar but don't notify you, nor count on the Dock's badge: the orchestrator hears of them. Its own waiting and errors notify as any session's. `notify-children` (**Settings › Notifications**) turns the children's back on.
- **You can still talk to a child**: click it and type. The orchestrator hears of it from the next report.
- **Closing** an orchestrator asks whether to close its children too or keep them as top-level sessions (`kero close <orchestrator> --children` closes them); a child in a worktree of its own keeps it, for the [cleanup](./changes.md). Closing a child takes it out of the tree, a worktree child asking about its worktree as any does.

`kero ls` marks orchestrators and children; `kero status <orchestrator>` shows its goal and children, and `kero status <child>` its orchestrator.

## Ama, the helper agent

Ama is an agent that runs Kero for you. Its name comes from *amagaeru*, the Japanese tree frog. Click the frog at the bottom of the sessions sidebar, or press **Ctrl+Shift+A** (**⇧⌘A** on macOS), or choose **View › Ama**, and ask it things such as:

- "Start a code review for every open PR in project X."
- "Clean up all old worktrees."
- "Add a new project for this folder and start an orchestrator there with this goal."
- "Which sessions need me?"
- "Close every finished session in project Y."

Ama is an ordinary agent session in a real terminal. It runs your default agent, or the one you pick in **Settings › Agents › Ama** (`ama-agent`). It doesn't do project work itself: for real work it starts a session, or an orchestrator, in the right project. It belongs to no project and runs in a folder of its own, `ama` beside Kero's configuration file (`~/.config/kero/ama`). The sidebar doesn't list it, Needs You doesn't count it, and it sends no notifications. The frog shows a dot while Ama waits for you.

Ama keeps running when you close its popover, and the next time you open it you're back in the same conversation. In the popover:

- **New Chat** starts a fresh conversation.
- **History** goes back to one of Ama's earlier conversations.
- **Clear** clears the screen.

Kero writes Ama's instructions into its folder as `CLAUDE.md` and `AGENTS.md`, and rewrites them as Kero updates. Your own instructions go in `USER.md` beside them. Kero creates that file once and never changes it, and Ama reads it first. **Settings › Agents › Ama** opens it. Kero also installs its skill for Ama's agent before Ama starts, as `kero skill setup` does.

Ama manages Kero with the commands in [Managing Kero](./cli.md#managing-kero).

## Remote Control

With **Remote Control** on (the default, in **Settings › Agents**), agents can also drive the app itself: show a session with `kero focus`, or run menu actions with `kero do`. Turn it off with `remote-control = false` to keep agents to their own sessions.

Ama doesn't need Remote Control: kerod gives Ama's panes a secret, `$KERO_AMA`, which is a hash of Ama's session with a key that kerod makes anew each time it starts. `kero do`, `kero focus` and `kero browser` pass it on, and the app takes the action without Remote Control only when kerod confirms the secret is Ama's. No file holds it, other panes never get it, and it means nothing to another machine's kerod, so Remote Control still decides for every other agent.

## Environment

Every pane gets:

| Variable       | Value                          |
| -------------- | ------------------------------ |
| `KERO_SESSION` | The id of the pane's session   |
| `KERO_PANE`    | The id of the pane             |

Check `$KERO_PANE` to tell whether a script runs inside Kero.
