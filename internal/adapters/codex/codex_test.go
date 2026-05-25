package codex

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_NoRootAgentsMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Path: "agents/ag1.md", Body: "agent body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md by default; sync owns the entry-point. got: %v", err)
	}
}

func TestEmit_LegacyRulesFile_WritesConcatenated(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"codex": {RulesFile: "AGENTS.md"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("legacy rules-file should contain concatenated rule body:\n%s", got)
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
	got := readFile(t, filepath.Join(dir, ".codex/skills/yaml-validator/SKILL.md"))
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

func TestEmit_SharedSubagentsFalse_SkipsSkillEmission(t *testing.T) {
	dir := testutil.TempCwd(t)

	off := false
	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {SharedSubagents: &off},
	}}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator", Body: "Run yamllint.",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/skills/yaml-validator/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf(".codex/skills should be skipped when shared-subagents is false: %v", err)
	}
}

func TestEmit_SharedSubagentsTrue_EmitsSkills(t *testing.T) {
	dir := testutil.TempCwd(t)

	on := true
	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {SharedSubagents: &on},
	}}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator", Body: "Run yamllint.",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md when shared-subagents=true: %v", err)
	}
}

// Codex skills emit at .codex/skills/ regardless of whether claude is
// also enabled. The legacy claude-aware suppression (#216) is gone now
// that the path is .codex/skills/ instead of .agents/skills/ — claude's
// .claude/skills/ no longer overlaps so duplication is no longer a
// concern.
func TestEmit_SkillsEmitWhenClaudeAlsoEnabled(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Targets: []string{"claude", "codex"}}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator", Body: "Run yamllint.",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected .codex/skills/ even when claude is also enabled: %v", err)
	}
}

func TestEmit_SkillsEmitWhenCodexAlone(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Targets: []string{"codex"}}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator", Body: "Run yamllint.",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md when codex is the only target: %v", err)
	}
}

// Explicit `outputs.codex.shared-subagents: true` is harmless when codex
// is alone — default is already on.
func TestEmit_SharedSubagents_ExplicitTrueRedundantButOK(t *testing.T) {
	dir := testutil.TempCwd(t)

	on := true
	cfg := &config.Config{
		Targets: []string{"claude", "codex"},
		Outputs: map[string]config.Output{
			"codex": {SharedSubagents: &on},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator", Body: "Run yamllint.",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("explicit shared-subagents=true should still emit: %v", err)
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
	got := readFile(t, filepath.Join(dir, ".codex/skills/explorer/SKILL.md"))
	if !strings.Contains(got, "description: explorer") {
		t.Errorf("expected description to fall back to name:\n%s", got)
	}
}

// When a skill spec carries divergent claude/codex descriptions via
// `x-codex.description`, the codex emit must reproduce the codex value
// instead of the claude top-level one (#312). Without this, every
// merged skill leaked the claude description into .codex/skills/.
func TestEmit_SkillFolder_HonorsXCodexDescription(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "changelog",
			Meta: map[string]any{
				"name":        "changelog",
				"description": "claude-side description",
				"x-codex": map[string]any{
					"description": "codex-side description",
				},
			},
			Body: "body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/skills/changelog/SKILL.md"))
	if !strings.Contains(got, "description: codex-side description") {
		t.Errorf("expected x-codex.description to win on codex emit:\n%s", got)
	}
	if strings.Contains(got, "claude-side description") {
		t.Errorf("claude-side description leaked into codex emit:\n%s", got)
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
	got := readFile(t, filepath.Join(dir, ".codex/skills/yaml-validator/agents/openai.yaml"))
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

	skillMd := readFile(t, filepath.Join(dir, ".codex/skills/yaml-validator/SKILL.md"))
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
	if _, err := os.Stat(filepath.Join(dir, ".codex/skills/explorer/agents/openai.yaml")); err == nil {
		t.Errorf("agents/openai.yaml should not be written without x-codex extras")
	}
}

func TestEmit_SkillFolder_PropagatesAssets(t *testing.T) {
	dir := testutil.TempCwd(t)

	srcSkill := filepath.Join(dir, ".agnostic-ai", "skills", "yaml-validator")
	if err := os.MkdirAll(filepath.Join(srcSkill, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "SKILL.md"), []byte("---\nname: yaml-validator\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "scripts", "run.py"), []byte("print('ok')\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "fixtures.json"), []byte(`{"k":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Path: filepath.Join(srcSkill, "SKILL.md"),
			Body: "body",
			Meta: map[string]any{"description": "Validate YAML."},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"scripts/run.py", "fixtures.json"} {
		got, err := os.ReadFile(filepath.Join(dir, ".codex", "skills", "yaml-validator", filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("missing propagated asset %q: %v", rel, err)
			continue
		}
		want, _ := os.ReadFile(filepath.Join(srcSkill, filepath.FromSlash(rel)))
		if string(got) != string(want) {
			t.Errorf("asset %q not byte-identical:\ngot:  %q\nwant: %q", rel, got, want)
		}
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, ".codex", "skills", "yaml-validator", "scripts", "run.py"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("executable bit dropped on emit: mode=%v", info.Mode().Perm())
		}
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
}

func TestEmit_WritesCommand(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindCommand,
			Name: "deploy",
			Meta: map[string]any{
				"name":        "deploy",
				"description": "deploy the app",
			},
			Body: "Run the deploy.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".codex/prompts/deploy.md"))
	if !strings.Contains(got, "Run the deploy.") {
		t.Errorf("missing body: %s", got)
	}
	if !strings.Contains(got, "description: deploy the app") {
		t.Errorf("missing frontmatter description: %s", got)
	}
}

func TestEmit_CommandsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {CommandsDir: "vendor/prompts"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindCommand, Name: "deploy", Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/prompts/deploy.md")); err != nil {
		t.Errorf("expected commands at override path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/prompts/deploy.md")); !os.IsNotExist(err) {
		t.Errorf("default prompts dir should be skipped when override set")
	}
}

func TestEmit_OutputsCarryProvenanceHeader(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill body"},
		{Kind: spec.KindCommand, Name: "cmd1", Meta: map[string]any{"description": "d"}, Body: "cmd body"},
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{"event": "PostToolUse", "command": "echo"}},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		".codex/agents/ag1.toml",
		".codex/skills/sk1/SKILL.md",
		".codex/prompts/cmd1.md",
		".codex/hooks.json",
		".codex/config.toml",
	} {
		got, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		// hooks.json has no provenance header (JSON cannot carry comments
		// cleanly); just assert it parses with the hook present.
		if strings.HasSuffix(p, ".json") {
			if !strings.Contains(string(got), `"PostToolUse"`) {
				t.Errorf("%s missing emitted hook:\n%s", p, got)
			}
			continue
		}
		if !strings.Contains(string(got), "Generated by agnostic-ai") {
			t.Errorf("%s missing provenance header:\n%s", p, got)
		}
	}
}

// outputs.<target>.provenance-header=false must suppress the header
// across every emit format, including .toml and the legacy single-file
// rules layout. Regression for #276.
func TestEmit_ProvenanceHeaderToggleOff_SuppressesEverywhere(t *testing.T) {
	dir := testutil.TempCwd(t)

	off := false
	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {ProvenanceHeader: &off},
	}}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill body"},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	defer emit.ProvenanceFor(cfg, "codex")()
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		".codex/agents/ag1.toml",
		".codex/skills/sk1/SKILL.md",
		".codex/config.toml",
	} {
		got, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if strings.Contains(string(got), "Generated by agnostic-ai") {
			t.Errorf("%s carries header despite toggle off:\n%s", p, got)
		}
	}
}

// Regression for #293: `::target codex` fences inside an agent body
// render only into the codex emit; `::target claude` fences disappear.
// Tests the integration of Entry.BodyFor with Bundle.For + adapter
// dispatch.
func TestEmit_AgentBody_PerTargetFences(t *testing.T) {
	dir := testutil.TempCwd(t)

	body := "Shared intro.\n\n::target codex\nCodex-only paragraph.\n::end\n\n::target claude\nClaude-only paragraph.\n::end\n\nShared outro.\n"
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "diverge", Body: body}}
	if err := New().Emit(spec.NewBundle(entries).For("codex"), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/agents/diverge.toml"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "Codex-only paragraph.") {
		t.Errorf("codex emit missing its fenced section:\n%s", out)
	}
	if strings.Contains(out, "Claude-only paragraph.") {
		t.Errorf("codex emit leaked claude-scoped fence:\n%s", out)
	}
	for _, marker := range []string{"::target", "::end"} {
		if strings.Contains(out, marker) {
			t.Errorf("emit kept raw fence marker %q:\n%s", marker, out)
		}
	}
}

// Regression for #292: an agent scoped to claude via `target:` must
// not emit into .codex/agents/ when sync dispatches through the
// EmitWithProvenance wrapper (which calls bundle.For(target)).
func TestEmit_AgentScopedToOtherTarget_SkipsCodex(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "claude-only", Body: "x",
			Meta: map[string]any{"target": "claude"}},
		{Kind: spec.KindAgent, Name: "both", Body: "y"},
	}
	if err := New().Emit(spec.NewBundle(entries).For("codex"), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/agents/claude-only.toml")); !os.IsNotExist(err) {
		t.Errorf("claude-only agent should not appear in .codex/agents/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/agents/both.toml")); err != nil {
		t.Errorf("unscoped agent should still emit to codex: %v", err)
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
