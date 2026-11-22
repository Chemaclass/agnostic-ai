package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

const (
	mailboxSendPath     = "/api/mailbox/send"
	mailboxThreadsPath  = "/api/mailbox/threads"
	mailboxThreadPath   = "/api/mailbox/thread"
	mailboxStagePath    = "/api/mailbox/stage"
	mailboxRefsPath     = "/api/mailbox/refs"
	mailboxClaimsPath   = "/api/mailbox/claims"
	mailboxClaimPath    = "/api/mailbox/claim"
	mailboxReleasePath  = "/api/mailbox/release"
	mailboxDismissPath  = "/api/mailbox/conflict/dismiss"
	mailboxHandoffPath  = "/api/mailbox/handoff"
	mailboxApprovePath  = "/api/mailbox/approve"
	mailboxQueuePath    = "/api/mailbox/queue"
	mailboxSettingsPath = "/api/mailbox/settings"
	// JSON escaping can grow a message well past the limit on its text.
	maxMailboxRequestBytes = 64 << 10
)

func (h *conversationsHandler) mailboxStore() (mailbox.Store, bool) {
	if !h.userLevel {
		return mailbox.Store{}, false
	}
	return mailboxStoreAt(h.scanner.MarksPath)
}

func mailboxOriginStatus(r *http.Request) int {
	// A browser origin allowed to read other views cannot read local mail.
	return requestOriginStatus(r.WithContext(context.Background()))
}

func (h *conversationsHandler) serveMailbox(w http.ResponseWriter, r *http.Request) {
	store, ok := h.mailboxStore()
	if !ok {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "Open the companion to use the mailbox."})
		return
	}
	if status := mailboxOriginStatus(r); status != 0 {
		writeEditJSON(w, status, editError{Error: "The mailbox is available from the local server origin only."})
		return
	}
	switch r.URL.Path {
	case mailboxThreadsPath:
		h.readMailboxThreads(w, r, store)
		return
	case mailboxClaimsPath:
		h.readMailboxClaims(w, r, store)
		return
	case mailboxSettingsPath:
		if r.Method == http.MethodGet {
			writeEditJSON(w, http.StatusOK, store.Settings())
			return
		}
	}
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	switch r.URL.Path {
	case mailboxSendPath:
		h.sendMailboxMessage(w, r, store)
	case mailboxThreadPath:
		h.changeMailboxThread(w, r, store)
	case mailboxStagePath:
		h.stageMailboxMessage(w, r, store)
	case mailboxRefsPath:
		h.readMailboxRefs(w, r)
	case mailboxClaimPath, mailboxReleasePath:
		h.changeMailboxClaim(w, r, store)
	case mailboxDismissPath:
		h.dismissMailboxConflict(w, r, store)
	case mailboxHandoffPath:
		h.answerMailboxHandoff(w, r, store)
	case mailboxApprovePath:
		approveMailboxMessage(w, r, store)
	case mailboxQueuePath:
		queueMailboxMessage(w, r, store)
	case mailboxSettingsPath:
		saveMailboxSettings(w, r, store)
	}
}

func writeMailboxError(w http.ResponseWriter, err error) {
	if refusal, ok := errors.AsType[*mailbox.Refusal](err); ok {
		writeEditJSON(w, http.StatusUnprocessableEntity, struct {
			Error string `json:"error"`
			Guard string `json:"guard"`
		}{Error: refusal.Error(), Guard: refusal.Guard})
		return
	}
	if errors.Is(err, mailbox.ErrBusy) {
		writeEditJSON(w, http.StatusConflict, editError{Error: "The mailbox is busy. Wait for the current action to finish and try again."})
		return
	}
	writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
}

func mailboxBodyRefusal(err error) *mailbox.Refusal {
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		return mailbox.NewRefusal("size")
	}
	return mailbox.NewRefusal("format")
}

type mailboxSendRequest struct {
	From conversations.Ref `json:"from"`
	mailbox.SendRequest
}

func (h *conversationsHandler) sendMailboxMessage(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	ctx := r.Context()
	refuse := func(req mailboxSendRequest, refusal *mailbox.Refusal) {
		if err := store.RecordRefusal(ctx, req.From, req.SendRequest, refusal); err != nil {
			writeMailboxError(w, err)
			return
		}
		writeMailboxError(w, refusal)
	}

	var raw json.RawMessage
	if err := decodeOneJSON(w, r, maxMailboxRequestBytes, &raw); err != nil {
		refuse(mailboxSendRequest{}, mailboxBodyRefusal(err))
		return
	}
	fields := mailboxFields(raw)
	if mailboxReviewEcho(fields) {
		var audit struct {
			From   conversations.Ref `json:"from"`
			To     conversations.Ref `json:"to"`
			Thread string            `json:"thread"`
		}
		_ = json.Unmarshal(raw, &audit)
		refuse(mailboxSendRequest{
			From:        audit.From,
			SendRequest: mailbox.SendRequest{To: audit.To, Type: mailbox.ReviewRequest, Thread: audit.Thread},
		}, mailbox.NewRefusal("echo"))
		return
	}
	var req mailboxSendRequest
	if err := decodeStrictJSON(bytes.NewReader(raw), &req); err != nil || malformedMailboxHandoff(fields) {
		refuse(mailboxSendRequest{}, mailboxBodyRefusal(err))
		return
	}
	if refusal := mailbox.ValidateSend(req.From, req.SendRequest); refusal != nil {
		refuse(req, refusal)
		return
	}
	source, to, refusal := mailboxParticipants(ctx, h.scanner, h.scanOptions(""), req.From, req.To, req.Thread)
	if refusal != nil {
		refuse(req, refusal)
		return
	}
	req.To = to
	if req.Type == mailbox.ReviewRequest && !filepath.IsAbs(source.Folder) {
		refuse(req, mailbox.NewRefusal("refs"))
		return
	}
	receipt, err := store.SendFromWorktree(ctx, req.From, req.SendRequest, source.Folder)
	if err != nil {
		writeMailboxError(w, err)
		return
	}
	writeEditJSON(w, http.StatusOK, receipt)
}

// mailboxParticipants is the unknown-session guard the server and the
// command line share: both sessions must be local terminal sessions the
// scanner lists. A bare target ID resolves to the one tool that has it.
func mailboxParticipants(ctx context.Context, scanner *conversations.Scanner, opts conversations.ScanOptions, from, to conversations.Ref, thread string) (conversations.Conversation, conversations.Ref, *mailbox.Refusal) {
	candidates := []conversations.Ref{to}
	if to.Tool == "" {
		candidates = []conversations.Ref{{Tool: conversations.ToolClaude, ID: to.ID}, {Tool: conversations.ToolCodex, ID: to.ID}}
	}
	found := scanner.FindRefs(ctx, opts, append(candidates, from)...)
	source := found[from]
	if !canUseMailbox(source) || thread == "" && source.Parent != nil {
		return conversations.Conversation{}, conversations.Ref{}, mailbox.NewRefusal("unknown-session")
	}
	var destination conversations.Ref
	for _, candidate := range candidates {
		if !canUseMailbox(found[candidate]) {
			continue
		}
		if destination.ID != "" {
			refusal := mailbox.NewRefusal("unknown-session")
			refusal.Reason = "Two local sessions use that ID."
			refusal.Next = "Choose a session row to specify its tool."
			return conversations.Conversation{}, conversations.Ref{}, refusal
		}
		destination = candidate
	}
	if destination.ID == "" {
		return conversations.Conversation{}, conversations.Ref{}, mailbox.NewRefusal("unknown-session")
	}
	return source, destination, nil
}

// claimOwnerGone reports whether a claim's owner is known to be gone:
// its row cannot use the mailbox, or a complete read of its tool did not
// list its files. An owner whose store could not be read in full, or
// whose files were listed without a row, is not gone.
func claimOwnerGone(read conversations.RefsRead, owner conversations.Ref) bool {
	if row, found := read.Rows[owner]; found {
		return !canUseMailbox(row)
	}
	return read.Complete[owner.Tool] && !read.Listed[owner]
}

func canUseMailbox(c conversations.Conversation) bool {
	return (c.Tool == conversations.ToolClaude || c.Tool == conversations.ToolCodex) &&
		c.App != conversations.AppCloud && c.App != conversations.AppChat && c.App != conversations.AppCowork
}

func (h *conversationsHandler) readMailboxThreads(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	if !allowGetOnly(w, r) {
		return
	}
	query := r.URL.Query()
	ref := conversations.Ref{Tool: conversations.Tool(query.Get("tool")), ID: query.Get("id")}
	if refusal := mailbox.ValidateRef(ref); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	list, err := store.ThreadsOf(r.Context(), ref)
	if err != nil {
		writeMailboxError(w, err)
		return
	}
	writeEditJSON(w, http.StatusOK, list)
}

func (h *conversationsHandler) changeMailboxThread(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	var req struct {
		Thread string `json:"thread"`
		Action string `json:"action"`
	}
	if err := decodeOneJSON(w, r, maxMailboxRequestBytes, &req); err != nil {
		writeMailboxError(w, mailboxBodyRefusal(err))
		return
	}
	if err := store.Change(r.Context(), req.Thread, req.Action); err != nil {
		writeMailboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mailboxStageRequest struct {
	Thread  string `json:"thread"`
	N       int    `json:"n"`
	How     string `json:"how"`
	Confirm bool   `json:"confirm"`
}

type mailboxStageReply struct {
	Staged  bool   `json:"staged"`
	Typed   bool   `json:"typed,omitempty"`
	Message string `json:"message,omitempty"`
	Text    string `json:"text,omitempty"`
}

func (h *conversationsHandler) stageMailboxMessage(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	ctx := r.Context()
	var req mailboxStageRequest
	if err := decodeOneJSON(w, r, maxMailboxRequestBytes, &req); err != nil {
		writeMailboxError(w, mailboxBodyRefusal(err))
		return
	}
	if req.N < 1 || req.How != "copy" && (req.How != "type" || req.Confirm) {
		writeMailboxError(w, mailbox.NewRefusal("format"))
		return
	}
	if req.Confirm {
		h.confirmMailboxStage(w, ctx, store, req, "Message copied to the clipboard.")
		return
	}
	stage, err := store.PrepareStage(ctx, req.Thread, req.N)
	if err != nil {
		writeMailboxError(w, err)
		return
	}
	if req.How == "copy" {
		writeEditJSON(w, http.StatusOK, mailboxStageReply{Text: stage.Text})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, h.openReplyWait)
	defer cancel()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(h.openReplyWait))
	recipient, found := h.scanner.Find(ctx, stage.To.Tool, stage.To.ID, h.scanOptions(""))
	if !found || !canUseMailbox(recipient) || !recipient.Live {
		writeEditJSON(w, http.StatusOK, mailboxStageReply{Message: "The recipient has no live terminal here. Copy the message instead."})
		return
	}
	var reply conversations.SendReply
	typed, err := store.TypeStage(ctx, req.Thread, req.N, func(stage mailbox.Stage) bool {
		reply = conversations.SendComposed(ctx, recipient, stage.Text, h.scanner.Processes, h.launcher)
		return reply.Sent
	})
	if err != nil {
		if typed {
			writeEditJSON(w, http.StatusOK, mailboxStageReply{Typed: true, Message: "The text was typed, but the mailbox could not save the result. Check that session before trying again."})
			return
		}
		writeMailboxError(w, err)
		return
	}
	writeEditJSON(w, http.StatusOK, mailboxStageReply{Staged: typed, Message: reply.Message})
}

func (h *conversationsHandler) confirmMailboxStage(w http.ResponseWriter, ctx context.Context, store mailbox.Store, req mailboxStageRequest, message string) {
	if err := store.ConfirmStage(ctx, req.Thread, req.N, req.How); err != nil {
		writeMailboxError(w, err)
		return
	}
	writeEditJSON(w, http.StatusOK, mailboxStageReply{Staged: true, Message: message})
}

func (h *conversationsHandler) addMailboxSummaries(ctx context.Context, store mailbox.Store, view *conversations.View, opts conversations.ScanOptions) {
	list, claims, err := store.Snapshot(ctx)
	if err != nil {
		view.MailboxWarning = "The mailbox could not be read. Try again."
	} else {
		view.MailboxWarning = list.Warning
	}
	page := make(map[conversations.Ref]int, len(view.Conversations))
	for i, row := range view.Conversations {
		if canUseMailbox(row) {
			page[conversations.Ref{Tool: row.Tool, ID: row.ID}] = i
		}
	}
	var offPage []conversations.Ref
	totals := conversations.MailboxClaims{Conflicts: len(claims.Conflicts)}
	for _, claim := range claims.Claims {
		totals.Paths += len(claim.Paths)
		if at, visible := page[claim.Owner]; visible {
			view.Conversations[at].ClaimCount++
			view.Conversations[at].ClaimedPaths += len(claim.Paths)
		} else {
			offPage = append(offPage, claim.Owner)
		}
	}
	if len(claims.Claims) > 0 || totals.Conflicts > 0 {
		view.MailboxClaims = &totals
	}
	conflicts := make(map[string][]conversations.MailboxConflict)
	for _, conflict := range claims.Conflicts {
		if conflict.Thread != "" {
			conflicts[conflict.Thread] = append(conflicts[conflict.Thread], conversations.MailboxConflict(conflict))
		}
	}
	var missing []conversations.Ref
	for _, thread := range list.Threads {
		for i, participant := range thread.Between {
			at, visible := page[participant]
			if !visible {
				continue
			}
			summary := mailboxSummary(thread, thread.Between[1-i])
			summary.Conflicts = conflicts[thread.ID]
			if peer, visible := page[summary.With]; visible {
				summary.WithTitle = view.Conversations[peer].Title
			} else {
				missing = append(missing, summary.With)
			}
			view.Conversations[at].Threads = append(view.Conversations[at].Threads, summary)
		}
	}
	if len(missing) == 0 && len(offPage) == 0 {
		return
	}
	read, recent := h.scanner.RecentRefsRead(opts, append(missing, offPage...)...)
	peers := read.Rows
	// Without a recent read nothing is known, so no claim counts as stale.
	if recent {
		for _, owner := range offPage {
			if claimOwnerGone(read, owner) {
				view.MailboxClaims.Stale++
			}
		}
	}
	for i := range view.Conversations {
		for j := range view.Conversations[i].Threads {
			summary := &view.Conversations[i].Threads[j]
			if peer, found := peers[summary.With]; summary.WithTitle == "" && found && canUseMailbox(peer) {
				summary.WithTitle = peer.Title
			}
		}
	}
}

func mailboxSummary(thread mailbox.Thread, with conversations.Ref) conversations.ThreadSummary {
	summary := conversations.ThreadSummary{ID: thread.ID, With: with, Status: thread.Status, Cap: thread.Cap}
	for _, message := range thread.Messages {
		summary.LastType = message.Type
		if message.Type != "done" && message.State != "rejected" {
			summary.Count++
		}
		switch message.State {
		case mailbox.Waiting:
			summary.Waiting++
		case mailbox.Held:
			summary.Held++
		}
	}
	return summary
}

func approveMailboxMessage(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	var req struct {
		Thread string `json:"thread"`
		N      int    `json:"n"`
		Action string `json:"action"`
	}
	if refusal := readMailboxFields(w, r, &req, "thread", "n", "action"); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	writeMailboxChange(w, store.Approve(r.Context(), req.Thread, req.N, req.Action))
}

func queueMailboxMessage(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	var req struct {
		Thread string `json:"thread"`
		N      int    `json:"n"`
	}
	if refusal := readMailboxFields(w, r, &req, "thread", "n"); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	writeMailboxChange(w, store.Queue(r.Context(), req.Thread, req.N))
}

func saveMailboxSettings(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	var req struct {
		ApproveAgentSends *bool `json:"approveAgentSends"`
	}
	if refusal := readMailboxFields(w, r, &req, "approveAgentSends"); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	if req.ApproveAgentSends == nil {
		writeMailboxError(w, mailbox.NewRefusal("format"))
		return
	}
	writeMailboxChange(w, store.SaveSettings(r.Context(), mailbox.Settings{ApproveAgentSends: *req.ApproveAgentSends}))
}

func writeMailboxChange(w http.ResponseWriter, err error) {
	if err != nil {
		writeMailboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
