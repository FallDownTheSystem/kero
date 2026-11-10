package main

import (
	"context"
	"errors"

	"github.com/egoist/mygo"

	"kero/internal/proto"

	"kero/locales"
)

// The page bridge: a local page in a browser pane (file:) has
// window.kero.send(message, options), which sends the message to the
// agent that opened the page with kero split --url, labelled with the
// page, through agent.send; or asks the user which agent, for a page no
// agent opened. Its promise resolves once the agent has the message:
// typed into its prompt as it got to it, or taken by kero browser
// wait-message. See docs/panes.md#sending-to-an-agent-from-a-page.

// pageBridge is the Kero service pages call (window.mygo.call("Kero.Send")).
type pageBridge struct{ a *App }

// PageMessage is what a page sends: Text, labelled with Label besides the
// page; Mode as agent.send's ("", "push" or "wait").
type PageMessage struct {
	Text  string `json:"text"`
	Label string `json:"label,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

// PageSent tells the page how its message went: State is delivered,
// queued (for the agent to get at its prompt) or waiting (for an agent's
// wait-message); kero:delivery tells the rest.
type PageSent struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
	To    string `json:"to,omitempty"`
}

// deliveryEvent tells a page that a message it sent reached its agent, or
// never will.
var deliveryEvent = mygo.NewEvent[proto.Delivery]("kero:delivery")

// Send sends a page's message to its agent.
func (b *pageBridge) Send(ctx context.Context, m PageMessage) (PageSent, error) {
	page := mygo.CallerPage(ctx)
	if page == nil || page.WebView() == nil {
		return PageSent{}, errors.New("window.kero.send is for the pages of Kero's browser panes")
	}
	view := page.WebView()
	pane := make(chan string, 1)
	b.a.update(func() {
		for id, e := range b.a.webs {
			if e.view == view {
				pane <- id
				return
			}
		}
		pane <- ""
	})
	id := <-pane
	if id == "" {
		return PageSent{}, errors.New("the page's browser pane closed")
	}
	return b.a.pageSend(ctx, id, m)
}

// pageSend sends a message of the page of browser pane id to its agent:
// the pane's opener's, else the one the user chose for the page before,
// else one the user chooses now.
func (a *App) pageSend(ctx context.Context, id string, m PageMessage) (PageSent, error) {
	type where struct {
		h      *host
		chosen string
	}
	got := make(chan where, 1)
	a.update(func() {
		w := where{h: a.hostOf(id)}
		// The choice stands while the page has the opener it had then:
		// an agent showing the page again takes its messages.
		if e, p := a.webs[id], a.paneByID(id); e != nil && p != nil && e.agentOpener == p.Opener {
			w.chosen = e.agent
		}
		got <- w
	})
	w := <-got
	cl := w.h.cl
	if cl == nil {
		return PageSent{}, errors.New(locales.HostNotConnected(w.h.name))
	}
	// The agent the user chose for the page, when kerod found none: its
	// opener closed, or it never had one.
	p := proto.AgentSendParams{From: id, To: w.chosen, Label: m.Label, Text: m.Text, Mode: m.Mode, User: true}
	res, err := a.sendToAgent(cl, p)
	if err != nil && p.To != "" {
		// The agent chosen before is gone: choose again.
		p.To = ""
		res, err = a.sendToAgent(cl, p)
	}
	if err != nil {
		return PageSent{}, err
	}
	if res.State == "nowhere" {
		chosen := make(chan string, 1)
		mine := make(chan *agentChoice, 1)
		a.update(func() {
			mine <- a.chooseAgent(w.h, locales.AgentChooseTitle(), func(to string) { chosen <- to })
			a.showWindow()
		})
		ch := <-mine
		var to string
		select {
		case to = <-chosen:
		case <-ctx.Done():
			// The page went: its choice goes, not another's opened since.
			a.update(func() {
				if a.pal.choose == ch {
					a.pal.open = false
					a.cancelChoice()
				}
			})
			return PageSent{}, ctx.Err()
		}
		if to == "" {
			return PageSent{}, errors.New("no agent chosen")
		}
		a.update(func() {
			if e, p := a.webs[id], a.paneByID(id); e != nil && p != nil {
				e.agent, e.agentOpener = to, p.Opener
			}
		})
		p.To = to
		if res, err = a.sendToAgent(cl, p); err != nil {
			return PageSent{}, err
		}
	}
	return PageSent{ID: res.ID, State: res.State, To: res.To}, nil
}

// tellPageDelivery tells a browser pane's page how a message it sent went.
func (a *App) tellPageDelivery(d proto.Delivery) bool {
	e := a.webs[d.From]
	if e == nil || e.view == nil {
		return false
	}
	deliveryEvent.EmitPage(e.view.Page(), d)
	return true
}

// bridgeScript defines window.kero in local pages, before their scripts:
// send(message, {label, mode, onState}) resolves with {id, state, to}
// once the agent has the message, and rejects when it never will;
// onState hears "queued" or "waiting" first. A kero:delivery event on
// window tells each message's delivery too. A page tells it is in Kero
// by window.kero, and copies its message where it is not.
const bridgeScript = `(() => {
  if (location.protocol !== "file:" || window.kero || !window.mygo) return;
  // told keeps what kerod told of a message before its call returned,
  // only while calls are in flight, which nothing else would let go of.
  const told = new Map(), waits = new Map();
  let calling = 0;
  window.mygo.on("kero:delivery", (d) => {
    const w = waits.get(d.id);
    if (w) {
      waits.delete(d.id);
      if (d.state === "delivered") w.resolve(d);
      else w.reject(new Error(d.error || "the message was not delivered"));
    } else if (calling > 0) told.set(d.id, d);
    window.dispatchEvent(new CustomEvent("kero:delivery", { detail: d }));
  });
  const send = async (message, options = {}) => {
    const text = typeof message === "string" ? message : JSON.stringify(message, null, 2);
    calling++;
    let r;
    try {
      r = await window.mygo.call("Kero.Send", { text, label: options.label || "", mode: options.mode || "" });
    } finally {
      calling--;
    }
    const early = told.get(r.id);
    told.delete(r.id);
    if (r.state === "delivered") return r;
    if (typeof options.onState === "function") options.onState(r.state, r);
    const done = (d) => ({ ...r, state: "delivered", to: d.to || r.to });
    if (early) {
      if (early.state === "delivered") return done(early);
      throw new Error(early.error || "the message was not delivered");
    }
    return new Promise((resolve, reject) => waits.set(r.id, { resolve: (d) => resolve(done(d)), reject }));
  };
  Object.defineProperty(window, "kero", { value: Object.freeze({ version: 1, send }), enumerable: true });
})();
`
