package junie

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
	if got := New().Name(); got != "junie" {
		t.Errorf("expected junie, got %s", got)
	}
}

func TestEmit_WritesRulesAndAgents(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".junie/rules/r1.md", ".junie/rules/agent-ag1.md", ".junie/skills/sk1/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}

	got, err := os.ReadFile(filepath.Join(dir, ".junie/rules/r1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "rule") {
		t.Errorf("rule file missing body: %s", got)
	}
}

// Native Agent Skills shipped for Junie 2026-07-31 (target-audit
// 2026-08-01): "Project scope: `<projectRoot>/.junie/skills/<skill-name>/`"
// and "The `SKILL.md` file is required. A folder without it is not
// recognized as a skill." The pre-fix flat form
// (`.junie/rules/skill-<name>.md`) never reaches that path and drops
// any bundled asset sitting next to the skill's source SKILL.md.
func TestEmit_Skill_WritesFolderNotFlatFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "reviewer-kit", Meta: map[string]any{"description": "Review helpers."}, Body: "Use these helpers."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/skills/reviewer-kit/SKILL.md"))
	for _, want := range []string{"name: reviewer-kit", "description: Review helpers.", "Use these helpers."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/rules/skill-reviewer-kit.md")); !os.IsNotExist(err) {
		t.Errorf("expected no flat .junie/rules/skill-reviewer-kit.md, err=%v", err)
	}
}

// TestEmit_SkillsDirOverride_WritesToCustomDir confirms
// outputs.junie.skills-dir redirects the folder-per-skill output,
// consistent with every other emit.OutputSkillsDir consumer.
func TestEmit_SkillsDirOverride_WritesToCustomDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"junie": {SkillsDir: "custom/skills"}},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "sk1", Body: "skill body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/sk1/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/sk1/SKILL.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/skills/sk1/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default skills dir once overridden, err=%v", err)
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

func TestEmit_RulesDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"junie": {RulesDir: "custom/junie-rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/junie-rules/r1.md")); err != nil {
		t.Errorf("override should write under the custom dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/rules/r1.md")); !os.IsNotExist(err) {
		t.Errorf("override must not also write the default dir, err=%v", err)
	}
}

func TestEmit_MCPFileWritten(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "server"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".junie/mcp/mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, `"mcpServers"`) {
		t.Errorf("expected mcpServers key: %s", body)
	}
	if !strings.Contains(body, `"fs"`) {
		t.Errorf("expected fs server entry: %s", body)
	}
}

func TestEmit_NoMCPFileWhenNoEntries(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/mcp/mcp.json")); !os.IsNotExist(err) {
		t.Errorf("expected no mcp.json when bundle has no MCP entries, err=%v", err)
	}
}

func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter must not write the root AGENTS.md; sync owns that file, err=%v", err)
	}
}

// Junie's own lookup order prefers `.junie/AGENTS.md` over the root
// file (target-audit 2026-08-01: junie.jetbrains.com/docs/junie-ide-plugin.html
// lists it as "the most preferred standard location"). Whether Junie's
// own rules folder already counts as "a file found in the `.junie`
// folder" (which would make the root file unreachable) is unresolved
// upstream, so this adapter writes the pointer body to both locations
// regardless of bundle content, mirroring how sync always writes the
// root file for every enabled AGENTS.md-family target.
func TestEmit_WritesJunieAGENTSMd(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/AGENTS.md"))
	if !strings.Contains(got, "Generated by agnostic-ai") {
		t.Errorf(".junie/AGENTS.md missing provenance header:\n%s", got)
	}
	if !strings.Contains(got, "# AI Project Conventions") {
		t.Errorf(".junie/AGENTS.md missing the canonical pointer body:\n%s", got)
	}
	if n := strings.Count(got, "Generated by agnostic-ai"); n != 1 {
		t.Errorf("expected exactly one provenance header in .junie/AGENTS.md, found %d:\n%s", n, got)
	}
}

// A hand-edited .agnostic-ai/AGNOSTIC_AI.md drives every entry-point
// file, not just the root one: .junie/AGENTS.md must mirror the same
// custom body, header stripped and re-applied exactly once.
func TestEmit_JunieAGENTSMd_MirrorsHandEditedAgnosticFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	custom := "<!-- Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`. -->\n\n# Custom conventions\n\nHand-edited pointer body.\n"
	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/AGENTS.md"))
	if !strings.Contains(got, "# Custom conventions") || !strings.Contains(got, "Hand-edited pointer body.") {
		t.Errorf(".junie/AGENTS.md must mirror the hand-edited AGNOSTIC_AI.md body, got:\n%s", got)
	}
	if n := strings.Count(got, "Generated by agnostic-ai"); n != 1 {
		t.Errorf("expected exactly one provenance header (no header stacking), found %d:\n%s", n, got)
	}
	if strings.Contains(got, "# AI Project Conventions") {
		t.Errorf("expected the hand-edited body, not the generated template, got:\n%s", got)
	}
}

// This adapter reads AGNOSTIC_AI.md when present but never creates it:
// that bootstrap stays sync's responsibility (internal/cli/entrypoint.go).
func TestEmit_DoesNotCreateAgnosticAIFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")); !os.IsNotExist(err) {
		t.Errorf("adapter must not create AGNOSTIC_AI.md itself; sync's central write owns that bootstrap, err=%v", err)
	}
}
