package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintGitignoreCommitTargets flags a `<target>:<kind>` gitignore.commit
// entry whose target is not configured (LINT017, warning). The entry
// then commits nothing, and a misspelled target keeps the files a cloud
// agent reads out of Git with no other sign.
func lintGitignoreCommitTargets(cfg *config.Config) []lintFinding {
	if cfg == nil {
		return nil
	}
	var out []lintFinding
	for _, entry := range cfg.Gitignore.Commit {
		target, _, scoped := strings.Cut(entry, ":")
		if scoped && !slices.Contains(cfg.Targets, target) {
			out = append(out, lintFinding{
				Code: "LINT017", Severity: lintWarn, Path: config.ConfigFileName,
				Message: fmt.Sprintf("gitignore.commit: %q names %s, which is not in targets, so it commits nothing", entry, target),
			})
		}
	}
	return out
}
