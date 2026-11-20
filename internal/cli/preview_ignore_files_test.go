package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncPlan_ListsTheIgnoreFilesSyncWrites(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	gitInit(t, dir)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [claude, codex]\ngitignore:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	planned, err := cliOutput(t, "sync", "--plan", "--json")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, path := range []string{".gitignore", ".worktreeinclude"} {
		if !strings.Contains(planned, `"path": "`+path+`"`) {
			t.Errorf("plan does not list %s:\n%s", path, planned)
		}
	}
	if _, err := os.Stat(".gitignore"); !os.IsNotExist(err) {
		t.Errorf("plan wrote .gitignore, err=%v", err)
	}

	execCLI(t, "sync")
	planned, err = cliOutput(t, "sync", "--plan", "--json")
	if err != nil {
		t.Fatalf("plan after sync: %v", err)
	}
	if strings.Contains(planned, ".gitignore") || strings.Contains(planned, ".worktreeinclude") {
		t.Errorf("plan after a sync still lists an ignore file:\n%s", planned)
	}
	if _, err := cliOutput(t, "sync", "--plan", "--check"); err != nil {
		t.Errorf("plan --check after sync: %v", err)
	}
}

func TestSyncPlan_IgnoreFilesWriteNothingAndNeverFailThePlan(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	gitInit(t, dir)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [claude, codex]\ngitignore:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")

	// The config turned off, the flag turned on: the block stays as is.
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [claude, codex]\ngitignore:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, ".git", "info", "agnostic-ai-exclude.lock")
	if err := os.Remove(lock); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	planned, err := cliOutput(t, "sync", "--plan", "--json", "--gitignore", "on")
	if err != nil || strings.Contains(planned, ".gitignore") {
		t.Errorf("plan --gitignore on lists the unchanged .gitignore (%v):\n%s", err, planned)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("plan left an exclude lock file, err=%v", err)
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(data), gitignoreBlockEnd, "", 1)
	if err := os.WriteFile(".gitignore", []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cliOutput(t, "sync", "--plan", "--check", "--gitignore", "on"); err != nil {
		t.Errorf("a broken managed block failed plan --check: %v", err)
	}
}

func TestInitCmd_DryRunListsTheGitignore(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	captureLogOut(t)

	out, err := runInitCapturingStdout(t, "--all", "--dry-run")
	if err != nil {
		t.Fatalf("init --dry-run: %v", err)
	}
	if !strings.Contains(out, "create: .gitignore") {
		t.Errorf("dry run does not list .gitignore:\n%s", out)
	}
	if _, err := os.Stat(".gitignore"); !os.IsNotExist(err) {
		t.Errorf("dry run wrote .gitignore, err=%v", err)
	}
}

func TestInitCmd_ExistingProjectFailsBeforeTheTargetChoice(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	captureLogOut(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr := &strings.Builder{}
	root := NewRootCmd("test")
	root.SetIn(strings.NewReader(""))
	root.SetErr(stderr)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("init on an existing project: err=%v", err)
	}
	if strings.Contains(stderr.String(), "enabled") {
		t.Errorf("init printed a target choice before failing:\n%s", stderr)
	}
}
