package cli

import (
	"context"
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
	conversationsViewPath = "/api/views/conversations"
	conversationOpenPath  = "/api/conversations/open"
)

// maxConversationOpenBytes bounds an open request: a tool, an id and an editor.
const maxConversationOpenBytes = 4 << 10

type conversationOpenRequest struct {
	Tool   conversations.Tool `json:"tool"`
	ID     string             `json:"id"`
	Editor string             `json:"editor"`
}

// conversationsHandler lists the coding tools' conversations and
// reopens one where it lives. A project server lists the conversations
// started in the project or one of its Git worktrees; a global server
// lists them all.
type conversationsHandler struct {
	root      string
	global    bool
	scanner   *conversations.Scanner
	launcher  conversations.Launcher
	processes conversations.Processes
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
		processes: scanner.Processes,
		worktrees: gitWorktrees,
	}
}

func (h *conversationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case conversationsViewPath:
		h.view(w, r)
	case conversationOpenPath:
		h.open(w, r)
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
	writeEditJSON(w, http.StatusOK, h.scanner.Scan(r.Context(), opts))
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
	if req.Tool != conversations.ToolClaude && req.Tool != conversations.ToolCodex && req.Tool != conversations.ToolCursor {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "tool must be claude, codex or cursor"})
		return
	}
	// The route comes from the server's own scan; the request only names
	// which conversation.
	conversation, ok := h.scanner.Find(r.Context(), req.Tool, req.ID, h.scanOptions(req.Editor))
	if !ok {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "That conversation is not in this list any more. Refresh the page."})
		return
	}
	writeEditJSON(w, http.StatusOK, conversations.Open(r.Context(), conversation, h.processes, h.launcher))
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
