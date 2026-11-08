---
title: Orchestrators
description: An agent session that plans the work, starts child sessions for it and follows them, while you talk to it alone.
---

# Orchestrators

An orchestrator is an agent session that plans the work, starts other sessions for it, its **children**, and follows them, while you talk to it alone.

## Starting one

Start one from the [New Session dialog](./sessions.md#new-session-dialog) with **Orchestrator** and an optional **Goal**, or:

```sh
kero new --orchestrator --goal "Ship the new login page"
kero new -p api --orchestrator --goal "Ship the new login page" -- codex   # another agent than default-agent
kero orchestrator <session>       # make a session one; kero orchestrator <session> off undoes it
```

`kero orchestrator <session> off` makes it an ordinary session again, whose children become top-level sessions.

## Its role prompt and agent

Its agent, the one chosen (else `default-agent`), starts on a role prompt: plan and delegate, start children with `kero new`, answer what it can, ask you only what it must, reply to a report that needs nothing with a plain OK rather than a summary, and keep the goal in mind. Replace the prompt in **Settings › Agents › Orchestrators** (`orchestrator-prompt`), where `{goal}` is its goal and `{agent}` the default agent. To give orchestrators' agents arguments of their own, such as `--model opus`, whichever agent they run, set **Arguments** there (`orchestrator-agent-args`): Kero adds them after the agent's command when an orchestrator starts and when it resumes.

## Children, reports and the sidebar

- **Children.** A session an orchestrator starts with `kero new` is its child, in its project. There is one level: a child's own `kero new` starts another child of the same orchestrator, never a grandchild. `kero new --parent <session>` names the orchestrator, `--parent none` starts a session of none. `orchestrator-max-children` limits how many run at once.
- **Reports.** Kero types into the orchestrator's prompt, once it's at it, when a child comes to wait for input, goes idle after working, fails, exits or closes: one message for what comes close together, and nothing for a state that lasts under two seconds. A child sends its own result with `kero report --to-parent "<summary>"`, which goes with the next report. A child its orchestrator closes itself isn't reported. A report waits until the orchestrator is at its prompt, and a child closed meanwhile takes its state lines with it: the orchestrator hears only that it closed, and still gets its summary.
- **The sidebar.** The orchestrator's row has its mark, and its goal in its tooltip; its children show right under it, each with a ↳, wherever they are moved. Its chevron folds them, which Kero remembers; folded, it counts the children that need you.
- **Notifications.** Children change the sidebar but don't notify you, nor count on the Dock's badge: the orchestrator hears of them. Its own waiting and errors notify as any session's. `notify-children` (**Settings › Notifications**) turns the children's back on.
- **You can still talk to a child**: click it and type. The orchestrator hears of it from the next report.
- **Closing** an orchestrator asks whether to close its children too or keep them as top-level sessions (`kero close <orchestrator> --children` closes them); a child in a worktree of its own keeps it, for the [cleanup](./worktrees.md#cleaning-up-worktrees). Closing a child takes it out of the tree, a worktree child asking about its worktree as any does.

`kero ls` marks orchestrators and children; `kero status <orchestrator>` shows its goal and children, and `kero status <child>` its orchestrator.
