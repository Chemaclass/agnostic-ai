package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runInitPiped(t *testing.T, stdin string, args ...string) error {
	t.Helper()
	root := NewRootCmd("test")
	root.SetIn(strings.NewReader(stdin))
	root.SetErr(&strings.Builder{})
	root.SetArgs(append([]string{"init"}, args...))
	return root.Execute()
}

func TestInitCmd_CreatesStarterNoteThatSyncShares(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)

	if err := runInitPiped(t, "claude,codex\n"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if got := readFile(t, adapters.AgnosticEntryPointPath); got != adapters.EntryPointBody() {
		t.Fatalf("%s = %q, want the starter note", adapters.AgnosticEntryPointPath, got)
	}
	note := "Run go test ./... before every commit."
	if err := os.WriteFile(filepath.FromSlash(adapters.AgnosticEntryPointPath), []byte(note+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sync := NewRootCmd("test")
	sync.SetArgs([]string{"sync"})
	if err := sync.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, note) {
		t.Errorf("AGENTS.md should carry the note, got:\n%s", got)
	}
	if got := readFile(t, "CLAUDE.md"); !strings.Contains(got, "@AGENTS.md") {
		t.Errorf("CLAUDE.md should point to AGENTS.md, got:\n%s", got)
	}
}

func TestInitCmd_StarterNoteForEveryLayout(t *testing.T) {
	for _, args := range [][]string{{"--all"}, {"--all", "."}, {"--all", "config/ai"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			if err := runInitPiped(t, "", args...); err != nil {
				t.Fatalf("init: %v", err)
			}
			if got := readFile(t, adapters.AgnosticEntryPointPath); got != adapters.EntryPointBody() {
				t.Errorf("%s = %q, want the starter note", adapters.AgnosticEntryPointPath, got)
			}
		})
	}
}

func TestInitCmd_KeepsAnExistingNote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeScopedFile(t, adapters.AgnosticEntryPointPath, "Mine.\n")

	if err := runInitPiped(t, "claude\n"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if got := readFile(t, adapters.AgnosticEntryPointPath); got != "Mine.\n" {
		t.Errorf("init replaced the existing note: %q", got)
	}
}

func TestInitCmd_FromKeepsImportedInstructions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeScopedFile(t, "CLAUDE.md", "# Team\n\nUse tabs.\n")

	if err := runInitPiped(t, "claude\n", "--from", "claude"); err != nil {
		t.Fatalf("init --from claude: %v", err)
	}
	got := readFile(t, adapters.AgnosticEntryPointPath)
	if !strings.Contains(got, "Use tabs.") || strings.Contains(got, "Replace this comment") {
		t.Errorf("%s should hold only the imported text, got:\n%s", adapters.AgnosticEntryPointPath, got)
	}
}

func TestInitCmd_DryRunListsStarterNote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	captureLogOut(t)

	out, err := runInitCapturingStdout(t, "--all", "--dry-run")
	if err != nil {
		t.Fatalf("init --dry-run: %v", err)
	}
	if want := "create: " + filepath.FromSlash(adapters.AgnosticEntryPointPath); !strings.Contains(out, want) {
		t.Errorf("output should contain %q, got:\n%s", want, out)
	}
	if _, err := os.Stat(".agnostic-ai"); !os.IsNotExist(err) {
		t.Errorf("dry run created .agnostic-ai (stat err: %v)", err)
	}
}

func TestInitCmd_DryRunSkipsAnExistingNote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	captureLogOut(t)
	writeScopedFile(t, adapters.AgnosticEntryPointPath, "Mine.\n")

	out, err := runInitCapturingStdout(t, "--all", "--dry-run")
	if err != nil {
		t.Fatalf("init --dry-run: %v", err)
	}
	if strings.Contains(out, "AGNOSTIC_AI.md") {
		t.Errorf("dry run should not list an existing note, got:\n%s", out)
	}
}

func TestImportAll_ReplacesTheStarterNoteWithARootAgentsMainFile(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	captureSummary(t)
	body := "# Repository Guidelines\n\nRun make lint before pushing.\n"
	writeScopedFile(t, "AGENTS.md", body)
	if err := runInitPiped(t, "claude\n"); err != nil {
		t.Fatalf("init: %v", err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "all"})
	if err := root.Execute(); err != nil {
		t.Fatalf("import all: %v", err)
	}
	if got := readFile(t, adapters.AgnosticEntryPointPath); got != body {
		t.Errorf("%s\ngot  %q\nwant %q", adapters.AgnosticEntryPointPath, got, body)
	}
}

// In the home folder, .agnostic-ai/AGNOSTIC_AI.md is the global
// instructions file, which sync --global shares with every project.
func TestInitCmd_LeavesTheGlobalNoteAloneInTheHomeFolder(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envUserGlobalRoot, filepath.Join(dir, ".agnostic-ai"))

	if err := runInitPiped(t, "claude\n"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(adapters.AgnosticEntryPointPath); !os.IsNotExist(err) {
		t.Errorf("init wrote the global note (stat err: %v)", err)
	}
}

func TestInitCmd_DryRunLeavesTheGlobalNoteOutThroughALinkedHome(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	testutil.Chdir(t, real)
	captureLogOut(t)
	t.Setenv(envUserGlobalRoot, filepath.Join(link, ".agnostic-ai"))

	out, err := runInitCapturingStdout(t, "--all", "--dry-run")
	if err != nil {
		t.Fatalf("init --dry-run: %v", err)
	}
	if strings.Contains(out, "AGNOSTIC_AI.md") {
		t.Errorf("dry run listed the global note, got:\n%s", out)
	}
}
