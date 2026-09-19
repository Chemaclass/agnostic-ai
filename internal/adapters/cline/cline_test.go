package cline

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

func TestEmit_WritesRulesAgentsAndSkills(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	a := New()
	if a.Name() != "cline" {
		t.Errorf("expected cline, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := a.Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".clinerules/r1.md", ".cline/agents/ag1.yml", ".cline/skills/sk1/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	// The pre-fix flat form must not be written; Cline only loads
	// SKILL.md from a folder (docs.cline.bot/customization/skills).
	if _, err := os.Stat(filepath.Join(dir, ".clinerules/skill-sk1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no flat .clinerules/skill-sk1.md, err=%v", err)
	}
	// Agents no longer share the rules directory (target-audit
	// 2026-08-01, #534): Cline's current config reference lists
	// `.cline/agents/` as its own top-level dir.
	if _, err := os.Stat(filepath.Join(dir, ".clinerules/agent-ag1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no rule-form agent-ag1.md under rules dir, err=%v", err)
	}
}

// TestEmit_SkillsDirOverride_WritesToCustomDir confirms
// outputs.cline.skills-dir redirects the folder-per-skill output,
// consistent with every other emit.OutputSkillsDir consumer.
func TestEmit_SkillsDirOverride_WritesToCustomDir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"cline": {SkillsDir: "custom/skills"},
		},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "sk1", Body: "skill body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/sk1/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/sk1/SKILL.md: %v", err)
	}
}

func TestEmit_NestedScopeRoutesUnderSubdir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend/api", Body: "rule"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".clinerules/backend/api/auth.md")); err != nil {
		t.Errorf("expected nested rules under .clinerules/backend/api: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend/api/.clinerules/auth.md")); !os.IsNotExist(err) {
		t.Errorf("expected no stray scope dir at repo root, err=%v", err)
	}
}

func TestEmit_WorkflowsDirEmitsAgentsAsWorkflows(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "ship-it",
			Meta: map[string]any{"description": "open and merge a PR"},
			Body: "Run the release.",
		},
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"cline": {WorkflowsDir: ".cline/workflows"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}

	wfPath := filepath.Join(dir, ".cline/workflows/ship-it.md")
	got, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("missing workflow: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "_open and merge a PR_") {
		t.Errorf("workflow missing description italic: %q", body)
	}
	if !strings.Contains(body, "Run the release.") {
		t.Errorf("workflow missing body: %q", body)
	}

	// The native agent file still emits alongside the workflow.
	if _, err := os.Stat(filepath.Join(dir, ".cline/agents/ship-it.yml")); err != nil {
		t.Errorf("agent file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".clinerules/r1.md")); err != nil {
		t.Errorf("rule missing: %v", err)
	}
}

// Cline's own workflows path, `.clinerules/workflows`, nests inside
// the default rules dir. Rules emission must leave it alone: nothing
// in Emit prunes the rules tree it just wrote into.
func TestEmit_WorkflowsDirNestedInRulesTreeCoexists(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "ship-it", Body: "Run the release."},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"cline": {WorkflowsDir: ".clinerules/workflows"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".clinerules/workflows/ship-it.md"))
	if err != nil {
		t.Fatalf("missing workflow: %v", err)
	}
	if !strings.Contains(string(got), "Run the release.") {
		t.Errorf("workflow missing body: %q", got)
	}
}

func TestEmit_RulesCarryProvenanceHeader(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".clinerules/r1.md", ".cline/agents/ag1.yml"} {
		got, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if !strings.Contains(string(got), "Generated by agnostic-ai") {
			t.Errorf("%s missing provenance header:\n%s", p, got)
		}
	}
}

func TestEmit_NoWorkflowsDirNoEmit(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cline/workflows")); !os.IsNotExist(err) {
		t.Errorf("expected no workflows dir; err=%v", err)
	}
}

// The default rules dir is `.clinerules`, the path the shipping VS
// Code extension hardcodes as GlobalFileNames.clineRules and resolves
// against the workspace root. `.cline/rules` appears only in the
// project tree on docs.cline.bot/getting-started/config and no Cline
// loader reads it, so it is reachable only as an explicit opt-in
// (target-audit 2026-09-18, #853).
func TestEmit_DefaultsToTheRulesDirClineReads(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".clinerules/r1.md")); err != nil {
		t.Errorf("rules must default to the path Cline reads: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cline/rules/r1.md")); !os.IsNotExist(err) {
		t.Errorf("no rule may land at the unread .cline/rules, err=%v", err)
	}
}

func TestEmit_RulesDirOverride_KeepsUnreadClineTree(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"cline": {RulesDir: ".cline/rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cline/rules/r1.md")); err != nil {
		t.Errorf("override should keep the opted-in tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".clinerules/r1.md")); !os.IsNotExist(err) {
		t.Errorf("override must not also write the default, err=%v", err)
	}
}

// Managed leftovers at the unread .cline/rules path are swept on sync
// (a project synced between #534 and #853 has its rules there);
// hand-authored files survive.
func TestEmit_SweepsUnreadClineRulesTree(t *testing.T) {
	dir := testutil.TempCwd(t)

	unread := filepath.Join(dir, ".cline", "rules")
	if err := os.MkdirAll(unread, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unread, "old.md"),
		[]byte("<!-- Generated by agnostic-ai -->\n\nold managed rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unread, "mine.md"),
		[]byte("hand-authored, no marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1/SKILL.md", Body: "skill body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(unread, "old.md")); !os.IsNotExist(err) {
		t.Errorf("managed rule at the unread path should be swept, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(unread, "mine.md")); err != nil {
		t.Errorf("hand-authored file must survive: %v", err)
	}
	// The sweep targets .cline/rules only. Skills live one level up at
	// .cline/skills and are confirmed by GlobalFileNames.
	if _, err := os.Stat(filepath.Join(dir, ".cline", "skills", "sk1", "SKILL.md")); err != nil {
		t.Errorf("sweep must not reach sibling .cline surfaces: %v", err)
	}
}

// A project that still carries the pre-#534 combined layout keeps its
// hand-authored .clinerules files: rules now emit into that same tree,
// and nothing sweeps it.
func TestEmit_NeverSweepsTheTreeClineReads(t *testing.T) {
	dir := testutil.TempCwd(t)

	rules := filepath.Join(dir, ".clinerules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "old.md"),
		[]byte("<!-- Generated by agnostic-ai -->\n\nold managed rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "mine.md"),
		[]byte("hand-authored, no marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"old.md", "mine.md"} {
		if _, err := os.Stat(filepath.Join(rules, name)); err != nil {
			t.Errorf("%s must survive; Emit must never sweep the tree Cline reads: %v", name, err)
		}
	}
}

// TestEmit_AgentsDirOverride confirms outputs.cline.agents-dir
// redirects the native agent output, consistent with every other
// emit.OutputAgentsDir consumer.
func TestEmit_AgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"cline": {AgentsDir: "custom/agents"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "agent body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/agents/a1.yml")); err != nil {
		t.Errorf("expected override dir to hold the agent file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cline/agents/a1.yml")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default agents dir, err=%v", err)
	}
}
