package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"kero/internal/client"
	"kero/internal/proto"
)

// worktreeCommand runs kero worktree: handoff moves the session it runs
// in, with its agent's conversation, into a new worktree of its own.
func worktreeCommand(c *client.Client, args []string) error {
	if len(args) == 0 || args[0] != "handoff" {
		return usageErr("kero worktree handoff <branch> [--base <ref>] [--prompt <text|->] (see kero help)")
	}
	f, err := parse(args[1:], map[string]string{"base": "base", "prompt": "prompt", "p": "prompt"}, map[string]string{"json": "json"})
	if err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return usageErr("kero worktree handoff <branch>: name the new branch, as fix-login")
	}
	pane := current()
	if pane == "" {
		return usageErr("kero worktree handoff runs in an agent's pane of Kero")
	}
	prompt := f.vals["prompt"]
	if prompt == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		prompt = string(b)
	}
	var res proto.HandoffResult
	if err := c.Call("worktree.handoff", proto.HandoffParams{Pane: pane, Branch: f.pos[0], Base: f.vals["base"], Prompt: strings.TrimSpace(prompt)}, &res); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(res)
	}
	from := ""
	if res.Base != "" {
		from = " from " + res.Base
	}
	fmt.Printf("Made the worktree %s, on the new branch %s%s.\n", res.Path, res.Branch, from)
	fmt.Printf("This session moves there once this turn ends: the agent starts again in %s, resuming this conversation.\n", res.Dir)
	if strings.TrimSpace(prompt) != "" {
		fmt.Println("It then goes on with the prompt given. End your turn now.")
	} else {
		fmt.Println("It then waits for the user. End your turn now.")
	}
	fmt.Println("Changes not committed stay in this checkout.")
	return nil
}
