package cli

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintUnmanagedDoubleStar reports each sync.unmanaged entry that uses `**`
// (LINT057, warning). The matcher reads it as `*`, so the entry covers one
// folder and misses the root and deeper folders.
func lintUnmanagedDoubleStar(cfg *config.Config) []lintFinding {
	if cfg == nil {
		return nil
	}
	var out []lintFinding
	for _, entry := range config.DoubleStarUnmanaged(cfg.Sync.Unmanaged) {
		out = append(out, lintFinding{
			Code: "LINT057", Severity: lintWarn, Path: config.ConfigFileName,
			Message: fmt.Sprintf("sync.unmanaged: %q uses **, which matches exactly one folder, like *, so it misses the root and deeper folders; add one entry per depth", entry),
		})
	}
	return out
}

// noteUnmanagedDoubleStar prints the LINT057 findings as sync notes.
func noteUnmanagedDoubleStar(cfg *config.Config) {
	for _, f := range lintUnmanagedDoubleStar(cfg) {
		adapters.NoteProject(f.Path + ": " + f.Message)
	}
}
