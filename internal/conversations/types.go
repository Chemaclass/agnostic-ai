// Package conversations lists recent chats from the coding tools' own
// stores (Claude Code, Codex and Cursor) and reopens one where it lives:
// the terminal tab that still runs it, a new terminal that resumes it,
// or the editor panel or desktop app it came from.
//
// A list row needs a title, the opening lines of the first prompt and
// the latest prompt. Each is withheld when it may hold a credential, and
// transcripts never leave the server. The JSON contract is mirrored in
// ui/src/contract.ts. Change both together.
package conversations

// SchemaVersion names the shape of View.
const SchemaVersion = "conversations/v1"

// Tool is a coding tool whose conversations the page lists.
type Tool string

const (
	ToolClaude Tool = "claude"
	ToolCodex  Tool = "codex"
	ToolCursor Tool = "cursor"
)

// App is where a conversation runs inside its tool. Runs a program
// started, such as SDK sessions and Codex exec runs, are not listed:
// resuming one replays a finished job.
type App string

const (
	AppCLI        App = "cli"
	AppIDE        App = "ide"
	AppDesktop    App = "desktop"
	AppBackground App = "background"
	// AppCloud is a Claude Code session that runs in Anthropic's cloud,
	// at claude.ai/code, not on this machine.
	AppCloud App = "cloud"
)

// Activity is what a live conversation is doing, when its tool says.
type Activity string

const (
	ActivityWorking Activity = "working"
	ActivityWaiting Activity = "waiting"
	ActivityIdle    Activity = "idle"
)

// RouteKind is how the server reopens a conversation.
type RouteKind string

const (
	// RouteFocus switches to the terminal tab that already runs it.
	RouteFocus RouteKind = "focus"
	// RouteTerminal runs Command in a new terminal window.
	RouteTerminal RouteKind = "terminal"
	// RouteLink opens Link in the app that owns the conversation.
	RouteLink RouteKind = "link"
)

// Withheld field names.
const (
	FieldTitle   = "title"
	FieldOpening = "opening"
	FieldPrompt  = "prompt"
)

// View is one page of the Conversations list.
type View struct {
	SchemaVersion string `json:"schemaVersion"`
	GeneratedAt   string `json:"generatedAt"`
	// From and To bound the page: conversations last written in [From, To),
	// plus, on the first page, every open one.
	From string `json:"from"`
	To   string `json:"to"`
	// Next is the Before time of the following page; empty when no older
	// conversation remains.
	Next string `json:"next,omitempty"`
	// Matches counts every conversation the search matched, across all
	// pages; set only when the scan searched.
	Matches *int `json:"matches,omitempty"`
	// NeedsYou counts the blocked and unread conversations of every age
	// and tool this server lists, whatever the page or the search.
	NeedsYou int `json:"needsYou"`
	// MarksNote says why the marks file could not be read or written;
	// rows then show no marks.
	MarksNote string `json:"marksNote,omitempty"`
	// CloudNote says why Claude Code cloud sessions are missing from the
	// list, on a server that reads them; rows then hold none.
	CloudNote     string         `json:"cloudNote,omitempty"`
	Tools         []Source       `json:"tools"`
	Conversations []Conversation `json:"conversations"`
}

// Source reports one tool's store.
type Source struct {
	Tool  Tool   `json:"tool"`
	Label string `json:"label"`
	// Found is false when the store is missing or unreadable; Note says why.
	Found bool `json:"found"`
	// Count is how many conversations the store holds, after hiding
	// subagent, review and scripted runs; Shown is how many this page holds.
	Count int    `json:"count"`
	Shown int    `json:"shown"`
	Note  string `json:"note,omitempty"`
}

// Route says how to reopen a conversation.
type Route struct {
	Kind  RouteKind `json:"kind"`
	Label string    `json:"label"`
	// Command resumes it in a shell; set for terminal and focus routes.
	Command string `json:"command,omitempty"`
	Link    string `json:"link,omitempty"`
}

// Conversation is one list row.
type Conversation struct {
	Tool  Tool   `json:"tool"`
	App   App    `json:"app"`
	ID    string `json:"id"`
	Title string `json:"title"`
	// Folder is where the conversation started, where it resumes.
	Folder    string `json:"folder,omitempty"`
	Branch    string `json:"branch,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	// UpdatedAt is when the person or the agent last wrote in it.
	UpdatedAt string   `json:"updatedAt"`
	Opening   []string `json:"opening,omitempty"`
	Prompt    string   `json:"prompt,omitempty"`
	Withheld  []string `json:"withheld,omitempty"`
	Live      bool     `json:"live,omitempty"`
	Activity  Activity `json:"activity,omitempty"`
	// State places the conversation for the person; see stateOf.
	State State `json:"state"`
	// SeenAt and CompletedAt are the person's marks, when set.
	SeenAt      string `json:"seenAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	Open        Route  `json:"open"`

	// process is the live process that runs it, for a focus route.
	process *liveProcess
	// editor is set for an editor extension's link: its folder opens
	// there first, so the link reaches the window that holds it.
	editor *editorApp
	// pending holds what fill still needs once the row lands on a page,
	// for a store whose prompts are read only for the rows shown.
	pending *texts
}

// liveProcess is a running process that has a conversation open.
type liveProcess struct {
	PID       int
	ProcStart string
}

// OpenReply is the answer to an open request. Opened is false when the
// server could not open it here; Message says why and Command, when
// set, resumes it by hand.
type OpenReply struct {
	Opened  bool   `json:"opened"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

var toolLabels = map[Tool]string{
	ToolClaude: "Claude Code",
	ToolCodex:  "Codex",
	ToolCursor: "Cursor",
}
