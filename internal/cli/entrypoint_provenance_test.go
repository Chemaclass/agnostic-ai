package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func writeProvenanceProject(t *testing.T, dir string, keepHeader string) {
	t.Helper()
	targets := []string{"claude", "codex", "gemini", "cursor", "copilot", "amp", "junie"}
	var cfg strings.Builder
	cfg.WriteString("version: 1\ntargets: [" + strings.Join(targets, ", ") + "]\noutputs:\n")
	for _, target := range targets {
		if target == keepHeader {
			continue
		}
		cfg.WriteString("  " + target + ": { provenance-header: false }\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "style.md")
	if err := os.MkdirAll(filepath.Dir(rule), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rule, []byte("---\nname: style\ndescription: Style.\n---\nUse tabs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAgnosticFile(t, "# Project instructions\n\nKeep it short.\n")
}

// A root instructions file skips the provenance header when every target
// that reads it opts out, and a later sync still treats it as its own
// (#2256).
func TestSync_EntryPointsSkipHeaderWhenEveryReaderOptsOut(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeProvenanceProject(t, dir, "")

	runSyncOK(t)
	for _, path := range []string{"AGENTS.md", "GEMINI.md", "CLAUDE.md", ".github/copilot-instructions.md"} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		if header.Has(string(data)) {
			t.Errorf("%s keeps the provenance header with every reader opted out:\n%s", path, data)
		}
	}

	var buf bytes.Buffer
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()
	runSyncOK(t)
	if strings.Contains(buf.String(), "hand-authored") {
		t.Errorf("second sync treats its own entry point as hand-authored:\n%s", buf.String())
	}
	runSyncOK(t, "--check")
}

// One reader that keeps the header keeps it on the shared file.
func TestSync_SharedEntryPointKeepsHeaderWhenOneReaderWantsIt(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeProvenanceProject(t, dir, "amp")

	runSyncOK(t)
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !header.Has(string(agents)) {
		t.Errorf("AGENTS.md lost the header that amp keeps:\n%s", agents)
	}
	gemini, err := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
	if err != nil {
		t.Fatal(err)
	}
	if header.Has(string(gemini)) {
		t.Errorf("GEMINI.md keeps the header gemini opts out of:\n%s", gemini)
	}
}

// doctor counts headerless entry points as managed, both from the sync
// record and, on a fresh clone with no record, from the opted-out readers.
func TestDoctor_HeaderlessEntryPointsAreNotUnmanaged(t *testing.T) {
	for _, freshClone := range []bool{false, true} {
		name := "after sync"
		if freshClone {
			name = "fresh clone"
		}
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			writeProvenanceProject(t, dir, "")
			runSyncOK(t)
			if freshClone {
				if err := os.Remove(stateFilePath(".")); err != nil {
					t.Fatal(err)
				}
			}
			out, _ := runCLI(t, "doctor")
			for _, path := range []string{"AGENTS.md", "GEMINI.md", "CLAUDE.md"} {
				if strings.Contains(out, "✗ "+path) {
					t.Errorf("doctor lists %s as unmanaged:\n%s", path, out)
				}
			}
		})
	}
}
