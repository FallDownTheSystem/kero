package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"kero/internal/client"
	"kero/internal/proto"
)

// worktreeListCommand runs kero worktree list: the worktrees of the
// repository of the pane it runs in, else of the current folder, with
// what git worktree list doesn't know.
func worktreeListCommand(c *client.Client, args []string) error {
	f, err := parse(args, map[string]string{"d": "dir", "dir": "dir"}, map[string]string{"json": "json"})
	if err != nil {
		return err
	}
	if len(f.pos) > 0 {
		return usageErr("kero worktree list [-d <dir>] [--json]")
	}
	p := proto.WorktreeListParams{Dir: f.vals["dir"]}
	if p.Dir == "" {
		if p.Pane = current(); p.Pane == "" {
			p.Dir, _ = os.Getwd()
		}
	}
	var res proto.WorktreeList
	if err := c.Call("worktree.list", p, &res); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(res)
	}
	fmt.Printf("%s, automatic cleanup %s\n", proto.ShortDir(res.Main), res.Cleanup)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PATH\tBRANCH\tSESSIONS\tKERO\tREMOVING LOSES\tCLEANUP")
	for _, wt := range res.Worktrees {
		branch := wt.Branch
		if branch == "" {
			branch = "(detached)"
		}
		path := proto.ShortDir(wt.Path)
		if wt.Main {
			path += " (main)"
		}
		sessions := strings.Join(wt.Sessions, ",")
		kero := "-"
		switch {
		case wt.Session != "":
			kero = "for " + wt.Session
			if wt.Base != "" {
				kero += " from " + wt.Base
			}
		case wt.Kero:
			kero = "yes"
		}
		cleanup := "-"
		if wt.Removes != "" {
			cleanup = "removes: " + wt.Removes
		} else if wt.Keeps != "" {
			cleanup = "keeps: " + wt.Keeps
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", path, branch, cmpDash(sessions), kero, losses(wt.GitWorktree, res.Default), cleanup)
	}
	return w.Flush()
}

// losses writes what removing a worktree, and its branch once merged,
// would lose, as the History tab tells it.
func losses(w proto.GitWorktree, def string) string {
	switch {
	case w.Main:
		return "-"
	case w.Prunable:
		return "nothing (folder gone)"
	case !w.State:
		return "?"
	}
	var parts []string
	if w.Files == 1 {
		parts = append(parts, "1 file changed")
	} else if w.Files > 1 {
		parts = append(parts, fmt.Sprintf("%d files changed", w.Files))
	}
	if w.Unpushed == 1 {
		parts = append(parts, "1 commit unpushed")
	} else if w.Unpushed > 1 {
		parts = append(parts, fmt.Sprintf("%d commits unpushed", w.Unpushed))
	}
	if len(parts) == 0 {
		parts = append(parts, "nothing")
	}
	if w.Merged && def != "" {
		parts = append(parts, "merged into "+def)
	}
	return strings.Join(parts, ", ")
}

func cmpDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// worktreeStatusCommand runs kero worktree status: how the session it
// runs in stands with its worktree, as an agent asks before a handoff.
func worktreeStatusCommand(c *client.Client, args []string) error {
	f, err := parse(args, nil, map[string]string{"json": "json"})
	if err != nil {
		return err
	}
	pane := current()
	if pane == "" {
		return usageErr("kero worktree status runs in a pane of Kero")
	}
	var st proto.WorktreeStatus
	if err := c.Call("worktree.status", proto.WorktreeStatusParams{Pane: pane}, &st); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(st)
	}
	fmt.Printf("session %s, pane %s, in %s\n", st.Session, st.Pane, proto.ShortDir(st.Dir))
	switch {
	case st.Root == "":
		fmt.Println("not in a git repository")
	case st.Linked:
		fmt.Printf("in the linked worktree %s, on %s\n", proto.ShortDir(st.Root), cmpDash(st.Branch))
	default:
		fmt.Printf("in the main checkout %s, on %s\n", proto.ShortDir(st.Root), cmpDash(st.Branch))
	}
	if st.Main != "" {
		fmt.Printf("project root %s, default branch %s\n", proto.ShortDir(st.Main), cmpDash(st.Default))
	}
	if st.Own != nil {
		from := ""
		if st.Own.Base != "" {
			from = " from " + st.Own.Base
		}
		fmt.Printf("the session's own worktree, made by Kero: %s, on %s%s\n", proto.ShortDir(st.Own.Path), st.Own.Branch, from)
	} else {
		fmt.Println("the session has no worktree of its own")
	}
	if su := st.Setup; su != nil {
		line := "setup " + su.Phase
		if su.Script != "" {
			line += " (" + su.Script + ")"
		}
		if su.Error != "" {
			line += ": " + su.Error
		}
		if su.Pane != "" && su.Running() {
			line += ", in pane " + su.Pane
		}
		fmt.Println(line)
	}
	if st.PR != nil {
		fmt.Print("pull request: ")
		printPRLink(st.PR)
	} else {
		fmt.Println("no pull request linked")
	}
	origin := "the local base branch"
	if st.FromOrigin {
		origin = "origin's copy of the base branch, fetched"
	}
	fmt.Printf("new worktrees start from %s (worktree-from-origin), of the %s branch (worktree-base)\n", origin, map[string]string{"current": "folder's", "default": "default"}[st.WorktreeBase])
	if st.Handoff == "" {
		fmt.Println("kero worktree handoff <branch> can move this session into a new worktree")
	} else {
		fmt.Println("kero worktree handoff refuses: " + st.Handoff)
	}
	return nil
}
