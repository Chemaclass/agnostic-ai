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

	err := filepath.WalkDir(".agnostic-ai", func(path string, d fs.DirEntry, err error) error {
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
