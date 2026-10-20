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

This installs a `kero` skill that teaches agents the CLI:

- Claude Code reads it from `~/.claude/skills/kero`
- Codex, OpenCode and other agents read it from `~/.agents/skills/kero`

Pass `--agent claude` or `--agent codex` to install for one agent only. The skill comes from your Kero binary, so run `kero skill setup` again after updating Kero. `kero skill status` tells you whether an installed skill is current.

```sh
kero skill show      # print the skill
kero skill remove    # uninstall it
```

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

Tell you what's going on:

```sh
kero rename $KERO_SESSION fix-login-redirect
kero report working "Migrating the database" --progress 40
kero report blocked "Approve the production deploy?"
kero report done "Deployed to staging"
```

A `blocked` report turns the session to *waiting* and notifies you, so agents should use it only when they can't continue without you.

## Remote Control

With **Remote Control** on (the default, in **Settings › Agents**), agents can also drive the app itself: show a session with `kero focus`, or run menu actions with `kero do`. Turn it off with `remote-control = false` to keep agents to their own sessions.

## Environment

Every pane gets:

| Variable       | Value                          |
| -------------- | ------------------------------ |
| `KERO_SESSION` | The id of the pane's session   |
| `KERO_PANE`    | The id of the pane             |

Check `$KERO_PANE` to tell whether a script runs inside Kero.
