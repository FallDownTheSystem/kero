---
title: Changes
description: See a session's git branch and what its agent changed, file by file, beside its panes.
---

# Changes

When a session works in a git repository, Kero shows its branch and what changed, and lets you review an agent's work beside its panes.

## Branches at a glance

Each pane's title shows the branch it works on, with the lines its changes add and remove since the last commit: `fix-login +40 −2`. A merge or rebase in progress shows there too.

In the sidebar, a session on a branch other than the repository's default one, such as an agent's in a worktree of its own, shows that branch by its name. Hover a session for its branch, how far it is ahead of or behind its upstream, and how many files changed.

## The changes panel

Click a pane's branch, or the Explorer button at the window's top right, or press <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>G</kbd>, to open the Explorer on the changes, at the right of the session. The Explorer button shows the view the Explorer showed last, and stays at the window's corner as the Explorer opens, to close it again. The **Widen Explorer** button in the Explorer's header gives it all the room the panes leave, until you click it again. With `sessions-sidebar = right`, the sessions take the window's right and the Explorer its left, with its button at the top left. It lists the files that changed, with what each adds and removes, since one of:

- **Last Turn**: where the session's agent last started working, so you see what it did in its latest turn, and nothing that changed before it. Kero takes a snapshot of the worktree as the agent starts, without touching your index or files.
- **Uncommitted**: the last commit, untracked files included.
- **Branch**: where the branch left the repository's default branch, as a pull request would show it.

Each file shows with its diff right below it, one after another, and the diffs follow the files while the agent keeps editing them. Scrolled into a long diff, its file's name stays at the top. Files start folded, as a list of what changed: click a file's name to unfold its diff, or to fold it away. The button beside the list's summary unfolds all the files, or folds them all while one is unfolded; in **Uncommitted**, each section's title has its own. Drag the panel's edge to resize it.

Diffs show unified while the panel is narrow and side by side once it's wide. The button at the right of the list's summary switches between the two, which the `diff-layout` setting keeps. The wrap button beside it wraps long lines at the panel's width, as it does in the Files view and editor panes. **Tree View** beside that shows the files in their folders: click a folder to fold it. A folder that holds only another folder shares its row, as `src/app`.

### Reviewing a turn

In **Last Turn**, hover a change for two buttons:

- **Revert** puts the old lines back in the file.
- **Keep** marks the change as reviewed, and hides it.

Revert works only while no agent in the session is working, so it never races an agent's edits, and only on a file that's still as the diff shows it. A file that changed meanwhile shows anew instead.

## Staging and committing

In **Uncommitted**, the files list in two sections: **Staged Changes**, what the next commit takes, and **Changes**, what it doesn't yet. Each file's buttons stage or unstage it, or discard its changes, which asks first: an untracked file is deleted, a deleted one comes back. The buttons on a section's title do the same for all of its files. **Stage All Changes** leaves conflicts out, as staging one marks it resolved.

Right-click a file and choose **Add to .gitignore**, or a folder of the tree and **Add Folder to .gitignore**, to add it to the `.gitignore` at the top of the worktree. Git goes on tracking a file it tracks already, however `.gitignore` lists it: for a tracked file, or a folder with tracked files, Kero asks whether to stop tracking them too. They then leave the repository at the next commit, and stay on disk.

To stage part of a file, hover a change of its diff, and click **Stage**, or **Unstage** in Staged Changes; **Discard** in Changes puts its old lines back in the file. To take some lines of a change alone, click their numbers, Shift-click to select a range, and the change's buttons become **Stage N Lines**, **Unstage N Lines** and **Discard N Lines**. A file a git filter changes as it's stored, as Git LFS does, stages whole.

<a id="committing"></a>

Write the message in the box under the list, and click **Commit**, or press <kbd>⌘</kbd> <kbd>↩</kbd> (<kbd>Ctrl</kbd> <kbd>Enter</kbd>). **Write Message**, the sparkles in the box, has the message written of what's staged: by a fork of the conversation of the agent in the pane, which knows why it made the changes, without changing that conversation; else by a new run of Claude Code or Codex. Set `commit-message-command` to write it with another command, which runs in your shell and gets the prompt on its stdin. `{session}` in it is the ID of the pane's agent's conversation, so your own launcher can fork it: `cc -p --resume {session} --fork-session`. The arrow beside Commit has **Commit and Push**, **Amend Last Commit**, **Sign Off** and **Skip Hooks**.

## Branches, syncing and stashes

The bar above the list names the worktree's branch. Click it to switch branches: type to filter them, or to name a new branch, and press <kbd>Return</kbd> (<kbd>Enter</kbd>). A branch's menu there switches to it, makes a branch from it, opens it in a session of its own worktree, renames or deletes it, merges it into yours, or rebases yours onto it. Switching with changes not committed asks to stash them first, or to take them along.

The sync button beside it pulls what the branch is behind, then pushes what it's ahead, or publishes a branch with no upstream. The **⋯** menu has the rest:

- **Fetch**, **Fetch from All Remotes** and **Fetch and Prune**.
- **Pull**, as your git configuration says, or merging, rebasing, or fast-forward only.
- **Push**, **Force Push**, which pushes over the remote's branch only as last fetched, and **Push with Tags**. A push is of the branch alone, to where it pushes to.
- **Stash All Changes**, **Stash Staged Changes** and **Stash with Message…**.
- **New Branch…**, **Undo Last Commit**, which keeps its changes staged, and **Refresh**.

A pull, a merge and a rebase stash the changes not committed first, and put them back once they end; when those conflict, they stay in the stash. A fetch, pull or push that needs a password fails with **Run in Terminal**, which runs the same command in a terminal where git can ask for it.

An operation that changes the files while an agent works in the worktree, as a pull or a discard, asks first: **When It's Done** runs it once the agent finishes, **Now** at once. Staging, committing and fetching don't ask.

## History

The Explorer's **History** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>Y</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Y</kbd> on Windows and Linux), draws the worktree's commits as a graph: every branch's with **All**, or the branch's, its upstream's and the default branch's with **Branch**. Local branches and the remotes' show apart, so a branch and its remote's at one commit show both. Commits not pushed yet are marked. **Filter** finds commits by their message, author or hash; a file's menu in the Files view shows the history of that file.

Click a commit to see its files, and a file to see its diff. The changes not committed show on top, and the stash under them, to apply, pop or drop. A commit's menu:

- **Copy Hash** and **Copy Branch Name**.
- **Check Out**: the branch at the commit, or the commit itself, detached, when no branch is there.
- **New Branch Here…** and **New Session in Worktree Here…**.
- **Merge … into …**, with a merge commit too, **Rebase … onto …**, and **Interactively Rebase … onto …**.
- **Cherry-Pick**, **Revert Commit**, and **Reset to This Commit**: soft, keeping the changes since staged, mixed, keeping them unstaged, or hard, discarding them.

### Interactive rebase

**Interactively Rebase … onto …**, from a commit's menu or a branch's, lists the commits your branch has since that commit, the newest on top. Set what happens to each from its menu, or with a key: <kbd>P</kbd> **Pick** keeps it, <kbd>R</kbd> **Reword** gives it a new message, <kbd>E</kbd> **Edit** stops there so you can change it, <kbd>S</kbd> **Squash** and <kbd>F</kbd> **Fixup** combine it with the commit below it, keeping both messages or only that one's, and <kbd>D</kbd> **Drop** removes it. Drag a commit, or press <kbd>Alt</kbd> <kbd>↑</kbd> and <kbd>Alt</kbd> <kbd>↓</kbd>, to move it. A reworded commit opens a panel under it for its new subject and description. Below the list, **Commit** and **Changes** show the chosen commit's message and files.

**Back up the current state to a temporary branch** first makes a branch where yours is now, to go back to. **Rebase** stashes the changes not committed, as a rebase does, and replays the commits as listed. It stops at a commit to edit, or a conflict, like any rebase: **Continue** goes on. A range with merge commits can't be rebased this way, as it would be flattened.

## Conflicts

A merge, rebase, cherry-pick or revert that stops on conflicts says so in the bar and over the commit box, and lists each conflicted file in Changes. Resolve a file in your editor or a terminal and click **Mark Resolved**, or take one side's version of it from its menu: **Use main's Version** or **Use feature's Version**, named after the branches, which asks first, as edits made to the file are lost.

**Open in Merge Editor**, in the same menu, opens the file in an editor pane as the merge editor: the two versions side by side on top, lined up conflict by conflict, and the file below them, where you can edit it. Check **Use main's lines** or **Use feature's lines** above a conflict to put that version's lines in the file; check both for both, in the order you check them. The bar under it counts the conflicts left and steps between them, and its menu uses one version's lines in all of them, or shows the file as plain text. **Save and Mark Resolved**, once none is left, saves the file and stages it. Any editor pane of a file in conflict shows it this way, until it is marked resolved. **Resolve with Agent**, on a file or over the commit box for all of them, asks the agent working in the worktree to resolve them and stage them, leaving the commit to you.

Once none is left, **Continue** goes on with the rebase, cherry-pick or revert, and **Commit Merge** commits a merge, with the message git prepared. **Abort** gives the operation up, back to where it started, after asking.

<a id="sessions-in-their-own-worktree"></a>

## Sessions in their own worktree

Agents working side by side in one checkout trip over each other's changes. **Session › New Session in Worktree…** (<kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>T</kbd>) asks for a branch name, then starts a session in a git worktree of its own: a folder checking out that new branch, from the repository's default branch. A project's menu in the sidebar has it too, for a session of that project's repository. On Windows and Linux, which show no menu bar, the menu bar's menus are in the sidebar's **Menus** button (⋯), beside Settings.

- The worktree goes in `.kero/worktrees/<branch>` inside the repository, which Kero ignores through `.git/info/exclude`, not your `.gitignore`. Set `worktree-dir` to put worktrees elsewhere.
- Set `worktree-base = current` to branch from the session you're in instead of the default branch.
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

The Explorer's **Files** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>E</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>E</kbd> on Windows and Linux), shows the files of the worktree of the pane you're in, or of its folder outside a repository. On a remote host, they're that host's.

Click a folder, or press <kbd>→</kbd> and <kbd>←</kbd>, to open and close it. Click a file to open it below the tree, highlighted as its language is, and edit it there: <kbd>⌘</kbd> <kbd>S</kbd> (<kbd>Ctrl</kbd> <kbd>S</kbd>) saves it, keeping its line endings. A file you edited stays open, marked unsaved, while you look at others. The wrap button above it wraps long lines at the view's width, and for a Markdown file, **Preview Markdown** shows it as it reads. Images show as pictures; binary files and those over 4 MB don't open. With `files-preview` off, a click only chooses a file, and its menu's **Preview** opens it below the tree. To keep a file beside your terminals, open it in an [editor pane](./panes.md#editor-panes).

The buttons under the tree make a new file or folder in the folder you chose, or in the chosen file's folder: type its name, or a path such as `src/util/new.go`, and press <kbd>Return</kbd> (<kbd>Enter</kbd>), or <kbd>Esc</kbd> to leave it. A new file opens below the tree. The other buttons there refresh the tree and collapse its folders. A folder's menu makes a new file or folder in it, and opens a terminal there.

The **Filter** field between them finds files and folders by name in all of the folder, its closed folders too, and shows them in the folders they're in. Type a slash to match paths instead, as `src/main`. Folders git ignores, such as `node_modules`, are found by their name, but not searched. <kbd>↓</kbd> or <kbd>Return</kbd> (<kbd>Enter</kbd>) goes to the first file found, and <kbd>Esc</kbd> clears the filter, showing the file you chose in the whole tree.

A save writes the file only as it was when you opened it. When an agent or another program changed it since, Kero says so instead, and you **Reload** it, dropping your edits, or **Overwrite** it. While the file has no edits of yours, it follows the changes on disk.

Double-click a file, or right-click it, to open it in your editor: the `editor` setting, as `code` or `zed`, or the app the system opens the file with. With `files-double-click = pane`, a double click opens it in an editor pane instead. Dotfiles show unless `files-hidden` is off. A folder's menu opens the folder in your editor too, or in the app the system opens text files with. The menu also opens it in a pane, or with another app, shows it in File Explorer or Finder, and copies its path, or its path from the worktree.

## From the command line

`kero diff` prints a pane's changes as `git diff` does, untracked files included:

```sh
kero diff                       # this pane's uncommitted changes
kero diff --turn                # what its agent changed in its last turn
kero diff review --branch       # the review session's changes since the default branch
kero diff src/auth              # just these paths
```

`kero status --json` and `kero ls --json` include each pane's branch and changes as `git`.

`kero do` shows the panel's views, as an agent shows you what it did. They need Remote Control, in **Settings › Agents**:

```sh
kero do changes.show                 # the Changes view
kero do files.show                   # the Files view
kero do files.show path=src/main.go  # it, with this file open, or this folder
```

## Turning it off

Set `git-status = false`, or turn off **Show git status** in Settings, to stop Kero running git in your repositories.
