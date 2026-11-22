---
title: Assistants
description: Agents of no project, each with a name, a role and checks Kero wakes it for daily, weekly or as Kero starts, which report to you and start sessions for the work.
---

# Assistants

An assistant is an agent of yours that belongs to no project: it has a name, a color and an avatar, a role, what it does or keeps track of, and checks Kero wakes it for. One may follow your Jira tickets each morning, another read your mail, another clean up merged branches each Monday or remind you of questions you haven't answered. Assistants have no tools of their own: they use what your machine gives agents, its MCP servers, CLIs and skills.

## The Assistants view

The toggle at the top of the sessions sidebar switches between **Sessions** and **Assistants** (**View › Sessions or Assistants**). Assistants is marked with a dot and a count while assistants need you, and a green dot while one has news you haven't read.

The view groups assistants by their **Group**, those of none first. Each row shows the avatar, the name and its session's status, and on a second line its status line, else the first line of its last report, else its role, with the time of its next check, **Paused**, or a problem at its right. The sessions an assistant started show under it.

Click an assistant to show its session in the window, as any session; one with no session starts one. **+**, or **Session › New Assistant…**, adds one. Right-click an assistant for **Run Checks Now**, **New Chat**, **Pause Checks**, **Edit…**, **Show Folder**, **Close Session** and **Delete Assistant…**. The Sessions view doesn't list assistants' sessions, but lists the sessions they start, each in its own project.

## Creating one

The dialog asks for its **Name**, **Role** (one line, as "Keeps my Jira tickets moving"), **Color** (one of eight, or any color of yours from the well after them), **Avatar** (its initial, one of twenty glyphs, nine symbols and eleven animals, or an image of yours, PNG, JPEG, GIF or WebP up to 2 MB), what it does or keeps track of, its **Group**, its **Checks**, and its **Agent** and **Arguments**. Created, its session starts and shows, for you to answer what its agent asks first, as whether to trust its folder.

Checks run unattended: an agent that asks for permission waits for you. Give it arguments that allow what its checks need, as Claude Code's `--allowedTools` or `--permission-mode`.

## Checks

A check is **Daily** at a time, **Weekly** on days at a time, or **When Kero starts**, each with what to check. Checks run only while Kero is running on this computer, in its local time. Kero records when each ran: a check missed while Kero was closed, or the computer off, runs once when Kero is next running, however many times it was missed. A **When Kero starts** check runs once each time kerod starts, at most every four hours, so that a restart for an update doesn't repeat it. With kerod installed as a service (`kero daemon install-service`), checks run with Kero's window closed too.

A check reaches the assistant once its agent is at its prompt, as "Kero: message from your schedule:", with the checks due and when each was due; a busy assistant gets it after its turn. Kero starts the assistant's session first when it has none.

## Reports and notifications

An assistant ends each check with a report, in the four parts of every agent's report in Kero (Result, Not done, Where, Next), with `kero assistants report`. Kero keeps the last ten. A report that starts "Waiting on you:" shows the assistant waiting on its question, as Needs You counts it, until you answer in its session, an answer is sent to it (as Ama's `kero message`), or it reports again. A check or a child's report reaching it meanwhile leaves the question showing.

Kero notifies you, under the **Notifications** settings, when an assistant needs you or failed, and when it finished a check: titled "Jira finished a check", with its report's result. Its other turns, as answering a session it started, only mark it unread.

## Its folder

Each assistant has a folder, `assistants/<id>` beside Kero's configuration file, with:

- `CLAUDE.md` and `AGENTS.md`: its instructions, which Kero writes from its role, what it keeps track of and its checks, and rewrites as you edit it.
- `USER.md`: your own instructions for it, which Kero makes once and never changes.
- `MEMORY.md`: its notes, which it keeps between checks and chats, and Kero never changes.

Its session runs in the folder, and its panes get `KERO_ASSISTANT`, its id. Deleting an assistant keeps the folder unless you delete it too.

## Sessions it starts

For work in a repository, or work that runs long, an assistant starts a session with `kero new`, naming its project or folder, which becomes its child: it hears of the child as an [orchestrator](./orchestrators.md) does, and messages it with `kero message`. **An orchestrator's sessions** (Settings › Notifications) decides whether its children notify you, and `orchestrator-max-children` limits them.

## From the command line

```sh
kero assistants                         # each, how it stands, its next check
kero assistants news --json             # what each reported last, and who waits for you
kero assistants create Jira --role "Keeps my Jira tickets moving" \
    --check 'daily@09:00=What moved in my tickets since yesterday?'
kero assistants run jira                # its checks, now
kero assistants pause jira
```

`kero assistants help` lists them all. [Ama](./ama.md) answers "Any news from my assistants?" from `kero assistants news`.
