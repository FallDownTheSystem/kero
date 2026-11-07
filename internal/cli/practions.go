package cli

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kero/internal/client"
	"kero/internal/proto"
)

// prActionCommands are the kero pr commands that act on a pull request on
// GitHub, as its page in Kero does (pr.action), or check it out
// (pr.checkout).
var prActionCommands = map[string]string{
	"state":         proto.PRActState,
	"merge":         proto.PRActMerge,
	"ready":         proto.PRActReady,
	"draft":         proto.PRActDraft,
	"close":         proto.PRActClose,
	"reopen":        proto.PRActReopen,
	"update-branch": proto.PRActUpdateBranch,
	"delete-branch": proto.PRActDeleteBranch,
	"checkout":      "",
}

// prActionCommand runs kero pr state, merge, ready, draft, close, reopen,
// update-branch, delete-branch and checkout: of the pull request given by
// its number, else the one the session is linked to (kero pr link), of
// the repository of the pane it runs in.
func prActionCommand(c *client.Client, args []string) error {
	sub := args[0]
	f, err := parse(args[1:], map[string]string{"remote": "remote", "head": "head", "name": "name", "n": "name"},
		map[string]string{"json": "json", "merge": "merge", "squash": "squash", "rebase": "rebase", "auto": "auto",
			"disable-auto": "disable-auto", "delete-branch": "delete-branch", "worktree": "worktree"})
	if err != nil {
		return err
	}
	pane := current()
	if pane == "" {
		return usageErr("kero pr %s runs in a pane of Kero, in the pull request's repository", sub)
	}
	var st proto.State
	if err := c.Call("state", nil, &st); err != nil {
		return err
	}
	t, err := find(st, pane)
	if err != nil {
		return err
	}
	remote := f.vals["remote"]
	var number int
	switch len(f.pos) {
	case 0:
		if t.s.PR == nil {
			return usageErr("which pull request? give its number, as kero pr %s 123: this session is linked to none (kero pr link)", sub)
		}
		number = t.s.PR.Number
		if remote == "" {
			remote = t.s.PR.Remote
		}
	case 1:
		number, err = strconv.Atoi(strings.TrimPrefix(f.pos[0], "#"))
		if err != nil || number <= 0 {
			return usageErr("%q is no pull request's number", f.pos[0])
		}
	default:
		if sub != "checkout" {
			return usageErr("kero pr %s takes one pull request's number", sub)
		}
	}
	if sub == "checkout" {
		return prCheckoutCommand(c, t, f, remote, number)
	}
	p := proto.PRActionParams{Pane: pane, Remote: remote, Number: number, Op: prActionCommands[sub], Head: f.vals["head"]}
	switch sub {
	case "merge":
		for _, m := range []string{"merge", "squash", "rebase"} {
			if f.set[m] {
				if p.Method != "" {
					return usageErr("kero pr merge takes one of --merge, --squash and --rebase")
				}
				p.Method = m
			}
		}
		p.DeleteBranch = f.set["delete-branch"]
		switch {
		case f.set["auto"] && f.set["disable-auto"]:
			return usageErr("kero pr merge takes --auto or --disable-auto, not both")
		case f.set["auto"]:
			p.Op = proto.PRActAutoMerge
		case f.set["disable-auto"]:
			p.Op, p.Method = proto.PRActDisableAutoMerge, ""
		}
	case "update-branch":
		if f.set["rebase"] {
			p.Method = "rebase"
		}
	}
	var res proto.PRActionResult
	if err := c.Call("pr.action", p, &res); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(res)
	}
	switch p.Op {
	case proto.PRActState:
	case proto.PRActAutoMerge:
		fmt.Printf("#%d merges once its requirements are met\n", number)
	case proto.PRActDisableAutoMerge:
		fmt.Printf("#%d no longer merges by itself\n", number)
	default:
		fmt.Printf("#%d %s\n", number, map[string]string{"merge": "merged", "ready": "is ready for review", "draft": "is a draft again",
			"close": "closed", "reopen": "reopened", "update-branch": "has its branch updated", "delete-branch": "has its branch deleted"}[sub])
	}
	printPRActions(number, res.State)
	return nil
}

// printPRActions writes how a pull request stands, as its actions read
// it: open or not, a draft, whether it merges and how.
func printPRActions(number int, a *proto.PRActions) {
	if a == nil {
		return
	}
	var parts []string
	state := strings.ToLower(a.State)
	if a.Draft {
		state += ", draft"
	}
	parts = append(parts, fmt.Sprintf("#%d %s, %s into %s", number, state, a.Head, a.Base))
	if a.State == "OPEN" {
		if a.Mergeable == "CONFLICTING" {
			parts = append(parts, "merge conflicts")
		} else if a.MergeState != "" {
			parts = append(parts, "merge state "+strings.ToLower(a.MergeState))
		}
		if a.AutoMerge != "" {
			parts = append(parts, "auto-merge by "+strings.ToLower(a.AutoMerge))
		}
		if len(a.Methods) > 0 {
			parts = append(parts, "merges by "+strings.Join(a.Methods, ", "))
		}
	}
	if !a.CanWrite {
		parts = append(parts, "you may not merge it")
	}
	fmt.Println(strings.Join(parts, "; "))
}

// prCheckoutCommand runs kero pr checkout: the pull request's branch,
// checked out in this pane's worktree, which refuses one with changes; or
// with --worktree, a new session in a worktree of its own, running the
// command after --, the shell else. A worktree that has the pull request
// already is used as it is.
func prCheckoutCommand(c *client.Client, t target, f *flags, remote string, number int) error {
	pane := t.ref()
	worktree := f.set["worktree"]
	if len(f.rest) > 0 && !worktree {
		return usageErr("a command after -- runs in a new --worktree's session")
	}
	// The pull request's branch and repository, which the checkout fetches.
	var res proto.PRActionResult
	if err := c.Call("pr.action", proto.PRActionParams{Pane: pane, Remote: remote, Number: number}, &res); err != nil {
		return err
	}
	if res.State == nil {
		return errors.New("couldn't read the pull request")
	}
	var got proto.PRCheckedOut
	if err := c.Call("pr.checkout", proto.PRCheckoutParams{Pane: pane, Remote: remote, Number: number,
		Head: res.State.Head, CrossRepo: res.State.CrossRepo, Here: !worktree}, &got); err != nil {
		return err
	}
	if got.Worktree != "" && !worktree {
		return fmt.Errorf("#%d is checked out in %s already: work there, or kero pr checkout %d --worktree for a session in it", number, got.Worktree, number)
	}
	if !worktree {
		if f.set["json"] {
			return printJSON(got)
		}
		fmt.Printf("Checked out #%d here, on %s\n", number, got.Branch)
		return nil
	}
	if got.Worktree != "" && len(f.rest) == 0 {
		// A session works there already: that one, as the app goes to it.
		if id := worktreeSession(c, got.Worktree); id != "" {
			fmt.Fprintf(os.Stderr, "#%d is checked out in %s already, in session %s\n", number, got.Worktree, id)
			fmt.Println(id)
			return nil
		}
	}
	cp := proto.CreateParams{Project: t.s.Project, After: t.s.ID, Name: cmp.Or(f.vals["name"], "#"+strconv.Itoa(number)),
		Command: f.rest}
	if got.Worktree != "" {
		cp.Dir = got.Worktree
	} else {
		cp.Dir = paneDir(t)
		cp.Worktree, cp.Base, cp.PushRemote, cp.PushBranch = got.Branch, got.Base, got.PushRemote, got.PushBranch
	}
	var s proto.Session
	if err := c.Call("create", cp, &s); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(s)
	}
	if got.Worktree != "" {
		fmt.Fprintf(os.Stderr, "#%d is checked out in %s already: started a session there\n", number, got.Worktree)
	}
	fmt.Println(s.ID)
	return nil
}

// worktreeSession is a session that works in the worktree at path, one
// made for it first; "" for none.
func worktreeSession(c *client.Client, path string) string {
	var st proto.State
	if err := c.Call("state", nil, &st); err != nil {
		return ""
	}
	in := ""
	for _, s := range st.Sessions {
		if s.Worktree != nil && filepath.Clean(s.Worktree.Path) == filepath.Clean(path) {
			return s.ID
		}
		for _, p := range s.Panes {
			// By its worktree's root: the main checkout holds the others.
			if in == "" && p.Git != nil && filepath.Clean(p.Git.Root) == filepath.Clean(path) {
				in = s.ID
			}
		}
	}
	return in
}

// paneDir is the folder of the target's pane, its worktree's root when it
// knows one, for a session in a new worktree of the same repository.
func paneDir(t target) string {
	p := t.s.PaneByID(t.pane)
	if p == nil {
		p = t.s.FocusPane()
	}
	if p == nil {
		return ""
	}
	if p.Git != nil && p.Git.Root != "" {
		return p.Git.Root
	}
	return p.WorkDir()
}
