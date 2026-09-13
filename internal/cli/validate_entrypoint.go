package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintEntryPointFences flags ::target / ::targets names in
// .agnostic-ai/AGNOSTIC_AI.md whose block reaches no file: a name that
// matches no built-in or configured target (a typo drops the paragraph
// everywhere with no other signal), or a built-in target that reads no
// pointer entry point (cursor, or any target on the legacy rules-file
// layout). A configured name outside the registry is an external
// adapter and passes. A missing file is not an issue: sync seeds it on
// the first run.
func lintEntryPointFences(root string, cfg *config.Config) []validationIssue {
	data, err := os.ReadFile(filepath.Join(root, adapters.AgnosticEntryPointPath))
	if err != nil {
		return nil
	}
	body := header.Strip(string(data))
	builtin := adapters.Names()
	var out []validationIssue
	for _, name := range spec.FenceTargets(body) {
		var msg string
		switch {
		case contains(builtin, name):
			if adapters.EntryPointPath(cfg, name) == "" || adapters.HasLegacyRulesFile(cfg, name) {
				msg = fmt.Sprintf("target %q reads no entry-point file; the ::target block reaches no file", name)
			}
		case !contains(cfg.Targets, name):
			msg = fmt.Sprintf("unknown target %q in ::target fence; the block reaches no file", name)
		}
		if msg == "" {
			continue
		}
		out = append(out, validationIssue{
			Path:    adapters.AgnosticEntryPointPath,
			Field:   "::target",
			Message: msg,
		})
	}
	return out
}
