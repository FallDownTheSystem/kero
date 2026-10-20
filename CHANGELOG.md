# Changelog

## 2.0.3

- An agent sitting at its prompt no longer shows as needing you, as Claude
  Code did a while after answering. A bell or a program's desktop
  notification leaves its status alone; programs' notifications show as
  they are.

## 2.0.2

- Installing the command line tool on macOS asks for an administrator's
  password instead of failing.

## 2.0.1

- Kero speaks Simplified Chinese; pick the language in Settings.
- Rename a project from its menu in the sidebar.
- `kero skill setup` hooks Claude Code, Codex, Pi and OpenCode into Kero as
  they start: an agent learns its session and pane, names an unnamed session
  after its task, and resumes when kerod restarts.
- `kero new|split --wait` and `kero wait --exit` exit with the command's
  status.
- When kerod runs another version of Kero, a Restart Service chip offers to
  restart it. Settings shows updates on its General page.
- A home directory pane shows `~` once in its header.

## 2.0.0

Kero, rewritten from the ground up: sessions grouped by project in a sidebar
that shows which are working, which need you and which are done. Sessions run
in kerod, a daemon of their own, so quitting Kero leaves them running.

- Runs on macOS, Windows and Linux.
- The `kero` CLI drives sessions from a shell or an agent.
- Updates itself from Kero's releases.
