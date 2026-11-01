package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/term"
)

// noTargetsKept reports whether cfg lists no target, in which case sync,
// watch and the drift reports keep every generated file: an empty list is
// more often a half-saved config than a request to delete everything
// (#2952). --allow-empty-targets lifts it.
func noTargetsKept(cfg *config.Config) bool { return cfg != nil && len(cfg.Targets) == 0 }

const emptyTargetsWarning = "targets lists no tool; keeping files until it does (run `agnostic-ai sync --allow-empty-targets` to remove them)"

func warnEmptyTargets(w io.Writer) {
	_, _ = fmt.Fprintf(w, "  %s %s\n", term.Bang(w), emptyTargetsWarning)
}

// writeEmptyTargetsJSON answers `sync --json` for an empty targets list.
func writeEmptyTargetsJSON(cmd *cobra.Command, command string) error {
	return writeIndentedJSON(cmd, syncJSONOutput{
		jsonOutput{Version: "1", Command: command}.forOutput(),
		syncDrops{
			Warnings: []dropRecord{{Target: "agnostic-ai", Message: emptyTargetsWarning, Suggestion: "agnostic-ai sync --allow-empty-targets"}},
			Notes:    []dropRecord{},
		},
	})
}
