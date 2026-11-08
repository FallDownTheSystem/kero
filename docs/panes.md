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

In the sidebar, a split session's row shows how many panes it has, after its name. Click the count to list the panes under the row, each with its program, its name, its branch or folder when it differs, and its status; it tells which pane needs you. Click a pane there to go to it, or right-click it to zoom or close it. Click the count again to fold them back.

When a pane's program exits, the pane stays so you can read its output. **Restart Pane** starts its shell again in the same directory.

To show an agent what a terminal printed, such as a failing test's output, select the text, right-click it and choose **Send to Session…**, then the agent's session. The agent gets the text as a code block, with the pane it came from and what it runs (an agent such as Claude Code, a program, or the shell, such as pwsh), typed into its prompt once it is at it.

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
| `-u`, `--url <url>`                   | Open a browser showing the page instead of a shell; a path to a local file shows that file |
| `--file <path>`                       | Open an editor of the file instead of a shell  |
| `--preview`                           | With `--file` of a Markdown file, show it as it reads rather than its text |

Run inside Kero, `kero split` splits the pane it runs in. Elsewhere, name a target: `kero split my-session --down`.

A file, or a local page, that a pane of the session already shows isn't split again: `kero split` focuses that pane instead, with `--preview` its preview, prints its id, and says so on stderr. An agent showing you the plan it rewrote finds it where it was.

## Editor panes

An editor pane shows a file beside your terminals, highlighted as its language is, to read and edit: <kbd>⌘</kbd> <kbd>S</kbd> (<kbd>Ctrl</kbd> <kbd>S</kbd>) saves it, only as it was when it was read, as in the [viewer](./changes.md#editing). It splits, zooms, moves and closes like any pane, and comes back on the same file when Kero restarts. Closing it with edits not saved asks first.

Open one from the Files tab: right-click a file and choose **Open in Pane**, or click **Open in Pane** in the viewer's header while it shows the file. Or drag a file onto a pane: dropped near an edge, it opens beside the pane on that side; dropped in the middle of an editor pane, it takes the place of its file, unless that one has unsaved edits. Files and folders dragged from your file manager open the same way, a folder in a terminal there; on a terminal, only near its left, right or top edge, as the rest of it pastes their paths. From a shell, or an agent:

```sh
kero split --file src/main.go
kero new --file src/main.go     # in a session of its own
```

A file that doesn't exist yet opens empty and marked new, as long as its folder exists. Saving creates it.

Right-click a pane's split button, right or down, and choose **New File** for an empty `untitled.txt` in the pane's folder. Until you type in it, **Open File…** in its header opens a file of your choice in its place.

Right-click the file's name at the top of the pane to copy its path, whole or relative to the session's folder, open it in your editor or another app, show it in your file manager, or see its history.

Its header has the file's buttons, as the viewer's does: save, wrap long lines, and open it in your editor, with a dot while it has unsaved edits. In a pane too narrow for them, they're in its **File Actions** menu. A Markdown file has **Preview Markdown** too, which shows it as it reads, its images beside it included, until you click it again to edit its text; **Pane › Toggle Markdown Preview** does the same for the pane you're in. The pane keeps its preview when Kero restarts. **Smaller Text** and **Bigger Text** zoom the text of every file, and their Markdown previews, apart from the terminals'; with a file focused, <kbd>⌘</kbd> <kbd>=</kbd>, <kbd>⌘</kbd> <kbd>-</kbd> and <kbd>⌘</kbd> <kbd>0</kbd> (<kbd>Ctrl</kbd> on Windows and Linux) do the same.

An agent opens a Markdown file previewed, or turns its preview on or off:

```sh
kero split --file plans/plan.md --preview
kero do pane.preview off
```

The pane follows its file: when another program, such as an agent, writes it, the pane shows what it wrote, unless you have unsaved edits in it, which stay; saving them then asks whether to overwrite.

## Browser panes

A browser pane shows a web page beside your terminals: the dev server an agent is working on, the docs it reads, the pull request it opened. It splits, zooms, moves and closes like any pane, and comes back on the same page when Kero restarts.

| Action           | Shortcut                               |
| ---------------- | -------------------------------------- |
| New browser pane | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>B</kbd> |
| Open location    | <kbd>⌘</kbd> <kbd>L</kbd>              |
| Pick elements    | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd> |

Right-click a pane's split button and choose **Browser** to open one beside that pane. Type an address in its bar, such as `localhost:3000` or `github.com`, or words to search for. An address typed in the command palette (<kbd>⌘</kbd> <kbd>P</kbd>) opens in a new browser pane too. <kbd>⌘</kbd> <kbd>F</kbd> finds text in the page.

From a shell, or an agent, open one with `--url`:

```sh
kero split --url localhost:5173
kero new --url https://github.com/egoist/kero/pulls
kero split --url .backlog/pages/brief.html
```

A path to a file of this machine, relative to the folder you run `kero split` in, or a `file:///` URL, shows a local page: its pictures and stylesheets beside it load from the disk, and scripts from CDNs from the web. As the file changes on disk, the page reloads. A script of the page's own folder loaded as a module (`<script type="module">`), or fetched, doesn't load from a local page: browsers allow those only from a server.

An agent can then use the page as you would, with `kero browser`: click, type, read its text, wait for it and take a screenshot. See [Command Line](./cli.md#browsing).

```sh
kero browser type 'input[name=q]' "kero" --enter
kero browser text main
```

### Sending to an agent from a page

A local page (`file:`) in a browser pane can send a message to an agent, such as the choices you made on a decision brief the agent showed you. The page calls `window.kero.send(message, options)`; Kero types the message into the prompt of the agent whose pane opened the page with `kero split --url` (or showed it again), once the agent is at its prompt, or hands it to the agent while it waits with [`kero browser wait-message`](./cli.md#browsing). A page no agent opened asks you which agent to send to, as **Send to Session…** does, and keeps sending there.

Each message tells the agent where it came from: `Kero: message from database pick, from the page "Pick and send" (file:///…/send-to-agent.html) in browser pane 3f2a:`, then its text. The promise `send` returns resolves once the agent has the message, so the page can say it's sent, and rejects if the agent's pane closes first.

| Option    | What it does |
| --------- | ------------ |
| `label`   | What the message is, before the page's title and URL |
| `mode`    | `"push"` always types it into the prompt; `"wait"` keeps it for `kero browser wait-message` alone; unset, it goes to an agent waiting for it, else into the prompt |
| `onState` | Hears `"queued"` (the agent is working: it gets it at its prompt) or `"waiting"` (kept for `wait-message`) before the promise resolves |

A message that isn't a string is sent as JSON. `window.kero` is there only in Kero, so a page tells by it whether it can send, and copies the message to the clipboard where it can't:

```html
<!doctype html>
<meta charset="utf-8">
<title>Pick and send</title>
<h1>Which database?</h1>
<label><input type="radio" name="d1" value="A" checked> A: SQLite</label>
<label><input type="radio" name="d1" value="B"> B: Postgres</label>
<p><button id="send">Send</button> <span id="status"></span></p>
<script>
const send = document.getElementById("send");
const status = document.getElementById("status");
send.onclick = async () => {
  const text = "D1: " + document.querySelector('input[name="d1"]:checked').value;
  if (!window.kero) {
    await navigator.clipboard.writeText(text);
    status.textContent = "Copied: paste it to the agent";
    return;
  }
  status.textContent = "Sending…";
  try {
    await window.kero.send(text, {
      label: "database pick",
      onState: (state) => {
        status.textContent = state === "queued" ? "Queued: the agent is working" : "Waiting for the agent";
      },
    });
    status.textContent = "Sent";
  } catch (err) {
    status.textContent = "Not sent: " + err.message;
  }
};
</script>
```

A `kero:delivery` event on `window` tells each message's delivery too, with its `id`, `state` (`delivered` or `failed`) and `error`.

### Picking elements for an agent

To show an agent what to change on a page, turn on **Pick Elements**: the pointer-in-a-box button in the pane's toolbar, <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd> (<kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>C</kbd> on Windows and Linux), or the Pane menu. Click an element, write what you want of it in the note that opens, and pick more; moving the pointer while you write leaves the note as it is. **Parent** picks the element around it: point at it to see what it would pick, outlined dashed. **Send**, or <kbd>Ctrl</kbd> <kbd>Enter</kbd>, sends them all; <kbd>Esc</kbd> stops.

The picks go where the page's own messages go ([Sending to an agent from a page](#sending-to-an-agent-from-a-page)): to the agent that opened the page with `kero split --url`, typed into its prompt once it is at it, or to an agent waiting for the page's messages. For a page no agent opened, Kero asks you which agent, once, and remembers; close the list without choosing and Kero copies the picks for you to paste.

A CSS selector rarely leads an agent to the source, so each element comes with what does, strongest first:

1. **Where it is in the source**, when the page can tell: the file and line of Svelte's and React 18's development builds, the line React 19's stack leads to through the source map, the component's file for Vue, with the line of its tag in it, and the attributes inspector plugins add, such as `data-v-inspector`. Kero checks a line against the file and corrects it when the tag is elsewhere. For a page of a local file, it is the line of the element's start tag in the file. Without either, the message says there is none, so the agent goes straight to the hints.
2. **Search hints**: the strings likely written in the source as they are, to grep for: the class attribute, the id, test id, label, name, link and placeholder, and a phrase or two of the element's own text, 4 to 8 words that end at a word and leave out characters a source may write as entities, such as ’ and —; each is marked when no other element on the page has it.
3. **Where it is on the page**: the landmark and heading above it and which of its like it is ("in `<main>`, below the heading "Pricing", the 2nd of 3 `div.card`"), its parent and siblings, and its HTML, cut short. A `<script>`, `<style>`, `<svg>`, `<template>` or `<noscript>` there keeps its tag and attributes, its contents shown as their size, such as `/* 1.2 KB omitted */`.
4. Its selector and DOM path, the page's URL and viewport, its main computed styles, and a picture of it, boxed.

The pictures are PNG files in the `kero-picks` folder of the temporary folder, removed a day later, which the message names, so an agent in a terminal opens them by their paths. **Screenshots** in the picker's bar takes two: **Elements**, a picture of each element with a margin around it, boxed (one you scrolled away from is scrolled back into sight for it, and the page back after), and **Page**, the page as you see it, every element boxed and numbered as the message numbers them. Both are on; Kero remembers what you turn off for your next picks.

The component chain comes from development builds only: a production build tells the hints and the rest.

### Blocking ads

Browser panes block ads and trackers with filter lists, [EasyList](https://easylist.to) and EasyPrivacy unless you choose others. Kero downloads them the first time a browser pane opens and refreshes them every week. Pages that open before the lists are ready get them as soon as they are, from their next load on.

Choose the lists under **Block ads** in **Settings › Browser**: lists of cookie notices, social buttons, annoyances and Chinese sites' ads are there too. A list of another URL goes in the [configuration](./configuration.md#browser), as `browser-filter-list`. More lists hide more, and now and then break a page.

Kero blocks what WebKit's content blockers can: requests to ad and tracker servers, and the elements the lists hide. Filters that need a blocker's own scripts, such as those that hide elements by their text or rewrite what a page loads, are left out. A page on a domain the lists block doesn't open in a browser pane.

To browse without blocking, turn off **Block ads** in **Settings › Browser**, or set `browser-block-ads = false` in the [configuration](./configuration.md#browser). Open pages keep what they loaded until they reload.

### Signing in with your browser's cookies

Browser panes keep cookies of their own, so a site you're signed in to in Chrome or Edge asks you to sign in again in a pane. Rather than sign in to each, give the panes your browser's cookies:

1. In Chrome (144 or later) or Edge, open `chrome://inspect/#remote-debugging` (`edge://inspect/#remote-debugging` in Edge) and turn on remote debugging. It lasts until the browser quits.
2. In **Settings › Browser**, under **Sign-ins**, click **Import** beside the browser, or run `kero browser cookies import chrome` (or `edge`).
3. The browser asks whether to allow the connection: allow it. The dialog can show in any of the browser's windows, including an installed app's (PWA), such as a desktop app you added from a site. Look there if no regular window shows it. The import is the same either way: an installed app runs in the browser and its profile.

Kero copies the cookies of the browser's profile it deems the default, which signs the panes in where you are there, and leaves the browser as it was: it reads the cookies and disconnects.

Many web apps keep their sign-in in the site's localStorage or IndexedDB rather than in cookies. Kero copies those too, of the sites of the browser's open tabs, as the browser tells a site's storage only to a page of it: open such an app in the browser before you import, then reload its pane. `kero browser cookies import chrome --domain app.example.com` copies one site's, and opens a tab of it, unseen, for as long as it reads when none is open. A site's databases replace the pane's of the same name; Kero leaves out a site's databases past 8 MB, and values the browser keeps for itself, such as a site's encryption keys, which no page can copy, and tells you what it left out. Import again when a sign-in expires. Panes keep the cookies when Kero restarts, and every `kero browser` command works on the pages as before.

Chrome and Edge don't let other programs open your profile or read its cookie files, and ignore `--remote-debugging-port` on it: the switch in `chrome://inspect` is the one way in, and asks you each time. Cookies sign you in: agents in your sessions can use the pages, and `kero browser cookies save`, as you can.

### Driving panes with DevTools tools

On Windows, browser panes can open a DevTools port, the one Chrome opens with `--remote-debugging-port`, for tools built on the DevTools protocol: [Playwright](https://playwright.dev) with `connectOverCDP`, [chrome-devtools-mcp](https://github.com/ChromeDevTools/chrome-devtools-mcp), Puppeteer and Lighthouse. Turn on **DevTools port** in **Settings › Browser**, or set `browser-devtools = true` in the [configuration](./configuration.md#browser), then quit Kero and start it again: the panes' browser opens the port as it starts.

Every pane is a page of the one browser, so the panes share one port, `http://127.0.0.1:9222` unless `browser-devtools-port` says another. A tool picks a pane by its URL or title. `kero browser devtools` prints the port and each pane's page there:

```sh
$ kero browser devtools
http://127.0.0.1:9222
3f2a	8C1D…	http://localhost:5173/	Vite App
9b41	E07A…	https://github.com/egoist/kero	GitHub - egoist/kero
```

`--json` prints `endpoint`, `port`, `browser` (the browser's WebSocket URL), and for each pane its `pane`, `session`, `url`, `title`, `target` and `webSocketDebuggerUrl`. A pane that hasn't shown its page yet has no target: show it once.

With Playwright, connect to the port and take the pane's page by its URL:

```js
import { chromium } from "playwright";

const browser = await chromium.connectOverCDP("http://127.0.0.1:9222");
const page = browser.contexts()[0].pages().find((p) => p.url().startsWith("http://localhost:5173"));
await page.getByRole("button", { name: "Add" }).click();
await browser.close(); // disconnects; the panes stay open
```

For chrome-devtools-mcp, give the agent the port as `--browser-url`, with Claude Code or in another agent's MCP settings:

```sh
claude mcp add chrome-devtools -- npx chrome-devtools-mcp@latest --browser-url=http://127.0.0.1:9222
```

```json
{ "mcpServers": { "chrome-devtools": { "command": "npx", "args": ["chrome-devtools-mcp@latest", "--browser-url=http://127.0.0.1:9222"] } } }
```

The agent then lists the panes' pages and selects one by its URL or title. Open panes in Kero, as with `kero split --url`: a tool's own new tabs or windows aren't panes, and WebView2 may refuse them.

The port listens on `127.0.0.1` alone, but any program on this computer can then use the panes' pages, signed in as they are, without asking: turn it on while you use it. The port is Windows only: the system's web view on macOS and Linux opens none, and there `kero browser` drives the panes.
