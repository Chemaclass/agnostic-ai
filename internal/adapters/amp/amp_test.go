package amp

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
	if got := New().Name(); got != "amp" {
		t.Errorf("Name() = %q, want %q", got, "amp")
	}
}

// Command specs write nothing: Amp's manual documents .agents/skills/
// and .agents/checks/ but no file-based command surface, and its
// migration guidance is to delete the old command file rather than
// point at a replacement path. Writing to .agents/commands/ for a
// Command spec would be a file Amp never reads. See #553.
func TestEmit_CommandKind_WritesNoFile_WarnsUnsupported(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindCommand, Name: "deploy", Path: "commands/deploy.md", Meta: map[string]any{"description": "Ship it"}, Body: "Run the deploy steps."},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "commands", "deploy.md")); !os.IsNotExist(err) {
		t.Errorf("command spec should not write a file amp never reads, err=%v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 1 {
		t.Errorf("expected 1 capability warning for unsupported command kind, got %d", got)
	}
}

// Default emission no longer writes AGENTS.md at all; sync owns the
// entry-point write so codex + amp + warp can coexist at a single
// AGENTS.md path. The singular AGENT.md is never written.
func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md by default, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENT.md")); !os.IsNotExist(err) {
		t.Errorf("singular AGENT.md should not be written, err=%v", err)
	}
}

func TestEmit_LegacyRulesFile_WritesConcatenated(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"amp": {RulesFile: "AGENTS.md"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("legacy rules-file should contain concatenated rule body:\n%s", got)
	}
}

func TestEmit_Agent_WritesCommandFile(t *testing.T) {
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
	cmd := readFile(t, filepath.Join(dir, ".agents/commands/pr-reviewer.md"))
	for _, want := range []string{
		"description: Review PRs like an owner.",
		"Open the PR. Read it. Comment.",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in %s", want, cmd)
		}
	}
}

// Skills emit natively as a folder per skill under .agents/skills/, not
// as the removed .agents/commands/skill-<name>.md command form.
func TestEmit_Skill_WritesNativeSkillFolder(t *testing.T) {
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
	// The removed command-form surface must not be written.
	if _, err := os.Stat(filepath.Join(dir, ".agents/commands/skill-yaml-validator.md")); !os.IsNotExist(err) {
		t.Errorf("skill should not emit a command-form file, err=%v", err)
	}
}

func TestEmit_SkillsDirOverride_WritesToCustomDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"amp": {SkillsDir: "custom/skills"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator", Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/yaml-validator/SKILL.md: %v", err)
	}
}

// A custom key under x-amp reaches the SKILL.md frontmatter; shared
// top-level keys stay stripped. See #367.
func TestEmit_Skill_CustomXAmpKeyReachesFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Meta: map[string]any{
				"description": "Validate YAML.",
				"globs":       "src/**",
				"x-amp":       map[string]any{"some-amp-key": "manual"},
			},
			Body: "Validate against schema.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	if !strings.Contains(got, "some-amp-key: manual") {
		t.Errorf("missing custom x-amp key in %s", got)
	}
	if strings.Contains(got, "globs:") {
		t.Errorf("shared top-level key leaked into %s", got)
	}
}

// A pre-existing AGENT.md with our provenance marker is renamed to
// AGENT.md.bak and the user is warned.
func TestEmit_MigratesLegacyAGENTMd_WhenAgnosticGenerated(t *testing.T) {
	dir := testutil.TempCwd(t)

	legacy := "# AGENT.md\n\nGenerated by agnostic-ai. Do not edit by hand.\n\nstale.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r", Body: "new rule"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENT.md")); !os.IsNotExist(err) {
		t.Errorf("legacy AGENT.md should be removed, err=%v", err)
	}
	bak := readFile(t, filepath.Join(dir, "AGENT.md.bak"))
	if !strings.Contains(bak, "stale.") {
		t.Errorf("backup should preserve old content: %s", bak)
	}
}

// An AGENT.md not authored by agnostic-ai is left alone (no rename).
func TestEmit_KeepsLegacyAGENTMd_WhenUserAuthored(t *testing.T) {
	dir := testutil.TempCwd(t)

	userContent := "# AGENT.md\n\nMy own notes.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r", Body: "new rule"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENT.md"))
	if got != userContent {
		t.Errorf("user-authored AGENT.md should be preserved verbatim: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENT.md.bak")); !os.IsNotExist(err) {
		t.Errorf("no backup should be made for user-authored file, err=%v", err)
	}
}

func TestEmit_CommandsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"amp": {CommandsDir: "vendor/amp/commands"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "ag", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/amp/commands/ag.md")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

// Stdio MCP emits to .amp/settings.json under amp.mcpServers (note dot).
func TestEmit_MCP_StdioWritesAmpMcpServersKey(t *testing.T) {
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
	got := readFile(t, filepath.Join(dir, ".amp/settings.json"))
	for _, want := range []string{
		`"amp.mcpServers"`,
		`"fs"`,
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
	got := readFile(t, filepath.Join(dir, ".amp/settings.json"))
	for _, want := range []string{
		`"linear"`,
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

	if err := os.MkdirAll(filepath.Join(dir, ".amp"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"amp.theme": "dark", "amp.editor.tabSize": 4}`
	if err := os.WriteFile(filepath.Join(dir, ".amp/settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".amp/settings.json"))
	for _, want := range []string{
		`"amp.theme": "dark"`,
		`"amp.editor.tabSize": 4`,
		`"amp.mcpServers"`,
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
			"amp": {MCPFile: "vendor/amp.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/amp.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_MCP_NoFileWhenNoEntries(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".amp/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no settings file when no MCP entries, err=%v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
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
