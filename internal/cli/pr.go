package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"kero/internal/client"
	"kero/internal/proto"
)

// prCommand runs kero pr: the draft review of a pull request, which the
// user posts from Kero; nothing here posts.
func prCommand(c *client.Client, args []string) error {
	if len(args) == 0 {
		return usageErr("kero pr comment|edit|delete|summary|show (see kero help)")
	}
	sub := args[0]
	f, err := parse(args[1:], map[string]string{"pr": "pr", "remote": "remote"}, map[string]string{"base": "base", "json": "json"})
	if err != nil {
		return err
	}
	pane := current()
	if pane == "" {
		return usageErr("kero pr runs in a pane of Kero")
	}
	p := proto.PRReviewParams{Pane: pane, Remote: f.vals["remote"]}
	if v := f.vals["pr"]; v != "" {
		n, err := strconv.Atoi(strings.TrimPrefix(v, "#"))
		if err != nil || n <= 0 {
			return usageErr("--pr takes a pull request's number, not %q", v)
		}
		p.Number = n
	}
	words := append(f.pos, f.rest...)
	text := func(from int) (string, error) {
		if len(words) <= from {
			return "", usageErr("no text: give it, or - to read it from stdin")
		}
		if len(words) == from+1 && words[from] == "-" {
			b, err := io.ReadAll(os.Stdin)
			return strings.TrimSpace(string(b)), err
		}
		return strings.Join(words[from:], " "), nil
	}
	id := func() (int, error) {
		if len(words) == 0 {
			return 0, usageErr("which comment? give its id, as kero pr show lists them")
		}
		n, err := strconv.Atoi(strings.TrimPrefix(words[0], "#"))
		if err != nil {
			return 0, usageErr("no comment %q: give its id, as kero pr show lists them", words[0])
		}
		return n, nil
	}
	switch sub {
	case "comment":
		if len(words) == 0 {
			return usageErr("kero pr comment <path>[:<line>[-<end>]] <text…|->")
		}
		body, err := text(1)
		if err != nil {
			return err
		}
		// A path alone is the file's.
		if !strings.Contains(words[0], ":") {
			p.Op, p.Comment = "comment", proto.PRDraftComment{Path: words[0], File: true, Body: body}
			break
		}
		path, start, line, err := linesOf(words[0])
		if err != nil {
			return err
		}
		side := "RIGHT"
		if f.set["base"] {
			side = "LEFT"
		}
		p.Op, p.Comment = "comment", proto.PRDraftComment{Path: path, Side: side, Start: start, Line: line, Body: body}
	case "edit":
		n, err := id()
		if err != nil {
			return err
		}
		body, err := text(1)
		if err != nil {
			return err
		}
		p.Op, p.Comment = "comment", proto.PRDraftComment{ID: n, Body: body}
	case "delete":
		n, err := id()
		if err != nil {
			return err
		}
		p.Op, p.ID = "delete", n
	case "summary":
		body, err := text(0)
		if err != nil {
			return err
		}
		p.Op, p.Summary = "summary", body
	case "show":
		p.Op = "get"
	default:
		return usageErr("no kero pr %s: comment, edit, delete, summary or show", sub)
	}
	var dr *proto.PRReviewDraft
	if err := c.Call("pr.review", p, &dr); err != nil {
		return err
	}
	switch {
	case dr == nil:
		return fmt.Errorf("no draft review: start one from Kero's Review view")
	case sub == "show" && f.set["json"]:
		b, _ := json.MarshalIndent(dr, "", "  ")
		fmt.Println(string(b))
	case sub == "show":
		printReview(dr)
	case sub == "comment" && p.Comment.ID == 0:
		cm := dr.Comments[len(dr.Comments)-1]
		fmt.Printf("draft comment %d on %s\n", cm.ID, placeOf(cm))
	default:
		fmt.Println("ok")
	}
	return nil
}

// linesOf reads path:line or path:start-end.
func linesOf(s string) (path string, start, line int, err error) {
	i := strings.LastIndex(s, ":")
	if i <= 0 {
		return "", 0, 0, usageErr("%q names no lines: give <path>:<line> or <path>:<start>-<end>", s)
	}
	path = s[:i]
	from, to, ranged := strings.Cut(s[i+1:], "-")
	if line, err = strconv.Atoi(from); err != nil || line <= 0 {
		return "", 0, 0, usageErr("%q names no lines: give <path>:<line> or <path>:<start>-<end>", s)
	}
	if ranged {
		start = line
		if line, err = strconv.Atoi(to); err != nil || line < start {
			return "", 0, 0, usageErr("%q names no lines: give <path>:<start>-<end>", s)
		}
	}
	return path, start, line, nil
}

// placeOf is where a comment is, as path:12 or path:10-12, the base's
// lines said so, or path (file).
func placeOf(c proto.PRDraftComment) string {
	if c.File {
		return c.Path + " (file)"
	}
	s := c.Path + ":" + strconv.Itoa(c.Line)
	if c.Start > 0 && c.Start < c.Line {
		s = c.Path + ":" + strconv.Itoa(c.Start) + "-" + strconv.Itoa(c.Line)
	}
	if c.Side == "LEFT" {
		s += " (base)"
	}
	return s
}

func printReview(dr *proto.PRReviewDraft) {
	fmt.Printf("%s#%d %s\n", dr.Repo, dr.Number, dr.Title)
	if dr.Summary != "" {
		fmt.Printf("\nsummary:\n%s\n", indent(dr.Summary))
	}
	if len(dr.Comments) == 0 {
		fmt.Println("\nno comments")
	}
	for _, c := range dr.Comments {
		fmt.Printf("\n%d  %s  [%s]\n%s\n", c.ID, placeOf(c), c.By, indent(c.Body))
	}
}

func indent(s string) string { return "    " + strings.ReplaceAll(s, "\n", "\n    ") }
