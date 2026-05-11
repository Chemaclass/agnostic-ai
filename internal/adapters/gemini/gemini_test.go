package gemini

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
	if got := New().Name(); got != "gemini" {
		t.Errorf("Name() = %q, want %q", got, "gemini")
	}
}

func TestEmit_RootRulesGoToRootGEMINI(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "house-style", Body: "Be terse."},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "GEMINI.md"))
	for _, want := range []string{"# GEMINI.md", "## Rules", "Be terse."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_ScopedRule_RoutesToNestedGEMINI(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind:  spec.KindRule,
			Name:  "auth-style",
			Scope: "backend",
			Body:  "Validate at boundary.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	nested := readFile(t, filepath.Join(dir, "backend/GEMINI.md"))
	for _, want := range []string{"GEMINI.md (backend)", "Validate at boundary."} {
		if !strings.Contains(nested, want) {
			t.Errorf("missing %q in %s", want, nested)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Errorf("expected no root GEMINI.md when only scoped rules exist, err=%v", err)
	}
}

func TestEmit_RuleGlobs_RoutesByPrefix(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "api-only",
			Meta: map[string]any{"globs": "docs/api/**"},
			Body: "API rules.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "docs/api/GEMINI.md"))
	if !strings.Contains(got, "API rules.") {
		t.Errorf("expected scoped GEMINI.md by globs prefix: %s", got)
	}
}

// Agents emit as TOML slash commands under .gemini/commands/.
func TestEmit_Agent_WritesCommandTOML(t *testing.T) {
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
	toml := readFile(t, filepath.Join(dir, ".gemini/commands/pr-reviewer.toml"))
	for _, want := range []string{
		`description = "Review PRs like an owner."`,
		`prompt = """`,
		"Open the PR. Read it. Comment.",
		`"""`,
	} {
		if !strings.Contains(toml, want) {
			t.Errorf("missing %q in %s", want, toml)
		}
	}
}

// Root GEMINI.md lists agents but does not duplicate their bodies.
func TestEmit_RootGEMINI_ListsAgentsWithoutDuplicatingBody(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "pr-reviewer",
			Meta: map[string]any{"description": "Review PRs."},
			Body: "Long body should NOT appear in GEMINI.md.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	gemini := readFile(t, filepath.Join(dir, "GEMINI.md"))
	for _, want := range []string{"## Agents", "pr-reviewer", "Review PRs.", ".gemini/commands/pr-reviewer.toml"} {
		if !strings.Contains(gemini, want) {
			t.Errorf("missing %q in %s", want, gemini)
		}
	}
	if strings.Contains(gemini, "Long body should NOT appear") {
		t.Errorf("agent body should not be duplicated in root GEMINI.md: %s", gemini)
	}
}

// Skills default to reference-only in root GEMINI.md.
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
	gemini := readFile(t, filepath.Join(dir, "GEMINI.md"))
	for _, want := range []string{"## Skills", "yaml-validator", "Validate YAML."} {
		if !strings.Contains(gemini, want) {
			t.Errorf("missing %q in %s", want, gemini)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".gemini/commands/yaml-validator.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no skill command file by default, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gemini/commands/skill-yaml-validator.toml")); !os.IsNotExist(err) {
		t.Errorf("expected no skill command file by default (prefixed), err=%v", err)
	}
}

// Skills emit as TOML commands when emit-skills-as-commands is true.
func TestEmit_Skill_EmitsAsCommand_WhenOptIn(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"gemini": {EmitSkillsAsCommands: true},
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
	toml := readFile(t, filepath.Join(dir, ".gemini/commands/skill-yaml-validator.toml"))
	for _, want := range []string{
		`description = "Validate YAML."`,
		"Validate against schema.",
	} {
		if !strings.Contains(toml, want) {
			t.Errorf("missing %q in %s", want, toml)
		}
	}
}

func TestEmit_CommandsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"gemini": {CommandsDir: "vendor/commands"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "ag", Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/commands/ag.toml")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

// Stdio MCP emits to .gemini/settings.json under mcpServers.
func TestEmit_MCP_StdioWritesMcpServersKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem", "."},
				"env":     map[string]any{"ALLOWED_PATHS": "."},
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".gemini/settings.json"))
	for _, want := range []string{
		`"mcpServers"`,
		`"fs"`,
		`"command": "npx"`,
		`"ALLOWED_PATHS"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// HTTP MCP uses Gemini's `httpUrl` key, not the standard `url`.
func TestEmit_MCP_HTTPUsesHttpUrlKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type":    "http",
				"url":     "https://api.githubcopilot.com/mcp/",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".gemini/settings.json"))
	if !strings.Contains(got, `"httpUrl"`) {
		t.Errorf("expected httpUrl key (not url): %s", got)
	}
	if !strings.Contains(got, `"https://api.githubcopilot.com/mcp/"`) {
		t.Errorf("missing url value: %s", got)
	}
}

// Hooks emit under hooks.<event> = [{matcher, command}, ...].
func TestEmit_Hook_GroupsByEvent(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "pre-bash",
			Meta: map[string]any{
				"event":   "BeforeTool",
				"matcher": "Bash",
				"command": "echo pre-tool",
			},
		},
		{
			Kind: spec.KindHook,
			Name: "session",
			Meta: map[string]any{
				"event":   "SessionStart",
				"command": "echo started",
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".gemini/settings.json"))
	for _, want := range []string{
		`"hooks"`,
		`"BeforeTool"`,
		`"matcher": "Bash"`,
		`"command": "echo pre-tool"`,
		`"SessionStart"`,
		`"command": "echo started"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Hook with no event is skipped (no key to route into).
func TestEmit_Hook_SkipsWhenNoEvent(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "bad",
			Meta: map[string]any{"command": "echo nope"},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gemini/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no settings file when hook has no event, err=%v", err)
	}
}

func TestEmit_Settings_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := os.MkdirAll(filepath.Join(dir, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"theme": "dark", "selectedAuthType": "oauth-personal"}`
	if err := os.WriteFile(filepath.Join(dir, ".gemini/settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".gemini/settings.json"))
	for _, want := range []string{
		`"theme": "dark"`,
		`"selectedAuthType": "oauth-personal"`,
		`"mcpServers"`,
		`"fs"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_Settings_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"gemini": {MCPFile: "vendor/gemini.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/gemini.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_Settings_NoFileWhenNoMCPOrHooks(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gemini/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no settings file when no MCP/hook entries, err=%v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Errorf("expected no GEMINI.md for empty bundle, err=%v", err)
	}
}

// TOML escaping: bodies with quotes and backslashes round-trip safely.
func TestEmit_Agent_TOMLEscapesQuotesAndBackslashes(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "tricky",
			Body: `Body with "quotes" and a \ backslash.`,
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	toml := readFile(t, filepath.Join(dir, ".gemini/commands/tricky.toml"))
	if !strings.Contains(toml, `Body with \"quotes\" and a \\ backslash.`) {
		t.Errorf("expected escaped basic-string chars: %s", toml)
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
