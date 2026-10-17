package cli

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func strongTier() map[string]config.ModelTier {
	return map[string]config.ModelTier{"strong": {
		Models: map[string]string{"claude": "opus", "codex": "gpt-5.5"},
		Effort: map[string]any{"claude": "xhigh", "codex": "high"},
	}}
}

func TestMergeCodexAgentIntoExisting_KeepsATierThatMatches(t *testing.T) {
	existing := "---\nname: reviewer\nmodel: strong\n---\n\nReview.\n"
	doc := map[string]any{"model": "gpt-5.5", "model_reasoning_effort": "high"}
	out, err := mergeCodexAgentIntoExisting(existing, "reviewer", doc, strongTier())
	if err != nil {
		t.Fatal(err)
	}
	fm := parseAgentFrontmatter(t, out)
	if fm["model"] != "strong" {
		t.Errorf("model = %#v, want the tier name", fm["model"])
	}
	if _, set := fm["x-codex"]; set {
		t.Errorf("a Codex value the tier gives must not pin Codex: %#v", fm["x-codex"])
	}

	out, err = mergeCodexAgentIntoExisting(existing, "reviewer", map[string]any{"model": "o4-mini"}, strongTier())
	if err != nil {
		t.Fatal(err)
	}
	if xcodex, _ := parseAgentFrontmatter(t, out)["x-codex"].(map[string]any); xcodex["model"] != "o4-mini" {
		t.Errorf("a Codex model that differs from the tier must still import: %s", out)
	}
}

func TestImportFromClaude_KeepsATierThatMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\nmodels:\n  strong: {claude: opus, codex: gpt-5.5, effort: {claude: xhigh}}\n")
	writeFile(t, filepath.Join(dir, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: d.\nmodel: strong\n---\n\nReview.\n")
	writeFile(t, filepath.Join(dir, "agents", "scout.md"), "---\nname: scout\ndescription: d.\nmodel: strong\n---\n\nScout.\n")
	writeFile(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: d.\nmodel: opus\neffort: xhigh\n---\n\nReview.\n")
	writeFile(t, filepath.Join(dir, ".claude", "agents", "scout.md"), "---\nname: scout\ndescription: d.\nmodel: haiku\n---\n\nScout.\n")
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	reviewer := readFileString(t, filepath.Join(dir, "agents", "reviewer.md"))
	if !strings.Contains(reviewer, "model: strong\n") || strings.Contains(reviewer, "effort") {
		t.Errorf("a Claude model the tier gives must keep the tier:\n%s", reviewer)
	}
	if scout := readFileString(t, filepath.Join(dir, "agents", "scout.md")); !strings.Contains(scout, "model: {claude: haiku}") {
		t.Errorf("a Claude model that differs from the tier must import:\n%s", scout)
	}
}

func TestLint_ModelTierEdgeCases(t *testing.T) {
	dir := budgetProject(t, `targets: [claude, codex]
models:
  careful: {effort: high}
  opus: {claude: opus, codex: gpt-5.5}
  skilled: {claude: sonnet}
`)
	writeTierAgent(t, dir, "reviewer", "careful")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "tidy", "SKILL.md"), "---\nname: tidy\ndescription: Tidy.\nmodel: skilled\n---\n\nTidy.\n")

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("lint: %v\n%s", err, out)
	}
	lines := strings.Join(findingLines(out, "LINT025"), "\n")
	if strings.Contains(lines, "models.careful") {
		t.Errorf("an effort-only tier sets no model to miss:\n%s", lines)
	}
	if !strings.Contains(lines, `models.opus: the tier name is a Claude model name`) {
		t.Errorf("a tier named like a Claude model must warn:\n%s", out)
	}
	if !strings.Contains(lines, "models.skilled has no codex model and no default") {
		t.Errorf("a tier a skill names must be checked:\n%s", out)
	}
}

func TestLoadProject_RejectsUnknownTierTarget(t *testing.T) {
	budgetProject(t, "targets: [claude]\nmodels:\n  strong: {claud: opus}\n")
	if _, _, err := loadProject("."); err == nil || !strings.Contains(err.Error(), `models.strong: unknown target "claud" (did you mean claude?)`) {
		t.Fatalf("loadProject() error = %v", err)
	}
}

func TestSync_TierEffortSkipsTargetsWhoseModelTheSpecSets(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\nmodels:\n  strong: {claude: opus, codex: gpt-5.5, effort: high}\n")
	writeTierAgent(t, dir, "reviewer", "{codex: o4-mini, default: strong}")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if claude := readFileString(t, filepath.Join(dir, ".claude", "agents", "reviewer.md")); !strings.Contains(claude, "effort: high\n") {
		t.Errorf("claude takes the tier's model and effort:\n%s", claude)
	}
	codex := readFileString(t, filepath.Join(dir, ".codex", "agents", "reviewer.toml"))
	if !strings.Contains(codex, `model = "o4-mini"`) || strings.Contains(codex, "model_reasoning_effort") {
		t.Errorf("codex's own model must not take the tier effort:\n%s", codex)
	}
}

func TestLSPLinter_ReportsModelFindings(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\nmodels:\n  balanced: {claude: sonnet}\n")
	writeTierAgent(t, dir, "reviewer", "balanced")
	diags, err := lspLinter(dir)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, d := range diags[filepath.Join(dir, config.ConfigFileName)] {
		codes = append(codes, d.Code)
	}
	if !reflect.DeepEqual(codes, []string{"LINT025"}) {
		t.Errorf("config diagnostics = %v, all = %#v", codes, diags)
	}
}
