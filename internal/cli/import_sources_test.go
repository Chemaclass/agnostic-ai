package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// advertisedImportSources returns the names `importSources()` prints,
// which is what the help text and the unknown-source error both show.
func advertisedImportSources() []string {
	var out []string
	for _, name := range strings.Split(importSources(), ", ") {
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// TestIsKnownImportSource_AcceptsEverySourceTheHelpTextLists pins the
// invariant that broke in #905: `importSources()` feeds both the help
// text and the "unknown source" error, while `isKnownImportSource`
// gated multi-source runs from a separate hardcoded list. The two
// drifted, so `import antigravity codex` failed with a message that
// listed `antigravity` as supported.
func TestIsKnownImportSource_AcceptsEverySourceTheHelpTextLists(t *testing.T) {
	for _, name := range advertisedImportSources() {
		if !isKnownImportSource(name) {
			t.Errorf("importSources() advertises %q but isKnownImportSource rejects it", name)
		}
	}
}

// TestRunImport_DispatchesEverySourceTheHelpTextLists closes the same
// drift one layer down. Passing validation only means the name is in a
// list; `runImport`'s switch is what actually has to handle it, and a
// name missing there reaches the unknown-source error at run time,
// after validation has already waved it through.
//
// Import failures from an empty project are expected and ignored. Only
// the unknown-source code is a failure here.
func TestRunImport_DispatchesEverySourceTheHelpTextLists(t *testing.T) {
	for _, name := range advertisedImportSources() {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)

			err := runImport(dir, name, &config.Config{Sources: config.Sources{}})
			if err != nil && strings.Contains(err.Error(), "unknown source") {
				t.Errorf("importSources() advertises %q but runImport does not dispatch it: %v", name, err)
			}
		})
	}
}
