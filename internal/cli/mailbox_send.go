package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

// mailboxAgentRefs takes the output of `mailbox refs` as it is printed.
// The store resolves the branch and the uncommitted count again.
type mailboxAgentRefs struct {
	mailbox.ReviewInput
	Branch      string `json:"branch"`
	Uncommitted int    `json:"uncommitted"`
}

type mailboxAgentPayload struct {
	To     string            `json:"to"`
	Thread string            `json:"thread"`
	Type   string            `json:"type"`
	Body   string            `json:"body"`
	Model  string            `json:"model"`
	Refs   *mailboxAgentRefs `json:"refs"`
	Gives  *mailbox.Gives    `json:"gives"`
}

func newMailboxSendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send",
		Short: "Send a mailbox message from this session",
		Long: `Send one message from the session this runs in. Pass one JSON object on standard input with "to" (tool:id) or "thread", and "type": question, finding, review_request, handoff or done. Add "body", "refs" (from mailbox refs), "gives" and "model" as the type needs. The mailbox sets everything else.

For a handoff, a receiver is enough. The mailbox resolves the sender's Git worktree, branch, commit and handoff note. Add "gives" only when offering claimed paths.

The sender is the session this command detects, which is context, not proof of identity.`,
		Example: `  agnostic-ai mailbox send <<'MSG'
  {"to": "claude:4e8b1c2d-9a3f-4b6e-8c7d-1f2a3b4c5d6e", "type": "question", "body": "Does the store lock cover the claims file too?"}
  MSG
  agnostic-ai mailbox send <<'MSG'
  {"thread": "th_20261121_7f3a9c10", "type": "finding", "body": "Yes. One lock covers the whole folder."}
  MSG
  agnostic-ai mailbox send <<'MSG'
  {"to": "codex:4e8b1c2d-9a3f-4b6e-8c7d-1f2a3b4c5d6e", "type": "handoff"}
  MSG`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
				raw, err := readMailboxPayload(cmd.InOrStdin())
				if err != nil {
					return err
				}
				req, refused := parseMailboxPayload(ctx, agent, raw)
				if refused != nil {
					return agent.refuse(ctx, req, refused)
				}
				return agent.send(ctx, cmd.OutOrStdout(), req)
			})
		},
	}
}

func newMailboxDoneCmd() *cobra.Command {
	var thread string
	cmd := &cobra.Command{
		Use:     "done",
		Short:   "Close a mailbox thread this session is in",
		Long:    "Send a done message that closes the thread once its accepted messages are delivered. The sender is the session this command detects.",
		Example: "  agnostic-ai mailbox done --thread th_20261121_7f3a9c10",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
				req := mailbox.SendRequest{Type: mailbox.Done, Thread: thread}
				to, err := agent.store.Peer(ctx, thread, agent.me)
				if refused, ok := errors.AsType[*mailbox.Refusal](err); ok {
					return agent.refuse(ctx, req, refused)
				}
				if err != nil {
					return err
				}
				req.To = to
				return agent.send(ctx, cmd.OutOrStdout(), req)
			})
		},
	}
	cmd.Flags().StringVar(&thread, "thread", "", "thread to close")
	_ = cmd.MarkFlagRequired("thread")
	return cmd
}

func readMailboxPayload(in io.Reader) ([]byte, error) {
	if stdinIsTerminal(in) {
		return nil, errors.New("no message: pass one JSON object on standard input")
	}
	raw, err := io.ReadAll(io.LimitReader(in, maxMailboxRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the message: %w", err)
	}
	return raw, nil
}

func parseMailboxPayload(ctx context.Context, agent mailboxAgent, raw []byte) (mailbox.SendRequest, *mailbox.Refusal) {
	if len(raw) > maxMailboxRequestBytes {
		return mailbox.SendRequest{}, mailbox.NewRefusal("size")
	}
	fields := mailboxFields(raw)
	if !mailboxKeysAllowed(fields, "to", "thread", "type", "body", "refs", "gives", "model") {
		return mailbox.SendRequest{}, &mailbox.Refusal{
			Guard: "format", Reason: "A message takes one JSON object with to or thread, type, body, refs, gives and model, each once.",
			Next: "Remove the other fields. The mailbox sets the sender, state, number and times.",
		}
	}
	if mailboxAgentEcho(fields) {
		return mailbox.SendRequest{Type: mailbox.ReviewRequest}, mailbox.NewRefusal("echo")
	}
	var payload mailboxAgentPayload
	if err := decodeStrictJSON(bytes.NewReader(raw), &payload); err != nil || malformedMailboxGives(fields) {
		return mailbox.SendRequest{}, mailbox.NewRefusal("format")
	}
	req := mailbox.SendRequest{Type: payload.Type, Body: payload.Body, Thread: payload.Thread, Gives: payload.Gives, Model: payload.Model}
	if payload.Refs != nil {
		req.Refs = &payload.Refs.ReviewInput
	}
	if (payload.To == "") == (payload.Thread == "") {
		return req, &mailbox.Refusal{Guard: "format", Reason: "A message names its reader with to, or its thread with thread, and not both.", Next: "Use to for a new thread and thread for a reply."}
	}
	if payload.Thread != "" {
		to, err := agent.store.Peer(ctx, payload.Thread, agent.me)
		if err != nil {
			if refused, ok := errors.AsType[*mailbox.Refusal](err); ok {
				return req, refused
			}
			return req, &mailbox.Refusal{Guard: "format", Reason: "The mailbox could not read that thread.", Next: "Check the thread ID with mailbox list."}
		}
		req.To = to
		return req, nil
	}
	tool, id, ok := strings.Cut(payload.To, ":")
	if !ok {
		return req, &mailbox.Refusal{Guard: "format", Reason: "The reader is written as tool:id, such as codex:<session ID>.", Next: "Name the reader's tool and session ID."}
	}
	req.To = conversations.Ref{Tool: conversations.Tool(tool), ID: id}
	return req, nil
}

// mailboxAgentEcho refuses text or unknown keys on a review request, which carries Git refs only.
func mailboxAgentEcho(fields []mailboxField) bool {
	if !hasMailboxKind(fields, mailbox.ReviewRequest) {
		return false
	}
	for _, field := range fields {
		switch field.key {
		case "body":
			return true
		case "refs":
			if nested := mailboxFields(field.raw); nested != nil &&
				!mailboxKeysAllowed(nested, "worktree", "base", "commit", "paths", "branch", "uncommitted") {
				return true
			}
		}
	}
	return false
}

func malformedMailboxGives(fields []mailboxField) bool {
	for _, field := range fields {
		if field.key == "gives" {
			if nested := mailboxFields(field.raw); nested != nil && !mailboxKeysAllowed(nested, "worktree", "paths") {
				return true
			}
		}
	}
	return false
}

// refuse stores the refusal as the server does, without any text from the request.
func (a mailboxAgent) refuse(ctx context.Context, req mailbox.SendRequest, refused *mailbox.Refusal) error {
	return errors.Join(refused, a.store.RecordRefusal(ctx, a.me, req, refused))
}

func (a mailboxAgent) send(ctx context.Context, out io.Writer, req mailbox.SendRequest) error {
	if refused := mailbox.ValidateSend(a.me, req); refused != nil {
		return a.refuse(ctx, req, refused)
	}
	_, to, refused := mailboxParticipants(ctx, a.scanner, conversations.ScanOptions{}, a.me, req.To, req.Thread)
	if refused != nil {
		return a.refuse(ctx, req, refused)
	}
	req.To = to
	if req.Type == mailbox.Handoff {
		resume, refused := handoffResume(ctx, a.folder)
		if refused != nil {
			return a.refuse(ctx, req, refused)
		}
		req.Resume = resume
	}
	receipt, err := a.store.SendFromAgent(ctx, a.me, req, a.folder)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, mailboxReceiptText(req.Type, receipt))
	return err
}

func mailboxReceiptText(kind string, receipt mailbox.Receipt) string {
	where := fmt.Sprintf("Thread %s, message %d.", receipt.Thread, receipt.N)
	if kind == mailbox.Handoff && receipt.Resume != nil {
		if receipt.Resume.Note != "" {
			where += "\nHandoff note: " + receipt.Resume.Note + "."
		} else {
			branch := receipt.Resume.Branch
			if branch == "" {
				branch = "this detached checkout"
			}
			where += "\nNo handoff note for " + branch + " yet. Write one with the handoff skill. The reader finds the newest note when it resumes."
		}
	}
	switch {
	case receipt.State == mailbox.Held && kind == mailbox.Done:
		return "Held for approval in the companion. " + where + " The thread closes after it is approved."
	case receipt.State == mailbox.Held:
		return "Held for approval in the companion. " + where
	case kind == mailbox.Done:
		return "The thread is closing. " + where + " It closes once its waiting messages are delivered."
	default:
		return "Queued for delivery with the reader's next prompt. " + where
	}
}
