---
title: Status & Notifications
description: How Kero tracks what each pane is doing, and how programs report their own status.
---

# Status & Notifications

Kero tracks what every pane is doing, so you can leave agents to work and come back when one needs you.

## Pane status

Every pane has a status, shown in the sidebar:

| Status    | Meaning                                       |
| --------- | --------------------------------------------- |
| `working` | A program is busy: an agent thinking, a tool it runs |
| `waiting` | It needs you: a question, a permission prompt |
| `idle`    | Nothing is happening; the shell is at a prompt |
| `error`   | The program reported that it failed           |
| `exited`  | The program ended; the pane keeps its output  |

Kero tells a coding agent's status from what its screen shows: its spinner, a permission prompt, its prompt waiting for you. Redrawing the same screen, as when you resize a pane, changes nothing. Other programs are `working` while their title shows a spinner, or when they [report their status](#reporting-status-from-a-program).

A session shows the status of its panes, and sessions that need you stand out. Press <kbd>⌘</kbd> <kbd>J</kbd> to jump to the next one.

## Notifications

Kero notifies you when you're looking elsewhere and:

- a session comes to need you (`notify-on-waiting`)
- an agent finishes (`notify-on-done`)
- a program reports that it failed (`notify-on-error`)

The Dock icon shows how many sessions need you (`dock-badge`) and can bounce once when one starts to (`dock-bounce`). See [Configuration](./configuration.md).

## Reporting status from a program

Programs and agents can tell Kero what they're doing with `kero report`:

```sh
kero report working "Migrating the database" --progress 40
kero report blocked "Approve the production deploy?" --kind permission
kero report done "Deployed to staging"
kero report clear
```

The states are `idle`, `working`, `done`, `blocked`, `error` and `clear`. Kero shows the message in the sidebar. A `blocked` report turns the session to *waiting*, and `done` or `blocked` notify you when you're away.

To flag a pane as needing you without changing its state:

```sh
kero notify "Need your API key to continue"
```

### The escape sequence

`kero report` writes an OSC 7501 escape sequence to the terminal, which any program can emit directly:

```txt
ESC ] 7501 ; state=<state> [:progress=<n>] [:kind=<kind>] [:msg=<base64>] ESC \
```

Fields are separated by `:`, and `msg` and `title` are Base64-encoded. For example, in a shell:

```sh
printf '\e]7501;state=working:progress=40:msg=%s\e\\' "$(printf 'Building' | base64)"
```
