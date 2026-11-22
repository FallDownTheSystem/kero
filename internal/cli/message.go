package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"kero/internal/client"
	"kero/internal/proto"
	"kero/internal/settings"
)

// messageCmd is kero message: a message to another session's agent, as
// Kero's own go (agent.send), through the agent's own way in (its inbox,
// codex queue, OpenCode's service) once it is at its prompt, and typed
// into its prompt only without one. The agent reads whom it is from, so
// an orchestrator's answer to its child is no word of the user's, as kero
// prompt's would be.
func messageCmd(c *client.Client, st proto.State, args []string) error {
	f, err := parse(args, nil, map[string]string{"json": "json"})
	if err != nil {
		return err
	}
	if len(f.pos) < 1 {
		return usageErr(`kero message <session|pane> <text…|->`)
	}
	t, err := find(st, f.pos[0])
	if err != nil {
		return err
	}
	text := strings.Join(append(f.pos[1:], f.rest...), " ")
	if text == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		text = string(b)
	}
	if strings.TrimSpace(text) == "" {
		return usageErr(`kero message <session|pane> <text…|->`)
	}
	var from *proto.Session
	if ref := current(); ref != "" {
		if ft, err := find(st, ref); err == nil {
			from = &ft.s
		}
	}
	var res proto.AgentSendResult
	if err := c.Call("agent.send", proto.AgentSendParams{To: t.ref(), Label: messageLabel(from, t.s), Text: text, Mode: "push"}, &res); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(res)
	}
	switch res.State {
	case "delivered":
		fmt.Printf("delivered to %s\n", res.To)
	case "queued":
		fmt.Printf("queued for %s: its agent gets it once it is at its prompt\n", res.To)
	default:
		fmt.Println(res.State)
	}
	return nil
}

// messageLabel is whom a message to session to is from, as its agent
// reads it ("Kero: message from …:"): its orchestrator, its child,
// another session, or no session's terminal, for from nil.
func messageLabel(from *proto.Session, to proto.Session) string {
	if from == nil {
		return "a terminal outside Kero's sessions"
	}
	name := fmt.Sprintf("session %s %s", from.ID, proto.Quoted(from.DisplayName(settings.Load().SessionName)))
	switch {
	case from.Ama:
		name = fmt.Sprintf("Ama, Kero's helper agent (session %s)", from.ID)
	case from.Assistant != "":
		name = fmt.Sprintf("the assistant %s (session %s)", proto.Quoted(from.Name), from.ID)
	}
	switch {
	case from.ID == to.ID:
		return "this session (" + name + ")"
	case to.Parent != "" && to.Parent == from.ID:
		return "your orchestrator, " + name
	case from.Parent != "" && from.Parent == to.ID:
		return "your child " + name
	}
	return name
}
