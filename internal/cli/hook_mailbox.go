package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

const (
	mailboxHookTimeout = 2 * time.Second
	// mailboxReserveTimeout leaves the rest of the time limit to save the result.
	mailboxReserveTimeout = mailboxHookTimeout - mailbox.DeliveryLockWait
	mailboxHookMaxInput   = 64 << 10
)

type mailboxHookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	Source        string `json:"source"`
}

type mailboxReservation struct {
	store    mailbox.Store
	event    string
	delivery mailbox.Delivery
}

func newHookMailboxCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "mailbox",
		Short: "Print waiting mailbox messages from inside a prompt or session-start hook",
		Long: "Reads the hook payload on stdin and prints the messages waiting for that session in the " +
			"reply format the target adds to the model's context, then marks them delivered. " +
			"Runs for claude and codex on UserPromptSubmit, and on SessionStart for startup and resume. " +
			"Prints nothing and exits 0 when there is nothing to deliver or anything fails.",
		Example: `  # The mailbox built-in runs it on every prompt
  agnostic-ai hook mailbox --target codex`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				target = os.Getenv(adapters.HookTargetEnv)
			}
			deliverMailbox(cmd.Context(), target, cmd.InOrStdin(), cmd.OutOrStdout())
			return nil
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target that runs the hook (default $"+adapters.HookTargetEnv+")")
	return cmd
}

// deliverMailbox prints from this goroutine only, so a reservation that
// finishes after the time limit can never print late.
func deliverMailbox(ctx context.Context, target string, in io.Reader, out io.Writer) {
	settle, cancelSettle := context.WithTimeout(context.WithoutCancel(ctx), mailboxHookTimeout)
	defer cancelSettle()
	reserving, cancelReserve := context.WithTimeout(ctx, mailboxReserveTimeout)
	defer cancelReserve()
	reserved := make(chan mailboxReservation, 1)
	go func() { reserved <- reserveMailbox(reserving, target, in) }()
	var r mailboxReservation
	select {
	case r = <-reserved:
	case <-reserving.Done():
		select {
		case late := <-reserved:
			_ = late.store.PutBack(settle, late.delivery)
		default:
		}
		return
	}
	if r.delivery.Block == "" {
		return
	}
	reply, err := mailboxHookReply(target, r.event, r.delivery.Block)
	if err == nil {
		var n int
		n, err = io.WriteString(out, reply)
		if err == nil && n < len(reply) {
			err = io.ErrShortWrite
		}
	}
	if err != nil {
		_ = r.store.PutBack(settle, r.delivery)
		return
	}
	_ = r.store.Confirm(settle, r.delivery)
}

func reserveMailbox(ctx context.Context, target string, in io.Reader) mailboxReservation {
	raw, err := io.ReadAll(io.LimitReader(in, mailboxHookMaxInput+1))
	if err != nil || len(raw) > mailboxHookMaxInput {
		return mailboxReservation{}
	}
	var input mailboxHookInput
	if json.Unmarshal(raw, &input) != nil || !mailboxHookRuns(target, input) {
		return mailboxReservation{}
	}
	to := conversations.Ref{Tool: conversations.Tool(target), ID: input.SessionID}
	if mailbox.ValidateRef(to) != nil {
		return mailboxReservation{}
	}
	marks := conversations.DefaultMarksPath()
	if marks == "" {
		return mailboxReservation{}
	}
	store := mailbox.Store{Dir: filepath.Join(filepath.Dir(marks), "mailbox")}
	delivery, err := store.Reserve(ctx, to)
	if err != nil {
		return mailboxReservation{}
	}
	return mailboxReservation{store: store, event: input.HookEventName, delivery: delivery}
}

// mailboxHookRuns keeps delivery to a human prompt, a start or a resume:
// compaction and clear also fire SessionStart, with no one at the prompt.
func mailboxHookRuns(target string, input mailboxHookInput) bool {
	if target != string(conversations.ToolClaude) && target != string(conversations.ToolCodex) {
		return false
	}
	switch input.HookEventName {
	case "UserPromptSubmit":
		return true
	case "SessionStart":
		return input.Source == "startup" || input.Source == "resume"
	default:
		return false
	}
}

func mailboxHookReply(target, event, block string) (string, error) {
	if target == string(conversations.ToolClaude) {
		return block + "\n", nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	type hookContext struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	}
	err := enc.Encode(map[string]hookContext{"hookSpecificOutput": {event, block}})
	return b.String(), err
}
