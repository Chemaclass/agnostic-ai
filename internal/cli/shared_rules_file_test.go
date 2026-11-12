package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
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

// Two writers whose merged files differ still collide when a reader
// keeps their rules: neither merged file survives sync, but they differ.
func TestSync_TwoKeptRulesFilesWithDifferentContent_Collide(t *testing.T) {
	dir := sharedRulesFileProject(t, "targets: [codex, amp, crush]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n  amp:\n    rules-file: AGENTS.md\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "pair.md"), "---\nname: pair\ntargets: [codex, crush]\n---\nPair rule.\n")

	_, err := runCLI(t, "sync", "--gitignore=off")

	if err == nil || !strings.Contains(err.Error(), "AGENTS.md ← amp, codex") {
		t.Fatalf("want the output collision naming amp and codex, got %v", err)
	}
}

// A rules-file on another target's instructions file keeps working when
// the owner still loads it: Claude imports it from CLAUDE.md, and the
// file sync writes there carries the rules.
func TestSync_RulesFileOnAnotherTargetsInstructionsFile_KeptWhenTheOwnerImportsIt(t *testing.T) {
	for _, tc := range []struct{ peer, path string }{{"codex", "AGENTS.md"}, {"amp", "AGENTS.md"}, {"gemini", "GEMINI.md"}} {
		t.Run(tc.peer, func(t *testing.T) {
			dir := sharedRulesFileProject(t, "targets: [claude, "+tc.peer+"]\noutputs:\n  claude:\n    rules-file: "+tc.path+"\n")
			notes := captureNotes(t)

			if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
			if !strings.Contains(string(claude), "@"+tc.path) {
				t.Errorf("CLAUDE.md must import %s, got:\n%s", tc.path, claude)
			}
			if !strings.Contains(notes.String(), "outputs.claude.rules-file has no effect on "+tc.path) {
				t.Errorf("sync must say the key has no effect, got:\n%s", notes.String())
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "validate"); err != nil {
				t.Errorf("validate: %v\n%s", err, out)
			}
		})
	}
}

// When the owner does not read the file, its merged rules reach no one
// once sync replaces them, and validate agrees with sync.
func TestValidate_RulesFileOnAnotherTargetsInstructionsFile_LostWhenTheOwnerDoesNotReadIt(t *testing.T) {
	for name, config := range map[string]string{
		"codex on CLAUDE.md":  "targets: [claude, codex]\noutputs:\n  codex:\n    rules-file: CLAUDE.md\n",
		"gemini on AGENTS.md": "targets: [gemini, codex]\noutputs:\n  gemini:\n    rules-file: AGENTS.md\n",
	} {
		t.Run(name, func(t *testing.T) {
			sharedRulesFileProject(t, config)

			if out, err := runCLI(t, "validate"); err == nil || !strings.Contains(out, "rules-file") {
				t.Errorf("validate must reject it: %v\n%s", err, out)
			}
			if _, err := runCLI(t, "sync", "--gitignore=off"); err == nil || !strings.Contains(err.Error(), "loses those rules") {
				t.Errorf("sync must stop: %v", err)
			}
		})
	}
}

// A run that writes no instructions file on the path leaves the merged
// file in place, so it says nothing about the key and checks the merged
// file.
func TestSync_PartialRunWithoutTheReaderKeepsTheMergedFile(t *testing.T) {
	dir := sharedRulesFileProject(t, "targets: [codex, amp]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "hand edit\n")

	if out, err := runCLI(t, "sync", "--check", "--target", "codex"); err == nil {
		t.Errorf("sync --check --target codex must see the hand edit:\n%s", out)
	}

	notes := captureNotes(t)
	if out, err := runCLI(t, "sync", "--gitignore=off", "--target", "codex"); err != nil {
		t.Fatalf("sync --target codex: %v\n%s", err, out)
	}
	if strings.Contains(notes.String(), "has no effect") {
		t.Errorf("a run that keeps the merged file must not call the key ineffective:\n%s", notes.String())
	}
	if out, err := runCLI(t, "sync", "--check", "--target", "codex"); err != nil {
		t.Errorf("sync --check --target codex after sync --target codex: %v\n%s", err, out)
	}
}

// An external adapter's merged file cannot be read for rules, so sync
// stops instead of guessing it is safe to replace.
func TestSync_ExternalRulesFileOnASharedInstructionsFile_Stops(t *testing.T) {
	sharedRulesFileProject(t, "targets: [codex, foo]\noutputs:\n  foo:\n    file: AGENTS.md\n    rules-file: AGENTS.md\n")
	externalRulesFileAdapterOnPath(t, "foo", "AGENTS.md")

	if _, err := runCLI(t, "sync", "--gitignore=off"); err == nil || !strings.Contains(err.Error(), "outputs.foo.rules-file") {
		t.Fatalf("sync must stop and name outputs.foo.rules-file, got %v", err)
	}
}

// Every spelling of AGENTS.md is one file: the ledger holds one entry
// and the next sync finds nothing to change (#3329).
func TestSync_RulesFileOnASharedInstructionsFile_SettlesWhateverItsSpelling(t *testing.T) {
	for _, spelling := range []string{"AGENTS.md", "./AGENTS.md", ".//AGENTS.md", "AGENTS.md/"} {
		t.Run(spelling, func(t *testing.T) {
			dir := sharedRulesFileProject(t, "targets: [codex, amp]\noutputs:\n  codex:\n    rules-file: "+spelling+"\n")

			if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
			first, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("second sync: %v\n%s", err, out)
			}
			if _, err := os.Stat(filepath.Join(dir, "AGENTS.md.bak")); err == nil {
				t.Error("the second sync must not save AGENTS.md.bak")
			}
			if second, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(second) != string(first) {
				t.Errorf("the second sync must not change AGENTS.md:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			assertLedgerSpellsAGENTSmdOnce(t, readStateFile("."))
		})
	}
}

// A ledger written before #3329 holds both spellings. The next sync must
// settle to the clean one without a backup and keep the live file.
func TestSync_DotSlashRulesFileWithAStaleLedgerEntry_Settles(t *testing.T) {
	dir := sharedRulesFileProject(t, "targets: [codex, amp]\noutputs:\n  codex:\n    rules-file: ./AGENTS.md\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	want, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	state := readStateFile(".")
	state.Outputs = append(state.Outputs, "./AGENTS.md")
	state.OutputSums["./AGENTS.md"] = "3af4330a8a25f3ef8f926b6cc1ea6b616a0eba92f98e391ac45ce2ea742cd2af"
	if err := replaceStateFile(".", state); err != nil {
		t.Fatal(err)
	}

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync over the stale ledger: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md.bak")); err == nil {
		t.Error("sync must not save AGENTS.md.bak")
	}
	if got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md")); err != nil || string(got) != string(want) {
		t.Errorf("sync must keep AGENTS.md as it was (err %v):\nwant:\n%s\ngot:\n%s", err, want, got)
	}
	assertLedgerSpellsAGENTSmdOnce(t, readStateFile("."))
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

func TestSync_RulesFileOnASharedInstructionsFile_BacksUpAHandEditOnce(t *testing.T) {
	dir := sharedRulesFileProject(t, "targets: [codex, amp]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	want, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "HAND-EDIT\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync after the edit: %v\n%s", err, out)
	}

	if bak, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md.bak")); string(bak) != "HAND-EDIT\n" {
		t.Errorf("AGENTS.md.bak must hold the edit, got %q", bak)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != string(want) {
		t.Errorf("sync must restore AGENTS.md, got:\n%s", got)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Two syncs before #3329 left codex's merged file in AGENTS.md, amp's
// file in AGENTS.md.bak, and a ledger with both spellings. The ledger
// recorded the merged bytes under `./AGENTS.md`, so sync may replace them.
func TestSync_DotSlashRulesFileStuckOnAnOldBackup_Heals(t *testing.T) {
	dir := sharedRulesFileProject(t, "targets: [codex, amp]\noutputs:\n  codex:\n    rules-file: ./AGENTS.md\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	amp, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if out, err := runCLI(t, "sync", "--gitignore=off", "--target", "codex"); err != nil {
		t.Fatalf("sync --target codex: %v\n%s", err, out)
	}
	merged, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(merged) == string(amp) {
		t.Fatal("setup: AGENTS.md must hold codex's merged file")
	}
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md.bak"), string(amp))
	state := readStateFile(".")
	state.Outputs = append(state.Outputs, "./AGENTS.md")
	state.OutputSums["./AGENTS.md"] = adapters.ContentSum(string(merged))
	state.OutputSums["AGENTS.md"] = adapters.ContentSum(string(amp))
	state.Backups = map[string]string{"./AGENTS.md.bak": adapters.ContentSum(string(amp))}
	if err := replaceStateFile(".", state); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "sync", "--gitignore=off")

	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if strings.Contains(out, "hand edit") {
		t.Errorf("sync must not take its own merged file for a hand edit:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(got) != string(amp) {
		t.Errorf("AGENTS.md must hold amp's file again, got:\n%s", got)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

func assertLedgerSpellsAGENTSmdOnce(t *testing.T, state syncStateFile) {
	t.Helper()
	var spellings []string
	for _, p := range state.Outputs {
		if strings.HasSuffix(p, "AGENTS.md") || strings.HasSuffix(p, "AGENTS.md/") {
			spellings = append(spellings, p)
		}
	}
	if len(spellings) != 1 || spellings[0] != "AGENTS.md" {
		t.Errorf("the ledger must list AGENTS.md once, got %q", spellings)
	}
	for p := range state.OutputSums {
		if p != "AGENTS.md" && strings.Contains(p, "AGENTS.md") {
			t.Errorf("the ledger must not hold a sum for %q", p)
		}
	}
}
