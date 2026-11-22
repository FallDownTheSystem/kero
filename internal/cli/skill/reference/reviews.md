<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: reviews and review guides

## Review a pull request

When the user starts a review of a pull request from Kero, your comments
go to a draft review, which the user reads, edits and posts to GitHub from
Kero. Never post them yourself, with gh or otherwise:

    kero pr comment src/parse.go:42 "This drops the last token."
    kero pr comment src/parse.go:40-44 - <<'EOF'
    A comment of several lines,
    read from stdin.
    EOF
    kero pr comment src/old.go:10 --base "Why remove this?"   # the base's lines
    kero pr comment src/api.go "Split this file?"   # the whole file, no lines
    kero pr summary "Two bugs in the parser; the rest looks good."
    kero pr show                               # the draft so far, with ids
    kero pr edit 3 "Clearer text"              # or delete 3

Lines are the file's line numbers at the pull request's head (of the base
with --base), which is checked out where you review it. GitHub takes
comments only on lines of the diff, a change and the three lines around
it: kero pr comment says which lines a file has when it refuses one. A
path with no lines comments on the whole file. The
user's own comments show in kero pr show too; don't remove them. Outside a
review the user started in your pane, name the pull request: --pr 123;
with no draft yet, ask the user to click Start Draft on it in Kero.

## Write a review guide of a pull request

A review guide is a tour of a pull request that the user reads in Kero's
Guide tab instead of reading the diff line by line: a summary, then stops
in reading order, each on the lines worth reading with a note on them. It
is not a review: write no comments for its author, and post nothing:

    kero pr guide summary "Moves retries onto a queue; the worker is new."
    kero pr guide stop src/queue.go:10-42 --title "The new queue" - <<'EOF'
    Why it exists and what to check in it.
    EOF
    kero pr guide stop src/api.go:88 --title "Who enqueues" --closely "Is dropping the old path safe?"
    kero pr guide stop testdata/golden.txt --title "Snapshots"   # whole file
    kero pr guide show                         # the stops so far, with ids
    kero pr guide edit 2 --title "Clearer"     # or move 2 --at 1, delete 2
    kero pr guide clear                        # start the tour again

Stops go in the order you add them (--at <n> puts one nth): start where
the change starts, follow it outward, and put mechanical changes (renames,
generated files, snapshots) last. Every changed file should be on some
stop. --closely marks a stop with a call only a maintainer can make. Lines
are the pull request's head's (the base's with --base) and must show some
of the diff; a path with no lines is the whole file. Outside a guide the
user started in your pane, name the pull request: --pr 123.
