package antigravity

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
	if got := New().Name(); got != "antigravity" {
		t.Errorf("Name() = %q, want %q", got, "antigravity")
	}
}

func TestEmit_WritesRulesDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "commits", Body: "Use conventional commits."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".agents/rules/commits.md"))
	if !strings.Contains(got, "Use conventional commits.") {
		t.Errorf("missing rule body in %s", got)
	}
}

func TestEmit_NoAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "security", Path: "rules/security.md", Body: "Never expose secrets."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	ruleFile := filepath.Join(dir, ".agents/rules/agent-deployer.md")
	if _, err := os.Stat(ruleFile); err != nil {
		t.Errorf("expected agent rule file at %s: %v", ruleFile, err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents")); !os.IsNotExist(err) {
		t.Errorf("expected no .agents/ dir for empty bundle, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy .agent/ dir for empty bundle, err=%v", err)
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".agents/skills/deploy/SKILL.md"))
	for _, want := range []string{"name: deploy", "description: Run deployments.", "Deploy to production."} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, got)
		}
	}
	// The skill must NOT also leak a rule-form file.
	if _, err := os.Stat(filepath.Join(dir, ".agents/rules/skill-deploy.md")); !os.IsNotExist(err) {
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
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/deploy/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/deploy/SKILL.md: %v", err)
	}
}

// antigravity.google/docs/mcp documents `cwd` on stdio MCP servers
// (issue #556).
func TestEmit_MCP_StdioWritesCwd(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"cwd":     "/workspace/project",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/mcp_config.json"))
	if !strings.Contains(got, `"cwd": "/workspace/project"`) {
		t.Errorf("missing cwd in %s", got)
	}
}

// antigravity.google/docs/mcp documents `headers` for remote servers
// (issue #556). The remote branch must keep using `serverUrl`, never
// `url`: the vendor doc states legacy `url` / `httpUrl` "are not
// supported."
func TestEmit_MCP_RemoteWritesHeadersAndKeepsServerURL(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type":    "http",
				"url":     "https://api.githubcopilot.com/mcp/",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/mcp_config.json"))
	for _, want := range []string{
		`"serverUrl": "https://api.githubcopilot.com/mcp/"`,
		`"Authorization": "Bearer x"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, `"url":`) {
		t.Errorf("remote servers must use serverUrl, not the unsupported url key, got:\n%s", got)
	}
}

// antigravity.google/docs/mcp documents `disabled` as Antigravity's own
// key (issue #556), unlike codex and kilo which map the spec's
// `disabled` onto their own `enabled: false`.
func TestEmit_MCP_StdioDisabledWritesNativeKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/mcp_config.json"))
	if !strings.Contains(got, `"disabled": true`) {
		t.Errorf("Antigravity documents its own disabled key; must pass it through, got:\n%s", got)
	}
}

// The disabled passthrough applies to remote servers too, not only stdio.
func TestEmit_MCP_RemoteDisabledWritesNativeKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP, Name: "linear",
			Meta: map[string]any{"type": "http", "url": "https://mcp.linear.app", "disabled": true},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/mcp_config.json"))
	if !strings.Contains(got, `"disabled": true`) {
		t.Errorf("expected disabled: true on a disabled remote server, got:\n%s", got)
	}
}

// A spec that does not set `disabled` must not gain a `disabled` key.
func TestEmit_MCP_NotDisabledNoDisabledKey(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/mcp_config.json"))
	if strings.Contains(got, `"disabled"`) {
		t.Errorf("expected no disabled key when disabled is unset, got:\n%s", got)
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
