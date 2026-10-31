---
title: Panes
description: Split a session into panes to run a dev server, tests or a second agent side by side.
---

# Panes

Split a session to run things side by side: an agent on the left, its dev server on the right, a test watcher below.

| Action             | Shortcut                                          |
| ------------------ | ------------------------------------------------- |
| Split right        | <kbd>⌘</kbd> <kbd>D</kbd>                         |
| Split down         | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>D</kbd>            |
| Zoom pane          | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>↩</kbd>            |
| Next / previous pane | <kbd>⌘</kbd> <kbd>]</kbd> / <kbd>[</kbd>        |
| Move between panes | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>←</kbd> <kbd>→</kbd> <kbd>↑</kbd> <kbd>↓</kbd> |
| Restart pane       | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>R</kbd>            |
| Close pane         | <kbd>⌘</kbd> <kbd>W</kbd>                         |

**Zoom** fills the session with one pane; press it again to bring the others back.

When a pane's program exits, the pane stays so you can read its output. **Restart Pane** starts its shell again in the same directory.

## From the command line

`kero split` starts a pane beside another and prints its id:

```sh
kero split --down -- npm run dev
kero split --right --ratio 0.3 -- go test ./... -count=1
```

| Flag                                  | Meaning                                        |
| ------------------------------------- | ---------------------------------------------- |
| `--right` (default), `--down`, `--left`, `--up` | Where the new pane goes               |
| `-r`, `--ratio <0..1>`                | The new pane's share of the space (default 0.5) |
| `-d`, `--dir <dir>`                   | Start it in another directory                  |
| `-w`, `--wait`                        | Wait for its command to end, and exit with its status |
| `-u`, `--url <url>`                   | Open a browser showing the page instead of a shell |
| `--file <path>`                       | Open an editor of the file instead of a shell  |

Run inside Kero, `kero split` splits the pane it runs in. Elsewhere, name a target: `kero split my-session --down`.

## Editor panes

An editor pane shows a file beside your terminals, highlighted as its language is, to read and edit: <kbd>⌘</kbd> <kbd>S</kbd> (<kbd>Ctrl</kbd> <kbd>S</kbd>) saves it, only as it was when it was read, as in the [Files view](./changes.md#files). It splits, zooms, moves and closes like any pane, and comes back on the same file when Kero restarts. Closing it with edits not saved asks first.

Open one from the Files view: right-click a file and choose **Open in Pane**, or click the split button above a file. Or drag a file onto a pane: dropped near an edge, it opens beside the pane on that side; dropped in the middle of an editor pane, it takes the place of its file, unless that one has unsaved edits. Files and folders dragged from your file manager open the same way, a folder in a terminal there; on a terminal, only near its left, right or top edge, as the rest of it pastes their paths. From a shell, or an agent:

```sh
kero split --file src/main.go
kero new --file src/main.go     # in a session of its own
```

A file that isn't there yet opens empty, marked new, in a folder that is: saving it makes it.

Right-click a pane's split button, right or down, and choose **New File** for an empty `untitled.txt` in the pane's folder. Until you type in it, its bar's **Open File…** opens a file of your choice in its place. With `files-pane = replace`, a file opened in a pane takes the place of the one in the editor pane you were in last, unless that one has unsaved edits; a file's **Open in New Pane** still opens it in a pane of its own.

Right-click the file's name at the top of the pane to copy its path, whole or relative to the session's folder, open it in your editor or another app, show it in your file manager, or see its history.

The bar below the file says whether it's new or unsaved, and has its buttons: save, wrap long lines, and open it in your editor. A Markdown file has **Preview Markdown** too, which shows it as it reads, its images beside it included, until you click it again to edit its text. **Smaller Text** and **Bigger Text** zoom the text of every file, and their Markdown previews, apart from the terminals'; with a file focused, <kbd>⌘</kbd> <kbd>=</kbd>, <kbd>⌘</kbd> <kbd>-</kbd> and <kbd>⌘</kbd> <kbd>0</kbd> (<kbd>Ctrl</kbd> on Windows and Linux) do the same.

## Browser panes

A browser pane shows a web page beside your terminals: the dev server an agent is working on, the docs it reads, the pull request it opened. It splits, zooms, moves and closes like any pane, and comes back on the same page when Kero restarts.

| Action           | Shortcut                               |
| ---------------- | -------------------------------------- |
| New browser pane | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>B</kbd> |
| Open location    | <kbd>⌘</kbd> <kbd>L</kbd>              |

Right-click a pane's split button and choose **Browser** to open one beside that pane. Type an address in its bar, such as `localhost:3000` or `github.com`, or words to search for. An address typed in the command palette (<kbd>⌘</kbd> <kbd>P</kbd>) opens in a new browser pane too. <kbd>⌘</kbd> <kbd>F</kbd> finds text in the page.

From a shell, or an agent, open one with `--url`:

```sh
kero split --url localhost:5173
kero new --url https://github.com/egoist/kero/pulls
```

An agent can then use the page as you would, with `kero browser`: click, type, read its text, wait for it and take a screenshot. See [Command Line](./cli.md#browsing).

```sh
kero browser type 'input[name=q]' "kero" --enter
kero browser text main
```

### Blocking ads

Browser panes block ads and trackers with filter lists, [EasyList](https://easylist.to) and EasyPrivacy unless you choose others. Kero downloads them the first time a browser pane opens and refreshes them every week. Pages that open before the lists are ready get them as soon as they are, from their next load on.

Choose the lists under **Block ads** in **Settings › Browser**: lists of cookie notices, social buttons, annoyances and Chinese sites' ads are there too. A list of another URL goes in the [configuration](./configuration.md#browser), as `browser-filter-list`. More lists hide more, and now and then break a page.

Kero blocks what WebKit's content blockers can: requests to ad and tracker servers, and the elements the lists hide. Filters that need a blocker's own scripts, such as those that hide elements by their text or rewrite what a page loads, are left out. A page on a domain the lists block doesn't open in a browser pane.

To browse without blocking, turn off **Block ads** in **Settings › Browser**, or set `browser-block-ads = false` in the [configuration](./configuration.md#browser). Open pages keep what they loaded until they reload.
