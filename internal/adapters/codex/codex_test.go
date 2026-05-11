package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_RootAgentsMd(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Path: "agents/ag1.md", Body: "agent body",
			Meta: map[string]any{"description": "agent desc"}},
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{"event": "X"}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("missing rule body in:\n%s", got)
	}
	if !strings.Contains(got, "_agent desc_") {
		t.Errorf("missing agent description in:\n%s", got)
	}
	if !strings.Contains(got, "Source: `.codex/agents/ag1.toml`") {
		t.Errorf("missing agent toml reference in:\n%s", got)
	}
	if strings.Contains(got, "agent body") {
		t.Errorf("agent body must live only in the toml file, not AGENTS.md:\n%s", got)
	}
	for _, want := range []string{"<!-- source: rules/r1.md -->", "<!-- source: agents/ag1.md -->"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing provenance %q in:\n%s", want, got)
		}
	}
}

func TestEmit_AgentTOMLFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "pr-reviewer",
			Body: "Review like an owner.\nLead with concrete findings.",
			Meta: map[string]any{
				"description": "PR reviewer",
				"model":       "gpt-5",
				"x-codex": map[string]any{
					"sandbox_mode":           "read-only",
					"model_reasoning_effort": "high",
					"nickname_candidates":    []any{"Atlas", "Delta"},
				},
			}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/agents/pr-reviewer.toml"))
	for _, want := range []string{
		`name = "pr-reviewer"`,
		`description = "PR reviewer"`,
		`developer_instructions = """`,
		"Review like an owner.",
		`model = "gpt-5"`,
		`sandbox_mode = "read-only"`,
		`model_reasoning_effort = "high"`,
		`nickname_candidates = ["Atlas", "Delta"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("toml missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_AgentTOML_NoExtras(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "explorer", Body: "Trace execution paths."},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/agents/explorer.toml"))
	for _, want := range []string{
		`name = "explorer"`,
		`description = "explorer"`, // falls back to name when frontmatter description missing
		`developer_instructions = """`,
		"Trace execution paths.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("toml missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"model =", "sandbox_mode =", "nickname_candidates ="} {
		if strings.Contains(got, unwanted) {
			t.Errorf("toml should omit empty optional %q:\n%s", unwanted, got)
		}
	}
}

func TestEmit_AgentTOML_RespectsAgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"codex": {AgentsDir: "vendor/codex/agents"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "scout", Body: "Look around."},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/codex/agents/scout.toml")); err != nil {
		t.Errorf("expected toml at custom agents-dir: %v", err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "vendor/codex/agents/scout.toml") {
		t.Errorf("AGENTS.md should reference the override path:\n%s", got)
	}
}

func TestEmit_NestedByLayoutScope(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend", Body: "auth body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "backend", "AGENTS.md"))
	if !strings.Contains(got, "auth body") {
		t.Errorf("expected scoped AGENTS.md content:\n%s", got)
	}
}

func TestEmit_NestedByGlobs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "root-rule",
			Meta: map[string]any{"globs": "**/*"}, Body: "root content"},
		{Kind: spec.KindRule, Name: "src-rule",
			Meta: map[string]any{"globs": "src/**"}, Body: "src content"},
		{Kind: spec.KindRule, Name: "tests-rule",
			Meta: map[string]any{"globs": "tests/**/*.go"}, Body: "tests content"},
		{Kind: spec.KindRule, Name: "deep-rule",
			Meta: map[string]any{"globs": "docs/api/**"}, Body: "api content"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path, mustContain string
	}{
		{"AGENTS.md", "root content"},
		{"src/AGENTS.md", "src content"},
		{"tests/AGENTS.md", "tests content"},
		{"docs/api/AGENTS.md", "api content"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got := readFile(t, filepath.Join(dir, c.path))
			if !strings.Contains(got, c.mustContain) {
				t.Errorf("%s missing %q:\n%s", c.path, c.mustContain, got)
			}
		})
	}
}

func TestEmit_SkillsListedInRoot(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator",
			Path: "skills/yaml-validator.md",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "## Skills") {
		t.Errorf("missing Skills section:\n%s", got)
	}
	if !strings.Contains(got, "yaml-validator") {
		t.Errorf("missing skill name:\n%s", got)
	}
	if !strings.Contains(got, ".agents/skills/yaml-validator/SKILL.md") {
		t.Errorf("AGENTS.md should point at the per-skill SKILL.md:\n%s", got)
	}
}

func TestEmit_SkillFolderLayout(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator",
			Path: "skills/yaml-validator.md",
			Body: "Run yamllint, then suggest fixes.",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	for _, want := range []string{
		"name: yaml-validator",
		"description: Validate YAML.",
		"Run yamllint, then suggest fixes.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "globs:") {
		t.Errorf("SKILL.md frontmatter should not leak unrelated keys:\n%s", got)
	}
}

func TestEmit_SkillFolder_DefaultsDescriptionToName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "explorer",
			Body: "Trace the call graph."},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/explorer/SKILL.md"))
	if !strings.Contains(got, "description: explorer") {
		t.Errorf("expected description to fall back to name:\n%s", got)
	}
}

func TestEmit_SkillFolder_OpenAIYAMLFromXCodex(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator",
			Body: "Validate YAML files.",
			Meta: map[string]any{
				"description": "Validate YAML.",
				"x-codex": map[string]any{
					"interface": map[string]any{
						"display_name": "YAML Validator",
						"brand_color":  "#3B82F6",
					},
					"policy": map[string]any{
						"allow_implicit_invocation": false,
					},
					"dependencies": map[string]any{
						"tools": []any{
							map[string]any{
								"type":  "mcp",
								"value": "yamllint",
							},
						},
					},
				},
			}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/agents/openai.yaml"))
	for _, want := range []string{
		"display_name: YAML Validator",
		"brand_color: '#3B82F6'",
		"allow_implicit_invocation: false",
		"type: mcp",
		"value: yamllint",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("openai.yaml missing %q in:\n%s", want, got)
		}
	}

	skillMd := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	if strings.Contains(skillMd, "interface:") || strings.Contains(skillMd, "x-codex") {
		t.Errorf("SKILL.md frontmatter should not contain x-codex/interface fields:\n%s", skillMd)
	}
}

func TestEmit_SkillFolder_NoOpenAIYAMLWithoutExtras(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "explorer", Body: "Look around."},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/explorer/agents/openai.yaml")); err == nil {
		t.Errorf("agents/openai.yaml should not be written without x-codex extras")
	}
}

func TestEmit_SkillFolder_RespectsSkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"codex": {SkillsDir: "vendor/codex/skills"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "explorer", Body: "Look around."},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/codex/skills/explorer/SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md at custom skills-dir: %v", err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "vendor/codex/skills/explorer/SKILL.md") {
		t.Errorf("AGENTS.md should reference the override path:\n%s", got)
	}
}

func TestEmit_AgentsAndSkillsAttachToRootOnly(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "src-rule",
			Meta: map[string]any{"globs": "src/**"}, Body: "src content"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	root := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(root, "## Agents") {
		t.Errorf("agents listing must be in root AGENTS.md: %s", root)
	}
	srcDoc := readFile(t, filepath.Join(dir, "src", "AGENTS.md"))
	if strings.Contains(srcDoc, "## Agents") {
		t.Errorf("agents listing must not appear in nested AGENTS.md: %s", srcDoc)
	}
}

func TestEmit_OutputOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"codex": {File: "vendor/AGENTS.md"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x", Meta: map[string]any{"globs": "src/**"}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/src/AGENTS.md")); err != nil {
		t.Errorf("expected nested override path: %v", err)
	}
}

func TestAdapterName(t *testing.T) {
	if New().Name() != "codex" {
		t.Errorf("expected codex, got %s", New().Name())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
