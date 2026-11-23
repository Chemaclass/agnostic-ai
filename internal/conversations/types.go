// Package conversations lists recent chats from the coding tools' own
// stores (Claude Code, Codex and Cursor) and the Claude app's chats and
// Cowork tasks, reopens one where it lives:
// the terminal tab that still runs it, a new terminal that resumes it,
// or the editor panel or desktop app it came from, and deletes an
// archived Claude Code session from this machine.
//
// A list row needs a title, the opening lines of the first prompt, the
// latest prompt and the agent's latest reply. Each is withheld when it
// may hold a credential, and transcripts never leave the server. A name
// the person chose in the marks file replaces the tool's title. A first
// prompt that starts with a link line names the conversation it
// continues (see link.go). The JSON contract is mirrored in
// ui/src/contract.ts. Change both together.
package conversations

import "time"

// SchemaVersion names the shape of View.
const SchemaVersion = "conversations/v1"

// Tool is a coding tool whose conversations the page lists.
type Tool string

const (
	ToolClaude Tool = "claude"
	ToolCodex  Tool = "codex"
	ToolCursor Tool = "cursor"
	// ToolChatGPT is ChatGPT's plain chats, read from the ChatGPT desktop app.
	ToolChatGPT Tool = "chatgpt"
)

// App is where a conversation runs inside its tool. Runs a program
// started, such as SDK sessions and Codex exec runs, are not listed:
// resuming one replays a finished job. A Codex exec run a Claude Code
// session started shows among that session's helpers instead while it runs.
type App string

const (
	AppCLI        App = "cli"
	AppIDE        App = "ide"
	AppDesktop    App = "desktop"
	AppBackground App = "background"
	// AppCloud is a conversation that runs in its tool's cloud, not on
	// this machine: a Claude Code session at claude.ai/code, or a Codex
	// cloud thread.
	AppCloud App = "cloud"
	// AppChat is a plain chat with an assistant, outside any coding
	// tool: a claude.ai chat the Claude app keeps, or a ChatGPT chat.
	AppChat App = "chat"
	// AppCowork is a Claude Cowork task the Claude app keeps.
	AppCowork App = "cowork"
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
	// RouteTerminal runs Command in a new tab of the app it last ran in,
	// or a new Terminal window.
	RouteTerminal RouteKind = "terminal"
	// RouteLink opens Link in the app that owns the conversation.
	RouteLink RouteKind = "link"
)

// Withheld field names.
const (
	FieldTitle   = "title"
	FieldOpening = "opening"
	FieldPrompt  = "prompt"
	FieldReply   = "reply"
)

// ApprovalsNote is what the approvals setting adds to a row.
type ApprovalsNote struct {
	// Approved counts the requests the approvals hook approved for this
	// session, as far back as its decision log reaches.
	Approved int `json:"approved,omitempty"`
	// Recent are the latest of those requests, newest first.
	Recent []ApprovedCall `json:"recent,omitempty"`
	// Reason says why a permission wait reached the person: the
	// always-ask rule that matched, or why approvals did not answer. It
	// comes from agnostic-ai, never from text the session wrote.
	Reason string `json:"reason,omitempty"`
	// Pending are the requests the approvals hook holds for an answer
	// from the companion, oldest first.
	Pending []PendingRequest `json:"pending,omitempty"`
}

// PendingRequest is one request the companion can approve or deny.
// Input is the tool input as the hook read it, never the session's text.
type PendingRequest struct {
	ID      string       `json:"id"`
	Digest  string       `json:"digest"`
	Call    string       `json:"call"`
	Input   PendingInput `json:"input"`
	Expires string       `json:"expires"`
	// Cwd is the folder the call runs in, where a relative path starts.
	Cwd string `json:"cwd,omitempty"`
}

// PendingInput is what a pending request runs or writes: the command,
// the file with the text an edit replaces and writes, or a Codex patch.
type PendingInput struct {
	Command  string `json:"command,omitempty"`
	FilePath string `json:"file_path,omitempty"`
	Old      string `json:"old,omitempty"`
	New      string `json:"new,omitempty"`
	Patch    string `json:"patch,omitempty"`
	// Background and Timeout are how a command runs: in the background,
	// and the time limit in milliseconds Claude Code gives it.
	Background bool  `json:"background,omitempty"`
	Timeout    int64 `json:"timeout,omitempty"`
}

// ApprovedCall is one request the approvals hook approved.
type ApprovedCall struct {
	Time string `json:"time"`
	Call string `json:"call"`
	// Detail is what the call acted on, without anything that may hold
	// a credential; empty when it was left out.
	Detail string `json:"detail,omitempty"`
}

// View is one page of the Conversations list.
type View struct {
	SchemaVersion string `json:"schemaVersion"`
	GeneratedAt   string `json:"generatedAt"`
	// From and To bound the page: conversations last written in [From, To),
	// plus, on the first page, every open, blocked or unread one.
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
	// PermissionWaits holds live permission waits across all pages and
	// filters, so the server can adjust NeedsYou and track ongoing waits.
	PermissionWaits []Conversation `json:"-"`
	// MarksNote says why the marks file could not be read or written;
	// rows then show no marks.
	MarksNote string `json:"marksNote,omitempty"`
	// CloudNote says why a tool's cloud conversations are missing from
	// the list, on a server that reads them; rows then hold none of them.
	CloudNote      string `json:"cloudNote,omitempty"`
	MailboxWarning string `json:"mailboxWarning,omitempty"`
	// MailboxClaims sums the file claims of every session, listed or not;
	// nil when no claim and no conflict exists.
	MailboxClaims *MailboxClaims `json:"mailboxClaims,omitempty"`
	// ApprovalsMode is the loosest active approvals mode for the list.
	// It is empty when no approvals route is active.
	ApprovalsMode string         `json:"approvalsMode,omitempty"`
	Tools         []Source       `json:"tools"`
	Conversations []Conversation `json:"conversations"`
	// Pinned is ScanOptions.NeedsYou: rows that need the person whatever
	// their state.
	Pinned map[Ref]bool `json:"-"`
}

// NeedsYouAnyway reports whether row counts in NeedsYou for a reason
// its state does not hold, so a change of its state leaves the count.
func (v View) NeedsYouAnyway(row Conversation) bool {
	return v.Pinned[Ref{Tool: row.Tool, ID: row.ID}]
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
	// Needs names the command the store cannot be read without, such as
	// sqlite3, which Windows does not ship.
	Needs string `json:"needs,omitempty"`
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
	// Reply is the agent's latest message since the latest prompt, as one
	// line: the question it asks, or the start of the message.
	Reply    string   `json:"reply,omitempty"`
	Withheld []string `json:"withheld,omitempty"`
	Live     bool     `json:"live,omitempty"`
	Activity Activity `json:"activity,omitempty"`
	// WorkingSubagents counts native tasks with a live owner and an unfinished
	// lifecycle. Nil means unavailable; zero means no working subagents.
	WorkingSubagents *int `json:"workingSubagents,omitempty"`
	// Helpers are the agents this conversation started that are no rows
	// of their own and still work: its native subagents, and the Codex runs
	// a Claude Code session started with `codex exec`. Nil when none works
	// or their states are unknown; see helpers.go.
	Helpers     []helper     `json:"helpers,omitempty"`
	StuckShells []StuckShell `json:"stuckShells,omitempty"`
	// Stoppable is the process of a live Claude Code session in a
	// terminal that this server can end; see StopSession. A stop names
	// it, so a chat that moved to another process is not stopped.
	Stoppable *ProcessRef `json:"stoppable,omitempty"`
	// Waiting says what a waiting conversation waits for, when its tool
	// says: an approval, with the call when the transcript shows it, or
	// an answer. Nil when the kind is unknown.
	Waiting *waitingOn `json:"waiting,omitempty"`
	// QuietSince is set when the wait was read from silence, not reported
	// by the tool: the time of the last write, since when nothing came.
	QuietSince string `json:"quietSince,omitempty"`
	// State places the conversation for the person; see stateOf.
	State State `json:"state"`
	// Unread: an active conversation whose latest write the person has
	// not seen: a terminal session resumed by hand after it closed with a
	// reply, until the person reads it. It counts not toward NeedsYou.
	// An archived row is never unread.
	Unread bool `json:"unread,omitempty"`
	// SeenAt and CompletedAt are the person's marks, when set.
	SeenAt      string `json:"seenAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	// DeleteUnsupported says why an archived conversation cannot be
	// deleted from here; set only on completed and dormant rows.
	DeleteUnsupported string `json:"deleteUnsupported,omitempty"`
	// Parent is the conversation this one continues, read from the link
	// line of its own first prompt. The parent may be on no page.
	Parent *Ref `json:"parent,omitempty"`
	// Cloud is the claude.ai session that shows this live local one too,
	// with Remote Control on. Its cloud row is not listed apart.
	Cloud *CloudRef `json:"cloud,omitempty"`
	// CloudResumeCommand resumes this cloud conversation in a terminal.
	CloudResumeCommand string `json:"cloudResumeCommand,omitempty"`
	// Approvals is what the approvals hook decided for this session; nil
	// when it decided nothing and has nothing to say about a wait.
	Approvals  *ApprovalsNote  `json:"approvals,omitempty"`
	Open       Route           `json:"open"`
	Threads    []ThreadSummary `json:"threads,omitempty"`
	ClaimCount int             `json:"claimCount,omitempty"`
	// ClaimedPaths counts the paths across the session's file claims.
	ClaimedPaths int `json:"claimedPaths,omitempty"`
	// ClaimConflicts are the active conflicts this session holds or
	// requests, with or without a thread.
	ClaimConflicts []MailboxConflict `json:"claimConflicts,omitempty"`

	// process is the live process that holds it, for a focus route.
	process *liveProcess
	// placed is where that session shows, when its terminal was found.
	placed *placement
	// lastTab is the tab an ended session, or a Claude Code background
	// session, last ran in, when remembered; its app gets the new tab.
	lastTab *terminalHost
	// editor is set for an editor extension's link: its folder opens
	// there first, so the link reaches the window that holds it.
	editor *editorApp
	// store is a cursor-agent chat's store.db, which its running process
	// holds open.
	store string
	// recency is a Cursor editor chat's last activity in milliseconds.
	// UpdatedAt keeps only the second, so two writes within one second
	// would share a cache key.
	recency int64
	// file is a Claude Code session's transcript, as the scan found it.
	file string
	// renamed is the name the person chose, shown over the tool's title.
	renamed string
	// untyped is a thread opened and never written in: no title, prompt
	// or reply. The list leaves it out unless the person named it.
	untyped bool
	// liveRead: the scan asked whether a process holds this row and got
	// an answer, so a row that is not Live is closed, not unknown.
	liveRead bool
	// liveUnknown: the read of the processes that may hold this row
	// failed or could not see it, so it may still run.
	liveUnknown bool
	// readAt is when the read that gave this row began. A kept result
	// or a cloud list can be older than the scan that shows it.
	readAt time.Time
	// pending holds what fill still needs once the row lands on a page,
	// for a store whose prompts are read only for the rows shown.
	pending *texts
	// bridge is the Remote Control id a live Claude Code session wrote.
	bridge string
	// heldBy is the id of the local row that took this cloud row in.
	heldBy string
	// subagent names a Codex subagent's thread, which opens only through
	// the row that lists it as a helper; empty for any other row.
	subagent string
	// appArchived: the conversation's own app keeps it archived, as the
	// app's flag says on this read.
	appArchived bool
	// appArchivedAt is when the app archived it, for an app that keeps
	// that time; zero otherwise.
	appArchivedAt time.Time
	// underAppArchive: archived in its app and, since then, nothing was
	// written and its tool never said it was working. See appArchives.
	underAppArchive bool
	shellSession    *liveProcess
	shellOnly       bool
	mainWorking     bool
	// pendingReply is a reply whose credential check waits for a page,
	// for a store that lists thousands of rows with their replies.
	pendingReply *texts
}

type StuckShell struct {
	PID          int    `json:"pid"`
	ProcStart    string `json:"procStart"`
	Command      string `json:"command"`
	Since        string `json:"since"`
	HelperID     string `json:"helperId,omitempty"`
	HelperName   string `json:"helperName,omitempty"`
	ProcessCount int    `json:"processCount"`
}

type ThreadSummary struct {
	ID        string            `json:"id"`
	With      Ref               `json:"with"`
	WithTitle string            `json:"withTitle,omitempty"`
	Status    string            `json:"status"`
	Count     int               `json:"count"`
	Cap       int               `json:"cap"`
	Waiting   int               `json:"waiting"`
	Held      int               `json:"held,omitempty"`
	LastType  string            `json:"lastType,omitempty"`
	Conflicts []MailboxConflict `json:"conflicts,omitempty"`
}

// MailboxClaims counts claimed paths, the claims whose owner is gone,
// and the active conflicts.
type MailboxClaims struct {
	Paths     int `json:"paths"`
	Stale     int `json:"stale"`
	Conflicts int `json:"conflicts"`
}

type MailboxConflict struct {
	ID        string `json:"id"`
	Worktree  string `json:"worktree"`
	Path      string `json:"path"`
	Requester Ref    `json:"requester"`
	Holder    *Ref   `json:"holder,omitempty"`
	Sender    *Ref   `json:"sender,omitempty"`
	Thread    string `json:"thread,omitempty"`
	N         int    `json:"n,omitempty"`
}

// Ref names one conversation of any tool.
type Ref struct {
	Tool Tool   `json:"tool"`
	ID   string `json:"id"`
}

// CloudRef is a cloud session a local row also stands for, and its page.
type CloudRef struct {
	ID   string `json:"id"`
	Link string `json:"link"`
	// row is the cloud row the local one took in, which opens on claude.ai.
	row *Conversation
	// liveWritten, on a closed local row, is the cloud row's last write
	// read while the session ran; zero on a live one.
	liveWritten time.Time
}

// liveProcess is a running process that has a conversation open.
type liveProcess struct {
	PID       int
	ProcStart string
}

// OpenReply is the answer to an open request. Opened is false when the
// server could not open it here; Message says why and Command, when
// set, resumes the session by hand. A session that still runs in a
// terminal no tab can show gets one with a note to end that process
// first, since a second copy would compete with it. One whose tab was
// found, and a Codex thread its background service keeps, get none.
// A Claude Code background session gets claude attach, which starts no copy.
type OpenReply struct {
	Opened  bool   `json:"opened"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
	// Guessed says the only possible Codex tab came forward.
	Guessed bool `json:"guessed,omitempty"`
	// Helper is the Claude Code subagent the open was asked for, when
	// one was named; the chat it runs in is what opened.
	Helper *helper `json:"helper,omitempty"`
	// Reached says what came to the front when a helper was named:
	// reachedSubagent for its own view, reachedParent for the chat it
	// runs in, where Message says how to reach it.
	Reached string `json:"reached,omitempty"`
}

const (
	reachedSubagent = "subagent"
	reachedParent   = "parent"
)

var toolLabels = map[Tool]string{
	ToolClaude:  "Claude Code",
	ToolCodex:   "Codex",
	ToolCursor:  "Cursor",
	ToolChatGPT: "ChatGPT",
}
