package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

type mailboxClaimRequest struct {
	conversations.Ref
	mailbox.Gives
}

func (h *conversationsHandler) readMailboxClaims(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	if !allowGetOnly(w, r) {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 0 && (len(query) != 2 || len(query["tool"]) != 1 || len(query["id"]) != 1) {
		writeMailboxError(w, mailbox.NewRefusal("format"))
		return
	}
	ctx := r.Context()
	ref := conversations.Ref{Tool: conversations.Tool(query.Get("tool")), ID: query.Get("id")}
	var view mailbox.ClaimView
	if len(query) == 0 {
		view, err = store.Claims(ctx)
	} else {
		if refusal := mailbox.ValidateRef(ref); refusal != nil {
			writeMailboxError(w, refusal)
			return
		}
		view, err = store.ClaimsOf(ctx, ref)
	}
	if err != nil {
		writeMailboxError(w, err)
		return
	}

	owners := make([]conversations.Ref, 0, len(view.Claims)+1)
	if ref.ID != "" {
		owners = append(owners, ref)
	}
	for _, claim := range view.Claims {
		owners = append(owners, claim.Owner)
	}
	var found map[conversations.Ref]conversations.Conversation
	if len(owners) > 0 {
		found = h.scanner.FindRefs(ctx, h.scanOptions(""), owners...)
	}
	for i := range view.Claims {
		claim := &view.Claims[i]
		claim.Stale = !canUseMailbox(found[claim.Owner])
	}
	worktree := ""
	if row := found[ref]; canUseMailbox(row) {
		worktree, _ = mailbox.ResolveClaimWorktree(ctx, row.Folder)
	}
	writeEditJSON(w, http.StatusOK, struct {
		mailbox.ClaimView
		Worktree string `json:"worktree,omitempty"`
	}{view, worktree})
}

func (h *conversationsHandler) changeMailboxClaim(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	var req mailboxClaimRequest
	if refusal := readMailboxFields(w, r, &req, "tool", "id", "worktree", "paths"); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	if refusal := mailbox.ValidateRef(req.Ref); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	ctx := r.Context()
	var err error
	if r.URL.Path == mailboxReleasePath {
		err = store.Release(ctx, req.Ref, req.Gives)
	} else {
		row, found := h.scanner.Find(ctx, req.Tool, req.ID, h.scanOptions(""))
		if !found || !canUseMailbox(row) {
			writeMailboxError(w, mailbox.NewRefusal("unknown-session"))
			return
		}
		err = store.Claim(ctx, req.Ref, req.Gives, row.Folder, "human")
	}
	if err != nil {
		writeMailboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *conversationsHandler) answerMailboxHandoff(w http.ResponseWriter, r *http.Request, store mailbox.Store) {
	var req struct {
		Thread string `json:"thread"`
		N      int    `json:"n"`
		Action string `json:"action"`
	}
	if refusal := readMailboxFields(w, r, &req, "thread", "n", "action"); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	if req.N < 1 || req.Action != "accept" && req.Action != "decline" {
		writeMailboxError(w, mailbox.NewRefusal("format"))
		return
	}
	if err := store.AnswerHandoff(r.Context(), req.Thread, req.N, req.Action); err != nil {
		writeMailboxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func readMailboxFields(w http.ResponseWriter, r *http.Request, dst any, keys ...string) *mailbox.Refusal {
	var raw json.RawMessage
	if err := decodeOneJSON(w, r, maxMailboxRequestBytes, &raw); err != nil {
		return mailboxBodyRefusal(err)
	}
	if !mailboxKeysAllowed(mailboxFields(raw), keys...) || decodeStrictJSON(bytes.NewReader(raw), dst) != nil {
		return mailbox.NewRefusal("format")
	}
	return nil
}

func malformedMailboxHandoff(fields []mailboxField) bool {
	if !hasMailboxKind(fields, mailbox.Handoff) {
		return false
	}
	if !mailboxKeysAllowed(fields, "from", "to", "type", "body", "thread", "gives") {
		return true
	}
	for _, field := range fields {
		nested := mailboxFields(field.raw)
		if nested == nil {
			continue
		}
		switch field.key {
		case "from", "to":
			if !mailboxKeysAllowed(nested, "tool", "id") {
				return true
			}
		case "gives":
			if !mailboxKeysAllowed(nested, "worktree", "paths") {
				return true
			}
		}
	}
	return false
}
