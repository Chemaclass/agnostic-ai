package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func codexProjectWithMissingCodexHome(t *testing.T) *config.Config {
	t.Helper()
	dir := testutil.TempCwd(t)
	t.Setenv("CODEX_HOME", filepath.Join(dir, "codex-home-not-made-yet"))
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestCodexHookTrust_SkipsAMissingCodexHomeWhenNoHooksFileExists(t *testing.T) {
	cfg := codexProjectWithMissingCodexHome(t)

	if findings := collectCodexHookTrust(cfg, nil); len(findings) != 0 {
		t.Errorf("no hooks file, yet doctor reports hook trust: %v", findings)
	}
}

func TestCodexHookTrust_ReportsAMissingCodexHomeWhenTheProjectHasHooks(t *testing.T) {
	cfg := codexProjectWithMissingCodexHome(t)
	mustWriteFile(t, filepath.Join(".codex", "hooks.json"), `{"hooks":{}}`)

	findings := collectCodexHookTrust(cfg, nil)
	if len(findings) != 1 || !strings.Contains(findings[0].Problem, "CODEX_HOME") {
		t.Errorf("project hooks with an unreadable CODEX_HOME: findings = %v", findings)
	}
}

func TestDoctorFix_SavesAHandEditedFileAsBakBeforeWriting(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\nalwaysApply: true\n---\nKeep it short.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	original, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	edited := string(original) + "my hand edit\n"
	mustWriteFile(t, "CLAUDE.md", edited)

	if out, err := runCLI(t, "doctor", "--fix", "--scope", "project"); err != nil {
		t.Fatalf("doctor --fix: %v\n%s", err, out)
	}
	backup, err := os.ReadFile("CLAUDE.md.bak")
	if err != nil {
		t.Fatalf("doctor --fix left no backup of the hand edit: %v", err)
	}
	if string(backup) != edited {
		t.Errorf("backup = %q, want the edited file", backup)
	}
	if got, _ := os.ReadFile("CLAUDE.md"); string(got) != string(original) {
		t.Errorf("doctor --fix did not restore the generated file:\n%s", got)
	}
	if _, ok := readStateFile(".").Backups["CLAUDE.md.bak"]; !ok {
		t.Errorf("state file does not record the backup: %v", readStateFile(".").Backups)
	}
}

func TestDoctorFix_FailsWhenAnEarlierBakBlocksTheBackup(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	mustWriteFile(t, "CLAUDE.md.bak", "an earlier copy I keep\n")
	mustWriteFile(t, "CLAUDE.md", "my hand edit\n")

	if out, err := runCLI(t, "doctor", "--fix", "--scope", "project"); err == nil {
		t.Fatalf("doctor --fix passed while it kept the edit:\n%s", out)
	}
	if got, _ := os.ReadFile("CLAUDE.md"); string(got) != "my hand edit\n" {
		t.Errorf("doctor --fix overwrote an edit it could not back up:\n%s", got)
	}
	if got, _ := os.ReadFile("CLAUDE.md.bak"); string(got) != "an earlier copy I keep\n" {
		t.Errorf("doctor --fix replaced the earlier .bak:\n%s", got)
	}
}
