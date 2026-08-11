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

// TestEmit_WritesRulesInlineAndAgentsNatively confirms rule bodies
// inline into .junie/AGENTS.md, the only file Junie's strict-precedence
// guidelines lookup ever opens in a synced project (#552), rather than
// landing in the pre-fix `.junie/rules/` directory, which nothing
// reads, while agent bodies land at their own native
// `.junie/agents/<name>.md` file (#604) instead of inlining alongside
// the rules.
func TestEmit_WritesRulesInlineAndAgentsNatively(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/skills/sk1/SKILL.md")); err != nil {
		t.Errorf("missing .junie/skills/sk1/SKILL.md")
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/rules")); !os.IsNotExist(err) {
		t.Errorf("expected no .junie/rules directory, err=%v", err)
	}

	entryBody := readFile(t, filepath.Join(dir, ".junie/AGENTS.md"))
	for _, want := range []string{"### r1", "rule body"} {
		if !strings.Contains(entryBody, want) {
			t.Errorf(".junie/AGENTS.md missing %q:\n%s", want, entryBody)
		}
	}
	if strings.Contains(entryBody, "agent body") {
		t.Errorf(".junie/AGENTS.md must not inline the agent body once a native destination exists:\n%s", entryBody)
	}

	agentFile := readFile(t, filepath.Join(dir, ".junie/agents/ag1.md"))
	if !strings.Contains(agentFile, "agent body") {
		t.Errorf(".junie/agents/ag1.md missing the agent body:\n%s", agentFile)
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

// TestEmit_RulesDirOverride_SweepsStaleGeneratedFiles covers a project
// that customized outputs.junie.rules-dir under the pre-#552 adapter:
// this adapter no longer writes rule/agent files to any rules-dir (see
// the package doc), but the override still resolves the sweep target so
// a stale agnostic-ai-managed copy is cleaned up wherever the user had
// it pointed, exactly like RemoveGeneratedTree's other callers.
func TestEmit_RulesDirOverride_SweepsStaleGeneratedFiles(t *testing.T) {
	dir := testutil.TempCwd(t)

	stale := filepath.Join(dir, "custom/junie-rules/r1.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	staleContent := "<!-- Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`. -->\n\n# r1\n\nold rule body\n"
	if err := os.WriteFile(stale, []byte(staleContent), 0o644); err != nil {
		t.Fatal(err)
	}
	handAuthored := filepath.Join(dir, "custom/junie-rules/notes.md")
	if err := os.WriteFile(handAuthored, []byte("# hand-authored notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{"junie": {RulesDir: "custom/junie-rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("expected the stale agnostic-ai-managed file to be swept, err=%v", err)
	}
	if _, err := os.Stat(handAuthored); err != nil {
		t.Errorf("expected the hand-authored file to survive the sweep: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/rules")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default .junie/rules once overridden, err=%v", err)
	}

	got := readFile(t, filepath.Join(dir, ".junie/AGENTS.md"))
	if !strings.Contains(got, "rule body") {
		t.Errorf(".junie/AGENTS.md missing the rule body:\n%s", got)
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

// Junie's own lookup order always checks `.junie/AGENTS.md` first
// (target-audit 2026-08-08: junie.jetbrains.com/docs/junie-ide-plugin.html
// lists it as "the most preferred standard location", and #552 confirmed
// that lookup is strict precedence, not a merge, so this is the only
// file Junie ever opens in a synced project). This adapter writes the
// pointer body there unconditionally; a bare pointer with no rule
// content would leave Junie with nothing (#552's core bug), since there
// is no other surface for a rule to fall back to.
func TestEmit_WritesJunieAGENTSMd(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
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
	if !strings.Contains(got, "### r1") || !strings.Contains(got, "rule body") {
		t.Errorf(".junie/AGENTS.md missing the inlined rule body:\n%s", got)
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

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/AGENTS.md"))
	if !strings.Contains(got, "# Custom conventions") || !strings.Contains(got, "Hand-edited pointer body.") {
		t.Errorf(".junie/AGENTS.md must mirror the hand-edited AGNOSTIC_AI.md body, got:\n%s", got)
	}
	if !strings.Contains(got, "### r1") || !strings.Contains(got, "rule body") {
		t.Errorf(".junie/AGENTS.md must still inline the rule body on top of the hand-edited pointer, got:\n%s", got)
	}
	if n := strings.Count(got, "Generated by agnostic-ai"); n != 1 {
		t.Errorf("expected exactly one provenance header (no header stacking), found %d:\n%s", n, got)
	}
	if strings.Contains(got, "# AI Project Conventions") {
		t.Errorf("expected the hand-edited body, not the generated template, got:\n%s", got)
	}
}

// TestEmit_StaleAgentsAppendix_DroppedOnNextSync covers a project last
// synced by the pre-#604 adapter: `.junie/AGENTS.md` on disk still
// carries the old sentinel-marked `## Agents` block. `.junie/AGENTS.md`
// is fully regenerated from the canonical pointer body on every sync
// rather than patched in place, so the stale block disappears on the
// very next run with no dedicated sweep step, the same way any other
// stale content in that file would.
func TestEmit_StaleAgentsAppendix_DroppedOnNextSync(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	stale := "<!-- Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`. -->\n\n# AI Project Conventions\n\n<!-- agnostic-ai:agents:start -->\n\n## Agents\n\n### ag1\n\n<!-- source: agents/ag1.md -->\nold agent body\n\n<!-- agnostic-ai:agents:end -->\n"
	if err := os.MkdirAll(filepath.Join(dir, ".junie"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".junie", "AGENTS.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "new agent body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/AGENTS.md"))
	if strings.Contains(got, "## Agents") || strings.Contains(got, "old agent body") || strings.Contains(got, "agnostic-ai:agents:start") {
		t.Errorf("expected the stale ## Agents block swept on re-sync, got:\n%s", got)
	}

	agentFile := readFile(t, filepath.Join(dir, ".junie/agents/ag1.md"))
	if !strings.Contains(agentFile, "new agent body") {
		t.Errorf(".junie/agents/ag1.md missing the current agent body:\n%s", agentFile)
	}
}

// TestEmit_WritesNativeAgentFile confirms an agent spec lands at its
// native `.junie/agents/<name>.md` file with vendor-documented
// frontmatter fields passed through verbatim (#604): the file format
// needs no translation, since agnostic-ai's own field names already
// match junie.jetbrains.com/docs/junie-cli-subagents.html's example.
func TestEmit_WritesNativeAgentFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "code-review-helper",
			Meta: map[string]any{
				"description":         "Review a change and propose a safe patch",
				"tools":               []any{"Read", "Grep", "Edit"},
				"disallowedTools":     []any{"Bash", "WebSearch"},
				"mcpServers":          []any{"github"},
				"model":               "sonnet",
				"reasoningLevel":      "high",
				"maxTurns":            20,
				"skills":              []any{"kotlin", "writerside"},
				"allowPromptArgument": true,
			},
			MetaKeys: []string{"description", "tools", "disallowedTools", "mcpServers", "model", "reasoningLevel", "maxTurns", "skills", "allowPromptArgument"},
			Body:     "You are a careful code reviewer.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/agents/code-review-helper.md"))
	for _, want := range []string{
		"Generated by agnostic-ai",
		"description: Review a change and propose a safe patch",
		"tools:\n  - Read\n  - Grep\n  - Edit",
		"disallowedTools:\n  - Bash\n  - WebSearch",
		"mcpServers:\n  - github",
		"model: sonnet",
		"reasoningLevel: high",
		"maxTurns: 20",
		"skills:\n  - kotlin\n  - writerside",
		"allowPromptArgument: true",
		"You are a careful code reviewer.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf(".junie/agents/code-review-helper.md missing %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	entryBody := readFile(t, filepath.Join(dir, ".junie/AGENTS.md"))
	if strings.Contains(entryBody, "careful code reviewer") {
		t.Errorf(".junie/AGENTS.md must not also carry the agent body:\n%s", entryBody)
	}
}

// TestEmit_AgentsDirOverride_WritesToCustomDir confirms
// outputs.junie.agents-dir redirects the per-agent output, including
// to the shared `.agents/` tree the vendor also documents scanning.
func TestEmit_AgentsDirOverride_WritesToCustomDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"junie": {AgentsDir: ".agents"}},
	}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/ag1.md")); err != nil {
		t.Errorf("expected .agents/ag1.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/agents/ag1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default agents dir once overridden, err=%v", err)
	}
}

// TestEmit_WritesNativeCommandFile confirms a command spec lands at
// its native `.junie/commands/<name>.md` file (#605). The vendor
// documents `description` as the only frontmatter field, but any other
// key still passes through the same way emitAgents does.
func TestEmit_WritesNativeCommandFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind:     spec.KindCommand,
			Name:     "explain",
			Meta:     map[string]any{"description": "Explain the code in $file"},
			MetaKeys: []string{"description"},
			Body:     "Explain the code in $file and suggest improvements.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/commands/explain.md"))
	for _, want := range []string{
		"Generated by agnostic-ai",
		"description: Explain the code in $file",
		"Explain the code in $file and suggest improvements.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf(".junie/commands/explain.md missing %q:\n%s", want, got)
		}
	}
}

// TestEmit_CommandsDirOverride_WritesToCustomDir confirms
// outputs.junie.commands-dir redirects the per-command output.
func TestEmit_CommandsDirOverride_WritesToCustomDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"junie": {CommandsDir: "custom/commands"}},
	}
	entries := []spec.Entry{{Kind: spec.KindCommand, Name: "cmd1", Body: "command body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/commands/cmd1.md")); err != nil {
		t.Errorf("expected custom/commands/cmd1.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/commands/cmd1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default commands dir once overridden, err=%v", err)
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
