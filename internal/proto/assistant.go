package proto

import "strings"

// Assistant is one of the user's assistants: a saved definition, with a
// role and what it keeps track of, checks Kero wakes it for, and at most
// one live agent session, of no project, in a folder of its own. kerod
// keeps them (assistants.json) and is the only one to change them: the
// app and the CLI go through its methods (assistant.*).
type Assistant struct {
	// ID is a slug of its first name, its folder's name; it never changes.
	ID string `json:"id"`
	// Name is unique, of any case.
	Name string `json:"name"`
	// Color is one of settings.HostColors, a custom color as #rrggbb
	// (lowercase, opaque), or "" for the one its name picks.
	Color string `json:"color,omitempty"`
	// Avatar is "" for its name's initial, a built-in glyph's name, or
	// "file:<name>" of an image in its folder.
	Avatar string `json:"avatar,omitempty"`
	// Group is the sidebar's heading it goes under, "" for none.
	Group string `json:"group,omitempty"`
	// Role is one line, as "Keeps my Jira tickets moving".
	Role string `json:"role"`
	// Tracks is what it does or keeps track of, in lines.
	Tracks string `json:"tracks,omitempty"`
	// Agent is the command it runs, "" for default-agent's, with
	// AgentArgs after it.
	Agent     string           `json:"agent,omitempty"`
	AgentArgs string           `json:"agentArgs,omitempty"`
	Checks    []AssistantCheck `json:"checks,omitempty"`
	// Paused: no check runs.
	Paused  bool  `json:"paused,omitempty"`
	Created int64 `json:"created"`

	// What kerod keeps, which clients only read: its folder, its live
	// session ("" for none), its last report with its text clipped, when
	// its next timed check is (unix ms, 0 for none), and why a check due
	// has not run ("" while all is well).
	Dir        string           `json:"dir,omitempty"`
	Session    string           `json:"session,omitempty"`
	LastReport *AssistantReport `json:"lastReport,omitempty"`
	NextCheck  int64            `json:"nextCheck,omitempty"`
	Problem    string           `json:"problem,omitempty"`
}

// The kinds of an assistant's checks.
const (
	CheckStart  = "start"  // as kerod starts, at most every 4 hours
	CheckDaily  = "daily"  // each day at Time
	CheckWeekly = "weekly" // on Days at Time
)

// AssistantCheck is a check Kero wakes an assistant for, in the
// machine's local time.
type AssistantCheck struct {
	ID   string `json:"id"` // "c1", "c2"…
	Kind string `json:"kind"`
	Time string `json:"time,omitempty"` // "09:00", of daily and weekly
	Days []int  `json:"days,omitempty"` // weekly: time.Weekday values
	// Text is what to check; "" for its usual check.
	Text string `json:"text,omitempty"`
	// Created is when it was added, or its Kind, Time or Days changed;
	// Last when it last ran. Unix ms.
	Created int64 `json:"created"`
	Last    int64 `json:"last,omitempty"`
}

// AssistantReport is a report an assistant gave the user (kero assistants
// report): Waiting, that it starts with "Waiting on you:"; Checks, the
// checks it followed, when known.
type AssistantReport struct {
	At      int64    `json:"at"`
	Text    string   `json:"text"`
	Waiting bool     `json:"waiting,omitempty"`
	Checks  []string `json:"checks,omitempty"`
}

// WaitingPrefix opens a report that waits for the user.
const WaitingPrefix = "Waiting on you:"

// ReportWaits tells whether a report's text waits for the user: it starts
// with "Waiting on you:", after any Markdown's *, # or spaces.
func ReportWaits(text string) bool {
	t := strings.TrimLeft(text, " \t\r\n*#_>")
	return len(t) >= len(WaitingPrefix) && strings.EqualFold(t[:len(WaitingPrefix)], WaitingPrefix)
}

type (
	// AssistantParams name an assistant, by id, a prefix of it or its
	// name; New asks assistant.start for a fresh chat; Check, of
	// assistant.run, its one check to run, "" for all; Folder, of
	// assistant.remove, removes its folder too; Limit, of
	// assistant.reports, how many.
	AssistantParams struct {
		ID     string `json:"id"`
		New    bool   `json:"new,omitempty"`
		Check  string `json:"check,omitempty"`
		Folder bool   `json:"folder,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	// AssistantEdit is an assistant's definition to create
	// (assistant.create) or change (assistant.update), with ID: what is
	// nil stays as it is. Start starts a new one's session; Image is the
	// path of an image to copy into its folder as its avatar.
	AssistantEdit struct {
		ID        string            `json:"id,omitempty"`
		Name      *string           `json:"name,omitempty"`
		Color     *string           `json:"color,omitempty"`
		Avatar    *string           `json:"avatar,omitempty"`
		Image     string            `json:"image,omitempty"`
		Group     *string           `json:"group,omitempty"`
		Role      *string           `json:"role,omitempty"`
		Tracks    *string           `json:"tracks,omitempty"`
		Agent     *string           `json:"agent,omitempty"`
		AgentArgs *string           `json:"agentArgs,omitempty"`
		Checks    *[]AssistantCheck `json:"checks,omitempty"`
		Paused    *bool             `json:"paused,omitempty"`
		Start     bool              `json:"start,omitempty"`
	}
	// AssistantReportParams keep an assistant's report: Assistant's, else
	// that of the session of From, a pane or a session.
	AssistantReportParams struct {
		Assistant string `json:"assistant,omitempty"`
		From      string `json:"from,omitempty"`
		Text      string `json:"text"`
	}
)
