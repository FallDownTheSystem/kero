<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: assistants

An assistant is one of the user's agents of no project, with a name, a
role and checks Kero wakes it for while Kero runs: daily, weekly, or when
Kero starts. Each has a folder of its own under Kero's configuration and
one session at most, which shows in the sidebar's Assistants view.
$KERO_ASSISTANT is set, to its id, in an assistant's panes.

## As an assistant

Your instructions are CLAUDE.md (or AGENTS.md) in your folder, which Kero
writes from your settings; USER.md is the user's, and MEMORY.md your own
notes, which Kero never changes. Read both before each check.

A check comes as "Kero: message from your schedule:", once your agent is
at its prompt, listing the checks due and when each was due. A check due
while Kero was not running comes once, late, and says so.

- Keep your status line current, as reporting.md says:
  `kero report working "Reading today's Jira tickets"`, then `done` with
  the result.
- End every check with a report to the user, which Kero keeps and Ama
  relays when the user asks for news:

      kero assistants report - <<'EOF'
      Result: …
      Not done: …
      Where: …
      Next: …
      EOF

  The four parts, and starting with "Waiting on you:" when you need the
  user, are as reporting.md describes (`kero skill show reporting`). A
  report starting "Waiting on you:" shows the assistant waiting on its
  question until the user answers, in your session or by a message sent
  to you (as Ama's kero message), or you report again; a check or a
  child's report reaching you meanwhile leaves it. When nothing
  changed, one line says so. `kero report --to-parent` from your session
  does the same.
- After each check, write in MEMORY.md what the next one needs, dated,
  and keep it under 200 lines.

Do short work in your folder yourself. Work in a repository, or work that
runs long, goes to a child session, which you start in its project or
folder (one or the other is required: your folder is no project's):

    kero new -p <project> -n <name> [--worktree <branch>] -- <agent> '<brief>'
    kero new -d <folder> -n <name> -- <agent> '<brief>'

Brief it as orchestrating.md says (the goal in the user's words, what is
decided, what done looks like, how to report). Its reports come to you as
"Kero: message from your child sessions:"; answer it, and give it more
work, with `kero message <child> "<text>"`, never kero prompt. Close a
child once its work is merged, or has nothing to ship: `kero close <child>`.
Your children count against orchestrator-max-children.

Ask before what can't be undone: mail or messages in the user's name,
changing tickets, deleting branches, merging.

## Managing assistants

Create, change or remove one only as the user asked.

    kero assistants [ls] [--json]                  # each, how it stands, its next check
    kero assistants show <assistant> [--json]      # its definition, checks, folder, session
    kero assistants create <name> --role <text> [--tracks <text> | --tracks-file <file>] [--group <g>]
          [--color <name|#rrggbb>] [--avatar <glyph|image>] [--agent <command>] [--args <args>] [--check <spec>]… [--no-start]
    kero assistants edit <assistant> [--name <n>] [the flags of create] [--remove-check <id>]…
    kero assistants pause|resume <assistant>       # its checks stop, or run again
    kero assistants run <assistant> [<check id>]   # its checks, or one, now
    kero assistants start <assistant> [--new]      # its session; --new, a fresh chat
    kero assistants remove <assistant> [--delete-folder]

A check's spec is `start`, `daily@09:00` or `weekly@mon,thu@09:00`, with
`=<what to check>` after it, as `--check 'daily@09:00=What moved in my
Jira tickets?'`. An assistant is named by its id, a prefix of it, or its
name; kero message, read, status and focus take one for its session.
A color is one of red, orange, amber, green, teal, blue, purple, pink,
or `#rrggbb`. An avatar glyph is one of star, heart, bolt, moon, sun,
leaf, bell, mail, flag, cat, dog, fox, owl, rabbit, bear, bird, fish,
bee, turtle, ghost.

## News

`kero assistants news --json` tells, for each assistant: its name and
role, its status and status line, needsYou with its question, its last
report in full with its time, unread, its childrenWaiting, its nextCheck,
paused, and a problem when its checks don't run. Lead with those that
need the user, then a line for each other.
