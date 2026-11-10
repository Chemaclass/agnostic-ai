package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

const (
	conversationsViewPath  = "/api/views/conversations"
	conversationOpenPath   = "/api/conversations/open"
	conversationMarkPath   = "/api/conversations/mark"
	conversationRenamePath = "/api/conversations/rename"
)

// maxConversationOpenBytes bounds an open request: a tool, an id and an editor.
const maxConversationOpenBytes = 4 << 10

type conversationOpenRequest struct {
	Tool   conversations.Tool `json:"tool"`
	ID     string             `json:"id"`
	Editor string             `json:"editor"`
}

type conversationMarkRequest struct {
	Tool conversations.Tool `json:"tool"`
	ID   string             `json:"id"`
	// As is seen, completed, open (which takes a completion back) or
	// hidden (out of every list until something new is written or the
	// conversation starts working).
	As conversations.Mark `json:"as"`
}

type conversationRenameRequest struct {
	Tool  conversations.Tool `json:"tool"`
	ID    string             `json:"id"`
	Title string             `json:"title"`
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
}

func newConversationsHandler(root string, global bool) *conversationsHandler {
	scanner := conversations.NewScanner()
	return &conversationsHandler{
		root:      root,
		global:    global,
		scanner:   scanner,
		launcher:  conversations.SystemLauncher(),
		worktrees: gitWorktrees,
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
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "no endpoint at " + r.URL.Path})
	}
}

func (h *conversationsHandler) view(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeEditJSON(w, http.StatusMethodNotAllowed, editError{Error: r.Method + " is not supported here; use GET"})
		return
	}
	opts := h.scanOptions(r.URL.Query().Get("editor"))
	if before := r.URL.Query().Get("before"); before != "" {
		at, err := time.Parse(time.RFC3339, before)
		if err != nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "before must be a time such as 2026-11-01T12:00:00Z, the next value of the previous page"})
			return
		}
		opts.Before = at
	}
	opts.Query = r.URL.Query().Get("q")
	if len([]rune(opts.Query)) > maxConversationQuery {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "q is longer than 200 characters"})
		return
	}
	if tool := r.URL.Query().Get("tool"); tool != "" {
		if !knownConversationTool(conversations.Tool(tool)) {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "tool must be claude, codex or cursor"})
			return
		}
		opts.Tool = conversations.Tool(tool)
	}
	// The tag is taken before the scan. A scan that itself writes the
	// marks then changes the stamp, and the caller's next question gets
	// one more scan instead of a list that missed a write made meanwhile.
	tag := h.listTag(r)
	w.Header().Set("ETag", tag)
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeEditJSON(w, http.StatusOK, h.scanner.Scan(r.Context(), opts))
}

// listTag names what a list request would answer: it changes when a
// scan could answer differently (conversations.Scanner.Stamp) or the
// request asks for something else. A caller that polls sends it back as
// If-None-Match and gets 304 without a scan while it holds.
func (h *conversationsHandler) listTag(r *http.Request) string {
	sum := sha256.Sum256([]byte(h.scanner.Stamp() + "\x00" + r.URL.RawQuery))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// maxConversationQuery bounds a search, which every row is matched against.
const maxConversationQuery = 200

func knownConversationTool(tool conversations.Tool) bool {
	return tool == conversations.ToolClaude || tool == conversations.ToolCodex || tool == conversations.ToolCursor
}

func (h *conversationsHandler) open(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var req conversationOpenRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConversationOpenBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send {tool, id, editor}"})
		return
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send one JSON object"})
		return
	}
	if !knownConversationTool(req.Tool) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "tool must be claude, codex or cursor"})
		return
	}
	// The route comes from the server's own scan; the request only names
	// which conversation.
	reply, found := h.scanner.Reopen(r.Context(), req.Tool, req.ID, h.scanOptions(req.Editor), h.launcher)
	if !found {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "That conversation is not in this list any more. Refresh the page."})
		return
	}
	writeEditJSON(w, http.StatusOK, reply)
}

// mark records that the person saw a conversation, completed it, took
// a completion back, or hid it.
func (h *conversationsHandler) mark(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var req conversationMarkRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConversationOpenBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send {tool, id, as}"})
		return
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send one JSON object"})
		return
	}
	if !knownConversationTool(req.Tool) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "tool must be claude, codex or cursor"})
		return
	}
	if !conversations.KnownMark(req.As) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "as must be seen, completed, open or hidden"})
		return
	}
	// Only a conversation the server lists gets a mark, so the file
	// never holds an id a request made up.
	if _, ok := h.scanner.Find(r.Context(), req.Tool, req.ID, h.scanOptions("")); !ok {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "That conversation is not in this list any more. Refresh the page."})
		return
	}
	if err := h.scanner.Mark(req.Tool, req.ID, req.As); err != nil {
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rename saves the name the person chose for a conversation, which
// every server sharing the marks file then shows.
func (h *conversationsHandler) rename(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var req conversationRenameRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConversationOpenBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send {tool, id, title}"})
		return
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send one JSON object"})
		return
	}
	if !knownConversationTool(req.Tool) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "tool must be claude, codex or cursor"})
		return
	}
	title, err := conversations.CleanTitle(req.Title)
	if err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
		return
	}
	if _, ok := h.scanner.Find(r.Context(), req.Tool, req.ID, h.scanOptions("")); !ok {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "That conversation is not in this list any more. Refresh the page."})
		return
	}
	if err := h.scanner.Rename(req.Tool, req.ID, title); err != nil {
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
