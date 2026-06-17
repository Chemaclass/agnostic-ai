package cursor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Cursor has no settings surface, so a settings spec is unsupported and
// reported (errors under on-unsupported: error). Confirms the settings
// kind flows through the generic capability path (#432).
func TestEmit_SettingsKindUnsupported(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "x"}},
	})
	err := New().Emit(b, &config.Config{OnUnsupported: "error"}, false)
	if err == nil || !strings.Contains(err.Error(), "settings") {
		t.Fatalf("expected unsupported-settings error, got %v", err)
	}
}

func TestEmit_WritesMdcRule(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	a := New()
	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "my-rule",
			Meta: map[string]any{"description": "desc", "globs": "**/*.go"},
			Body: "rule body",
		},
	}
	if err := a.Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".cursor/rules/my-rule.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `globs: "**/*.go"`) {
		t.Errorf("missing globs: %s", got)
	}
	if !strings.Contains(string(got), "alwaysApply: true") {
		t.Errorf("rule should default alwaysApply=true: %s", got)
	}
}

func TestEmit_AgentDefaultsAlwaysApplyFalse(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "agent1", Meta: map[string]any{}, Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".cursor/rules/agent1.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "alwaysApply: false") {
		t.Errorf("agent should default alwaysApply=false: %s", got)
	}
}

func TestEmit_NestedScopeRoutesUnderSubdir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend", Body: "rule"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor/rules/backend/auth.mdc")); err != nil {
		t.Errorf("expected nested file under .cursor/rules/backend: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend/.cursor/rules/auth.mdc")); !os.IsNotExist(err) {
		t.Errorf("expected no stray scope dir at repo root, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor/rules/auth.mdc")); !os.IsNotExist(err) {
		t.Errorf("expected no root file when scope set, err=%v", err)
	}
}

func TestEmit_WritesMCPFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

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
	got, err := os.ReadFile(filepath.Join(dir, ".cursor/mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"mcpServers"`) {
		t.Errorf("expected mcpServers key: %s", got)
	}
}

func TestEmit_SkillWritesMdcFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "sk1", Meta: map[string]any{"description": "skill desc"}, Body: "skill body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".cursor/rules/skill-sk1.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "description: skill desc") {
		t.Errorf("missing description: %s", got)
	}
	if !strings.Contains(string(got), "alwaysApply: false") {
		t.Errorf("skill should default alwaysApply=false: %s", got)
	}
}

func TestEmit_CommandsDirEmitsAgentAsCommand(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			target: {CommandsDir: ".cursor/commands"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "code-reviewer", Meta: map[string]any{
			"description": "reviews code",
			"model":       "sonnet-4-6",
		}, Body: "Body of the prompt.\n"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(dir, ".cursor/commands/code-reviewer.md")
	got, err := os.ReadFile(commandPath)
	if err != nil {
		t.Fatalf("expected command file at %s: %v", commandPath, err)
	}
	if !strings.Contains(string(got), "description: reviews code") {
		t.Errorf("missing description in command: %s", got)
	}
	if !strings.Contains(string(got), "model: sonnet-4-6") {
		t.Errorf("missing model in command: %s", got)
	}
	if !strings.Contains(string(got), "Body of the prompt.") {
		t.Errorf("missing body in command: %s", got)
	}
	// Rule-form emission must still happen (back-compat).
	if _, err := os.Stat(filepath.Join(dir, ".cursor/rules/code-reviewer.mdc")); err != nil {
		t.Errorf("rule-form emission must remain when commands-dir is set: %v", err)
	}
}

func TestEmit_RulesCarryProvenanceHeader(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Meta: map[string]any{"description": "d"}, Body: "body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".cursor/rules/r1.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Generated by agnostic-ai") {
		t.Errorf("cursor rule missing provenance header:\n%s", got)
	}
	// Frontmatter must still parse cleanly: the header must live below
	// the closing `---` delimiter.
	idxFM := strings.Index(string(got), "---\n")
	idxClose := strings.Index(string(got), "\n---\n")
	idxHeader := strings.Index(string(got), "<!-- Generated")
	if idxFM != 0 || idxHeader < idxClose {
		t.Errorf("header must follow frontmatter close:\n%s", got)
	}
}

func TestEmit_NoCommandsDirSkipsCommandEmission(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "agent1", Meta: map[string]any{}, Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor/commands/agent1.md")); err == nil {
		t.Error("commands dir not configured; should not emit command file")
	}
}
