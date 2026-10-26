package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestGlobalSettings_SkipSpecsRoutedAwayFromTarget(t *testing.T) {
	entries := []spec.Entry{
		{Path: "settings/defaults.yaml", Meta: map[string]any{
			"model":    map[string]any{"claude": "opus", "codex": "gpt-6-luna"},
			"effort":   "medium",
			"x-claude": map[string]any{"env": map[string]any{"DEMO": "kept"}},
		}},
		{Path: "settings/codex.yaml", Meta: map[string]any{
			"targets":  []any{"codex"},
			"model":    "gpt-6-mini",
			"effort":   "high",
			"x-claude": map[string]any{"env": map[string]any{"DEMO": "excluded"}},
		}},
		{Path: "settings/not-claude.yaml", Meta: map[string]any{
			"targets-exclude": []any{"claude"},
			"model":           "gpt-6-nano",
		}},
	}
	for target, want := range map[string]map[string]string{
		"claude": {"model": "opus", "effortLevel": "medium", "env.DEMO": "kept"},
		"codex":  {"model": "gpt-6-nano", "model_reasoning_effort": "high"},
	} {
		got := map[string]string{}
		sources := map[string]string{}
		for _, s := range globalSettingsFor(target, globalTargets[target], entries) {
			got[s.key] = s.value.(string)
			sources[s.key] = s.source
		}
		if len(got) != len(want) {
			t.Errorf("%s settings = %v, want %v", target, got, want)
		}
		for key, value := range want {
			if got[key] != value {
				t.Errorf("%s %s = %q, want %q", target, key, got[key], value)
			}
		}
		if target == "claude" && (sources["model"] != "settings/defaults.yaml" || sources["effortLevel"] != "settings/defaults.yaml") {
			t.Errorf("claude sources = %v, want the eligible spec", sources)
		}
		if target == "codex" && sources["model"] != "settings/not-claude.yaml" {
			t.Errorf("codex model source = %q, want the last eligible spec", sources["model"])
		}
	}
}

func TestGlobalSettings_LintSkipsEffortRoutedAway(t *testing.T) {
	entries := []spec.Entry{{Path: "settings/codex.yaml", Meta: map[string]any{"targets": []any{"codex"}, "effort": "ludicrous"}}}
	if issues := lintGlobalSettings(entries, []string{"claude"}); len(issues) != 0 {
		t.Errorf("a Codex-only effort must not be checked against Claude: %v", issues)
	}
}

func TestSyncGlobal_SettingsHonorTargetFilters(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), "model: opus\neffort: medium\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "codex.yaml"),
		"name: codex\ntargets: [codex]\nmodel: should-not-reach-claude\neffort: high\nx-claude:\n  env:\n    DEMO: excluded-value\n")
	claudePath := filepath.Join(home, ".claude", "settings.json")

	plan, _, err := runGlobalAgentTest("--only", "claude", "--plan")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if strings.Contains(plan, "should-not-reach-claude") || strings.Contains(plan, "DEMO") || !strings.Contains(plan, `"opus"`) {
		t.Errorf("plan must list only eligible values:\n%s", plan)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	var claude map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, claudePath)), &claude); err != nil {
		t.Fatal(err)
	}
	if claude["model"] != "opus" || claude["effortLevel"] != "medium" || claude["env"] != nil {
		t.Errorf("claude settings = %v, want only the unrestricted spec", claude)
	}
	if _, _, err := runGlobalCheck("sync", "--only", "claude", "--check"); err != nil {
		t.Errorf("check after sync must agree: %v", err)
	}

	if _, warnings, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync codex: %v\n%s", err, warnings)
	}
	want := "model = \"should-not-reach-claude\"\nmodel_reasoning_effort = \"high\"\n"
	if got := readGlobalTest(t, filepath.Join(home, ".codex", "config.toml")); got != want {
		t.Errorf("config.toml = %q, want %q", got, want)
	}
}

func TestSyncGlobal_SettingsRoutedAwayRemoveOwnedValue(t *testing.T) {
	home, source := globalAgentTestHome(t)
	defaults := filepath.Join(source, "settings", "defaults.yaml")
	mustWriteGlobalTest(t, defaults, "model: opus\n")
	claudePath := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, claudePath, "{\n  \"theme\": \"dark\"\n}\n")

	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if !strings.Contains(readGlobalTest(t, claudePath), `"model": "opus"`) {
		t.Fatalf("first sync must write the model:\n%s", readGlobalTest(t, claudePath))
	}
	mustWriteGlobalTest(t, defaults, "targets: [codex]\nmodel: opus\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("resync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, claudePath); got != "{\n  \"theme\": \"dark\"\n}\n" {
		t.Errorf("routing the spec away must restore the original bytes, got %q", got)
	}
}

func TestExplainGlobal_SkipsTargetsASettingsSpecExcludes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"),
		"targets: [codex]\nmodel: gpt-6-mini\neffort: high\nx-claude:\n  env:\n    DEMO: excluded-value\n")

	out, _, err := runGlobalCheck("explain", "settings/defaults.yaml")
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, out)
	}
	if strings.Contains(out, "[claude]") || !strings.Contains(out, filepath.ToSlash(filepath.Join(home, ".codex", "config.toml"))+` (key "model")`) {
		t.Errorf("explain must name only Codex keys:\n%s", out)
	}
}

func TestImportGlobal_ReadsModelASettingsSpecRoutesAway(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "codex.yaml"), "targets: [codex]\nmodel: gpt-6-mini\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), "{\"model\": \"sonnet\"}\n")

	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(source, "settings", "imported.yaml")); !strings.Contains(got, "claude: sonnet") {
		t.Errorf("a Codex-only model must not stop the Claude import:\n%s", got)
	}
}
