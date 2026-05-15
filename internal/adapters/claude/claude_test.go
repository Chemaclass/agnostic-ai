package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_WritesAgent(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	a := New()
	if a.Name() != "claude" {
		t.Errorf("expected claude, got %s", a.Name())
	}
	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "reviewer",
			Meta: map[string]any{"name": "reviewer"},
			Body: "do reviews",
		},
	}
	if err := a.Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "do reviews") {
		t.Errorf("missing agent body in %s", got)
	}
}

func TestEmit_PreservesArbitraryAgentFrontmatter(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{
		Kind: spec.KindAgent,
		Name: "reviewer",
		Body: "do reviews",
		Meta: map[string]any{
			"name":              "reviewer",
			"description":       "code reviewer",
			"custom-vendor-key": "vendor-value",
			"tools":             []any{"Read", "Edit"},
		},
	}}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude/agents/reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"custom-vendor-key: vendor-value",
		"tools:",
		"do reviews",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_WritesSkillNested(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "validator", Body: "skill body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/skills/validator/SKILL.md")); err != nil {
		t.Error("expected nested skill file")
	}
}

func TestEmit_PreservesArbitrarySkillFrontmatter(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{
		Kind: spec.KindSkill,
		Name: "validator",
		Body: "skill body",
		Meta: map[string]any{
			"name":               "validator",
			"description":        "validate stuff",
			"custom-vendor-key":  "vendor-value",
			"experimental-flags": []any{"a", "b"},
			"nested": map[string]any{
				"deep": "value",
			},
		},
	}}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude/skills/validator/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"custom-vendor-key: vendor-value",
		"experimental-flags:",
		"nested:",
		"deep: value",
		"skill body",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_PropagatesSkillAssets(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	srcSkill := filepath.Join(dir, ".agnostic-ai", "skills", "validator")
	if err := os.MkdirAll(filepath.Join(srcSkill, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "SKILL.md"), []byte("---\nname: validator\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "scripts", "run.py"), []byte("print('ok')\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "README.md"), []byte("docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "validator",
			Path: filepath.Join(srcSkill, "SKILL.md"),
			Meta: map[string]any{"name": "validator"},
			Body: "body",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"scripts/run.py", "README.md"} {
		got, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "validator", filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("missing propagated asset %q: %v", rel, err)
			continue
		}
		want, _ := os.ReadFile(filepath.Join(srcSkill, filepath.FromSlash(rel)))
		if string(got) != string(want) {
			t.Errorf("asset %q not byte-identical:\ngot:  %q\nwant: %q", rel, got, want)
		}
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, ".claude", "skills", "validator", "scripts", "run.py"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("executable bit dropped on emit: mode=%v", info.Mode().Perm())
		}
	}
}

func TestEmit_WritesRulesPerFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule one"},
		{Kind: spec.KindRule, Name: "r2", Body: "rule two"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{".claude/rules/r1.md", ".claude/rules/r2.md"} {
		got, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
		if !strings.Contains(string(got), "rule") {
			t.Errorf("%s missing body: %s", p, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("CLAUDE.md should not be written by default, got err=%v", err)
	}
}

func TestEmit_RuleWithoutMetaHasNoLeadingBlankLineOrSyntheticHeading(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body line\n"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/rules/r1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(got), "\n") {
		t.Errorf("rule file starts with blank line: %q", got)
	}
	if strings.Contains(string(got), "# r1") {
		t.Errorf("rule file has synthetic # heading: %q", got)
	}
	if !strings.HasSuffix(string(got), "rule body line\n") {
		t.Errorf("expected body after header, got %q", got)
	}
}

func TestEmit_RuleWithMetaRoundTripsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule,
			Name: "r1",
			Meta: map[string]any{"name": "r1", "description": "short"},
			Body: "rule body\n",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/rules/r1.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.HasPrefix(s, "---\n") {
		t.Errorf("expected frontmatter start, got %q", s)
	}
	if !strings.Contains(s, "description: short") {
		t.Errorf("missing description in frontmatter: %q", s)
	}
	if strings.Contains(s, "# r1") {
		t.Errorf("synthetic # heading must not be injected: %q", s)
	}
	if !strings.Contains(s, "rule body") {
		t.Errorf("missing body: %q", s)
	}
}

func TestEmit_AgentWithoutMetaHasNoLeadingBlankLine(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "reviewer", Body: "do reviews\n"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/agents/reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(got), "\n") {
		t.Errorf("agent file starts with blank line: %q", got)
	}
	if !strings.HasSuffix(string(got), "do reviews\n") {
		t.Errorf("expected body after header, got %q", got)
	}
}

func TestEmit_SkillWithoutMetaHasNoLeadingBlankLine(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "validator", Body: "skill body\n"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/skills/validator/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(got), "\n") {
		t.Errorf("skill file starts with blank line: %q", got)
	}
	if !strings.HasSuffix(string(got), "skill body\n") {
		t.Errorf("expected body after header, got %q", got)
	}
}

func TestEmit_CommandWithoutMetaHasNoLeadingBlankLine(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindCommand, Name: "deploy", Body: "run deploy\n"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/commands/deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(got), "\n") {
		t.Errorf("command file starts with blank line: %q", got)
	}
	if !strings.HasSuffix(string(got), "run deploy\n") {
		t.Errorf("expected body after header, got %q", got)
	}
}

func TestEmit_RulesFileOverrideConcatenates(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {RulesFile: "CLAUDE.md"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule one"},
		{Kind: spec.KindRule, Name: "r2", Body: "rule two"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "rule one") || !strings.Contains(string(got), "rule two") {
		t.Errorf("rules missing: %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/rules/r1.md")); !os.IsNotExist(err) {
		t.Errorf("rules dir should not be written when rules-file is set")
	}
}

func TestEmit_WritesHookSettings(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "h1",
			Meta: map[string]any{
				"event":   "PostToolUse",
				"matcher": "Edit",
				"command": "echo hi",
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Errorf("expected hooks key in settings.json: %s", raw)
	}
}

func TestEmit_MergesHooksBySameEventAndMatcher(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "cmd1",
		}},
		{Kind: spec.KindHook, Name: "h2", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "cmd2",
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	groups := doc.Hooks["PostToolUse"]
	if len(groups) != 1 {
		t.Fatalf("expected 1 matcher group, got %d: %s", len(groups), raw)
	}
	if len(groups[0].Hooks) != 2 {
		t.Fatalf("expected 2 commands in matcher group, got %d: %s", len(groups[0].Hooks), raw)
	}
	if groups[0].Hooks[0].Command != "cmd1" || groups[0].Hooks[1].Command != "cmd2" {
		t.Errorf("commands wrong order: %s", raw)
	}
}

func TestEmit_HookCommandAsList(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event":   "PreToolUse",
			"matcher": "Bash",
			"command": []any{"cmd1", "cmd2", "cmd3"},
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"cmd1", "cmd2", "cmd3"} {
		if !strings.Contains(string(raw), cmd) {
			t.Errorf("missing %q: %s", cmd, raw)
		}
	}
}

func TestEmit_PreservesUserKeysInSettings(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	settingsPath := filepath.Join(dir, ".claude/settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
  "statusLine": {"type": "command", "command": "echo status"},
  "enabledPlugins": ["foo", "bar"]
}
`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "echo hi",
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	if _, ok := parsed["statusLine"]; !ok {
		t.Errorf("statusLine missing after sync: %s", raw)
	}
	if _, ok := parsed["enabledPlugins"]; !ok {
		t.Errorf("enabledPlugins missing after sync: %s", raw)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Errorf("hooks not written: %s", raw)
	}
}

func TestEmit_LoadsSettingsOverlay(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{
  "statusLine": {"type": "command", "command": "echo status"},
  "enabledPlugins": ["foo", "bar"]
}
`
	if err := os.WriteFile(overlayPath, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "echo hi",
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	if _, ok := parsed["statusLine"]; !ok {
		t.Errorf("statusLine missing after sync: %s", raw)
	}
	if _, ok := parsed["enabledPlugins"]; !ok {
		t.Errorf("enabledPlugins missing after sync: %s", raw)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Errorf("hooks not written: %s", raw)
	}
}

func TestEmit_OverlayWithoutHooks(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{"statusLine": {"type": "command", "command": "echo status"}}` + "\n"
	if err := os.WriteFile(overlayPath, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatalf("expected settings.json with overlay-only content: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	if _, ok := parsed["statusLine"]; !ok {
		t.Errorf("statusLine missing: %s", raw)
	}
	if _, ok := parsed["hooks"]; ok {
		t.Errorf("hooks should be absent without hook specs: %s", raw)
	}
}

func TestEmit_OverlayKeyOrderSurvivesSync(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{
  "hooks": {"PreToolUse": []},
  "statusLine": {"type": "command", "command": "echo status"},
  "enabledPlugins": {"plugin-a": true}
}
`
	if err := os.WriteFile(overlayPath, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "echo hi",
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	idxHooks := strings.Index(s, `"hooks"`)
	idxStatus := strings.Index(s, `"statusLine"`)
	idxPlugins := strings.Index(s, `"enabledPlugins"`)
	if !(idxHooks >= 0 && idxStatus > idxHooks && idxPlugins > idxStatus) {
		t.Errorf("expected hooks < statusLine < enabledPlugins order, got:\n%s", s)
	}
	// Inner keys of statusLine (an overlay-owned block) must preserve
	// source order: type before command.
	statusBlock := extractBlock(s, `"statusLine":`)
	idxType := strings.Index(statusBlock, `"type":`)
	idxCmd := strings.Index(statusBlock, `"command":`)
	if !(idxType >= 0 && idxCmd > idxType) {
		t.Errorf("statusLine nested keys re-ordered (want type before command), block:\n%s", statusBlock)
	}
}

// extractBlock returns the substring of doc starting from the position
// of key up to the matching closing brace at the same nesting level.
// Best-effort; returns the doc tail when the brace search fails.
func extractBlock(doc, key string) string {
	i := strings.Index(doc, key)
	if i < 0 {
		return ""
	}
	tail := doc[i:]
	depth := 0
	for j, r := range tail {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return tail[:j+1]
			}
		}
	}
	return tail
}

func TestEmit_NoOverlayNoHooks_SkipsSettings(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json should not be written: %v", err)
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
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem"},
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers key: %s", raw)
	}
	if _, ok := servers["fs"]; !ok {
		t.Errorf("expected fs server: %s", raw)
	}
}

func TestEmit_MCPFileOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {MCPFile: "vendor/.mcp.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "x", Meta: map[string]any{"command": "true"}},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/.mcp.json")); err != nil {
		t.Errorf("expected vendor/.mcp.json: %v", err)
	}
}

func TestEmit_WritesCommand(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindCommand,
			Name: "deploy",
			Meta: map[string]any{
				"name":        "deploy",
				"description": "deploy the app",
			},
			Body: "Run the deploy.",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/commands/deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "Run the deploy.") {
		t.Errorf("missing body: %s", s)
	}
	if !strings.Contains(s, "description: deploy the app") {
		t.Errorf("missing frontmatter description: %s", s)
	}
}

func TestEmit_AllOutputsCarryProvenanceHeader(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Meta: map[string]any{"name": "ag1"}, Body: "agent body"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill body"},
		{Kind: spec.KindCommand, Name: "cmd1", Meta: map[string]any{"description": "d"}, Body: "command body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		".claude/rules/r1.md",
		".claude/agents/ag1.md",
		".claude/skills/sk1/SKILL.md",
		".claude/commands/cmd1.md",
	} {
		got, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if !strings.Contains(string(got), "Generated by agnostic-ai") {
			t.Errorf("%s missing provenance header:\n%s", p, got)
		}
		// When the file has frontmatter, the header must live below the
		// closing delimiter so YAML parsing stays valid.
		if strings.HasPrefix(string(got), "---\n") {
			idxClose := strings.Index(string(got), "\n---\n")
			idxHeader := strings.Index(string(got), "<!-- Generated")
			if idxHeader > 0 && idxHeader < idxClose {
				t.Errorf("%s header must follow frontmatter close:\n%s", p, got)
			}
		}
	}
}

func TestEmit_CommandsDirOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {CommandsDir: "vendor/commands"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindCommand, Name: "deploy", Body: "x"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/commands/deploy.md")); err != nil {
		t.Errorf("expected commands at override path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/commands/deploy.md")); !os.IsNotExist(err) {
		t.Errorf("default commands dir should be skipped when override set")
	}
}

func TestEmit_OutputOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {Dir: "vendor/.claude", RulesFile: "vendor/CLAUDE.md"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
		{Kind: spec.KindAgent, Name: "a1", Body: "y"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"vendor/CLAUDE.md", "vendor/.claude/agents/a1.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("expected %s to exist", p)
		}
	}
}
