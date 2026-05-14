package cli

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// writeAgnosticEntryPoints writes the canonical
// `.agnostic-ai/AGNOSTIC_AI.md` plus the conventional root entry-point
// file for each enabled target (CLAUDE.md, AGENTS.md, GEMINI.md, ...).
// All files share the same pointer body so collision-prone paths
// (e.g. codex + amp both at AGENTS.md) are deduplicated to a single
// write.
//
// Targets that opted into the legacy concatenated rules-file layout
// via `outputs.<target>.rules-file` are skipped here: the adapter
// owns the entry-point write in that case so the pointer body does
// not clobber the concatenated content.
func writeAgnosticEntryPoints(cfg *config.Config, targets []string, dryRun bool) error {
	body := adapters.RenderEntryPoint(cfg)
	if err := adapters.WriteFile(adapters.AgnosticEntryPointPath, body, dryRun); err != nil {
		return fmt.Errorf("write %s: %w", adapters.AgnosticEntryPointPath, err)
	}
	seen := map[string]bool{adapters.AgnosticEntryPointPath: true}
	for _, t := range targets {
		if adapters.HasLegacyRulesFile(cfg, t) {
			continue
		}
		path := adapters.EntryPointPath(cfg, t)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		if err := adapters.WriteFile(path, body, dryRun); err != nil {
			return fmt.Errorf("write entry-point %s: %w", path, err)
		}
	}
	return nil
}
