package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

func (h *conversationsHandler) readMailboxRefs(w http.ResponseWriter, r *http.Request) {
	var raw json.RawMessage
	if err := decodeOneJSON(w, r, maxMailboxRequestBytes, &raw); err != nil {
		writeMailboxError(w, mailboxBodyRefusal(err))
		return
	}
	var req struct {
		conversations.Ref
		Base string `json:"base"`
	}
	if !mailboxKeysAllowed(mailboxFields(raw), "tool", "id", "base") ||
		decodeStrictJSON(bytes.NewReader(raw), &req) != nil {
		writeMailboxError(w, mailbox.NewRefusal("format"))
		return
	}
	if refusal := mailbox.ValidateRef(req.Ref); refusal != nil {
		writeMailboxError(w, refusal)
		return
	}
	row, found := h.scanner.Find(r.Context(), req.Tool, req.ID, h.scanOptions(""))
	if !found || !canUseMailbox(row) {
		writeMailboxError(w, mailbox.NewRefusal("unknown-session"))
		return
	}
	if !filepath.IsAbs(row.Folder) {
		writeMailboxError(w, mailbox.NewRefusal("refs"))
		return
	}
	refs, err := mailbox.ResolveReviewRefs(r.Context(), row.Folder, req.Base, "")
	if err != nil {
		writeMailboxError(w, err)
		return
	}
	writeEditJSON(w, http.StatusOK, refs)
}

type mailboxField struct {
	key string
	raw json.RawMessage
}

// Keep fields in input order so duplicate keys cannot disappear into a map.
func mailboxFields(raw json.RawMessage) []mailboxField {
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return nil
	}
	fields := []mailboxField{}
	for dec.More() {
		token, err := dec.Token()
		key, isString := token.(string)
		if err != nil || !isString {
			return nil
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil
		}
		fields = append(fields, mailboxField{key: key, raw: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil
	}
	return fields
}

func mailboxKeysAllowed(fields []mailboxField, allowed ...string) bool {
	if fields == nil {
		return false
	}
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if seen[field.key] || !slices.Contains(allowed, field.key) {
			return false
		}
		seen[field.key] = true
	}
	return true
}

func mailboxReviewEcho(raw json.RawMessage) bool {
	fields := mailboxFields(raw)
	review := slices.ContainsFunc(fields, func(field mailboxField) bool {
		var kind string
		return strings.EqualFold(field.key, "type") &&
			json.Unmarshal(field.raw, &kind) == nil && kind == mailbox.ReviewRequest
	})
	if !review {
		return false
	}
	if !mailboxKeysAllowed(fields, "from", "to", "type", "thread", "refs", "models") {
		return true
	}
	for _, field := range fields {
		nested := mailboxFields(field.raw)
		if nested == nil {
			continue // the strict decoder handles invalid field types
		}
		switch field.key {
		case "from", "to":
			if !mailboxKeysAllowed(nested, "tool", "id") {
				return true
			}
		case "refs":
			if !mailboxKeysAllowed(nested, "worktree", "base", "commit", "paths") {
				return true
			}
		case "models":
			if !mailboxKeysAllowed(nested, "from", "to") {
				return true
			}
		}
	}
	return false
}
