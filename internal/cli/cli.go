// Package cli is the kero command: sessions driven from a shell or by an
// agent, through the daemon the app shows.
package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"kero/internal/actions"
	"kero/internal/build"
	"kero/internal/client"
	"kero/internal/proto"
	"kero/internal/service"
	"kero/internal/settings"
	"kero/internal/update"
)

const usage = `kero — terminal sessions that keep running, for you and your agents

Usage:
  kero                          open the app
  kero ls [--json]              list sessions and their panes
      -a, --all                   on this machine and every host of kero
                                  hosts, each under the name -H takes
      -p, --project <name|id>     only the project's
      -s, --status <statuses>     only those in these: idle, working,
                                  waiting, error, exited (comma-separated)
  kero new [options] [-- cmd…]  start a session, printing its id
      -p, --project <name|id>     project to put it in (default: by directory)
      -n, --name <name>           name it
      -d, --dir <dir>             start it there (default: here)
      -f, --focus                 show it in the app
      -w, --wait                  wait for cmd to end, exit with its status
      -u, --url <url>             a browser showing url, not a shell; or
                                  a local file, as page.html
      --file <path>               an editor of the file, not a shell; a file
                                  not there yet is made as it is saved
      --preview                   with --file of Markdown: show it as it
                                  reads, not its text
      --worktree <branch>         in a git worktree of its own, on a new
                                  branch from the default branch
      --base <ref>                the branch to start it from instead
      --orchestrator              an orchestrator: its agent (cmd, else the
                                  default-agent setting's) starts on the
                                  role prompt, to plan the work and start
                                  sessions for it, its children
      --goal <text>               the orchestrator's goal
      --parent <session|none>     the orchestrator it is a child of (default:
                                  this session's, run in an orchestrator or
                                  in a child of one; none for no parent)
  kero orchestrator [session] [on|off] [--goal <text>]
                                make a session an orchestrator, its agent
                                given the role prompt, or with off an
                                ordinary session again, whose children
                                become top-level sessions
  kero split [target] [options] [-- cmd…]
                                start a pane beside a pane, printing its id
      --right (default), --down, --left, --up
      -r, --ratio <0..1>          the new pane's share (default 0.5)
      -d, --dir <dir>             start it there (default: the pane's)
      -w, --wait                  wait for cmd to end, exit with its status
      -u, --url <url>             a browser showing url, not a shell, as
                                  a dev server's page: localhost:3000, or
                                  a local file's: docs/brief.html
      --file <path>               an editor of the file, not a shell; a file
                                  not there yet is made as it is saved
      --preview                   with --file of Markdown: show it as it
                                  reads, not its text
                                a file or local page a pane of the session
                                shows already focuses that pane instead,
                                which shows the file as it changes on disk
  kero send <target> <text…>    type text into a pane
      -e                          interpret \n \r \t \e \xHH escapes
      --enter                     press Enter after it
  kero key <target> <key…>      press keys: enter, tab, escape, up, down,
                                left, right, backspace, ctrl+c, ctrl+d, …
  kero prompt <target> <text…>  send text and Enter, wait until the pane
                                stops working, then print its screen
  kero read [target] [-a] [-n lines]
                                print the screen (-a with the scrollback)
      --reply                     print instead the last reply of the
                                  pane's agent, whole, from its transcript
  kero wait [target] [--for idle,waiting,error,exited] [--timeout 10m]
                                wait until the pane stops working (or
                                reaches one of the statuses), print it
      -x, --exit                  instead wait for the command it started
                                  with to end, exit with its status
  kero events [target] [-p project] [--for statuses] [--json]
                                print what happens as it does, a line an
                                event: panes changing status, sessions
                                and panes starting and closing, renames,
                                moves to another project, notifications;
                                until the target closes
      --for <statuses>            only panes going to these: idle,
                                  working, waiting, error, exited
      --once                      exit after the first event (1 if the
                                  target closes or the timeout comes first)
      -t, --timeout <dur>         stop watching after this long
  kero status [target] [--json]
  kero diff [target] [--turn | --branch] [path…]
                                the changes in the pane's git worktree, as
                                git diff prints them: since HEAD, untracked
                                files too; --turn since its agent last
                                started working; --branch since it left
                                the default branch
  kero browser [target] <command> [args…]
                                drive a browser pane's page (needs Remote
                                Control, in Settings › Agents); the target
                                is a browser pane, or a session, for its
                                browser (default: this session's)
      open <url>                  go to a page, and wait for it to load
      back, forward, reload       and wait for the page to load
      text [selector]             print the page's text, or an element's
      html [selector]             print the page's HTML, or an element's
      snapshot [selector]         print what the page shows: roles, names,
                                  and a ref for each element
      click <selector>            click an element
      type <selector> <text…>     type text into a field, in place of what
                                  it holds (--enter: press Enter after it)
      wait [selector] [--text t]  wait for an element to show (or the text
                                  to), or without either, the page to load
      eval <js…>                  run JavaScript, print its value
      screenshot [-o file.png]    save a picture of the page, print where
      cookies save [file]         save the browser's cookies (Playwright's
                                  JSON, or cookies.txt for a .txt file)
      cookies load <file>         load cookies from either
      cookies import <chrome|edge>
                                  copy your browser's cookies as it runs,
                                  and the localStorage and IndexedDB of the
                                  sites of its open tabs (with --domain, of
                                  that site, in a tab opened unseen if none
                                  is); turn on remote debugging in it first,
                                  at chrome://inspect/#remote-debugging, and
                                  allow Kero in its dialog
      wait-message [--json]       wait for the next message the page sends
                                  with window.kero.send, and print it
                                  (needs no Remote Control; no timeout
                                  unless given one)
      console [--errors] [--clear] [--json]
                                  print what the page logged, threw and
                                  the browser told of, with where from:
                                  since the pane's page was made (on macOS
                                  and Linux, its document); --clear
                                  empties the log after
      network [--failed] [--url s] [--clear] [--json]
                                  print the page's requests: method,
                                  status, type, size, time, URL, and why
                                  one failed (CORS, blocked, net::ERR_…)
      network body <id> [-o file] print a response's body (#id from the
                                  list)
      emulate device <name>       show the page as a phone or a tablet
                                  does (no name lists them)
      emulate viewport <w>x<h> [--scale n] [--mobile] [--touch]
      emulate user-agent <ua…>
      emulate network <offline|slow-3g|3g|4g|none>
      emulate reset               as it is again; emulate alone prints
                                  what the page is shown as (network
                                  and emulate: Windows only)
      --ref <ref>                 a snapshot's element, in place of a
                                  selector
      --domain <domain>           cookies of a site and its subdomains
      -t, --timeout <dur>         how long to wait (default 10s)
  kero browser devtools [--json]
                                the browser panes' DevTools port, for
                                Playwright's connectOverCDP and
                                chrome-devtools-mcp, and each pane's page
                                there (Windows; browser-devtools = true,
                                then start Kero again)
  kero history [query] [-p project] [-n count] [--json]
                                the agents' conversations panes ran, the
                                last to run first, to resume
  kero resume <conversation> [-f]
                                start a session resuming a conversation
                                (its id, a prefix of it, or its name), or
                                show the one running it, printing its id
  kero rename <session> <name>
  kero focus <target>           show a session (and pane) in the app
  kero close <target>           end a session (or, given a pane, the pane)
      --remove-worktree           remove the session's own worktree too,
                                  and its branch once merged
      --force                     even if that loses changes or commits
      --children                  an orchestrator's children too, which
                                  otherwise become top-level sessions
  kero close <target…> | --all [-p project] [--status statuses]
                                end several sessions: those named, or all
                                those of a project, in these statuses
                                (never Ama's, nor the one it runs in)
      -n, --dry-run               list them instead
  kero move <session> <project> move a session to another project
  kero restart <target>         start an exited pane's shell again
  kero worktree list [-d <dir>|-p <project>] [--json]
                                the repository's worktrees: the sessions in
                                each, whether Kero made it, what removing
                                it would lose, what the cleanup makes of it
  kero worktree status [--json]
                                this session's worktree: where it works,
                                the worktree Kero made for it and its
                                setup, its pull request, the settings new
                                worktrees take, whether handoff would go
  kero worktree create <branch> [-d <dir>|-p <project>] [--base <ref>] [-- cmd…]
                                a session in a new worktree of its own, on
                                a new branch, of the repository of -d (or
                                the project's folder, or here), printing
                                its id; as kero new --worktree
  kero worktree remove <branch|path> [-d <dir>|-p <project>] [--branch] [--force]
                                remove a worktree, as the History tab
                                does: one Kero made for a session closes
                                the session; --branch deletes its branch
                                too; --force even with changes
  kero worktree cleanup [--dry-run] [--json]
                                run the automatic worktree cleanup now, of
                                every project's repository (--dry-run: say
                                what it would remove)
  kero worktree prune [-d <dir>|-p <project>]
                                forget worktrees whose folders are gone
  kero worktree handoff <branch> [--base <ref>] [--prompt <text|->]
                                move this session into a git worktree of
                                its own, on a new branch from the default
                                branch (or as worktree-base says, or
                                --base): once this agent's turn ends, it
                                starts again there, resuming its
                                conversation, then goes on with --prompt
  kero notify [target] <message>
                                flag the pane as needing you
  kero pr comment <path>[:<line>[-<end>]] <text…|->
                                draft a review comment on lines of the
                                pull request's diff, or on the file with
                                no lines (- reads the text from stdin);
                                --base: on the base's lines
  kero pr edit <id> <text…|->   change a draft comment's text
  kero pr delete <id>           delete a draft comment
  kero pr summary <text…|->     the review's summary, its body on GitHub
  kero pr show [--json]         the draft review: summary and comments;
                                the user posts it from Kero
      --pr <number>               the pull request, when the review wasn't
                                  started in this pane from Kero
      --remote <name>             its repository's remote
  kero pr guide stop <path>[:<line>[-<end>]] --title <title> [note…|-]
                                add a stop to the pull request's review
                                guide, the tour of it in Kero's Guide tab;
                                a path alone is the whole file; --at <n>
                                puts it nth, --closely marks it Look
                                Closely, --base: on the base's lines
  kero pr guide edit <id> [--title t] [--closely|--not-closely] [note…|-]
  kero pr guide move <id> --at <n>
  kero pr guide delete <id>     delete a stop
  kero pr guide summary <text…|->
                                what the change does, atop the guide
  kero pr guide clear           remove its summary and stops, to rewrite it
  kero pr guide show [--json]   the guide: summary and stops, with ids;
                                --pr and --remote as for kero pr
  kero pr list [-d <dir>|-p <project>] [--mine] [--json]
                                the repository's open pull requests
  kero pr review <number> [-d <dir>|-p <project>] [--guide]
                                have an agent review it in a new session,
                                in a worktree of its branch, as the Review
                                tab does (--guide: write its review guide);
                                needs the app open, and Remote Control
                                but for Ama
  kero pr link [<number>|<url>] [--session s] [--remote r] [--json]
                                link the session to a pull request, which
                                Kero watches for its agent; with neither,
                                the open one of the session's branch
  kero pr unlink [--session s]  unlink it, which Kero then leaves unlinked
  kero pr status [--session s] [--json]
                                the session's pull request: its checks,
                                and whether it merges
  kero pr state [<number>] [--json]
                                whether a pull request is open, a draft,
                                and merges, and by which methods
  kero pr merge [<number>] [--squash|--merge|--rebase] [--delete-branch]
                                merge it on GitHub, by the repository's
                                default method unless given one; --auto
                                once its requirements are met, or
                                --disable-auto; --head <sha> only at it
  kero pr ready|draft|close|reopen [<number>]
                                mark it ready for review, make it a draft
                                again, close or reopen it
  kero pr update-branch [<number>] [--rebase]
                                bring its branch up to date with its base
  kero pr delete-branch [<number>]
                                delete its branch, once closed or merged
  kero pr checkout [<number>] [--worktree [-n name] [-- command…]]
                                check its branch out here, refusing a
                                worktree with changes; --worktree: in a new
                                session in a worktree of its own (one that
                                has it already, and its session); prints
                                the session's id
      (these take the session's linked pull request without a number,
      act as the user signed in to gh, and --remote <name> as kero pr)
  kero report <state> [message] [--kind k] [--progress n] [--app a] [--id i]
                                report this program's status (OSC 7501):
                                idle, working, done, blocked, error, clear
  kero report --to-parent <summary…|->
                                send this child session's orchestrator a
                                summary of its result, with its next
                                report of its children
  kero do <action> [key=value…]  run an action of the app, as its menus do
                                (needs Remote Control, in Settings › Agents,
                                but for Ama, whose $KERO_AMA kerod knows),
                                as kero do files.show path=src/main.go
      pane.preview [on|off] [pane=<target>]
                                  show an editor pane's Markdown as it
                                  reads, or its text (default: toggle;
                                  the session's Markdown editor pane)
  kero actions                 list the actions kero do runs
  kero config path|show|edit|check
                                the configuration file: where it is, every
                                key with its value (--defaults: as kero
                                comes), open it in $EDITOR, check it
  kero config get [--repo <owner>/<name>] [--text] [--default] <key>
                                a setting's value (a list's, a line each);
                                --text, a prompt as its lines; --default,
                                Kero's own, its prompt where the setting is
                                empty; --repo, that repository's line alone
  kero config set [--repo <owner>/<name>] [--file <path>|-] <key> [value…]
                                set a setting, as Settings does, keeping
                                the file's comments; no value for its
                                default (a list: each value a line, none
                                to empty it); --file, the value from a
                                file or stdin, as its lines; --repo, that
                                repository's line alone, the others kept
  kero projects [--json]        list projects, in the sidebar's order
  kero projects add <dir> [-n <name>] [--json]
                                add a project of a folder, printing its id
                                (the one there is, if any)
  kero projects rename <project> <name>
  kero projects remove <project> [--force]
                                remove a project from the sidebar; --force
                                when it has sessions, which close
  kero projects move <project> <position|top|bottom|up|down>
                                move a project in that order (1 is the
                                top)
  kero skill setup|status|show|remove
                                teach coding agents to use kero
  kero clients [--json]         list who is connected to kerod
  kero hosts [ls]               the other machines Kero reaches
  kero hosts add <name> <address> [--remote-control]
                                save one: [user@]host or ssh://… over SSH,
                                wsl://distro, http(s)://… for a listener
  kero hosts remove|check <name>
  kero hosts install <name|address> [--version v]
                                install Kero there, over SSH
  kero daemon status|stop|restart
  kero daemon serve [--listen tailnet://:7373]…
                                run kerod in the foreground, never idle,
                                as a service does
  kero daemon install-service|uninstall-service
                                run kerod with the machine (systemd, launchd)
  kero daemon update [version]  update the server build of Kero

Global options:
  -H, --host <name|address>     act on kerod on another machine: a host of
                                kero hosts, or an address ($KERO_HOST)

A target is a session or a pane: an id, a prefix of it, or a session's
name; a session stands for its focused pane. Inside kero, $KERO_PANE and
$KERO_SESSION name the pane and session a command runs in, the default
target.

Exit status: 0 ok, 1 failed, 2 bad usage, 3 no such target (or an
ambiguous one), 4 kerod unreachable.
`

// Commands lists the subcommands, which main sends here.
var Commands = []string{"ls", "list", "new", "split", "send", "key", "prompt", "read", "capture", "wait", "events", "status", "diff",
	"history", "resume", "rename", "move", "focus", "close", "kill", "restart", "notify", "pr", "worktree", "report", "orchestrator", "projects", "skill", "hook", "inbox", "daemon", "do", "actions", "config",
	"browser", "hosts", "clients", "__bridge", "-H", "--host", "help", "--help", "-h", "version", "--version"}

// IsCommand reports whether an argument starts a command of the CLI, as
// main tells them from the app's.
func IsCommand(arg string) bool {
	return slices.Contains(Commands, arg) || strings.HasPrefix(arg, "--host=") || strings.HasPrefix(arg, "-H") && len(arg) > 2
}

// Errors of exit statuses other than 1.
var (
	errUsage       = errors.New("usage")
	errNotFound    = errors.New("not found")
	errUnreachable = errors.New("unreachable")
)

// exitStatus is a waited-for program's exit status, which kero exits
// with.
type exitStatus int

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// Run runs a subcommand, returning the exit code.
func Run(args []string) int {
	SyncSkill()
	host, env, args, err := splitHost(args)
	if err == nil && host != "" && len(args) > 0 && localOnly[args[0]] {
		// $KERO_HOST is for what acts on sessions: the rest is of this
		// machine, as an agent's hook in a shell whose profile sets it.
		if env {
			host = ""
		} else {
			err = usageErr("kero %s is of this machine: run it on the host itself, without -H", args[0])
		}
	}
	if err == nil {
		onHost = host
		err = run(args, host)
	}
	if err == nil {
		return 0
	}
	var es exitStatus
	if errors.As(err, &es) {
		return int(es)
	}
	fmt.Fprintln(os.Stderr, "kero:", err)
	switch {
	case errors.Is(err, errUsage):
		return 2
	case errors.Is(err, errNotFound):
		return 3
	case errors.Is(err, errUnreachable):
		return 4
	}
	return 1
}

// hereFile makes a path the user typed absolute, of the shell's folder it
// is typed in, which kerod and the app have no idea of; on another host
// (-H) it stays as it is, for kerod there to make of its home.
func hereFile(path, host string) string {
	if path == "" || host != "" || filepath.IsAbs(path) || strings.HasPrefix(path, "~") {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// shownIn is what kero new or split (cmd) shows in place of a shell: the
// page --url names, a local file's as a file:// URL (localPage), or the
// file --file names, absolute (hereFile), its preview with --preview,
// which only a Markdown file has: asking for another's is a mistake to
// tell of, not one to leave the user looking at its text unawares.
func shownIn(cmd string, f *flags, command []string, host string) (page, file string, err error) {
	runs := len(command) > 0 || f.set["wait"]
	page, err = localPage(f.vals["url"], host)
	if err != nil {
		return "", "", err
	}
	if page != "" && runs {
		return "", "", usageErr("kero %s --url shows a page, and runs no command", cmd)
	}
	file = hereFile(f.vals["file"], host)
	if file != "" && (runs || page != "") {
		return "", "", usageErr("kero %s --file shows a file, and runs no command", cmd)
	}
	if f.set["preview"] && (file == "" || !proto.MarkdownFile(file)) {
		return "", "", usageErr("kero %s --preview shows a Markdown file as it reads: --file notes/plan.md --preview", cmd)
	}
	return page, file, nil
}

// localPage makes what kero split --url names a URL to load: a file of
// this machine, as .backlog/pages/brief.html or C:\docs\brief.html, of the
// shell's folder, is its file:// URL, which the browser pane loads from
// the disk, as it does the file's pictures and scripts beside it; the
// rest, as localhost:3000, stays for kerod to make a URL of. A path to an
// HTML file that isn't there is a mistake, not a site to look up. On
// another host (-H) it stays as it is, the app's disk being no host's.
func localPage(text, host string) (string, error) {
	if text == "" || host != "" || strings.Contains(text, "://") {
		return text, nil
	}
	path := hereFile(text, host)
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return proto.FileURL(path), nil
	}
	switch strings.ToLower(filepath.Ext(text)) {
	case ".html", ".htm":
		return "", fmt.Errorf("%w: no file %s", errNotFound, path)
	}
	return text, nil
}

// editorPane is the editor pane of a Markdown file kero do pane.preview
// acts on: the pane named, else of the session, its focused pane when it
// is one, else its one, as kero browser finds a session's browser.
func editorPane(t target, named bool) (*proto.Pane, error) {
	md := func(p *proto.Pane) bool { return p != nil && p.Editor() && proto.MarkdownFile(p.File) }
	if named && t.pane != "" {
		p := t.s.PaneByID(t.pane)
		if !md(p) {
			return nil, fmt.Errorf("%w: pane %s is no editor of a Markdown file", errNotFound, t.pane)
		}
		return p, nil
	}
	if p := t.s.FocusPane(); md(p) {
		return p, nil
	}
	var found []*proto.Pane
	var ids []string
	for i := range t.s.Panes {
		if p := &t.s.Panes[i]; md(p) {
			found, ids = append(found, p), append(ids, p.ID)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, fmt.Errorf("%w: session %s has no editor pane of a Markdown file (open one with kero split --file plan.md)", errNotFound, t.s.ID)
	}
	return nil, fmt.Errorf("%w: session %s has several editor panes of Markdown files, name one with pane=: %s", errNotFound, t.s.ID, strings.Join(ids, ", "))
}

// previewCmd is kero do pane.preview [on|off|toggle] [pane=<target>]: it
// sets what kerod keeps of the pane, as kero split --preview does, so it
// needs neither the app open nor Remote Control.
func previewCmd(c *client.Client, st proto.State, args []string) error {
	mode, ref := "toggle", ""
	for _, arg := range args {
		switch k, v, ok := strings.Cut(arg, "="); {
		case !ok && (arg == "on" || arg == "off" || arg == "toggle"):
			mode = arg
		case ok && (k == "pane" || k == "target"):
			ref = v
		default:
			return usageErr("kero do pane.preview [on|off|toggle] [pane=<target>]")
		}
	}
	var t target
	var err error
	if ref != "" {
		t, err = find(st, ref)
	} else {
		t, err = findOrCurrent(st, nil)
	}
	if err != nil {
		return err
	}
	p, err := editorPane(t, ref != "")
	if err != nil {
		return err
	}
	on := mode == "on" || mode == "toggle" && !p.Preview
	if err := c.Call("pane.preview", proto.PreviewParams{ID: p.ID, Preview: on}, nil); err != nil {
		return err
	}
	fmt.Println(p.ID)
	return nil
}

func usageErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errUsage}, args...)...)
}

// flags parses flags anywhere among the arguments: names that take a
// value, and booleans; the rest are positional, all of them after "--".
type flags struct {
	vals map[string]string
	set  map[string]bool
	pos  []string
	rest []string // after --
}

func parse(args []string, valued, boolean map[string]string) (*flags, error) {
	f := &flags{vals: map[string]string{}, set: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			f.rest = args[i+1:]
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			f.pos = append(f.pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if canon, ok := valued[name]; ok {
			if !hasVal {
				if i+1 >= len(args) {
					return nil, usageErr("%s needs a value", a)
				}
				i++
				val = args[i]
			}
			f.vals[canon] = val
			continue
		}
		if canon, ok := boolean[name]; ok {
			f.set[canon] = true
			continue
		}
		return nil, usageErr("unknown flag %s", a)
	}
	return f, nil
}

func run(args []string, host string) error {
	cmd, args := args[0], args[1:]
	switch cmd {
	case "__bridge":
		return bridge(args)
	case "hosts":
		return hostsCmd(args)
	case "help", "--help", "-h":
		u := usage
		if build.Server {
			// No app to open.
			u = strings.Replace(u, "  kero                          open the app\n", "", 1)
		}
		fmt.Print(u)
		return nil
	case "version", "--version":
		return versionCmd(host)
	case "daemon":
		return daemonCmd(args, host)
	case "report":
		if slices.Contains(args, "--to-parent") {
			return reportToParent(args)
		}
		return report(args)
	case "actions":
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ACTION\tKEYS\tWHAT")
		for _, a := range actions.All {
			if a.Remote {
				fmt.Fprintf(w, "%s\t%s\t%s\n", a.ID, actions.Glyphs(a.Keys), strings.TrimSuffix(a.Title, "…"))
			}
		}
		return w.Flush()
	case "skill":
		return skillCmd(args)
	case "hook":
		return hookCmd(args)
	case "inbox":
		return inboxCmd(args)
	case "config":
		return configCmd(args)
	}
	if cmd == "ls" || cmd == "list" {
		f, err := parse(args, filterFlags, map[string]string{"json": "json", "j": "json", "all": "all", "a": "all"})
		if err != nil {
			return err
		}
		if f.set["all"] {
			return listAll(f.set["json"])
		}
	}
	c, done, err := connect(host)
	if err != nil {
		if errors.Is(err, client.ErrVersion) {
			return fmt.Errorf("%w: the running kerod is of another version: run `kero daemon restart` (it restarts sessions)", errUnreachable)
		}
		if errors.Is(err, errUnreachable) || errors.Is(err, errNotFound) {
			return err
		}
		return fmt.Errorf("%w: %v", errUnreachable, err)
	}
	defer done()
	if cmd == "clients" {
		return clientsCmd(c, args)
	}
	var st proto.State
	if err := c.Call("state", nil, &st); err != nil {
		return err
	}

	switch cmd {
	case "ls", "list":
		f, err := parse(args, filterFlags, map[string]string{"json": "json", "j": "json", "all": "all", "a": "all"})
		if err != nil {
			return err
		}
		sf, err := newFilter(st, f)
		if err != nil {
			return err
		}
		if sf.any() {
			st.Sessions = sf.pick(st)
		}
		if f.set["json"] {
			return printJSON(st.Sessions)
		}
		listSessions(st)
		return nil

	case "move":
		return moveSession(c, st, args)

	case "projects":
		if len(args) > 0 && slices.Contains([]string{"add", "rename", "remove", "rm", "close"}, args[0]) {
			return projectsCmd(c, st, args[0], args[1:])
		}
		f, err := parse(args, nil, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		switch {
		case len(f.pos) > 0 && f.pos[0] == "move":
			return moveProject(c, st, f.pos[1:])
		case len(f.pos) > 0 && f.pos[0] != "ls" && f.pos[0] != "list":
			return projectsCmd(c, st, f.pos[0], nil)
		}
		if f.set["json"] {
			return printJSON(st.Projects)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSESSIONS\tDIR")
		for _, p := range st.Projects {
			n := 0
			for _, s := range st.Sessions {
				if s.Project == p.ID {
					n++
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", p.ID, p.Name, n, proto.ShortDir(p.Dir))
		}
		return w.Flush()

	case "new":
		f, err := parse(args,
			map[string]string{"p": "project", "project": "project", "n": "name", "name": "name", "d": "dir", "dir": "dir", "timeout": "timeout", "t": "timeout",
				"u": "url", "url": "url", "file": "file", "worktree": "worktree", "base": "base", "goal": "goal", "parent": "parent"},
			map[string]string{"f": "focus", "focus": "focus", "json": "json", "wait": "wait", "w": "wait", "preview": "preview", "orchestrator": "orchestrator"})
		if err != nil {
			return err
		}
		command := append(f.pos, f.rest...)
		if f.set["wait"] && len(command) == 0 {
			return usageErr("kero new --wait needs a command: kero new --wait -- cmd…")
		}
		page, file, err := shownIn("new", f, command, host)
		if err != nil {
			return err
		}
		// The file is resolved here, in this checkout, not in the worktree
		// the session would start in.
		if file != "" && f.vals["worktree"] != "" {
			return usageErr("kero new --file shows a file of this checkout, not of a new --worktree")
		}
		if f.vals["base"] != "" && f.vals["worktree"] == "" {
			return usageErr("kero new --base starts a --worktree's branch")
		}
		orch := f.set["orchestrator"]
		switch {
		case f.vals["goal"] != "" && !orch:
			return usageErr("kero new --goal is an --orchestrator's")
		case orch && (page != "" || file != ""):
			return usageErr("kero new --orchestrator starts an agent, not a browser or an editor")
		case orch && f.vals["parent"] != "" && f.vals["parent"] != proto.NoParent:
			return usageErr("an orchestrator has no --parent: there is one level of them")
		}
		parent := f.vals["parent"]
		if parent != "" && parent != proto.NoParent {
			pt, err := find(st, parent)
			if err != nil {
				return err
			}
			parent = pt.s.ID
		}
		dir := f.vals["dir"]
		// Ama's folder is no project's: the sessions it starts go where
		// it says.
		if dir == "" && f.vals["project"] == "" && host == "" && amaToken() != "" {
			return usageErr("kero new from Ama: name the project (-p) or the folder (-d) the session is for")
		}
		// Here, unless the session is of another machine, whose kerod
		// then starts it in the project's directory, or the home.
		if dir == "" && f.vals["project"] == "" && host == "" {
			dir, _ = os.Getwd()
		}
		var s proto.Session
		if err := c.Call("create", proto.CreateParams{
			Project: f.vals["project"], Name: f.vals["name"], Dir: dir, Command: command, URL: page, File: file, Preview: f.set["preview"],
			After: os.Getenv("KERO_SESSION"), Worktree: f.vals["worktree"], Base: f.vals["base"],
			Orchestrator: orch, Goal: f.vals["goal"], Parent: parent, By: current(),
		}, &s); err != nil {
			return err
		}
		if f.set["focus"] {
			c.Call("select", proto.FocusParams{Session: s.ID, Token: amaToken()}, nil)
		}
		if f.set["json"] {
			if err := printJSON(s); err != nil {
				return err
			}
		} else {
			fmt.Println(s.ID)
		}
		if f.set["wait"] {
			return waitExit(c, s.Panes[0].ID, f.vals["timeout"])
		}
		return nil

	case "split":
		f, err := parse(args, map[string]string{"r": "ratio", "ratio": "ratio", "d": "dir", "dir": "dir", "timeout": "timeout", "t": "timeout", "u": "url", "url": "url", "file": "file"},
			map[string]string{"right": "right", "down": "down", "left": "left", "up": "up", "json": "json", "f": "focus", "focus": "focus", "wait": "wait", "w": "wait", "preview": "preview"})
		if err != nil {
			return err
		}
		t, err := findOrCurrent(st, f.pos)
		if err != nil {
			return err
		}
		dir := "right"
		for _, d := range []string{"down", "left", "up"} {
			if f.set[d] {
				dir = d
			}
		}
		ratio, _ := strconv.ParseFloat(f.vals["ratio"], 64)
		command := append(f.pos[min(1, len(f.pos)):], f.rest...)
		if f.set["wait"] && len(command) == 0 {
			return usageErr("kero split --wait needs a command: kero split --wait -- cmd…")
		}
		page, file, err := shownIn("split", f, command, host)
		if err != nil {
			return err
		}
		var s proto.Session
		if err := c.Call("split", proto.CreateParams{Split: t.ref(), Direction: dir, Ratio: ratio,
			Dir: f.vals["dir"], Command: command, URL: page, File: file, Preview: f.set["preview"]}, &s); err != nil {
			return err
		}
		if t.s.PaneByID(s.Focus) != nil {
			// kerod focused the pane already showing it (reuse): said
			// apart from the id, which a script reads.
			fmt.Fprintf(os.Stderr, "pane %s shows it already: focused it, rather than split\n", s.Focus)
		}
		if f.set["focus"] {
			c.Call("select", proto.FocusParams{Session: s.ID, Pane: s.Focus, Token: amaToken()}, nil)
		}
		if f.set["json"] {
			if err := printJSON(s); err != nil {
				return err
			}
		} else {
			fmt.Println(s.Focus)
		}
		if f.set["wait"] {
			return waitExit(c, s.Focus, f.vals["timeout"])
		}
		return nil

	case "send", "prompt":
		f, err := parse(args, map[string]string{"timeout": "timeout", "t": "timeout"},
			map[string]string{"e": "escapes", "enter": "enter", "E": "enter"})
		if err != nil {
			return err
		}
		if len(f.pos) < 1 {
			return usageErr("kero %s <target> <text…>", cmd)
		}
		t, err := find(st, f.pos[0])
		if err != nil {
			return err
		}
		text := strings.Join(append(f.pos[1:], f.rest...), " ")
		if text == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			text = string(b)
		}
		if f.set["escapes"] {
			text = unescape(text)
		}
		enter := f.set["enter"] || cmd == "prompt"
		switch {
		case text != "" && enter && (cmd == "prompt" || pasteable(text)):
			// Pasted, so that its newlines do not run an agent's prompt
			// line by line, and kerod presses Enter once the program drew
			// the paste, then answers.
			var res any
			if err := c.Call("send", proto.SendParams{ID: t.ref(), Data: text, Paste: true, Submit: true, Sync: true}, &res); err != nil {
				return err
			}
			switch res {
			case proto.SubmittedSync:
			case proto.Submitted:
				// A kerod that knows no Sync presses Enter after it
				// answered: the wait starts once it likely did.
				time.Sleep(3 * time.Second)
			default:
				// An older kerod pastes alone.
				time.Sleep(500 * time.Millisecond)
				if err := c.Call("send", proto.SendParams{ID: t.ref(), Data: "\r"}, nil); err != nil {
					return err
				}
			}
		default:
			if text != "" {
				if err := c.Call("send", proto.SendParams{ID: t.ref(), Data: text}, nil); err != nil {
					return err
				}
			}
			if enter {
				// Apart from the text, which a program could take for a
				// paste of a newline.
				if text != "" {
					time.Sleep(120 * time.Millisecond)
				}
				if err := c.Call("send", proto.SendParams{ID: t.ref(), Data: "\r"}, nil); err != nil {
					return err
				}
			}
		}
		if cmd == "send" {
			return nil
		}
		timeout, err := parseTimeout(f.vals["timeout"])
		if err != nil {
			return err
		}
		var info proto.Pane
		if err := c.Call("wait", proto.WaitParams{ID: t.ref(), Timeout: timeout}, &info); err != nil {
			if err.Error() == "timed out" {
				return fmt.Errorf("timed out: the prompt went to %s, which is still working; kero wait %s waits on", f.pos[0], f.pos[0])
			}
			return err
		}
		var res proto.ReadResult
		if err := c.Call("read", proto.ReadParams{ID: t.ref()}, &res); err != nil {
			return err
		}
		fmt.Print(res.Text)
		return nil

	case "key":
		if len(args) < 2 {
			return usageErr("kero key <target> <key…>")
		}
		t, err := find(st, args[0])
		if err != nil {
			return err
		}
		var data []byte
		for _, k := range args[1:] {
			b, err := keyBytes(k)
			if err != nil {
				return err
			}
			data = append(data, b...)
		}
		return c.Call("send", proto.SendParams{ID: t.ref(), Data: string(data)}, nil)

	case "read", "capture":
		f, err := parse(args, map[string]string{"n": "lines", "lines": "lines"}, map[string]string{"a": "all", "all": "all", "reply": "reply"})
		if err != nil {
			return err
		}
		t, err := findOrCurrent(st, f.pos)
		if err != nil {
			return err
		}
		lines, _ := strconv.Atoi(f.vals["lines"])
		var res proto.ReadResult
		if err := c.Call("read", proto.ReadParams{ID: t.ref(), Scrollback: f.set["all"], Lines: lines, Reply: f.set["reply"]}, &res); err != nil {
			return err
		}
		fmt.Print(res.Text)
		return nil

	case "wait":
		f, err := parse(args, map[string]string{"for": "for", "timeout": "timeout", "t": "timeout"}, map[string]string{"json": "json", "exit": "exit", "x": "exit"})
		if err != nil {
			return err
		}
		t, err := findOrCurrent(st, f.pos)
		if err != nil {
			return err
		}
		if f.set["exit"] {
			if f.vals["for"] != "" {
				return usageErr("kero wait: --exit or --for, not both")
			}
			return waitExit(c, t.ref(), f.vals["timeout"])
		}
		var want []proto.Status
		for _, w := range strings.Split(f.vals["for"], ",") {
			if w = strings.TrimSpace(w); w != "" {
				want = append(want, proto.Status(w))
			}
		}
		timeout, err := parseTimeout(f.vals["timeout"])
		if err != nil {
			return err
		}
		var info proto.Pane
		if err := c.Call("wait", proto.WaitParams{ID: t.ref(), Status: want, Timeout: timeout}, &info); err != nil {
			return err
		}
		if f.set["json"] {
			err = printJSON(info)
		} else {
			fmt.Println(info.Status)
		}
		if err == nil && info.Status == proto.Exited && info.ExitCode != 0 {
			return exitStatus(info.ExitCode)
		}
		return err

	case "events":
		return eventsCmd(c, st, args)

	case "status":
		f, err := parse(args, nil, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		t, err := findOrCurrent(st, f.pos)
		if err != nil {
			return err
		}
		if f.set["json"] {
			if t.pane != "" {
				return printJSON(t.s.PaneByID(t.pane))
			}
			return printJSON(t.s)
		}
		p := t.s.Pane
		if t.pane != "" {
			p = *t.s.PaneByID(t.pane)
		}
		line := fmt.Sprintf("%s\t%s\t%s", t.s.ID, p.Status, t.s.DisplayName(settings.Load().SessionName))
		if p.Message != "" {
			line += "\t" + p.Message
		}
		fmt.Println(line)
		if t.pane == "" {
			printTree(st, t.s)
		}
		return nil

	case "diff":
		f, err := parse(args, nil, map[string]string{"turn": "turn", "branch": "branch", "base": "branch"})
		if err != nil {
			return err
		}
		if f.set["turn"] && f.set["branch"] {
			return usageErr("kero diff: --turn or --branch, not both")
		}
		// A target first, unless it names none: then every argument is a
		// path, of the pane the command runs in.
		var t target
		paths := f.pos
		if len(f.pos) > 0 {
			if t, err = find(st, f.pos[0]); err == nil {
				paths = f.pos[1:]
			} else if current() == "" {
				return err
			}
		}
		if t.s.ID == "" {
			if t, err = findOrCurrent(st, nil); err != nil {
				return err
			}
		}
		base := "head"
		switch {
		case f.set["turn"]:
			base = "turn"
		case f.set["branch"]:
			base = "branch"
		}
		pane := diffPane(t, base == "turn")
		if pane == nil || pane.Git == nil {
			return errors.New("not in a git repository (or git-status is off)")
		}
		var res proto.ReadResult
		if len(paths) == 0 {
			paths = []string{""}
		}
		for _, p := range paths {
			rel, err := repoPath(pane.Git.Root, p, host != "")
			if err != nil {
				return err
			}
			if err := c.Call("git.diff", proto.GitParams{Pane: pane.ID, Base: base, Path: rel}, &res); err != nil {
				return err
			}
			fmt.Print(res.Text)
		}
		return nil

	case "history":
		f, err := parse(args, map[string]string{"p": "project", "project": "project", "n": "count", "count": "count"}, map[string]string{"json": "json", "j": "json"})
		if err != nil {
			return err
		}
		list, err := conversations(c, st, f.vals["project"], strings.Join(f.pos, " "))
		if err != nil {
			return err
		}
		n := 20
		if v := f.vals["count"]; v != "" {
			if n, err = strconv.Atoi(v); err != nil || n < 0 {
				return usageErr("kero history -n takes a count")
			}
		}
		if n > 0 && len(list) > n && !f.set["json"] {
			list = list[:n]
		}
		if f.set["json"] {
			return printJSON(list)
		}
		listConversations(st, list)
		return nil

	case "resume":
		f, err := parse(args, nil, map[string]string{"f": "focus", "focus": "focus", "json": "json"})
		if err != nil {
			return err
		}
		if len(f.pos) == 0 {
			return usageErr("kero resume <conversation> (see kero history)")
		}
		list, err := conversations(c, st, "", "")
		if err != nil {
			return err
		}
		conv, err := findConversation(list, strings.Join(f.pos, " "))
		if err != nil {
			return err
		}
		var s proto.Session
		if err := c.Call("conversation.resume", proto.ConversationParams{Agent: conv.Agent, ID: conv.ID, After: os.Getenv("KERO_SESSION")}, &s); err != nil {
			return err
		}
		if f.set["focus"] {
			c.Call("select", proto.FocusParams{Session: s.ID, Token: amaToken()}, nil)
		}
		if f.set["json"] {
			return printJSON(s)
		}
		fmt.Println(s.ID)
		return nil

	case "rename":
		if len(args) < 2 {
			return usageErr("kero rename <session> <name>")
		}
		t, err := find(st, args[0])
		if err != nil {
			return err
		}
		return c.Call("rename", proto.RenameParams{ID: t.s.ID, Name: strings.Join(args[1:], " "), By: os.Getenv("KERO_PANE")}, nil)

	case "focus":
		if len(args) < 1 {
			return usageErr("kero focus <target>")
		}
		t, err := find(st, args[0])
		if err != nil {
			return err
		}
		var shown bool
		if err := c.Call("select", proto.FocusParams{Session: t.s.ID, Pane: t.pane, Token: amaToken()}, &shown); err != nil {
			return err
		}
		if !shown {
			return errors.New("the app is not open")
		}
		return nil

	case "close", "kill", "restart":
		f, err := parse(args, filterFlags, map[string]string{"remove-worktree": "remove", "force": "force", "children": "children", "all": "all", "dry-run": "dry", "n": "dry"})
		if err != nil {
			return err
		}
		sf, err := newFilter(st, f)
		if err != nil {
			return err
		}
		if cmd != "restart" && (sf.any() || f.set["all"] || f.set["dry"] || len(f.pos) > 1) {
			if (sf.any() || f.set["all"]) && len(f.pos) > 0 {
				return usageErr("kero %s: targets, or --all, -p and --status, not both", cmd)
			}
			return closeMany(c, st, f, sf)
		}
		if len(f.pos) < 1 {
			return usageErr("kero %s <target>", cmd)
		}
		t, err := find(st, f.pos[0])
		if err != nil {
			return err
		}
		remove := f.set["remove"]
		if remove && t.s.Worktree == nil {
			return fmt.Errorf("session %s has no worktree of its own", t.s.ID)
		}
		switch {
		case cmd == "restart":
			return c.Call("restart", proto.IDParams{ID: t.ref()}, nil)
		case t.pane != "" && len(t.s.Panes) > 1 && !remove:
			return c.Call("pane.close", proto.IDParams{ID: t.pane}, nil)
		}
		if err := c.Call("close", proto.CloseParams{ID: t.s.ID, RemoveWorktree: remove, Force: f.set["force"], By: current()}, nil); err != nil {
			return err
		}
		// Then its children, which tell the orchestrator, gone, nothing;
		// each as kero close closes it alone: its worktree stays, for the
		// cleanup.
		if f.set["children"] {
			for _, k := range children(st, t.s.ID) {
				if err := c.Call("close", proto.CloseParams{ID: k.ID, By: current()}, nil); err != nil {
					return fmt.Errorf("closing child %s: %w", k.ID, err)
				}
			}
		}
		return nil

	case "orchestrator":
		f, err := parse(args, map[string]string{"goal": "goal"}, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		pos, off := f.pos, false
		if n := len(pos); n > 0 && (pos[n-1] == "on" || pos[n-1] == "off") {
			off, pos = pos[n-1] == "off", pos[:n-1]
		}
		if len(pos) > 1 {
			return usageErr("kero orchestrator [session] [on|off] [--goal <text>]")
		}
		t, err := findOrCurrent(st, pos)
		if err != nil {
			return err
		}
		p := proto.OrchestratorParams{ID: t.s.ID, Off: off}
		if g, ok := f.vals["goal"]; ok {
			if off {
				return usageErr("kero orchestrator off takes no --goal")
			}
			p.Goal = &g
		}
		var s proto.Session
		if err := c.Call("orchestrator", p, &s); err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(s)
		}
		switch {
		case off:
			fmt.Printf("session %s is an ordinary session again\n", s.ID)
		case t.s.Orchestrator:
			fmt.Printf("session %s is an orchestrator; goal: %s\n", s.ID, orDash(s.Goal))
		case s.Agent == "":
			fmt.Printf("session %s is an orchestrator, but runs no agent Kero knows to give the role prompt to\n", s.ID)
		default:
			fmt.Printf("session %s is an orchestrator: its agent gets the role prompt\n", s.ID)
		}
		return nil

	case "do":
		if len(args) < 1 {
			return usageErr("kero do <action> [key=value…] (see kero actions)")
		}
		if args[0] == "pane.preview" {
			return previewCmd(c, st, args[1:])
		}
		a, ok := actions.Find(args[0])
		if !ok || !a.Remote {
			return fmt.Errorf("%w: no action %q (see kero actions)", errNotFound, args[0])
		}
		kv := map[string]string{}
		for _, arg := range args[1:] {
			k, v, ok := strings.Cut(arg, "=")
			if !ok {
				return usageErr("%q is not key=value", arg)
			}
			if k == "path" || k == "dir" {
				v = hereFile(v, host)
			}
			kv[k] = v
		}
		var delivered bool
		if err := c.Call("do", proto.DoParams{Action: a.ID, Args: kv, Token: amaToken()}, &delivered); err != nil {
			return err
		}
		if !delivered {
			return errors.New("the app is not open")
		}
		return nil

	case "browser":
		// The browser's DevTools port, which no pane is the target of.
		if len(args) > 0 && args[0] == "devtools" {
			return devtoolsCmd(st, args[1:])
		}
		return browserCmd(c, st, args)

	case "notify":
		f, err := parse(args, map[string]string{"session": "target", "s": "target", "title": "title"}, nil)
		if err != nil {
			return err
		}
		ref := f.vals["target"]
		if ref == "" {
			ref = current()
		}
		if ref == "" {
			return usageErr("which pane? run it in Kero, or give --session")
		}
		t, err := find(st, ref)
		if err != nil {
			return err
		}
		return c.Call("notify", proto.NotifyParams{ID: t.ref(), Title: f.vals["title"], Body: strings.Join(append(f.pos, f.rest...), " ")}, nil)
	case "pr":
		return prCommand(c, args)
	case "worktree":
		return worktreeCommand(c, args)
	}
	return usageErr("unknown command %q (see kero help)", cmd)
}

// target is what a command acts on: a session, or one of its panes.
type target struct {
	s    proto.Session
	pane string // "" for the session, which stands for its focused pane
}

func (t target) ref() string {
	if t.pane != "" {
		return t.pane
	}
	return t.s.ID
}

func find(st proto.State, ref string) (target, error) {
	for _, s := range st.Sessions {
		if s.ID == ref {
			return target{s: s}, nil
		}
		for _, p := range s.Panes {
			if p.ID == ref {
				return target{s: s, pane: p.ID}, nil
			}
		}
	}
	var matches []target
	for _, s := range st.Sessions {
		if strings.HasPrefix(s.ID, ref) || strings.EqualFold(s.Name, ref) || strings.EqualFold(s.DisplayName(settings.Load().SessionName), ref) {
			matches = append(matches, target{s: s})
			continue
		}
		for _, p := range s.Panes {
			if strings.HasPrefix(p.ID, ref) {
				matches = append(matches, target{s: s, pane: p.ID})
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return target{}, fmt.Errorf("%w: no session or pane %q", errNotFound, ref)
	}
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.ref()
	}
	slices.Sort(ids)
	return target{}, fmt.Errorf("%w: %q names several: %s", errNotFound, ref, strings.Join(ids, ", "))
}

// diffPane is the pane whose worktree kero diff shows: the target's, or
// for a turn, the pane of its session whose agent started one, the
// target first.
func diffPane(t target, turn bool) *proto.Pane {
	p := t.s.PaneByID(t.pane)
	if p == nil {
		p = t.s.FocusPane()
	}
	if !turn || p == nil || p.Turn != nil {
		return p
	}
	for i := range t.s.Panes {
		if t.s.Panes[i].Turn != nil {
			return &t.s.Panes[i]
		}
	}
	return p
}

// repoPath writes a path given on the command line, relative to the
// current directory, as one from a worktree's root, slash-separated; ""
// stays "", for every file. A path of another machine's worktree
// (remote) is from its root already, unless absolute.
func repoPath(root, p string, remote bool) (string, error) {
	if p == "" {
		return "", nil
	}
	if remote {
		p = strings.ReplaceAll(p, `\`, "/")
		if strings.HasPrefix(p, "/") {
			rel, ok := strings.CutPrefix(p, strings.TrimSuffix(root, "/")+"/")
			if !ok {
				return "", fmt.Errorf("%s is not in the worktree %s", p, root)
			}
			p = rel
		}
		return strings.TrimPrefix(path.Clean("/"+p), "/"), nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is not in the worktree %s", p, proto.ShortDir(root))
	}
	if rel == "." {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}

// amaToken is the secret kerod gives Ama's panes ($KERO_AMA), which kero
// do, kero focus and kero browser pass on to this machine's kerod: the app
// takes them from Ama without Remote Control. Another host's kerod knows
// nothing of it, and gets none.
func amaToken() string {
	if onHost != "" {
		return ""
	}
	return os.Getenv("KERO_AMA")
}

// current is the pane a command runs in, inside kero.
func current() string {
	// A pane of this machine names nothing on another.
	if onHost != "" {
		return ""
	}
	if p := os.Getenv("KERO_PANE"); p != "" {
		return p
	}
	return os.Getenv("KERO_SESSION")
}

func findOrCurrent(st proto.State, pos []string) (target, error) {
	if len(pos) > 0 {
		return find(st, pos[0])
	}
	if ref := current(); ref != "" {
		return find(st, ref)
	}
	return target{}, usageErr("which session? (outside kero, name one)")
}

func daemonCmd(args []string, host string) error {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	if host != "" {
		return remoteDaemonCmd(sub, args[min(1, len(args)):], host)
	}
	switch sub {
	case "status":
		svc := service.Get()
		c, err := client.Connect(true)
		if err != nil {
			fmt.Println("kerod is not running")
		} else {
			defer c.Close()
			var st proto.State
			c.Call("state", nil, &st)
			panes := 0
			for _, s := range st.Sessions {
				panes += len(s.Panes)
			}
			fmt.Printf("kerod %d running (Kero %s, pid %d) for %s: %d sessions, %d panes\n", c.Welcome.Version, orDash(c.Welcome.App), c.Welcome.PID,
				time.Since(time.UnixMilli(c.Welcome.Started)).Round(time.Second), len(st.Sessions), panes)
		}
		fmt.Println("socket:", proto.SocketPath())
		if svc.Installed {
			fmt.Printf("service: %s (%s)\n", orDash(svc.Detail), svc.File)
			if !svc.Linger {
				fmt.Println("  it stops as you log out: loginctl enable-linger keeps it running")
			}
		}
		if l := settings.Load().Listen; len(l) > 0 {
			fmt.Println("listening:", strings.Join(l, ", "))
		}
		return nil
	case "stop", "restart":
		// The service's manager stops and restarts it, but Task
		// Scheduler, whose kerod stops as any, and starts with the task.
		if service.Installed() {
			var err error
			if sub == "stop" {
				err = service.Stop()
			} else {
				err = service.Restart()
			}
			if !errors.Is(err, service.ErrUnsupported) {
				if err == nil && sub == "restart" {
					fmt.Println("kerod restarted")
				}
				return err
			}
		}
		if err := StopDaemon(); err != nil {
			return err
		}
		if sub == "restart" {
			c, err := client.Connect(false)
			if err != nil {
				return err
			}
			c.Close()
			fmt.Println("kerod restarted")
		}
		return nil
	case "serve":
		return serveCmd(args[1:])
	case "install-service":
		f, err := parse(args[1:], nil, map[string]string{"no-linger": "no-linger"})
		if err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		// A user's services stop as they log out, unless they linger: a
		// service would then end kerod as an SSH bridge does, which a kerod
		// of its own outlives. --no-linger takes it as it is.
		if !f.set["no-linger"] {
			if err := service.EnableLinger(); err != nil {
				return fmt.Errorf("%v; without it kerod would stop as you log out (kero daemon install-service --no-linger installs it all the same)", err)
			}
		}
		if err := service.Install(exe, StopDaemon); err != nil {
			return err
		}
		fmt.Printf("kerod runs as a service: %s\n", service.Get().File)
		return nil
	case "uninstall-service":
		if err := service.Uninstall(); err != nil {
			return err
		}
		fmt.Println("kerod no longer runs as a service: the app, or a bridge, starts it")
		return nil
	case "update":
		if !build.Server {
			return errors.New("this is Kero's desktop app, which updates itself: kero daemon update is of the server build")
		}
		version := ""
		if len(args) > 1 {
			version = args[1]
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if version == "" {
			v, err := update.Latest(ctx)
			if err != nil {
				return err
			}
			version = v
		}
		if strings.TrimPrefix(version, "v") == build.Version() {
			fmt.Printf("Kero %s is installed\n", build.Version())
			return nil
		}
		// A version named is the user's to go back to.
		if err := update.Install(ctx, version, update.InstallDir(), build.Version(), len(args) > 1); err != nil {
			return err
		}
		fmt.Printf("installed Kero %s: kerod runs it as it restarts (kero daemon restart), its sessions with it\n", strings.TrimPrefix(version, "v"))
		return nil
	}
	return usageErr("unknown daemon command %q", sub)
}

// remoteDaemonCmd is kero -H <host> daemon …: how kerod on the host is,
// and restarting it, or updating Kero there, as the app does.
func remoteDaemonCmd(sub string, args []string, host string) error {
	rc, err := dialHost(host)
	if err != nil {
		return err
	}
	defer rc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	switch sub {
	case "status":
		h := rc.Welcome.Host
		fmt.Printf("%s: %s (%s/%s), Kero %s (protocol %d)\n", host, h.Hostname, h.OS, h.Arch, orDash(h.Kero), h.Version)
		c, err := client.ConnectVia(rc)
		if err != nil {
			fmt.Println("kerod:", err)
			return nil
		}
		defer c.Close()
		var st proto.State
		c.Call("state", nil, &st)
		fmt.Printf("kerod %d running (Kero %s, pid %d) for %s: %d sessions\n", c.Welcome.Version, orDash(c.Welcome.App), c.Welcome.PID,
			time.Since(time.UnixMilli(c.Welcome.Started)).Round(time.Second), len(st.Sessions))
		return nil
	case "restart":
		if err := rc.Host(ctx, proto.HostRequest{Kind: proto.KindRestart}); err != nil {
			return err
		}
		fmt.Printf("kerod on %s restarted\n", host)
		return nil
	case "update":
		version := releasedVersion()
		if len(args) > 0 {
			version = strings.TrimPrefix(args[0], "v")
		}
		if version == "" {
			v, err := update.Latest(ctx)
			if err != nil {
				return err
			}
			version = v
		}
		fmt.Printf("updating Kero on %s to %s…\n", host, version)
		if err := rc.Host(ctx, proto.HostRequest{Kind: proto.KindUpdate, App: version}); err != nil {
			return err
		}
		fmt.Printf("installed: kerod on %s runs it as it restarts (kero -H %s daemon restart), its sessions with it\n", host, host)
		return nil
	}
	return usageErr("kero -H <host> daemon status|restart|update [version]")
}

// StopDaemon stops the running daemon, which saves its sessions to start
// them again, whatever its version.
func StopDaemon() error {
	pid := client.DaemonPID()
	if pid == 0 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if client.Interrupt(p) != nil {
		p.Kill()
	}
	for range 100 {
		if client.DaemonPID() == 0 {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return errors.New("kerod did not stop")
}

// conversations lists the conversations kerod remembers, of a project
// (by id or name) when one is named, matching query when there is one.
func conversations(c *client.Client, st proto.State, project, query string) ([]proto.Conversation, error) {
	var list []proto.Conversation
	if err := c.Call("conversations", nil, &list); err != nil {
		if strings.Contains(err.Error(), "unknown method") {
			return nil, errors.New("this kerod remembers no conversations: update it, or run `kero daemon restart`")
		}
		return nil, err
	}
	if project != "" {
		id := ""
		for _, p := range st.Projects {
			if p.ID == project || strings.EqualFold(p.Name, project) {
				id = p.ID
			}
		}
		if id == "" {
			return nil, fmt.Errorf("no project %q", project)
		}
		list = slices.DeleteFunc(list, func(cv proto.Conversation) bool { return cv.Project != id })
	}
	if q := strings.ToLower(strings.TrimSpace(query)); q != "" {
		list = slices.DeleteFunc(list, func(cv proto.Conversation) bool {
			for _, w := range strings.Fields(q) {
				if !strings.Contains(strings.ToLower(cv.Name+" "+cv.Title+" "+cv.Dir+" "+cv.Agent+" "+cv.ID), w) {
					return true
				}
			}
			return false
		})
	}
	return list, nil
}

// findConversation finds a conversation by its id, a unique prefix of
// it, or its name.
func findConversation(list []proto.Conversation, ref string) (proto.Conversation, error) {
	var found []proto.Conversation
	for _, cv := range list {
		if cv.ID == ref {
			return cv, nil
		}
		if strings.HasPrefix(cv.ID, ref) || strings.EqualFold(cv.Label(), ref) {
			found = append(found, cv)
		}
	}
	switch len(found) {
	case 0:
		return proto.Conversation{}, fmt.Errorf("%w: no conversation %q (see kero history)", errNotFound, ref)
	case 1:
		return found[0], nil
	}
	return proto.Conversation{}, fmt.Errorf("%q could be %d conversations: give more of its id (see kero history)", ref, len(found))
}

func listConversations(st proto.State, list []proto.Conversation) {
	if len(list) == 0 {
		fmt.Println("no conversations yet (agents' conversations show here once Kero's skill is set up: kero skill setup)")
		return
	}
	names := map[string]string{}
	for _, p := range st.Projects {
		names[p.ID] = p.Name
	}
	now := time.Now()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tAGENT\tPROJECT\tNAME\tLAST RAN\tDIR")
	for _, cv := range list {
		when := proto.Ago(cv.Updated, now)
		if cv.Session != "" {
			when = "running in " + cv.Session
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", cv.ID, cv.Agent, orDash(names[cv.Project]), cv.Label(), when, proto.ShortDir(cv.Dir))
	}
	w.Flush()
}

func listSessions(st proto.State) {
	if len(st.Sessions) == 0 {
		fmt.Println("no sessions (start one with kero new)")
		return
	}
	names := map[string]string{}
	for _, p := range st.Projects {
		names[p.ID] = p.Name
	}
	now := time.Now()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROJECT\tNAME\tSTATUS\tPROGRAM\tSINCE\tDIR")
	status := func(p proto.Pane) string {
		s := string(p.Status)
		if p.Progress >= 0 {
			s += fmt.Sprintf(" %d%%", p.Progress)
		}
		if p.Unread {
			s += "*"
		}
		return s
	}
	// A browser's program is the browser, and its place its page; an
	// editor's, the editor and its file.
	program := func(p proto.Pane) string {
		switch {
		case p.Browser():
			return "browser"
		case p.Editor():
			return "editor"
		}
		return orDash(p.Process)
	}
	where := func(p proto.Pane) string {
		switch {
		case p.Browser():
			return p.URL
		case p.Editor():
			return proto.ShortDir(p.File)
		}
		return proto.ShortDir(p.WorkDir())
	}
	for _, s := range proto.InTree(st.Sessions, func(s proto.Session) (string, string) { return s.ID, s.Parent }) {
		name := s.DisplayName(settings.Load().SessionName)
		switch {
		case s.Orchestrator:
			name += " (orchestrator)"
		case s.Parent != "":
			name = "↳ " + name
		}
		project := names[s.Project]
		if s.Ama {
			project = "(Ama)" // of no project: the helper agent's
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, project, name, status(s.Pane),
			program(s.Pane), proto.Ago(s.Since, now), where(s.Pane))
		if len(s.Panes) > 1 {
			for _, p := range s.Panes {
				mark := " "
				if p.ID == s.Focus {
					mark = "›"
				}
				fmt.Fprintf(w, "  %s %s\t\t%s\t%s\t%s\t%s\t%s\n", mark, p.ID, p.PaneName(), status(p),
					program(p), proto.Ago(p.Since, now), where(p))
			}
		}
	}
	w.Flush()
}

// children are an orchestrator's child sessions, in the sidebar's order.
func children(st proto.State, id string) []proto.Session {
	var kids []proto.Session
	for _, s := range st.Sessions {
		if s.Parent == id {
			kids = append(kids, s)
		}
	}
	return kids
}

// printTree prints, after kero status's line, a session's part in an
// orchestrator's tree: its goal and children, or its orchestrator.
func printTree(st proto.State, s proto.Session) {
	name := settings.Load().SessionName
	if s.Orchestrator {
		fmt.Printf("orchestrator\tgoal: %s\n", orDash(s.Goal))
		for _, k := range children(st, s.ID) {
			line := fmt.Sprintf("  ↳ %s\t%s\t%s", k.ID, k.Status, k.DisplayName(name))
			if k.Message != "" {
				line += "\t" + strings.Join(strings.Fields(k.Message), " ")
			}
			fmt.Println(line)
		}
		return
	}
	for _, o := range st.Sessions {
		if o.ID == s.Parent {
			fmt.Printf("child of\t%s\t%s\n", o.ID, o.DisplayName(name))
		}
	}
}

// reportToParent sends this child session's orchestrator a summary of its
// result (kero report --to-parent), which kerod puts in the orchestrator's
// next report of its children.
func reportToParent(args []string) error {
	f, err := parse(args, map[string]string{"s": "session", "session": "session"}, map[string]string{"to-parent": "to-parent"})
	if err != nil {
		return err
	}
	text := strings.Join(append(f.pos, f.rest...), " ")
	if text == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		text = string(b)
	}
	if strings.TrimSpace(text) == "" {
		return usageErr("kero report --to-parent <summary…|->")
	}
	from := f.vals["session"]
	if from == "" {
		from = current()
	}
	if from == "" {
		return usageErr("kero report --to-parent: which session? Run it in a child session's pane, or give --session")
	}
	c, done, err := connect("")
	if err != nil {
		return fmt.Errorf("%w: %v", errUnreachable, err)
	}
	defer done()
	return c.Call("agent.send", proto.AgentSendParams{From: from, Text: text, ToParent: true}, nil)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func parseTimeout(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		n, err2 := strconv.Atoi(s)
		if err2 != nil {
			return 0, usageErr("bad timeout %q", s)
		}
		d = time.Duration(n) * time.Second
	}
	return d.Milliseconds(), nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// keys are the names kero key takes, as the bytes a terminal sends.
var keys = map[string]string{
	"enter": "\r", "return": "\r", "tab": "\t", "escape": "\x1b", "esc": "\x1b", "space": " ",
	"backspace": "\x7f", "delete": "\x1b[3~", "up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C",
	"left": "\x1b[D", "home": "\x1b[H", "end": "\x1b[F", "pageup": "\x1b[5~", "pagedown": "\x1b[6~",
	"shift+tab": "\x1b[Z",
}

func keyBytes(name string) ([]byte, error) {
	n := strings.ToLower(name)
	if b, ok := keys[n]; ok {
		return []byte(b), nil
	}
	if rest, ok := strings.CutPrefix(n, "ctrl+"); ok && len(rest) == 1 {
		c := rest[0]
		switch {
		case c >= 'a' && c <= 'z':
			return []byte{c - 'a' + 1}, nil
		case c >= '@' && c <= '_':
			return []byte{c - '@'}, nil
		}
	}
	if rest, ok := strings.CutPrefix(n, "alt+"); ok {
		b, err := keyBytes(rest)
		if err != nil {
			return nil, err
		}
		return append([]byte{0x1b}, b...), nil
	}
	if len([]rune(name)) == 1 {
		return []byte(name), nil
	}
	return nil, usageErr("unknown key %q", name)
}

// waitExit waits for the command a pane started with to end, and exits
// with its status: or its shell's, if that ended first.
func waitExit(c *client.Client, pane, timeout string) error {
	ms, err := parseTimeout(timeout)
	if err != nil {
		return err
	}
	var info proto.Pane
	if err := c.Call("wait", proto.WaitParams{ID: pane, Exit: true, Timeout: ms}, &info); err != nil {
		return err
	}
	code := info.ExitCode
	if info.CommandExit != nil {
		code = *info.CommandExit
	}
	if code != 0 {
		return exitStatus(code)
	}
	return nil
}

// report writes an OSC 7501 report of the program's status to the
// terminal, as a script reports what it does: kero shows it, and so do
// other terminals that read the protocol.
func report(args []string) error {
	f, err := parse(args, map[string]string{"kind": "kind", "progress": "progress", "app": "app", "id": "id", "title": "title"}, nil)
	if err != nil {
		return err
	}
	if len(f.pos) < 1 {
		return usageErr("kero report <idle|working|done|blocked|error|clear> [message]")
	}
	state := f.pos[0]
	if !slices.Contains([]string{"idle", "working", "done", "blocked", "error", "clear"}, state) {
		return usageErr("unknown state %q", state)
	}
	pairs := []string{"state=" + state}
	for _, k := range []string{"id", "kind", "progress", "app"} {
		if v := f.vals[k]; v != "" {
			pairs = append(pairs, k+"="+v)
		}
	}
	if t := f.vals["title"]; t != "" {
		pairs = append(pairs, "title="+base64.StdEncoding.EncodeToString([]byte(t)))
	}
	if msg := strings.Join(append(f.pos[1:], f.rest...), " "); msg != "" {
		pairs = append(pairs, "msg="+base64.StdEncoding.EncodeToString([]byte(msg)))
	}
	body := strings.Join(pairs, ":")
	seq := "\x1b]7501;" + body + "\x1b\\"
	// To the terminal, even when the output goes elsewhere.
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		defer tty.Close()
		_, err = tty.WriteString(seq)
		return err
	}
	// No terminal, as under an agent's shell tool, which captures the
	// output: in Kero, kerod takes the report for the pane instead.
	if pane := current(); pane != "" && !term.IsTerminal(int(os.Stdout.Fd())) {
		c, err := client.Connect(true)
		if err != nil {
			return fmt.Errorf("no terminal to report to, and kerod: %w", err)
		}
		defer c.Close()
		return c.Call("pane.report", proto.ReportParams{ID: pane, Report: body}, nil)
	}
	_, err = os.Stdout.WriteString(seq)
	return err
}

// pasteable tells text kero send --enter pastes: lines of text, which
// typed would run line by line, and no keys, as an escape sequence or a
// control character, which a paste would hand the program as text.
func pasteable(text string) bool {
	if !strings.Contains(text, "\n") {
		return false
	}
	for _, r := range text {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return false
		}
	}
	return true
}

// unescape interprets the escapes of echo -e: \n \r \t \e \a \b \\ \xHH.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'e':
			b.WriteByte(0x1b)
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case '\\':
			b.WriteByte('\\')
		case 'x':
			if i+2 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					b.WriteByte(byte(v))
					i += 2
					continue
				}
			}
			b.WriteString(`\x`)
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
