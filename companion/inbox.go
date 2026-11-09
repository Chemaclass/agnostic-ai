package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// pollEvery is how often the companion asks the server whether the list changed.
const pollEvery = 5 * time.Second

// maxRequestBytes bounds a request from the companion's own pages: a tool, an id and a mark.
const maxRequestBytes = 4 << 10

// The server's endpoints the companion uses.
const (
	serverListPath = "/api/views/conversations"
	serverOpenPath = "/api/conversations/open"
	serverMarkPath = "/api/conversations/mark"
)

// inbox keeps the latest list of conversations for the pages. It holds
// the server's answer as it came and reads nothing out of it.
type inbox struct {
	server *server
	// builtFor is the agnostic-ai release this companion was built for.
	builtFor string
	// changed tells the pages that the latest list is new.
	changed func()

	mu sync.Mutex
	// latest is the server's last list; nil before the first one and
	// after a failed read, so the pages never show an old list as new.
	latest json.RawMessage
	// tag is the server's tag of latest; sent back, the server answers
	// "unchanged" without reading anything.
	tag string
	// problem is why the last read failed; empty when it worked.
	problem string
	// refresh asks the poll loop for a read now, after an open or a mark.
	refresh chan struct{}
	// stopPoll ends the running poll loop; nil while none runs.
	stopPoll context.CancelFunc
}

func newInbox(server *server, builtFor string, changed func()) *inbox {
	return &inbox{server: server, builtFor: builtFor, changed: changed, refresh: make(chan struct{}, 1)}
}

// start begins asking the server every pollEvery, until stop.
func (in *inbox) start() {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.stopPoll != nil {
		return
	}
	ctx, stop := context.WithCancel(context.Background())
	in.stopPoll = stop
	go in.poll(ctx)
}

// stop ends the asking and forgets the list, so the next start reads a new one.
func (in *inbox) stop() {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.stopPoll != nil {
		in.stopPoll()
		in.stopPoll = nil
	}
	in.latest, in.tag, in.problem = nil, "", ""
}

func (in *inbox) poll(ctx context.Context) {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		in.read(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-in.refresh:
		}
	}
}

// read asks the server for the list, naming the one already held. The
// server answers "unchanged" while no store was written and the minute
// has not turned, which costs it a look at the files' sizes and times.
func (in *inbox) read(ctx context.Context) {
	in.mu.Lock()
	held := in.tag
	in.mu.Unlock()
	answer, err := in.server.call(ctx, http.MethodGet, serverListPath, nil, held)
	if ctx.Err() != nil {
		// Stopped meanwhile: stop has already forgotten the list.
		return
	}
	if err == nil && answer.status == http.StatusNotModified {
		return
	}
	in.mu.Lock()
	switch {
	case err != nil:
		in.latest, in.tag, in.problem = nil, "", err.Error()
	case answer.status != http.StatusOK:
		in.latest, in.tag, in.problem = nil, "", errorOf(answer)
	default:
		in.latest, in.tag, in.problem = answer.body, answer.tag, ""
	}
	in.mu.Unlock()
	in.changed()
}

// errorOf is the message of a refused call, as the server worded it.
func errorOf(answer reply) string {
	var refused apiError
	if err := json.Unmarshal(answer.body, &refused); err != nil || refused.Error == "" {
		return "agnostic-ai answered " + http.StatusText(answer.status) + "."
	}
	return refused.Error
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

// inboxReply is what the pages read: the server's list as it came, and
// a note from the companion itself when it has one.
type inboxReply struct {
	View json.RawMessage `json:"view"`
	Note string          `json:"note,omitempty"`
}

// versionNote says that the companion and the installed agnostic-ai are
// different releases; empty when they match or one does not say.
func (in *inbox) versionNote() string {
	serves := in.server.runningVersion()
	if in.builtFor == "" || serves == "" || in.builtFor == serves {
		return ""
	}
	return "This companion was built for agnostic-ai " + in.builtFor + " and " + serves + " is installed. Run: agnostic-ai companion install"
}

// list answers GET /api/inbox?q=<words>&before=<time>. Without q it
// returns the latest polled list; with q it asks the server, which
// searches every conversation one week at a time, and before is the
// next value of the page already read.
func (in *inbox) list(w http.ResponseWriter, r *http.Request) {
	if query := r.URL.Query().Get("q"); query != "" {
		asked := url.Values{"q": {query}}
		if before := r.URL.Query().Get("before"); before != "" {
			asked.Set("before", before)
		}
		answer, err := in.server.call(r.Context(), http.MethodGet, serverListPath+"?"+asked.Encode(), nil, "")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
			return
		}
		if answer.status != http.StatusOK {
			writeJSON(w, answer.status, apiError{Error: errorOf(answer)})
			return
		}
		writeJSON(w, http.StatusOK, inboxReply{View: answer.body, Note: in.versionNote()})
		return
	}
	in.mu.Lock()
	latest, problem := in.latest, in.problem
	in.mu.Unlock()
	if latest == nil && problem == "" {
		// Nothing read yet: read now instead of answering with nothing.
		in.read(r.Context())
		in.mu.Lock()
		latest, problem = in.latest, in.problem
		in.mu.Unlock()
	}
	if latest == nil {
		writeJSON(w, http.StatusBadGateway, apiError{Error: problem})
		return
	}
	writeJSON(w, http.StatusOK, inboxReply{View: latest, Note: in.versionNote()})
}

// pass hands a page's request to the server's endpoint at path as it
// came, and the server's answer back. The server checks the request; it
// is the one that opens and marks. A call that worked asks for a new
// list.
func (in *inbox) pass(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiError{Error: "the request is larger than the companion sends"})
			return
		}
		answer, err := in.server.call(r.Context(), http.MethodPost, path, body, "")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
			return
		}
		if answer.status < 300 {
			in.refreshSoon()
		}
		if len(answer.body) == 0 {
			w.WriteHeader(answer.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(answer.status)
		_, _ = w.Write(answer.body)
	}
}
