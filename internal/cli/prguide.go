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

// guideCommand runs kero pr guide: the review guide of a pull request, a
// tour of it in reading order, which the user reads in Kero's Guide tab;
// nothing of it goes to GitHub.
func guideCommand(c *client.Client, args []string) error {
	const usage = "kero pr guide stop|edit|delete|summary|clear|show (see kero help)"
	if len(args) == 0 {
		return usageErr(usage)
	}
	sub := args[0]
	f, err := parse(args[1:], map[string]string{"pr": "pr", "remote": "remote", "title": "title", "t": "title", "at": "at"},
		map[string]string{"base": "base", "json": "json", "closely": "closely", "not-closely": "not-closely"})
	if err != nil {
		return err
	}
	pane := current()
	if pane == "" {
		return usageErr("kero pr guide runs in a pane of Kero")
	}
	p := proto.PRGuideParams{Pane: pane, Remote: f.vals["remote"]}
	if v := f.vals["pr"]; v != "" {
		n, err := strconv.Atoi(strings.TrimPrefix(v, "#"))
		if err != nil || n <= 0 {
			return usageErr("--pr takes a pull request's number, not %q", v)
		}
		p.Number = n
	}
	if v := f.vals["at"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return usageErr("--at takes a stop's place in the tour, from 1, not %q", v)
		}
		p.At = n
	}
	words := append(f.pos, f.rest...)
	text := func(from int, need bool) (string, error) {
		if len(words) <= from {
			if need {
				return "", usageErr("no text: give it, or - to read it from stdin")
			}
			return "", nil
		}
		if len(words) == from+1 && words[from] == "-" {
			b, err := io.ReadAll(os.Stdin)
			return strings.TrimSpace(string(b)), err
		}
		return strings.Join(words[from:], " "), nil
	}
	id := func() (int, error) {
		if len(words) == 0 {
			return 0, usageErr("which stop? give its id, as kero pr guide show lists them")
		}
		n, err := strconv.Atoi(strings.TrimPrefix(words[0], "#"))
		if err != nil {
			return 0, usageErr("no stop %q: give its id, as kero pr guide show lists them", words[0])
		}
		return n, nil
	}
	side := "RIGHT"
	if f.set["base"] {
		side = "LEFT"
	}
	switch sub {
	case "stop":
		if len(words) == 0 {
			return usageErr("kero pr guide stop <path>[:<line>[-<end>]] --title <title> [note…|-]")
		}
		note, err := text(1, false)
		if err != nil {
			return err
		}
		st := proto.PRGuideStop{Path: words[0], Title: f.vals["title"], Note: note, Closely: f.set["closely"]}
		// A path alone is the whole file.
		if strings.Contains(words[0], ":") {
			path, start, line, err := linesOf(words[0])
			if err != nil {
				return err
			}
			st.Path, st.Side, st.Start, st.Line = path, side, start, line
		}
		p.Op, p.Stop = "stop", st
	case "edit":
		n, err := id()
		if err != nil {
			return err
		}
		note, err := text(1, false)
		if err != nil {
			return err
		}
		// What the edit leaves out stays: Closely, as the guide has it.
		var g *proto.PRGuide
		get := p
		get.Op = "get"
		if err := c.Call("pr.guide", get, &g); err != nil {
			return err
		}
		closely := false
		if g != nil {
			for _, s := range g.Stops {
				if s.ID == n {
					closely = s.Closely
				}
			}
		}
		switch {
		case f.set["closely"]:
			closely = true
		case f.set["not-closely"]:
			closely = false
		}
		p.Op, p.Stop = "stop", proto.PRGuideStop{ID: n, Title: f.vals["title"], Note: note, Closely: closely}
	case "move":
		n, err := id()
		if err != nil {
			return err
		}
		if p.At == 0 {
			return usageErr("kero pr guide move <id> --at <place>")
		}
		var g *proto.PRGuide
		get := p
		get.Op = "get"
		if err := c.Call("pr.guide", get, &g); err != nil {
			return err
		}
		closely := false
		if g != nil {
			for _, s := range g.Stops {
				if s.ID == n {
					closely = s.Closely
				}
			}
		}
		p.Op, p.Stop = "stop", proto.PRGuideStop{ID: n, Closely: closely}
	case "delete":
		n, err := id()
		if err != nil {
			return err
		}
		p.Op, p.ID = "delete", n
	case "summary":
		body, err := text(0, true)
		if err != nil {
			return err
		}
		p.Op, p.Summary = "summary", body
	case "clear":
		p.Op = "clear"
	case "show":
		p.Op = "get"
	default:
		return usageErr("no kero pr guide %s: stop, edit, move, delete, summary, clear or show", sub)
	}
	var g *proto.PRGuide
	if err := c.Call("pr.guide", p, &g); err != nil {
		return err
	}
	switch {
	case g == nil:
		return fmt.Errorf("no guide: name the pull request with --pr <number>")
	case sub == "show" && f.set["json"]:
		b, _ := json.MarshalIndent(g, "", "  ")
		fmt.Println(string(b))
	case sub == "show":
		printGuide(g)
	case sub == "stop":
		for i, s := range g.Stops {
			if s.ID > p.Stop.ID {
				p.Stop.ID = s.ID
				p.At = i + 1
			}
		}
		fmt.Printf("stop %d (%d of %d) on %s\n", p.Stop.ID, p.At, len(g.Stops), stopPlace(g.Stops[p.At-1]))
	default:
		fmt.Println("ok")
	}
	return nil
}

// stopPlace is where a stop is, as path:12, path:10-12, the base's lines
// said so, or path (file).
func stopPlace(s proto.PRGuideStop) string {
	if s.Line == 0 {
		return s.Path + " (file)"
	}
	return placeOf(proto.PRDraftComment{Path: s.Path, Side: s.Side, Start: s.Start, Line: s.Line})
}

func printGuide(g *proto.PRGuide) {
	fmt.Printf("%s#%d %s\n", g.Repo, g.Number, g.Title)
	if g.Summary != "" {
		fmt.Printf("\nsummary:\n%s\n", indent(g.Summary))
	}
	if len(g.Stops) == 0 {
		fmt.Println("\nno stops")
	}
	for i, s := range g.Stops {
		closely := ""
		if s.Closely {
			closely = "  [look closely]"
		}
		fmt.Printf("\n%d. (id %d)  %s  %s%s\n", i+1, s.ID, stopPlace(s), s.Title, closely)
		if s.Note != "" {
			fmt.Println(indent(s.Note))
		}
	}
}
