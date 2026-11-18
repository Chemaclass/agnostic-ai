package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// pollEvery is how often the companion asks the server whether the list changed.
const pollEvery = 5 * time.Second

// changeWait bounds how long a mark, rename or delete waits for the list
// it changed; past it the call answers and the poll loop brings the list.
const changeWait = 3 * time.Second

// maxRequestBytes bounds a request from the companion's own pages: a tool, an id and a mark or a name.
const maxRequestBytes = 4 << 10

// The server's endpoints the companion uses.
const (
	serverListPath   = "/api/views/conversations"
	serverOpenPath   = "/api/conversations/open"
	serverMarkPath   = "/api/conversations/mark"
	serverRenamePath = "/api/conversations/rename"
	serverDeletePath = "/api/conversations/delete"
)

// inbox keeps the latest list of conversations for the pages. It holds
// the server's answer as it came and reads nothing out of it.
type inbox struct {
	server *server
	// builtFor is the agnostic-ai release this companion was built for.
	builtFor string
	// changed tells the pages that the latest list is new.
	changed func()
	// readClipboard returns the copied text; empty when none is.
	readClipboard func() string
	// readClipboardHTML returns the copied HTML, as a browser puts it
	// beside the text; empty when none is or the system cannot say.
	readClipboardHTML func() string

	// reading lets one read at a time run, from naming the held tag to
	// telling changed, so an older answer never replaces a newer one.
	// Take it before mu; changed takes mu through held.
	reading sync.Mutex
	mu      sync.Mutex
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
	// changeWait bounds the read after a change.
	changeWait time.Duration
	// stopPoll ends the running poll loop; nil while none runs.
	stopPoll context.CancelFunc
	// opens counts the opens the pages asked for, so a late answer knows
	// whether a newer open came after it.
	opens uint64
	// lateFailure is why an open the page stopped waiting for did not
	// open, until the person dismisses it or a newer open begins; nil
	// when none.
	lateFailure *lateFailure
}

// lateFailure is a kept failure: id is the number of its open, and tool
// and chat name the conversation, whose title the list gives when read.
// sentTitle is the title the page sent, for a chat the list does not hold.
type lateFailure struct {
	id         uint64
	tool, chat string
	sentTitle  string
	failure    openFailure
}

// openFailure is a failed open as the panel shows it.
type openFailure struct {
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

func newInbox(server *server, builtFor string, changed func()) *inbox {
	return &inbox{server: server, builtFor: builtFor, changed: changed, refresh: make(chan struct{}, 1), changeWait: changeWait}
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
	in.reading.Lock()
	defer in.reading.Unlock()
	in.readLocked(ctx)
}

// readFirst reads unless a read already ran, such as the poller's
// first one while a page waited for it.
func (in *inbox) readFirst(ctx context.Context) {
	in.reading.Lock()
	defer in.reading.Unlock()
	in.mu.Lock()
	ran := in.latest != nil || in.problem != ""
	in.mu.Unlock()
	if !ran {
		in.readLocked(ctx)
	}
}

func (in *inbox) readLocked(ctx context.Context) {
	in.mu.Lock()
	held := in.tag
	in.mu.Unlock()
	answer, err := in.server.call(ctx, http.MethodGet, serverListPath, nil, held)
	if ctx.Err() != nil {
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

// version names the latest list, so the pages can tell a list event they
// already read past; empty when none is held.
func (in *inbox) version() string {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.tag
}

// held is the latest list; nil when none is.
func (in *inbox) held() json.RawMessage {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.latest
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

// inboxReply is what the pages read: the server's list as it came, a
// note from the companion itself when it has one, and the version of the
// list held when it was asked, which the list event names too.
type inboxReply struct {
	View    json.RawMessage `json:"view"`
	Note    string          `json:"note,omitempty"`
	Version string          `json:"version,omitempty"`
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
		version := in.version()
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
		writeJSON(w, http.StatusOK, inboxReply{View: answer.body, Note: in.versionNote(), Version: version})
		return
	}
	in.mu.Lock()
	latest, problem, tag := in.latest, in.problem, in.tag
	in.mu.Unlock()
	if latest == nil && problem == "" {
		// Nothing read yet: read now instead of answering with nothing.
		in.readFirst(r.Context())
		in.mu.Lock()
		latest, problem, tag = in.latest, in.problem, in.tag
		in.mu.Unlock()
	}
	if latest == nil {
		writeJSON(w, http.StatusBadGateway, apiError{Error: problem})
		return
	}
	writeJSON(w, http.StatusOK, inboxReply{View: latest, Note: in.versionNote(), Version: tag})
}

// stillOpening is the answer to an open the server did not answer in
// time. The open may still happen, so it is no failure: the page keeps
// the list hidden and says nothing.
type stillOpening struct {
	Pending bool   `json:"pending"`
	Message string `json:"message"`
}

// pass hands a page's request to the server's endpoint at path as it
// came, and the server's answer back. The server checks the request; it
// is the one that opens, marks, renames and deletes. A mark, rename or
// delete that worked answers once the list it changed is held, so the
// page draws it right away. With read=later, such as each mark of a run
// but the last, it leaves that list to the poll loop.
func (in *inbox) pass(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiError{Error: "the request is larger than the companion sends"})
			return
		}
		if path == serverOpenPath {
			in.passOpen(w, r, body)
			return
		}
		answer, err := in.server.callWithin(r.Context(), in.server.callWait, http.MethodPost, path, body, "")
		switch {
		case err != nil || answer.status >= 300:
		case r.URL.Query().Get("read") == "later":
			in.refreshSoon()
		default:
			in.readAfterChange(r.Context())
		}
		in.answer(w, answer, err)
	}
}

// readAfterChange reads the list a change made, for up to changeWait. A
// poll read already running goes first. When the wait ends or the page
// gives up, the poll loop reads the list instead.
func (in *inbox) readAfterChange(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, in.changeWait)
	defer cancel()
	read := make(chan struct{})
	go func() {
		in.read(ctx)
		close(read)
	}()
	select {
	case <-read:
	case <-ctx.Done():
	}
	if ctx.Err() != nil {
		in.refreshSoon()
	}
}

// answer writes the server's answer to a passed call, or why none came.
func (in *inbox) answer(w http.ResponseWriter, answer reply, err error) {
	switch {
	case err != nil && timedOut(err):
		writeJSON(w, http.StatusGatewayTimeout, apiError{Error: err.Error()})
		return
	case err != nil:
		writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
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

// passOpen hands an open to the server. After openWait the page gets a
// pending reply, and the call goes on up to openLateWait: a failure
// that arrives then is kept for the panel, unless a newer open began.
func (in *inbox) passOpen(w http.ResponseWriter, r *http.Request, body []byte) {
	body, title := withoutTitle(body)
	var named struct {
		Tool string `json:"tool"`
		ID   string `json:"id"`
	}
	_ = json.Unmarshal(body, &named)
	in.mu.Lock()
	in.opens++
	this := in.opens
	in.lateFailure = nil
	in.mu.Unlock()
	type outcome struct {
		answer reply
		err    error
	}
	settled := make(chan outcome, 1)
	go func() {
		answer, err := in.server.callWithin(context.WithoutCancel(r.Context()), in.server.openLateWait, http.MethodPost, serverOpenPath, body, "")
		settled <- outcome{answer, err}
	}()
	waited := time.NewTimer(in.server.openWait)
	defer waited.Stop()
	select {
	case got := <-settled:
		if got.err == nil && got.answer.status < 300 {
			in.refreshSoon()
		}
		in.answer(w, got.answer, got.err)
		return
	case <-waited.C:
		in.refreshSoon()
		writeJSON(w, http.StatusAccepted, stillOpening{Pending: true, Message: "Still opening. This takes longer than usual."})
	case <-r.Context().Done():
	}
	go func() {
		got := <-settled
		in.settleLate(lateFailure{id: this, tool: named.Tool, chat: named.ID, sentTitle: title}, lateOutcomeOf(got.answer, got.err))
	}()
}

// noLateAnswer is what the panel reads about an open that got no answer
// after its pending reply: a running server stops writing after 90
// seconds, which says nothing about whether the chat opened.
const noLateAnswer = "agnostic-ai did not answer in time. The chat may still have opened."

// lateOutcome is how an open ended after its pending reply: failure is
// what the panel shows, nil when there is nothing to show; said is the
// outcome the log gets, without the reason.
type lateOutcome struct {
	failure *openFailure
	said    string
}

func lateOutcomeOf(answer reply, err error) lateOutcome {
	switch {
	case errors.Is(err, errResting):
		return lateOutcome{said: "the companion quit first"}
	case err != nil:
		return lateOutcome{failure: &openFailure{Message: noLateAnswer}, said: "no answer"}
	case answer.status >= 300:
		return lateOutcome{failure: &openFailure{Message: errorOf(answer)}, said: "not opened"}
	}
	var told struct {
		Opened  *bool  `json:"opened"`
		Message string `json:"message"`
		Command string `json:"command"`
	}
	if answer.status != http.StatusOK || json.Unmarshal(answer.body, &told) != nil || told.Opened == nil {
		return lateOutcome{said: "no word on whether it opened"}
	}
	if *told.Opened {
		return lateOutcome{said: "opened"}
	}
	return lateOutcome{failure: &openFailure{Message: told.Message, Command: told.Command}, said: "not opened"}
}

// settleLate keeps the failure of the open late names for the panel,
// and asks for a new list either way.
func (in *inbox) settleLate(late lateFailure, outcome lateOutcome) {
	defer in.refreshSoon()
	log.Print("an open the page stopped waiting for: " + outcome.said)
	if outcome.failure == nil {
		return
	}
	in.mu.Lock()
	newest := late.id == in.opens
	if newest {
		late.failure = *outcome.failure
		in.lateFailure = &late
	}
	in.mu.Unlock()
	if newest {
		in.changed()
	}
}

// lateOpenReply is a kept failure as the panel reads it. Title is the
// chat's name in the list held now; empty when the list does not hold it.
type lateOpenReply struct {
	ID      uint64 `json:"id"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

// lateOpen answers GET /api/open/late with the kept failure of an open
// the page stopped waiting for; 204 when there is none. Reading it keeps
// it: the panel shows it again until the person dismisses it.
func (in *inbox) lateOpen(w http.ResponseWriter, _ *http.Request) {
	in.mu.Lock()
	late, latest := in.lateFailure, in.latest
	in.mu.Unlock()
	if late == nil {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	title := titleIn(latest, late.tool, late.chat)
	if title == "" {
		title = late.sentTitle
	}
	writeJSON(w, http.StatusOK, lateOpenReply{ID: late.id, Title: title, Message: late.failure.Message, Command: late.failure.Command})
}

// titleIn is the title list gives the chat; empty when it does not list it.
func titleIn(list json.RawMessage, tool, chat string) string {
	var view struct {
		Conversations []struct {
			Tool  string `json:"tool"`
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"conversations"`
	}
	if json.Unmarshal(list, &view) != nil {
		return ""
	}
	for _, row := range view.Conversations {
		if row.Tool == tool && row.ID == chat {
			return row.Title
		}
	}
	return ""
}

// dismissLateOpen answers POST /api/open/late/dismiss {id}: the person
// dismissed that failure, so it is no longer kept. Another id changes
// nothing, since a newer failure may have replaced it.
func (in *inbox) dismissLateOpen(w http.ResponseWriter, r *http.Request) {
	var dismissed struct {
		ID uint64 `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&dismissed); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send {id}"})
		return
	}
	in.mu.Lock()
	if in.lateFailure != nil && in.lateFailure.id == dismissed.ID {
		in.lateFailure = nil
	}
	in.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// maxSentTitle bounds the title a page sends with an open: a list row shows one line.
const maxSentTitle = 200

// withoutTitle takes the title a page sent out of an open, since the
// server refuses any field but tool, id and editor. A body that is no
// JSON object goes on as it came, for the server to refuse.
func withoutTitle(body []byte) ([]byte, string) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return body, ""
	}
	sent, ok := fields["title"]
	if !ok {
		return body, ""
	}
	delete(fields, "title")
	rest, err := json.Marshal(fields)
	if err != nil {
		return body, ""
	}
	var title string
	_ = json.Unmarshal(sent, &title)
	if runes := []rune(title); len(runes) > maxSentTitle {
		title = string(runes[:maxSentTitle])
	}
	return rest, title
}
