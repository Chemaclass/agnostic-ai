package cli

import (
	"strings"
	"testing"
)

// TestIsKnownImportSource_AcceptsEverySourceTheHelpTextLists pins the
// invariant that broke in #905: `importSources()` feeds both the help
// text and the "unknown source" error, while `isKnownImportSource`
// gated multi-source runs from a separate hardcoded list. The two
// drifted, so `import antigravity codex` failed with a message that
// listed `antigravity` as supported.
func TestIsKnownImportSource_AcceptsEverySourceTheHelpTextLists(t *testing.T) {
	for _, name := range strings.Split(importSources(), ", ") {
		if name == "" {
			continue
		}
		if !isKnownImportSource(name) {
			t.Errorf("importSources() advertises %q but isKnownImportSource rejects it", name)
		}
	}
}
