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
// rules routed to codex. Sync then writes cline's instructions file over
// it, which carries no rules block, so codex lost its rules (#3302).
func TestSync_RulesFileOnASharedInstructionsFile_StopsWhenRulesAreLost(t *testing.T) {
	for name, tc := range map[string]struct{ config, rule, reader string }{
		"cline reads it": {config: "targets: [codex, cline]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n", reader: "cline"},
		"amp misses a codex rule": {
			config: "targets: [codex, amp]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n",
			rule:   "---\nname: only-codex\ntarget: codex\n---\nCodex only.\n",
			reader: "amp",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := sharedRulesFileProject(t, tc.config)
			if tc.rule != "" {
				mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "only-codex.md"), tc.rule)
			}

			out, err := runCLI(t, "sync", "--gitignore=off")

			if err == nil {
				t.Fatalf("sync must stop, got:\n%s", out)
			}
			for _, want := range []string{"AGENTS.md", "codex", tc.reader, "outputs.codex.rules-file"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error must name %q, got: %v", want, err)
				}
			}
			if _, statErr := os.Stat(filepath.Join(dir, "AGENTS.md")); statErr == nil {
				t.Error("sync must stop before it writes AGENTS.md")
			}

			out, err = runCLI(t, "validate")

			if err == nil || !strings.Contains(out, "outputs.codex.rules-file") || !strings.Contains(out, tc.reader) {
				t.Errorf("validate must reject the config and name codex and %s: %v\n%s", tc.reader, err, out)
			}
		})
	}
}

// When the other reader inlines every rule the writer would have merged,
// the shared file already carries them. Sync keeps writing it as before,
// says the rules-file has no effect there, and lint warns, in both
// directions.
func TestSync_RulesFileOnASharedInstructionsFile_KeepsTheFileWhenRulesSurvive(t *testing.T) {
	for _, tc := range []struct{ writer, reader string }{{"codex", "amp"}, {"amp", "codex"}} {
		t.Run(tc.writer+" writes, "+tc.reader+" reads", func(t *testing.T) {
			dir := sharedRulesFileProject(t, "targets: [codex, amp]\noutputs:\n  "+tc.writer+":\n    rules-file: AGENTS.md\n")
			notes := captureNotes(t)

			out, err := runCLI(t, "sync", "--gitignore=off")

			if err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			if !strings.Contains(string(data), "SHARED-BODY") || !strings.Contains(string(data), "Use tabs.") {
				t.Errorf("AGENTS.md must keep the body and the rule, got:\n%s", data)
			}
			if note := notes.String(); !strings.Contains(note, "outputs."+tc.writer+".rules-file has no effect on AGENTS.md") || !strings.Contains(note, tc.reader) {
				t.Errorf("sync must say the rules-file has no effect there, got:\n%s", note)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "validate"); err != nil {
				t.Errorf("validate: %v\n%s", err, out)
			}
			out, _ = runCLI(t, "lint")
			lines := findingLines(out, "LINT063")
			if len(lines) != 1 || !strings.Contains(lines[0], "outputs."+tc.writer+".rules-file") || !strings.Contains(lines[0], tc.reader) {
				t.Errorf("want one LINT063 naming %s and %s, got:\n%s", tc.writer, tc.reader, out)
			}
		})
	}
}

// The documented form with the default targets: several of them inline
// rules into AGENTS.md, so nothing is lost and sync passes.
func TestSync_RulesFileOnAGENTSmdWithDefaultTargets_Passes(t *testing.T) {
	sharedRulesFileProject(t, "outputs:\n  codex:\n    rules-file: AGENTS.md\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
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
