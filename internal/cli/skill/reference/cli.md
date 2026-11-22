<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: core commands

## Run something beside you

    kero split --down -- npm run dev          # a pane under yours; prints its id
    kero new -n review -- claude              # a session of its own
    kero new -n api -d ~/code/app -- npm run dev
    kero split --url localhost:3000           # a browser pane, to show the user a page
    kero split --file src/main.go             # an editor pane, to show the user a file
    kero split --file notes/plan.md           # a file not there yet: saving it makes it
    kero new --file src/main.go               # an editor in a session of its own
    kero do files.show path=src/main.go       # show it in the side panel's Files view
    kero do changes.show                      # show the worktree's changes

## Show the user a document

    kero split --file plans/plan.md --preview # Markdown, rendered as it reads
    kero split --url .backlog/pages/brief.html # a local HTML page, scripts from CDNs too
    kero do pane.preview off                  # the plan's text, to edit; on: rendered again

The pane follows the file: rewrite it, and the pane shows the new one,
unless the user has unsaved edits in it. Running the same kero split again
focuses the pane that shows the file instead of opening another, and says
so on stderr.

## Drive it

    kero send <target> "some text" --enter    # type, then press Enter
    kero key <target> ctrl+c                  # enter, escape, up, tab, ctrl+d…
    kero prompt <target> "run the tests"      # send + Enter, wait, print screen
    kero message <target> "<text>"            # to an agent, labelled as from you; - reads stdin
    kero wait <target> [--for idle,waiting,error,exited] [--timeout 10m]
    kero wait <target> --exit                 # until its command ends; exits with its status
    kero new --worktree <branch> -- claude    # an agent on a new branch, in a worktree of its own
    kero events [target] [--for waiting,error] [--json]
                                              # a line per change, as it happens
    kero history [query] [--json]             # agents' past conversations, to resume
    kero resume <conversation> [--parent <session|none>]  # a session resuming one
    kero read <target> [-n 40] [-a]           # the screen, or with scrollback
    kero status <target> --json               # status, program, directory
    kero ls                                   # every session and pane here

To tell another agent something, message it: kero message gives it the
text once it is at its prompt, through its inbox, labelled with your
session, so that it doesn't take it for the user's words; --json says
whether it was delivered, queued or went nowhere. kero prompt and kero key
type as the user would: keep them for an agent's permission or question
dialog, and for programs that aren't agents.

A pane's status is working, waiting (it needs the user), idle, error or
exited. `kero wait` with no --for waits until it stops working. A
command such as a test run doesn't show as working: wait for it with --exit,
or start it with --wait, which exits with the command's status:

    pane=$(kero split --down -- go test ./...)
    kero wait "$pane" --exit || kero read "$pane" -n 80
    kero split --down --wait -- make build    # the same, in one step

## Tell the user

    kero report working "Migrating the database" --progress 40
    kero report blocked "Approve the production deploy?" --kind permission
    kero report done "Deployed to staging"
    kero notify "Need your API key to continue"

Reports are OSC 7501 sequences: Kero shows them in its sidebar, and
notifies the user when you are blocked or done while they look elsewhere.
Report blocked only when you truly cannot go on without them. What to
write in each, and how to keep the line current, is in reporting.md.
