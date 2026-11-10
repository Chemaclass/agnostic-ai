package conversations

import (
	"context"
	"time"
)

// CloudSessions lists the conversations that run in a tool's cloud
// rather than on this machine. Each call asks the cloud once; the
// Scanner decides how often that happens.
type CloudSessions interface {
	// List returns the cloud conversations, or a *CloudError that says
	// why they could not be read. A person who does not use the tool
	// has none, which is no error.
	List(ctx context.Context) ([]Conversation, error)
}

// CloudReason is why cloud sessions could not be listed. The list is
// closed: each reason has one sentence the person reads.
type CloudReason string

const (
	// CloudUnsupported: this system has no known place for the sign-in.
	CloudUnsupported CloudReason = "unsupported"
	// CloudNoCredential: no sign-in that may list cloud sessions was found.
	CloudNoCredential CloudReason = "no-credential"
	// CloudSignedOut: the sign-in expired, or the cloud no longer accepts it.
	CloudSignedOut CloudReason = "signed-out"
	// CloudRefused: the cloud answered, but not with the list.
	CloudRefused CloudReason = "refused"
	// CloudUnreachable: no answer in time.
	CloudUnreachable CloudReason = "unreachable"
	// CloudUnreadable: an answer that is not the list we know.
	CloudUnreadable CloudReason = "unreadable"
)

// cloudReasonTexts is what the person reads for each tool and reason.
// The provider's own words stay in CloudError.Cause.
var cloudReasonTexts = map[Tool]map[CloudReason]string{
	ToolClaude: {
		CloudUnsupported:  "this system has no supported place to read the Claude sign-in from; only macOS is supported.",
		CloudNoCredential: "no Claude sign-in that may list cloud sessions was found on this machine. Sign in with `claude` first.",
		CloudSignedOut:    "the Claude sign-in has expired or was refused. Run `claude` once to renew it.",
		CloudRefused:      "claude.ai refused to list them.",
		CloudUnreachable:  "claude.ai did not answer in time.",
		CloudUnreadable:   "claude.ai answered with something this version cannot read.",
	},
	ToolCodex: {
		CloudNoCredential: "the Codex sign-in on this machine is not a ChatGPT login. Sign in with `codex login` first.",
		CloudSignedOut:    "the Codex sign-in has expired or was refused. Run `codex` once to renew it.",
		CloudRefused:      "the Codex cloud refused to list them.",
		CloudUnreachable:  "the Codex cloud did not answer in time.",
		CloudUnreadable:   "the Codex cloud answered with something this version cannot read.",
	},
}

// cloudNotePrefixes start what View.CloudNote says about each tool.
var cloudNotePrefixes = map[Tool]string{
	ToolClaude: "Claude Code cloud sessions are not listed: ",
	ToolCodex:  "Codex cloud threads are not listed: ",
}

// CloudError is a failed cloud read. Error is the sentence the person
// reads; Cause keeps what the provider or the system said, for logs.
// Neither ever holds a credential.
type CloudError struct {
	// Tool is whose cloud could not be read.
	Tool   Tool
	Reason CloudReason
	Cause  error
}

func (e *CloudError) Error() string {
	text, known := cloudReasonTexts[e.Tool][e.Reason]
	if !known {
		return "an unknown problem."
	}
	return text
}

func (e *CloudError) Unwrap() error { return e.Cause }

// cloudNote is what View.CloudNote says about a failed read of tool's
// cloud. A failure that is not a *CloudError is a programming slip in
// a CloudSessions; its text is not shown, since it may hold the
// provider's words.
func cloudNote(tool Tool, err error) string {
	cloudErr, ok := err.(*CloudError)
	if !ok {
		return cloudNotePrefixes[tool] + "an unknown problem."
	}
	return cloudNotePrefixes[tool] + cloudErr.Error()
}

// keptCloud is the last cloud read and when it ran.
type keptCloud struct {
	at   time.Time
	rows []Conversation
	err  error
}

// cloudRows returns tool's cloud conversations and, when they could
// not be read, the note that says why. Each cloud is asked at most
// once per freshSpan: a success or a failure stands until the span
// ends, so a failure never shows an older list as current. A project
// server lists only conversations whose folder is in the project, and
// cloud sessions have none, so it never asks.
func (s *Scanner) cloudRows(ctx context.Context, opts ScanOptions, tool Tool, cloud CloudSessions) ([]Conversation, string) {
	if cloud == nil || len(opts.Within) > 0 {
		return nil, ""
	}
	now := s.Now()
	last := s.keptClouds[tool]
	if last == nil || spanOf(now) != spanOf(last.at) {
		rows, err := cloud.List(ctx)
		// A canceled read stopped early; keeping it would show its gap until the span ends.
		if ctx.Err() != nil {
			return nil, ""
		}
		last = &keptCloud{at: now, rows: rows, err: err}
		if s.keptClouds == nil {
			s.keptClouds = map[Tool]*keptCloud{}
		}
		s.keptClouds[tool] = last
	}
	if last.err != nil {
		return nil, cloudNote(tool, last.err)
	}
	// Each scan fills and marks its own copies.
	rows := make([]Conversation, 0, len(last.rows))
	for _, c := range last.rows {
		if c.Live && now.Sub(lastWritten(c)) >= cloudLiveSpan {
			c.Live, c.Activity = false, ""
		}
		s.fill(&c, texts{title: c.Title})
		rows = append(rows, c)
	}
	return rows, ""
}

// cloudLiveSpan is how long a cloud's word that a conversation works or
// waits is believed after its last write. A cloud agent writes as it
// works, and no process on this machine can be checked for it, so one
// that wrote nothing for a day is taken as stopped whatever its status
// says: a failed session can keep a working status for weeks.
const cloudLiveSpan = activeSpan
