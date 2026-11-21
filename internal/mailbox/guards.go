package mailbox

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const (
	maxBodyRunes = 4000
	maxLineBytes = 5000
	threadCap    = 8
)

var threadID = regexp.MustCompile(`^th_[0-9]{8}_[0-9a-f]{8}$`)

type Refusal struct {
	Guard  string
	Reason string
	Next   string
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("Not sent (guard: %s). %s\n%s", r.Guard, r.Reason, r.Next)
}

func NewRefusal(guard string) *Refusal {
	switch guard {
	case "unknown-session":
		return &Refusal{guard, "The mailbox does not know that local session.", "Pick a local Claude Code or Codex session or check its ID."}
	case "not-in-thread":
		return &Refusal{guard, "Those sessions are not the two participants in this thread.", "Open a thread for these sessions or start a new one."}
	case "secret":
		return &Refusal{guard, "A field looks like it contains a credential.", "Remove the credential before sending."}
	case "size":
		return &Refusal{guard, "The message exceeds 4,000 characters or 5,000 encoded bytes.", "Shorten the message and try again."}
	case "cap":
		return &Refusal{guard, "This thread reached its message limit.", "Close the thread or allow 8 more messages in the companion."}
	case "paused":
		return &Refusal{guard, "This thread is paused.", "Resume the thread in the companion before trying again."}
	case "closed":
		return &Refusal{guard, "This thread is closing or closed.", "Start a new thread to send another message."}
	default:
		return &Refusal{"format", "The mailbox could not read the supplied fields.", "Check the message type and fields and try again."}
	}
}

func ValidateRef(ref conversations.Ref) *Refusal {
	if refused := checkStrings(string(ref.Tool), ref.ID); refused != nil {
		return refused
	}
	if !supportedRef(ref) {
		return NewRefusal("unknown-session")
	}
	return nil
}

func supportedRef(ref conversations.Ref) bool {
	return (ref.Tool == conversations.ToolClaude || ref.Tool == conversations.ToolCodex) && conversations.ValidSessionRef(ref)
}

// ValidateSend allows a bare target ID so the server can resolve the tool
// from its session list before sending.
func ValidateSend(from conversations.Ref, req SendRequest) *Refusal {
	if refused := checkStrings(string(from.Tool), from.ID, string(req.To.Tool), req.To.ID, req.Type, req.Thread, req.Body); refused != nil {
		return refused
	}
	if !supportedRef(from) || !validTarget(req.To) {
		return NewRefusal("unknown-session")
	}
	if from == req.To {
		return &Refusal{"format", "The sender and recipient are the same session.", "Pick a different session as the recipient."}
	}
	if req.Thread != "" && !threadID.MatchString(req.Thread) {
		return NewRefusal("format")
	}
	if utf8.RuneCountInString(req.Body) > maxBodyRunes {
		return NewRefusal("size")
	}
	switch req.Type {
	case Question, Finding:
		body := conversations.CleanSendText(req.Body)
		if strings.TrimSpace(body) == "" {
			return &Refusal{"format", "A question or finding needs a message.", "Write a message and try again."}
		}
		if body != req.Body && preview.ProseSensitive(body) {
			return NewRefusal("secret")
		}
	case Done:
		if req.Thread == "" || req.Body != "" {
			return &Refusal{"format", "A done message needs an existing thread and no body.", "Use Close thread on the thread you want to finish."}
		}
	default:
		return &Refusal{"format", "This mailbox accepts questions, findings and done messages.", "Choose a supported message type."}
	}
	return nil
}

func validTarget(ref conversations.Ref) bool {
	if ref.Tool != "" {
		return supportedRef(ref)
	}
	return supportedRef(conversations.Ref{Tool: conversations.ToolClaude, ID: ref.ID}) ||
		supportedRef(conversations.Ref{Tool: conversations.ToolCodex, ID: ref.ID})
}

func checkStrings(values ...string) *Refusal {
	for _, value := range values {
		if len(value) > maxBodyRunes*utf8.UTFMax {
			return NewRefusal("size")
		}
		if value != "" && preview.ProseSensitive(value) {
			return NewRefusal("secret")
		}
	}
	return nil
}

func knownTool(tool conversations.Tool) conversations.Tool {
	switch tool {
	case conversations.ToolClaude, conversations.ToolCodex:
		return tool
	default:
		return ""
	}
}

func knownType(kind string) string {
	switch kind {
	case Question, Finding, Done, "review_request", "handoff":
		return kind
	default:
		return ""
	}
}
