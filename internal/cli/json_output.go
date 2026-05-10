package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// fileRecord is one entry in a JSON command output's writes or skipped list.
type fileRecord struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Bytes  int    `json:"bytes"`
}

// errorRecord reports a per-target error in a JSON command output.
type errorRecord struct {
	Target  string `json:"target"`
	Message string `json:"message"`
}

// jsonOutput is the stable schema emitted by --json on sync, revert, and
// doctor. The version field is bumped on any breaking change.
type jsonOutput struct {
	Version string        `json:"version"`
	Command string        `json:"command"`
	Writes  []fileRecord  `json:"writes"`
	Skipped []fileRecord  `json:"skipped"`
	Errors  []errorRecord `json:"errors"`
}

func emitJSON(cmd *cobra.Command, out jsonOutput) error {
	if out.Writes == nil {
		out.Writes = []fileRecord{}
	}
	if out.Skipped == nil {
		out.Skipped = []fileRecord{}
	}
	if out.Errors == nil {
		out.Errors = []errorRecord{}
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
