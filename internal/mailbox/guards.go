package mailbox

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const (
	maxBodyRunes       = 4000
	maxLineBytes       = 5000
	threadCap          = 8
	maxReviewPaths     = 200
	maxReviewPathBytes = 400
	maxModelNameBytes  = 400
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
	case "echo":
		return &Refusal{guard, "A review request carries Git refs only.", "Remove the text, verdict, or other unsupported fields before sending."}
	case "refs":
		return &Refusal{guard, "The review's Git refs could not be verified.", "Refresh the review details and choose an existing base and commit."}
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
	if req.Type == ReviewRequest && req.Body != "" {
		return NewRefusal("echo")
	}
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
	if req.Type != ReviewRequest && (req.Refs != nil || req.Models != nil) {
		return NewRefusal("format")
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
	case ReviewRequest:
		return validateReviewInput(req.Refs, req.Models)
	default:
		return &Refusal{"format", "This mailbox accepts questions, findings, review requests and done messages.", "Choose a supported message type."}
	}
	return nil
}

func validateReviewInput(input *ReviewInput, models *ModelInput) *Refusal {
	if input == nil {
		return NewRefusal("refs")
	}
	if refused := checkReviewStrings(input.Worktree, input.Base, input.Commit); refused != nil {
		return refused
	}
	if refused := validateReviewPaths(input.Paths); refused != nil {
		return refused
	}
	if models != nil {
		if refused := checkReviewStrings(models.From, models.To); refused != nil {
			return refused
		}
		if len(models.From) > maxModelNameBytes || len(models.To) > maxModelNameBytes {
			return &Refusal{"size", "A model name exceeds 400 bytes.", "Shorten the model name or leave it blank."}
		}
	}
	if input.Base == "" {
		return missingBase()
	}
	if !filepath.IsAbs(input.Worktree) {
		return NewRefusal("refs")
	}
	return nil
}

func validateReviewPaths(paths []string) *Refusal {
	if len(paths) > maxReviewPaths {
		return reviewSizeRefusal()
	}
	for _, candidate := range paths {
		if len(candidate) > maxReviewPathBytes {
			return reviewSizeRefusal()
		}
		if refused := checkReviewStrings(candidate); refused != nil {
			return refused
		}
		if !filepath.IsLocal(candidate) || candidate == "." || path.Clean(candidate) != candidate {
			return NewRefusal("refs")
		}
	}
	return nil
}

func checkReviewStrings(values ...string) *Refusal {
	for _, value := range values {
		if !utf8.ValidString(value) {
			return &Refusal{"refs", "A review field cannot be represented as UTF-8.", "Use paths and names that can be written as Unicode text."}
		}
	}
	return checkStrings(values...)
}

func reviewSizeRefusal() *Refusal {
	return &Refusal{"size", "The review exceeds 200 paths, 400 bytes per path, or 5,000 encoded bytes.", "Split the changes into smaller reviews."}
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
	case Question, Finding, Done, ReviewRequest, "handoff":
		return kind
	default:
		return ""
	}
}
