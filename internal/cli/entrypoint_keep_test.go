package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const handWrittenClaude = "# My project\n\n## Conventions\n\nUse pnpm. Never touch prod.\n"

// The first sync after init replaced a hand-written CLAUDE.md with the
// placeholder, which is what an agent setting agnostic-ai up runs (#1611).
func TestSync_KeepsAHandWrittenInstructionsFileItHasNotImported(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", handWrittenClaude)

	for _, args := range [][]string{{"sync"}, {"sync", "--json"}, {"sync", "--dry-run"}} {
		_, err := runCLI(t, args...)
		if err == nil || !strings.Contains(err.Error(), "keep them:    agnostic-ai import claude") {
			t.Errorf("%s: err = %v, want the import step", strings.Join(args, " "), err)
		}
	}
	if got := readFile(t, "CLAUDE.md"); got != handWrittenClaude {
		t.Errorf("CLAUDE.md changed:\n%s", got)
	}
	if _, err := os.Stat(".agnostic-ai/AGNOSTIC_AI.md"); err == nil {
		t.Error("sync wrote AGNOSTIC_AI.md before stopping")
	}
}

func TestSync_WritesAnInstructionsFileOnceImported(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", handWrittenClaude)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	runSyncOK(t)

	if got := readFile(t, "CLAUDE.md"); !strings.Contains(got, "Never touch prod.") {
		t.Errorf("CLAUDE.md lost the imported text:\n%s", got)
	}
}
