package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintEntryPointFences flags ::target / ::targets names in
// .agnostic-ai/AGNOSTIC_AI.md that match no known target. A typo there
// drops the paragraph from every entry-point file with no other signal.
// A missing file is not an issue: sync seeds it on the first run.
func lintEntryPointFences(root string) []validationIssue {
	data, err := os.ReadFile(filepath.Join(root, adapters.AgnosticEntryPointPath))
	if err != nil {
		return nil
	}
	body := header.Strip(string(data))
	known := adapters.Names()
	var out []validationIssue
	for _, name := range spec.FenceTargets(body) {
		if contains(known, name) {
			continue
		}
		out = append(out, validationIssue{
			Path:    adapters.AgnosticEntryPointPath,
			Field:   "::target",
			Message: fmt.Sprintf("unknown target %q in ::target fence; the block reaches no file", name),
		})
	}
	return out
}
