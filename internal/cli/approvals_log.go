package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func newApprovalsLogCmd() *cobra.Command {
	var (
		asJSON bool
		since  time.Duration
	)
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Print the decisions the approvals hook made",
		Long: "Prints each request the approvals hook saw, oldest first: when, the tool, the call, a detail with " +
			"anything that may hold a credential left out, and whether it approved or asked, with the rule that asked. " +
			"The log is $AGNOSTIC_AI_HOME/local/approvals/decisions.jsonl and one older file beside it.",
		Example: `  agnostic-ai approvals log
  agnostic-ai approvals log --since 1h --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			source, err := userSourceRoot()
			if err != nil {
				return err
			}
			entries, err := approvals.ReadLog(approvals.LogPath(source))
			if err != nil {
				return err
			}
			if since > 0 {
				cutoff := time.Now().Add(-since)
				kept := entries[:0]
				for _, e := range entries {
					if !e.Time.Before(cutoff) {
						kept = append(kept, e)
					}
				}
				entries = kept
			}
			if asJSON {
				if entries == nil {
					entries = []approvals.Entry{}
				}
				return writeIndentedJSON(cmd, entries)
			}
			if len(entries) == 0 {
				cmd.Println("No approvals decisions logged.")
				return nil
			}
			for _, e := range entries {
				decision := string(e.Decision)
				if e.Rule != "" {
					decision += " (" + e.Rule + ")"
				}
				cmd.Println(fmt.Sprintf("%s  %-7s %-16s %-10s %s", e.Time.Local().Format("2006-01-02 15:04:05"), e.Tool, decision, e.Call, e.Detail))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the decisions as a JSON array")
	cmd.Flags().DurationVar(&since, "since", 0, "Only decisions from this long ago on, such as 1h or 30m")
	return cmd
}
