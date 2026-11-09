package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// pollEvery is how often the widget checks the tools' stores for a change.
const pollEvery = 5 * time.Second

// maxRequestBytes bounds a request from the widget's own pages: a tool, an id and a mark.
const maxRequestBytes = 4 << 10

// maxQuery bounds a search, as the server does.
const maxQuery = 200

// inbox reads every conversation on this machine, as a global
// agnostic-ai server does, and keeps the latest list for the pages.
type inbox struct {
	scanner  *conversations.Scanner
	launcher conversations.Launcher
	// changed tells the pages that the latest list is new.
	changed func()

	mu     sync.Mutex
	latest *conversations.View
	// refresh asks the poll loop for a read now, after an open or a mark.
	refresh chan struct{}
}

func newInbox(scanner *conversations.Scanner, launcher conversations.Launcher, changed func()) *inbox {
	return &inbox{scanner: scanner, launcher: launcher, changed: changed, refresh: make(chan struct{}, 1)}
}

// poll reads the list now, then checks every pollEvery whether a store
// changed and reads again only when one did, when a refresh is asked
// for, or when the last read is StampMaxAge old, until ctx ends. While
// nothing is written anywhere, a tick costs a look at the files' sizes
// and times.
func (in *inbox) poll(ctx context.Context) {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	var readStamp string
	var readAt time.Time
	for {
		asked := false
		select {
		case <-in.refresh:
			asked = true
		default:
		}
		stamp := in.scanner.Stamp()
		due := asked || stamp != readStamp || time.Since(readAt) >= conversations.StampMaxAge
		if due {
			view := in.scanner.Scan(ctx, conversations.ScanOptions{})
			// The scan may itself write the marks, so the stamp is taken after it.
			readStamp, readAt = in.scanner.Stamp(), time.Now()
			in.mu.Lock()
			in.latest = &view
			in.mu.Unlock()
			in.changed()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-in.refresh:
			// Put the request back for the top of the loop to see.
			in.refreshSoon()
		}
	}
}

func (in *inbox) refreshSoon() {
	select {
	case in.refresh <- struct{}{}:
	default:
		// A read is already asked for.
	}
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// list answers GET /api/inbox?q=<words>. Without q it returns the latest
// polled list; with q it searches every conversation.
func (in *inbox) list(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if len([]rune(query)) > maxQuery {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "The search is longer than 200 characters."})
		return
	}
	if query != "" {
		writeJSON(w, http.StatusOK, in.scanner.Scan(r.Context(), conversations.ScanOptions{Query: query}))
		return
	}
	in.mu.Lock()
	latest := in.latest
	in.mu.Unlock()
	if latest == nil {
		writeJSON(w, http.StatusOK, in.scanner.Scan(r.Context(), conversations.ScanOptions{}))
		return
	}
	writeJSON(w, http.StatusOK, latest)
}

type conversationRequest struct {
	Tool conversations.Tool `json:"tool"`
	ID   string             `json:"id"`
	// As is the mark, for a mark request: seen, completed or open.
	As conversations.Mark `json:"as,omitempty"`
}

func readRequest(w http.ResponseWriter, r *http.Request) (conversationRequest, bool) {
	return readBody[conversationRequest](w, r, "{tool, id}")
}

// open answers POST /api/open: the conversation opens where it lives
// and counts as seen.
func (in *inbox) open(w http.ResponseWriter, r *http.Request) {
	req, ok := readRequest(w, r)
	if !ok {
		return
	}
	reply, found := in.scanner.Reopen(r.Context(), req.Tool, req.ID, conversations.ScanOptions{}, in.launcher)
	if !found {
		writeJSON(w, http.StatusNotFound, apiError{Error: "That conversation is gone."})
		return
	}
	in.refreshSoon()
	writeJSON(w, http.StatusOK, reply)
}

// mark answers POST /api/mark.
func (in *inbox) mark(w http.ResponseWriter, r *http.Request) {
	req, ok := readRequest(w, r)
	if !ok {
		return
	}
	if !conversations.KnownMark(req.As) {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "as must be seen, completed or open"})
		return
	}
	if _, found := in.scanner.Find(r.Context(), req.Tool, req.ID, conversations.ScanOptions{}); !found {
		writeJSON(w, http.StatusNotFound, apiError{Error: "That conversation is gone."})
		return
	}
	if err := in.scanner.Mark(req.Tool, req.ID, req.As); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	// The pages read the list again once the poll loop says it changed.
	in.refreshSoon()
	w.WriteHeader(http.StatusNoContent)
}

// markAllSeen answers POST /api/mark-all-seen: everything written up to
// now counts as read.
func (in *inbox) markAllSeen(w http.ResponseWriter, _ *http.Request) {
	if err := in.scanner.MarkAllSeen(); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	in.refreshSoon()
	w.WriteHeader(http.StatusNoContent)
}
