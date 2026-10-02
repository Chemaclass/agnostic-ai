package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A catch-all glob carries no scoping intent: the rule stays always-on,
// and an import of the synced file reads it back as the same rule (#1597).
func TestSync_CatchAllGlobsRuleStaysAlwaysOnThroughImport(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	mustWriteFile(t, ".agnostic-ai/rules/style.md", "---\nname: style\ndescription: Style.\nglobs: \"**/*\"\n---\nKeep it tidy.\n")
	runSyncOK(t)
	if got := readFile(t, ".cursor/rules/style.mdc"); !strings.Contains(got, "alwaysApply: true") {
		t.Fatalf("catch-all rule not always-on:\n%s", got)
	}

	if out, err := runCLI(t, "import", "cursor", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	runSyncOK(t)

	if got := readFile(t, ".cursor/rules/style.mdc"); !strings.Contains(got, "alwaysApply: true") {
		t.Errorf("rule lost always-on after import and sync:\n%s", got)
	}
}

func TestSync_NarrowGlobsRuleAttachesToMatchingFiles(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	mustWriteFile(t, ".agnostic-ai/rules/react.md", "---\nname: react\ndescription: React.\nglobs: \"src/**/*.{ts,tsx}\"\n---\nFunction components.\n")
	runSyncOK(t)

	if got := readFile(t, ".cursor/rules/react.mdc"); !strings.Contains(got, "alwaysApply: false") {
		t.Errorf("narrow globs rule still always-on:\n%s", got)
	}
}
