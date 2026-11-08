// Installed by Kero (`kero skill setup`), which replaces it as Kero
// updates; `kero skill remove` removes it. OpenCode's service runs this
// for every terminal: it tells the agent of a session that runs in a Kero
// pane which session and pane it runs in, as the pane's terminal plugin
// (tui.js) told Kero, and tells Kero each end of the session's turns, for
// an orchestrator's reports.
import { execFile } from "node:child_process"

const KERO = __KERO__

// lookup asks Kero what to tell a session's agent; wait gives its pane a
// moment to tell Kero the session, as when the session was just made.
function lookup(sessionID, wait) {
  return new Promise((resolve) => {
    const args = ["hook", "context", "opencode", sessionID, ...(wait ? ["wait"] : [])]
    const child = execFile(KERO, args, { timeout: 5000 }, (err, stdout) => {
      resolve(err ? "" : stdout.trim())
    })
    child.on("error", () => resolve(""))
  })
}

export default {
  id: "kero",
  async setup(ctx) {
    // What each session is told, once Kero knows it; a session it doesn't
    // know is asked about a few times, as its pane may not have told yet.
    const known = new Map()
    await ctx.session.hook("context", async (event) => {
      let entry = known.get(event.sessionID)
      if (!entry) known.set(event.sessionID, (entry = { text: "", tries: 0 }))
      if (!entry.text && entry.tries < 5) {
        entry.tries++
        entry.text = await lookup(event.sessionID, entry.tries === 1)
      }
      if (entry.text) event.system.push({ type: "text", text: entry.text })
    })
    // A turn's end, which Kero takes note of for a session it knows; it
    // runs neither after an interruption nor a failure, which Kero tells
    // from the pane's screen.
    // An OpenCode without the event still runs the rest.
    try {
      await ctx.session.hook("experimental.idle", async (event) => {
        if (!known.get(event.sessionID)?.text) return
        const background = Array.isArray(event.background) ? event.background : []
        const child = execFile(KERO, ["hook", "opencode"], { timeout: 5000 }, () => {})
        child.on("error", () => {})
        child.stdin?.end(
          JSON.stringify({
            hook_event_name: "Stop",
            session_id: event.sessionID,
            background_tasks: background.map((b) => ({ type: typeof b?.type === "string" ? b.type : "background task" })),
          }),
        )
      })
    } catch {}
  },
}
