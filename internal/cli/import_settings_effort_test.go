package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func importEffortProject(t *testing.T, target string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
	return dir
}

func assertSettingsEffort(t *testing.T, dir, file, want string) {
	t.Helper()
	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", file))
	if !strings.Contains(got, "effort: "+want) {
		t.Errorf("settings/%s must carry effort: %s:\n%s", file, want, got)
	}
}

func TestImportClaude_PromotesEffortLevelIntoSettings(t *testing.T) {
	dir := importEffortProject(t, "claude")
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{"effortLevel": "high", "theme": "dark"}`)

	execCLI(t, "import", "claude")

	assertSettingsEffort(t, dir, "claude.yaml", "high")
	overlay := readFile(t, filepath.Join(dir, ".agnostic-ai", "overlays", "claude.settings.json"))
	if strings.Contains(overlay, "effortLevel") || !strings.Contains(overlay, `"theme"`) {
		t.Errorf("the overlay must drop effortLevel and keep other keys:\n%s", overlay)
	}
	execCLI(t, "sync")
	if got := readFile(t, filepath.Join(dir, ".claude", "settings.json")); !strings.Contains(got, `"effortLevel": "high"`) {
		t.Errorf("sync must write the promoted effort back:\n%s", got)
	}
	execCLI(t, "sync", "--check")

	execCLI(t, "import", "claude")
	assertSettingsEffort(t, dir, "claude.yaml", "high")
}

func TestImportClaude_KeepsARejectedEffortInTheOverlay(t *testing.T) {
	dir := importEffortProject(t, "claude")
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{"effortLevel": "max", "theme": "dark"}`)

	execCLI(t, "import", "claude")

	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "settings", "claude.yaml")); !os.IsNotExist(err) {
		t.Errorf("Claude rejects max, so nothing is promoted; stat err = %v", err)
	}
	overlay := readFile(t, filepath.Join(dir, ".agnostic-ai", "overlays", "claude.settings.json"))
	if !strings.Contains(overlay, `"effortLevel": "max"`) {
		t.Errorf("the overlay must keep the effort sync cannot rebuild:\n%s", overlay)
	}
	execCLI(t, "sync")
	if got := readFile(t, filepath.Join(dir, ".claude", "settings.json")); !strings.Contains(got, `"effortLevel": "max"`) {
		t.Errorf("sync must keep writing the native value:\n%s", got)
	}
}

func TestImportClaude_ShadowedEffortSurvivesUnderXClaude(t *testing.T) {
	dir := importEffortProject(t, "claude")
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{"effortLevel": "high", "theme": "dark"}`)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "team.yaml"), "effort: low\n")

	execCLI(t, "import", "claude")

	spec := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "claude.yaml"))
	if !strings.Contains(spec, "x-claude:") || !strings.Contains(spec, "effortLevel: high") || strings.Contains(spec, "effort: high") {
		t.Errorf("a shadowed effort must stay Claude-only under x-claude:\n%s", spec)
	}
	execCLI(t, "sync")
	if got := readFile(t, filepath.Join(dir, ".claude", "settings.json")); !strings.Contains(got, `"effortLevel": "high"`) {
		t.Errorf("the imported value must beat the team default for Claude:\n%s", got)
	}
}

func TestImportAll_KeepsEachToolsOwnEffort(t *testing.T) {
	dir := importEffortProject(t, "claude, copilot")
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Claude\n")
	writeFile(t, filepath.Join(dir, ".github", "copilot-instructions.md"), "# Copilot\n")
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{"effortLevel": "high"}`)
	writeFile(t, filepath.Join(dir, ".github", "copilot", "settings.json"), `{"effortLevel": "xhigh"}`)

	execCLI(t, "import", "all")
	execCLI(t, "sync")

	if got := readFile(t, filepath.Join(dir, ".claude", "settings.json")); !strings.Contains(got, `"effortLevel": "high"`) {
		t.Errorf("Claude effort changed across import and sync:\n%s", got)
	}
	if got := readFile(t, filepath.Join(dir, ".github", "copilot", "settings.json")); !strings.Contains(got, `"effortLevel": "xhigh"`) {
		t.Errorf("Copilot effort changed across import and sync:\n%s", got)
	}
}

func TestImportCodex_PromotesTopLevelEffortOnly(t *testing.T) {
	dir := importEffortProject(t, "codex")
	writeFile(t, filepath.Join(dir, ".codex", "config.toml"), `model = "gpt-6-sol"
model_reasoning_effort = "ultra"

[profiles.fast]
model_reasoning_effort = "low"
`)

	execCLI(t, "import", "codex")

	assertSettingsEffort(t, dir, "codex.yaml", "ultra")
	overlay := readFile(t, filepath.Join(dir, ".agnostic-ai", "overlays", "codex.config.toml"))
	if strings.Contains(overlay, `"ultra"`) || !strings.Contains(overlay, `model = "gpt-6-sol"`) || !strings.Contains(overlay, `model_reasoning_effort = "low"`) {
		t.Errorf("the overlay must drop only the top-level effort:\n%s", overlay)
	}
	execCLI(t, "sync")
	got := readFile(t, filepath.Join(dir, ".codex", "config.toml"))
	if strings.Count(got, `model_reasoning_effort = "ultra"`) != 1 || !strings.Contains(got, `model_reasoning_effort = "low"`) {
		t.Errorf("sync must write the promoted effort once and keep the profile's:\n%s", got)
	}
}

func TestImportCopilot_PromotesEffortLevelIntoSettings(t *testing.T) {
	dir := importEffortProject(t, "copilot")
	writeFile(t, filepath.Join(dir, ".github", "copilot", "settings.json"), `{"model": "gpt-6-sol", "effortLevel": "xhigh"}`)

	execCLI(t, "import", "copilot")

	assertSettingsEffort(t, dir, "imported.yaml", "xhigh")
}

func TestImportFactory_PromotesReasoningEffortIntoSettings(t *testing.T) {
	dir := importEffortProject(t, "factory")
	writeFile(t, filepath.Join(dir, ".factory", "settings.json"), `{"model": "claude-opus-5-5", "reasoningEffort": "max"}`)

	execCLI(t, "import", "factory")

	assertSettingsEffort(t, dir, "factory.yaml", "max")
}
