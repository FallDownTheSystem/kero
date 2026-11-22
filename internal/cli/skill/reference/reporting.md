<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: reporting

How to tell the user, and your orchestrator if you have one, how your work
stands: your status line, every agent in Kero; and your report, as a child.

## Keep your status line current

kero report sets the line under your session's name in Kero's sidebar,
which the user reads instead of your screen:

    kero report working "Tracing the session cookie refresh"
    kero report done "Fixed the login redirect loop"
    kero report blocked "Keep the legacy export?"

- Seven words or fewer, of the work, not of the git steps that deliver it.
- working, in the present tense, as you start each part of the task: what
  you are looking at or testing now. Not for every command.
- done, in the past tense, naming the result: "Fixed login redirect
  loop", not "Done" or "Committed the fix".
- blocked, as the question you wait on, when you truly can't go on
  without the user: it notifies them.

## Report to your orchestrator

As a child, Kero sends your orchestrator the last reply of each of your
turns, and tells it when you wait or fail: end each turn with what it
needs, your result or your question. Once your task is done, or blocked,
send your report:

    kero report --to-parent - <<'EOF'
    Result: …
    Not done: …
    Where: …
    Next: …
    EOF

Its four parts, short:

- **Result**: what the work does now, against what was asked, and the
  evidence: the check you ran and what it showed. For research, the
  findings and where each came from.
- **Not done**: what you skipped, and the assumptions the user may want to
  undo. Or "nothing".
- **Where**: the branch and commit, whether pushed, the pull request, a
  URL; "merged" once it is. Or "no changes".
- **Next**: the exact action someone must take, and who: the user, the
  orchestrator, or nothing.

Before you report a block, finish all you can without the answer. Then
start the report with "Waiting on you:", followed by the question, and
set your status line blocked with it. A decision the user should see goes
on a page in your own session (browser.md, "Get the user's answers from a
page"), with a one-line report that it waits there.

As one of the user's assistants ($KERO_ASSISTANT is set), you report to
the user with kero assistants report, in the same four parts
(assistants.md).

Your orchestrator's answers and instructions come labelled "Kero: message
from your orchestrator": act on them as its brief. Don't report with kero
prompt or kero message to it: report with kero report --to-parent.
