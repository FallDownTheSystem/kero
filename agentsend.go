package main

import (
	"strings"

	"kero/internal/client"
	"kero/internal/proto"

	"kero/locales"
)

// sendToAgent sends a message to a coding agent the one way kerod has of
// it (agent.send, internal/daemon/deliver.go): typed into the agent's
// prompt once it is at it, labelled with where it comes from. It blocks:
// call it off the main thread. An older kerod, which has no agent.send,
// takes a message to a pane pasted with Enter at once, as before.
func (a *App) sendToAgent(cl *client.Client, p proto.AgentSendParams) (proto.AgentSendResult, error) {
	var res proto.AgentSendResult
	err := cl.Call("agent.send", p, &res)
	if err != nil && strings.Contains(err.Error(), "unknown method") && p.To != "" {
		text := proto.AgentMessage{Label: p.Label, Text: p.Text}.Prompt()
		return proto.AgentSendResult{To: p.To, State: "delivered"}, a.submit(cl, p.To, text)
	}
	return res, err
}

// delivered hears how a message from a pane went (proto.Delivery), once
// it reached its agent or never will.
func (a *App) delivered(h *host, d proto.Delivery) {
	if d.State == "failed" {
		a.toast(locales.AgentSendFailed(d.Error))
	}
}
