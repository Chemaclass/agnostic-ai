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

// The previews stop the same way the sync they preview does.
func TestSyncPreviews_StopOnAHandWrittenInstructionsFile(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", handWrittenClaude)

	for _, args := range [][]string{{"sync", "--check"}, {"sync", "--plan"}, {"sync", "--json", "--dry-run"}} {
		if _, err := runCLI(t, args...); err == nil || !strings.Contains(err.Error(), "agnostic-ai import claude") {
			t.Errorf("%s: err = %v, want the import step", strings.Join(args, " "), err)
		}
	}
}

// Personal text the local layer holds reaches CLAUDE.md, so it is not
// lost, and importing would copy it into the shared body.
func TestSync_LocalLayerTextCountsAsHeld(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "## Shared\n\nShared text.\n")
	mustWriteFile(t, ".agnostic-ai/local/AGNOSTIC_AI.md", "## Mine\n\nMy text.\n")
	mustWriteFile(t, "CLAUDE.md", "## Shared\n\nShared text.\n\n## Mine\n\nMy text.\n")

	runSyncOK(t)
}

// --backup replaces the file and keeps it, the way out after trimming
// what import captured.
func TestSync_BackupReplacesAHandWrittenFileAndKeepsIt(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", handWrittenClaude)

	runSyncOK(t, "--backup")

	if got := readFile(t, "CLAUDE.md.bak"); got != handWrittenClaude {
		t.Errorf("CLAUDE.md.bak = %q, want the hand-written file", got)
	}
}

// Previews with --backup agree with `sync --backup`, which proceeds.
func TestSyncPreviews_WithBackupProceed(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", handWrittenClaude)

	if out, err := runCLI(t, "sync", "--json", "--dry-run", "--backup"); err != nil {
		t.Errorf("sync --json --dry-run --backup: %v\n%s", err, out)
	}
}

// The warning names no step that would move local text into the shared
// body.
func TestSync_NoWarningForTextTheLocalLayerHolds(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "## Shared\n\nShared text.\n")
	mustWriteFile(t, ".agnostic-ai/local/AGNOSTIC_AI.md", "## Mine\n\nMy text.\n")
	mustWriteFile(t, "CLAUDE.md", "## Shared\n\nShared text.\n\n## Mine\n\nMy text.\n")
	log := captureLog(t)

	runSyncOK(t)

	if strings.Contains(log.String(), "appears hand-authored") {
		t.Errorf("sync warned about text the local layer holds:\n%s", log.String())
	}
}
