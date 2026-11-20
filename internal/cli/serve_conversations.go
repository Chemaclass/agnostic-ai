package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/markdown"
)

const (
	conversationsViewPath  = "/api/views/conversations"
	conversationOpenPath   = "/api/conversations/open"
	conversationMarkPath   = "/api/conversations/mark"
	conversationRenamePath = "/api/conversations/rename"
	conversationDeletePath = "/api/conversations/delete"
	conversationSendPath   = "/api/conversations/send"
)

// conversationPaths are the endpoints conversationsHandler serves. The
// mux mounts each one and serializeAPI leaves each out of its lock.
var conversationPaths = []string{conversationsViewPath, conversationOpenPath, conversationMarkPath, conversationRenamePath, conversationDeletePath, conversationSendPath}

// maxConversationOpenBytes bounds an open request: a tool, an id, an editor and a helper.
const maxConversationOpenBytes = 4 << 10

// conversationRef names one conversation in a request body.
type conversationRef struct {
	Tool conversations.Tool `json:"tool"`
	ID   string             `json:"id"`
}

func (ref conversationRef) tool() conversations.Tool { return ref.Tool }

type conversationOpenRequest struct {
	conversationRef
	Editor string `json:"editor"`
	// Helper names a Claude Code subagent of the conversation: the open
	// reaches the chat it runs in and the reply names it.
	Helper string `json:"helper"`
}

// maxConversationSendBytes is the API body limit bufferAPIBody sets: JSON
// escaping can grow the text past MaxSendBytes, which is checked decoded.
const maxConversationSendBytes = 1 << 20

type conversationSendRequest struct {
	conversationRef
	Text string `json:"text"`
	HTML string `json:"html"`
}

type conversationMarkRequest struct {
	conversationRef
	// As is seen, completed, open (which takes a completion back) or
	// hidden (out of every list until something new is written or the
	// conversation starts working).
	As conversations.Mark `json:"as"`
}

type conversationRenameRequest struct {
	conversationRef
	Title string `json:"title"`
}

type conversationDeleteRequest struct {
	conversationRef
}

// conversationDeleteRefusal is a refused delete: error is a complete
// sentence on its own; removed and remaining are set when a removal
// failed part of the way.
type conversationDeleteRefusal struct {
	Error     string                      `json:"error"`
	Reason    conversations.DeleteRefusal `json:"reason,omitempty"`
	Removed   []string                    `json:"removed,omitempty"`
	Remaining []string                    `json:"remaining,omitempty"`
}

var deleteRefusalStatus = map[conversations.DeleteRefusal]int{
	conversations.DeleteNotListed:   http.StatusNotFound,
	conversations.DeleteNotArchived: http.StatusConflict,
	conversations.DeleteRunning:     http.StatusConflict,
	conversations.DeleteChanged:     http.StatusConflict,
	conversations.DeleteUnsupported: http.StatusUnprocessableEntity,
	conversations.DeleteFailed:      http.StatusInternalServerError,
}

// conversationsHandler lists the coding tools' conversations and
// reopens one where it lives. A project server lists the conversations
// started in the project or one of its Git worktrees; a global server
// lists them all.
type conversationsHandler struct {
	root     string
	global   bool
	scanner  *conversations.Scanner
	launcher conversations.Launcher
	// worktrees lists a project's Git worktrees; nil skips them.
	worktrees func(root string) []string
	// renameInTool writes a new name into the tool that keeps the
	// conversation.
	renameInTool func(ctx context.Context, c conversations.Conversation, title string) error
	// logOut takes the lines serve logs, such as a name a tool did not take.
	logOut io.Writer
	// openReplyWait is how long an open or a send may take to answer;
	// tests shorten it.
	openReplyWait time.Duration
	// approvals adds what the approvals hook decided to the list; nil
	// leaves the list as the tools report it.
	approvals *approvalsReader
}

// conversationOpenReplyTimeout outlasts the companion's openTimeout and
// sendTimeout in companion/server.go, so an open or a send slower than
// the server's write timeout still answers.
const conversationOpenReplyTimeout = 90 * time.Second

func newConversationsHandler(root string, global bool) *conversationsHandler {
	scanner := conversations.NewScanner()
	return &conversationsHandler{
		root:          root,
		global:        global,
		scanner:       scanner,
		launcher:      conversations.SystemLauncher(),
		worktrees:     gitWorktrees,
		renameInTool:  conversations.NewRenamer(scanner.Stores, scanner.Processes).Rename,
		logOut:        os.Stderr,
		openReplyWait: conversationOpenReplyTimeout,
		approvals:     newApprovalsReader(),
	}
}

func (h *conversationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case conversationsViewPath:
		h.view(w, r)
	case conversationOpenPath:
		h.open(w, r)
	case conversationMarkPath:
		h.mark(w, r)
	case conversationRenamePath:
		h.rename(w, r)
	case conversationDeletePath:
		h.delete(w, r)
	case conversationSendPath:
		h.send(w, r)
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "no endpoint at " + r.URL.Path})
	}
}

func (h *conversationsHandler) view(w http.ResponseWriter, r *http.Request) {
	if !allowGetOnly(w, r) {
		return
	}
	query := r.URL.Query()
	var before time.Time
	if text := query.Get("before"); text != "" {
		at, err := time.Parse(time.RFC3339, text)
		if err != nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "before must be a time such as 2026-11-01T12:00:00Z, the next value of the previous page"})
			return
		}
		before = at
	}
	search := query.Get("q")
	if len([]rune(search)) > maxConversationQuery {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "the search is longer than 200 characters"})
		return
	}
	tool := conversations.Tool(query.Get("tool"))
	if tool != "" && !checkConversationTool(w, tool) {
		return
	}
	// The tag is taken before the scan. A scan that itself writes the
	// marks then changes the stamp, and the caller's next question gets
	// one more scan instead of a list that missed a write made meanwhile.
	stamp := h.scanner.TakeStamp()
	tagged := stamp.Tag
	if h.approvals != nil {
		tagged += "\x00" + h.approvals.stamp()
	}
	tag := listTag(tagged, r)
	w.Header().Set("ETag", tag)
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	// The worktree lookup starts git, so a 304 skips it.
	opts := h.scanOptions(query.Get("editor"))
	opts.Before, opts.Query, opts.Tool = before, search, tool
	view := h.scanner.ScanStamped(r.Context(), opts, stamp)
	if h.approvals != nil {
		h.approvals.decorate(&view)
	}
	writeEditJSON(w, http.StatusOK, view)
}

// listTag names what a list request would answer: it changes when a
// scan could answer differently (conversations.Scanner.Stamp) or the
// request asks for something else. A caller that polls sends it back as
// If-None-Match and gets 304 without a scan while it holds.
func listTag(stamp string, r *http.Request) string {
	sum := sha256.Sum256([]byte(stamp + "\x00" + r.URL.RawQuery))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// maxConversationQuery bounds a search, which every row is matched against.
const maxConversationQuery = 200

func knownConversationTool(tool conversations.Tool) bool {
	return tool == conversations.ToolClaude || tool == conversations.ToolCodex || tool == conversations.ToolCursor || tool == conversations.ToolChatGPT
}

// checkConversationTool answers 400 for a tool the conversations list
// does not know.
func checkConversationTool(w http.ResponseWriter, tool conversations.Tool) bool {
	if knownConversationTool(tool) {
		return true
	}
	writeEditJSON(w, http.StatusBadRequest, editError{Error: "tool must be claude, codex, cursor or chatgpt"})
	return false
}

// readConversationRequest decodes one conversation request of at most
// maxConversationOpenBytes into req, answering 400 with usage for a body
// it cannot read.
func readConversationRequest(w http.ResponseWriter, r *http.Request, usage string, req interface{ tool() conversations.Tool }) bool {
	return conversationRequestOK(w, req, decodeOneJSON(w, r, maxConversationOpenBytes, req), usage)
}

// conversationRequestOK answers 400 and is false when a decoded request
// failed or names an unknown tool.
func conversationRequestOK(w http.ResponseWriter, req interface{ tool() conversations.Tool }, err error, usage string) bool {
	switch {
	case errors.Is(err, errExtraJSONValue):
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send one JSON object"})
		return false
	case err != nil:
		writeEditJSON(w, http.StatusBadRequest, editError{Error: usage})
		return false
	}
	return checkConversationTool(w, req.tool())
}

// writeConversationGone answers 404 for a conversation the server no
// longer lists.
func writeConversationGone(w http.ResponseWriter) {
	writeEditJSON(w, http.StatusNotFound, editError{Error: "That conversation is not in this list any more. Refresh the page."})
}

func (h *conversationsHandler) open(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	deadline := time.Now().Add(h.openReplyWait)
	_ = http.NewResponseController(w).SetWriteDeadline(deadline)
	var req conversationOpenRequest
	if !readConversationRequest(w, r, "send {tool, id, editor, helper}", &req) {
		return
	}
	if req.Helper != "" && !conversations.ValidAgentID(req.Helper) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "helper must be an agent id"})
		return
	}
	// The route comes from the server's own scan; the request only names
	// which conversation. A caller that stops waiting does not stop the
	// open: a terminal script cut short may leave a tab half opened, and
	// the caller cannot tell whether it opened.
	reply, found := h.scanner.ReopenHelper(context.WithoutCancel(r.Context()), req.Tool, req.ID, req.Helper, h.scanOptions(req.Editor), h.launcher)
	if r.Context().Err() != nil || !time.Now().Before(deadline) {
		h.logLateOpen(req.Tool, reply, found)
	}
	if !found {
		writeConversationGone(w)
		return
	}
	writeEditJSON(w, http.StatusOK, reply)
}

// logLateOpen says how an open ended whose reply no longer reaches its
// caller. The reason stays out: it may quote a system error.
func (h *conversationsHandler) logLateOpen(tool conversations.Tool, reply conversations.OpenReply, found bool) {
	outcome := "opened"
	switch {
	case !found:
		outcome = "not in the list any more"
	case !reply.Opened:
		outcome = "not opened"
	}
	_, _ = fmt.Fprintf(h.logOut, "an open of a %s conversation answered after its caller left: %s\n", tool, outcome)
}

// send types text into a conversation's input without submitting it.
func (h *conversationsHandler) send(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(h.openReplyWait))
	tooLarge := func(size string) {
		writeEditJSON(w, http.StatusRequestEntityTooLarge, editError{Error: fmt.Sprintf("The text is %s the %d KB a send takes, so nothing was typed.", size, conversations.MaxSendBytes>>10)})
	}
	var req conversationSendRequest
	err := decodeOneJSON(w, r, maxConversationSendBytes, &req)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		tooLarge("over")
		return
	}
	if !conversationRequestOK(w, req, err, "send {tool, id, text, html?}") {
		return
	}
	if len(req.Text) > conversations.MaxSendBytes {
		tooLarge(fmt.Sprintf("%d KB, over", (len(req.Text)+1023)>>10))
		return
	}
	// The conversation comes from the server's own scan; the request only
	// names which one.
	conversation, ok := h.scanner.Find(r.Context(), req.Tool, req.ID, h.scanOptions(""))
	if !ok {
		writeConversationGone(w)
		return
	}
	text, asMarkdown := req.sendable()
	send := conversations.Send
	if asMarkdown {
		send = conversations.SendMarkdown
	}
	writeEditJSON(w, http.StatusOK, send(r.Context(), conversation, text, h.scanner.Processes, h.launcher))
}

// sendable is what a send types: the copied HTML as Markdown when that
// adds something to the plain text, else the text. Markdown too long to
// send falls back to the text.
func (req conversationSendRequest) sendable() (string, bool) {
	if converted := markdown.FromHTML(req.HTML, req.Text); converted != "" && len(converted) <= conversations.MaxSendBytes {
		return converted, true
	}
	return req.Text, false
}

// mark records that the person saw a conversation, completed it, took
// a completion back, or hid it.
func (h *conversationsHandler) mark(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var req conversationMarkRequest
	if !readConversationRequest(w, r, "send {tool, id, as}", &req) {
		return
	}
	if !conversations.KnownMark(req.As) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "as must be seen, completed, open or hidden"})
		return
	}
	// Only a conversation the server lists gets a mark, so the file
	// never holds an id a request made up.
	if _, ok := h.scanner.Find(r.Context(), req.Tool, req.ID, h.scanOptions("")); !ok {
		writeConversationGone(w)
		return
	}
	if err := h.scanner.Mark(req.Tool, req.ID, req.As); err != nil {
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rename writes the name the person chose into the conversation's tool
// and then saves it in the marks file, which every server sharing it
// then shows. A name the tool does not take is not saved.
func (h *conversationsHandler) rename(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var req conversationRenameRequest
	if !readConversationRequest(w, r, "send {tool, id, title}", &req) {
		return
	}
	title, err := conversations.CleanTitle(req.Title)
	if err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
		return
	}
	// The lock spans the tool's write and the marks' write, so a second
	// rename of the conversation cannot put its name in one and leave
	// this one's in the other.
	release, err := h.scanner.LockRename(req.Tool, req.ID)
	if errors.Is(err, conversations.ErrRenameInProgress) {
		writeEditJSON(w, http.StatusConflict, editError{Error: "This conversation is being renamed already. Wait for that to finish, then try again."})
		return
	}
	if err != nil {
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
		return
	}
	defer release()
	conversation, ok := h.scanner.Find(r.Context(), req.Tool, req.ID, h.scanOptions(""))
	if !ok {
		writeConversationGone(w)
		return
	}
	// The tool takes the name first, so the list never shows a name the
	// tool behind it does not have. It may take seconds.
	if err := h.renameInTool(r.Context(), conversation, title); err != nil {
		h.logRenameFailure(conversation, err)
		status, message := renameFailureReply(err)
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	if err := h.scanner.Rename(req.Tool, req.ID, title); err != nil {
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
	// The tab follows after the reply, since an unanswered Automation
	// prompt holds osascript; a tab that cannot take the name keeps its own.
	go func(ctx context.Context) {
		_ = conversations.TitleTab(ctx, conversation, h.scanner.Processes, h.launcher, title)
	}(context.WithoutCancel(r.Context()))
}

// renameFailureStatuses is the status each reason a tool did not take a
// name answers with.
var renameFailureStatuses = map[conversations.RenameReason]int{
	// This kind of conversation cannot be renamed by this server.
	conversations.RenameNotSupported: http.StatusNotImplemented,
	// The program the tool needs is missing on this machine.
	conversations.RenameNoProgram: http.StatusServiceUnavailable,
	// The tool's sign-in is missing or no longer accepted.
	conversations.RenameSignedOut: http.StatusForbidden,
	// A running program holds the conversation.
	conversations.RenameInUse: http.StatusConflict,
	// The tool, standing behind this server, failed.
	conversations.RenameRefused:     http.StatusBadGateway,
	conversations.RenameUnreachable: http.StatusBadGateway,
	conversations.RenameUnexpected:  http.StatusBadGateway,
}

// renameFailureReply is the status and the person's message for a name
// the tool did not take. What the tool said stays in the log.
func renameFailureReply(err error) (int, string) {
	var renameErr *conversations.RenameError
	if !errors.As(err, &renameErr) {
		return http.StatusInternalServerError, "The name could not be saved in the tool. The server log says why."
	}
	status, known := renameFailureStatuses[renameErr.Reason]
	if !known {
		return http.StatusInternalServerError, renameErr.Message()
	}
	return status, renameErr.Message()
}

// logRenameFailure logs a name the tool did not take, with what the
// tool said. Neither ever holds a credential.
func (h *conversationsHandler) logRenameFailure(c conversations.Conversation, err error) {
	if cause := errors.Unwrap(err); cause != nil {
		_, _ = fmt.Fprintf(h.logOut, "! could not rename %s conversation %s in the tool: %v: %v\n", c.Tool, c.ID, err, cause)
		return
	}
	_, _ = fmt.Fprintf(h.logOut, "! could not rename %s conversation %s in the tool: %v\n", c.Tool, c.ID, err)
}

// delete removes a conversation under Archived from this machine for good.
// It answers 200 only once the conversation's transcript is gone.
func (h *conversationsHandler) delete(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var req conversationDeleteRequest
	if !readConversationRequest(w, r, "send {tool, id}", &req) {
		return
	}
	reply, err := h.scanner.Delete(r.Context(), req.Tool, req.ID, h.scanOptions(""))
	var refusal *conversations.DeleteError
	switch {
	case err == nil:
		writeEditJSON(w, http.StatusOK, reply)
	case errors.As(err, &refusal) && refusal.Kind == conversations.DeleteNotListed:
		writeEditJSON(w, http.StatusNotFound, editError{Error: refusal.Message})
	case errors.As(err, &refusal):
		status, known := deleteRefusalStatus[refusal.Kind]
		if !known {
			status = http.StatusInternalServerError
		}
		writeEditJSON(w, status, conversationDeleteRefusal{Error: refusal.Message, Reason: refusal.Kind, Removed: refusal.Removed, Remaining: refusal.Remaining})
	default:
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
	}
}

func (h *conversationsHandler) scanOptions(editor string) conversations.ScanOptions {
	opts := conversations.ScanOptions{Editor: editor}
	if h.global {
		return opts
	}
	root, err := filepath.Abs(h.root)
	if err != nil {
		return opts
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	opts.Within = []string{root}
	if h.worktrees != nil {
		opts.Within = append(opts.Within, h.worktrees(root)...)
	}
	return opts
}

const worktreeTimeout = 3 * time.Second

// gitWorktrees lists root's folder in each worktree of the repository
// that holds it, so a project's conversations include those started in
// its worktrees but not in sibling folders of a larger repository. A
// folder outside Git has none.
func gitWorktrees(root string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeTimeout)
	defer cancel()
	prefix, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--show-prefix").Output()
	if err != nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	inside := filepath.FromSlash(strings.TrimSpace(string(prefix)))
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, filepath.Join(path, inside))
		}
	}
	return paths
}
