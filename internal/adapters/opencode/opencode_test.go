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

func TestEmit_NoAgentsMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write .opencode/AGENTS.md by default; sync owns the entry-point, err=%v", err)
	}
}

func TestEmit_LegacyRulesFile_WritesConcatenated(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"opencode": {RulesFile: ".opencode/AGENTS.md"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".opencode/AGENTS.md"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("legacy rules-file should contain concatenated rule body:\n%s", got)
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

// Skills emit no command file by default.
func TestEmit_Skill_NoCommandByDefault(t *testing.T) {
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

// Stdio MCP emits to opencode.json with type=local + command array.
func TestEmit_MCP_StdioWritesLocalCommand(t *testing.T) {
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
	got := readFile(t, filepath.Join(dir, "opencode.json"))
	for _, want := range []string{
		`"$schema": "https://opencode.ai/config.json"`,
		`"mcp"`,
		`"fs"`,
		`"type": "local"`,
		`"npx"`,
		`"@modelcontextprotocol/server-filesystem"`,
		`"environment"`,
		`"ALLOWED_PATHS"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_HTTPWritesRemoteURL(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "linear",
			Meta: map[string]any{
				"type":    "http",
				"url":     "https://mcp.linear.app",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "opencode.json"))
	for _, want := range []string{
		`"type": "remote"`,
		`"url": "https://mcp.linear.app"`,
		`"Authorization"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// User-managed keys in an existing opencode.json survive the sync.
func TestEmit_MCP_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	existing := `{"theme": "dark", "model": "claude-opus-4-7"}`
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "opencode.json"))
	for _, want := range []string{
		`"theme": "dark"`,
		`"model": "claude-opus-4-7"`,
		`"mcp"`,
		`"fs"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"opencode": {MCPFile: "vendor/opencode.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/opencode.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_MCP_NoFileWhenNoEntries(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "opencode.json")); !os.IsNotExist(err) {
		t.Errorf("expected no opencode.json when no MCP entries, err=%v", err)
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
