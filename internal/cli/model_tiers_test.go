package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

const modelTiersConfig = `targets: [claude, codex]
models:
  strong: {claude: opus, codex: gpt-5.5, effort: {claude: xhigh, codex: high}}
`

func writeTierAgent(t *testing.T, dir, name, model string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", name+".md"),
		"---\nname: "+name+"\ndescription: Reviews diffs.\nmodel: "+model+"\n---\n\nReview.\n")
}

func TestSync_ResolvesModelTierPerTarget(t *testing.T) {
	dir := budgetProject(t, modelTiersConfig)
	writeTierAgent(t, dir, "reviewer", "strong")
	writeTierAgent(t, dir, "literal", "gpt-5.4")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	claude := readFileString(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"))
	for _, want := range []string{"model: opus\n", "effort: xhigh\n"} {
		if !strings.Contains(claude, want) {
			t.Errorf("claude agent lacks %q:\n%s", want, claude)
		}
	}
	codex := readFileString(t, filepath.Join(dir, ".codex", "agents", "reviewer.toml"))
	for _, want := range []string{`model = "gpt-5.5"`, `model_reasoning_effort = "high"`} {
		if !strings.Contains(codex, want) {
			t.Errorf("codex agent lacks %q:\n%s", want, codex)
		}
	}
	if literal := readFileString(t, filepath.Join(dir, ".codex", "agents", "literal.toml")); !strings.Contains(literal, `model = "gpt-5.4"`) {
		t.Errorf("a model that names no tier must stay literal:\n%s", literal)
	}
}

func TestLint_FlagsModelTierGapsAndForeignClaudeNames(t *testing.T) {
	dir := budgetProject(t, `targets: [claude, codex]
models:
  strong: {claude: opus, codex: gpt-5.5}
  balanced: {claude: sonnet}
  shared: {default: opus}
  unused: {claude: haiku}
`)
	writeTierAgent(t, dir, "reviewer", "strong")
	writeTierAgent(t, dir, "scout", "balanced")
	writeTierAgent(t, dir, "helper", "shared")
	writeTierAgent(t, dir, "literal", "sonnet")
	writeTierAgent(t, dir, "scoped", "{claude: sonnet}")

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("model findings are advisory: %v\n%s", err, out)
	}
	gaps := strings.Join(findingLines(out, "LINT025"), "\n")
	if !strings.Contains(gaps, `agnostic-ai.yaml: models.balanced has no codex model and no default`) {
		t.Errorf("missing tier gap finding:\n%s", out)
	}
	if strings.Contains(gaps, "models.strong") || strings.Contains(gaps, "models.unused") || strings.Contains(gaps, "claude model") {
		t.Errorf("covered targets flagged:\n%s", gaps)
	}
	foreign := strings.Join(findingLines(out, "LINT026"), "\n")
	for _, want := range []string{
		`literal.md: model "sonnet" is a Claude model name codex cannot load`,
		`agnostic-ai.yaml: models.shared.default "opus" is a Claude model name codex cannot load`,
	} {
		if !strings.Contains(foreign, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(foreign, "scoped.md") || strings.Contains(foreign, "reviewer.md") {
		t.Errorf("scoped or tier models flagged per agent:\n%s", foreign)
	}
	if _, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Error("lint --strict must fail on model findings")
	}
}

func TestExplain_ShowsResolvedModelPerTarget(t *testing.T) {
	dir := budgetProject(t, modelTiersConfig)
	writeTierAgent(t, dir, "reviewer", "strong")

	out, err := runCLI(t, "explain", ".agnostic-ai/agents/reviewer.md")
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, out)
	}
	for _, want := range []string{
		"model (tier strong):",
		"[claude] opus, effort xhigh",
		"[codex] gpt-5.5, effort high",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("explain lacks %q:\n%s", want, out)
		}
	}

	out, err = runCLI(t, "explain", ".agnostic-ai/agents/reviewer.md", "--json")
	if err != nil {
		t.Fatalf("explain --json: %v\n%s", err, out)
	}
	var got explainOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if got.ModelTier != "strong" || len(got.Models) != 2 || got.Models[1] != (explainModel{Target: "codex", Model: "gpt-5.5", Effort: "high"}) {
		t.Errorf("json models = %q %#v", got.ModelTier, got.Models)
	}
}

func TestImportFromClaude_SuggestsTiersForRepeatedModels(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"reviewer", "architect"} {
		writeFile(t, filepath.Join(dir, ".claude", "agents", name+".md"),
			"---\nname: "+name+"\ndescription: d.\nmodel: opus\n---\n\nBody.\n")
	}
	writeFile(t, filepath.Join(dir, ".claude", "agents", "scout.md"),
		"---\nname: scout\ndescription: d.\nmodel: haiku\n---\n\nBody.\n")
	summary := captureSummary(t)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	got := summary.String()
	if !strings.Contains(got, "2 agents set model opus") || !strings.Contains(got, "models: {<tier>: {claude: opus}}") {
		t.Errorf("no tier suggestion for repeated model:\n%s", got)
	}
	if strings.Contains(got, "model haiku") {
		t.Errorf("a model one agent uses needs no tier:\n%s", got)
	}
}

func TestLintAndSync_TreatNewerClaudeAliasesAsClaudeModels(t *testing.T) {
	dir := budgetProject(t, `targets: [claude, codex]
models:
  frontier: {default: fable}
  best: {claude: opus, codex: gpt-6.1-sol}
`)
	writeTierAgent(t, dir, "architect", "frontier")
	writeTierAgent(t, dir, "planner", "opusplan")
	writeTierAgent(t, dir, "reader", `"sonnet[1m]"`)

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("model findings are advisory: %v\n%s", err, out)
	}
	if !strings.Contains(strings.Join(findingLines(out, "LINT025"), "\n"), "models.best: the tier name is a Claude model name") {
		t.Errorf("a tier named best must warn:\n%s", out)
	}
	foreign := strings.Join(findingLines(out, "LINT026"), "\n")
	for _, want := range []string{
		`agnostic-ai.yaml: models.frontier.default "fable" is a Claude model name codex cannot load`,
		`planner.md: model "opusplan" is a Claude model name codex cannot load`,
		`reader.md: model "sonnet[1m]" is a Claude model name codex cannot load; write model: {claude: "sonnet[1m]"} or name a tier`,
	} {
		if !strings.Contains(foreign, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}

	writeTierAgent(t, dir, "architect", "fable")
	notes := &strings.Builder{}
	adapters.SetWarner(notes)
	t.Cleanup(func() { adapters.SetWarner(os.Stderr) })
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if want := "`model` on 1 agent has no effect on codex (fable is a Claude model name; write model: {claude: fable} so codex uses its own default)"; !strings.Contains(notes.String(), want) {
		t.Errorf("a shared model: fable must raise the codex coverage note %q:\n%s", want, notes)
	}
	if want := `sonnet[1m] is a Claude model name; write model: {claude: "sonnet[1m]"} so codex uses its own default`; !strings.Contains(notes.String(), want) {
		t.Errorf("a bracketed alias must be quoted in the note %q:\n%s", want, notes)
	}
}

func TestImportFromClaude_KeepsBracketedAliasesValidYAML(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"reviewer", "architect"} {
		writeFile(t, filepath.Join(dir, ".claude", "agents", name+".md"),
			"---\nname: "+name+"\ndescription: d.\nmodel: opus[1m]\n---\n\nBody.\n")
	}
	summary := captureSummary(t)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	spec := readFileString(t, filepath.Join(dir, "agents", "reviewer.md"))
	if !strings.Contains(spec, "model:\n  claude: opus[1m]\n") {
		t.Errorf("opus[1m] should import scoped to claude:\n%s", spec)
	}
	hint := `models: {<tier>: {claude: "opus[1m]"}}`
	if !strings.Contains(summary.String(), hint) {
		t.Fatalf("missing quoted tier hint %q:\n%s", hint, summary)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(strings.TrimPrefix(hint, "models: ")), &parsed); err != nil {
		t.Errorf("the hint must parse as YAML: %v", err)
	}
}
