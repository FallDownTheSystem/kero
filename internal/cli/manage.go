package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"kero/internal/actions"
	"kero/internal/client"
	"kero/internal/proto"
	"kero/internal/settings"
)

// What the app does that a shell or an agent, as Ama, does with these:
// projects added, renamed and removed; sessions moved to a project, listed
// and closed by project and status; worktrees made, removed, cleaned up
// and pruned; settings read and written; pull requests listed and
// reviewed.

// sessionFilter picks sessions: of a project, in one of the statuses.
type sessionFilter struct {
	project  string // the project's id, "" for any
	statuses []proto.Status
}

// filterFlags are the flags kero ls and kero close take to pick sessions.
var filterFlags = map[string]string{"p": "project", "project": "project", "status": "status", "s": "status"}

// newFilter reads a sessionFilter from kero ls's or kero close's flags.
func newFilter(st proto.State, f *flags) (sessionFilter, error) {
	var sf sessionFilter
	if ref := f.vals["project"]; ref != "" {
		id, err := findProject(st, ref)
		if err != nil {
			return sf, err
		}
		sf.project = id
	}
	for _, w := range strings.Split(f.vals["status"], ",") {
		switch w = strings.TrimSpace(w); w {
		case "":
		case "idle", "working", "waiting", "error", "exited":
			sf.statuses = append(sf.statuses, proto.Status(w))
		default:
			return sf, usageErr("no status %q: idle, working, waiting, error or exited", w)
		}
	}
	return sf, nil
}

func (sf sessionFilter) any() bool { return sf.project != "" || len(sf.statuses) > 0 }

func (sf sessionFilter) picks(s proto.Session) bool {
	return (sf.project == "" || s.Project == sf.project) && (len(sf.statuses) == 0 || slices.Contains(sf.statuses, s.Status))
}

// pick returns the state's sessions the filter picks.
func (sf sessionFilter) pick(st proto.State) []proto.Session {
	var out []proto.Session
	for _, s := range st.Sessions {
		if sf.picks(s) {
			out = append(out, s)
		}
	}
	return out
}

// closeMany is kero close with filters, or several targets: each session
// picked closes, as kero close closes one, but Ama's and the one it runs
// in, which a filter never picks. --dry-run lists them instead.
func closeMany(c *client.Client, st proto.State, f *flags, sf sessionFilter) error {
	var picked []proto.Session
	if sf.any() || f.set["all"] {
		self := ""
		if t, err := findOrCurrent(st, nil); err == nil {
			self = t.s.ID
		}
		for _, s := range sf.pick(st) {
			if !s.Ama && s.ID != self {
				picked = append(picked, s)
			}
		}
	} else {
		for _, ref := range f.pos {
			t, err := find(st, ref)
			if err != nil {
				return err
			}
			picked = append(picked, t.s)
		}
	}
	// --children: each orchestrator's children follow it, once each, and,
	// as with one target, keep their worktrees for the cleanup.
	kid := map[string]bool{}
	if f.set["children"] {
		seen := map[string]bool{}
		for _, s := range picked {
			seen[s.ID] = true
		}
		var all []proto.Session
		for _, s := range picked {
			all = append(all, s)
			for _, k := range children(st, s.ID) {
				if !seen[k.ID] {
					seen[k.ID], kid[k.ID] = true, true
					all = append(all, k)
				}
			}
		}
		picked = all
	}
	if len(picked) == 0 {
		fmt.Println("no sessions to close")
		return nil
	}
	names := projectNames(st)
	var failed []string
	for _, s := range picked {
		line := fmt.Sprintf("%s\t%s\t%s\t%s", s.ID, orDash(names[s.Project]), s.DisplayName(settings.Load().SessionName), s.Status)
		if f.set["dry"] {
			fmt.Println("would close", line)
			continue
		}
		remove := f.set["remove"] && s.Worktree != nil && !kid[s.ID]
		if err := c.Call("close", proto.CloseParams{ID: s.ID, RemoveWorktree: remove, Force: f.set["force"], By: current()}, nil); err != nil {
			failed = append(failed, s.ID+": "+err.Error())
			continue
		}
		fmt.Println("closed", line)
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

func projectNames(st proto.State) map[string]string {
	names := map[string]string{}
	for _, p := range st.Projects {
		names[p.ID] = p.Name
	}
	return names
}

// projectsCmd is kero projects add, rename and remove.
func projectsCmd(c *client.Client, st proto.State, sub string, args []string) error {
	switch sub {
	case "add":
		f, err := parse(args, map[string]string{"n": "name", "name": "name"}, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		if len(f.pos) != 1 {
			return usageErr("kero projects add <folder> [-n <name>]")
		}
		var p proto.Project
		if err := c.Call("project.create", proto.ProjectParams{Dir: hereFile(f.pos[0], onHost), Name: f.vals["name"]}, &p); err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(p)
		}
		fmt.Println(p.ID)
		return nil
	case "rename":
		if len(args) < 2 {
			return usageErr("kero projects rename <project> <name>")
		}
		id, err := findProject(st, args[0])
		if err != nil {
			return err
		}
		return c.Call("project.rename", proto.ProjectParams{ID: id, Name: strings.Join(args[1:], " ")}, nil)
	case "remove", "rm", "close":
		f, err := parse(args, nil, map[string]string{"force": "force", "f": "force"})
		if err != nil {
			return err
		}
		if len(f.pos) != 1 {
			return usageErr("kero projects remove <project> [--force]")
		}
		id, err := findProject(st, f.pos[0])
		if err != nil {
			return err
		}
		// As the sidebar's Close Project asks first: its sessions end.
		if n := len(sessionFilter{project: id}.pick(st)); n > 0 && !f.set["force"] {
			return fmt.Errorf("the project has %d session(s), which removing it closes: --force closes them too", n)
		}
		return c.Call("project.close", proto.IDParams{ID: id}, nil)
	}
	return usageErr("kero projects [ls] [--json] | add <folder> | rename <project> <name> | remove <project> | %s", moveUsage)
}

// moveSession is kero move <session> <project>: the session goes to the
// project, at its end, as dragging its row there does.
func moveSession(c *client.Client, st proto.State, args []string) error {
	if len(args) != 2 {
		return usageErr("kero move <session> <project>")
	}
	t, err := find(st, args[0])
	if err != nil {
		return err
	}
	id, err := findProject(st, args[1])
	if err != nil {
		return err
	}
	return c.Call("move", proto.MoveParams{ID: t.s.ID, Project: id, Index: -1}, nil)
}

// configValue is kero config get and set: a setting by its key, as the
// configuration file writes it; a list's values a line each. Its options
// come before the key, as what follows the key is the value, flags and
// all: --repo, one repository's line of a key that holds a line a
// repository; --text, a prompt as its lines, not as the file escapes
// them; get's --default, Kero's own value, its own prompt where the
// setting is empty; set's --file, the value from a file, - for stdin,
// as its lines.
func configValue(sub string, args []string) error {
	usage := map[string]string{
		"get": "kero config get [--repo <owner>/<name>] [--text] [--default] <key>",
		"set": "kero config set [--repo <owner>/<name>] [--file <path>|-] <key> [value…]",
	}[sub]
	var repo, file string
	text, builtin, fromFile := false, false, false
options:
	for len(args) > 0 {
		switch {
		case args[0] == "--repo" && len(args) > 1:
			repo, args = args[1], args[2:]
		case args[0] == "--text":
			text, args = true, args[1:]
		case args[0] == "--default" && sub == "get":
			builtin, args = true, args[1:]
		case args[0] == "--file" && sub == "set" && len(args) > 1:
			file, fromFile, args = args[1], true, args[2:]
		case strings.HasPrefix(args[0], "--") && args[0] != "--":
			return usageErr("%s (unknown option %s)", usage, args[0])
		default:
			break options
		}
	}
	if len(args) == 0 {
		return usageErr("%s (kero config show lists the keys)", usage)
	}
	name := args[0]
	if sub == "get" {
		if len(args) != 1 {
			return usageErr(usage)
		}
		if text || builtin || repo != "" {
			v, err := settings.Value(name, repo, builtin)
			if err != nil {
				return fmt.Errorf("%w: %v", errNotFound, err)
			}
			if v != "" {
				fmt.Println(v)
			}
			return nil
		}
		vals, err := settings.Get(name)
		if err != nil {
			return fmt.Errorf("%w: %v", errNotFound, err)
		}
		for _, v := range vals {
			fmt.Println(v)
		}
		return nil
	}
	vals := args[1:]
	if len(vals) > 0 && vals[0] == "--" {
		vals = vals[1:]
	}
	var err error
	switch {
	case fromFile:
		if len(vals) > 0 {
			return usageErr("%s (--file is the value: give no other)", usage)
		}
		var b []byte
		if file == "-" {
			b, err = io.ReadAll(os.Stdin)
		} else {
			b, err = os.ReadFile(file)
		}
		if err == nil {
			err = settings.SetValue(name, repo, string(b))
		}
	case text || repo != "":
		err = settings.SetValue(name, repo, strings.Join(vals, " "))
	default:
		err = settings.Set(name, vals)
	}
	if err != nil {
		return err
	}
	if p := settings.Problems(); len(p) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(p, "\n"))
	}
	return nil
}

// worktreeDir is the folder of the repository a worktree command acts on:
// -d's, a project's (-p), else the pane's or this one's.
func worktreeDir(c *client.Client, f *flags) (string, error) {
	if d := f.vals["dir"]; d != "" {
		return hereFile(d, onHost), nil
	}
	if ref := f.vals["project"]; ref != "" {
		var st proto.State
		if err := c.Call("state", nil, &st); err != nil {
			return "", err
		}
		id, err := findProject(st, ref)
		if err != nil {
			return "", err
		}
		for _, p := range st.Projects {
			if p.ID == id && p.Dir != "" {
				return p.Dir, nil
			}
		}
		return "", fmt.Errorf("the project %s has no folder", ref)
	}
	if onHost != "" {
		return "", usageErr("on another host, name the repository's folder: -d <folder>")
	}
	return os.Getwd()
}

// worktreeCreate is kero worktree create <branch>: a session in a new
// worktree of its own, on a new branch, as New Session in Worktree makes
// one; kero new --worktree with the folder named.
func worktreeCreate(c *client.Client, args []string) error {
	f, err := parse(args, map[string]string{"d": "dir", "dir": "dir", "p": "project", "project": "project", "base": "base", "n": "name", "name": "name"},
		map[string]string{"json": "json", "f": "focus", "focus": "focus"})
	if err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return usageErr("kero worktree create <branch> [-d <folder>|-p <project>] [--base <ref>] [-- command…]")
	}
	dir, err := worktreeDir(c, f)
	if err != nil {
		return err
	}
	project := ""
	if f.vals["project"] != "" {
		project = f.vals["project"]
	}
	var s proto.Session
	if err := c.Call("create", proto.CreateParams{Project: project, Dir: dir, Name: f.vals["name"], Command: f.rest,
		Worktree: f.pos[0], Base: f.vals["base"], After: os.Getenv("KERO_SESSION")}, &s); err != nil {
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
}

// worktreeRemove is kero worktree remove <branch|path>: as the History
// tab removes one. A session's own worktree goes with the session, which
// closes; one another session works in stays; changes it would lose
// need --force.
func worktreeRemove(c *client.Client, args []string) error {
	f, err := parse(args, map[string]string{"d": "dir", "dir": "dir", "p": "project", "project": "project"},
		map[string]string{"force": "force", "branch": "branch", "json": "json"})
	if err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return usageErr("kero worktree remove <branch|path> [-d <folder>] [--branch] [--force]")
	}
	dir, err := worktreeDir(c, f)
	if err != nil {
		return err
	}
	var list proto.WorktreeList
	if err := c.Call("worktree.list", proto.WorktreeListParams{Dir: dir}, &list); err != nil {
		return err
	}
	ref := f.pos[0]
	var wt *proto.WorktreeInfo
	for i := range list.Worktrees {
		w := &list.Worktrees[i]
		if w.Branch == ref || samePathCLI(w.Path, hereFile(ref, onHost)) || samePathCLI(w.Path, ref) {
			wt = w
			break
		}
	}
	switch {
	case wt == nil:
		return fmt.Errorf("%w: no worktree %q of %s (see kero worktree list)", errNotFound, ref, proto.ShortDir(list.Main))
	case wt.Main:
		return errors.New("that is the repository's main checkout, which stays")
	case wt.Session != "":
		// Its session's own: closing the session removes it, as the app's
		// Remove does.
		if err := c.Call("close", proto.CloseParams{ID: wt.Session, RemoveWorktree: true, Force: f.set["force"]}, nil); err != nil {
			return err
		}
		fmt.Printf("closed session %s, and removed its worktree %s\n", wt.Session, proto.ShortDir(wt.Path))
		return nil
	case len(wt.Sessions) > 0:
		return fmt.Errorf("sessions work in it: %s; close them first", strings.Join(wt.Sessions, ", "))
	case wt.Files > 0 && !f.set["force"]:
		return fmt.Errorf("it has %d file(s) changed, which removing it loses: --force removes it anyway", wt.Files)
	}
	p := proto.GitOpParams{Op: "worktreeRemove", Dir: list.Main, Path: wt.Path, Force: f.set["force"]}
	if f.set["branch"] {
		p.Name = wt.Branch
	}
	var res proto.GitOpResult
	if err := c.Call("git.op", p, &res); err != nil {
		return err
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	fmt.Println("removed", proto.ShortDir(wt.Path))
	return nil
}

// samePathCLI compares two paths as the file system would, near enough
// for a path the user typed: cleaned, and of any case on Windows.
func samePathCLI(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	return proto.Within(a, b) && proto.Within(b, a)
}

// worktreeCleanup is kero worktree cleanup: Kero's automatic cleanup,
// now, as Settings › Git's Clean Up Now runs it; --dry-run says what it
// would remove, of each project's repository.
func worktreeCleanup(c *client.Client, args []string) error {
	f, err := parse(args, nil, map[string]string{"dry-run": "dry", "n": "dry", "json": "json"})
	if err != nil {
		return err
	}
	if !f.set["dry"] {
		var res proto.WorktreeCleanupResult
		if err := c.Call("worktree.cleanup", nil, &res); err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(res)
		}
		for _, r := range res.Removed {
			fmt.Printf("removed %s (%s): %s\n", proto.ShortDir(r.Path), cmpDash(r.Branch), r.Reason)
		}
		for _, e := range res.Failed {
			fmt.Println("failed:", e)
		}
		if len(res.Removed) == 0 && len(res.Failed) == 0 {
			fmt.Println("nothing to clean up (kero worktree list tells why each stays)")
		}
		return nil
	}
	var st proto.State
	if err := c.Call("state", nil, &st); err != nil {
		return err
	}
	seen := map[string]bool{}
	var would []proto.WorktreeInfo
	for _, p := range st.Projects {
		if p.Dir == "" {
			continue
		}
		var list proto.WorktreeList
		if c.Call("worktree.list", proto.WorktreeListParams{Dir: p.Dir}, &list) != nil || seen[list.Main] {
			continue
		}
		seen[list.Main] = true
		for _, w := range list.Worktrees {
			if w.Removes != "" {
				would = append(would, w)
			}
		}
	}
	if f.set["json"] {
		return printJSON(would)
	}
	if len(would) == 0 {
		fmt.Println("the cleanup would remove nothing (kero worktree list tells why each stays)")
	}
	for _, w := range would {
		fmt.Printf("would remove %s (%s): %s\n", proto.ShortDir(w.Path), cmpDash(w.Branch), w.Removes)
	}
	return nil
}

// worktreePrune is kero worktree prune: git worktree prune, of the
// records of worktrees whose folders are gone, as the History tab's Prune.
func worktreePrune(c *client.Client, args []string) error {
	f, err := parse(args, map[string]string{"d": "dir", "dir": "dir", "p": "project", "project": "project"}, nil)
	if err != nil {
		return err
	}
	dir, err := worktreeDir(c, f)
	if err != nil {
		return err
	}
	var res proto.GitOpResult
	if err := c.Call("git.op", proto.GitOpParams{Op: "worktreePrune", Dir: dir}, &res); err != nil {
		return err
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	if out := strings.TrimSpace(res.Output); out != "" {
		fmt.Println(out)
	}
	return nil
}

// prList is kero pr list: the repository's open pull requests, as the
// Review tab lists them.
func prList(c *client.Client, args []string) error {
	f, err := parse(args, map[string]string{"d": "dir", "dir": "dir", "p": "project", "project": "project", "remote": "remote", "filter": "filter"},
		map[string]string{"json": "json", "mine": "mine"})
	if err != nil {
		return err
	}
	dir, err := worktreeDir(c, f)
	if err != nil {
		return err
	}
	filter := f.vals["filter"]
	if f.set["mine"] {
		filter = "mine"
	}
	var res proto.PRList
	if err := c.Call("pr.list", proto.PRListParams{Dir: dir, Remote: f.vals["remote"], Filter: filter}, &res); err != nil {
		return err
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	if f.set["json"] {
		return printJSON(res.PRs)
	}
	if len(res.PRs) == 0 {
		fmt.Println("no open pull requests")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NUMBER\tTITLE\tAUTHOR\tBRANCH\tCHECKS\tREVIEW")
	for _, pr := range res.PRs {
		title := pr.Title
		if pr.Draft {
			title = "[draft] " + title
		}
		fmt.Fprintf(w, "#%d\t%s\t%s\t%s\t%s\t%s\n", pr.Number, title, pr.Author, pr.Head, cmpDash(pr.Checks), cmpDash(pr.Review))
	}
	return w.Flush()
}

// prReview is kero pr review <number>: the app starts an agent's review of
// the pull request in a new session in a worktree of its branch, with the
// prompt and agent Settings › Reviews give its repository, as the Review
// tab does; --guide has it write the review guide instead. It needs the
// app open, and Remote Control, as kero do does, but for Ama.
func prReview(c *client.Client, args []string) error {
	f, err := parse(args, map[string]string{"d": "dir", "dir": "dir", "p": "project", "project": "project", "remote": "remote"},
		map[string]string{"guide": "guide"})
	if err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return usageErr("kero pr review <number> [-d <folder>|-p <project>] [--guide]")
	}
	n, err := strconv.Atoi(strings.TrimPrefix(f.pos[0], "#"))
	if err != nil || n <= 0 {
		return usageErr("kero pr review takes a pull request's number, not %q", f.pos[0])
	}
	dir, err := worktreeDir(c, f)
	if err != nil {
		return err
	}
	kv := map[string]string{"number": strconv.Itoa(n), "dir": dir, "remote": f.vals["remote"]}
	if f.set["guide"] {
		kv["guide"] = "true"
	}
	var delivered bool
	if err := c.Call("do", proto.DoParams{Action: actions.PRReview, Args: kv, Token: amaToken()}, &delivered); err != nil {
		return err
	}
	if !delivered {
		return errors.New("the app is not open")
	}
	fmt.Printf("asked the app to start the review of #%d: it shows in the sidebar as #%d (needs Remote Control, but for Ama)\n", n, n)
	return nil
}
