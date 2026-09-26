package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	sharedInstructions = "# Shared\n\nShared line.\n"
	localInstructions  = "Local line.\n"
)

// syncWithLocalInstructions writes a project whose shared and local
// AGNOSTIC_AI.md both hold text, then runs a real sync.
func syncWithLocalInstructions(t *testing.T, targets string) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: "+targets+"\n")
	writeFile(t, filepath.Join(".agnostic-ai", "rules", "r1.md"), "---\nname: r1\n---\nRule line.\n")
	writeAgnosticFile(t, sharedInstructions)
	writeFile(t, filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"), localInstructions)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

func TestSync_AppendsProjectLocalInstructionsToEveryEntryPoint(t *testing.T) {
	syncWithLocalInstructions(t, "[claude, codex, gemini, junie]")

	for _, path := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md", filepath.Join(".junie", "AGENTS.md")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s not written: %v", path, err)
		}
		got := string(data)
		shared, local := strings.Index(got, "Shared line."), strings.Index(got, "Local line.")
		if shared < 0 || local < 0 || local < shared {
			t.Errorf("%s: want the local text after the shared text:\n%s", path, got)
		}
		if rules := strings.Index(got, adapters.RulesEndMarker); rules >= 0 && local < rules {
			t.Errorf("%s: want the local text after the rules block:\n%s", path, got)
		}
	}
}

func TestSync_KeepsProjectLocalInstructionsOutOfTheSharedSource(t *testing.T) {
	syncWithLocalInstructions(t, "[claude]")

	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != sharedInstructions {
		t.Errorf("AGNOSTIC_AI.md changed:\n%s", data)
	}
}

func TestSyncCheck_CleanWithProjectLocalInstructions(t *testing.T) {
	syncWithLocalInstructions(t, "[claude, codex]")

	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v\n%s", err, out)
	}
}

func TestSyncCheck_ReportsDriftWhenProjectLocalInstructionsChange(t *testing.T) {
	syncWithLocalInstructions(t, "[claude]")
	writeFile(t, filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"), "Changed local line.\n")

	if out, err := runCLI(t, "sync", "--check"); err == nil {
		t.Errorf("sync --check passed after the local instructions changed:\n%s", out)
	}
}

// Import reads the entry points sync wrote. The local text is private, so
// neither the mirrored AGNOSTIC_AI.md nor a sliced rule may absorb it.
func TestImport_KeepsProjectLocalInstructionsOutOfSharedSpecs(t *testing.T) {
	syncWithLocalInstructions(t, "[claude, codex, gemini]")
	// Drop the rules block so the slicers read the whole file, the path a
	// hand-edited or rule-less entry point takes.
	for _, path := range []string{"AGENTS.md", "GEMINI.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		stripped := adapters.StripGeneratedAppendices(string(data))
		stripped += "\n" + adapters.LocalStartMarker + "\n\nLocal line.\n\n" + adapters.LocalEndMarker + "\n"
		writeFile(t, path, stripped)
	}

	if _, err := mirrorMainFile(".", "CLAUDE.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := sliceMainFileByH2(".", "GEMINI.md", filepath.Join(".agnostic-ai", "rules")); err != nil {
		t.Fatal(err)
	}
	if _, err := importCodexRules(".", filepath.Join(".agnostic-ai", "rules"), config.Sources{}, importCodexOpts{}); err != nil {
		t.Fatal(err)
	}

	// The local layer is the source of that text; only the shared specs
	// must stay free of it.
	err := filepath.WalkDir(".agnostic-ai", func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path == filepath.FromSlash(defaultProjectUser) {
			return filepath.SkipDir
		}
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "Local line.") {
			t.Errorf("%s absorbed the local instructions:\n%s", path, data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWatchDirs_IncludesProjectLocalDir(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"), localInstructions)

	cfg, _, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(".", defaultProjectUser)
	for _, p := range watchDirs(".", cfg) {
		if p == want {
			return
		}
	}
	t.Errorf("watchDirs missing %s\ngot %v", want, watchDirs(".", cfg))
}

func TestPlanWatchResync_ProjectLocalInstructionsForceFull(t *testing.T) {
	dir := setupIncrementalFixture(t)
	testutil.Chdir(t, dir)

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md")
	plan := planWatchResync(".", cfg, b, []string{local}, []string{"claude", "cursor"})
	if !plan.full {
		t.Errorf("a local instructions edit must force a full re-sync, got %+v", plan)
	}
	if strings.Contains(plan.reason, "unrecognized") {
		t.Errorf("local instructions are a known input, got reason %q", plan.reason)
	}
}

// `outputs.<target>.rules-file` naming the target's own entry point is the
// legacy layout: the adapter owns that file and the central writer skips
// it. The local text still belongs in the file the tool reads.
func TestSync_AppendsProjectLocalInstructionsToLegacyRulesFileEntryPoint(t *testing.T) {
	for _, tc := range []struct{ target, path string }{
		{"claude", "CLAUDE.md"},
		{"codex", "AGENTS.md"},
		{"gemini", "GEMINI.md"},
		{"copilot", filepath.Join(".github", "copilot-instructions.md")},
	} {
		t.Run(tc.target, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\noutputs:\n  "+tc.target+":\n    rules-file: "+filepath.ToSlash(tc.path)+"\n")
			writeFile(t, filepath.Join(".agnostic-ai", "rules", "r1.md"), "---\nname: r1\n---\nRule line.\n")
			writeAgnosticFile(t, sharedInstructions)
			writeFile(t, filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"), "Local line.\n\n::target "+tc.target+"\nFenced line.\n::end\n\n::target cursor\nForeign line.\n::end\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("%s not written: %v", tc.path, err)
			}
			got := string(data)
			rules, local := strings.Index(got, "Rule line."), strings.Index(got, "Local line.")
			if rules < 0 || local < 0 || local < rules {
				t.Errorf("%s: want the local text after the rule bodies:\n%s", tc.path, got)
			}
			if !strings.Contains(got, adapters.LocalStartMarker) || !strings.Contains(got, "Fenced line.") {
				t.Errorf("%s: want the marked local block with its %s fence:\n%s", tc.path, tc.target, got)
			}
			if strings.Contains(got, "Foreign line.") {
				t.Errorf("%s: a fence for another target leaked:\n%s", tc.path, got)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check after sync: %v\n%s", err, out)
			}
		})
	}
}

// Two targets whose legacy rules-file is one shared AGENTS.md write the
// same merged document. A local fence for one of them must reach both
// writes alike, as in the central shared file, or they collide.
func TestSync_SharedLegacyRulesFileKeepsOneLocalView(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, amp]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n  amp:\n    rules-file: AGENTS.md\n")
	writeFile(t, filepath.Join(".agnostic-ai", "rules", "r1.md"), "---\nname: r1\n---\nRule line.\n")
	writeAgnosticFile(t, sharedInstructions)
	writeFile(t, filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"), "Local line.\n\n::target codex\nCodex line.\n::end\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	data, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Codex line.") {
		t.Errorf("AGENTS.md: want the codex fence kept for its codex reader:\n%s", data)
	}
}
