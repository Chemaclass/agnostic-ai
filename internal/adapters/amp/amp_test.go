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

// Command specs write nothing: a full sweep of Amp's docs finds no
// file-based command surface, and its migration guidance is to delete
// the old command file rather than point at a replacement path.
// Writing to .agents/commands/ for a Command spec would be a file Amp
// never reads. See #553.
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

// Agent specs write no command file: Amp removed custom commands on
// 2026-01-29 and its migration steps end with "Delete the original
// command file", so `.agents/commands/` is a directory the vendor tells
// users to delete. See #727.
func TestEmit_Agent_WritesNoCommandFile(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(dir, ".agents/commands/pr-reviewer.md")); !os.IsNotExist(err) {
		t.Errorf("agent spec should not write into the retired commands dir, err=%v", err)
	}
}

// The one place an agent body reaches Amp is the opted-in merged rules
// document, which is why KindAgent stays in caps.Supports.
func TestEmit_Agent_ReachesLegacyRulesFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"amp": {RulesFile: "AGENTS.md"}},
	}
	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "pr-reviewer",
			Path: "agents/pr-reviewer.md",
			Meta: map[string]any{"description": "Review PRs like an owner."},
			Body: "Open the PR. Read it. Comment.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{"## Agents", "### pr-reviewer", "Open the PR. Read it. Comment."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in merged rules file:\n%s", want, got)
		}
	}
}

// Without outputs.amp.rules-file an agent reaches Amp through no
// emitted file at all, so the user gets a coverage note rather than a
// silent green sync (the failure mode #727 reported).
func TestEmit_Agent_NotesCoverageGap_WhenNoRulesFile(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "pr-reviewer", Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCoverageNotesCount(); got != 1 {
		t.Errorf("expected 1 coverage note for an agent with no rules-file, got %d", got)
	}
}

// A prior sync's `.agents/commands/<name>.md` is swept so upgrading
// users do not keep a stale file Amp no longer reads. Hand-authored
// files there carry no provenance header and survive.
func TestEmit_SweepsRetiredCommandsDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	commands := filepath.Join(dir, ".agents", "commands")
	if err := os.MkdirAll(commands, 0o755); err != nil {
		t.Fatal(err)
	}
	generated := emit.WithHeader("stale agent body\n", emit.FormatMarkdown)
	if err := os.WriteFile(filepath.Join(commands, "pr-reviewer.md"), []byte(generated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commands, "mine.md"), []byte("my own notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(commands, "pr-reviewer.md")); !os.IsNotExist(err) {
		t.Errorf("generated command file should be swept, err=%v", err)
	}
	if got := readFile(t, filepath.Join(commands, "mine.md")); got != "my own notes\n" {
		t.Errorf("user-authored file should survive the sweep, got %q", got)
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

// A skill scopes its own MCP servers through x-amp.mcpServers, which is
// the surface Amp's manual recommends over user settings "for most use
// cases": "a skill can define MCP servers in a sibling `mcp.json` file or
// in the `mcpServers` field of its `SKILL.md` frontmatter", and Amp
// prefers `mcpServers` when both are present.
//
// This needs no cross-kind spec relationship; the existing passthrough
// already lands the key verbatim. Pinned here because the docs now point
// users at it, so silently dropping it would break a documented path
// (#591).
func TestEmit_Skill_XAmpMCPServersReachesFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "deploy",
			Meta: map[string]any{
				"description": "Deploy the service.",
				"x-amp": map[string]any{
					"mcpServers": map[string]any{
						"deployer": map[string]any{
							"command": "npx",
							"args":    []any{"-y", "@acme/deploy-mcp"},
						},
					},
				},
			},
			Body: "Run the deploy.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/deploy/SKILL.md"))
	for _, want := range []string{"mcpServers:", "deployer:", "command: npx", "@acme/deploy-mcp"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in skill frontmatter:\n%s", want, got)
		}
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

func swapAmpWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	return buf
}

// outputs.amp.commands-dir is inert now that Amp reads no command
// directory: pointing it somewhere else must not resurrect the surface,
// and the user hears why instead of finding an empty path.
func TestEmit_CommandsDirOverride_WarnsAndWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapAmpWarner(t)

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
	if _, err := os.Stat(filepath.Join(dir, "vendor/amp/commands/ag.md")); !os.IsNotExist(err) {
		t.Errorf("commands-dir override should write nothing, err=%v", err)
	}
	out := buf.String()
	for _, want := range []string{"outputs.amp.commands-dir", "vendor/amp/commands", "removed custom commands"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected warning to name %q, got: %q", want, out)
		}
	}
}

func TestEmit_NoCommandsDir_NoWarning(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapAmpWarner(t)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); out != "" {
		t.Errorf("expected no warning when commands-dir is unset, got: %q", out)
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

// buildMCPEntry enumerated a fixed field set with no escape hatch, so
// even an explicit `x-amp` key was dropped, unlike the skill renderer
// which merges one. The field this unblocks is
// `includeTools`, "optional but recommended" per
// ampcode.com/docs/customize/skills (#634).
func TestEmit_MCP_XAmpPassthroughReachesIncludeTools(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "linear",
			Meta: map[string]any{
				"type": "http", "url": "https://mcp.linear.app/sse",
				"x-amp": map[string]any{
					"includeTools": []any{"list_issues", "create_issue"},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".amp/settings.json"))
	for _, want := range []string{`"includeTools"`, `"list_issues"`, `"create_issue"`} {
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
