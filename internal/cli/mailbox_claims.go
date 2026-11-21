package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

func newMailboxClaimCmd() *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:     "claim [--worktree <dir>] <path>...",
		Short:   "Claim paths in a worktree for this session",
		Long:    "Claim paths so other sessions see that this session works on them. Run it inside the worktree; --worktree defaults to the Git root of the current folder. A path another session holds is refused and names the holder.",
		Example: `  agnostic-ai mailbox claim internal/mailbox docs/site/content/docs/mailbox.md`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, paths []string) error {
			return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
				gives, err := agent.gives(ctx, worktree, paths)
				if err != nil {
					return err
				}
				if row, found := agent.scanner.Find(ctx, agent.me.Tool, agent.me.ID, conversations.ScanOptions{}); !found || !canUseMailbox(row) {
					return mailbox.NewRefusal("unknown-session")
				}
				if err := agent.store.Claim(ctx, agent.me, gives, agent.folder); err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Claimed %d path%s in %s.\n", len(paths), pluralS(len(paths)), gives.Worktree)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&worktree, "worktree", "", "worktree that holds the paths (default: the Git root of the current folder)")
	return cmd
}

func newMailboxReleaseCmd() *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:     "release [--worktree <dir>] <path>...",
		Short:   "Release paths this session claimed",
		Long:    "Release claimed paths so other sessions can claim them. --worktree defaults to the Git root of the current folder. A path this session does not hold is left as it is.",
		Example: `  agnostic-ai mailbox release internal/mailbox`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, paths []string) error {
			return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
				gives, err := agent.gives(ctx, worktree, paths)
				if err != nil {
					return err
				}
				if err := agent.store.Release(ctx, agent.me, gives); err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Released.")
				return err
			})
		},
	}
	cmd.Flags().StringVar(&worktree, "worktree", "", "worktree that holds the paths (default: the Git root of the current folder)")
	return cmd
}

func (a mailboxAgent) gives(ctx context.Context, worktree string, paths []string) (mailbox.Gives, error) {
	if worktree == "" {
		root, err := mailbox.ResolveClaimWorktree(ctx, a.folder)
		if err != nil {
			return mailbox.Gives{}, err
		}
		worktree = root
	}
	return mailbox.Gives{Worktree: worktree, Paths: paths}, nil
}

func newMailboxAnswerCmd(action string) *cobra.Command {
	var thread string
	var n int
	short, done := "Accept a handoff offered to this session", "Handoff accepted."
	if action == "decline" {
		short, done = "Decline a handoff offered to this session", "Handoff declined."
	}
	cmd := &cobra.Command{
		Use:     action + " --thread <id> --n <n>",
		Short:   short,
		Long:    "Answer a handoff that was offered to the session this runs in. Accepting moves the paths to this session only if no other session holds them at that moment.",
		Example: "  agnostic-ai mailbox " + action + " --thread th_20261121_7f3a9c10 --n 2",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
				if n < 1 {
					return mailbox.NewRefusal("format")
				}
				if err := agent.store.AnswerHandoffFromAgent(ctx, agent.me, thread, n, action); err != nil {
					return err
				}
				text := done
				if action == "accept" {
					if view, err := agent.store.ThreadFor(ctx, thread, agent.me); err == nil && n <= len(view.Messages) {
						if gives := view.Messages[n-1].Gives; gives != nil {
							text += fmt.Sprintf(" Work only in %s: %s.", gives.Worktree, strings.Join(gives.Paths, ", "))
						}
					}
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), text)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&thread, "thread", "", "thread that holds the handoff")
	cmd.Flags().IntVar(&n, "n", 0, "number of the handoff message in the thread")
	_ = cmd.MarkFlagRequired("thread")
	_ = cmd.MarkFlagRequired("n")
	return cmd
}

func newMailboxListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the mailbox threads this session is in",
		Long:  "Print the threads of the session this runs in as JSON. A message's text, refs and paths show only when this session sent it or it was already delivered to this session; a message waiting for approval or delivery shows its type and state only.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
				list, err := agent.store.ThreadsFor(ctx, agent.me)
				if err != nil {
					return err
				}
				return printMailboxJSON(cmd.OutOrStdout(), list)
			})
		},
	}
}

func newMailboxShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <thread>",
		Short:   "Show one mailbox thread this session is in",
		Long:    "Print one thread of the session this runs in as JSON, with the same limits as mailbox list.",
		Example: "  agnostic-ai mailbox show th_20261121_7f3a9c10",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMailboxAgent(cmd, func(ctx context.Context, agent mailboxAgent) error {
				thread, err := agent.store.ThreadFor(ctx, args[0], agent.me)
				if err != nil {
					return err
				}
				return printMailboxJSON(cmd.OutOrStdout(), thread)
			})
		},
	}
}
