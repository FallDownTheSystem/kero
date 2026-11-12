# Fork changelog

The release notes of the fork's own releases (FORK.md), which its update
window shows. Each version is upstream's, X.Y.Z, with what that version
brings in [upstream's changelog](CHANGELOG.md), and the fork's changes on
top of it.

## 2.1.35-fork.13

Built on MyGo 0.9.0, MyGo's latest release, past the commit Kero 2.1.35
requires, with the fork's MyGo changes on top. It brings MyGo's changes
since then, among them highlighting of Svelte, HCL/Terraform, ERB,
Jinja and Nix files and of scripts by their `#!` line; the update window
offering a release that came out while it was open; a transparent
WebView background and the shadow of borderless windows fixed on
Windows; Ctrl+I, Ctrl+M and Ctrl+[ sending Tab, Enter and Esc in the
terminal; and a program killed in the terminal no longer leaving mouse
and key reporting on.

- **Diffs that aren't edited end where git's do**: a staged change's
  diff, a commit's and History's no longer show the empty line after a
  file's last newline. A last line without a newline is marked **No
  newline at end of file**, and a line that gained or lost its newline
  shows as that line changed, not as an empty line added or deleted
- Unstaging chosen lines of a staged change keeps each line's own
  ending in these diffs, and a file's last line its missing one

## 2.1.35-fork.12

- A pull request's review session is named after the pull request's
  number and title, `#42 Fix the login redirect`, not its number alone,
  and its **Review** mark sits at the right of its row, under the time
  in two-line rows

## 2.1.35-fork.11

- **Claude Code acts on your page answers again**: an answer you send
  from a page, **Send to Session…** and a review's **Send to Agent**
  reached Claude Code as a background event, which Claude would not take
  as your words, and it refused to act on your "accept". What you send
  yourself is now typed into Claude's prompt as yours. Orchestrators'
  reports and PR watch messages still arrive as events
- **Orchestrators work in a new way**: an orchestrator gives each piece of
  work to a subagent of its own or to a child session, whichever fits.
  It tells each child to show its decisions to you as a page in the
  child's own session, and to report when a decision waits. The
  orchestrator then tells you only which session waits, and does not
  copy the decisions into its own chat
- **Agents can change any setting, the prompts too**: `kero config get
  --default <key>` prints Kero's own prompt, `--text` reads a prompt as
  its lines, and `kero config set --file <path> <key>` sets it from a
  file. `--repo owner/name` reads or sets one repository's value, such
  as its review prompt, and keeps the other repositories' values
- Ama answers in a cheerful, quirky way

## 2.1.35-fork.10

- **Kero's messages no longer type over what you are writing**: an
  orchestrator's reports, PR watch messages and a page's answers reach
  the agent through its own way in, as a turn of its own, and your
  half-written prompt stays as it is. Claude Code gets them through
  Kero's plugin, Codex through `codex queue`, OpenCode through its
  service, and Pi and OMP through Kero's extension
- **Press Update in Settings, or run `kero skill setup`, once** to add
  Kero's plugin to Claude Code. `kero skill status` shows it as missing
  until then, and messages are typed in as before
- A long message reaches Claude Code as its start and a file with the
  whole text, which Claude reads
- Where a message is still typed in, as for an agent with no way in,
  Kero stops pressing Enter for it once you type, so it never sends your
  own text with it
- **New Session with no worktree starts in the project's checkout**:
  from a session in a worktree, the dialog with the worktree switch off
  started the new session in that worktree. It now starts it in the same
  folder of the main checkout

## 2.1.35-fork.9

- **An orchestrator hears of a child's turn when it ends**: Kero's hook
  in Claude Code and Codex now runs at the end of each turn too, and
  tells Kero the turn's end, its reply and what still runs in the
  background, so that a report no longer waits on the child's screen and
  transcript, nor reports a compaction as a turn. The Pi, OMP and OpenCode
  plugins tell it as well
- A turn that fails, as on an API error, is reported with its error, and
  a Codex or Pi turn you interrupt as interrupted
- An interrupted Claude Code turn, or an agent with no hook, is reported
  from its screen, as before
- **Run `kero skill setup` once** to add the new hooks: `kero skill status`
  shows a Claude Code or Codex hook of an older Kero as outdated. The hooks
  run in the background and print nothing, so they never hold an agent up

## 2.1.35-fork.8

- **An orchestrator hears what its child said**: when a child of Claude
  Code, Codex or Pi finishes its turn, its report carries the reply it
  ended the turn with, so that the orchestrator answers a question at
  once, without reading the child's screen. A long reply comes as its
  first and last thousand characters, and `kero read <child> --reply`
  prints it whole
- The report says what the child left running in the background, as "1
  shell, 1 monitor still running", so that the orchestrator knows it isn't
  done
- No report for a turn with no new reply, as one that only compacted the
  conversation, nor for one the child ended with its own
  `kero report --to-parent`
- Children are told that their last reply goes to their orchestrator, so
  they end a turn with their result or their question
- **`kero prompt` sends a long text whole**: it pastes the text, so that
  its lines reach an agent as one prompt, instead of typing them one by
  one, each line's end an Enter that sent only the last line. It presses
  Enter once the agent has the text, and its timeout says that the prompt
  went and the agent is still working
- `kero read --reply` prints an agent's last reply, whole, from its
  transcript

## 2.1.35-fork.7

- **Deleting Ama's chats**: History in Ama's window has Delete Chat, to
  delete one past chat, and Delete All Chats…, to delete all but the one
  running, each after a confirmation. A deleted chat leaves Ama's
  History; its agent keeps its own record of it
- Ama's window no longer has Clear, which cleared the screen of a
  full-screen agent and so did nothing
- Ama's header has room between the frog, Ama's name and its status
- The frog's dot for an answer you haven't read is the theme's green, not
  grey on the grey frog
- A confirmation in a small window, as Ama's can be, keeps a margin
  around it

## 2.1.35-fork.6

- **Ama knows Kero's docs**: Kero writes its docs into Ama's folder, in
  `docs/`, with an index of them in its CLAUDE.md and AGENTS.md, so that
  Ama reads the page it needs before it answers, as how setup scripts in
  `kero.json` work, instead of searching
- **New pages in the docs**: Worktrees (with setup scripts), Files &
  Search, Pull Requests, Orchestrators, Ama, The Window and
  Troubleshooting, in English, Japanese and Chinese, and fixes to the
  others: `kero daemon stop` saves the sessions, `kero do` runs actions,
  the keybind syntax, the environment that Kero gives agents
- **Ama's icon** is the frog of Kero's logo, solid with its face, belly
  and feet, as large as the icons beside it

## 2.1.35-fork.5

- **Ama's role prompt** is a setting: Settings › Agents › Ama › Role
  prompt (`ama-prompt`) shows Kero's, which you can change, and Reset to
  Default brings back. Kero writes it into the CLAUDE.md and AGENTS.md of
  Ama's folder, which its agent reads from its next chat
- **Arguments for Ama and orchestrators**, whichever agent they run:
  Settings › Agents › Ama › Arguments (`ama-agent-args`) and Orchestrators
  › Arguments (`orchestrator-agent-args`), as `--model opus`, go after the
  agent's command as it starts and as it resumes, so that one agent serves
  all three
- The picker's screenshots box an element at the page's top or side, as
  example.com's `<body>` at 0, with every line: a side at the picture's
  edge has its line inside the element, and its number, in the page's
  screenshot, inside too
- Importing sign-ins says that the browser may ask in any of its windows,
  an installed app's (PWA) too, often the one used last. Kero connects to
  the browser, whose installed apps share its profile, and imports the
  same cookies whichever window asks

## 2.1.35-fork.4

- **Ama's window**: Ama opens in a window of its own, above Kero's, which
  you move by its header, to another display too, and resize by its edges.
  Kero remembers its size and its place by the nearest corner of Kero's
  window, in DIPs, so that it keeps its size on displays of other scales:
  it goes with Kero's window as that moves or resizes, and stays inside a
  display as Kero's window nears a screen's edge or a display goes away.
  Right-click the frog, or Settings › Agents › Ama › Window, to put it back
  above the frog
- Ama's text has a size of its own, a point under the terminals' unless
  Settings › Agents › Ama › Text size (`ama-font-size`) says otherwise
- Claude Code's status line is off in Ama's window
  (`ama-hide-status-line`, on): Kero puts an empty one in the
  `.claude/settings.json` of Ama's folder, beside what you put there, and
  leaves your own Claude Code settings alone. Claude Code has no way to
  remove it but turning off every hook, Kero's among them, so an empty row
  stays where it was
- With detailed sidebar rows, an orchestrator's mark and the chevron that
  folds its children sit at the right end of its second line, under the
  time and status, so its name keeps the first line; compact rows keep
  them by the name
- An orchestrator's role prompt, and every report Kero sends it, tell it
  to answer a report that needs nothing with a plain OK, not a summary
- A child closed while its orchestrator was busy no longer reaches it as
  "finished its turn: it is idle" afterwards. Kero handed a report to the
  agent's mailbox as soon as it was due, where it waited, already
  written, for the orchestrator's turn to end, and a close never reached
  it. Reports now wait in Kero until the orchestrator is at its prompt,
  and a child that closes meanwhile takes its state lines (idle, waiting,
  failed, exited) out of them: they would have the orchestrator act on a
  session that is gone, and "closed" says all that is left. What it
  wrote with kero report --to-parent still goes, as that is its result
- The picker's HTML of an element, its parent and its siblings shows a
  `<script>`, `<style>`, inline `<svg>`, `<template>` or `<noscript>` as
  its tag and attributes with its contents' size, such as
  `<style data-vite-dev-id="…">/* 1.2 KB omitted */</style>`, rather
  than the code or path data
- A message to an agent names the pane's title, the session and the page
  as they are written, `titled "C:\Program Files\PowerShell\7\pwsh.exe"`
  rather than with every backslash doubled, and on one line: a line break
  in a title, or in a label a page sends with, reads as a space, and a `"`
  in it as `'`

## 2.1.35-fork.3

- **Orchestrator sessions**: an agent session that plans the work, starts
  other sessions for it, its children, and follows them, while you talk to
  it alone. Start one with **Orchestrator** and an optional **Goal** in the
  New Session dialog, `kero new --orchestrator --goal "…"`, or make a
  session one with its menu's Make Orchestrator or `kero orchestrator
  <session>`. Its agent starts on a role prompt you can replace in Settings
  › Agents › Orchestrators (`orchestrator-prompt`)
- A session an orchestrator starts with `kero new` is its child, one level
  only: a child's `kero new` starts another child of the same orchestrator.
  `kero new --parent <session|none>` sets it, and
  `orchestrator-max-children` limits how many run at once
- Kero types into the orchestrator's prompt when a child waits for input,
  goes idle after working, fails, exits or closes, as one message for what
  comes close together and nothing for a flicker; a child sends its own
  result with `kero report --to-parent "<summary>"`
- In the sidebar, an orchestrator's row has its mark and its goal in its
  tooltip, and its children follow right under it with a ↳; its chevron
  folds them, counting those that need you. Children don't notify you or
  count on the badge, unless Settings › Notifications › An orchestrator's
  sessions (`notify-children`) is on
- Closing an orchestrator asks whether to close its children too or keep
  them (`kero close <orchestrator> --children`); `kero ls` and `kero status`
  show the tree
- Ama, Kero's helper agent, named after *amagaeru*, the Japanese tree frog. Open it from the frog at the bottom of the sessions sidebar, with Ctrl+Shift+A (⇧⌘A), or from View › Ama, and ask it to start, find, review or close sessions, worktrees and projects for you. It runs your default agent, or the one in Settings › Agents › Ama, in a real terminal, in a folder of its own beside Kero's configuration. It keeps running when you close it. New Chat starts a fresh conversation, History goes back to an earlier one, and Clear clears the screen. The sidebar doesn't list it, Needs You doesn't count it, and the frog shows a dot while it waits for you.
- Kero writes Ama's instructions as CLAUDE.md and AGENTS.md in its folder and keeps its skill installed for Ama's agent. Your own instructions go in USER.md, which Kero never changes. Settings › Agents › Ama opens it.
- Ama drives the app (kero do, kero focus, kero browser, kero pr review) without Remote Control, which still decides for every other agent.
- New commands: kero projects add, rename and remove; kero move; kero ls and kero close with -p and --status, kero close --all and --dry-run, and closing several sessions at once; kero worktree create, remove, cleanup and prune; kero pr list and kero pr review; kero config get and set.

## 2.1.35-fork.2

MyGo's fork adds `Page.CallDevTools` and `OnDevToolsEvent` (for `kero
browser console`, `network` and `emulate`), `App.SetBrowserArguments`
(for the DevTools port) and the theme's `LineAccent` (for the cursor's
color).

- Send to Session… tells the agent what the pane runs as well as its id:
  its agent (Claude Code), else its program, else its shell (pwsh), and
  the title the program set, so the agent knows at once what output it
  reads
- The picker's note keeps the keyboard as the pointer moves; pointing at
  Parent outlines what it would pick, and the note stays put so Parent can
  be clicked again
- Picks' text hints are short phrases that grep, and an element with no
  source location says so
- Picker screenshots go to a kero-picks folder in the temporary folder
  and are deleted a day later, as are older builds' kero-pick-*.png
- Importing sign-ins from Chrome or Edge also brings the localStorage and
  IndexedDB of the sites open in the browser, where many web apps keep
  their sign-in: open the app in the browser first, then Import in
  Settings › Browser, or `kero browser cookies import chrome --domain
  app.example.com`, which opens a hidden tab of the site if none is open.
  Reload the pane afterwards
- Browser panes on Windows can open a DevTools port (Settings › Browser ›
  DevTools port, or `browser-devtools`/`browser-devtools-port`) for
  Playwright's `connectOverCDP`, chrome-devtools-mcp, Puppeteer and
  Lighthouse; `kero browser devtools [--json]` prints the endpoint and
  each pane's page there
- `kero browser console`, `network` and `emulate`: an agent reads a
  browser pane's console (console.*, uncaught errors, promise rejections
  nothing caught, CORS and failed loads, each with its file and line), its
  requests (status, type, size, time, and why one failed: CORS, blocked,
  `net::ERR_…`, with `network body <id>` for a response's body), and shows
  the page as a phone or tablet (`emulate device iphone-15`, or a viewport
  with `--scale`, `--mobile`, `--touch`), with another user agent, or on
  a slow network or none (`emulate network slow-3g`, `offline`); `emulate
  reset` undoes it. They log from the pane's first load, keep the last
  1,000 of each until `--clear`, and take `--json`. `network` and
  `emulate` are Windows only; on macOS and Linux `console` has what the
  current document logged
- Text fields' cursor (the URL bar, the branch name box, the conversation
  search and the others) is the theme's Highlight color again, not its
  Highlight background, which stays the fill of highlighted menu rows,
  checked boxes and primary buttons. When the two highlight colors
  differ, links and the borders of focused fields change color too, from
  the Highlight background to the Highlight

## 2.1.35-fork.1

Upstream 2.1.35 (CHANGELOG.md): **wrap long lines** in the viewer's diffs
and files (`wrap-lines`); a **close button** on a session in the sidebar
as you hover it; `window-padding-color` picks what the padding around a
terminal's text shows; on a pane of a remote host, pasted JPEG, GIF and
WebP images, and **files dropped on the pane**, are uploaded to the host
rather than pasting this computer's paths; updating Kero while an SSH
bridge is connected no longer loses the saved sessions; the Kero skill
teaches agents to run short commands themselves and reuse their panes.
MyGo now follows the unreleased commit Kero 2.1.35 requires (46089cb,
after MyGo 0.8.3), with the fork's MyGo changes on top: it brings MyGo's
fixes since 0.8.1, among them Chinese text in one font on English
Windows, and terminals that let an app take dropped files first.

- Wrapping long lines is upstream's now, in place of the fork's own: one
  button wraps the viewer's diffs and files, editor panes, History's
  diffs and the merge editor alike, and the setting is `wrap-lines`
  (Settings › Git, beside Diff layout), kept with your settings rather
  than the app's window state. Its icon is upstream's, and its tooltip
  says Wrap Lines or Don't Wrap Lines. If you had wrapping on, turn it on
  once more: the fork's old choice is not carried over
- The session's close button closes it as its menu's Close does: a
  session in a worktree still asks what to do with the worktree
- Dragging files from another app onto a local pane opens them, or pastes
  their paths into a terminal, as before; on a remote host's terminal,
  they are now uploaded there, as upstream does

## 2.1.34-fork.16

- Send what a terminal printed to an agent: select it, right-click and
  choose Send to Session…, then the agent's session. The agent gets the
  text as a code block, with the pane it came from, typed into its prompt
  once it is at its prompt
- A local page in a browser pane can send the agent that opened it with
  `kero split --url` a message: `window.kero.send(message, {label, mode})`.
  The message carries the page's title and URL, and is typed into the
  agent's prompt once the agent is ready, or handed to an agent waiting
  with `kero browser wait-message --json`. The page hears when the agent
  has it, to show "sent", and a page no agent opened asks which agent.
  Pages tell by `window.kero` whether they are in Kero (Panes › Sending to
  an agent from a page, with an example page)
- Review's Send to Agent and PR watch send the same way: a review sent
  while the agent works is typed in once it is at its prompt, rather than
  into its busy prompt
- Kero's agent skill (`kero skill setup`) describes `window.kero.send` and
  `kero browser wait-message`
- Pick Elements: a browser pane's Pick mode (its toolbar's button,
  Ctrl+Alt+C / Alt+Shift+C, or `kero browser pick`) lets you click
  elements of the page, write a note on each and send them to the agent
  that opened the page. Each comes with where it is in the source first:
  the file and line from Vue, Svelte and React development builds (React
  19's through the source map), checked against the file, or the line of
  its tag in a local HTML file; then strings to grep for, marked when
  unique on the page; where it sits on the page and its HTML; its
  selector, styles and a boxed screenshot last
- The picker's screenshots go with the picks as files the message names,
  which agents open by path: each element boxed (scrolled back into sight
  if you scrolled away), and the page as you see it, every element boxed
  and numbered. Both are on; the picker's bar turns either off, and Kero
  remembers it
- Browser panes can sign in where you are in Chrome or Edge: turn on
  remote debugging in the browser (chrome://inspect/#remote-debugging),
  then click Import in Settings › Browser › Sign-ins, or run `kero browser
  cookies import chrome` (or `edge`), and allow Kero in the browser's
  dialog. The panes keep the cookies, and `kero browser` works on the
  pages as before
- Closing a session with its worktree removes it on Windows too: Kero
  waits for the agent and whatever it started, as MCP servers, node and
  git, to exit (killing what is left after 5 seconds) instead of a fixed
  200 ms, and tries again for a few seconds while the folder is still in
  use. Git, failing so with "Permission denied", had forgotten the
  worktree and emptied it, but left its folder and the merged branch:
  both go now. Removing a review's worktree or one in the History tab,
  the automatic cleanup and a handoff's try again too, and Kero's own git
  status stays out of a worktree while it goes
- An empty folder that such a removal left takes no new worktree's place:
  one of its name goes in it

## 2.1.34-fork.15

- Agents can act on pull requests: `kero pr state`, `merge` (with the
  repository's default method unless given one, `--auto` for
  auto-merge), `ready`, `draft`, `close`, `reopen`, `update-branch` and
  `delete-branch`, of the pull request given or the session's linked
  one, and `kero pr checkout`, here or `--worktree` in a new session,
  reusing a worktree that has it already
- `kero worktree list` tells of each worktree what `git worktree list`
  doesn't: the sessions in it, whether Kero made it, what removing it
  would lose, and what the automatic cleanup will do with it. `kero
  worktree status` tells an agent of its own session's worktree, its
  setup script, its pull request, the settings new worktrees take, and
  whether a handoff would go. Both take `--json`
- `kero worktree handoff` refuses a session already in a linked worktree
- Kero's agent skill (`kero skill setup`) describes all of these, and
  fork.14's handoff, review guide and pull request watch, so agents pick
  them up; an installed skill follows the update by itself

## 2.1.34-fork.14

- New worktrees start from origin: a session's new branch fetches the
  remote's copy of its base first and starts from it, unless the fetch
  fails or the local branch has commits the remote's lacks (Settings ›
  Git, on by default)
- A worktree's setup script runs as Kero makes the worktree: the first
  `runOnWorktreeCreate` of the repository's `kero.json`, else its
  `t3.json`, in a terminal below the session, with a progress card that
  can cancel it. Submodules are checked out first. `"async": false`
  holds the agent until the script ends
- Kero can name a new worktree's branch: New Session has a Task box,
  passed to the agent as its first prompt; leave the branch empty, or
  click the sparkle, and the model names it from the task. Settings ›
  Git picks its prefix: the kind of the work (feat/, fix/), a fixed one,
  or your own prompt's
- An agent can move its own session, with its conversation, into a new
  worktree: `kero worktree handoff <branch>` (Claude Code, Codex and
  OpenCode; uncommitted changes stay in the old checkout)
- Automatic worktree cleanup (Settings › Git, off by default): as Kero
  starts, hourly and on Delete Now, it removes the worktrees it made
  once inactive for some days, their pull request merged, with no
  commits of their own, or with their last session closed, and never
  one a session works in or one with changes. Projects can have rules
  of their own, or none
- Sessions know their pull request, found from their pushed branch or
  linked with `kero pr link`, and Kero watches it while the session is
  open: failed checks, merge conflicts and, as Settings › Git › Wake the
  session's agent has it, passing checks and new comments type a message
  into the agent's prompt
- Pull request actions on the PR page and in the list: Merge (with the
  repository's default method, which Review setup can change), auto-merge,
  Ready for Review, Convert to Draft, Update Branch, Close, Reopen,
  Delete Branch, Revert, and approving a fork's waiting workflow runs
- Check Out a pull request here, in the pane's folder, or in a new
  worktree; a worktree that already has it is reused
- Review guides: Write Guide has an agent write a tour of a pull request,
  which the PR page's Guide tab shows stop by stop, with Full Diff,
  Comment and Ask Agent

## 2.1.34-fork.13

- Removing a worktree from the New Session dialog's Existing worktrees
  now takes it off the list. A worktree made for a session, which goes
  as that session closes, used to stay listed until the dialog opened
  again

## 2.1.34-fork.12

- New Session in a worktree has a New branch switch, on by default: name
  the branch, and pick what it starts from in Start from, which first
  shows the default branch (or the folder's branch, when Settings ›
  worktree base is current). Turn it off to check out an existing branch
  as it is
- Sessions in a git worktree show a worktree mark in the sidebar, next to
  their branch, and say so in their tooltip. Hosts show it after their
  next update

## 2.1.34-fork.11

- Customize Theme has Background colors: 2 to 4 colors blended from top
  to bottom under the cards, as Aurora's. Editing a copy of one of
  Kero's themes starts with its own
- The Accent is now two colors: Highlight, for icons, outlines, links
  and the faint washes of selected rows, and Highlight background, for
  primary buttons, highlighted menu rows and pills. Text on Highlight
  background is white or near-black, whichever reads better, so Kero's
  dark themes now show dark text on their light blue buttons. A theme
  saved with an Accent takes it as both
- Each of these colors and the Background glow follows the theme until
  you change it: the highlight follows the cursor, the glow the
  highlight background
- Export copies the theme as the text of a Ghostty theme, Kero's colors
  too; Import… pastes one into the theme being edited, saved only when
  you click Save

## 2.1.34-fork.10

- Customize Theme has an Accent: the color of Kero's buttons and
  highlights, apart from the cursor's, so a bright cursor can have
  quieter buttons. It is the cursor's color, or the theme's blue when the
  cursor is gray, unless you choose another, and the Background glow
  follows it. Kero saves it on a `# kero-accent:` line, which Ghostty
  reads as a comment

## 2.1.34-fork.9

- Fixed: in a narrow pane or right sidebar, the header's buttons no
  longer run under the window's minimize, maximize and close buttons.
  They fold into the More (…) menu as the room runs out:
  - A browser pane's back, forward and reload go there once its split
    buttons have, and the address bar narrows further
  - Narrower still, a pane's Zoom and Close go there too, with the
    buttons of the window's corner
  - The right sidebar puts its Zoom and Hide buttons there once its tabs
    show as icons, and then the tabs that don't fit

## 2.1.34-fork.8

- Fixed: the divider beside a browser pane couldn't be dragged while the
  page had the keyboard, as it does with Focus follows mouse on after the
  pointer passed over the page. The page takes the keyboard back once the
  drag ends

## 2.1.34-fork.7

Built on MyGo 0.8.1, MyGo's latest release, in place of the unreleased
commit Kero 2.1.34 requires, with the fork's MyGo changes on top:

- Menus have Windows 11's look: rounded corners and, with transparency
  effects on, Acrylic behind them (a solid color elsewhere), and their
  shortcuts in WinUI's caption font. The tray icon's menu opens above
  the taskbar, centered on the icon
- Scrolling the other way at the end of a list or a diff turns at once
- The terminal links more paths: in Chinese text, in quotes, and with a
  `(line,col)` or `#L` place after them
- Changed: the fork's menus keep their compact rows (26 DIPs) and the
  system's menu font size, now on MyGo's new menus: a highlighted item's
  highlight is a little shorter than its row, as in Windows 11's own
  menus, and a menu has a little room above its first item and below its
  last

## 2.1.34-fork.6

- History's chosen commit keeps its graph in line with the rows around
  it in the dark, and its highlight stands off the sidebar's left edge,
  as off its right. A chosen row in any list no longer moves its text

## 2.1.34-fork.5

- Customize Theme has a Background glow: the color that lights the
  window from the top right corner, in place of the theme's cursor
  color, or its blue when the cursor is gray. Kero saves it on a
  `# kero-glow:` line, which Ghostty reads as a comment

## 2.1.34-fork.4

- A question widens to keep its buttons on one line, as one with a long
  branch's name; they wrap only in a window too narrow for that
- Clearer wording across the fork's dialogs, warnings, settings and
  tooltips: removing a worktree says what it deletes and how to keep
  unpushed commits, the interactive rebase's message editor confirms
  with Use Message, and the Start Draft tip names `kero pr comment`

## 2.1.34-fork.3

- In the New Session dialog, right-click an existing worktree to remove
  it, or remove it with its branch; Kero says first what removing it
  would lose, as changed files or commits on no remote
- A question asked over a dialog shows on top of it
- A question's buttons go onto a second line when a long name, as a
  branch's, leaves no room for them on one, instead of running past the
  dialog's edge

## 2.1.34-fork.2

- A project's + opens the New Session dialog: an agent session (the
  default), a terminal, a new file or a browser. An agent or a terminal
  can start in a worktree, on a branch you pick or name; a branch
  checked out elsewhere offers that worktree or a new branch from it, and
  the repository's existing worktrees are a click away. New Session in
  Worktree…, and New Worktree in History, open the same dialog
- Settings › Agents lists the agent commands the dialog offers (`agent`,
  such as `cc` or `codex --full-auto`) and the one it chooses first
  (`default-agent`, `claude` unless set)
- Review with Agent starts the agent with its prompt, instead of typing
  the prompt in once the agent looks ready, which sometimes left it
  unsent; `{prompt}` in a repository's review command puts it where the
  agent wants it
- The Review view lists its review agents, with their status and their
  pull request, to open, stop or clean up; review sessions carry a
  Review badge in the sidebar, and a pull request with an agent shows an
  Agent button
- Reviewing a pull request again offers its existing agent, or a new
  review in its worktree
- Agents show you documents: `kero split --file plan.md --preview` (and
  `kero new`) opens a Markdown file previewed, `kero do pane.preview
  on|off` turns an editor pane's preview on or off, and **Pane › Toggle
  Markdown Preview** does it from the menu; the pane keeps its preview
  when Kero restarts
- `kero split --url` takes a path to a local HTML page, as
  `.backlog/pages/brief.html`, and shows it from the disk, its scripts
  from CDNs too
- Editor panes show their file again as another program rewrites it,
  unless they have unsaved edits; browser panes of a local page reload it
  as it changes
- `kero split` of a file or local page a pane of the session already
  shows focuses that pane instead of splitting again, and says so

## 2.1.34-fork.1

Upstream 2.1.34 (CHANGELOG.md): **reorder projects** in the sidebar by
dragging a project's header, from its menu's **Move** (Up, Down, to Top,
to Bottom), or with `kero projects move`; **Cell width** in Settings ›
Terminal (`cell-width`) widens or narrows the terminal's columns;
**Session count** in Settings › General hides the number beside each
project; your own agents in `agents.toml`; <kbd>Alt</kbd>-click on a
marked shell prompt moves its cursor; `kerod.log` says why a restored
pane came back as a shell. MyGo now follows the unreleased commit Kero
2.1.34 requires (3b9d676), with the fork's MyGo changes on top: it
brings MyGo's fixes since 0.7.4, among them AltGr typing its character
in the terminal on Windows (upstream's fix, in place of the fork's own).

- Dragging a project in the sidebar is upstream's now, in place of the
  fork's: a line shows where it goes, and its menu has **Move** too. The
  hairlines between projects stay
- Settings › Terminal's sample is upstream's real terminal, which shows
  the cell width; pointing at a theme, or editing one, still recolors it
- Restored editor panes and your own **Resume** commands keep working
  alongside upstream's notes in `kerod.log`

## 2.1.33-fork.1

Upstream 2.1.33 (CHANGELOG.md): **Keep computer awake** (Settings ›
General, the tray menu, the command palette), always or while agents
work; each session in the sidebar starts with its program's icon, and a
split session's count of panes lists them; **View › Toggle Dark Mode**
(<kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>L</kbd>); **Open in Default App**
in the Files tab's menu; **Font features** (`font-feature`) in
Settings › Terminal; OMP (oh-my-pi) as a coding agent; the terminal is a
text field to dictation apps; a second **Close Pane** while the viewer
asks to save its edits no longer closes the pane beside it; the browser's
address bar opens a bracketed public IPv6 address. MyGo is unchanged (the
fork's 0.7.4), so the fixes upstream took from MyGo's later, unreleased
commits wait for its next release, among them the Windows UI Automation
provider's server-side flag, which dictation apps may rely on to see the
terminal's text.

- The Files tab's menu has upstream's **Open in Default App** first, above
  the fork's Open in Pane, Show History, Open in Editor, Open With and
  Show in Explorer
- **Close Pane** with the keyboard in the Files tree still closes the
  viewer, and waits, as upstream's does, while the viewer asks to save

## 2.1.32-fork.1

Upstream 2.1.32 (CHANGELOG.md): **Close Pane** (<kbd>Ctrl</kbd>
<kbd>Shift</kbd> <kbd>W</kbd>) with the keyboard in the viewer closes the
viewer, asking first to save its edits, instead of the pane beside it,
and opening a diff or the branch's changes gives the viewer the keyboard;
`kero ls --all` lists every host's sessions; Kero keeps its agent skill
and the Pi and OpenCode plugins current as it starts; Codex's hook runs
from every Windows shell, including a Kero installed under a path with
spaces; filter list rules with `$~document` block every other kind of
request. MyGo is unchanged (the fork's 0.7.4).

- A file clicked in the Files tab still leaves the keyboard in the tree,
  to move in it with the arrow keys, rather than giving it to the viewer
  as upstream does; **Close Pane** there closes the viewer too, not the
  pane beside it

## 2.1.31-fork.10

- The Review tab keeps showing the pull requests it last fetched for a
  repository and filter while it refreshes them, instead of emptying the
  list; a refresh that fails keeps the list and says why

## 2.1.31-fork.9

- **Start Draft**, in a pull request's review bar, starts a draft review
  without launching an agent or a worktree, so any agent can add comments
  to it with `kero pr comment --pr <number>`

## 2.1.31-fork.8

- The branch switcher groups **Worktrees**, **Branches** (in their
  folders), **Remotes**, **Tags** and **Stashes**, each one folding, under
  one filter that searches them all
- Clicking an entry scrolls History's graph to its commit and highlights
  it, without checking anything out; double-click or Return checks a
  branch out, or opens a worktree's session
- **Settings › Git › Hide old branches** hides branches with no commit in
  that many weeks, unless the filter finds them; a row shows them on
  demand

## 2.1.31-fork.7

- History opens a commit's diffs in the viewer, as Changes does: an open
  commit lists its files in their folders, and clicking one shows all of
  the commit's diffs there, scrolled to it. As you scroll the viewer, the
  file at its top is highlighted in History

## 2.1.31-fork.6

- **Stash some files**: in the Changes tab, **Add to Stash** on files,
  folders or a section moves them to a **To Stash** section; name the
  stash if you like and click **Stash N Files**. The other files stay
  as they are
- **Worktrees in History**: a Worktrees section lists every worktree
  with what removing it would lose (changed files, commits on no
  remote) or that it's merged or pushed. Open it in a session, push its
  branch, merge it into yours, or remove it, with its branch when
  nothing would be lost. **Prune** forgets worktrees whose folders are
  gone, **+** starts a session in a new one, and a commit's menu opens
  any branch in a new worktree

## 2.1.31-fork.5

- **Zoom Sidebar**, beside the right sidebar's hide button, hides the
  panes and gives the sidebar their room, for History's graph and the
  Review list, as Zoom Explorer did before the rebase; **Show All Panes**
  brings them back
- The right sidebar's Files tab comes before Changes again
- History's diffs fit line numbers of five digits and more

## 2.1.31-fork.4

Built on MyGo 0.7.4, MyGo's latest release, which has the MyGo fixes
2.1.31-fork.1 lacked, and more:

- The terminal's, the viewer's and the diffs' scroll bars have their
  new look: their thumbs show while the pointer is over them, with no
  track under them, and a wide diff's horizontal scroll bar is as high
  as its lines
- The mouse wheel scrolls a diff over its separators too, and dragging
  a selection past an editor's left or right edge scrolls it sideways
- Text copied from Notepad pastes on Windows again
- Menus' labels are no longer cut off, at any display scale
- Windows: no black where a window shrinks, and frameless windows draw
  up to their edges on Windows 10
- Making a release's delta update can no longer hang on a large app
- `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` and
  `WEBVIEW2_USER_DATA_FOLDER` apply as WebView2's own loader takes them,
  and the installer's desktop shortcut check box is in the installer's
  language
- Changed: the fork's own fixes for reading the clipboard past formats
  that don't render (as the Photos app's) and for the width of menu
  items gave way to MyGo's fixes of the same. The first context menu
  after a start, or after moving to another display scale, may be a
  little wider than it needs

## 2.1.31-fork.3

- The Files tab shows its folder's whole path above the tree where it
  fits beside the buttons, else only the folder's name, as a pane's
  header shows its folder

## 2.1.31-fork.2

- An editor pane's buttons are in its header, as the viewer's are,
  rather than in a bar below the file: Preview Markdown, the text's
  size, wrapping, Open in Editor, and for a new file Save and Open
  File…. A dot marks unsaved edits, as in the viewer. In a pane too
  narrow for them, they're in one **File Actions** menu
- The merge editor's conflicts left, the way between them, the version
  to take in all of them and Mark Resolved are in the pane's header too

## 2.1.31-fork.1

Upstream 2.1.28 to 2.1.31 (CHANGELOG.md): the right sidebar's Changes
and Files tabs, files opening in a viewer card beside the panes to read
and edit, New File, New Folder, Rename and Delete, file icons, the
changed files' tree beside their diffs with Collapse All, and
`scrollback-limit` setting the app's copy of a pane's scrollback too.

The fork's features now build on upstream's sidebar and viewer, in
place of the Explorer:

- The right sidebar has Changes, Files, History and Review tabs, styled
  as the left one. A file you click in Changes, Files or Search, a
  pull request, and a new pull request's form open in the viewer beside
  the panes, which **Zoom Viewer** spreads over them. With
  `sessions-sidebar = right`, the sidebar and the viewer take the left
- Files keeps the fork's tools on upstream's tree: the filter, Search,
  Refresh, hidden files as Settings say, dragging files onto panes,
  moving in the tree with the arrow keys, and a menu that opens a file in
  an editor pane, shows its history, opens a terminal in a folder, opens
  in your editor or another app, shows in File Explorer and copies paths
- The viewer has the fork's Markdown preview, image view, text zoom and
  wrap, and **Open in Pane**, which opens the file in an editor pane of
  its own. Editor panes save as the viewer does, asking before
  overwriting a file changed on disk, and their unsaved edits outlast a
  quit
- Go to File opens the file in the viewer, and Cmd+Enter (Ctrl+Enter) in
  an editor pane
- Changed:
  - A click on a file always opens it in the viewer, so the Files
    settings for what a double click does (`files-double-click`), where
    a pane's file goes (`files-pane`) and the preview below the tree
    (`files-preview`) are gone; the preview below the tree too. Opening
    a file in a pane always opens a new one, and a file dropped on an
    editor pane replaces its file. Old settings files with these keys
    still load
  - Changes always shows its files as a tree, as upstream's tab does,
    without the Tree View button; a folder's buttons still stage,
    unstage and discard its files
  - Diffs fold and unfold with the viewer's Collapse All and Expand All
    in place of each section's buttons
- Not in this release, as the fork stays on MyGo's latest release
  (0.7.3) while upstream 2.1.31 builds on MyGo commits after it: the
  new look of the terminal's and the viewer's scroll bars, the scroll
  bar under wide diff lines, and the fix for pasting text copied from
  Notepad on Windows

## 2.1.27-fork.13

- Right-click a file pane's name at the top to copy its path, whole or
  relative, open it in your editor or with another app, show it in the
  file manager, or show its history
- Files and folders dragged in from the file manager open in panes, as
  ones from the Files view do: beside a pane when dropped near its edge,
  in an editor pane's place at its middle, a folder in a terminal. On a
  terminal, only its left, right and top edges open them; the rest of it
  pastes their paths, as before

## 2.1.27-fork.12

- Discarding, staging or unstaging a folder of thousands of files works
  on Windows, where it failed with "The filename or extension is too
  long": the paths go to git in batches that fit a command line

## 2.1.27-fork.11

- Search results and the Files view's header show a file's whole name
  when there's room, as the Changes view does, and keep its extension
  when they cut it

## 2.1.27-fork.10

- The Changes view shows a file's whole name when the row has room for
  it, and cut, keeps its extension: report_FI….pdf
- Checking out a remote's branch, as origin/master, works when your
  branch of its name is behind it: Kero brings it up to the remote's.
  When each has commits the other lacks, it says to switch and pull
- An open commit's subject in History wraps under its branch pills,
  across the row, rather than in a narrow column beside them

## 2.1.27-fork.9

- Files and their Markdown previews zoom on their own: Smaller Text and
  Bigger Text in the file's bar, or Ctrl + = / - / 0 while a file has
  the focus; terminals keep their size
- On Windows, right-click menus are smaller, with text the size of
  Kero's and shorter rows
- A long branch name in a menu's item is cut in its middle, so the menu
  stays narrow

## 2.1.27-fork.8

- The Changes view lists a file git considers changed even when it shows
  no lines changed, as one written anew with other line endings, which
  a branch switch refuses to overwrite: stage or discard it from there
- Expand All Files and Collapse All Files open and fold the tree's
  folders too
- With focus following the mouse off, a click on a Markdown preview's
  text focuses its pane

## 2.1.27-fork.7

- In History, branch and tag names are in regular text; only the branch
  you're on is bold, with an outline and no check

## 2.1.27-fork.6

- History's pills share one filled style, told apart by bigger icons;
  only the branch you're on has an outline
- Stashes show as pills on the commit they were made on; right-click one
  to apply, pop or drop it, as in the Stashes list
- A branch checked out in another worktree shows a folder icon, and its
  commit's menu has Open Worktree, which goes to that worktree's
  session or opens one, and Copy Worktree Path

## 2.1.27-fork.5

- Widen Explorer is now Zoom Explorer: like Zoom Pane, it hides the
  panes and gives the Explorer their room, rather than squeezing them;
  Show All Panes brings them back

## 2.1.27-fork.4

- History shows branch and tag names whole when the view has room, and in
  a narrow view truncates the commit's subject before them
- Branch and tag pills take the color of their lane in the graph; the
  branch checked out has a check, bold text and a border

## 2.1.27-fork.3

- The History graph draws every branch in a lane of its own: past six,
  lanes no longer pile onto the last one, and with many they draw closer
  together in a narrow view

## 2.1.27-fork.2

- The pull requests list shows checks and review as separate pills, so
  ✓ Checks and Approved no longer read as one
- Repository Setup… sets a repository's own prompts for commit
  messages, pull request descriptions and reviews; Settings › Git keeps
  the defaults, now with the review prompt too
- On Windows, the window reopens where it was closed: on a display left
  of the main one it no longer creeps right on every launch, and on a
  display of other scaling it no longer grows toward the other display

## 2.1.27-fork.1

- Based on Kero 2.1.27, with 2.1.25 and 2.1.26 (see the
  [changelog](CHANGELOG.md)): a theme now colors the whole window, its
  panes, text, accent and terminals; Paper and four more themes; a
  second line for each session in the sidebar (Settings › General ›
  Sidebar rows); scrolling speed for the wheel and the trackpad
  (Settings › Terminal › Scrolling); a symlinked configuration stays one
- Built on MyGo 0.7.3: on Windows, a window that grows shows its
  background rather than black, and a window no longer looks magnified
  while it resizes
- Theme colors moved with upstream's: Settings › Appearance › Ghostty
  themes chooses a Ghostty theme or your own, with each theme's swatch,
  a preview of the theme under the pointer, now in the whole window,
  and Customize Theme beside it. Terminal has no Colors group anymore
- File Colors (`file-theme`) offer Kero's themes by name; a file theme
  of `kero` reads as `plain`, Kero's former colors. A card in a file
  theme of its own takes that theme's text colors too
- The Explorer lies flat on the theme with a hairline in the light, as
  the panes do

## 2.1.24-fork.11

- Uncommitted lists the changes not staged first and the staged ones
  below, so the rows under the pointer stay put as you stage
- Staging and unstaging show at once, and clicks in quick succession all
  go through; the list behind it refreshes faster
- The Changes view's rows are shorter, for more files at once

## 2.1.24-fork.10

- In the Changes view's tree, a folder's buttons stage, unstage or
  discard the files in it
- A review's prompt is sent more reliably: Kero presses Enter once the
  agent has taken the paste, and again while it hasn't started, also
  while Kero's window is hidden

## 2.1.24-fork.9

- HTML within a pull request's text, as the badges of severity Copilot's
  comments start with, shows as what it means, `[High severity]`, not as
  its source

## 2.1.24-fork.8

- Settings' pages go General, Appearance, Terminal, Files, Git

## 2.1.24-fork.7

- Reviewing a pull request in a worktree works while your checkout or
  another worktree is on its branch: each review gets a branch of its own,
  `review/pr-<number>`, which pushes to the pull request's branch
- Git's error messages show the line that says what went wrong, not
  git's progress

## 2.1.24-fork.6

- New worktrees, for sessions and for reviews, get the ignored files a
  `.worktreeinclude` at the top of the repository lists, such as
  `.claude/settings.local.json` or `.env`, as Claude Code's worktrees do

## 2.1.24-fork.5

- Settings has a **Git** page and a **Files** page of their own, out of
  General
- Settings' descriptions are shorter

## 2.1.24-fork.4

- Approve a pull request, or post a summary alone, with no comments: the
  review's bar is on every pull request's page
- Comment on a whole file, with the comment button in its header; agents
  can too (run `kero skill setup` again)
- **Resolve Conversation** on a thread of comments resolves it on GitHub,
  and folds it to a line
- The **Conversation** tab shows the threads of comments on lines and
  files, with their replies; click a thread's file to see it in **Files**
- Replies are set apart from the comment they answer
- A pull request whose draft comments were all deleted no longer shows as
  in review
- On a narrow panel, **Post Review** moves to a line of its own, and the
  verdict becomes a menu, so nothing is cut off

## 2.1.24-fork.3

- A pull request's page scrolls to the next comment in the same file too,
  not only to the first

## 2.1.24-fork.2

- The Review tab's list marks each pull request with a draft review:
  **Reviewing** while its agent works, **Review Ready** once it's done,
  and how many comments the agent has added so far
- **Review** starts the agent in the background: Kero stays where you
  were, and the list shows when the review is ready
- A pull request's page jumps from comment to comment (▲ ▼ beside its
  tabs), through your draft review's comments and those already posted,
  opening each file and scrolling to the comment
- The review's summary box shows more lines

## 2.1.24-fork.1

The fork on Kero 2.1.24, and on a fork of MyGo 0.7.1:

- Images a program like Pi showed no longer stay over the screen once it
  clears it, as with Pi's `/new`
- After Kero relaunches to use a newer kerod, quitting it quits it
  instead of leaving it stuck open
- A mouse wheel's notch scrolls a terminal three rows, not seven
- Shift-click extends a terminal's selection
- With **Show accent color on title bars and window borders** on in
  Windows, the accent no longer shows as a strip over the top of Kero's
  window
- The installer is now named `kero-setup-2.1.24-fork.1-amd64.exe`, not
  `Kero.Setup.2.1.24-fork.1.amd64.exe`

## 2.1.21-fork.10

- The Explorer has a **Review** tab: your GitHub repository's pull
  requests, those asking for your review, yours, or all open ones
- **New Pull Request** pushes your branch and opens a pull request, or a
  draft, from a form of its base, title and description, with its
  commits and files under it. ✨ writes the title and description, from
  the prompt in Settings › General › Git
- Click a pull request to see it in Kero: its description, reviewers,
  checks, comments and reviews, and its changes with the comments on
  their lines. Folded sections, as Dependabot writes, open with a click
- **Review with Agent** starts an agent on the pull request, in a
  worktree of its branch. Its comments go to a draft review that you
  read, edit or delete, and ask the agent about, then post as one review
  with **Post Review**: the agent never posts. Run `kero skill setup`
  again so agents learn the `kero pr` commands
- Kero notifies you of new pull requests that ask for your review, or of
  every new one (Settings › General › Git)
- In **Branch**, a change has **Revert**, which puts back the file's
  lines from where the branch began

## 2.1.21-fork.9

- Right-click a changed file in the Changes view, or a commit's file in
  the History view, to open it: **Open in Editor**, **Open in Pane**, and
  **Open in New Pane** when Open in Pane replaces an editor pane's file.
  This takes the place of the file's Open File button, and History's
  **Show File History** moved into the same menu
- The Search view's files and found lines have that menu too; a line opens
  its file at that line
- Drag a changed file, a commit's file, or a search result onto a pane to
  open it there, as from the Files view; a found line opens at its line
- A review comment's card no longer cuts off its bottom edge
- The review comments and **Send to Agent** now sit below the files,
  above the commit box

## 2.1.21-fork.8

- **App Font** (Settings › Appearance) sets the font of Kero's own text,
  apart from the terminals': Kero comes with Inter, Geist, Mona Sans,
  Public Sans, IBM Plex Sans, Plus Jakarta Sans, Manrope, Figtree, DM
  Serif Display and Playfair Display, and any installed font works
- Text sits centered beside the icons next to it, where it sat low before
- The branch switcher lists the tags after the remote branches: choosing
  one checks out its commit
- In the History view, a branch and its remote's at the same commit share
  one chip, with both icons; refs too many for the row are cut off rather
  than drawn over the commit's message. Hovering a commit shows the author
  and the email on lines of their own, and a click on an open commit's
  message closes it
- A changed file's path shows when you hover its name, not anywhere on
  its row
- **Search** moved from the Explorer's tabs to a button in the Files
  view's toolbar; Find (Ctrl+Shift+F) in the Explorer opens it, and in a
  diff or a file there still finds in it

## 2.1.21-fork.7

- **Review comments**: comment on lines of the Changes view's diffs, as on
  a pull request, from the **+** by a line's number or a change's
  **Comment**, and **Send to Agent** sends them all to the agent in the
  worktree as one prompt, each with its file, lines and code
- **Search**, a tab of the Explorer (Ctrl+Shift+S), finds text in the
  folder's files, by case, word or regular expression, and opens a file
  at the line found
- **Go to File…** (Ctrl+Shift+N) finds a file of the project by its name
  as you type, and opens it in an editor pane

## 2.1.21-fork.6

- An editor pane takes the keyboard as you click it, or as the pointer
  moves into it with focus-follows-mouse, also while it previews Markdown
  or shows an image, where the keyboard stayed in the pane before

## 2.1.21-fork.5

- The Changes view's files start folded, a list of what changed: each
  section, and Last Turn's and Branch's lists, has a button that unfolds
  all its files, or folds them while one is unfolded
- **Tree View**, beside Wrap Lines, shows the changed files in their
  folders
- Right-click a file, or a folder of the tree, to **Add to .gitignore**:
  for what git tracks already, Kero asks whether to stop tracking it too
- Drag a project by its name to reorder the sidebar's projects, which a
  faint line now keeps apart
- **Preview Markdown** shows a Markdown file as it reads, in an editor
  pane and in the Files view
- Chevrons turn quicker, without overshooting

## 2.1.21-fork.4

- **Search conversations** finds a Claude Code conversation by the slash
  command you started it with, as `/client-checkin KLS000`, and finds the
  ones that had nothing else from you, which it left out
- `claude -p`'s conversations are left out of the search again

## 2.1.21-fork.3

- **Search conversations** finds Claude Code conversations that a wrapped
  build of Claude Code ran, and `claude -p`'s, which it left out
- AltGr types its characters in a terminal on Windows, as AltGr+Q types
  ä, where it sent Control and the key
- Searching a host whose kerod is too old for it says to restart kerod,
  where it said no conversation matched

## 2.1.21-fork.2

- **Search conversations** at the foot of the sidebar finds the agents'
  conversations by what was said in them, as fast-resume does: every one
  Claude Code, Codex and Pi keep on each host, also those that never ran
  in Kero. Click one for its messages that have your words, and resume it;
  filter with `agent:`, `dir:` and `date:`
- **Resume commands** in Settings › Agents: resume each agent's
  conversations with a launcher or flags of your own, as
  `cc --yolo --resume {session}`, in your shell

## 2.1.21-fork.1

The fork on Kero 2.1.21:

- A session's time stays whole beside a long name in a narrow sidebar
- **Split Right** starts a session when the window has none

## 2.1.20-fork.4

- Pushing a branch made from another's remote branch, as from
  `origin/main`, pushes it to a branch of its own name, which it then
  tracks, never over that branch

## 2.1.20-fork.3

- A **Menus** button (⋯) beside Settings in the sidebar has the menu
  bar's menus, with their shortcuts, where Windows shows no menu bar
- **New Session in Worktree…** in a project's menu in the sidebar

## 2.1.20-fork.2

- An open commit in the History tab shows all of its subject, wrapped
- A commit's tooltip fits a GitHub noreply address on one line
- **Write Message** keeps only the message, not what a launcher of the
  agent prints as it starts

## 2.1.20-fork.1

The fork on Kero 2.1.20, with its quick terminal, and on a fork of MyGo of
its own:

- Conflicted files open in a **merge editor**: theirs and yours beside the
  result, each conflict's side to take, Previous and Next, and **Save and
  Mark Resolved** once none is left
- **Interactively rebase** your branch onto another from its menu: pick,
  reword, edit, squash, fixup or drop each commit, drag or Alt+↑/↓ to
  reorder them, see each one's changes, and keep a backup branch
- Git actions and their confirmations keep the focus in the Explorer, not
  the terminal
- Select a diff's lines by dragging, across both sides; the lines deleted
  show as selected too
- `commit-message-command` runs in your shell, as your own commands and
  functions do, and `{session}` in it is the pane's agent's conversation
- Copied files, as from ShareX, paste into a terminal as their paths
- Context menus as wide as their items, and the History tab's scope toggle
  fixed
- The window opens on the screen it was on last

## 2.1.19-fork.4

Git in the Explorer:

- **Uncommitted** lists Staged Changes apart from Changes: stage,
  unstage and discard files, changes, or lines selected by their numbers
- Commit from the box under the list, with Commit and Push, Amend, Sign
  Off and Skip Hooks; **Write Message** has the message written by a fork
  of the pane's agent's conversation, or by `commit-message-command`
- The branch bar switches, creates, renames and deletes branches, syncs
  with the upstream, and fetches, pulls, pushes and stashes from its ⋯
  menu; a failed fetch, pull or push can run again in a terminal
- Merge a branch into yours, or rebase yours onto it, from its menu or a
  commit's; pulls, merges and rebases stash changes not committed and put
  them back
- Conflicts resolve by **Use main's Version** or the other branch's, by
  hand and **Mark Resolved**, or with **Resolve with Agent**; then
  Continue, Commit Merge or Abort
- The **History** tab (Ctrl+Shift+Y) draws the commits as a graph, local
  and remote branches apart, with each commit's files, the stash, and
  Check Out, Copy Branch Name, Cherry-Pick, Revert and Reset
- What changes a worktree's files while an agent works there asks first,
  to run when it's done or now

## 2.1.19-fork.3

- A **Filter** in the Files view's toolbar finds files and folders by name
  in all of the folder, or by path with a slash, and shows them in the
  folders they're in; Esc shows the file chosen in the whole tree

## 2.1.19-fork.2

- Drag a file from the Files view onto a pane: near an edge, it opens
  beside the pane on that side; in the middle of an editor pane, it takes
  the place of its file
- **Open in New Pane** in a file's menu, when files open in place of the
  last file pane's (`files-pane = replace`)
- The whole of a row in the Files view opens its menu, its edges too

## 2.1.19-fork.1

The fork on Kero 2.1.19, with its sessions in worktrees, the agents'
conversations to resume, folders dropped onto the sidebar to open them,
and the sidebar's empty space to move the window, also at the window's
right.

## 2.1.18-fork.1

The first release of the fork from its own feed, on Kero 2.1.18:

- The Explorer: the Files view, beside the sessions, with new files and
  folders, and a folder that opens with one click
- Editor panes, and `kero new --file` to open a file in one
- Pane headers that fit narrow panes, and the Changes view's lines wrapped
- Settings for the Explorer's side, paths, files and what dims while a
  window is unfocused; focus that follows the mouse
- Terminal themes previewed as you point at them, your own themes, and the
  files' theme
- **Quit and End All Sessions** in the app icon's menu
- Updates from the fork's releases, not egoist/kero's
