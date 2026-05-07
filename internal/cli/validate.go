package cli

import (
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate agnostic specs.",
		Example: `  # Parse every spec, list any issues
  agnostic-ai validate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, b, err := loadProject(".")
			if err != nil {
				return err
			}
			entries := b.All()
			cmd.Printf("loaded %d entries.\n", len(entries))
			if len(entries) == 0 {
				cmd.PrintErrln(emptySpecsHint)
				return nil
			}
			issues := lintEntries(entries)
			reportIssues(cmd, issues)
			return nil
		},
	}
}

// lintEntries scans loaded specs for fixable problems. Today it
// catches markdown specs that are missing `name:` in frontmatter
// (the spec loader still produces an Entry by deriving the name from
// the filename, which masks the omission until something else looks
// at the raw frontmatter, e.g. a downstream tool that round-trips
// the source file).
func lintEntries(entries []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range entries {
		if isMarkdownKind(e.Kind) && !metaHasString(e.Meta, "name") {
			out = append(out, validationIssue{
				Path:    e.Path,
				Field:   "name",
				Message: "frontmatter is missing required 'name:' field",
				kind:    issueMissingName,
				entry:   e,
			})
		}
	}
	return out
}

// reportIssues prints the issue list to the command's stdout. An empty
// list prints nothing so a clean validate run stays terse.
func reportIssues(cmd *cobra.Command, issues []validationIssue) {
	if len(issues) == 0 {
		return
	}
	cmd.Printf("%d issue(s) found:\n", len(issues))
	for _, i := range issues {
		marker := " "
		if i.Autofixable() {
			marker = "*"
		}
		cmd.Printf("  %s %s: %s\n", marker, i.Path, i.Message)
	}
	cmd.Printf("(* = autofixable; rerun with --fix)\n")
}

// validationIssue is one problem flagged by the linter. A non-zero
// kind makes the issue autofixable; the kind enum decides which fix
// routine `--fix` will run.
type validationIssue struct {
	Path    string
	Field   string
	Message string
	kind    issueKind
	entry   spec.Entry
}

// Autofixable reports whether `validate --fix` knows how to repair the
// issue without further user input.
func (i validationIssue) Autofixable() bool {
	return i.kind != issueNone
}

type issueKind int

const (
	issueNone issueKind = iota
	issueMissingName
)

func isMarkdownKind(k spec.Kind) bool {
	switch k {
	case spec.KindAgent, spec.KindSkill, spec.KindRule:
		return true
	}
	return false
}

func metaHasString(meta map[string]any, key string) bool {
	v, ok := meta[key]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && s != ""
}
