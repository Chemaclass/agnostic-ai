package integration

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorEnvironment_ReportsCloudTargetsAndGitStatus(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor, copilot, codex, openhands, amp, ellipsis, gemini]\ngitignore:\n  enabled: true\n  commit: [cursor:environments]\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\nx-copilot:\n  setup-steps: true\n")
	run := func(args ...string) string {
		t.Helper()
		out, err := ellipsisCommand(t, binary, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run("sync")
	handoffGit(t, dir, "add", ".cursor/environment.json")
	before := sourceSnapshot(t, dir)
	out := run("doctor", "environment")
	for _, want := range []string{"Claude Code", "Cursor", "GitHub Copilot", "Codex Cloud", "OpenHands", "Amp", "Ellipsis", "gitignore.commit: [cloud], which includes claude:environments, claude:hooks", ".cursor/environment.json (tracked)", "setup-steps.yml", ".openhands/setup.sh", ".agents/setup", "project-readiness"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "npm ci") != 2 || !strings.Contains(out, "Paste into Setup script") || !strings.Contains(out, "Paste into Install script") {
		t.Errorf("missing Claude Code or Codex Cloud install instructions:\n%s", out)
	}
	if strings.Contains(out, "gemini") {
		t.Errorf("non-cloud target in report:\n%s", out)
	}
	if !maps.Equal(sourceSnapshot(t, dir), before) {
		t.Error("doctor changed the repository")
	}
}

func TestDoctorEnvironment_FiltersTargetsAndWorksBeforeSync(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor, codex]\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\nx-codex:\n  install: pnpm install\n")
	t.Setenv("NPM_TOKEN", "DO-NOT-PRINT-THIS-VALUE")
	out, err := ellipsisCommand(t, binary, dir, "doctor", "environment", "-t", "codex", "--json")
	if err != nil {
		t.Fatalf("doctor environment --json: %v\n%s", err, out)
	}
	var report struct {
		Targets []struct {
			Target  string `json:"target"`
			Install string `json:"install"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	if len(report.Targets) != 1 || report.Targets[0].Target != "codex" || report.Targets[0].Install != "pnpm install" {
		t.Errorf("unexpected report:\n%s", out)
	}
	if strings.Contains(out, "DO-NOT-PRINT-THIS-VALUE") {
		t.Error("report printed a local variable value")
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex")); !os.IsNotExist(err) {
		t.Errorf("doctor wrote files: %v", err)
	}
}

func TestDoctorEnvironment_ReportsExistingNativeSetupWithoutSpecs(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [cursor, copilot, claude]\n")
	writeProjectFile(t, dir, ".cursor/environment.json", `{"install":"npm ci"}`)
	writeProjectFile(t, dir, ".github/workflows/copilot-setup-steps.yml", "jobs: {}\n")
	writeProjectFile(t, dir, ".claude/settings.json", `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"sh setup.sh"}]}]}}`)
	handoffGit(t, dir, "add", ".cursor/environment.json")
	out, err := ellipsisCommand(t, binary, dir, "doctor", "environment")
	if err != nil {
		t.Fatalf("doctor environment: %v\n%s", err, out)
	}
	for _, want := range []string{".cursor/environment.json (tracked)", ".github/workflows/copilot-setup-steps.yml (untracked)", ".claude/settings.json (untracked)"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
}

func TestDoctorEnvironment_ShowsNativePathsForRedirectedOutputs(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	writeProjectFile(t, dir, "agnostic-ai.yaml", `version: 1
targets: [cursor, copilot, openhands]
outputs:
  cursor:
    environment-file: preview/cursor.json
  copilot:
    setup-file: preview/copilot.yml
  openhands:
    setup-file: preview/openhands.sh
`)
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\nx-copilot:\n  setup-steps: true\n")
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	out, err := ellipsisCommand(t, binary, dir, "doctor", "environment")
	if err != nil {
		t.Fatalf("doctor environment: %v\n%s", err, out)
	}
	for _, want := range []string{
		"preview/cursor.json (untracked, inspection only)", ".cursor/environment.json (missing)",
		"preview/copilot.yml (untracked, inspection only)", ".github/workflows/copilot-setup-steps.yml (missing)",
		"preview/openhands.sh (untracked, inspection only)", ".openhands/setup.sh (missing)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
}

// With a hand-written .gitignore, doctor environment prints the lines to
// change so each ignored setup file becomes trackable: a folder line
// turns into `/dir/*` with `!` exceptions, and a `**/NAME` line gets a
// root `!/NAME` after it. Applying them leaves nothing to change.
func TestDoctorEnvironment_PrintsGitignoreLinesForHandWrittenIgnores(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude, openhands]\ngitignore:\n  enabled: false\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\n")
	writeProjectFile(t, dir, ".gitignore", "# generated\n/.claude/\n**/setup.sh\n")
	run := func(args ...string) string {
		t.Helper()
		out, err := ellipsisCommand(t, binary, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run("sync")
	out := run("doctor", "environment")
	want := `Ignore file changes
  Sync does not manage your ignore files. Make these edits, then commit the files above. Line numbers refer to the file as it is now.
  .gitignore line 2: replace
    /.claude/
  with
    /.claude/*
    !/.claude/hooks/
    /.claude/hooks/*
    !/.claude/hooks/agnostic-ai-cloud-install.sh
    !/.claude/settings.json
  .gitignore after line 3 (**/setup.sh): add
    !/.openhands/setup.sh
`
	if !strings.Contains(out, want) {
		t.Errorf("report lacks the ignore file changes:\n%s", out)
	}
	if strings.Contains(out, "Set gitignore.commit") {
		t.Errorf("report gives managed-ignore advice for a hand-written .gitignore:\n%s", out)
	}
	var report struct {
		ManagedIgnores bool `json:"managed_ignores"`
		Edits          []struct {
			Source string   `json:"source"`
			Line   int      `json:"line"`
			Add    []string `json:"add"`
		} `json:"gitignore_edits"`
	}
	if err := json.Unmarshal([]byte(run("doctor", "environment", "--json")), &report); err != nil {
		t.Fatal(err)
	}
	if report.ManagedIgnores || len(report.Edits) != 2 || report.Edits[1].Line != 3 {
		t.Errorf("JSON edits = %+v", report)
	}

	writeProjectFile(t, dir, ".gitignore", "# generated\n/.claude/*\n!/.claude/hooks/\n/.claude/hooks/*\n!/.claude/hooks/agnostic-ai-cloud-install.sh\n!/.claude/settings.json\n**/setup.sh\n!/.openhands/setup.sh\n")
	out = run("doctor", "environment")
	if strings.Contains(out, "(ignored)") || strings.Contains(out, "Ignore file changes") {
		t.Errorf("report still finds ignored files after the edits:\n%s", out)
	}
	handoffGit(t, dir, "add", "-A")
	for _, rel := range []string{".claude/settings.json", ".claude/hooks/agnostic-ai-cloud-install.sh", ".openhands/setup.sh"} {
		if strings.TrimSpace(handoffGit(t, dir, "ls-files", rel)) != rel {
			t.Errorf("git add did not stage %s", rel)
		}
	}
}

func TestDoctorEnvironment_SuggestsOneCommitKindWhenOnlyOneApplies(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [openhands]\ngitignore:\n  enabled: true\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\n")
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	out, err := ellipsisCommand(t, binary, dir, "doctor", "environment")
	if err != nil {
		t.Fatalf("doctor environment: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Set gitignore.commit: [openhands:environments] and commit the files.") || strings.Contains(out, "Ignore file changes") {
		t.Errorf("unexpected managed-ignore advice:\n%s", out)
	}
}
