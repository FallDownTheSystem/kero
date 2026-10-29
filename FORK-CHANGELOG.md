# Fork changelog

The release notes of the fork's own releases (FORK.md), which its update
window shows. Each version is upstream's, X.Y.Z, with what that version
brings in [upstream's changelog](CHANGELOG.md), and the fork's changes on
top of it.

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
