---
title: Troubleshooting
description: Where Kero keeps its files and log, how to restart kerod, what to do when an update fails, and how to check each part.
---

# Troubleshooting

<a id="where-kero-keeps-things"></a>

## Where Kero keeps things

Kero keeps your settings in one folder and its own state in another.

**The configuration folder**, `~/.config/kero` (or `$XDG_CONFIG_HOME/kero`; on Windows too, as `C:\Users\<you>\.config\kero`). `kero config path` prints the file's path.

| File | What it is |
| --- | --- |
| `config` | Your [settings](./configuration.md) |
| `agents.toml` | [Your own agents](./status.md#your-own-agents), when you have some |
| `ama/` | [Ama's folder](./ama.md#its-folder): its instructions, your `USER.md`, Kero's docs |

**The data folder**: `%AppData%\kero` on Windows, `~/Library/Application Support/kero` on macOS, and `~/.config/kero` on Linux, where it is the configuration folder too. `$KERO_HOME` moves it. **Settings › Advanced** shows it, and its log.

| File | What it is |
| --- | --- |
| `kerod.log` | kerod's [log](#logs) |
| `state.json` | The projects and sessions, which kerod starts again after a restart |
| `sessions/` | Each pane's scrollback |
| `conversations.json` | The agents' [past conversations](./sessions.md#past-conversations) |
| `app.json`, `window-state.json` | The window: its size and place, the sidebars, Ama's window |
| `updater.json` | When Kero last checked for updates, and a version you skipped |
| `reviews/`, `guides/` | [Draft reviews](./pull-requests.md#draft-reviews) and [review guides](./pull-requests.md#review-guides), a file per pull request |
| `worktree-cleanup.json` | What the [worktree cleanup](./worktrees.md#cleaning-up-worktrees) knows of worktrees |
| `skills/kero/` | Kero's skill, which the agents' skills link to |
| `adblock/` | The [ad blocking](./panes.md#blocking-ads) filter lists |
| `bin/` | The `kero` command panes find on their `PATH` |
| `kerod.sock`, `kerod.lock`, `kerod.running` | How the app and `kero` reach kerod, and whether it is running; `$KERO_SOCKET` moves the socket |

On Windows, browser panes keep their cookies and site data in `%LOCALAPPDATA%\Kero\WebView2`.

Worktrees Kero makes go in each repository's `.kero/worktrees`, or under `worktree-dir` ([Worktrees](./worktrees.md#where-worktrees-go-and-what-they-start-from)).

<a id="logs"></a>

## Logs

kerod writes one log, `kerod.log` in the data folder: on Windows, `%AppData%\kero\kerod.log`. **Settings › Advanced › Log** shows it in the file manager. When the app or `kero` starts kerod, what kerod prints as it starts, or fails to, goes there too.

Each run of kerod starts with `--- daemon run ---` and a line naming its version and process. It logs sessions starting, closing and coming back after a restart, worktrees made and removed, `.worktreeinclude` copies and [setup scripts](./worktrees.md#setup-scripts) (`setup script of <worktree> runs in pane <pane>: <command>`, then how the setup ended), problems in `agents.toml` (`agents: …`), and errors. `previous daemon exit was unclean` says kerod last ended without saving, as when the computer lost power. Kero empties the log as kerod starts once it is over 4 MB.

`kero daemon serve` writes the log to its standard error too.

<a id="restarting-kerod"></a>

## Restarting kerod

kerod is the background service that runs your sessions ([kerod](./sessions.md#kerod)). Restart it after an update, when the app says it is of another version, or when it stops answering:

- The sidebar's **restart** button, which shows once Kero updated: "Kero was updated: restart the background service to run the new version".
- **Settings › Advanced › Background service › Restart**.
- **Restart Background Service** in the command palette.
- `kero daemon restart`.

A restart saves the sessions first, and starts them again where they were, with their scrollback; agents resume their conversations. Programs that aren't agents come back at a shell.

```sh
kero daemon status       # is kerod running: its process, uptime, sessions and panes, socket
kero daemon restart
kero daemon stop         # stop it; the sessions start again as it next starts
```

`kero` exits with status `4` when it can't reach kerod; the app's sidebar says **Can't reach kerod**. Opening the app, or any `kero` command, starts kerod again. A `kero` of another version than kerod's says to run `kero daemon restart`.

kerod quits by itself after 30 seconds with no sessions and nothing connected, unless it runs as a service (`kero daemon serve`) or listens for other machines; the log says `idle: exiting`.

## Updates

Kero checks for updates once a day while it's open, and **Settings › General › Check for Updates** checks now. **Download and install updates automatically** installs one as it comes; it takes effect the next time Kero opens.

**A check that fails**, as `504 Gateway Timeout` from GitHub, changes nothing: an automatic check fails silently and tries again later, and **Check for Updates** says it couldn't check. A status of 500 to 504 is GitHub's side, not yours: try again later. To update now, install it by hand:

1. Download the installer, `kero-setup-<version>-amd64.exe` on Windows, from the releases page of the repository your Kero updates from: for this build, [github.com/FallDownTheSystem/kero/releases](https://github.com/FallDownTheSystem/kero/releases).
2. Stop kerod, which keeps `Kero.exe` in use: **Quit and End All Sessions…**, or `kero daemon stop`.
3. Run the installer from File Explorer or a terminal of your own, not from inside a Kero pane or an agent's shell ([why](#panes-without-colors)).

After an update, restart kerod as the sidebar asks, so that your sessions run the new version. Update a host's Kero with `kero -H <host> daemon update`, or `kero hosts install <host>` ([Remote Hosts](./remote.md#versions-and-updates)).

<a id="panes-without-colors"></a>

## Panes without colors

kerod passes the environment it started with to every pane. Started from a shell that sets `NO_COLOR`, as an agent's often does, every program in every pane leaves its colors out. Quit Kero with **Quit and End All Sessions…**, or `kero daemon stop`, and open it again from the Start menu, the Dock or a terminal of your own. The same goes for other variables a shell set for itself.

## Agents

- **An agent's status doesn't show, its session isn't named, or it doesn't pick up its conversation after a restart**: Kero's hook isn't set up for it. `kero skill status` says what's installed and current, and `kero skill setup` installs it ([Install the skill](./agents.md#install-the-skill)). The hook works only in sessions started after setup; Claude Code runs hooks only in folders you trust, and Codex asks you to approve it with `/hooks`.
- **`kero do`, `kero focus` or `kero browser` fails for an agent**: they need **Remote Control** (**Settings › Agents**, `remote-control`) and the app open. `kero browser wait-message`, `kero browser devtools` and `kero do pane.preview` don't. [Ama](./ama.md#without-remote-control) needs no Remote Control.
- **An agent's command isn't found in the New Session dialog**: Kero runs agents in your shell, as you would, so an alias works only in a shell that defines it. `kero config get agent` lists the agents, and `kero config check` reports a bad `agents.toml`.

## Worktrees

- **A setup script didn't run**: it runs only for worktrees Kero makes, and only the first script with `"runOnWorktreeCreate": true` in `kero.json`, or `t3.json` ([Setup scripts](./worktrees.md#setup-scripts)). `kero worktree status`, in the worktree's session, shows its setup; `kerod.log` shows what ran and how it ended. A `kero.json` that isn't valid JSON fails the setup.
- **An ignored file is missing in a new worktree**: list it in the main checkout's `.worktreeinclude` ([Copying ignored files](./worktrees.md#copying-ignored-files)).
- **A worktree won't go**: `kero worktree list` says what removing each would lose, and what the cleanup makes of it. The cleanup never removes a worktree with changes, one a session works in, or one made in the last ten minutes.

## Settings

`kero config check` reports unknown keys and bad values, in the configuration file and in `agents.toml`. `kero config set <key>`, with no value, puts one key back to its default, and **Settings › Advanced › Reset all settings** every key, the keyboard shortcuts too; your sessions stay.

## Checking each part

```sh
kero version                 # the version of kero and of its protocol
kero daemon status           # kerod
kero clients                 # the apps and commands connected to kerod
kero config check            # the configuration file and agents.toml
kero skill status            # the skill and the hooks, for each agent
kero worktree status         # this session's worktree, setup and pull request
kero hosts check <host>      # a host: connect, and say how it went
kero browser devtools        # the browser panes' DevTools port
kero ls --json               # every session and pane, with its status
```
