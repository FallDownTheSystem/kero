// Package orchestrator is what an orchestrator session starts on: the
// role prompt its agent is given as its first message, and how that goes
// into an agent's command, as the New Session dialog's Task does.
//
// An orchestrator is an ordinary agent session whose agent plans the
// work, starts other sessions for it with kero new, its children, and
// follows them from the reports kerod types into its conversation
// (internal/daemon/orchestrator.go). One level alone: a child's kero new
// starts another child of the same orchestrator.
package orchestrator

import (
	"path/filepath"
	"slices"
	"strings"
)

// DefaultPrompt is the role prompt, when the orchestrator-prompt setting
// is "". {goal} in it is the goal, {agent} the agent command a child
// starts (the default-agent setting). It quotes with ' alone: it goes to
// the agent as an argument of its command, which a double quote may cut
// short where Windows passes it to a .cmd launcher.
const DefaultPrompt = "You are an orchestrator session in Kero, a terminal app for coding agents. " +
	"The user talks to you and rarely looks at the sessions you start.\n\n" +
	"Goal: {goal}\n\n" +
	"How you work:\n" +
	"- Plan the work and delegate it. Do no detailed work yourself: no code changes, no long investigations.\n" +
	"- Give each piece of work to a subagent of your own or to a child session, whichever fits the job: a child is " +
	"a Kero session the user can watch and take over, for work that needs the user or runs long. Start one with " +
	"kero new, which makes it your child, as `kero new -n <name> --worktree <branch> -- {agent} '<task>'`. " +
	"Give each piece a task it can finish alone.\n" +
	"- Tell each child, in its task, to show the user its decisions, and the work it asks the user to accept, " +
	"as a page in its own session (kero split --url), to wait there for the answer the page sends, " +
	"and to send a one-line kero report --to-parent when a decision waits for the user.\n" +
	"- Don't relay a child's decisions here: tell the user only which session waits for them.\n" +
	"- Kero sends your children's reports into this conversation: when one finishes its turn, with the reply it ended it with, " +
	"waits for input or fails, and the summaries they send with kero report --to-parent. " +
	"Read a child's whole last reply with `kero read <id> --reply`, its screen with `kero read <id>`, " +
	"answer it with `kero prompt <id> '<answer>'`, and see them all with `kero status`.\n" +
	"- Answer what you can yourself. Ask the user only what you must.\n" +
	"- Keep your replies to reports short. When a child goes idle or reports and needs nothing from you, " +
	"reply with just 'OK' or 'Acknowledged': no summary, no recap of its work.\n" +
	"- Keep the goal in mind, and tell the user when it is reached, or what stands in its way."

// NoGoal is what the prompt says of the goal of an orchestrator given
// none.
const NoGoal = "none yet: ask the user what to work on."

// Prompt is the role prompt of an orchestrator of goal: template, the
// setting's, else DefaultPrompt, with its {goal} and {agent}. A template
// that has no {goal} gets the goal after it.
func Prompt(template, goal, agent string) string {
	if strings.TrimSpace(template) == "" {
		template = DefaultPrompt
	}
	goal = strings.TrimSpace(goal)
	text := template
	if strings.Contains(text, "{goal}") {
		g := goal
		if g == "" {
			g = NoGoal
		}
		text = strings.ReplaceAll(text, "{goal}", g)
	} else if goal != "" {
		text = strings.TrimRight(text, "\n") + "\n\nGoal: " + goal
	}
	if agent = strings.TrimSpace(agent); agent == "" {
		agent = "claude"
	}
	return strings.ReplaceAll(text, "{agent}", agent)
}

// Placeholder in an agent's command is where its first prompt goes.
const Placeholder = "{prompt}"

// AgentArgs is an agent's command that starts it on a first prompt: in
// place of Placeholder, else after the command, as OpenCode's --prompt
// for it. An empty task leaves the command as it is.
func AgentArgs(command []string, task string) []string {
	task = strings.TrimSpace(task)
	if task == "" || len(command) == 0 {
		return command
	}
	out := slices.Clone(command)
	placed := false
	for i, word := range out {
		if strings.Contains(word, Placeholder) {
			out[i], placed = strings.ReplaceAll(word, Placeholder, task), true
		}
	}
	switch {
	case placed:
	case strings.TrimSuffix(filepath.Base(strings.ReplaceAll(out[0], `\`, "/")), ".exe") == "opencode":
		out = append(out, "--prompt", task)
	default:
		out = append(out, task)
	}
	return out
}
