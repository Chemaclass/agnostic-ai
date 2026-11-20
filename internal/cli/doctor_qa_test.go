package cli

import (
	"bytes"
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
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
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
	writeFile(t, filepath.Join(".codex", "hooks.json"), `{"hooks":{}}`)

	findings := collectCodexHookTrust(cfg, nil)
	if len(findings) != 1 || !strings.Contains(findings[0].Problem, "CODEX_HOME") {
		t.Errorf("project hooks with an unreadable CODEX_HOME: findings = %v", findings)
	}
}

func TestDoctorFix_SavesAHandEditedFileAsBakBeforeWriting(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\nalwaysApply: true\n---\nKeep it short.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	original, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	edited := string(original) + "my hand edit\n"
	writeFile(t, "CLAUDE.md", edited)

	var logged bytes.Buffer
	previous := logOut
	logOut = &logged
	t.Cleanup(func() { logOut = previous })
	if out, err := runCLI(t, "doctor", "--fix", "--scope", "project"); err != nil {
		t.Fatalf("doctor --fix: %v\n%s", err, out)
	}
	if !strings.Contains(logged.String(), "overwrote a hand edit to CLAUDE.md (saved as CLAUDE.md.bak)") {
		t.Errorf("doctor --fix did not say it saved the edit:\n%s", logged.String())
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
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	writeFile(t, "CLAUDE.md.bak", "an earlier copy I keep\n")
	writeFile(t, "CLAUDE.md", "my hand edit\n")

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

func TestDoctorFix_SaysItSavedARemovedHandEdit(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	spec := filepath.Join(dir, ".agnostic-ai", "rules", "style.md")
	writeFile(t, spec, "---\nname: style\nglobs: \"*.go\"\n---\nKeep it short.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	generated := filepath.Join(".claude", "rules", "style.md")
	body, err := os.ReadFile(generated)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, generated, string(body)+"my hand edit\n")
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}

	var logged bytes.Buffer
	previous := logOut
	logOut = &logged
	t.Cleanup(func() { logOut = previous })
	out, _ := runCLI(t, "doctor", "--fix", "--scope", "project")
	if _, err := os.Stat(generated + ".bak"); err != nil {
		t.Fatalf("doctor --fix kept the edited leftover instead of removing it:\n%s\n%s", out, logged.String())
	}
	want := "removed a hand edit to " + generated + " (saved as " + generated + ".bak)"
	if !strings.Contains(filepath.ToSlash(logged.String()), filepath.ToSlash(want)) {
		t.Errorf("doctor --fix did not say it saved the removed edit:\n%s", logged.String())
	}
}

// doctor --fix shares sync's edit guard, so an edit made through a
// symlinked output is saved too (#3736).
func TestDoctorFix_SavesAHandEditMadeThroughASymlinkedOutput(t *testing.T) {
	handEditProject(t, "claude")
	generated, edited := linkHandEditOutput(t, handEditSkill)
	var logged bytes.Buffer
	previous := logOut
	logOut = &logged
	t.Cleanup(func() { logOut = previous })

	if out, err := runCLI(t, "doctor", "--fix", "--scope", "project"); err != nil {
		t.Fatalf("doctor --fix: %v\n%s", err, out)
	}

	if got, _ := os.ReadFile(handEditSkill + ".bak"); string(got) != edited {
		t.Errorf(".bak = %q, want the hand edit", got)
	}
	if got, _ := os.ReadFile("notes/linked.md"); string(got) != generated {
		t.Errorf("link target = %q, want the spec's version", got)
	}
	if want := "overwrote a hand edit to " + handEditSkill + " (saved as " + handEditSkill + ".bak)"; !strings.Contains(logged.String(), want) {
		t.Errorf("doctor --fix output does not say %q:\n%s", want, logged.String())
	}
}
