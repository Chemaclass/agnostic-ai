package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func sharedRulesFileProject(t *testing.T, config string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n"+config)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Project\n\nSHARED-BODY\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nUse tabs.\n")
	return dir
}

// Codex's merged rules file holds no AGNOSTIC_AI.md body and only the
// rules routed to codex, so it is wrong for any other reader of that
// file, and the instructions file sync writes for that reader drops the
// merged rules. Sync used to write both, one after the other (#3302).
func TestSync_RulesFileOnASharedInstructionsFile_StopsAndNamesBothTargets(t *testing.T) {
	for _, reader := range []string{"cline", "amp"} {
		t.Run(reader, func(t *testing.T) {
			dir := sharedRulesFileProject(t, "targets: [codex, "+reader+"]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n")

			out, err := runCLI(t, "sync", "--gitignore=off")

			if err == nil {
				t.Fatalf("sync must stop, got:\n%s", out)
			}
			for _, want := range []string{"AGENTS.md", "codex", reader, "outputs.codex.rules-file"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error must name %q, got: %v", want, err)
				}
			}
			if _, statErr := os.Stat(filepath.Join(dir, "AGENTS.md")); statErr == nil {
				t.Error("sync must stop before it writes AGENTS.md")
			}

			out, err = runCLI(t, "validate")

			if err == nil || !strings.Contains(out, "outputs.codex.rules-file") || !strings.Contains(out, reader) {
				t.Errorf("validate must reject the config and name codex and %s: %v\n%s", reader, err, out)
			}
		})
	}
}

// The policy that settles two adapters cannot settle this: neither
// version is right for both readers.
func TestSync_RulesFileOnASharedInstructionsFile_StopsUnderPreferSpec(t *testing.T) {
	sharedRulesFileProject(t, "targets: [codex, cline]\noutputs:\n  codex:\n    rules-file: AGENTS.md\nsync:\n  collision-policy: prefer-spec\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err == nil {
		t.Fatalf("sync must stop, got:\n%s", out)
	}
}

// A user-owned AGENTS.md is written by neither, so nothing collides.
func TestSync_RulesFileOnASharedUnmanagedInstructionsFile_Passes(t *testing.T) {
	sharedRulesFileProject(t, "targets: [codex, cline]\noutputs:\n  codex:\n    rules-file: AGENTS.md\nsync:\n  unmanaged: [AGENTS.md]\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

// Two targets that both write their merged rules file to AGENTS.md go
// through the ordinary collision check: different content stops sync,
// the same content is written once.
func TestSync_TwoRulesFilesOnOnePath(t *testing.T) {
	config := "targets: [codex, amp]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n  amp:\n    rules-file: AGENTS.md\n"
	t.Run("different content", func(t *testing.T) {
		dir := sharedRulesFileProject(t, config)
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "only-codex.md"), "---\nname: only-codex\ntarget: codex\n---\nCodex only.\n")

		_, err := runCLI(t, "sync", "--gitignore=off")

		if err == nil || !strings.Contains(err.Error(), "output collision") || !strings.Contains(err.Error(), "amp, codex") {
			t.Fatalf("want an output collision naming amp and codex, got %v", err)
		}
	})
	t.Run("same content", func(t *testing.T) {
		dir := sharedRulesFileProject(t, config)

		if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
			t.Fatalf("sync: %v\n%s", err, out)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
		if !strings.Contains(string(data), "Use tabs.") {
			t.Errorf("AGENTS.md must hold the merged rules, got:\n%s", data)
		}
		if out, err := runCLI(t, "sync", "--check"); err != nil {
			t.Errorf("sync --check: %v\n%s", err, out)
		}
	})
}

func TestSync_RulesFileOnItsOwnInstructionsFile_StillWritesTheMergedFile(t *testing.T) {
	dir := sharedRulesFileProject(t, "targets: [codex]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(data), "Use tabs.") {
		t.Errorf("AGENTS.md must hold the merged rules, got:\n%s", data)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Any adapter output on a path sync then writes as an instructions file
// was lost without a word. An external adapter is one way to get there.
func TestSync_AdapterFileOnAnInstructionsFile_StopsAndNamesBoth(t *testing.T) {
	dir := sharedRulesFileProject(t, "targets: [codex, foo]\n")
	externalRulesFileAdapterOnPath(t, "foo", "AGENTS.md")

	_, err := runCLI(t, "sync", "--gitignore=off")

	if err == nil || !strings.Contains(err.Error(), "AGENTS.md ← foo, and the instructions file of codex") {
		t.Fatalf("want a collision naming foo and codex, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "AGENTS.md")); statErr == nil {
		t.Error("sync must stop before it writes AGENTS.md")
	}
}
