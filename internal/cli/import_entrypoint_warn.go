package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// warnUncapturedEntryPoints prints a warning for every target root
// entry-point file that exists in the project, is hand-authored (no
// agnostic-ai header), and holds content different from the body just
// captured into AGNOSTIC_AI.md from mirroredSrc.
//
// import mirrors a single source entry-point into the shared body. A
// sibling entry-point with unique content (e.g. a project keeping both a
// rich CLAUDE.md and a distinct AGENTS.md) would otherwise be overwritten
// by the next sync with no notice, silently discarding real instructions
// (#415). Surfacing it lets the user merge that content into
// AGNOSTIC_AI.md before syncing.
//
// Generated siblings (carrying the agnostic-ai header) are skipped: sync
// rewrites them with the same body, so nothing is lost.
func warnUncapturedEntryPoints(root, mirroredSrc, capturedBody string) {
	want := strings.TrimSpace(capturedBody)
	for _, rel := range adapters.ConventionalEntryPointPaths() {
		if rel == mirroredSrc {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		if header.Has(string(data)) {
			continue
		}
		body := strings.TrimSpace(adapters.StripGeneratedAppendices(string(data)))
		if body == "" || body == want {
			continue
		}
		summaryf("  ! %s has unique content not captured by this import — sync will overwrite it with the shared body. Merge it into %s first to keep it.\n",
			rel, agnosticMainFile)
	}
}
