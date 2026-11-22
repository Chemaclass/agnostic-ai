package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

const mailboxCommandTimeout = 30 * time.Second

// mailboxSession is what an agent command reads to name its sender and
// its peers; tests replace mailboxEnv with a fake process table and stores.
type mailboxSession struct {
	scanner *conversations.Scanner
	getenv  func(string) string
	pid     int
}

var mailboxEnv = func() mailboxSession {
	return mailboxSession{scanner: conversations.NewScanner(), getenv: os.Getenv, pid: os.Getpid()}
}

// mailboxAgent is the session an agent command runs in, and its mailbox.
type mailboxAgent struct {
	me      conversations.Ref
	store   mailbox.Store
	scanner *conversations.Scanner
	folder  string
}

func mailboxStoreAt(marksPath string) (mailbox.Store, bool) {
	if marksPath == "" {
		return mailbox.Store{}, false
	}
	return mailbox.Store{Dir: filepath.Join(filepath.Dir(marksPath), "mailbox")}, true
}

// currentMailboxAgent fails before any store call, so a command run
// outside a session leaves nothing behind, not even an audit record.
func currentMailboxAgent() (mailboxAgent, error) {
	env := mailboxEnv()
	ref, _, err := conversations.CurrentSession(env.scanner.Processes, env.scanner.Stores, env.getenv, env.pid)
	if err != nil {
		return mailboxAgent{}, fmt.Errorf("mailbox commands run inside a Claude Code or Codex session: %w", err)
	}
	if refused := mailbox.ValidateRef(ref); refused != nil {
		if refused.Guard == "unknown-session" {
			refused.Reason = "The mailbox works between Claude Code and Codex sessions."
			refused.Next = "Run this from one of those."
		}
		return mailboxAgent{}, refused
	}
	store, ok := mailboxStoreAt(env.scanner.MarksPath)
	if !ok {
		return mailboxAgent{}, errors.New("could not find the agnostic-ai configuration folder that holds the mailbox")
	}
	folder, err := os.Getwd()
	if err != nil {
		return mailboxAgent{}, fmt.Errorf("read the current folder: %w", err)
	}
	return mailboxAgent{me: ref, store: store, scanner: env.scanner, folder: folder}, nil
}

func mailboxCommandError(err error) error {
	if errors.Is(err, mailbox.ErrBusy) {
		return errors.New("the mailbox is busy; try again")
	}
	return err
}

func runMailboxAgent(cmd *cobra.Command, run func(context.Context, mailboxAgent) error) error {
	agent, err := currentMailboxAgent()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), mailboxCommandTimeout)
	defer cancel()
	return mailboxCommandError(run(ctx, agent))
}

// runListedMailboxAgent is runMailboxAgent for commands with no target,
// so send's participant check does not cover the sender.
func runListedMailboxAgent(cmd *cobra.Command, run func(context.Context, mailboxAgent) error) error {
	return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
		if err := agent.listed(ctx); err != nil {
			return err
		}
		return run(ctx, agent)
	})
}

func printMailboxJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func newMailboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mailbox",
		Short: "Send and answer messages between local agent sessions",
		Long: `Send and answer messages between local Claude Code and Codex sessions.

Each command detects the session it runs in and uses it as the sender. That detection is context, not proof of who runs the command. Messages from an agent wait for approval in the companion unless that switch is off, and the reader's next prompt delivers them.`,
	}
	cmd.AddCommand(
		newMailboxSendCmd(),
		newMailboxDoneCmd(),
		newMailboxListCmd(),
		newMailboxShowCmd(),
		newMailboxClaimCmd(),
		newMailboxReleaseCmd(),
		newMailboxAnswerCmd("accept"),
		newMailboxAnswerCmd("decline"),
		newMailboxRefsCmd(),
		newMailboxStatsCmd(),
	)
	return cmd
}

func newMailboxRefsCmd() *cobra.Command {
	var base, commit string
	cmd := &cobra.Command{
		Use:     "refs",
		Short:   "Print the Git refs for a code review",
		Example: "  agnostic-ai mailbox refs --base origin/main",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			folder, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("read the current folder: %w", err)
			}
			refs, err := mailbox.ResolveReviewRefs(cmd.Context(), folder, base, commit)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(refs)
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "base ref or commit ID (uses local origin/HEAD when present)")
	cmd.Flags().StringVar(&commit, "commit", "", "commit ref or ID to review (default HEAD)")
	return cmd
}
