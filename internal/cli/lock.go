package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"kero/internal/client"
	"kero/internal/heavy"
	"kero/internal/proto"
	"kero/internal/settings"
)

// heavyTimeout is the timeout, in seconds, of the hook that holds a
// build for a slot of heavy-slots, and heavyWait how long it waits: short
// of its timeout, so that the agent never sees the hook fail, and runs the
// command anyway once the wait runs out.
const (
	heavyTimeout = 600
	heavyWait    = 540 * time.Second
)

// heavyEnv tells a command kero lock heavy runs that it holds a slot: a
// kero lock heavy inside it, as a script's, runs at once.
const heavyEnv = "KERO_HEAVY_SLOT"

// heavyCommand is the shell command of a tool call's input, "" for none.
func heavyCommand(in hookInput) string {
	var input struct {
		Command any `json:"command"`
	}
	json.Unmarshal(in.ToolInput, &input)
	switch c := input.Command.(type) {
	case string:
		return c
	case []any: // Codex's older shell tool: ["bash", "-lc", "…"]
		var words []string
		for _, w := range c {
			if s, ok := w.(string); ok {
				words = append(words, s)
			}
		}
		return strings.Join(words, " ")
	}
	return ""
}

// slotKey names a tool call's slot, to give it back by as the call ends.
func slotKey(agent string, in hookInput) string {
	if in.ToolUseID == "" {
		return ""
	}
	return agent + ":" + in.Session + ":" + in.ToolUseID
}

// holdHeavy holds a build or a test run an agent's tool call starts
// until a slot of heavy-slots is free, as its PreToolUse hook: it decides
// nothing, so the agent's own permissions go on as they would, and says
// so to the agent when it waited, or ran out of waiting. Outside Kero, or
// on any trouble, it lets the command run at once.
func holdHeavy(agent string, in hookInput, pane string) error {
	cmd := heavyCommand(in)
	set := settings.Load()
	if cmd == "" || set.HeavySlots <= 0 || !heavy.Heavy(cmd, set.HeavyCommands) {
		return nil
	}
	c, err := client.Connect(true)
	if err != nil {
		return nil
	}
	defer c.Close()
	var r proto.SlotResult
	p := proto.SlotParams{Pane: pane, PID: os.Getpid(), Agent: agent, Session: in.Session, Dir: in.Cwd,
		Key: slotKey(agent, in), Command: cmd, Wait: heavyWait.Milliseconds()}
	if c.Call("slot.take", p, &r) != nil || r.Off {
		return nil
	}
	waited := time.Duration(r.Waited) * time.Millisecond
	var note string
	switch {
	case !r.Got:
		note = fmt.Sprintf("Kero held this command %s for a build slot (heavy-slots: %d builds and test runs at once over the machine) and then let it run without one, as the others still ran.", waited.Round(time.Second), set.HeavySlots)
	case waited >= 2*time.Second:
		note = fmt.Sprintf("Kero held this command %s for a build slot (heavy-slots), while other sessions built or ran tests: that wait is not the command's.", waited.Round(time.Second))
	default:
		return nil
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":     "PreToolUse",
		"additionalContext": note,
	}})
}

// freeHeavy gives back the slot of a tool call, as it ends.
func freeHeavy(agent string, in hookInput) {
	key := slotKey(agent, in)
	cmd := heavyCommand(in)
	if key == "" || cmd == "" || !heavy.Heavy(cmd, settings.Load().HeavyCommands) {
		return
	}
	c, err := client.Connect(true)
	if err != nil {
		return
	}
	defer c.Close()
	c.Call("slot.release", proto.SlotParams{Key: key}, nil)
}

// lockCmd runs kero lock heavy [--timeout d] -- <command…>: the command,
// once a slot of heavy-slots is free, holding it while it runs. It is for
// an agent whose builds kero hook doesn't hold, and for scripts. Outside
// Kero, inside another kero lock heavy, or once --timeout runs out, it
// runs the command at once.
func lockCmd(args []string) error {
	if len(args) == 0 || args[0] != "heavy" {
		return usageErr("kero lock heavy [--timeout <duration>] -- <command…>")
	}
	f, err := parse(args[1:], map[string]string{"timeout": "timeout", "t": "timeout"}, nil)
	if err != nil {
		return err
	}
	command := append(f.pos, f.rest...)
	if len(command) == 0 {
		return usageErr("kero lock heavy: which command? kero lock heavy -- go test ./...")
	}
	wait, err := parseTimeout(f.vals["timeout"])
	if err != nil {
		return err
	}
	if os.Getenv(heavyEnv) == "" && settings.Load().HeavySlots > 0 {
		if c, err := client.Connect(true); err == nil {
			// Held while the connection stays: kerod gives it back as
			// this process ends, however it ends.
			defer c.Close()
			var r proto.SlotResult
			p := proto.SlotParams{Pane: os.Getenv("KERO_PANE"), Command: strings.Join(command, " "), Wait: wait, Conn: true}
			if c.Call("slot.take", p, &r) == nil && !r.Off && !r.Got {
				fmt.Fprintf(os.Stderr, "kero: no build slot came free in %s: running without one\n", time.Duration(r.Waited)*time.Millisecond)
			}
		}
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), heavyEnv+"=1")
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return exitStatus(max(1, ee.ExitCode()))
		}
		return err
	}
	return nil
}
