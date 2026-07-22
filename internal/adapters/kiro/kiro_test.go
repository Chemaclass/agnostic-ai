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

func TestEmit_Agent_ManualSteering(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ship-it", Body: "Run the release."}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".kiro/steering/agent-ship-it.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.HasPrefix(body, "---\ninclusion: manual\n---\n") {
		t.Errorf("expected inclusion: manual frontmatter first, got:\n%s", body)
	}
	if !strings.Contains(body, "Run the release.") {
		t.Errorf("missing agent body:\n%s", body)
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
