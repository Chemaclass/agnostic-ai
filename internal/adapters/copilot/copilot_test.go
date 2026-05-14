package copilot

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
	if got := New().Name(); got != "copilot" {
		t.Errorf("Name() = %q, want %q", got, "copilot")
	}
}

// Rule with globs frontmatter must emit a separate per-file
// instructions file with applyTo derived from the globs.
func TestEmit_RuleWithGlobs_WritesScopedInstruction(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "ts-style",
			Meta: map[string]any{"globs": "src/**/*.ts,**/*.tsx"},
			Body: "Prefer named exports.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".github/instructions/ts-style.instructions.md"))
	for _, want := range []string{
		"applyTo: \"src/**/*.ts,**/*.tsx\"",
		"Prefer named exports.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, ".github/copilot-instructions.md")); !os.IsNotExist(err) {
		t.Errorf("expected no main file when only scoped rules exist, err=%v", err)
	}
}

// A rule with alwaysApply: true used to merge into the main file; now
// the adapter skips per-file emission for it (alwaysApply still wins
// over globs) and `sync` writes a pointer body at
// .github/copilot-instructions.md instead.
func TestEmit_RuleWithAlwaysApply_NoPerFileInstruction(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "conventional-commits",
			Meta: map[string]any{"alwaysApply": true},
			Body: "Use Conventional Commits.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".github/copilot-instructions.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write .github/copilot-instructions.md by default; sync owns the entry-point, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/instructions/conventional-commits.instructions.md")); !os.IsNotExist(err) {
		t.Errorf("expected no per-file instruction for alwaysApply rule, err=%v", err)
	}
}

func TestEmit_LegacyRulesFile_WritesAlwaysOnRules(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"copilot": {RulesFile: ".github/copilot-instructions.md"}},
	}
	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "conventional-commits",
			Meta: map[string]any{"alwaysApply": true},
			Body: "Use Conventional Commits.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".github/copilot-instructions.md"))
	if !strings.Contains(got, "Use Conventional Commits.") {
		t.Errorf("legacy rules-file should contain always-on rule body:\n%s", got)
	}
}

// Agents always emit as catch-all instructions with the agent- prefix.
func TestEmit_Agent_WritesAgentPrefixedInstruction(t *testing.T) {
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

	got := readFile(t, filepath.Join(dir, ".github/instructions/agent-pr-reviewer.instructions.md"))
	for _, want := range []string{
		"applyTo: \"**\"",
		"Review PRs like an owner.",
		"Open the PR. Read it. Comment.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Skills emit as catch-all instructions with the skill- prefix.
func TestEmit_Skill_WritesSkillPrefixedInstruction(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Path: "skills/yaml-validator.md",
			Body: "Validate YAML with the schema.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".github/instructions/skill-yaml-validator.instructions.md"))
	if !strings.Contains(got, "applyTo: \"**\"") {
		t.Errorf("skill instructions missing applyTo: %s", got)
	}
	if !strings.Contains(got, "Validate YAML with the schema.") {
		t.Errorf("skill instructions missing body: %s", got)
	}
}

// A rule with no globs but a scope from source layout derives applyTo
// from the scope (so rules/backend/*.md auto-target backend/**).
func TestEmit_RuleWithScope_DerivesApplyToFromScope(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind:  spec.KindRule,
			Name:  "auth-style",
			Scope: "backend",
			Body:  "Validate inputs at the boundary.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".github/instructions/auth-style.instructions.md"))
	if !strings.Contains(got, "applyTo: \"backend/**\"") {
		t.Errorf("expected scope-derived applyTo, got: %s", got)
	}
}

// outputs.copilot.instructions-dir overrides the default directory.
func TestEmit_InstructionsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"copilot": {InstructionsDir: "custom/instructions"},
		},
	}
	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "r1",
			Meta: map[string]any{"globs": "src/**"},
			Body: "rule body",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/instructions/r1.instructions.md")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

// MCP emission is unchanged: VS Code schema at .vscode/mcp.json.
func TestEmit_MCPFile_Unchanged(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y"},
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".vscode/mcp.json"))
	for _, want := range []string{`"servers"`, `"type": "stdio"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// An empty bundle produces no files at all - no stub main file, no MCP file.
func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/copilot-instructions.md")); !os.IsNotExist(err) {
		t.Errorf("expected no main file for empty bundle, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".vscode/mcp.json")); !os.IsNotExist(err) {
		t.Errorf("expected no mcp file for empty bundle, err=%v", err)
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

func TestEmit_ChatmodesDirEmitsAgentAsChatmode(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			target: {ChatmodesDir: ".github/chatmodes"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "researcher", Meta: map[string]any{
			"description": "deep research",
			"model":       "sonnet",
			"tools":       []any{"Read", "Grep", "Bash"},
		}, Body: "You are a researcher.\n"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".github/chatmodes/researcher.chatmode.md"))
	if err != nil {
		t.Fatalf("expected chatmode file: %v", err)
	}
	for _, want := range []string{"description: deep research", "model: sonnet", "tools: [Read, Grep, Bash]", "You are a researcher."} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in chatmode body:\n%s", want, got)
		}
	}
	// Catch-all instruction-form emission preserved.
	if _, err := os.Stat(filepath.Join(dir, ".github/instructions/agent-researcher.instructions.md")); err != nil {
		t.Errorf("instruction-form must remain when chatmodes-dir is set: %v", err)
	}
}

func TestEmit_NoChatmodesDirSkipsEmission(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "x", Meta: map[string]any{}, Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/chatmodes/x.chatmode.md")); err == nil {
		t.Error("chatmodes dir not configured; should not emit chatmode file")
	}
}
