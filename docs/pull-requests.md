---
title: Pull Requests
description: List, read, review, create and merge a repository's GitHub pull requests in Kero, and have agents review them.
---

# Pull Requests

Kero works with a repository's pull requests on GitHub through the [GitHub CLI](https://cli.github.com), `gh`, on the host the repository is on, so `gh` must be installed and signed in there. Every action uses `gh` as you, so GitHub decides what you may do, and Kero offers only that.

<a id="review"></a>

## The Review tab

The right sidebar's **Review** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>P</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>K</kbd> on Windows and Linux), lists the open pull requests of the repository of the pane you're in. The list is of the remote the branch pushes to, `origin` most often; the repository's name above the list chooses another of its remotes on GitHub. **Open**, **Mine** and **To Review** filter the list, which shows each pull request's branches, checks and review. Right-click a pull request for Merge, Update Branch, Ready for Review or Convert to Draft, and Close.

## A pull request's page

Click a pull request to see it in the viewer beside the panes; the arrow beside it opens it on GitHub. **Conversation** shows its description, each reviewer and how their review stands (approved, changes requested, or awaiting review), its checks, which a click lists, its comments and reviews, and its threads of comments on lines and files, replies set apart under the comment they answer: click a thread's place to see it in **Files**. **Files** shows its changes, fetched with its base, each file's diff with the comments on its lines under them, and those on the whole file above it. A thread whose lines changed since shows in the conversation alone, marked outdated. **Resolve Conversation** resolves a thread on GitHub, which then folds to a line. The arrows beside the tabs go from comment to comment, yours and those posted. When a worktree of the repository has the pull request's head checked out, as a review's worktree has, right-click a file to open it, or drag it onto a pane; otherwise its files aren't on disk, and don't open.

<a id="reviewing-with-an-agent"></a>

## Reviewing with an agent

**Review** on a pull request has an agent review it. A click fetches the pull request's branch and starts the agent in a new session, in a [worktree of its own](./worktrees.md) of that branch. Each review checks the pull request out on a branch of its own, `review/pr-<number>`, then `review/pr-<number>-2` and on, so you can review it more than once, and while your checkout or another worktree has its branch. Pushing that branch, from the Changes tab, goes to the pull request's branch; with git, run `git push origin HEAD:<branch>`. Closing the review with its worktree deletes its branch once the pull request's has its commits. Right-click it, or the pull request, to send the review to the agent in the pane you're in, or to start the agent in a new session in the pane's folder. Before the review starts, its prompt shows for you to edit. A new session's agent gets the prompt as its command's argument, so it starts on the review at once, with nothing typed into it; the agent in the pane you're in gets it pasted.

A session that reviews a pull request is marked **Review** in the sidebar, and listed under **Review Agents**, above the pull requests, with its status, as the sidebar shows it, and its agent's last message. Click it to go to its session; its menu shows the pull request, or closes the session, which asks whether its worktree goes with it. A pull request with a review agent has **Agent** in place of **Review**, on its row and its page, which goes to the agent. A review whose session closed and kept its worktree stays in the list, to review again in that worktree, or to remove it: **Remove Worktree…** asks first, and says how many changed files it would lose. Reviewing a pull request that has a review already offers its agent, and to review in its worktree rather than a new one.

<a id="repository-setup"></a>

## Repository Setup

**Repository Setup…**, the gear in the Review tab, sets the agent command and the review prompt for each repository, as your repositories have different skills and tools: `/code-review high {ticket}`, for one. In the prompt, `{number}`, `{url}`, `{title}`, `{branch}`, `{base}` and `{author}` become the pull request's, and `{ticket}` the first issue key, such as `PROJ-123`, in its title, its description or its branch; when none has one, the review asks you for it. The command is `claude` unless you set another. Kero gives it the prompt as one argument, after the command, as Claude Code and Codex take a first prompt; put `{prompt}` where the command takes it otherwise, as `codex exec {prompt} --full-auto`. The command takes the pull request's placeholders too.

It also sets the repository's default merge method, `repo-merge-method`, and can replace the commit message, pull request and review prompts for that repository ([Prompts](#prompts)).

## Checking out

**Check Out**, in a pull request's menu and on its page, checks it out without an agent, on a branch of the pull request's name, as `gh pr checkout` does: the pull request's own branch, which pushes back to it, or `pr-<number>` for one from a fork. **Check Out Here** checks it out in the worktree of the pane you're in; Kero won't while that worktree has changes you haven't committed, so commit or stash them first, or check it out in a new worktree. A branch you have already is brought up to the pull request's when it is behind it, and keeps commits of its own. **Check Out in New Worktree** starts a session in a [worktree of its own](./worktrees.md) of that branch. When a worktree has the pull request already, on its branch, on a branch that pushes to it, or a review's, Kero goes there instead, to the session working in it or a new one.

## Merging and other actions

Under the pull request's title, its page says whether it merges as it is, and why not: conflicts, required reviews or checks, a branch behind its base, or a draft. Its button merges it, marks a draft ready for review, reopens a closed one, or deletes or reverts a merged one; the **⋯** beside it has the rest: **Merge…**, **Enable Auto-Merge…** or **Disable Auto-Merge**, **Update Branch** (or with a rebase), **Convert to Draft**, **Close Pull Request…**, **Revert…** and **Delete Branch…**. **Merge** asks how, of the methods the repository allows (squash, merge commit or rebase), and whether to delete the branch after; it merges only at the commit you saw, so a push since stops it. While the pull request's requirements aren't met, **Enable Auto-Merge** has GitHub merge it once they are. A fork's pull request whose workflow runs wait on a maintainer has **Approve Runs**, which asks first, as they run the fork's code.

**Merge** starts on the repository's default merge method, `repo-merge-method`, which [Repository Setup](#repository-setup) sets; or tick **Make this the repository's default** as you merge with another. Without one, it is the first the repository allows of squash, merge commit and rebase.

<a id="draft-reviews"></a>

## Draft reviews

The agent never posts to GitHub. Its comments go to a draft review that Kero keeps, with `kero pr comment`, which [Kero's skill](./agents.md) teaches it; Kero checks that each comment is on lines of the pull request's diff, the only lines GitHub takes comments on, or on a file the pull request changes. The default prompt has the agent review the whole pull request first, then check each comment it would leave against the code before it adds it.

The comments show on the pull request's **Files** as the agent adds them, marked **Agent**. Edit or delete any of them, or add your own from a line's gutter, as in Changes, or on a whole file with the comment button in its header. The sparkles on an agent's comment type a question about it into the agent's pane, for you to finish. The bar under the pull request has the review's summary, its text on GitHub, and its verdict: **Comment**, **Approve** or **Request Changes** (only Comment on your own pull request). **Post Review** asks first, then posts the summary and the comments as one review, so the author gets one notification. A review needs no comments: approve a pull request, or leave a summary alone, from the bar. The Review list marks each pull request with a draft that has comments or a summary, or whose agent is still at work. **Start Draft**, in the bar, starts a draft with no agent, for an agent of your own to add comments to with `kero pr comment --pr <number>`. **Discard Review…**, in the bar's menu, deletes the draft.

<a id="review-guides"></a>

## Review guides

A pull request's **Guide** tab is a tour of it, for reading it without going line by line: a summary, then stops in reading order, each on the lines worth reading with a note on them. **Write Guide** has an agent write it, in a worktree of the pull request as a review does, asked with `pr-guide-prompt` (Settings › Git); it writes with `kero pr guide`, and nothing of it goes to GitHub. The tab shows one stop at a time, an excerpt of its lines, with **Next Stop** and the list of stops above it; a stop marked **Look closely** has a call only a maintainer can make. **Full Diff** shows the stop's whole file, where you comment as on Files; **Comment** starts a comment on the stop's lines in your draft review, and **Ask Agent** types a question about the stop into the agent's pane. A guide written before the pull request changed says so, with **Rewrite Guide**. **Discard Guide**, in its menu, deletes it; the draft review stays.

<a id="pull-request-notifications"></a>

## Notifications

Kero checks the GitHub repositories of your projects every 5 minutes, and notifies you of each new pull request that asks for your review. Settings › Git sets which pull requests notify you, `pr-notify`: those that ask for your review, every new one someone else opens, or none, which stops checking; and how often Kero checks, `pr-poll-interval`. Click the notification to see the pull request in a session of its project. Kero remembers what it has seen, so a restart doesn't notify you again; the first check of a repository only learns what is open.

<a id="a-session-s-pull-request"></a>

## A session's pull request

A session knows its pull request. Once the session's branch is pushed and has an open pull request on GitHub, Kero links it to the session, at once when you make it with **New Pull Request**; another branch lets it go. You can link one by hand too, which no change of branch replaces: the watch button on a pull request's page links it to the session you're in, or unlinks it, and so do `kero pr link` and `kero pr unlink`. The session's tooltip says how its checks stand, and its menu shows or unlinks it.

While the session is open, Kero watches its pull request, every minute, and wakes the agent in the session with a message, [through its own way in](./agents.md#how-agents-get-keros-messages): when its checks fail, and when it can't merge for conflicts. The message starts "Kero PR watch:". The agent hears once it's at its prompt, never while it works or asks you something. Settings › Git sets what wakes it, `pr-watch-wake`: `all` also wakes it when the checks pass and when comments or reviews come, other than your own; `failures` only for failing checks and conflicts; `off`, nothing. Comments alone wake it 10 times at most for a pull request, so that a bot that answers the agent can't keep it going. In a session with no agent, Kero notifies you instead.

<a id="new-pull-request"></a>

## New pull request

**New Pull Request** makes a pull request of the branch of the pane you're in. The form shows the branch it goes into, the default branch of the push remote's repository at first; click it to choose another branch, or another remote's repository. It lists the default branch first, then the most recently changed, and typing filters the list. Under the title and the description, it lists the commits the pull request brings, and the files they change, each of whose diff a click unfolds. Only commits go in: the form says how many files you haven't committed, and how many commits the push remote doesn't have yet. The sparkles beside the title have the title and the description written, as **Write Message** writes a commit message, by `commit-message-command`. A branch with only one commit takes its subject as the title.

**Create Pull Request** pushes the commits the push remote doesn't have, then creates the pull request with `gh` and opens it. The arrow beside it has **Create as Draft**, and **Show on GitHub**, which opens GitHub's page that creates the pull request, with the title and the description filled in, for you to finish there. If the branch already has an open pull request, the form links to it instead.

<a id="prompts"></a>

## Prompts

Settings › Git has the prompts that tell the agent how to write a commit message and a pull request, `commit-message-prompt` and `pr-message-prompt`. They show Kero's own until you change them, and **Reset to Default** brings those back. The review prompt there, `pr-review-prompt`, is what a review agent is asked, and `pr-guide-prompt` what a guide's agent is. A repository's **Repository Setup…** in the Review tab can replace the first three for that repository; a prompt it leaves as it is follows the one in Settings.

## From the command line

```sh
kero pr list -p api --json          # the repository's open pull requests
kero pr review 123 -p api           # an agent's review in a worktree, as the Review tab starts it
kero pr review 123 -p api --guide   # an agent writing its review guide
kero pr checkout 123 --worktree -- claude
kero pr state 123                   # open, draft, whether it merges, and by which methods
kero pr merge 123 --squash --delete-branch
```

`kero pr review` needs the app open, and Remote Control unless [Ama](./ama.md) runs it. The commands agents use to write draft reviews and guides, and to act on a pull request, are in [Command Line](./cli.md#draft-reviews).
