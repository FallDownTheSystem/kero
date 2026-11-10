package main

import (
	"cmp"
	"slices"
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
	if a.tellPageDelivery(d) {
		return // the page tells it
	}
	if d.State == "failed" {
		a.toast(locales.AgentSendFailed(d.Error))
	}
}

// agentChoice is the palette asking which agent to send something to:
// those of host, then given the pane chosen, "" for none.
type agentChoice struct {
	host  *host
	title string
	then  func(pane string)
}

// chooseAgent asks the user which of a host's agents to send something
// to, in the palette, then calls then with the agent's pane, or "" once
// the user closed it unchosen. It returns the choice, for its asker to
// tell its own from another's.
func (a *App) chooseAgent(h *host, title string, then func(pane string)) *agentChoice {
	a.cancelChoice()
	ch := &agentChoice{host: h, title: title, then: then}
	// The palette, with neither conversations nor commands to load.
	a.pal = paletteState{open: true, reveal: true, gen: a.pal.gen + 1, choose: ch}
	return ch
}

// cancelChoice ends an agent's choice the palette shows, unchosen.
func (a *App) cancelChoice() {
	if ch := a.pal.choose; ch != nil {
		a.pal.choose = nil
		ch.then("")
	}
}

// agentItems are the agents an agentChoice offers: each pane of its host
// an agent runs in, of the session the user looks at first.
func (a *App) agentItems() []paletteItem {
	ch := a.pal.choose
	var items []paletteItem
	for _, g := range a.groups() {
		for _, s := range g.sessions {
			if a.hostOf(s.ID) != ch.host {
				continue
			}
			for _, p := range s.Panes {
				if p.Agent == "" || !p.Terminal() {
					continue
				}
				id, name := p.ID, proto.AgentNames[p.Agent]
				if name == "" {
					name = p.Agent
				}
				it := paletteItem{session: s, title: s.DisplayName(a.set.SessionName), detail: name + " · " + ch.host.shortDir(p.WorkDir()),
					words: p.Agent + " " + name, run: func() { ch.then(id) }}
				if s.ID == a.selected {
					items = slices.Insert(items, 0, it)
				} else {
					items = append(items, it)
				}
			}
		}
	}
	return items
}

// sendExcerpt sends text selected in a terminal pane to an agent the user
// picks (Send to Session…), as a fenced block, labelled with the pane.
func (a *App) sendExcerpt(from, text string) {
	h := a.hostOf(from)
	label := "the terminal pane " + from
	if s := a.sessionOfPane(from); s != nil {
		label += paneSource(s.PaneByID(from)) + " of session " + proto.Quoted(s.DisplayName(a.set.SessionName))
	}
	a.chooseAgent(h, locales.AgentChooseTitle(), func(to string) {
		if to == "" {
			return
		}
		a.sendTo(h, proto.AgentSendParams{To: to, From: from, Label: label, Text: fenced(text), Mode: "push", User: true})
	})
}

// paneSource tells what a terminal pane runs, for the agent to know what
// it reads the output of: " (Claude Code, titled "✳ Fix login")", its
// agent, else its program, else its shell, and the title the program
// set, when it says more; "" when nothing is known.
func paneSource(p *proto.Pane) string {
	if p == nil {
		return ""
	}
	program := cmp.Or(proto.AgentNames[p.Agent], p.Agent, p.Process, p.Shell)
	title := strings.TrimSpace(p.Title)
	switch {
	case program == "" && title == "":
		return ""
	case program == "":
		return " (titled " + proto.Quoted(title) + ")"
	case title == "" || strings.EqualFold(title, program):
		return " (" + program + ")"
	}
	return " (" + program + ", titled " + proto.Quoted(title) + ")"
}

// sendTo sends a message to an agent off the main thread, and tells the
// user how it went.
func (a *App) sendTo(h *host, p proto.AgentSendParams) {
	cl := h.cl
	if cl == nil {
		a.toast(locales.HostNotConnected(h.name))
		return
	}
	name := locales.GitAnAgent()
	if ap := a.paneByID(p.To); ap != nil && proto.AgentNames[ap.Agent] != "" {
		name = proto.AgentNames[ap.Agent]
	}
	go func() {
		res, err := a.sendToAgent(cl, p)
		a.update(func() {
			switch {
			case err != nil:
				a.toast(locales.AgentSendFailed(err.Error()))
			case res.State == "queued":
				a.toast(locales.AgentQueued(name))
			default:
				a.toast(locales.AgentSent(name))
			}
		})
	}()
}

// fenced is text in a Markdown code block, of a fence longer than any run
// of backticks in it.
func fenced(text string) string {
	text = strings.TrimRight(text, "\n")
	n, run := 3, 0
	for _, r := range text {
		if r == '`' {
			run++
			n = max(n, run+1)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", n)
	return fence + "\n" + text + "\n" + fence
}
