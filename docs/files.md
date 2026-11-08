---
title: Files & Search
description: Browse, open, edit and search the files of a session's worktree in the right sidebar.
---

# Files & Search

The right sidebar's **Files** tab and **Search** show the files of the worktree of the pane you're in, or of its folder outside a repository. On a remote host, they're that host's. See [The right sidebar](./changes.md#the-right-sidebar) for showing, resizing and zooming it.

## Files

The **Files** tab, or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>E</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>E</kbd> on Windows and Linux), shows the worktree's files as a tree. What git ignores shows faded, and `.git` doesn't show. Dotfiles show unless `files-hidden` is off.

Click a folder to unfold it, and a file to open it in the viewer beside the panes, or move in the tree with the keys: <kbd>↑</kbd> and <kbd>↓</kbd>, <kbd>→</kbd> and <kbd>←</kbd> to open and close a folder, <kbd>Return</kbd> (<kbd>Enter</kbd>) to open a file, and <kbd>⌘</kbd> <kbd>↩</kbd> (<kbd>Ctrl</kbd> <kbd>Enter</kbd>) to open it in an editor pane. A file opens highlighted as its language is, where you can find (<kbd>⌘</kbd> <kbd>F</kbd>) in it and edit it. The file follows its changes while you read it. The viewer shows one file at a time: opening another in its place asks whether to save your changes first. Its buttons make the text smaller or bigger and wrap long lines, and for a Markdown file, **Preview Markdown** shows it as it reads. Images show as pictures; binary files and those over 4 MB don't open. **Open in Editor** opens the file in your editor, and **Open in Pane** in an [editor pane](./panes.md#editor-panes), to keep it beside your terminals.

The buttons above the tree make a new file or folder at its top, search the files' text, refresh the tree and collapse its folders. A file's or folder's menu (right-click) makes one in its folder, renames it, or deletes it. Type the name where it will show, and press <kbd>Return</kbd> (<kbd>Enter</kbd>): a name like `docs/notes.md` makes the folders in it too, and a new file opens in the viewer. Delete asks first, and moves what you delete to the Trash. On a remote host, which has no Trash to move it to, it's deleted for good.

The **Filter** field under them finds files and folders by name in all of the folder, its closed folders too, and shows them in the folders they're in. Type a slash to match paths instead, as `src/main`. Folders git ignores, such as `node_modules`, are found by their name, but not searched. <kbd>↓</kbd> or <kbd>Return</kbd> (<kbd>Enter</kbd>) opens the first file found, and <kbd>Esc</kbd> clears the filter.

A file's menu also opens it in an editor pane of its own, and shows its history; a folder's opens a terminal there. Both open in your editor, with another app on Windows, or in File Explorer or Finder, and copy the path, or the path from the worktree. Drag a file onto a pane to open it in an editor pane beside it, or in an editor pane's place. Your editor is the `editor` setting, as `code` or `zed`, or else the app the system opens the file with.

<a id="editing"></a>

## Editing

A file you open from Files, an editor pane's file, and the new side of a diff in Changes edit as in VS Code or Zed. Press <kbd>⌘</kbd> <kbd>S</kbd> (<kbd>Ctrl</kbd> <kbd>S</kbd>) to save the file you're in, keeping its line endings. Until you do, a dot takes the place of the viewer's <kbd>×</kbd>, and of a diff's file name. Closing the viewer, or opening another file in its place, asks whether to save your changes first. Quitting Kero doesn't: your unsaved changes are still there, unsaved, when you open it again, with the files they're in.

While you haven't changed a file, it follows its changes on disk. Once you have, your version stays, and if an agent or another program changed the file meanwhile, saving asks whether to overwrite it or to discard your changes for the file as it is now.

## Go to File

**Session › Go to File…** (<kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>N</kbd>, <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>N</kbd> on Windows and Linux) finds a file of the worktree of the pane you're in by its name, as you type its letters in order: `usvc` finds `user_service.go`. Names match before paths; type a slash to match paths alone, as `src/main`. <kbd>Return</kbd> (<kbd>Enter</kbd>) opens the file in the viewer, as a click on it in Files does, and <kbd>⌘</kbd> <kbd>↩</kbd> (<kbd>Ctrl</kbd> <kbd>Enter</kbd>) in an editor pane of its own. In a repository, it lists the files git tracks and those it would, so what `.gitignore` lists, such as `node_modules`, stays out.

## Search

**Search**, from the Files tab's search button or <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>F</kbd> (<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>S</kbd> on Windows and Linux), searches the text of the files the Files tab shows, on the host they're on, in the right sidebar. It searches as you stop typing, of any case, and <kbd>Return</kbd> (<kbd>Enter</kbd>) searches again, as the files change. The buttons beside the field match case, match whole words, and take the text as a regular expression, as Go writes them.

The lines found list under their files, the text found highlighted: click a file to fold it, and a line to open its file at that line in an editor pane, or in the editor pane that shows it already. Right-click a file or a line to open it in your editor or in an editor pane, or drag it onto a pane to open it there, at the line. Files that git ignores, binary files, and files over 2 MB aren't searched, and a search shows its first 2,000 lines.

## From the command line

`kero do` shows them, as an agent shows you what it did. It needs Remote Control, in **Settings › Agents**:

```sh
kero do files.show                   # the Files tab
kero do files.show path=src/main.go  # it, with this file in the viewer, or this folder open
kero do search.show query=TODO       # Search, searching for TODO
kero split --file src/main.go        # an editor pane of a file
```
