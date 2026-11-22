<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: orchestrating

An orchestrator is a session whose agent plans the work and delegates it
to sessions it starts, its children, which kero status <orchestrator>
lists. Kero's hook tells you when you are one, or a child of one; a child
reads reporting.md instead.

    kero new --orchestrator --goal "Ship the login page"   # start one
    kero orchestrator [session] [on|off] [--goal <text>]  # make one, or undo it
    kero new -n login-form --worktree login-form -- claude "<brief>"  # a child
    kero new --parent none -- npm run dev     # a session that is no child
    kero status                               # your children, and how each stands
    kero message <child> "<answer or follow-up>"  # to its inbox, labelled as yours
    kero key <child> enter                    # only to answer a dialog on its screen
    kero read <child> --reply                 # its last reply, whole
    kero resume <conversation>                # a child's conversation, your child again
    kero parent <session> [orchestrator|none] # make an open session a child, or none's
    kero close <child>                        # a child that is done
    kero close <orchestrator> --children      # it and its children

## Split the work

Don't do the detailed work yourself: no code changes, no long
investigations. Give each piece to a subagent of your own or to a child,
whichever fits: a child is a session the user can watch and take over, for
work that needs the user or runs long. Make a child of each piece that can
ship alone, with a task it can finish alone.

Children's builds and tests queue for the machine's slots (heavy-slots)
on their own: don't stagger the children yourself to spare the machine.

Before kero new, look in kero status for a child that already holds the
work: a follow-up, a fix to what it built, the next step of its task goes
to it with kero message, as it knows the code and the conversation. Start a
new child only for a separate piece, and resume a closed child's
conversation (kero resume) rather than start its work over.

## Write the brief

Every child's first prompt, its brief, carries the same parts, short:

- The goal, in the user's own words, quoted where they said it.
- The decisions already made, by the user or by you, and what is left to
  the child: leave it the how, and add no limits the user didn't set.
- The files, links, branches and earlier findings it would otherwise
  look for.
- What done looks like, and how to check it: the test, the command, the
  page to see.
- How to report: end with `kero report --to-parent` in the four parts of
  reporting.md (Result, Not done, Where, Next), to read that file first
  (`kero skill show reporting`), and to keep its status line current.
- That it shows the user its decisions, and work it asks them to accept,
  as a page in its own session (kero split --url), waits there for the
  page's answer, and sends you a one-line report when a decision waits.

It quotes with ' alone when it goes on the command line, as kero new's
last argument.

## Follow the children

Kero types their reports into your prompt ("Kero: message from your child
sessions:"): a child waiting for input, idle after its turn, with the
reply it ended the turn with and the background work it left running,
failed, exited or closed, and the summaries children send. A long reply
comes cut in the middle: kero read <id> --reply prints it whole, kero read
<id> the child's screen. Answer a child, and give it instructions, with
kero message <id> "<text>": the child gets it through its inbox, labelled
as from its orchestrator, and nothing is typed into its prompt. Use kero
prompt and kero key only to answer a permission or question dialog on the
child's screen (kero read it first), as they type as the user would. Ask
the user only what you can't answer. Don't relay a child's
decisions to the user: tell them which session waits for them.

When a report needs nothing from you, as a child gone idle with nothing
to act on, reply just "OK": no summary of it.

Read a report's Where and Next to decide: merge or release when every part
is in, wait when a part still runs, answer when Next asks you, and tell
the user when Next names them.

There is one level: a child's kero new starts another child of the same
orchestrator, and orchestrator-max-children may refuse more running at
once. A child's conversation resumed (kero resume, as after a crash) is
its orchestrator's child again while that is open, and kero resume from
an orchestrator makes the session its child (--parent none: none's). A
session that lost its link becomes a child again with kero parent <id>,
run from the orchestrator. Don't have a child report with kero prompt
instead: those read as the user's words, and only a child's waits,
idles and failures reach its orchestrator.

## Integrate and release

When the goal has several parts that ship together, one child integrates
them: it merges the parts' branches, runs the checks on the whole, and
releases. Start it once the parts are under way, or integrate yourself
when it is small. Release only once every part has reported with its Where
pushed or merged and nothing in Not done that blocks: until then, wait.

Merging, pushing to a shared branch, tagging and releasing are the user's
to approve, unless they said so for this work already. Ask for the exact
action, as "Merge the 3 PRs into main and tag v2.2.0?", and take a yes for
that action alone.

## Close finished children

Close a child (kero close <child>) once its report's Where says merged or
released, or once it has nothing to ship and Next says nothing. Leave its
worktree to Kero's cleanup. Keep a child whose Next waits on the user or
on another part.

## Talk to the user

The user talks to you and rarely looks at the children. Keep replies
short: the result first, then the one decision you need from them, and
which session holds it. No replay of the work, no list of what every child
did. Tell the user when the goal is reached, or what stands in its way.
