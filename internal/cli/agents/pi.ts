// Installed by Kero (`kero skill setup`), which replaces it as Kero
// updates; `kero skill remove` removes it. Inside Kero, it tells Pi or
// OMP which session and pane it runs in, and tells Kero their session,
// to resume it after Kero's background service restarts, and each end of
// their turns, with the reply, for an orchestrator's reports. Outside
// Kero it does nothing.
import { execFile } from "node:child_process";
import { existsSync } from "node:fs";

const KERO = __KERO__;
const AGENT = __AGENT__;

// hook runs `kero hook <agent>` as Claude Code runs its hooks, returning
// what Kero tells the agent.
function hook(sessionID: string | undefined): Promise<string> {
  return new Promise((resolve) => {
    const child = execFile(KERO, ["hook", AGENT], { timeout: 5000 }, (err, stdout) => {
      try {
        resolve(JSON.parse(stdout).hookSpecificOutput.additionalContext ?? "");
      } catch {
        resolve("");
      }
    });
    child.on("error", () => resolve(""));
    child.stdin?.end(JSON.stringify({ hook_event_name: "SessionStart", session_id: sessionID ?? "" }));
  });
}

// tell runs `kero hook <agent>` with an event Kero only takes note of.
function tell(event: object) {
  const child = execFile(KERO, ["hook", AGENT], { timeout: 5000 }, () => {});
  child.on("error", () => {});
  child.stdin?.end(JSON.stringify(event));
}

// replyOf is the text of the last assistant message of a run's messages,
// null when it has none.
function replyOf(messages: any): string | null {
  if (!Array.isArray(messages)) return null;
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m?.role !== "assistant") continue;
    if (typeof m.content === "string") return m.content;
    if (!Array.isArray(m.content)) return null;
    return m.content
      .filter((part: any) => part?.type === "text" && typeof part.text === "string")
      .map((part: any) => part.text)
      .join("\n");
  }
  return null;
}

function sessionOf(ctx: any): string {
  try {
    return ctx.sessionManager.getSessionId() ?? "";
  } catch {
    return "";
  }
}

export default function (pi: any) {
  if (!process.env.KERO_PANE) return;
  let context = "";
  let reported: string | undefined;
  const sync = async (ctx: any) => {
    // A terminal session, or a one-off prompt; not one a program drives.
    if (ctx?.mode !== "tui" && ctx?.mode !== "print") return;
    let sessionID: string | undefined;
    try {
      // OMP allocates a session id at start but writes the transcript only
      // with the first message. Kero would resume an id that has no file, so
      // report one only once it exists; the turn events report it as soon as
      // the first turn writes it.
      const file = ctx.sessionManager.getSessionFile();
      if (ctx.mode === "tui" && file && existsSync(file)) {
        sessionID = ctx.sessionManager.getSessionId();
      }
    } catch {}
    // Nothing new to tell, and a context already fetched: keep it.
    if (sessionID === undefined && context) return;
    // An in-app /resume switches the conversation under the same pane:
    // tell Kero the new id, or it would resume the one it saw last.
    if (sessionID !== undefined) {
      if (sessionID === reported) return;
      reported = sessionID;
    }
    context = await hook(sessionID);
  };
  pi.on("session_start", (_event: any, ctx: any) => sync(ctx));
  pi.on("session_switch", (_event: any, ctx: any) => sync(ctx));
  // The turn's end: Pi's agent_settled, once no retry or follow-up is
  // left, with the reply of the run before it; OMP's last agent_end, which
  // no continuation follows.
  let reply: string | null = null;
  pi.on("agent_end", (event: any, ctx: any) => {
    sync(ctx);
    reply = replyOf(event?.messages);
    if (AGENT !== "omp" || event?.willContinue) return;
    tell({
      hook_event_name: "Stop",
      session_id: sessionOf(ctx),
      cwd: process.cwd(),
      last_assistant_message: reply,
      ...(event?.awaitingAsyncWork ? { background_tasks: [{ type: "background task" }] } : {}),
    });
  });
  pi.on("agent_settled", (event: any, ctx: any) => {
    if (event?.aborted) {
      tell({ hook_event_name: "Interrupt", session_id: sessionOf(ctx), cwd: process.cwd() });
    } else {
      tell({ hook_event_name: "Stop", session_id: sessionOf(ctx), cwd: process.cwd(), last_assistant_message: reply });
    }
    reply = null;
  });
  pi.on("turn_end", (_event: any, ctx: any) => sync(ctx));
  pi.on("before_agent_start", (event: any) => {
    if (!context) return;
    // Pi keeps prompt sections; OMP has none and replaces the prompt array.
    const sections = event?.systemPromptOptions?.sections;
    if (sections) {
      sections.kero = context;
      return;
    }
    if (Array.isArray(event?.systemPrompt)) {
      return { systemPrompt: [...event.systemPrompt, context] };
    }
  });
}
