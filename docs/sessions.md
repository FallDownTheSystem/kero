---
title: Sessions & Projects
description: How Kero groups terminal sessions by project and keeps them running in kerod.
---

# Sessions & Projects

Kero organizes your work in two levels: projects (folders) hold sessions (tasks), and every session keeps running in kerod until you close it.

## Sessions

A **session** is a unit of work: one task, one agent, one long-running process. It holds one or more [panes](./panes.md), each running a shell or a program.

| Action                     | Shortcut                                     |
| -------------------------- | -------------------------------------------- |
| New session                | <kbd>⌘</kbd> <kbd>T</kbd>                    |
| New session in home folder | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>T</kbd>       |
| Go to session…             | <kbd>⌘</kbd> <kbd>P</kbd>                    |
| Jump to session needing you | <kbd>⌘</kbd> <kbd>J</kbd>                   |
| Next / previous session    | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>]</kbd> / <kbd>[</kbd> |
| Rename session             | <kbd>⌘</kbd> <kbd>R</kbd>                    |
| Close session              | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>W</kbd>       |

Name sessions after their task, such as `fix-login-redirect` or `review-pr-42`, so you can find them in the sidebar and target them from the [CLI](./cli.md).

### Where a new session starts

The `new-session-directory` setting picks the folder:

- `current` (default): the directory of the session you're in
- `project`: the project's root folder
- `home`: your home folder

## Projects

A **project** is a folder. Kero groups sessions under their project in the sidebar. Open one with <kbd>⌘</kbd> <kbd>O</kbd>, or let `kero new` pick it by the directory a session starts in.

```sh
kero projects            # list projects
kero new -p my-app       # start a session in a project by name
```

## kerod

Sessions run in **kerod**, a daemon that the app starts on demand. Kero is a window onto kerod:

- Quitting Kero leaves every session running.
- Opening Kero again shows every session as it was, scrollback included.
- After kerod itself restarts, such as after a reboot, Kero starts your sessions again in their directories. Turn this off with `restore-sessions = false`.

```sh
kero daemon status       # is kerod running?
kero daemon restart      # restart it (sessions start again)
kero daemon stop         # stop it and end every session
```
