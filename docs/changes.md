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

Click a pane's branch, or the panel button at the window's top right, or press <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>G</kbd>, to open the changes panel at the right of the session. The panel button shows the view the panel showed last, and stays at the window's corner as the panel opens, to close it again. The **Widen** button in the panel's header gives it all the room the panes leave, until you click it again. With `sessions-sidebar = right`, the sessions take the window's right and the panel its left, with its button at the top left. It lists the files that changed, with what each adds and removes, since one of:

- **Last Turn**: where the session's agent last started working, so you see what it did in its latest turn, and nothing that changed before it. Kero takes a snapshot of the worktree as the agent starts, without touching your index or files.
- **Uncommitted**: the last commit, untracked files included.
- **Branch**: where the branch left the repository's default branch, as a pull request would show it.

Each file shows with its diff right below it, one after another, and the diffs follow the files while the agent keeps editing them. Scrolled into a long diff, its file's name stays at the top. Click a file's name to fold its diff away, or to unfold it: a binary file and one with very many changed lines start folded. Drag the panel's edge to resize it.

Diffs show unified while the panel is narrow and side by side once it's wide. The button at the right of the list's summary switches between the two, which the `diff-layout` setting keeps. The wrap button beside it wraps long lines at the panel's width, as it does in the Files view and editor panes.

### Reviewing a turn

In **Last Turn**, hover a change for two buttons:

- **Revert** puts the old lines back in the file.
- **Keep** marks the change as reviewed, and hides it.

Revert works only while no agent in the session is working, so it never races an agent's edits, and only on a file that's still as the diff shows it. A file that changed meanwhile shows anew instead.

<a id="sessions-in-their-own-worktree"></a>

## Sessions in their own worktree

Agents working side by side in one checkout trip over each other's changes. **Session › New Session in Worktree…** (<kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>T</kbd>) asks for a branch name, then starts a session in a git worktree of its own: a folder checking out that new branch, from the repository's default branch.

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

The panel's **Files** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>E</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>E</kbd> on Windows and Linux), shows the files of the worktree of the pane you're in, or of its folder outside a repository. On a remote host, they're that host's.

Click a folder's arrow, or press <kbd>→</kbd> and <kbd>←</kbd>, to open and close it. Click a file to open it below the tree, highlighted as its language is, and edit it there: <kbd>⌘</kbd> <kbd>S</kbd> (<kbd>Ctrl</kbd> <kbd>S</kbd>) saves it, keeping its line endings. A file you edited stays open, marked unsaved, while you look at others. The wrap button above it wraps long lines at the view's width. Images show as pictures; binary files and those over 4 MB don't open. With `files-preview` off, a click only chooses a file, and its menu's **Preview** opens it below the tree. To keep a file beside your terminals, open it in an [editor pane](./panes.md#editor-panes).

The buttons under the tree make a new file or folder in the folder you chose, or in the chosen file's folder: type its name, or a path such as `src/util/new.go`, and press <kbd>Return</kbd> (<kbd>Enter</kbd>), or <kbd>Esc</kbd> to leave it. A new file opens below the tree. The other buttons there refresh the tree and collapse its folders. A folder's menu makes a new file or folder in it, and opens a terminal there.

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
