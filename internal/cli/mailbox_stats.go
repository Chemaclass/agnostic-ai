package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

func newMailboxStatsCmd() *cobra.Command {
	var since string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "stats [--since <date>] [--json]",
		Short: "Print counts and durations from the mailbox",
		Long: `Print counts and durations from every mailbox thread on this computer: threads, messages, refusals, delivery time, human actions, handoffs and review requests. Durations are in seconds. With no data, counts are 0 and medians, percentiles and ratios are null.

The output holds no message text, paths or session IDs, and draws no conclusions.`,
		Example: `  agnostic-ai mailbox stats
  agnostic-ai mailbox stats --since 2026-11-01 --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var from time.Time
			if since != "" {
				parsed, err := time.Parse(time.DateOnly, since)
				if err != nil {
					return fmt.Errorf("--since takes a date such as 2026-11-01, not %q", since)
				}
				from = parsed
			}
			store, ok := mailboxStoreAt(conversations.DefaultMarksPath())
			if !ok {
				return errors.New("could not find the agnostic-ai configuration folder that holds the mailbox")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), mailboxCommandTimeout)
			defer cancel()
			stats, err := store.Stats(ctx, from)
			if err != nil {
				return mailboxCommandError(err)
			}
			for _, name := range stats.Unreadable {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: it could not be read\n", name)
			}
			note := ""
			if stats.AuditFull {
				note = fmt.Sprintf("audit.json holds only its last %d refusals without a thread; older ones are not counted.", mailbox.MaxAudit)
			}
			if asJSON {
				if note != "" {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), note)
				}
				return printMailboxJSON(cmd.OutOrStdout(), stats)
			}
			return printMailboxStats(cmd.OutOrStdout(), stats, note)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "count only messages, events and refusals from this date on (YYYY-MM-DD, UTC)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

// printMailboxStats prints each JSON value as its dotted key, in JSON
// order, so the plain and JSON output cannot name numbers differently.
func printMailboxStats(out io.Writer, stats mailbox.Stats, note string) error {
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var b strings.Builder
	var path []string
	expectKey := []bool{}
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		inObject := len(expectKey) > 0
		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{':
				expectKey = append(expectKey, true)
			case '}':
				expectKey = expectKey[:len(expectKey)-1]
				if len(path) > 0 {
					path = path[:len(path)-1]
				}
				if len(expectKey) > 0 {
					expectKey[len(expectKey)-1] = true
				}
			}
			continue
		}
		if inObject && expectKey[len(expectKey)-1] {
			path = append(path, token.(string))
			expectKey[len(expectKey)-1] = false
			continue
		}
		value := "null"
		if token != nil {
			value = fmt.Sprint(token)
		}
		fmt.Fprintf(&b, "%s: %s\n", strings.Join(path, "."), value)
		path = path[:len(path)-1]
		expectKey[len(expectKey)-1] = true
	}
	if note != "" {
		b.WriteString("note: " + note + "\n")
	}
	_, err = io.WriteString(out, b.String())
	return err
}
