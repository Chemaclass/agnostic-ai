package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

func newMailboxCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mailbox", Short: "Work with the local session mailbox"}
	cmd.AddCommand(newMailboxRefsCmd())
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
