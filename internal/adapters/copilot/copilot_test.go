package copilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
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

// An always-on rule (alwaysApply: true, or no globs/scope) emits a
// catch-all (applyTo: "**") per-file instruction by default, the same
// shape agents and skills use, so it reaches Copilot without an opt-in.
// The adapter still leaves .github/copilot-instructions.md to `sync`.
// The legacy concatenated layout (outputs.copilot.rules-file) is the one
// case that suppresses the per-file emission (see TestEmit_LegacyRulesFile_*).
func TestEmit_RuleAlwaysOn_WritesCatchAllInstruction(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "conventional-commits",
			Meta: map[string]any{"alwaysApply": true},
			Body: "Use Conventional Commits.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".github/copilot-instructions.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write .github/copilot-instructions.md by default; sync owns the entry-point, err=%v", err)
	}
	got := readFile(t, filepath.Join(dir, ".github/instructions/conventional-commits.instructions.md"))
	for _, want := range []string{"applyTo: \"**\"", "Use Conventional Commits."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in always-on rule instruction:\n%s", want, got)
		}
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".github/copilot-instructions.md"))
	if !strings.Contains(got, "Use Conventional Commits.") {
		t.Errorf("legacy rules-file should contain always-on rule body:\n%s", got)
	}
}

// Agents emit natively as custom agent profiles under .github/agents/,
// not as flattened catch-all instructions.
func TestEmit_Agent_WritesNativeAgentProfile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "pr-reviewer",
			Meta: map[string]any{"description": "Review PRs like an owner."},
			Body: "Open the PR. Read it. Comment.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".github/agents/pr-reviewer.agent.md"))
	for _, want := range []string{
		"name: pr-reviewer",
		"description: Review PRs like an owner.",
		"Open the PR. Read it. Comment.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/instructions/agent-pr-reviewer.instructions.md")); !os.IsNotExist(err) {
		t.Errorf("agents must not emit as flattened instructions anymore, err=%v", err)
	}
}

// x-copilot.{tools,model} pass through into the agent profile frontmatter.
func TestEmit_Agent_ToolsAndModelPassThrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "researcher",
			Meta: map[string]any{
				"description": "Research things.",
				"x-copilot": map[string]any{
					"tools": []any{"read", "search"},
					"model": "gpt-5",
				},
			},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".github/agents/researcher.agent.md"))
	for _, want := range []string{"tools:", "- read", "- search", "model: gpt-5"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Skills emit natively as one folder per skill under .github/skills/,
// not as flattened catch-all instructions.
func TestEmit_Skill_WritesNativeSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Path: "skills/yaml-validator.md",
			Meta: map[string]any{"description": "Validate YAML."},
			Body: "Validate YAML with the schema.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".github/skills/yaml-validator/SKILL.md"))
	for _, want := range []string{"name: yaml-validator", "description: Validate YAML.", "Validate YAML with the schema."} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/instructions/skill-yaml-validator.instructions.md")); !os.IsNotExist(err) {
		t.Errorf("skills must not emit as flattened instructions anymore, err=%v", err)
	}
}

// A custom key under x-copilot reaches the SKILL.md frontmatter while
// shared top-level keys stay stripped. See #367.
func TestEmit_Skill_CustomXCopilotKeyReachesFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Path: "skills/yaml-validator.md",
			Meta: map[string]any{
				"globs":     "src/**",
				"x-copilot": map[string]any{"some-copilot-key": "manual"},
			},
			Body: "Validate YAML with the schema.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".github/skills/yaml-validator/SKILL.md"))
	if !strings.Contains(got, "some-copilot-key: manual") {
		t.Errorf("missing custom x-copilot key in %s", got)
	}
	if strings.Contains(got, "globs:") {
		t.Errorf("shared top-level key leaked into %s", got)
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
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

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
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
	// The native agent profile still emits alongside the chat mode.
	if _, err := os.Stat(filepath.Join(dir, ".github/agents/researcher.agent.md")); err != nil {
		t.Errorf("agent profile must remain when chatmodes-dir is set: %v", err)
	}
}

func TestEmit_NoChatmodesDirSkipsEmission(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "x", Meta: map[string]any{}, Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/chatmodes/x.chatmode.md")); err == nil {
		t.Error("chatmodes dir not configured; should not emit chatmode file")
	}
}

// Agent Host does not read .vscode/mcp.json; it reads a workspace-root
// .mcp.json natively (code.visualstudio.com/docs/agent-customization/mcp-servers).
// The root copy is opt-in so a project that does not need it gets no
// surprise file at its root.
func TestEmit_MCP_RootFileIsOptIn(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "args": []any{"-y", "server"}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("root .mcp.json must not be written unless opted in, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".vscode/mcp.json")); err != nil {
		t.Errorf(".vscode/mcp.json is still the default: %v", err)
	}
}

func TestEmit_MCP_RootFileWritesVSCodeSchema(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"copilot": {RootMCPFile: ".mcp.json"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "args": []any{"-y", "server"}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	root := readFile(t, filepath.Join(dir, ".mcp.json"))
	// The vendor documents the same "servers" wrapper for the root file
	// as for .vscode/mcp.json, not Claude's "mcpServers".
	if !strings.Contains(root, `"servers"`) {
		t.Errorf("root .mcp.json must use the VS Code servers wrapper:\n%s", root)
	}
	if strings.Contains(root, `"mcpServers"`) {
		t.Errorf("root .mcp.json must not use Claude's mcpServers wrapper:\n%s", root)
	}
	if !strings.Contains(root, `"fs"`) {
		t.Errorf("root .mcp.json missing the server entry:\n%s", root)
	}
	// Both files carry the same servers, so the native path keeps working.
	if vscode := readFile(t, filepath.Join(dir, ".vscode/mcp.json")); vscode != root {
		t.Errorf("root and .vscode copies must match:\nroot:\n%s\nvscode:\n%s", root, vscode)
	}
}

func TestEmit_MCP_RootFileSkippedWhenNoServers(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"copilot": {RootMCPFile: ".mcp.json"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("no MCP specs means no root file, err=%v", err)
	}
}
