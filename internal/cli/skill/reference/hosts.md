<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: other machines

## Hand work to another machine

The user may have added other machines to Kero, each with a kerod of its
own: `kero ls --all` lists the sessions here and on every one of them,
under the name that `-H` takes, before the command:

    kero ls --all [--json]                    # every machine's sessions; a host out of reach says why
    kero hosts                                # the machines, and their addresses
    kero -H build new -n tests -d ~/code/app -- go test ./...
    kero -H build new -n review -- claude     # an agent there
    kero -H build prompt review "run the tests and fix them"
    kero -H build wait review
    kero -H build read review -n 80
    kero -H build close review

Every command acting on sessions takes -H. An id or a name is of its own
machine's sessions: give the same -H each time you act on one. With -H,
your own session is no default target, so name one each time, even for
split or notify. Paths there are that machine's, and without -d a session
starts in its project's directory, or in the home. kero focus, kero do
and kero browser drive the user's app, which a host may do only with its
Remote Control on, in Settings › Hosts: if it is off, say so rather than
work around it.
