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

Click a pane's branch or the diff button in its header, or press <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>G</kbd>, to open the changes panel at the right of the session. It lists the files that changed, with what each adds and removes, since one of:

- **Last Turn**: where the session's agent last started working, so you see what it did in its latest turn, and nothing that changed before it. Kero takes a snapshot of the worktree as the agent starts, without touching your index or files.
- **Uncommitted**: the last commit, untracked files included.
- **Branch**: where the branch left the repository's default branch, as a pull request would show it.

Each file shows with its diff right below it, one after another, and the diffs follow the files while the agent keeps editing them. Scrolled into a long diff, its file's name stays at the top. Click a file's name to fold its diff away, or to unfold it: a binary file and one with very many changed lines start folded. Drag the panel's edge to resize it.

Diffs show unified while the panel is narrow and side by side once it's wide. The button at the right of the list's summary switches between the two, which the `diff-layout` setting keeps.

### Reviewing a turn

In **Last Turn**, hover a change for two buttons:

- **Revert** puts the old lines back in the file.
- **Keep** marks the change as reviewed, and hides it.

Revert works only while no agent in the session is working, so it never races an agent's edits, and only on a file that's still as the diff shows it. A file that changed meanwhile shows anew instead.

## Files

The folder button in a pane's header, the panel's **Files** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>E</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>E</kbd> on Windows and Linux), shows the files of the worktree of the pane you're in, or of its folder outside a repository. On a remote host, they're that host's.

Click a folder's arrow, or press <kbd>→</kbd> and <kbd>←</kbd>, to open and close it. Click a file to open it below the tree, highlighted as its language is, and edit it there: <kbd>⌘</kbd> <kbd>S</kbd> (<kbd>Ctrl</kbd> <kbd>S</kbd>) saves it, keeping its line endings. A file you edited stays open, marked unsaved, while you look at others. The wrap button above it wraps long lines at the view's width. Images show as pictures; binary files and those over 4 MB don't open. To keep a file beside your terminals, open it in an [editor pane](./panes.md#editor-panes).

A save writes the file only as it was when you opened it. When an agent or another program changed it since, Kero says so instead, and you **Reload** it, dropping your edits, or **Overwrite** it. While the file has no edits of yours, it follows the changes on disk.

Double-click a file, or right-click it, to open it in your editor: the `editor` setting, as `code` or `zed`, or the app the system opens the file with. The menu also opens it in a pane, or with another app, shows it in File Explorer or Finder, and copies its path, or its path from the worktree.

## From the command line

`kero diff` prints a pane's changes as `git diff` does, untracked files included:

```sh
kero diff                       # this pane's uncommitted changes
kero diff --turn                # what its agent changed in its last turn
kero diff review --branch       # the review session's changes since the default branch
kero diff src/auth              # just these paths
```

`kero status --json` and `kero ls --json` include each pane's branch and changes as `git`.

## Turning it off

Set `git-status = false`, or turn off **Show git status** in Settings, to stop Kero running git in your repositories.
