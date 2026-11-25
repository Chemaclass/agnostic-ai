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
	for _, want := range []string{"Claude Code", "Cursor", "GitHub Copilot", "Codex Cloud", "OpenHands", "Amp", "Ellipsis", "gitignore.commit: [claude:environments", ".cursor/environment.json (tracked)", "setup-steps.yml", ".openhands/setup.sh", ".agents/setup", "project-readiness"} {
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
