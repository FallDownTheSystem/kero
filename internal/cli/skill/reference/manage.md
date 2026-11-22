<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: manage Kero

## Manage Kero

What the app's sidebar, dialogs and Settings do, these do from a shell;
Kero's helper agent, Ama, works with them:

    kero ls -p api --status waiting           # sessions of a project, in these statuses
    kero projects add ~/code/api -n api       # a project of a folder; prints its id
    kero projects rename api backend
    kero projects remove backend --force      # --force: it has sessions, which close
    kero move <session> <project>
    kero close --all -p api --status idle,exited --dry-run   # see which, then without --dry-run
    kero close <session> <session>…           # several at once
    kero worktree create fix-login -p api -- claude "fix the login"   # a session in a new worktree
    kero worktree remove fix-login -p api [--branch] [--force]
    kero worktree cleanup --dry-run           # what the automatic cleanup would remove; then without
    kero worktree prune -p api                # forget worktrees whose folders are gone
    kero pr list -p api [--json]              # the repository's open pull requests
    kero pr review 123 -p api                 # an agent's review in a worktree, as the Review tab starts it
    kero config get default-agent             # a setting, by the key kero config show lists
    kero config set notify-on-done false      # write one, as Settings does

Every setting of the Settings window has a key, the prompts and a GitHub
repository's own too (review-prompt, review-command,
repo-commit-message-prompt…): change one by its key, never by editing the
file. Options go before the key. To change a prompt, take it as its lines
into a file, edit that, and set it from the file; an empty prompt is
Kero's own, which --default gives:

    kero config get --text pr-review-prompt > prompt.md   # empty: Kero's own,
    kero config get --default pr-review-prompt > prompt.md   # which this gives
    kero config set --file prompt.md pr-review-prompt
    kero config get --repo acme/web review-prompt         # one repository's alone
    kero config set --repo acme/web --file prompt.md review-prompt   # the others stay
    kero config set --repo acme/web review-prompt         # no value: removed

kero close with --all, -p or --status never closes Ama's session, nor the
one it runs in. kero pr review needs the app open, and Remote Control, as
kero do does, unless Ama runs it. Close, remove and change settings only
as the user asked, and show them what --dry-run lists first.
