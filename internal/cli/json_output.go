package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
)

// fileRecord is one entry in a JSON command output's writes or skipped list.
type fileRecord struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Bytes  int    `json:"bytes"`
	// Backup is the `<path>.bak` holding a hand edit this write replaced.
	Backup string `json:"backup,omitempty"`
}

// errorRecord reports a per-target error in a JSON command output.
type errorRecord struct {
	Target  string `json:"target"`
	Message string `json:"message"`
}

// dropRecord is a capability warning or coverage note in `sync --json`.
type dropRecord struct {
	Target  string `json:"target"`
	Kind    string `json:"kind"`
	Count   int    `json:"count"`
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
	return writeIndentedJSON(cmd, out.forOutput())
}

// writeIndentedJSON encodes v to the command's stdout with two-space
// indentation, the layout every --json output shares.
func writeIndentedJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// addError records err as a run-level error. A nil err adds nothing.
func (o *jsonOutput) addError(err error) {
	if err != nil {
		o.Errors = append(o.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
	}
}

// forOutput is o as every --json command prints it: nil lists become
// `[]` rather than `null`, and record paths use `/` on every OS.
func (o jsonOutput) forOutput() jsonOutput {
	o.Writes = slashedRecords(o.Writes)
	o.Skipped = slashedRecords(o.Skipped)
	if o.Errors == nil {
		o.Errors = []errorRecord{}
	}
	return o
}

func slashedRecords(records []fileRecord) []fileRecord {
	out := make([]fileRecord, len(records))
	for i, r := range records {
		r.Path = filepath.ToSlash(r.Path)
		r.Backup = filepath.ToSlash(r.Backup)
		out[i] = r
	}
	return out
}

// slashLintPaths and its siblings copy findings with `/` paths, as every
// --json writer prints them.
func slashLintPaths(in []lintFinding) []lintFinding {
	out := slices.Clone(in)
	for i := range out {
		out[i].Path = filepath.ToSlash(out[i].Path)
	}
	return out
}

func slashHookTrustPaths(in []codex.HookTrustFinding) []codex.HookTrustFinding {
	out := slices.Clone(in)
	for i := range out {
		out[i].Path = filepath.ToSlash(out[i].Path)
	}
	return out
}

func slashPackagingPaths(in []packagingIgnoreFinding) []packagingIgnoreFinding {
	out := slices.Clone(in)
	for i := range out {
		out[i].Path = filepath.ToSlash(out[i].Path)
		out[i].Uncovered = slices.Clone(out[i].Uncovered)
		for j, u := range out[i].Uncovered {
			out[i].Uncovered[j] = filepath.ToSlash(u)
		}
	}
	return out
}

func slashReferencePaths(in []referenceFinding) []referenceFinding {
	out := slices.Clone(in)
	for i := range out {
		out[i].Source = filepath.ToSlash(out[i].Source)
		out[i].Path = filepath.ToSlash(out[i].Path)
	}
	return out
}
