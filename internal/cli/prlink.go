package cli

import (
	"fmt"
	"strconv"
	"strings"

	"kero/internal/client"
	"kero/internal/proto"
)

// prLinkCommand runs kero pr link, unlink and status: the pull request a
// session works on, which kerod watches for the session's agent.
func prLinkCommand(c *client.Client, args []string) error {
	sub := args[0]
	f, err := parse(args[1:], map[string]string{"session": "target", "s": "target", "remote": "remote"}, map[string]string{"json": "json"})
	if err != nil {
		return err
	}
	ref := f.vals["target"]
	if ref == "" {
		ref = current()
	}
	if ref == "" {
		return usageErr("which session? run it in Kero, or give --session")
	}
	p := proto.PRLinkParams{Session: ref, Remote: f.vals["remote"]}
	switch sub {
	case "link":
		if len(f.pos) > 1 {
			return usageErr("kero pr link [<number>|<url>] [--remote <name>] [--session <session>]")
		}
		if len(f.pos) == 1 {
			v := f.pos[0]
			if n, err := strconv.Atoi(strings.TrimPrefix(v, "#")); err == nil && n > 0 {
				p.Number = n
			} else if strings.Contains(v, "/pull/") {
				p.URL = v
			} else {
				return usageErr("%q is no pull request's number or URL", v)
			}
		}
	case "unlink":
		p.Unlink = true
	case "status":
		var st proto.State
		if err := c.Call("state", nil, &st); err != nil {
			return err
		}
		t, err := find(st, ref)
		if err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(t.s.PR)
		}
		printPRLink(t.s.PR)
		return nil
	}
	var link *proto.SessionPR
	if err := c.Call("pr.link", p, &link); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(link)
	}
	if sub == "unlink" {
		fmt.Println("unlinked")
		return nil
	}
	printPRLink(link)
	return nil
}

func printPRLink(l *proto.SessionPR) {
	if l == nil {
		fmt.Println("no pull request")
		return
	}
	how := "found by its branch"
	if l.Manual {
		how = "linked by hand"
	}
	fmt.Printf("%s#%d %s\n%s\n%s", l.Slug, l.Number, l.Title, l.URL, how)
	if l.State != "" && l.State != "OPEN" {
		fmt.Printf(", %s", strings.ToLower(l.State))
	}
	if l.Checks != "" {
		fmt.Printf(", checks %s", l.Checks)
	}
	if len(l.Failed) > 0 {
		fmt.Printf(" (%s)", strings.Join(l.Failed, ", "))
	}
	if l.Mergeable == "CONFLICTING" {
		fmt.Print(", merge conflicts")
	}
	fmt.Println()
}
