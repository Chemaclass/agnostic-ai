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
	return writeIndentedJSON(cmd, out.withEmptyLists())
}

// writeIndentedJSON encodes v to the command's stdout with two-space
// indentation, the layout every --json output shares.
func writeIndentedJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// withEmptyLists replaces nil lists with empty ones so consumers always
// see `[]` rather than `null`.
func (o jsonOutput) withEmptyLists() jsonOutput {
	if o.Writes == nil {
		o.Writes = []fileRecord{}
	}
	if o.Skipped == nil {
		o.Skipped = []fileRecord{}
	}
	if o.Errors == nil {
		o.Errors = []errorRecord{}
	}
	return o
}
