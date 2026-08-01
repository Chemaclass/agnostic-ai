package kiro

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
	if got := New().Name(); got != "kiro" {
		t.Errorf("expected kiro, got %s", got)
	}
}

func TestEmit_RuleUnscoped_InclusionAlways(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/steering/r1.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.HasPrefix(body, "---\ninclusion: always\n---\n") {
		t.Errorf("expected inclusion: always frontmatter first, got:\n%s", body)
	}
	if strings.Contains(body, "fileMatchPattern") {
		t.Errorf("unscoped rule should not carry fileMatchPattern:\n%s", body)
	}
	if !strings.Contains(body, "rule body") {
		t.Errorf("missing rule body:\n%s", body)
	}
}

func TestEmit_RuleWithGlobs_FileMatch(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body", Meta: map[string]any{"globs": "**/*.go"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/steering/r1.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.HasPrefix(body, "---\n") {
		t.Fatalf("frontmatter must be the first bytes of the file, got:\n%s", body)
	}
	if !strings.Contains(body, "inclusion: fileMatch") {
		t.Errorf("expected inclusion: fileMatch, got:\n%s", body)
	}
	if !strings.Contains(body, `fileMatchPattern: "**/*.go"`) && !strings.Contains(body, "fileMatchPattern: '**/*.go'") &&
		!strings.Contains(body, "fileMatchPattern: **/*.go") {
		t.Errorf("expected fileMatchPattern with the rule's glob, got:\n%s", body)
	}
}

func TestEmit_RuleWithScope_FileMatch(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/steering/auth.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "inclusion: fileMatch") {
		t.Errorf("expected inclusion: fileMatch for a scoped rule, got:\n%s", body)
	}
	if !strings.Contains(body, "backend/**") {
		t.Errorf("expected fileMatchPattern derived from scope, got:\n%s", body)
	}
}

// Agents emit at Kiro's native `.kiro/agents/<name>.md` surface
// (kiro.dev/docs/custom-agents/), not as a flattened steering file, so
// they appear in Kiro's own agent picker.
func TestEmit_Agent_WritesNativeAgentFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "ship-it",
			Meta: map[string]any{"description": "Ships releases.", "model": "sonnet"},
			Body: "Run the release.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/agents/ship-it.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.HasPrefix(body, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", body)
	}
	for _, want := range []string{"description: Ships releases.", "model: sonnet", "Run the release."} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "name:") {
		t.Errorf("kiro's agent schema has no name key; identity comes from the filename, got:\n%s", body)
	}
}

func TestEmit_Agent_DescriptionFallsBackToName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "no-desc", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/agents/no-desc.md"))
	if !strings.Contains(got, "description: no-desc") {
		t.Errorf("expected description fallback to agent name, got:\n%s", got)
	}
	if strings.Contains(got, "model:") || strings.Contains(got, "tools:") || strings.Contains(got, "name:") {
		t.Errorf("expected no name/model/tools keys when absent from meta, got:\n%s", got)
	}
}

// A generic `tools` list is agnostic-ai's Claude-style vocabulary,
// which Kiro's own tool identifiers are not confirmed to match (see the
// package doc), so it never reaches the frontmatter and surfaces one
// coverage note per sync instead of a silent no-op.
func TestEmit_Agent_ToolsSurfacesCoverageNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"Read"}}, Body: "body"},
		{Kind: spec.KindAgent, Name: "a2", Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note (only a1 declares tools), got %d", n)
	}
}

// A bundle where no agent sets tools must not surface a coverage note.
func TestEmit_Agent_NoToolsNoCoverageGap(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note when no agent sets tools, got %d", n)
	}
}

// x-kiro.tools is trusted to already be Kiro's own vocabulary (an
// author who writes directly under the kiro namespace is presumed to
// know it), so it passes through and does not count toward the
// coverage note the generic `tools` field triggers.
func TestEmit_Agent_XKiroToolsOverridePassesThrough(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{
				"description": "d",
				"tools":       []any{"Read"},
				"x-kiro":      map[string]any{"tools": []any{"fsRead"}},
			},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/agents/alpha.md"))
	if !strings.Contains(got, "fsRead") {
		t.Errorf("expected x-kiro.tools to pass through, got:\n%s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note once x-kiro.tools rescues the field, got %d", n)
	}
}

// x-kiro cannot resurrect name: Kiro's agent schema has no such key, so
// identity must stay tied to the filename even through the escape
// hatch. x-kiro.model, which is a documented field, still passes
// through.
func TestEmit_Agent_XKiroCannotReintroduceName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{
				"description": "d",
				"x-kiro":      map[string]any{"name": "override", "model": "opus"},
			},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/agents/alpha.md"))
	if strings.Contains(got, "name:") {
		t.Errorf("x-kiro must not reintroduce name, got:\n%s", got)
	}
	if !strings.Contains(got, "model: opus") {
		t.Errorf("expected x-kiro.model to pass through, got:\n%s", got)
	}
}

// Arbitrary x-kiro keys (mcpServers, permissions, hooks,
// keyboardShortcut, welcomeMessage, ...) pass through so the full
// documented agent schema is reachable without waiting on this
// adapter's allowlist.
func TestEmit_Agent_XKiroPassthrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{"description": "d", "x-kiro": map[string]any{"keyboardShortcut": "cmd+shift+a"}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kiro/agents/alpha.md"))
	if !strings.Contains(got, "keyboardShortcut: cmd+shift+a") {
		t.Errorf("expected x-kiro key to pass through, got:\n%s", got)
	}
}

func TestEmit_AgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"kiro": {AgentsDir: "custom/agents"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/agents/a1.md")); err != nil {
		t.Errorf("expected override dir to hold the agent file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/agents/a1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default agents dir, err=%v", err)
	}
}

// A prior sync's flattened `.kiro/steering/agent-<name>.md` (the old
// surface, see the package doc) is swept once the current sync writes
// the same agent natively, so a project does not end up with the agent
// duplicated under two different Kiro loading mechanisms.
func TestEmit_Agent_SweepsLegacySteeringFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	legacy := filepath.Join(dir, ".kiro/steering/agent-ship-it.md")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyBody := emit.WithHeader("---\ninclusion: manual\n---\n\nRun the release.", emit.FormatMarkdown)
	if err := os.WriteFile(legacy, []byte(legacyBody), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ship-it", Body: "Run the release."}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("expected the legacy steering file to be swept, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/agents/ship-it.md")); err != nil {
		t.Errorf("expected the native agent file to exist: %v", err)
	}
}

// A hand-authored file at the legacy path (no provenance header) is
// never touched by the sweep: RemoveGenerated only removes files it
// recognizes as agnostic-ai's own output.
func TestEmit_Agent_DoesNotSweepHandAuthoredLegacyFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	legacy := filepath.Join(dir, ".kiro/steering/agent-ship-it.md")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("---\ninclusion: manual\n---\n\nHand-authored.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ship-it", Body: "Run the release."}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatalf("expected the hand-authored legacy file to survive: %v", err)
	}
	if !strings.Contains(string(got), "Hand-authored.") {
		t.Errorf("hand-authored legacy file content changed:\n%s", got)
	}
}

func TestEmit_Skill_AutoSteeringWithNameDescription(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill, Name: "pdf-fill",
			Meta: map[string]any{"description": "fill PDF forms"},
			Body: "Fill in the form fields.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/steering/skill-pdf-fill.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.HasPrefix(body, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", body)
	}
	if !strings.Contains(body, "inclusion: auto") {
		t.Errorf("expected inclusion: auto, got:\n%s", body)
	}
	if !strings.Contains(body, "name: pdf-fill") {
		t.Errorf("expected name: pdf-fill, got:\n%s", body)
	}
	if !strings.Contains(body, "description: fill PDF forms") {
		t.Errorf("expected resolved description, got:\n%s", body)
	}
}

func TestEmit_Skill_DescriptionFallsBackToName(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "no-desc", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/steering/skill-no-desc.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "description: no-desc") {
		t.Errorf("expected description fallback to skill name, got:\n%s", got)
	}
}

func TestEmit_RulesDirOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
	cfg := &config.Config{Outputs: map[string]config.Output{"kiro": {RulesDir: "custom/steering"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/steering/r1.md")); err != nil {
		t.Errorf("expected override dir to hold the rule: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/steering/r1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default steering dir, err=%v", err)
	}
}

func TestEmit_MCPFileWritten(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "args": []any{"-y", "server"}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/settings/mcp.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, `"mcpServers"`) {
		t.Errorf("expected mcpServers map, got:\n%s", body)
	}
	if !strings.Contains(body, `"command": "npx"`) {
		t.Errorf("expected command field, got:\n%s", body)
	}
}

func TestEmit_NoMCPEntriesNoFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro/settings/mcp.json")); !os.IsNotExist(err) {
		t.Errorf("expected no mcp.json without mcp entries, err=%v", err)
	}
}

// A flat steering file cannot carry a skill's bundled sibling assets,
// so a folder-based skill with extra files (beyond SKILL.md) surfaces
// a coverage note instead of silently dropping them.
func TestEmit_SkillWithBundledAssets_NotesCoverageGap(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	skillDir := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "alpha", Path: filepath.Join(skillDir, "SKILL.md"), Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".kiro/steering/skill-alpha.md")); err != nil {
		t.Errorf("expected skill steering file: %v", err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note for the bundled-asset skill, got %d", n)
	}
}

// A flat-file skill (no sibling assets) must not trigger a coverage
// note.
func TestEmit_SkillWithoutBundledAssets_NoCoverageGap(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "s1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note for a flat-file skill, got %d", n)
	}
}

func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "a1", Body: "agent body"},
		{Kind: spec.KindSkill, Name: "s1", Body: "skill body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("kiro adapter must not write AGENTS.md; sync owns the entry-point, err=%v", err)
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
