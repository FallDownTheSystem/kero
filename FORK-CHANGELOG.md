# Fork changelog

The release notes of the fork's own releases (FORK.md), which its update
window shows. Each version is upstream's, X.Y.Z, with what that version
brings in [upstream's changelog](CHANGELOG.md), and the fork's changes on
top of it.

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
