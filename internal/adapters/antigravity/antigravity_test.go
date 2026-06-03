package antigravity

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
	if got := New().Name(); got != "antigravity" {
		t.Errorf("Name() = %q, want %q", got, "antigravity")
	}
}

func TestEmit_WritesRulesDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "commits", Body: "Use conventional commits."},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".agent/rules/commits.md"))
	if !strings.Contains(got, "Use conventional commits.") {
		t.Errorf("missing rule body in %s", got)
	}
}

func TestEmit_NoAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "security", Path: "rules/security.md", Body: "Never expose secrets."},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent/AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md by default; sync owns the entry-point, err=%v", err)
	}
}

func TestEmit_LegacyRulesFile_WritesConcatenated(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"antigravity": {RulesFile: ".agent/AGENTS.md"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "security", Path: "rules/security.md", Body: "Never expose secrets."},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agent/AGENTS.md"))
	if !strings.Contains(got, "Never expose secrets.") {
		t.Errorf("legacy rules-file should contain concatenated rule body:\n%s", got)
	}
}

func TestEmit_AgentRuleFile_StillWritten(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "deployer",
			Meta: map[string]any{"description": "Run deployments."},
			Body: "Deploy to production.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	ruleFile := filepath.Join(dir, ".agent/rules/agent-deployer.md")
	if _, err := os.Stat(ruleFile); err != nil {
		t.Errorf("expected agent rule file at %s: %v", ruleFile, err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent")); !os.IsNotExist(err) {
		t.Errorf("expected no .agent/ dir for empty bundle, err=%v", err)
	}
}

func TestEmit_RulesDirOverride_WritesRulesToCustomDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"antigravity": {RulesDir: "custom/rules"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "body"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/rules/r1.md")); err != nil {
		t.Errorf("expected custom/rules/r1.md: %v", err)
	}
}

func TestEmit_Skill_WritesNativeSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "deploy",
			Meta: map[string]any{"description": "Run deployments."},
			Body: "Deploy to production.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".agent/skills/deploy/SKILL.md"))
	for _, want := range []string{"name: deploy", "description: Run deployments.", "Deploy to production."} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, got)
		}
	}
	// The skill must NOT also leak a rule-form file.
	if _, err := os.Stat(filepath.Join(dir, ".agent/rules/skill-deploy.md")); !os.IsNotExist(err) {
		t.Errorf("skill should not emit a rule-form file, err=%v", err)
	}
}

func TestEmit_SkillsDirOverride_WritesToCustomDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"antigravity": {SkillsDir: "custom/skills"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "deploy", Body: "body"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/deploy/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/deploy/SKILL.md: %v", err)
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
