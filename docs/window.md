---
title: The Window
description: The parts of Kero's window, what each shows and where its buttons are, and the pages of Settings.
---

# The Window

Kero's window has three parts, from left to right: the **sessions sidebar**, the **panes** of the session you're in, and the **right sidebar**, with the **viewer** beside it when it shows a file or a pull request. With `sessions-sidebar = right`, the sessions sidebar takes the right, and the right sidebar and the viewer the left.

## The sessions sidebar

The sessions sidebar lists your [projects](./sessions.md#projects), each with its sessions under it. Show or hide it with **Toggle Sidebar** (<kbd>⌘</kbd> <kbd>B</kbd>, <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>B</kbd> on Windows and Linux).

- **A project's header** has its name, how many sessions it holds (`sidebar-session-count`), and a **+** that opens the [New Session dialog](./sessions.md#new-session-dialog). Its menu starts a session, a session in a worktree, moves the project, and removes it. Drag a header to reorder the projects.
- **A session's row** has its status icon and its name; with `sidebar-rows = detailed`, a second line has its status message, else its branch and changes. A session on a branch of its own, as in a [worktree](./worktrees.md), shows the branch's name. An [orchestrator](./orchestrators.md) has its mark, and its children show under it, each with a ↳; a pull request's review is marked **Review**. Hover a session for its branch, how far it is ahead of or behind its upstream, and its pull request's checks. Its menu (right-click) renames it, restarts it once it exited, marks it read, moves it to another project, copies its id or folder, shows its folder in the file manager, shows or unlinks its pull request, makes it an orchestrator, and closes it.
- With `session-history` on, **Ended** and **Archived** list the agents' ended conversations, to resume ([Past conversations](./sessions.md#past-conversations)).

At its foot:

- **Open Project**, **Go to Session** (the [command palette](./shortcuts.md#the-command-palette)), **Settings**, and **Menus** (⋯), which holds the menu bar's menus on Windows and Linux, which show no menu bar.
- At the right, a count of the sessions that need you, those working, and those done that you haven't seen; click one to go to the next such session. While kerod can't be reached, the foot says **Connecting…** or **Can't reach kerod** instead ([Troubleshooting](./troubleshooting.md#restarting-kerod)).
- A restart button once Kero updated and kerod still runs the old version: it restarts the background service, and your sessions with it, where they were.
- The frog, which opens [Ama](./ama.md). It shows a dot while Ama waits for you; its menu has **Reset Window Position**.

## Panes

The middle of the window is the session you're in: its [panes](./panes.md), terminals, editor panes and browser panes, side by side or one above another. Each pane's title shows its folder (`pane-folder`), and its branch with the lines its changes add and remove, as `fix-login +40 −2`. Click a pane's branch to show its [changes](./changes.md). Panes you're not in fade (`dim-unfocused-terminals`).

## The right sidebar

The right sidebar has four tabs:

| Tab | Shows | Page |
| --- | --- | --- |
| **Files** | The files of the pane's worktree, with **Search** of their text | [Files & Search](./files.md) |
| **Changes** | What changed, by the last turn, uncommitted or the branch; staging and committing, branches and stashes | [Changes](./changes.md#changes) |
| **History** | The commits as a graph, the stash, the repository's worktrees | [Changes](./changes.md#history) |
| **Review** | The repository's open pull requests, and the agents reviewing them | [Pull Requests](./pull-requests.md) |

Show or hide it with the button at the top right of the window, or open a tab with its shortcut ([Keyboard Shortcuts](./shortcuts.md#view)). Drag its edge to resize it; **Zoom Sidebar** beside the hide button gives it the panes' room, until you click **Show All Panes**. In a window too narrow for it beside the panes, it gets out of their way, and comes back as you widen the window.

## The viewer

A file you click in Files, Changes or Search, and a pull request you click in Review, opens in the viewer, a card beside the panes: a file to read and edit, its diffs, or the pull request's page. Close it with its <kbd>×</kbd>, or leave it with <kbd>Esc</kbd>. **Zoom Viewer** gives it the panes' room, and **Open in Pane** moves a file to an editor pane of its own.

## Cards and notifications

- A card at the bottom right shows each new worktree's [setup](./worktrees.md#where-worktrees-go-and-what-they-start-from) as it goes, with **Cancel**.
- Short messages, as an action's error, show for a moment over the window.
- [Notifications](./status.md#notifications) are the system's: when a session needs you, an agent finishes, a program fails, or a pull request asks for your review.

## Ama's window

[Ama](./ama.md) opens in a window of its own, above Kero's, from the frog or with <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>A</kbd> (<kbd>⇧</kbd> <kbd>⌘</kbd> <kbd>A</kbd> on macOS). Its header has **New Chat**, **History** and **Clear**.

## Kero's icon

Kero's icon in the notification area on Windows and Linux, and in the menu bar on macOS (`macos-show-app-icon`), keeps Kero running when you close its window. Its menu opens Kero, opens the [quick terminal](./shortcuts.md#quick-terminal), keeps the computer awake (`keep-computer-awake`), opens Settings, checks for updates, and quits: **Quit** leaves the sessions running in kerod, and **Quit and End All Sessions…** ends them.

## Settings

**Settings…** (<kbd>⌘</kbd> <kbd>,</kbd>, <kbd>Ctrl</kbd> <kbd>,</kbd>) opens the Settings window, whose search field finds a setting by its name or its key. Each page writes the keys of [Configuration](./configuration.md):

| Page | What it sets |
| --- | --- |
| **General** | Where new sessions start and how they're named, the sidebar's rows, closing and restoring sessions, opening Kero at login, keeping the computer awake, updates |
| **Appearance** | Light or dark, the language, the theme and its colors, the app's font, which side the sessions sidebar takes, fading what you're not in |
| **Terminal** | The terminals' font, its size, weight and features, line height, cursor, copying and the mouse |
| **Files** | Your editor, dotfiles in the Files tab, the colors of code |
| **Git** | Git status, diffs, the branch switcher, worktrees and their [cleanup](./worktrees.md#cleaning-up-worktrees), commit messages, pull requests: notifications, watching, prompts |
| **Shell** | The shell of new panes, its environment, the scrollback kept |
| **Browser** | Blocking ads, the DevTools port |
| **Notifications** | What notifies you, the sound, the Dock's badge |
| **Keyboard** | Every shortcut, to rebind |
| **Agents** | Kero's skill for agents, the agents and the default one, [orchestrators](./orchestrators.md), [Ama](./ama.md), resume commands, Remote Control |
| **Hosts** | [Remote hosts](./remote.md), and letting other machines reach this one |
| **Advanced** | The configuration file, the background service (**Restart**), the log, the data folder, **Reset all settings** |
