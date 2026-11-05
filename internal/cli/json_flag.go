package cli

import (
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/spf13/cobra"
)

// formatWithJSONFlag resolves --json to the command's JSON format value.
// An explicit --format naming anything else conflicts instead of losing.
func formatWithJSONFlag(cmd *cobra.Command, format string, jsonOut bool, jsonFormat string) (string, error) {
	if !jsonOut {
		return format, nil
	}
	if cmd.Flags().Changed("format") && format != jsonFormat {
		return "", errs.Coded(errs.CodeFlagConflict, "--json cannot be combined with --format %q", format)
	}
	return jsonFormat, nil
}
