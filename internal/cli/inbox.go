package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"kero/internal/client"
	"kero/internal/proto"
)

// inboxCmd waits for the messages Kero has for the agent of this pane,
// $KERO_PANE (or --pane), and prints each: on one line, its line breaks
// as \n, for Claude Code's monitor of Kero's plugin, which gives each line
// to Claude as an event; or as a line of JSON, {"text": ...}, for the
// extension Kero installs in Pi and OMP. With --follow it waits on, for
// as long as it runs, and through kerod's restarts; kerod gives the
// messages here, rather than typing them into the agent's prompt, while
// it does. Outside Kero it prints nothing.
func inboxCmd(args []string) error {
	f, err := parse(args, map[string]string{"pane": "pane"}, map[string]string{"follow": "follow", "f": "follow", "json": "json"})
	if err != nil {
		return err
	}
	if len(f.pos) > 0 || len(f.rest) > 0 {
		return usageErr("kero inbox [--follow] [--json] [--pane <id>]")
	}
	pane := f.vals["pane"]
	if pane == "" {
		pane = os.Getenv("KERO_PANE")
	}
	if pane == "" {
		return nil
	}
	emit := func(text string) error {
		if f.set["json"] {
			b, _ := json.Marshal(proto.InboxMessage{Text: text})
			_, err := fmt.Printf("%s\n", b)
			return err
		}
		_, err := fmt.Println(inboxEvent(text, inboxDir(), time.Now()))
		return err
	}
	next := func(c *client.Client, timeout time.Duration) (string, error) {
		var m proto.InboxMessage
		// JSON is for the extension in Pi and OMP, which gives each message
		// as the user's turn; a line, Claude Code's monitor's, an event.
		err := c.Call("agent.inbox", proto.InboxParams{Pane: pane, Timeout: timeout.Milliseconds(), Turns: f.set["json"]}, &m)
		return m.Text, err
	}
	if !f.set["follow"] {
		c, err := client.Connect(true)
		if err != nil {
			return fmt.Errorf("%w: %v", errUnreachable, err)
		}
		defer c.Close()
		text, err := next(c, 0)
		if err != nil {
			return err
		}
		return emit(text)
	}
	wait := time.Second
	for {
		c, err := client.Connect(true)
		if err != nil {
			// kerod restarting, or of another version as Kero updates: the
			// agent keeps running, and so does its listener.
			time.Sleep(wait)
			wait = min(2*wait, 10*time.Second)
			continue
		}
		wait = time.Second
		for {
			var text string
			if text, err = next(c, 10*time.Minute); err != nil {
				break
			}
			if text == "" {
				continue
			}
			if err := emit(text); err != nil {
				// The agent is gone: its message waits for the next way to
				// the pane's agent.
				c.Call("agent.inbox", proto.InboxParams{Pane: pane, Back: text}, nil)
				c.Close()
				return nil
			}
		}
		c.Close()
		// The pane is gone, closed or not known to kerod: so is its agent.
		if msg := err.Error(); strings.Contains(msg, "pane closed") || strings.Contains(msg, "no session or pane") {
			return nil
		}
		time.Sleep(time.Second)
	}
}

var oneLine = strings.NewReplacer("\r\n", `\n`, "\n", `\n`, "\r", `\n`)

// inboxLine is text on one line, each line break \n, so that no line of it
// starts another event. A \n the text has as it is stays: the agent reads
// it alike, and a Windows path, as C:\notes, stays as it was.
func inboxLine(text string) string { return oneLine.Replace(text) }

// eventMax is the longest line inboxEvent prints: Claude Code gives Claude
// about the first 500 characters of a monitor's line, and cuts the rest.
const eventMax = 450

// inboxKeep is how long the files of long messages stay, for an agent to
// read again.
const inboxKeep = 7 * 24 * time.Hour

// inboxDir holds the long messages given to agents, a file each.
func inboxDir() string { return filepath.Join(proto.Dir(), "inbox") }

// inboxEvent is text as a monitor's line: whole when it fits in eventMax;
// else its start, and the file in dir it is written to whole, for the
// agent to read. The files older than inboxKeep go as one is written.
func inboxEvent(text, dir string, now time.Time) string {
	line := []rune(inboxLine(text))
	if len(line) <= eventMax {
		return string(line)
	}
	os.MkdirAll(dir, 0o755)
	if old, err := os.ReadDir(dir); err == nil {
		for _, e := range old {
			if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > inboxKeep {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	path := filepath.Join(dir, fmt.Sprintf("message-%d.md", now.UnixNano()))
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		// No file to point to: as much of it as reaches the agent.
		return string(line[:eventMax-1]) + "…"
	}
	tail := []rune(fmt.Sprintf(" … [cut short: the whole message, %d characters, is in %s; read it]", utf8.RuneCountInString(text), path))
	return string(line[:max(0, eventMax-len(tail))]) + string(tail)
}
