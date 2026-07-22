package crush

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
	if got := New().Name(); got != "crush" {
		t.Errorf("Name() = %q, want %q", got, "crush")
	}
}

// The project-root AGENTS.md is written centrally by sync, never by
// this adapter: Crush has no per-rule or per-agent surface of its own.
func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
}

// Skills emit natively as one folder per skill under .agents/skills/,
// the shared cross-tool tree Crush scans first.
func TestEmit_Skill_WritesSharedSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Meta: map[string]any{"description": "Validate YAML."},
			Body: "Validate against schema.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	for _, want := range []string{"name: yaml-validator", "description: Validate YAML.", "Validate against schema."} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, got)
		}
	}
}

func TestEmit_Skill_SkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"crush": {SkillsDir: "custom/skills"}},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "yaml-validator", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/yaml-validator/SKILL.md: %v", err)
	}
}

// Stdio MCP emits to crush.json under mcp.<name> with type "stdio".
func TestEmit_MCP_StdioWritesCrushJSON(t *testing.T) {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"mcp"`,
		`"fs"`,
		`"type": "stdio"`,
		`"command": "npx"`,
		`"ALLOWED_PATHS"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_HTTPWritesURL(t *testing.T) {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"linear"`,
		`"type": "http"`,
		`"url": "https://mcp.linear.app"`,
		`"Authorization"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	existing := `{"models": {"large": "x"}, "providers": {"anthropic": {}}, "options": {"debug": true}}`
	if err := os.WriteFile(filepath.Join(dir, "crush.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "crush.json"))
	for _, want := range []string{
		`"large": "x"`,
		`"anthropic"`,
		`"debug": true`,
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
			"crush": {MCPFile: "vendor/crush.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/crush.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_NoCrushJSONWhenNoMCPs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crush.json")); !os.IsNotExist(err) {
		t.Errorf("expected no crush.json when no MCP entries, err=%v", err)
	}
}

// Stdio entries without a command are dropped: there is nothing for
// Crush to run.
func TestEmit_MCP_SkipsStdioWithoutCommand(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "bad", Meta: map[string]any{}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crush.json")); !os.IsNotExist(err) {
		t.Errorf("expected no crush.json when entry has no command, err=%v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "crush.json")); !os.IsNotExist(err) {
		t.Errorf("expected no crush.json for empty bundle, err=%v", err)
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
