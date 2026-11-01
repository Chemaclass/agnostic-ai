package cli

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintUnmanagedDoubleStar reports each sync.unmanaged entry that uses `**`
// (LINT057, warning). The matcher reads it as `*`.
func lintUnmanagedDoubleStar(cfg *config.Config) []lintFinding {
	if cfg == nil {
		return nil
	}
	var out []lintFinding
	for _, entry := range config.DoubleStarUnmanaged(cfg.Sync.Unmanaged) {
		out = append(out, lintFinding{
			Code: "LINT057", Severity: lintWarn, Path: config.ConfigFileName,
			Message: fmt.Sprintf("sync.unmanaged: %q uses **, which acts like * and stays inside one folder name; %s", entry, doubleStarAdvice(entry)),
		})
	}
	return out
}

// doubleStarAdvice says what the entry really matches and what to write.
func doubleStarAdvice(entry string) string {
	if dir, ok := strings.CutSuffix(entry, "/**"); ok && !strings.ContainsAny(dir, "*?[\\") {
		return "it matches only files directly inside " + dir + "/; write " + dir + "/ to own everything under it"
	}
	if strings.HasPrefix(entry, "**/") {
		return "it matches one folder deep, so it misses the root and deeper folders; add one entry per depth"
	}
	return "write each folder level out, one entry per depth"
}

// noteUnmanagedDoubleStar prints the LINT057 findings as sync notes.
func noteUnmanagedDoubleStar(cfg *config.Config) {
	for _, f := range lintUnmanagedDoubleStar(cfg) {
		adapters.NoteProject(f.Path + ": " + f.Message)
	}
}
