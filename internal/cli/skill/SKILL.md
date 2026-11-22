---
name: kero
description: Drive Kero, the terminal you run in ($KERO_PANE is set) — run servers, agents and browser panes beside you, report your status, orchestrate child sessions or report to your orchestrator, move into a worktree, act on pull requests and Kero PR watch messages, and manage Kero's sessions and settings. Use when running inside Kero and the task needs another terminal, a long-running process, a parallel agent, a page or the attention of the user, a worktree of your own, or a pull request.
---

# Kero

You run inside Kero, a terminal whose sessions keep running in the
background. $KERO_SESSION is your session and $KERO_PANE your pane; the
`kero` command drives them. A target is a session or a pane id (a prefix
works), or a session's name; a session stands for its focused pane.
`kero help` lists every command and option.

## When to use it

Run a command that ends on its own (a test run, a build, a lint, git) with
your own shell or command tool, as you would outside Kero: it needs no
pane. Start one with kero only when it should keep running or be seen: a
dev server or watcher, an interactive program, another agent, or output
the user should watch. Then make it a pane of your session with split
rather than a session of its own, and reuse what you started: restart a
server in its pane (kero key <pane> ctrl+c, then kero send <pane> "npm run
dev" --enter) rather than open another, and close a pane once its output
has served.

## Reference

The details are in files beside this one, under reference/. Read the one
a task needs before you start it; `kero skill show <name>` prints one too,
as `kero skill show orchestrating`.

- reference/cli.md: splitting panes, starting sessions, typing into them,
  waiting for and reading them, showing the user a file or document. Read
  before your first kero command beyond kero report.
- reference/reporting.md: your status line, and the four-part report to
  your orchestrator. Read before reporting to your orchestrator, as a
  child, and before your first kero report.
- reference/orchestrating.md: splitting the work, briefing children,
  following and routing their reports, integrating, releasing and closing
  them. Read before starting child sessions, as an orchestrator.
- reference/worktrees-prs.md: worktrees, moving your session into one,
  your pull request and acting on one. Read before a handoff, acting on a
  "Kero PR watch:" message, or merging.
- reference/browser.md: driving a browser pane, its console, network and
  emulation, the user's sign-ins, Pick mode, DevTools, and getting the
  user's answers from a page. Read before using a web page, or showing
  the user a page to decide on.
- reference/reviews.md: draft review comments and review guides of a pull
  request. Read before reviewing a pull request, or writing a guide of one.
- reference/hosts.md: the user's other machines (kero -H). Read before
  running anything on another machine.
- reference/manage.md: projects, sessions, worktrees and settings, as
  Kero's app and its helper agent, Ama, manage them. Read before changing
  Kero's projects, closing many sessions, or changing a setting.

## Etiquette

- Start a session or pane only when the user should have one: a few you
  reuse beat one for each command.
- Close what you started when it is done: `kero close <target>`.
- Never type into a pane you did not start unless the user asked.
- Exit status 3 means no such target, 4 that Kero is not running.
