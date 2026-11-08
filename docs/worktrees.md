---
title: Worktrees
description: Give each agent a git worktree of its own, set new worktrees up with a script, and clean them up.
---

# Worktrees

Agents working side by side in one checkout trip over each other's changes. A session in a git worktree of its own works on its own branch, in its own folder, so its agent can edit, build and commit without touching yours.

## Starting a session in a worktree

**Session › New Session in Worktree…** (<kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>T</kbd>) opens the [New Session dialog](./sessions.md#new-session-dialog) with **In a worktree** on, to start a session in a worktree checking out the branch you pick, or a new branch you name, from the repository's default branch. A project's menu in the sidebar has it too, for a session of that project's repository. On Windows and Linux, which show no menu bar, the menu bar's menus are in the sidebar's **Menus** button (⋯), beside Settings.

The session joins the repository's project, and the sidebar shows its branch. Other ways to start one:

- **+** in the History tab's [Worktrees section](#the-worktrees-section), or a commit's **New Session in Worktree Here…** and **Open … in New Worktree**.
- A branch's menu in the branch switcher, which opens it in a session of its own worktree.
- A pull request's **Review** and **Check Out in New Worktree** ([Pull Requests](./pull-requests.md)).
- An agent moving its own session into one ([below](#moving-a-session-into-a-worktree)).
- The command line:

```sh
kero new --worktree fix-login -- claude   # a session on a new branch, in its own worktree
kero new --worktree fix-login --base dev  # branch from dev
kero worktree create fix-login -p api -- claude "fix the login"   # of a project's repository
```

## Where worktrees go and what they start from

- A worktree goes in `.kero/worktrees/<branch>` inside the repository, which Kero ignores through `.git/info/exclude`, not your `.gitignore`. Set `worktree-dir` to put worktrees elsewhere.
- Before making the worktree, Kero fetches the base branch and starts from the remote's copy, so a stale local `main` doesn't matter. Offline, or when the local branch has commits the remote lacks, it starts from the local branch. Set `worktree-from-origin = false` to always use the local branch.
- Set `worktree-base = current` to make a new branch from the project folder's branch instead of the default branch.

These are in **Settings › Git**. `kero worktree status`, in a session, prints which of them new worktrees take.

When Kero makes a worktree, it sets it up in this order:

1. It fetches the base branch, and checks the worktree out.
2. It copies the ignored files [`.worktreeinclude`](#copying-ignored-files) lists.
3. It checks out the worktree's git submodules, when it has some.
4. It runs the repository's [setup script](#setup-scripts), when it has one, beside the session's agent or before it.

A card at the window's bottom right shows each step as it goes. **Cancel** on it stops the setup.

<a id="copying-ignored-files"></a>

## Copying ignored files: `.worktreeinclude`

A new worktree has only the files git tracks. To bring along ignored ones, such as `.env`, `.claude/settings.local.json` or `CLAUDE.local.md`, list them in a `.worktreeinclude` at the top of the repository's main checkout, in `.gitignore`'s syntax, as Claude Code's worktrees read it:

```gitignore
.env
.env.local
.claude/settings.local.json
CLAUDE.local.md
config/secrets/
```

Kero reads the main checkout's `.worktreeinclude`, committed or not, and copies from the main checkout each file that matches and git ignores; a tracked file is in the worktree already. It leaves a file the worktree has already as it is. It skips `node_modules` and other worktrees, and goes into a wholly ignored folder only for a pattern that names a path in it, such as `cache/keep.txt`. The [cleanup](#cleaning-up-worktrees) doesn't count these copies as the worktree's own, as they are the main checkout's.

<a id="setup-scripts"></a>

## Setup scripts: `kero.json`

To set a new worktree up, such as installing its packages or building it, give the repository a `kero.json` at its top, with a script that runs as a worktree is made:

```json
{
  "scripts": [
    { "name": "Install", "command": "bun install", "runOnWorktreeCreate": true }
  ]
}
```

| Key | Meaning |
| --- | --- |
| `name` | What the setup card calls the script (default: the file's name) |
| `command` | The command, as you'd type it in your shell: `npm ci && npm run build` |
| `runOnWorktreeCreate` | `true` to run it as a worktree is made. Kero runs the first script that has it, and only that one |
| `async` | `false` has the agent wait for the script to end. Left out, or `true`, the agent starts right away, beside it |

The script runs in a terminal below the session's own, in the worktree's folder, through your shell (the `shell` setting), with these set:

| Variable | Value |
| --- | --- |
| `KERO_WORKTREE_PATH` | The new worktree's folder |
| `KERO_PROJECT_ROOT` | The repository's main checkout, to copy or link things from |
| `KERO_WORKTREE_BRANCH` | The worktree's branch |

- **With `"async": false`**, the script's terminal starts first, and the agent starts above it once the script ends, even when it failed, so the agent can see to it. Use it when the agent needs the setup done, as dependencies to run tests.
- **Otherwise** the agent starts at once, and the script runs beside it.
- The terminal closes once the script succeeds, or as you cancel it, and stays, with its output, when it fails.
- With `cmd` as your shell, Kero can't tell when a command ends: the script runs on, and an agent waiting for it starts at once.

Kero looks for the script in the new worktree first, then in the main checkout, and in each, `kero.json` before `t3.json`: Kero reads [t3code](https://github.com/pingdotgg/t3code)'s `t3.json` too, which has the same form, and gives its scripts `T3CODE_PROJECT_ROOT` and `T3CODE_WORKTREE_PATH` as t3code does. So commit `kero.json` to share the setup with everyone, or keep one in your main checkout alone, ignored, for yourself. A file that isn't valid JSON fails the setup, and the card says why.

Every worktree Kero makes for a session runs it: from the New Session dialog, `kero new --worktree`, `kero worktree create`, `kero worktree handoff`, and a pull request's review or checkout in a new worktree. A worktree you make with `git worktree add` yourself doesn't.

To see how a setup went, run `kero worktree status` in the session, which prints its setup's steps, or read `kerod.log` ([Troubleshooting](./troubleshooting.md#logs)), which logs `setup script of <worktree> runs in pane <pane>: <command>` and how each setup ended.

## Closing a session in a worktree

Closing the session asks whether its worktree goes too:

- **Nothing uncommitted:** Remove Worktree is the default. The branch goes with it once it's merged; otherwise it stays, with its commits.
- **Uncommitted changes:** Kero says how many files would be lost, and Keep Worktree is the default.
- **Another session works in the worktree:** it stays, and Kero doesn't ask.

```sh
kero close fix-login --remove-worktree           # close it and remove the worktree
kero close fix-login --remove-worktree --force   # even with uncommitted changes
```

<a id="moving-a-session-into-a-worktree"></a>

## Moving a session into a worktree

An agent already at work in your checkout can move its own session, with its conversation, into a worktree of its own, rather than start another. `kero worktree handoff <branch>`, run by the agent in its pane, makes the worktree as a new session's is made, on a new branch from the default branch (or as `worktree-base` says, or `--base`), and makes it the session's. Once the agent's turn ends, Kero starts it again in the same folder of the worktree, resuming its conversation, and types the `--prompt` it was given, if any. Its other panes stay where they are, and so do changes it didn't commit. Kero's skill teaches agents to use it. A session with a worktree of its own already can't move again.

```sh
kero worktree handoff fix-login                          # from the default branch
kero worktree handoff fix-login --base HEAD --prompt "go on with the fix"
```

<a id="the-worktrees-section"></a>

## The Worktrees section

The History tab's **Worktrees** section, under the stash, lists the repository's worktrees, each with what removing it would lose: changed files and commits on no remote in orange, or *merged* or *pushed* in green when nothing would be lost. **Push** pushes a worktree's branch, **Open** shows the session working in it or starts one, and **Remove Worktree** asks first: a worktree with changes is removed only when you say **Remove Anyway**, one with commits not pushed keeps its branch, and one with nothing to lose can go with its branch, **Remove with Branch**. A worktree a session was made for closes with the session. Its menu also copies its path, merges its branch into yours, and **Prune Missing Worktrees** forgets those whose folders were deleted. **+** starts a session in a new worktree.

The branch switcher lists the worktrees too, under **Worktrees**: double-click one to open its session.

<a id="cleaning-up-worktrees"></a>

## Cleaning up worktrees

Worktrees add up. **Settings › Git › Worktree cleanup** has Kero remove the ones it made, as it starts, every hour, and when you click **Delete Now**. It's off until you turn it on. A worktree goes when any rule you choose matches:

- **Inactive for** some days: no commit, checkout or session in it for that long.
- **Pull request merged**: its branch's pull request on GitHub merged, at the commit the worktree is on. Kero asks `gh`, so this needs it signed in.
- **No commits of its own**: nothing on its branch beyond the default branch.
- **Last session closed**: no session works in it anymore, as when you closed the session and kept the worktree.

Kero removes only worktrees in its own folder (`.kero/worktrees`, or `worktree-dir`), and never one a session works in, one git keeps locked, one made in the last ten minutes, or one with anything of its own: uncommitted changes, untracked files, or ignored files a build can't make again. Ignored files that are copies of the main checkout's, as `.worktreeinclude` brings, and folders such as `node_modules`, `dist`, `build`, `target` and `.venv` don't count. A branch goes with its worktree once the default branch has all of it; otherwise it stays, with its commits.

**Project** picks a project of this computer to give rules of its own, **Custom**, or none, **Off**, which keeps its worktrees. A repository with a project turned off keeps its worktrees, whatever its other projects say.

## From the command line

```sh
kero worktree list -p api        # the worktrees: sessions in each, Kero's, what removing loses, the cleanup's verdict
kero worktree status             # this session's worktree, its setup, pull request, and whether handoff would go
kero worktree create fix-login -p api -- claude "fix the login"
kero worktree remove fix-login -p api --branch   # as the History tab removes it, its branch too
kero worktree cleanup --dry-run  # what the automatic cleanup would remove, now
kero worktree cleanup
kero worktree prune -p api       # forget worktrees whose folders are gone
```

`-d <folder>` names the repository by a folder of it, in place of `-p <project>`; with neither, it is the current folder's. `kero worktree remove` removes a worktree as the History tab does: a worktree Kero made for a session closes that session, and one that other sessions work in stays. `--json` prints `list`, `status` and `cleanup` as JSON.
