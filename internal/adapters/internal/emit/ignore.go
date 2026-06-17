package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// IgnoreBody concatenates the bodies of ignore specs into one
// gitignore-syntax block: each spec's trimmed body, joined by a blank
// line, in spec order. Returns "" when no spec contributes content.
func IgnoreBody(ignores []spec.Entry) string {
	parts := make([]string, 0, len(ignores))
	for _, e := range ignores {
		if body := strings.TrimSpace(e.Body); body != "" {
			parts = append(parts, body)
		}
	}
	return strings.Join(parts, "\n\n")
}

// WriteIgnoreFile writes the combined ignore patterns to path with a
// shell-style (`#`) provenance header, matching the comment syntax every
// gitignore-style ignore file (.cursorignore, .aiexclude, .aiderignore)
// understands. No-op when the patterns are empty so a target never writes a
// surprise empty ignore file.
func WriteIgnoreFile(ignores []spec.Entry, path string, dryRun bool) error {
	body := IgnoreBody(ignores)
	if body == "" {
		return nil
	}
	return WriteFile(path, WithHeader(body+"\n", FormatShell), dryRun)
}
