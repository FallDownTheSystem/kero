---
title: Changes
description: See a session's git branch and what its agent changed, file by file, beside its panes.
---

# Changes

When a session works in a git repository, Kero shows its branch and what changed, and lets you review an agent's work beside its panes.

## Branches at a glance

Each pane's title shows the branch it works on, with the lines its changes add and remove since the last commit: `fix-login +40 −2`. A merge or rebase in progress shows there too.

In the sidebar, a session on a branch other than the repository's default one, such as an agent's in a worktree of its own, shows that branch by its name. Hover a session for its branch, how far it is ahead of or behind its upstream, and how many files changed.

## The right sidebar

The sidebar at the right of the window has four tabs: **Files**, **Changes**, **History** and **Review**. Show or hide it with the button at the top right of the window, or press <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>B</kbd>. Drag its edge to resize it, wider for History's graph, or click **Zoom Sidebar** beside its hide button to hide the panes and give it their room, until you click **Show All Panes**. In a window too narrow for it beside your panes, it gets out of their way, and comes back as you widen the window; the button still shows it when you need it. A file you click in Changes, Files or Search, and a pull request you open in Review, opens in a card beside the panes, the viewer, which you close with its <kbd>×</kbd> or leave with <kbd>Esc</kbd>. The viewer's **Zoom Viewer** button hides the panes and gives the viewer their room, as Zoom Pane does a pane, until you click **Show All Panes**. With `sessions-sidebar = right`, the sessions take the window's right, and the right sidebar and the viewer its left.

## Changes

Click a pane's branch, or press <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>G</kbd>, to show the Changes tab with the diffs beside the panes. It lists the files that changed, in their folders, with what they add and remove, since one of:

- **Last Turn**: where the session's agent last started working, so you see what it did in its latest turn, and nothing that changed before it. Kero takes a snapshot of the worktree as the agent starts, without touching your index or files.
- **Uncommitted**: the last commit, untracked files included.
- **Branch**: where the branch left the repository's default branch, as a pull request would show it.

Click a file in the tab to show the diffs, scrolled to its own. Each file shows with its diff right below it, one after another, and the diffs follow the files while the agent keeps editing them. The file at the top of the diffs is highlighted in the tab. Scrolled into a long diff, its file's name stays at the top. Click a file's name to fold its diff away, or to unfold it: a binary file and one with very many changed lines start folded. **Collapse All** folds them all, and **Expand All** unfolds them. Right-click a file to open it in your editor or in an editor pane, or drag it onto a pane to open it there. Drag the gap between the panes and the diffs to resize them.

Diffs show unified while their card is narrow and side by side once it's wide. The button at the top of the diffs switches between the two, which the `diff-layout` setting keeps. The wrap button beside it wraps long lines at the card's width, as it does for files and in editor panes. In the tab, a folder that holds only another folder shares its row, as `src/app`, and a click folds a folder. In Uncommitted, a folder's buttons stage, unstage or discard the files in it, as a section's do all of its files.

### Reviewing a turn

In **Last Turn**, hover a change for two buttons:

- **Revert** puts the old lines back in the file.
- **Keep** marks the change as reviewed, and hides it.

Revert works only while no agent in the session is working, so it never races an agent's edits, and only on a file that's still as the diff shows it. A file that changed meanwhile shows anew instead.

In **Branch**, a change has **Revert** alone: it puts back the lines the file had where the branch began, as a change you haven't committed yet.

<a id="review-comments"></a>

### Review comments

Comment on an agent's changes as you would on a pull request, then send all of your comments to the agent at once. In **Last Turn**, **Uncommitted** and **Branch**:

- Hover a line's number and click the **+** that shows, to comment on that line. Select lines first by clicking their numbers, and Shift-clicking for a range, to comment on all of them.
- Or hover a change and click **Comment**, for the lines selected of it, or the whole change. This is how to comment on lines that were removed in a unified diff.

Write the comment in the box under the lines, and click **Comment**, or press <kbd>⌘</kbd> <kbd>↩</kbd> (<kbd>Ctrl</kbd> <kbd>Enter</kbd>). <kbd>Esc</kbd> drops it. The comment stays under its lines, to edit or delete.

A band above the files counts the worktree's comments and lists them: click one to open its file at its line. **Send to Agent** pastes the comments in the pane of the agent in the worktree as one prompt, and sends it. Each comment goes with its file, its line numbers and the text of its lines, so the agent finds them even after they moved. Then the comments sent go. **Copy as Prompt** copies the same prompt instead, to give to another agent, and the **×** discards the comments. Comments last until Kero quits.

## Staging and committing

In **Uncommitted**, the files list in two sections: **Changes**, what the next commit doesn't take yet, and below it **Staged Changes**, what it takes. A file you stage moves at once, and the next file comes up where it was. Each file's buttons stage or unstage it, or discard its changes, which asks first: an untracked file is deleted, a deleted one comes back. The buttons on a section's title do the same for all of its files. **Stage All Changes** leaves conflicts out, as staging one marks it resolved.

Right-click a file and choose **Add to .gitignore**, or a folder of the tree and **Add Folder to .gitignore**, to add it to the `.gitignore` at the top of the worktree. Git goes on tracking a file it tracks already, however `.gitignore` lists it: for a tracked file, or a folder with tracked files, Kero asks whether to stop tracking them too. They then leave the repository at the next commit, and stay on disk.

To stage part of a file, hover a change of its diff, and click **Stage**, or **Unstage** in Staged Changes; **Discard** in Changes puts its old lines back in the file. To take some lines of a change alone, click their numbers, Shift-click to select a range, and the change's buttons become **Stage N Lines**, **Unstage N Lines** and **Discard N Lines**. A file a git filter changes as it's stored, as Git LFS does, stages whole.

<a id="committing"></a>

Write the message in the box under the list, and click **Commit**, or press <kbd>⌘</kbd> <kbd>↩</kbd> (<kbd>Ctrl</kbd> <kbd>Enter</kbd>). **Write Message**, the sparkles in the box, has the message written of what's staged: by a fork of the conversation of the agent in the pane, which knows why it made the changes, without changing that conversation; else by a new run of Claude Code or Codex. Set `commit-message-command` to write it with another command, which runs in your shell and gets the prompt on its stdin. `{session}` in it is the ID of the pane's agent's conversation, so your own launcher can fork it: `cc -p --resume {session} --fork-session`. The arrow beside Commit has **Commit and Push**, **Amend Last Commit**, **Sign Off** and **Skip Hooks**.

## Branches, syncing and stashes

The bar above the list names the worktree's branch. Click it for the branch switcher: its groups, **Worktrees**, **Branches** in their folders, **Remotes**, **Tags** and **Stashes**, fold, and the filter above them finds what's in any of them, or names a new branch. Click an entry to show its commit in History's graph, chosen, the switcher staying open; double-click a branch, or press <kbd>Return</kbd> (<kbd>Enter</kbd>), to check it out, or a worktree to open its session. With **Settings › Git › Hide old branches**, branches with no commit in that many weeks wait behind **Show … older**, unless the filter finds them. A branch's menu there switches to it, makes a branch from it, opens it in a session of its own worktree, renames or deletes it, merges it into yours, or rebases yours onto it. Switching with changes not committed asks to stash them first, or to take them along.

The sync button beside it pulls what the branch is behind, then pushes what it's ahead, or publishes a branch with no upstream. The **⋯** menu has the rest:

- **Fetch**, **Fetch from All Remotes** and **Fetch and Prune**.
- **Pull**, as your git configuration says, or merging, rebasing, or fast-forward only.
- **Push**, **Force Push**, which pushes over the remote's branch only as last fetched, and **Push with Tags**. A push is of the branch alone, to where it pushes to.
- **Stash All Changes**, **Stash Staged Changes** and **Stash with Message…**.
- **New Branch…**, **Undo Last Commit**, which keeps its changes staged, and **Refresh**.

To stash some files and keep working on the rest, click a file's, folder's or section's **Add to Stash** button in the Changes tab: they move to a **To Stash** section under the others. Name the stash if you like, and click **Stash N Files**: their changes, staged or not, untracked files too, go into one stash entry, and the other files stay as they are.

A pull, a merge and a rebase stash the changes not committed first, and put them back once they end; when those conflict, they stay in the stash. A fetch, pull or push that needs a password fails with **Run in Terminal**, which runs the same command in a terminal where git can ask for it.

An operation that changes the files while an agent works in the worktree, as a pull or a discard, asks first: **When It's Done** runs it once the agent finishes, **Now** at once. Staging, committing and fetching don't ask.

## History

The right sidebar's **History** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>Y</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Y</kbd> on Windows and Linux), draws the worktree's commits as a graph: every branch's with **All**, or the branch's, its upstream's and the default branch's with **Branch**. Local branches and the remotes' show apart, so a branch and its remote's at one commit show both. Commits not pushed yet are marked. **Filter** finds commits by their message, author or hash; a file's menu in the Files tab shows the history of that file.

The **Worktrees** section, under the stash, lists the repository's worktrees, each with what removing it would lose: changed files and commits on no remote in orange, or *merged* or *pushed* in green when nothing would be lost. **Push** pushes a worktree's branch, **Open** shows the session working in it or starts one, and **Remove Worktree** asks first: a worktree with changes is removed only when you say **Remove Anyway**, one with commits not pushed keeps its branch, and one with nothing to lose can go with its branch, **Remove with Branch**. A worktree a session was made for closes with the session. Its menu also copies its path, merges its branch into yours, and **Prune Missing Worktrees** forgets those whose folders were deleted. **+** starts a session in a new worktree; a commit's menu opens its branch in one, **Open … in New Worktree**.

A commit's branches, tags and stashes show beside it, in its line's color, each kind with its icon: a branch, a remote's branch (a globe), a branch checked out in another worktree (a folder), a tag, a stash. The branch you're on is bold, with an outline. A stash's menu applies, pops or drops it; a commit whose branch is checked out in another worktree offers **Open Worktree** rather than checking it out. Click a commit to see its files, in their folders, and a file to open the commit's diffs in the viewer, scrolled to it; as you scroll there, the file at the top shows as chosen in the list. A file's menu opens it, as the Changes tab's does, or shows its history with **Show File History**. The changes not committed show on top, and the stash under them, to apply, pop or drop. A commit's menu:

- **Copy Hash** and **Copy Branch Name**.
- **Check Out**: the branch at the commit, or the commit itself, detached, when no branch is there. A remote's branch checks out as your branch of its name, brought up to the remote's first when it is behind; when each has commits the other lacks, switch to yours and pull.
- **New Branch Here…** and **New Session in Worktree Here…**.
- **Merge … into …**, with a merge commit too, **Rebase … onto …**, and **Interactively Rebase … onto …**.
- **Cherry-Pick**, **Revert Commit**, and **Reset to This Commit**: soft, keeping the changes since staged, mixed, keeping them unstaged, or hard, discarding them.

### Interactive rebase

**Interactively Rebase … onto …**, from a commit's menu or a branch's, lists the commits your branch has since that commit, the newest on top. Set what happens to each from its menu, or with a key: <kbd>P</kbd> **Pick** keeps it, <kbd>R</kbd> **Reword** gives it a new message, <kbd>E</kbd> **Edit** stops there so you can change it, <kbd>S</kbd> **Squash** and <kbd>F</kbd> **Fixup** combine it with the commit below it, keeping both messages or only that one's, and <kbd>D</kbd> **Drop** removes it. Drag a commit, or press <kbd>Alt</kbd> <kbd>↑</kbd> and <kbd>Alt</kbd> <kbd>↓</kbd>, to move it. A reworded commit opens a panel under it for its new subject and description. Below the list, **Commit** and **Changes** show the chosen commit's message and files.

**Back up the current state to a temporary branch** first makes a branch where yours is now, to go back to. **Rebase** stashes the changes not committed, as a rebase does, and replays the commits as listed. It stops at a commit to edit, or a conflict, like any rebase: **Continue** goes on. A range with merge commits can't be rebased this way, as it would be flattened.

## Conflicts

A merge, rebase, cherry-pick or revert that stops on conflicts says so in the bar and over the commit box, and lists each conflicted file in Changes. Resolve a file in your editor or a terminal and click **Mark Resolved**, or take one side's version of it from its menu: **Use main's Version** or **Use feature's Version**, named after the branches, which asks first, as edits made to the file are lost.

**Open in Merge Editor**, in the same menu, opens the file in an editor pane as the merge editor: the two versions side by side on top, lined up conflict by conflict, and the file below them, where you can edit it. Check **Use main's lines** or **Use feature's lines** above a conflict to put that version's lines in the file; check both for both, in the order you check them. The pane's header counts the conflicts left and steps between them, and its menu uses one version's lines in all of them, or shows the file as plain text. **Save and Mark Resolved**, once none is left, saves the file and stages it. Any editor pane of a file in conflict shows it this way, until it is marked resolved. **Resolve with Agent**, on a file or over the commit box for all of them, asks the agent working in the worktree to resolve them and stage them, leaving the commit to you.

Once none is left, **Continue** goes on with the rebase, cherry-pick or revert, and **Commit Merge** commits a merge, with the message git prepared. **Abort** gives the operation up, back to where it started, after asking.

<a id="sessions-in-their-own-worktree"></a>

## Sessions in their own worktree

Agents working side by side in one checkout trip over each other's changes. **Session › New Session in Worktree…** (<kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>T</kbd>) opens the [New Session dialog](./sessions.md#new-session-dialog) with **In a worktree** on, to start a session in a git worktree of its own: a folder checking out the branch you pick, or a new branch you name, from the repository's default branch. A project's menu in the sidebar has it too, for a session of that project's repository. On Windows and Linux, which show no menu bar, the menu bar's menus are in the sidebar's **Menus** button (⋯), beside Settings.

- The worktree goes in `.kero/worktrees/<branch>` inside the repository, which Kero ignores through `.git/info/exclude`, not your `.gitignore`. Set `worktree-dir` to put worktrees elsewhere.
- Before making the worktree, Kero fetches the base branch and starts from the remote's copy, so a stale local `main` doesn't matter. Offline, or when the local branch has commits the remote lacks, it starts from the local branch. Set `worktree-from-origin = false` to always use the local branch.
- Set `worktree-base = current` to make a new branch from the project folder's branch instead of the default branch.
- A new worktree has only the files git tracks. To bring along ignored ones, such as `.claude/settings.local.json`, `CLAUDE.local.md` or `.env`, list them in a `.worktreeinclude` at the top of the repository, in `.gitignore`'s syntax, as Claude Code's worktrees read it. Kero copies each ignored file that matches, unless the worktree has it already. It skips `node_modules` and other worktrees, and goes into a wholly ignored folder only for a pattern that names a path in it, such as `cache/keep.txt`.
- To set a new worktree up, as installing its packages, give the repository a `kero.json` with a script that runs as a worktree is made. Kero reads t3code's `t3.json` too, which has the same form, and reads either from the worktree first, then from the main checkout:

  ```json
  {
    "scripts": [
      { "name": "Install", "command": "bun install", "runOnWorktreeCreate": true }
    ]
  }
  ```

  The first script with `runOnWorktreeCreate` runs in a terminal below the session's own, with `KERO_WORKTREE_PATH`, `KERO_PROJECT_ROOT` (the main checkout) and `KERO_WORKTREE_BRANCH` set. The agent starts beside it right away, unless the script says `"async": false`, which has the agent wait for the script to end. The terminal closes once the script succeeds, and stays with its output when it fails. Kero checks out the worktree's submodules first, when it has some. A card at the window's bottom right shows each step as it goes; **Cancel** stops the script.
- The session joins the repository's project, and the sidebar shows its branch.

Closing the session asks whether its worktree goes too:

- **Nothing uncommitted:** Remove Worktree is the default. The branch goes with it once it's merged; otherwise it stays, with its commits.
- **Uncommitted changes:** Kero says how many files would be lost, and Keep Worktree is the default.
- **Another session works in the worktree:** it stays, and Kero doesn't ask.

From the command line:

```sh
kero new --worktree fix-login -- claude   # a session on a new branch, in its own worktree
kero new --worktree fix-login --base dev  # branch from dev
kero close fix-login --remove-worktree    # close it and remove the worktree
kero close fix-login --remove-worktree --force   # even with uncommitted changes
```

## Files

The right sidebar's **Files** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>E</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>E</kbd> on Windows and Linux), shows the files of the worktree of the pane you're in, or of its folder outside a repository. On a remote host, they're that host's. What git ignores shows faded, and `.git` doesn't show. Dotfiles show unless `files-hidden` is off.

Click a folder to unfold it, and a file to open it in the viewer beside the panes, or move in the tree with the keys: <kbd>↑</kbd> and <kbd>↓</kbd>, <kbd>→</kbd> and <kbd>←</kbd> to open and close a folder, <kbd>Return</kbd> (<kbd>Enter</kbd>) to open a file, and <kbd>⌘</kbd> <kbd>↩</kbd> (<kbd>Ctrl</kbd> <kbd>Enter</kbd>) to open it in an editor pane. A file opens highlighted as its language is, where you can find (<kbd>⌘</kbd> <kbd>F</kbd>) in it and edit it. The file follows its changes while you read it. The viewer shows one file at a time: opening another in its place asks whether to save your changes first. Its buttons make the text smaller or bigger and wrap long lines, and for a Markdown file, **Preview Markdown** shows it as it reads. Images show as pictures; binary files and those over 4 MB don't open. **Open in Editor** opens the file in your editor, and **Open in Pane** in an [editor pane](./panes.md#editor-panes), to keep it beside your terminals.

The buttons above the tree make a new file or folder at its top, search the files' text, refresh the tree and collapse its folders. A file's or folder's menu (right-click) makes one in its folder, renames it, or deletes it. Type the name where it will show, and press <kbd>Return</kbd> (<kbd>Enter</kbd>): a name like `docs/notes.md` makes the folders in it too, and a new file opens in the viewer. Delete asks first, and moves what you delete to the Trash. On a remote host, which has no Trash to move it to, it's deleted for good.

The **Filter** field under them finds files and folders by name in all of the folder, its closed folders too, and shows them in the folders they're in. Type a slash to match paths instead, as `src/main`. Folders git ignores, such as `node_modules`, are found by their name, but not searched. <kbd>↓</kbd> or <kbd>Return</kbd> (<kbd>Enter</kbd>) opens the first file found, and <kbd>Esc</kbd> clears the filter.

A file's menu also opens it in an editor pane of its own, and shows its history; a folder's opens a terminal there. Both open in your editor, with another app on Windows, or in File Explorer or Finder, and copy the path, or the path from the worktree. Drag a file onto a pane to open it in an editor pane beside it, or in an editor pane's place. Your editor is the `editor` setting, as `code` or `zed`, or else the app the system opens the file with.

### Editing

A file you open from Files, an editor pane's file, and the new side of a diff in Changes edit as in VS Code or Zed. Press <kbd>⌘</kbd> <kbd>S</kbd> (<kbd>Ctrl</kbd> <kbd>S</kbd>) to save the file you're in, keeping its line endings. Until you do, a dot takes the place of the viewer's <kbd>×</kbd>, and of a diff's file name. Closing the viewer, or opening another file in its place, asks whether to save your changes first. Quitting Kero doesn't: your unsaved changes are still there, unsaved, when you open it again, with the files they're in.

While you haven't changed a file, it follows its changes on disk. Once you have, your version stays, and if an agent or another program changed the file meanwhile, saving asks whether to overwrite it or to discard your changes for the file as it is now.

### Go to File

**Session › Go to File…** (<kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>N</kbd>, <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>N</kbd> on Windows and Linux) finds a file of the worktree of the pane you're in by its name, as you type its letters in order: `usvc` finds `user_service.go`. Names match before paths; type a slash to match paths alone, as `src/main`. <kbd>Return</kbd> (<kbd>Enter</kbd>) opens the file in the viewer, as a click on it in Files does, and <kbd>⌘</kbd> <kbd>↩</kbd> (<kbd>Ctrl</kbd> <kbd>Enter</kbd>) in an editor pane of its own. In a repository, it lists the files git tracks and those it would, so what `.gitignore` lists, such as `node_modules`, stays out.

## Search

**Search**, from the Files tab's search button or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>F</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>S</kbd> on Windows and Linux), searches the text of the files the Files tab shows, on the host they're on, in the right sidebar. It searches as you stop typing, of any case, and <kbd>Return</kbd> (<kbd>Enter</kbd>) searches again, as the files change. The buttons beside the field match case, match whole words, and take the text as a regular expression, as Go writes them.

The lines found list under their files, the text found highlighted: click a file to fold it, and a line to open its file at that line in an editor pane, or in the editor pane that shows it already. Right-click a file or a line to open it in your editor or in an editor pane, or drag it onto a pane to open it there, at the line. Files that git ignores, binary files, and files over 2 MB aren't searched, and a search shows its first 2,000 lines.

<a id="review"></a>

## Review

The right sidebar's **Review** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>P</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>K</kbd> on Windows and Linux), lists the open pull requests of the repository of the pane you're in, on GitHub. It reads them with the [GitHub CLI](https://cli.github.com) on the host the repository is on, so `gh` must be installed and signed in there. The list is of the remote the branch pushes to, `origin` most often; the repository's name above the list chooses another of its remotes on GitHub. **Open**, **Mine** and **To Review** filter the list, which shows each pull request's branches, checks and review.

Click a pull request to see it in the viewer beside the panes; the arrow beside it opens it on GitHub. **Conversation** shows its description, each reviewer and how their review stands (approved, changes requested, or awaiting review), its checks, which a click lists, its comments and reviews, and its threads of comments on lines and files, replies set apart under the comment they answer: click a thread's place to see it in **Files**. **Files** shows its changes, fetched with its base, each file's diff with the comments on its lines under them, and those on the whole file above it. A thread whose lines changed since shows in the conversation alone, marked outdated. **Resolve Conversation** resolves a thread on GitHub, which then folds to a line. The arrows beside the tabs go from comment to comment, yours and those posted. When a worktree of the repository has the pull request's head checked out, as a review's worktree has, right-click a file to open it, or drag it onto a pane; otherwise its files aren't on disk, and don't open.

**Review** on a pull request has an agent review it. A click fetches the pull request's branch and starts the agent in a new session, in a [worktree of its own](#sessions-in-their-own-worktree) of that branch. Each review checks the pull request out on a branch of its own, `review/pr-<number>`, then `review/pr-<number>-2` and on, so you can review it more than once, and while your checkout or another worktree has its branch. Pushing that branch, from the Changes tab, goes to the pull request's branch; with git, run `git push origin HEAD:<branch>`. Closing the review with its worktree deletes its branch once the pull request's has its commits. Right-click it, or the pull request, to send the review to the agent in the pane you're in, or to start the agent in a new session in the pane's folder. Before the review starts, its prompt shows for you to edit. A new session's agent gets the prompt as its command's argument, so it starts on the review at once, with nothing typed into it; the agent in the pane you're in gets it pasted.
A session that reviews a pull request is marked **Review** in the sidebar, and listed under **Review Agents**, above the pull requests, with its status, as the sidebar shows it, and its agent's last message. Click it to go to its session; its menu shows the pull request, or closes the session, which asks whether its worktree goes with it. A pull request with a review agent has **Agent** in place of **Review**, on its row and its page, which goes to the agent. A review whose session closed and kept its worktree stays in the list, to review again in that worktree, or to remove it: **Remove Worktree…** asks first, and says how many changed files it would lose. Reviewing a pull request that has a review already offers its agent, and to review in its worktree rather than a new one.

**Repository Setup…**, the gear, sets the agent command and the review prompt for each repository, as your repositories have different skills and tools: `/code-review high {ticket}`, for one. In the prompt, `{number}`, `{url}`, `{title}`, `{branch}`, `{base}` and `{author}` become the pull request's, and `{ticket}` the first issue key, such as `PROJ-123`, in its title, its description or its branch; when none has one, the review asks you for it. The command is `claude` unless you set another. Kero gives it the prompt as one argument, after the command, as Claude Code and Codex take a first prompt; put `{prompt}` where the command takes it otherwise, as `codex exec {prompt} --full-auto`. The command takes the pull request's placeholders too.

<a id="draft-reviews"></a>

### Draft reviews

The agent never posts to GitHub. Its comments go to a draft review that Kero keeps, with `kero pr comment`, which [Kero's skill](./agents.md) teaches it; Kero checks that each comment is on lines of the pull request's diff, the only lines GitHub takes comments on, or on a file the pull request changes. The default prompt has the agent review the whole pull request first, then check each comment it would leave against the code before it adds it.

The comments show on the pull request's **Files** as the agent adds them, marked **Agent**. Edit or delete any of them, or add your own from a line's gutter, as in Changes, or on a whole file with the comment button in its header. The sparkles on an agent's comment type a question about it into the agent's pane, for you to finish. The bar under the pull request has the review's summary, its text on GitHub, and its verdict: **Comment**, **Approve** or **Request Changes** (only Comment on your own pull request). **Post Review** asks first, then posts the summary and the comments as one review, so the author gets one notification. A review needs no comments: approve a pull request, or leave a summary alone, from the bar. The Review list marks each pull request with a draft that has comments or a summary, or whose agent is still at work. **Start Draft**, in the bar, starts a draft with no agent, for an agent of your own to add comments to with `kero pr comment --pr <number>`. **Discard Review…**, in the bar's menu, deletes the draft.

<a id="pull-request-notifications"></a>

### Notifications

Kero checks the GitHub repositories of your projects every 5 minutes, and notifies you of each new pull request that asks for your review. Settings › Git sets which pull requests notify you, `pr-notify`: those that ask for your review, every new one someone else opens, or none, which stops checking; and how often Kero checks, `pr-poll-interval`. Click the notification to see the pull request in a session of its project. Kero remembers what it has seen, so a restart doesn't notify you again; the first check of a repository only learns what is open.

<a id="new-pull-request"></a>

### New pull request

**New Pull Request** makes a pull request of the branch of the pane you're in. The form shows the branch it goes into, the default branch of the push remote's repository at first; click it to choose another branch, or another remote's repository. It lists the default branch first, then the most recently changed, and typing filters the list. Under the title and the description, it lists the commits the pull request brings, and the files they change, each of whose diff a click unfolds. Only commits go in: the form says how many files you haven't committed, and how many commits the push remote doesn't have yet. The sparkles beside the title have the title and the description written, as **Write Message** writes a commit message, by `commit-message-command`. A branch with only one commit takes its subject as the title.

**Create Pull Request** pushes the commits the push remote doesn't have, then creates the pull request with `gh` and opens it. The arrow beside it has **Create as Draft**, and **Show on GitHub**, which opens GitHub's page that creates the pull request, with the title and the description filled in, for you to finish there. If the branch already has an open pull request, the form links to it instead.

Settings › Git has the prompts that tell the agent how to write a commit message and a pull request, `commit-message-prompt` and `pr-message-prompt`. They show Kero's own until you change them, and **Reset to Default** brings those back. The review prompt there, `pr-review-prompt`, is what a review agent is asked. A repository's **Repository Setup…** in the Review tab can replace all three for that repository; a prompt it leaves as it is follows the one in Settings.

## From the command line

`kero diff` prints a pane's changes as `git diff` does, untracked files included:

```sh
kero diff                       # this pane's uncommitted changes
kero diff --turn                # what its agent changed in its last turn
kero diff review --branch       # the review session's changes since the default branch
kero diff src/auth              # just these paths
```

`kero status --json` and `kero ls --json` include each pane's branch and changes as `git`.

`kero do` shows the right sidebar's tabs, as an agent shows you what it did. They need Remote Control, in **Settings › Agents**:

```sh
kero do changes.show                 # the Changes tab
kero do files.show                   # the Files tab
kero do files.show path=src/main.go  # it, with this file in the viewer, or this folder open
kero do search.show query=TODO       # Search, searching for TODO
```

## Turning it off

Set `git-status = false`, or turn off **Show git status** in Settings, to stop Kero running git in your repositories.
