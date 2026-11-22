<!-- Part of the kero skill, which `kero skill setup` writes and Kero keeps current: edits here are lost. -->

# Kero: browser panes and pages

## Use a web page

A browser pane's page is yours to drive, as a user would, to check what
you built: open it with kero split --url, then

    kero browser open localhost:3000/todos     # go to a page, wait for it to load
    kero browser snapshot                      # roles, names and refs: button "Add" [ref=e5]
    kero browser click --ref e5                # click what a snapshot showed
    kero browser type --ref e3 "Buy milk"      # fill a field (--enter submits)
    kero browser click 'button[type=submit]'   # or name an element by CSS selector
    kero browser wait --text "Welcome"         # or wait <selector>; --timeout 30s
    kero browser text [selector]               # the page's text, or an element's
    kero browser html [selector]
    kero browser eval 'document.title'         # run JavaScript, print its value
    kero browser screenshot -o /tmp/page.png   # shown or not, without the focus
    kero browser back|forward|reload
    kero browser cookies save state.json       # or load: Playwright's, or cookies.txt
    kero browser cookies import chrome         # or edge: the user's sign-ins, cookies and site storage
    kero browser pick                          # the user picks elements, with notes, for you
    kero browser console --errors              # what the page threw and logged, file:line
    kero browser network --failed              # 404s, 500s, CORS and blocked requests
    kero browser network body 12               # a response's body, by its #id
    kero browser emulate device iphone-15      # or viewport 390x844 --mobile --touch
    kero browser emulate network slow-3g       # or offline, 3g, 4g; emulate reset undoes

Refs last until the page goes to another; one whose element is gone
fails: take a new snapshot.

When a page misbehaves, read its console and network before guessing:
console shows console.* output, uncaught errors and promises none
caught, and what the browser told of (CORS, failed loads), each with its
file and line; network shows each request with its status, and why one
failed. Both keep what came since the pane's page was made, the first
load too: --clear empties them, so that the next read shows only what a
reload or a click caused. --json prints their fields. emulate lasts while
the pane is open, through reloads and other pages; take a screenshot to
see the layout, and emulate reset when done. network and emulate work on
Windows only; on macOS and Linux, console has what the page's current
document logged.

When a page needs the user signed in, don't ask for their password: ask
them to turn on remote debugging in their browser, at
chrome://inspect/#remote-debugging (edge://inspect/#remote-debugging),
then run kero browser cookies import chrome --domain <site> (or edge)
and have them allow Kero in the browser's dialog, which can show in any
of its windows, an installed app's (PWA) too; the import waits two
minutes for it. It copies their browser's cookies into the panes, and the
site's localStorage and IndexedDB, where single-page apps keep their
sign-in, from a tab of the site (one it opens unseen if none is open);
the panes keep them. Reload the pane after it. Or they can click Import
in Settings › Browser themselves, which takes the storage of their open
tabs' sites.

To have the user show you what to change on a page, rather than describe
it, turn on Pick mode: kero browser pick. The user clicks elements and
writes a note on each (they can also do this from the pane's toolbar
without you asking), and their picks come to you as one message labelled
"element picks", once you are at your prompt, or to kero browser wait-message
if you wait for the page. For each element it gives, strongest first:
Source, the file and line from the framework's development build or the
page's own HTML file, checked against the file, or "Source location:
none" when the page tells none; Search hints, strings likely written in
the source as they are (★ marks one no other element on the page has),
to grep for: text hints are short phrases, as the source may wrap the
text or write ’ and — as entities; where it sits on the page and its HTML;
then the page's selector, which is not the source's, and its styles.
Screenshots are PNG files whose paths the message gives, removed a day
later: one of each
element, boxed, and one of the page with the elements boxed and numbered
as the message numbers them. Open them to see what the user means. Go
to Source first, then grep the hints.

With no target, kero browser drives your session's browser pane; name
one (kero browser <target> text) when there are several. It needs the app
open and Remote Control on, in Settings › Agents.

On Windows, the user can open a DevTools port for the panes (Settings ›
Browser › DevTools port, or browser-devtools = true, then start Kero
again), which tools of the DevTools protocol drive: Playwright's
connectOverCDP, chrome-devtools-mcp (--browser-url), Puppeteer,
Lighthouse. Every pane is a page of one browser on one port; pick yours
by its URL or title:

    kero browser devtools          # the endpoint, then pane, target, URL, title
    kero browser devtools --json   # {endpoint, port, browser, panes: [{pane, target, url, title, webSocketDebuggerUrl}]}

    const browser = await chromium.connectOverCDP("http://127.0.0.1:9222");
    const page = browser.contexts()[0].pages().find((p) => p.url().startsWith("http://localhost:3000"));

Use kero browser for what it does; the port is for what it doesn't, such
as tracing, intercepting requests and a Playwright script. Don't turn the port on
yourself: any program on the computer can then use the panes' sign-ins,
so ask the user. Open pages in panes (kero split --url), not with the
tool's own new tabs.

## Get the user's answers from a page

A local page (file:) you show the user in a browser pane can send you what
they pick, as a decision brief's picks: it has window.kero.send(message,
{label, mode}), which a page tells it is in Kero by, and copies the
message to the clipboard where it isn't:

    <button id="send">Send</button>
    <script>
    send.onclick = async () => {
      const text = "D1: A\nD2: B";
      if (!window.kero) return navigator.clipboard.writeText(text);
      send.textContent = "Sending…";
      await window.kero.send(text, { label: "decision brief",
        onState: (s) => { if (s === "queued") send.textContent = "Queued" } });
      send.textContent = "Sent";
    };
    </script>

The message goes to the agent whose pane opened the page with kero split
--url (or showed it again), labelled with the page: "Kero: message from
decision brief, from the page "Brief" (file:///…/brief.html) in browser
pane 3f2a:", then its text. Kero gives it to you once you are at your
prompt; or, while you wait for it, hands it to kero browser wait-message:

    kero split --url .backlog/pages/brief.html
    kero browser wait-message --json --timeout 9m   # until the page sends: {id, from, label, url, title, text, at}

wait-message prints the message, labelled, or with --json its fields, and
fails with "timed out" when none came in time: wait again, with a timeout
under your shell tool's own. It needs no Remote Control. A page that sends
with mode: "wait" keeps its messages, in order, for wait-message alone,
never typed in; mode: "push" always types them in. The page's promise
resolves once you have the message, and rejects if your pane closes
first; onState hears "queued" (you are working) or "waiting" (kept for
wait-message) before. A page no agent opened asks the user which agent.
