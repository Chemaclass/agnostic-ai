package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// syncFencedProject writes config and a fenced AGNOSTIC_AI.md in a temp
// cwd and runs a real sync, so every entry point holds the view sync
// renders for the configured readers.
func syncFencedProject(t *testing.T, config, source string) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", config)
	writeAgnosticFile(t, source)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

func assertSourceKept(t *testing.T, source string) {
	t.Helper()
	got, err := os.ReadFile(agnosticMainFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != source {
		t.Errorf("fenced source was replaced:\ngot:  %q\nwant: %q", got, source)
	}
}

// AGENTS.md is read by codex alone when only codex and gemini are enabled.
// The guard must render for the enabled targets, not every target that
// could read AGENTS.md (amp would pull its block in and break the match).
func TestMirrorMainFile_KeepsFencedSourceForEnabledReadersOnly(t *testing.T) {
	source := "Shared.\n\n::target amp\nAmp-only line.\n::end\n\n::target gemini\nGemini-only line.\n::end\n"
	syncFencedProject(t, "version: 1\ntargets: [codex, gemini]\n", source)

	result, err := mirrorMainFile(".", "AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}

	if result != mirrorUnchanged {
		t.Errorf("result=%v, want mirrorUnchanged", result)
	}
	assertSourceKept(t, source)
}

// outputs.gemini.file moves gemini onto AGENTS.md, so its block belongs
// to that file's view. The guard must honor the override.
func TestMirrorMainFile_KeepsFencedSourceWithEntryPointOverride(t *testing.T) {
	source := "Shared.\n\n::target gemini\nGemini-only line.\n::end\n"
	syncFencedProject(t, "version: 1\ntargets: [codex, gemini]\noutputs:\n  gemini:\n    file: AGENTS.md\n", source)

	result, err := mirrorMainFile(".", "AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}

	if result != mirrorUnchanged {
		t.Errorf("result=%v, want mirrorUnchanged", result)
	}
	assertSourceKept(t, source)
}

// Sync then import round trip: importing an untouched CLAUDE.md keeps the
// fenced source and says it is unchanged instead of "seeded".
func TestImportClaude_AfterSyncKeepsFencedSourceAndReportsUnchanged(t *testing.T) {
	source := "Shared.\n\n::target claude\nClaude-only line.\n::end\n\n::target gemini\nGemini-only line.\n::end\n"
	syncFencedProject(t, "version: 1\ntargets: [claude, gemini]\n", source)
	var log bytes.Buffer
	prev := logOut
	logOut = &log
	defer func() { logOut = prev }()

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	assertSourceKept(t, source)
	if strings.Contains(log.String(), agnosticMainFile+" seeded from") {
		t.Errorf("import claimed it seeded the kept source:\n%s", log.String())
	}
	if !strings.Contains(log.String(), agnosticMainFile+" unchanged") {
		t.Errorf("import should report the source unchanged:\n%s", log.String())
	}
}
