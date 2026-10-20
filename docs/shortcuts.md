---
title: Keyboard Shortcuts
description: Kero's default keyboard shortcuts and the action ids to rebind them.
---

# Keyboard Shortcuts

Rebind any of these in **Settings › Keyboard**, or with `keybind` in the [configuration file](./configuration.md#keyboard).

## Kero

| Action                       | Shortcut                               | Action id        |
| ---------------------------- | -------------------------------------- | ---------------- |
| Settings…                    | <kbd>⌘</kbd> <kbd>,</kbd>              | `settings.open`  |
| Quit (sessions keep running) | <kbd>⌘</kbd> <kbd>Q</kbd>              |                  |
| Quit and End All Sessions…   | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>Q</kbd> | `app.quitAll`    |

## Session

| Action                      | Shortcut                               | Action id           |
| --------------------------- | -------------------------------------- | ------------------- |
| New Session                 | <kbd>⌘</kbd> <kbd>T</kbd>              | `session.new`       |
| New Session in Home         | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>T</kbd> | `session.newHome`   |
| Open Project…               | <kbd>⌘</kbd> <kbd>O</kbd>              | `project.open`      |
| Go to Session…              | <kbd>⌘</kbd> <kbd>P</kbd>              | `palette.open`      |
| Jump to Session Needing You | <kbd>⌘</kbd> <kbd>J</kbd>              | `session.attention` |
| Next Session                | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>]</kbd> | `session.next`      |
| Previous Session            | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>[</kbd> | `session.previous`  |
| Rename Session              | <kbd>⌘</kbd> <kbd>R</kbd>              | `session.rename`    |
| Restart Pane                | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>R</kbd> | `pane.restart`      |
| Close                       | <kbd>⌘</kbd> <kbd>W</kbd>              | `pane.close`        |
| Close Session               | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>W</kbd> | `session.close`     |

## Pane

| Action        | Shortcut                               | Action id          |
| ------------- | -------------------------------------- | ------------------ |
| Split Right   | <kbd>⌘</kbd> <kbd>D</kbd>              | `pane.split.right` |
| Split Down    | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>D</kbd> | `pane.split.down`  |
| Zoom Pane     | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>↩</kbd> | `pane.zoom`        |
| Next Pane     | <kbd>⌘</kbd> <kbd>]</kbd>              | `pane.next`        |
| Previous Pane | <kbd>⌘</kbd> <kbd>[</kbd>              | `pane.previous`    |
| Pane Left     | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>←</kbd> | `pane.left`        |
| Pane Right    | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>→</kbd> | `pane.right`       |
| Pane Above    | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>↑</kbd> | `pane.up`          |
| Pane Below    | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>↓</kbd> | `pane.down`        |

## View

| Action         | Shortcut                  | Action id        |
| -------------- | ------------------------- | ---------------- |
| Toggle Sidebar | <kbd>⌘</kbd> <kbd>B</kbd> | `sidebar.toggle` |
| Bigger         | <kbd>⌘</kbd> <kbd>=</kbd> | `text.bigger`    |
| Smaller        | <kbd>⌘</kbd> <kbd>-</kbd> | `text.smaller`   |
| Actual Size    | <kbd>⌘</kbd> <kbd>0</kbd> | `text.reset`     |
