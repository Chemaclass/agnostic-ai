// Package mailbox stores and stages messages between local conversations.
package mailbox

import (
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

const (
	Question      = "question"
	Finding       = "finding"
	Done          = "done"
	ReviewRequest = "review_request"

	Open    = "open"
	Paused  = "paused"
	Closing = "closing"
	Closed  = "closed"

	Waiting  = "waiting"
	Staged   = "staged"
	Canceled = "cancelled" //nolint:misspell // mailbox/v1 uses this spelling on disk and over HTTP.
)

const schemaVersion = "mailbox/v1"

type SendRequest struct {
	To     conversations.Ref `json:"to"`
	Type   string            `json:"type"`
	Body   string            `json:"body,omitempty"`
	Thread string            `json:"thread,omitempty"`
	Refs   *ReviewInput      `json:"refs,omitempty"`
	Models *ModelInput       `json:"models,omitempty"`
}

type ReviewInput struct {
	Worktree string   `json:"worktree"`
	Base     string   `json:"base"`
	Commit   string   `json:"commit,omitempty"`
	Paths    []string `json:"paths,omitempty"`
}

type ReviewRefs struct {
	Worktree    string   `json:"worktree"`
	Commit      string   `json:"commit"`
	Base        string   `json:"base"`
	Branch      string   `json:"branch,omitempty"`
	Paths       []string `json:"paths"`
	Uncommitted int      `json:"uncommitted"`
}

type ModelInput struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type Model struct {
	Name     string `json:"name"`
	StatedBy string `json:"statedBy"`
}

type Receipt struct {
	Thread string `json:"thread"`
	N      int    `json:"n"`
	State  string `json:"state"`
}

type Actor struct {
	conversations.Ref
	By string `json:"by"`
}

type Message struct {
	N        int               `json:"n"`
	Type     string            `json:"type"`
	From     Actor             `json:"from"`
	To       conversations.Ref `json:"to"`
	Body     string            `json:"body,omitempty"`
	Refs     *ReviewRefs       `json:"refs,omitempty"`
	State    string            `json:"state"`
	At       time.Time         `json:"at"`
	QueuedAt time.Time         `json:"queuedAt"`
}

type Thread struct {
	SchemaVersion string               `json:"schemaVersion"`
	ID            string               `json:"id"`
	Status        string               `json:"status"`
	Cap           int                  `json:"cap"`
	Between       [2]conversations.Ref `json:"between"`
	Models        map[string]Model     `json:"models,omitempty"`
	Messages      []Message            `json:"messages"`
	Refusals      []RefusalRecord      `json:"refusals,omitempty"`
	Events        []Event              `json:"events"`
}

type Event struct {
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	Action string    `json:"action"`
	N      int       `json:"n,omitempty"`
}

type RefusalRecord struct {
	At     time.Time          `json:"at"`
	From   *conversations.Ref `json:"from,omitempty"`
	Type   string             `json:"type,omitempty"`
	Guard  string             `json:"guard"`
	Reason string             `json:"reason"`
}

type ThreadList struct {
	Threads []Thread `json:"threads"`
	Warning string   `json:"warning,omitempty"`
}

type Stage struct {
	Text string
	To   conversations.Ref
}
