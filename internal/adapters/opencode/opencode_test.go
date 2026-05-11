package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "opencode" {
		t.Errorf("Name() = %q, want %q", got, "opencode")
	}
}

func TestEmit_WritesAgentsMd_WithRules(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".opencode/AGENTS.md"))
	for _, want := range []string{"rule body", "<!-- source: rules/r1.md -->"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Each agent writes one command file under .opencode/commands/ with
// description frontmatter (required by OpenCode).
func TestEmit_Agent_WritesCommandWithDescriptionFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "pr-reviewer",
			Meta: map[string]any{"description": "Review PRs like an owner."},
			Body: "Open the PR. Read it. Comment.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	cmd := readFile(t, filepath.Join(dir, ".opencode/commands/pr-reviewer.md"))
	for _, want := range []string{
		"---",
		"description: Review PRs like an owner.",
		"Open the PR. Read it. Comment.",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in %s", want, cmd)
		}
	}
}

// x-opencode.{agent,model,subtask} pass through into the command frontmatter.
func TestEmit_Agent_XOpencodePassesThrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "ag1",
			Meta: map[string]any{
				"description": "Do thing.",
				"x-opencode": map[string]any{
					"agent":   "build",
					"model":   "openai/gpt-5",
					"subtask": true,
				},
			},
			Body: "body",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	cmd := readFile(t, filepath.Join(dir, ".opencode/commands/ag1.md"))
	for _, want := range []string{
		"description: Do thing.",
		"agent: build",
		"model: openai/gpt-5",
		"subtask: true",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in %s", want, cmd)
		}
	}
}

// Unrelated frontmatter keys (and the x- nested map itself) don't leak
// into the command frontmatter.
func TestEmit_Agent_OnlyAllowedFrontmatterKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "ag",
			Meta: map[string]any{
				"description": "x",
				"name":        "ag",
				"globs":       "src/**",
				"tools":       []any{"Read"},
				"x-opencode":  map[string]any{"agent": "build"},
			},
			Body: "body",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	cmd := readFile(t, filepath.Join(dir, ".opencode/commands/ag.md"))
	for _, leaked := range []string{"globs:", "tools:", "name:", "x-opencode:"} {
		if strings.Contains(cmd, leaked) {
			t.Errorf("unexpected leaked frontmatter %q in %s", leaked, cmd)
		}
	}
}

// Skills default to reference-only in AGENTS.md; no commands file.
func TestEmit_Skill_ReferenceOnlyByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Path: "skills/yaml-validator.md",
			Meta: map[string]any{"description": "Validate YAML."},
			Body: "Body content.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	agents := readFile(t, filepath.Join(dir, ".opencode/AGENTS.md"))
	for _, want := range []string{"## Skills", "yaml-validator", "Validate YAML."} {
		if !strings.Contains(agents, want) {
			t.Errorf("missing %q in %s", want, agents)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/commands/yaml-validator.md")); !os.IsNotExist(err) {
		t.Errorf("expected no skill command file by default, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/commands/skill-yaml-validator.md")); !os.IsNotExist(err) {
		t.Errorf("expected no prefixed skill command file by default, err=%v", err)
	}
}

// Skills emit as command files when emit-skills-as-commands is on.
func TestEmit_Skill_EmitsCommand_WhenOptIn(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"opencode": {EmitSkillsAsCommands: true},
		},
	}
	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Meta: map[string]any{"description": "Validate YAML."},
			Body: "Validate against schema.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	cmd := readFile(t, filepath.Join(dir, ".opencode/commands/skill-yaml-validator.md"))
	for _, want := range []string{
		"description: Validate YAML.",
		"Validate against schema.",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in %s", want, cmd)
		}
	}
}

// AGENTS.md references agents with a pointer to the command file path,
// rather than duplicating the agent body.
func TestEmit_AgentsMd_ListsAgentsWithoutDuplicatingBody(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "pr-reviewer",
			Meta: map[string]any{"description": "Review PRs."},
			Body: "Long body should NOT appear in AGENTS.md.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	agents := readFile(t, filepath.Join(dir, ".opencode/AGENTS.md"))
	for _, want := range []string{"## Agents", "pr-reviewer", "Review PRs.", ".opencode/commands/pr-reviewer.md"} {
		if !strings.Contains(agents, want) {
			t.Errorf("missing %q in %s", want, agents)
		}
	}
	if strings.Contains(agents, "Long body should NOT appear") {
		t.Errorf("agent body should not be duplicated in AGENTS.md: %s", agents)
	}
}

func TestEmit_CommandsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"opencode": {CommandsDir: "vendor/oc/commands"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "ag", Meta: map[string]any{"description": "x"}, Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/oc/commands/ag.md")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("expected no AGENTS.md for empty bundle, err=%v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
