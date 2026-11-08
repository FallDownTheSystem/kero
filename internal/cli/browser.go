package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kero/internal/client"
	"kero/internal/proto"
)

// browserCommands are what kero browser does to a page, which name no
// target: kero browser text is this session's browser's text.
var browserCommands = []string{"open", "back", "forward", "reload", "text", "html", "snapshot", "click", "type", "wait", "eval", "screenshot", "cookies", "wait-message", "pick", "console", "network", "emulate"}

// browserCmd drives a browser pane's page, which the app has: kerod asks
// the app, and hands its answer back.
func browserCmd(c *client.Client, st proto.State, args []string) error {
	f, err := parse(args, map[string]string{"timeout": "timeout", "t": "timeout", "o": "out", "out": "out", "text": "text", "ref": "ref", "domain": "domain", "url": "url", "scale": "scale"},
		map[string]string{"enter": "enter", "E": "enter", "json": "json", "errors": "errors", "clear": "clear", "failed": "failed", "mobile": "mobile", "touch": "touch"})
	if err != nil {
		return err
	}
	const use = "kero browser [target] <open|back|forward|reload|text|html|snapshot|click|type|wait|eval|screenshot|cookies|wait-message|pick|console|network|emulate> [args…]"
	pos := f.pos
	var t target
	if len(pos) > 0 && !slices.Contains(browserCommands, pos[0]) {
		if t, err = find(st, pos[0]); err != nil {
			return err
		}
		pos = pos[1:]
	} else {
		if t, err = findOrCurrent(st, nil); err != nil {
			return usageErr("which browser? (outside kero, name one): %s", use)
		}
		// The pane a command runs in is a terminal: its session's
		// browser is meant.
		t.pane = ""
	}
	if len(pos) == 0 {
		return usageErr("%s", use)
	}
	op, pos := pos[0], pos[1:]
	if !slices.Contains(browserCommands, op) {
		return usageErr("no browser command %q: %s", op, use)
	}
	pane, err := browserOf(t)
	if err != nil {
		return err
	}
	timeout, err := parseTimeout(f.vals["timeout"])
	if err != nil {
		return err
	}
	if slices.Contains(devlogCommands, op) {
		return devlogCmd(c, proto.BrowserParams{ID: pane, Op: op, Timeout: timeout, Ref: f.vals["ref"]}, op, pos, f)
	}
	if err := devlogFlag(f, op); err != nil {
		return err
	}
	if f.set["json"] && op != "wait-message" {
		return usageErr("--json is wait-message's, console's, network's and emulate's")
	}
	if op == "wait-message" {
		return waitMessage(c, pane, timeout, pos, f)
	}
	p := proto.BrowserParams{ID: pane, Op: op, Timeout: timeout, Ref: f.vals["ref"], Token: amaToken()}
	// --ref names the element in place of the selector.
	sel := func(pos []string) []string {
		if p.Ref == "" {
			return pos
		}
		return append([]string{""}, pos...)
	}
	// The text after the arguments, as kero send takes it: all of them,
	// those after -- too, or - for stdin.
	rest := func(from []string) (string, error) {
		text := strings.Join(append(from, f.rest...), " ")
		if text == "-" {
			b, err := io.ReadAll(os.Stdin)
			return string(b), err
		}
		return text, nil
	}
	switch op {
	case "open":
		if p.Text, err = rest(pos); err != nil {
			return err
		}
		if strings.TrimSpace(p.Text) == "" {
			return usageErr("kero browser [target] open <url>")
		}
	case "back", "forward", "reload", "screenshot", "pick":
		if len(pos) > 0 || len(f.rest) > 0 {
			return usageErr("kero browser [target] %s takes no arguments", op)
		}
	case "text", "html", "snapshot":
		if len(pos) > 1 || len(pos) == 1 && p.Ref != "" {
			return usageErr("kero browser [target] %s [selector | --ref ref]", op)
		}
		if len(pos) == 1 {
			p.Selector = pos[0]
		}
	case "click":
		if pos = sel(pos); len(pos) != 1 {
			return usageErr("kero browser [target] click <selector | --ref ref>")
		}
		p.Selector = pos[0]
	case "type":
		if pos = sel(pos); len(pos) < 1 {
			return usageErr("kero browser [target] type <selector | --ref ref> <text…> [--enter]")
		}
		p.Selector, p.Enter = pos[0], f.set["enter"]
		if p.Text, err = rest(pos[1:]); err != nil {
			return err
		}
	case "wait":
		if len(pos) > 1 || len(pos) == 1 && p.Ref != "" {
			return usageErr("kero browser [target] wait [selector | --ref ref] [--text text]")
		}
		if len(pos) == 1 {
			p.Selector = pos[0]
		}
		p.Text = f.vals["text"]
	case "cookies":
		return cookiesCmd(c, p, pos, f)
	case "eval":
		if p.Text, err = rest(pos); err != nil {
			return err
		}
		if strings.TrimSpace(p.Text) == "" {
			return usageErr("kero browser [target] eval <js…>")
		}
	}
	if f.vals["out"] != "" && op != "screenshot" {
		return usageErr("-o is screenshot's")
	}
	if f.set["enter"] && op != "type" {
		return usageErr("--enter is type's")
	}
	if f.vals["text"] != "" && op != "wait" {
		return usageErr("--text is wait's")
	}
	if p.Ref != "" && !slices.Contains([]string{"text", "html", "snapshot", "click", "type", "wait"}, op) {
		return usageErr("--ref names an element, which %s takes none of", op)
	}
	if f.vals["domain"] != "" {
		return usageErr("--domain is cookies'")
	}
	var res proto.BrowserResult
	if err := c.Call("browser", p, &res); err != nil {
		return err
	}
	if op == "screenshot" {
		return saveShot(res.PNG, pane, f.vals["out"])
	}
	if res.Text != "" {
		fmt.Print(res.Text)
		if !strings.HasSuffix(res.Text, "\n") {
			fmt.Println()
		}
	}
	return nil
}

// browserOf is the browser pane a target stands for: the pane named, or
// for a session, its focused pane when that is a browser, else its one
// browser.
func browserOf(t target) (string, error) {
	if t.pane != "" {
		if p := t.s.PaneByID(t.pane); p == nil || !p.Browser() {
			return "", fmt.Errorf("%w: pane %s is a terminal, not a browser", errNotFound, t.pane)
		}
		return t.pane, nil
	}
	if p := t.s.FocusPane(); p != nil && p.Browser() {
		return p.ID, nil
	}
	var ids []string
	for _, p := range t.s.Panes {
		if p.Browser() {
			ids = append(ids, p.ID)
		}
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return "", fmt.Errorf("%w: session %s has no browser pane (open one with kero split --url <url>)", errNotFound, t.s.ID)
	}
	return "", fmt.Errorf("%w: session %s has several browser panes, name one: %s", errNotFound, t.s.ID, strings.Join(ids, ", "))
}

// saveShot writes a screenshot where -o says, - for stdout, else to a
// file of its own in the temporary directory, and prints where it went.
func saveShot(png []byte, pane, out string) error {
	if len(png) == 0 {
		return errors.New("the page gave no picture")
	}
	if out == "-" {
		_, err := os.Stdout.Write(png)
		return err
	}
	if out == "" {
		out = filepath.Join(os.TempDir(), fmt.Sprintf("kero-%s-%s.png", pane, time.Now().Format("20060102-150405.000")))
	}
	if err := os.WriteFile(out, png, 0o644); err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

// waitMessage waits for the next message the browser pane's page sends
// with window.kero.send, and prints it: labelled with the page, or as
// JSON with --json. Messages sent while no agent waited are kept for it,
// in order, when the page sent them to be waited for (mode "wait").
func waitMessage(c *client.Client, pane string, timeout int64, pos []string, f *flags) error {
	if len(pos) > 0 || len(f.rest) > 0 {
		return usageErr("kero browser [target] wait-message [--timeout 10m] [--json]")
	}
	for _, k := range []string{"out", "text", "ref", "domain"} {
		if f.vals[k] != "" {
			return usageErr("wait-message takes only --timeout and --json")
		}
	}
	if f.set["enter"] {
		return usageErr("--enter is type's")
	}
	var msg proto.AgentMessage
	if err := c.Call("browser.message", proto.MessageParams{ID: pane, Timeout: timeout}, &msg); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(msg)
	}
	fmt.Println(msg.Prompt())
	return nil
}
