---
title: Changes
description: See a session's git branch and what its agent changed, review it, and stage, commit, branch, sync, stash and browse the history beside its panes.
---

# Changes

When a session works in a git repository, Kero shows its branch and what changed, and lets you review an agent's work beside its panes.

## Branches at a glance

Each pane's title shows the branch it works on, with the lines its changes add and remove since the last commit: `fix-login +40 −2`. A merge or rebase in progress shows there too.

In the sidebar, a session on a branch other than the repository's default one, such as an agent's in a worktree of its own, shows that branch by its name. Hover a session for its branch, how far it is ahead of or behind its upstream, and how many files changed.

## The right sidebar

The sidebar at the right of the window has four tabs: **Files** and its **Search** ([Files & Search](./files.md)), **Changes** and **History** (this page), and **Review** ([Pull Requests](./pull-requests.md)). Show or hide it with the button at the top right of the window, or press <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>B</kbd>. Drag its edge to resize it, wider for History's graph, or click **Zoom Sidebar** beside its hide button to hide the panes and give it their room, until you click **Show All Panes**. In a window too narrow for it beside your panes, it gets out of their way, and comes back as you widen the window; the button still shows it when you need it. A file you click in Changes, Files or Search, and a pull request you open in Review, opens in a card beside the panes, the viewer, which you close with its <kbd>×</kbd> or leave with <kbd>Esc</kbd>. The viewer's **Zoom Viewer** button hides the panes and gives the viewer their room, as Zoom Pane does a pane, until you click **Show All Panes**. With `sessions-sidebar = right`, the sessions take the window's right, and the right sidebar and the viewer its left.

## Changes

Click a pane's branch, or press <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>G</kbd>, to show the Changes tab with the diffs beside the panes. It lists the files that changed, in their folders, with what they add and remove, since one of:

- **Last Turn**: where the session's agent last started working, so you see what it did in its latest turn, and nothing that changed before it. Kero takes a snapshot of the worktree as the agent starts, without touching your index or files.
- **Uncommitted**: the last commit, untracked files included.
- **Branch**: where the branch left the repository's default branch, as a pull request would show it.

Click a file in the tab to show the diffs, scrolled to its own. Each file shows with its diff right below it, one after another, and the diffs follow the files while the agent keeps editing them. The file at the top of the diffs is highlighted in the tab. Scrolled into a long diff, its file's name stays at the top. Click a file's name to fold its diff away, or to unfold it: a binary file and one with very many changed lines start folded. **Collapse All** folds them all, and **Expand All** unfolds them. Right-click a file to open it in your editor or in an editor pane, or drag it onto a pane to open it there. Drag the gap between the panes and the diffs to resize them.

Diffs show unified while their card is narrow and side by side once it's wide. The button at the top of the diffs switches between the two, which the `diff-layout` setting keeps. The first button there wraps lines wider than the card rather than scrolling them sideways, as the same button above a file from Files or in an editor pane does, which the `wrap-lines` setting keeps for all of them. In the tab, a folder that holds only another folder shares its row, as `src/app`, and a click folds a folder. In Uncommitted, a folder's buttons stage, unstage or discard the files in it, as a section's do all of its files.

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

The **Worktrees** section, under the stash, lists the repository's worktrees, with what removing each would lose, to push, open or remove them ([Worktrees](./worktrees.md#the-worktrees-section)). **+** starts a session in a new worktree; a commit's menu opens its branch in one, **Open … in New Worktree**.

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
kero do files.show path=src/main.go  # the Files tab, with this file in the viewer (Files & Search)
```

## Turning it off

Set `git-status = false`, or turn off **Show git status** in Settings, to stop Kero running git in your repositories.
