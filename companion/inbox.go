package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// pollEvery is how often the companion asks the server whether the list changed.
const pollEvery = 5 * time.Second

// listVersionHeader names, on a change's answer, the version of the list
// read after it; absent when that read did not end in time.
const listVersionHeader = "List-Version"

// changeWait bounds how long a successful change waits for the list
// it changed; past it the call answers and the poll loop brings the list.
const changeWait = 3 * time.Second

// maxRequestBytes bounds a request from the companion's own pages: a tool, an id and a mark or a name.
const maxRequestBytes = 4 << 10

// The server's endpoints the companion uses.
const (
	serverListPath            = "/api/views/conversations"
	serverOpenPath            = "/api/conversations/open"
	serverMarkPath            = "/api/conversations/mark"
	serverRenamePath          = "/api/conversations/rename"
	serverDeletePath          = "/api/conversations/delete"
	serverStopShellPath       = "/api/conversations/stop-shell"
	serverStopSessionPath     = "/api/conversations/stop-session"
	serverMailboxSendPath     = "/api/mailbox/send"
	serverMailboxThreadsPath  = "/api/mailbox/threads"
	serverMailboxThreadPath   = "/api/mailbox/thread"
	serverMailboxStagePath    = "/api/mailbox/stage"
	serverMailboxRefsPath     = "/api/mailbox/refs"
	serverMailboxClaimsPath   = "/api/mailbox/claims"
	serverMailboxClaimPath    = "/api/mailbox/claim"
	serverMailboxReleasePath  = "/api/mailbox/release"
	serverMailboxDismissPath  = "/api/mailbox/conflict/dismiss"
	serverMailboxHandoffPath  = "/api/mailbox/handoff"
	serverMailboxApprovePath  = "/api/mailbox/approve"
	serverMailboxQueuePath    = "/api/mailbox/queue"
	serverMailboxSettingsPath = "/api/mailbox/settings"
	// serverApprovalsAskPath turns automatic approvals off; the server
	// takes no other mode.
	serverApprovalsAskPath = "/api/approvals/ask"
	// serverApprovalsDecidePath answers one request the approvals hook
	// holds for the companion.
	serverApprovalsDecidePath = "/api/approvals/decide"
	// The user settings document, its one write and the sync it may need.
	serverSettingsViewPath = "/api/views/settings"
	serverSettingsPath     = "/api/settings"
	serverSettingsSyncPath = "/api/settings/sync"
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
	writeClipboard    func(context.Context, string) bool

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
	// lateNotice is what a late open has to report until it is dismissed
	// or a newer open begins.
	lateNotice *lateOpenNotice
}

// lateOpenNotice is a kept notice: id is the number of its open, and tool
// and chat name the conversation, whose title the list gives when read.
// sentTitle is the title the page sent, for a chat the list does not hold.
type lateOpenNotice struct {
	id         uint64
	tool, chat string
	sentTitle  string
	notice     openNotice
}

type openNotice struct {
	message string
	command string
	guessed bool
	reached string
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
	answer, err := in.server.get(ctx, serverListPath, held)
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
	View              json.RawMessage `json:"view"`
	Note              string          `json:"note,omitempty"`
	Version           string          `json:"version,omitempty"`
	ConfigRoot        string          `json:"configRoot,omitempty"`
	MailboxNativeCopy bool            `json:"mailboxNativeCopy,omitempty"`
}

func (in *inbox) listReply(ctx context.Context, view json.RawMessage, version string) inboxReply {
	health := in.server.runningHealth(ctx)
	reply := inboxReply{View: view, Version: version, ConfigRoot: health.Root, MailboxNativeCopy: in.writeClipboard != nil}
	if in.builtFor != "" && health.Version != "" && in.builtFor != health.Version {
		reply.Note = "This companion was built for agnostic-ai " + in.builtFor + " and " + health.Version + " is installed. Run: agnostic-ai companion install"
	}
	return reply
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
		answer, err := in.server.get(r.Context(), serverListPath+"?"+asked.Encode(), "")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
			return
		}
		if answer.status != http.StatusOK {
			writeJSON(w, answer.status, apiError{Error: errorOf(answer)})
			return
		}
		writeJSON(w, http.StatusOK, in.listReply(r.Context(), answer.body, version))
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
	writeJSON(w, http.StatusOK, in.listReply(r.Context(), latest, tag))
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
// applies the action. A successful change answers once the list it changed
// is held, naming that list's version, so the page draws it right away. With read=later, such as each mark of a run
// but the last, it leaves that list to the poll loop.
func (in *inbox) pass(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch path {
		case serverMailboxSendPath, serverMailboxThreadPath, serverMailboxStagePath, serverMailboxRefsPath,
			serverMailboxClaimPath, serverMailboxReleasePath, serverMailboxDismissPath, serverMailboxHandoffPath,
			serverMailboxApprovePath, serverMailboxQueuePath, serverMailboxSettingsPath:
			if !mailboxPageAllowed(w, r) {
				return
			}
			if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
				writeJSON(w, http.StatusUnsupportedMediaType, apiError{Error: "Send Content-Type: application/json."})
				return
			}
		case serverSettingsPath, serverSettingsSyncPath:
			if !settingsPageAllowed(w, r) {
				return
			}
			if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
				writeJSON(w, http.StatusUnsupportedMediaType, apiError{Error: "Send Content-Type: application/json."})
				return
			}
		case serverApprovalsDecidePath:
			if !pageAllowed(w, r, "Requests are answered from the companion page only.") {
				return
			}
			if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
				writeJSON(w, http.StatusUnsupportedMediaType, apiError{Error: "Send Content-Type: application/json."})
				return
			}
		}
		limit := int64(maxRequestBytes)
		switch path {
		case serverMailboxSendPath, serverMailboxClaimPath, serverMailboxReleasePath:
			// Let the server answer with the mailbox size guard.
			limit = 1 << 20
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiError{Error: "the request is larger than the companion sends"})
			return
		}
		if path == serverOpenPath {
			in.passOpen(w, r, body)
			return
		}
		wait := in.server.callWait
		if path == serverSettingsSyncPath {
			// A sync writes every tool's user files.
			wait = in.server.sendWait
		}
		if path == serverMailboxStagePath {
			if in.copyMailboxMessage(w, r, body) {
				return
			}
			wait = in.server.sendWait
		}
		answer, err := in.server.callWithin(r.Context(), wait, http.MethodPost, path, body, "")
		switch {
		case err != nil || path == serverMailboxRefsPath || path == serverMailboxSettingsPath:
		case (path == serverMailboxClaimPath || path == serverMailboxHandoffPath) && mailboxOwnershipRefusal(answer):
			in.noteListChange(w, r)
		case answer.status >= 300:
		case path == serverMailboxStagePath && !mailboxDidStage(answer.body):
		default:
			in.noteListChange(w, r)
		}
		in.answer(w, answer, err)
	}
}

func mailboxOwnershipRefusal(answer reply) bool {
	if answer.status != http.StatusUnprocessableEntity {
		return false
	}
	var result struct {
		Guard string `json:"guard"`
	}
	return json.Unmarshal(answer.body, &result) == nil && result.Guard == "ownership"
}

func (in *inbox) noteListChange(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("read") == "later" {
		in.refreshSoon()
	} else if in.readAfterChange(r.Context()) {
		if version := in.version(); version != "" {
			w.Header().Set(listVersionHeader, version)
		}
	}
}

func mailboxDidStage(body []byte) bool {
	var result struct {
		Staged bool `json:"staged"`
	}
	return json.Unmarshal(body, &result) == nil && result.Staged
}

// The companion owns the clipboard action so the WebView need not keep
// a clipboard permission alive across the prepare request.
func (in *inbox) copyMailboxMessage(w http.ResponseWriter, r *http.Request, body []byte) bool {
	var req struct {
		Thread  string `json:"thread"`
		N       int    `json:"n"`
		How     string `json:"how"`
		Confirm bool   `json:"confirm"`
	}
	if in.writeClipboard == nil || json.Unmarshal(body, &req) != nil || req.How != "copy" {
		return false
	}
	if req.Confirm {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "Ask the companion to copy the message."})
		return true
	}

	ctx := r.Context()
	prepared, err := in.server.callWithin(ctx, in.server.callWait, http.MethodPost, serverMailboxStagePath, body, "")
	if err != nil || prepared.status != http.StatusOK {
		in.answer(w, prepared, err)
		return true
	}
	var message struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(prepared.body, &message) != nil || message.Text == "" {
		writeJSON(w, http.StatusBadGateway, apiError{Error: "agnostic-ai did not return a mailbox message to copy."})
		return true
	}
	if ctx.Err() != nil || !in.writeClipboard(ctx, message.Text) {
		writeJSON(w, http.StatusOK, map[string]any{"staged": false, "message": "Could not copy the message. It is still waiting."})
		return true
	}

	req.Confirm = true
	confirm, _ := json.Marshal(req)
	receipt, err := in.server.callWithin(context.WithoutCancel(ctx), in.server.callWait, http.MethodPost, serverMailboxStagePath, confirm, "")
	if err != nil || receipt.status != http.StatusOK || !mailboxDidStage(receipt.body) {
		in.refreshSoon()
		writeJSON(w, http.StatusOK, map[string]any{
			"staged":  false,
			"copied":  true,
			"message": "The message is on the clipboard, but the mailbox could not mark it staged. Refresh the thread and check its state.",
		})
		return true
	}
	in.noteListChange(w, r)
	in.answer(w, receipt, nil)
	return true
}

func (in *inbox) readMailbox(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mailboxPageAllowed(w, r) {
			return
		}
		endpoint := path
		if r.URL.RawQuery != "" {
			endpoint += "?" + r.URL.RawQuery
		}
		answer, err := in.server.get(r.Context(), endpoint, "")
		in.answer(w, answer, err)
	}
}

// readSettings passes the page's settings read, with the tag it holds,
// and the server's tag back, so a read with nothing new answers 304.
func (in *inbox) readSettings(w http.ResponseWriter, r *http.Request) {
	if !settingsPageAllowed(w, r) {
		return
	}
	answer, err := in.server.get(r.Context(), serverSettingsViewPath, r.Header.Get("If-None-Match"))
	if err == nil && answer.tag != "" {
		w.Header().Set("ETag", answer.tag)
	}
	in.answer(w, answer, err)
}

func settingsPageAllowed(w http.ResponseWriter, r *http.Request) bool {
	return pageAllowed(w, r, "Settings are changed from the companion page only.")
}

func mailboxPageAllowed(w http.ResponseWriter, r *http.Request) bool {
	return pageAllowed(w, r, "The mailbox is available from the companion page only.")
}

// pageAllowed answers 403 with refusal, and is false, for a request
// that did not come from the companion's own page.
func pageAllowed(w http.ResponseWriter, r *http.Request, refusal string) bool {
	origin := r.Header.Get("Origin")
	samePage := origin == "" || origin == "null" || origin == "wails://"+r.Host || origin == "http://"+r.Host
	if !samePage || strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		writeJSON(w, http.StatusForbidden, apiError{Error: refusal})
		return false
	}
	return true
}

// readAfterChange reads the list a change made, for up to changeWait,
// and says whether it did. A poll read already running goes first. When
// the wait ends or the page gives up, the poll loop reads the list instead.
func (in *inbox) readAfterChange(ctx context.Context) bool {
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
		return false
	}
	return true
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
// pending reply, and the call goes on up to openLateWait. A late failure
// or switch with a hint is kept for the panel until a newer open starts.
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
	in.lateNotice = nil
	in.mu.Unlock()
	type outcome struct {
		answer reply
		err    error
	}
	settled := make(chan outcome, 1)
	go func() {
		ctx := context.WithoutCancel(r.Context())
		answer, err := in.server.callWithin(ctx, in.server.openLateWait, http.MethodPost, serverOpenPath, body, "")
		if plain, named := withoutField(body, "helper"); named && err == nil && refusesHelper(answer) {
			answer, err = in.server.callWithin(ctx, in.server.openLateWait, http.MethodPost, serverOpenPath, plain, "")
		}
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
		in.settleLate(lateOpenNotice{id: this, tool: named.Tool, chat: named.ID, sentTitle: title}, lateOutcomeOf(got.answer, got.err))
	}()
}

// noLateAnswer is what the panel reads about an open that got no answer
// after its pending reply: a running server stops writing after 90
// seconds, which says nothing about whether the chat opened.
const noLateAnswer = "agnostic-ai did not answer in time. The chat may still have opened."

// lateOutcome is how an open ended after its pending reply. The notice is
// for the panel; said is the outcome the log gets, without the reason.
type lateOutcome struct {
	notice *openNotice
	said   string
}

func lateOutcomeOf(answer reply, err error) lateOutcome {
	switch {
	case errors.Is(err, errResting):
		return lateOutcome{said: "the companion quit first"}
	case err != nil:
		return lateOutcome{notice: &openNotice{message: noLateAnswer}, said: "no answer"}
	case answer.status >= 300:
		return lateOutcome{notice: &openNotice{message: errorOf(answer)}, said: "not opened"}
	}
	var told struct {
		Opened  *bool     `json:"opened"`
		Guessed bool      `json:"guessed"`
		Message string    `json:"message"`
		Command string    `json:"command"`
		Reached string    `json:"reached"`
		Helper  *struct{} `json:"helper"`
	}
	if answer.status != http.StatusOK || json.Unmarshal(answer.body, &told) != nil || told.Opened == nil {
		return lateOutcome{said: "no word on whether it opened"}
	}
	if *told.Opened {
		if told.Guessed {
			return lateOutcome{notice: &openNotice{message: told.Message, guessed: true}, said: "switched to a possible tab"}
		}
		if told.Reached != "subagent" && (told.Reached == "parent" || told.Helper != nil) {
			return lateOutcome{notice: &openNotice{message: told.Message, command: told.Command, reached: "parent"}, said: "opened the parent"}
		}
		return lateOutcome{said: "opened"}
	}
	return lateOutcome{notice: &openNotice{message: told.Message, command: told.Command}, said: "not opened"}
}

// settleLate keeps the latest open's notice and asks for a fresh list.
func (in *inbox) settleLate(late lateOpenNotice, outcome lateOutcome) {
	defer in.refreshSoon()
	log.Print("an open the page stopped waiting for: " + outcome.said)
	if outcome.notice == nil {
		return
	}
	in.mu.Lock()
	newest := late.id == in.opens
	if newest {
		late.notice = *outcome.notice
		in.lateNotice = &late
	}
	in.mu.Unlock()
	if newest {
		in.changed()
	}
}

// lateOpenReply is a kept notice as the panel reads it. Title is the
// chat's name in the list held now; empty when the list does not hold it.
type lateOpenReply struct {
	ID      uint64 `json:"id"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
	Guessed bool   `json:"guessed,omitempty"`
	Reached string `json:"reached,omitempty"`
}

// lateOpen answers GET /api/open/late with the kept notice of an open
// the page stopped waiting for; 204 when there is none. Reading it keeps
// it: the panel shows it again until the person dismisses it.
func (in *inbox) lateOpen(w http.ResponseWriter, _ *http.Request) {
	in.mu.Lock()
	late, latest := in.lateNotice, in.latest
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
	writeJSON(w, http.StatusOK, lateOpenReply{ID: late.id, Title: title, Message: late.notice.message, Command: late.notice.command, Guessed: late.notice.guessed, Reached: late.notice.reached})
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
// dismissed that notice. Another id changes nothing, since a newer
// notice may have replaced it.
func (in *inbox) dismissLateOpen(w http.ResponseWriter, r *http.Request) {
	var dismissed struct {
		ID uint64 `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&dismissed); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send {id}"})
		return
	}
	in.mu.Lock()
	if in.lateNotice != nil && in.lateNotice.id == dismissed.ID {
		in.lateNotice = nil
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

// refusesHelper reports whether the server is a release from before an
// open took a helper: it refuses the field with this exact error.
func refusesHelper(answer reply) bool {
	return answer.status == http.StatusBadRequest && errorOf(answer) == "send {tool, id, editor}"
}

// withoutField is body without the named field; named is false when
// body has no such field.
func withoutField(body []byte, name string) (rest []byte, named bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return body, false
	}
	if _, named = fields[name]; !named {
		return body, false
	}
	delete(fields, name)
	rest, err := json.Marshal(fields)
	if err != nil {
		return body, false
	}
	return rest, true
}
