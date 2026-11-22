<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: worktrees and pull requests

## Worktrees

What git worktree list doesn't tell: the sessions in each worktree, which
Kero made, what removing one would lose, and what the automatic cleanup
will do with it; and of your own session, the worktree Kero made for it,
its setup script's state, its pull request, and whether a handoff would go:

    kero worktree list                 # --json for all of it
    kero worktree status               # read it before a handoff

## Move into a worktree

To go on with this conversation in a git worktree of its own, on a new
branch, move your own session there, rather than start another:

    kero worktree handoff fix-login           # from the default branch
    kero worktree handoff fix-login --base HEAD --prompt "go on with the fix"

Make it the last thing you do, then end your turn: once it ends, Kero
starts you again in the worktree, resuming this conversation, and types
--prompt, if given, for you to go on with. Changes not committed stay in
this checkout: to take your work along, commit it first and hand off with
--base HEAD, which starts the branch from this checkout's. A session that
has a worktree of its own already, or works in a linked worktree, can't
move again: kero worktree status says so.

## Your pull request

Kero links your session to the pull request of its branch once you push
it and open one, and watches it while the session is open: it types a
message starting "Kero PR watch:" into your prompt when its checks fail,
and, as the user set it, when they pass, when comments or reviews come,
and when it has merge conflicts. Act on it as the user's request. Link
another by hand, or see how it stands:

    kero pr link 123        # or its URL; with neither, the branch's
    kero pr status          # --json for all of it
    kero pr unlink

## Act on a pull request

These act on GitHub as the user, through gh, on the pull request given by
its number, else the one your session is linked to:

    kero pr state                      # open, draft, mergeable, methods
    kero pr ready                      # or draft, close, reopen
    kero pr update-branch --rebase     # bring it up to date with its base
    kero pr merge --head "$(git rev-parse HEAD)"   # the repository's default method
    kero pr merge 123 --squash --delete-branch     # or --merge, --rebase
    kero pr merge --auto               # once its checks and reviews pass
    kero pr delete-branch              # once it is closed or merged
    kero pr checkout 123               # its branch here: refuses changes
    kero pr checkout 123 --worktree -- claude      # a new session in a worktree

Merge, close and delete a branch only when the user asked for it: a
merge can't be undone. A yes covers the one action it was given for
(this merge, this push, this tag), and carries over to no other: ask
again, naming the action, for the next. Pass --head with the commit you checked, so
that a merge refuses commits pushed since. A worktree that has the pull
request already is used as it is: kero pr checkout says where, rather
than check it out twice.
