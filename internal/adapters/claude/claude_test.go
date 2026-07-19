package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
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

// A custom key declared under x-claude reaches the emitted SKILL.md and
// stays scoped to Claude (other targets drop it). Guards the #367
// passthrough contract on the Claude side, where it already works via
// ResolveMetaOrdered flattening.
func TestEmit_CustomXClaudeKeyReachesSkillFrontmatter(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{
		Kind: spec.KindSkill,
		Name: "validator",
		Body: "skill body",
		Meta: map[string]any{
			"description": "validate stuff",
			"x-claude": map[string]any{
				"disable-model-invocation": true,
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
	if got := string(data); !strings.Contains(got, "disable-model-invocation: true") {
		t.Errorf("SKILL.md missing x-claude custom key in:\n%s", got)
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

// Flat-file skills (`skills/<name>.md`, the default `agnostic-ai new skill`
// scaffold) share one source directory. Each emitted skill folder must hold
// only its own SKILL.md, not a copy of every sibling skill's body (#387).
func TestEmit_FlatFileSkillsDoNotLeakSiblings(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	skillsDir := filepath.Join(dir, ".agnostic-ai", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{"alpha", "beta", "gamma"}
	entries := make([]spec.Entry, 0, len(names))
	for _, name := range names {
		path := filepath.Join(skillsDir, name+".md")
		if err := os.WriteFile(path, []byte("---\nname: "+name+"\n---\nbody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, spec.Entry{
			Kind: spec.KindSkill,
			Name: name,
			Path: path,
			Meta: map[string]any{"name": name},
			Body: "body",
		})
	}

	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, name := range names {
		folder := filepath.Join(dir, ".claude", "skills", name)
		got, err := os.ReadDir(folder)
		if err != nil {
			t.Fatalf("read %s: %v", folder, err)
		}
		var files []string
		for _, e := range got {
			files = append(files, e.Name())
		}
		if len(files) != 1 || files[0] != "SKILL.md" {
			t.Errorf("skill %q folder = %v, want only [SKILL.md]", name, files)
		}
	}
}

// A spec that lists codex-only subtrees under `x-codex.assets` must skip
// those subtrees during claude emit, even if they live in the agnostic
// source dir (#305).
func TestEmit_SkipsCodexOnlySkillAssets(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	srcSkill := filepath.Join(dir, ".agnostic-ai", "skills", "gh-issue")
	if err := os.MkdirAll(filepath.Join(srcSkill, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcSkill, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "SKILL.md"), []byte("---\nname: gh-issue\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "scripts", "gh.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "agents", "openai.yaml"), []byte("interface: cli\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "gh-issue",
			Path: filepath.Join(srcSkill, "SKILL.md"),
			Meta: map[string]any{
				"name": "gh-issue",
				"x-codex": map[string]any{
					"assets": []any{"scripts", "agents"},
				},
			},
			Body: "body",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"scripts", "agents", "scripts/gh.sh", "agents/openai.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "gh-issue", filepath.FromSlash(rel))); err == nil {
			t.Errorf("codex-only asset %q leaked into claude emit", rel)
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

func TestEmit_NestedRuleScopePreservedUnderRulesDir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend/api", Body: "rule"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/rules/backend/api/auth.md")); err != nil {
		t.Errorf("expected nested rule under .claude/rules/backend/api: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/rules/auth.md")); !os.IsNotExist(err) {
		t.Errorf("expected no flattened rule file, err=%v", err)
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

// A cross-tool `globs` scoping field (the Cursor spelling) maps onto
// Claude Code's `paths:` frontmatter so one spec scopes the rule on both
// tools. A spec-authored `paths` wins untouched and `globs` never leaks
// into the Claude emit.
func TestEmit_RuleGlobsMapToPathsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind:     spec.KindRule,
			Name:     "api",
			Meta:     map[string]any{"name": "api", "globs": "src/api/**/*.ts"},
			MetaKeys: []string{"name", "globs"},
			Body:     "api rule\n",
		},
		{
			Kind: spec.KindRule,
			Name: "web",
			Meta: map[string]any{
				"name":  "web",
				"globs": "web/**",
				"paths": []string{"apps/web/**"},
			},
			MetaKeys: []string{"name", "globs", "paths"},
			Body:     "web rule\n",
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	api := readFileT(t, filepath.Join(dir, ".claude/rules/api.md"))
	if !strings.Contains(api, "paths:") || !strings.Contains(api, "src/api/**/*.ts") {
		t.Errorf("globs should map to paths list:\n%s", api)
	}
	if strings.Contains(api, "globs:") {
		t.Errorf("globs must not leak into claude frontmatter:\n%s", api)
	}

	web := readFileT(t, filepath.Join(dir, ".claude/rules/web.md"))
	if !strings.Contains(web, "apps/web/**") {
		t.Errorf("spec-authored paths must win:\n%s", web)
	}
	if strings.Contains(web, "web/**\n") && strings.Contains(web, "globs:") {
		t.Errorf("globs must not leak when paths present:\n%s", web)
	}
}

func readFileT(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
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

// TestWriteSettings_CaptureReadsExistingSettingsWithoutOverlay regresses
// #465. With no import overlay, writeSettings falls back to the on-disk
// .claude/settings.json as the base. Under capture mode (sync --check /
// doctor) that read must still happen so the captured bytes carry the
// user's keys. Otherwise doctor reports false drift and --fix overwrites
// settings.json with the managed keys only, deleting the user's config.
func TestWriteSettings_CaptureReadsExistingSettingsWithoutOverlay(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const existing = `{"theme": "dark"}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event":   "PostToolUse",
			"matcher": "Edit",
			"command": "echo hi",
		}},
	}

	emit.StartCapture()
	err := New().Emit(spec.NewBundle(entries), &config.Config{}, false)
	files := emit.StopCapture()
	if err != nil {
		t.Fatal(err)
	}

	var settings string
	suffix := filepath.Join(".claude", "settings.json")
	for _, f := range files {
		if strings.HasSuffix(f.Path, suffix) {
			settings = f.Content
		}
	}
	if settings == "" {
		t.Fatalf("no settings.json captured: %v", files)
	}
	for _, want := range []string{`"theme"`, `"dark"`, `"hooks"`} {
		if !strings.Contains(settings, want) {
			t.Errorf("captured settings.json missing %q:\n%s", want, settings)
		}
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

// A hook scoped to a different target must not surface in claude's
// settings.json, even though claude supports the kind. Two-hook bundle:
// one scoped to codex, one un-scoped; only the un-scoped hook lands.
func TestEmit_HookTargetScopingFiltersOtherTargets(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "codex-only",
			Meta: map[string]any{
				"event": "PostToolUse", "matcher": "Edit",
				"command": "echo codex", "target": "codex",
			},
		},
		{
			Kind: spec.KindHook, Name: "everywhere",
			Meta: map[string]any{
				"event": "PostToolUse", "matcher": "Edit",
				"command": "echo all",
			},
		},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	out := string(raw)
	if !strings.Contains(out, "echo all") {
		t.Errorf("expected un-scoped hook in claude settings:\n%s", out)
	}
	if strings.Contains(out, "echo codex") {
		t.Errorf("codex-scoped hook should not land in claude settings:\n%s", out)
	}
}

// Per-hook timeout (seconds) and statusMessage are first-class Claude
// schema fields. When the spec carries them via Meta they must propagate
// to every emitted `hooks[].{...}` object so behavior survives a
// round-trip.
func TestEmit_HookTimeoutAndStatusMessagePropagate(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "h1",
			Meta: map[string]any{
				"event":         "PostToolUse",
				"matcher":       "Edit|Write",
				"command":       []any{"a.sh", "b.sh"},
				"timeout":       30,
				"statusMessage": "Running formatters",
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
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type          string `json:"type"`
				Command       string `json:"command"`
				Timeout       int    `json:"timeout"`
				StatusMessage string `json:"statusMessage"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	groups := doc.Hooks["PostToolUse"]
	if len(groups) != 1 || len(groups[0].Hooks) != 2 {
		t.Fatalf("expected 1 matcher group with 2 hooks, got: %s", raw)
	}
	for _, h := range groups[0].Hooks {
		if h.Timeout != 30 {
			t.Errorf("expected timeout=30 on every hook, got %d (%s)", h.Timeout, h.Command)
		}
		if h.StatusMessage != "Running formatters" {
			t.Errorf("expected statusMessage to propagate, got %q (%s)", h.StatusMessage, h.Command)
		}
	}
}

// async, asyncRewake, shell, and if are current command-hook schema
// fields; dropping any of them on emit would strip behavior the user
// authored (a background hook would become blocking, a scoped hook
// would fire on every call).
func TestEmit_HookAsyncShellIfPropagate(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "h1",
			Meta: map[string]any{
				"event":       "PreToolUse",
				"matcher":     "Bash",
				"command":     "check.sh",
				"async":       true,
				"asyncRewake": true,
				"once":        true,
				"shell":       "bash",
				"if":          "Bash(git *)",
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
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Async       bool   `json:"async"`
				AsyncRewake bool   `json:"asyncRewake"`
				Once        bool   `json:"once"`
				Shell       string `json:"shell"`
				If          string `json:"if"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	groups := doc.Hooks["PreToolUse"]
	if len(groups) != 1 || len(groups[0].Hooks) != 1 {
		t.Fatalf("expected 1 group with 1 hook, got: %s", raw)
	}
	h := groups[0].Hooks[0]
	if !h.Async || !h.AsyncRewake || !h.Once || h.Shell != "bash" || h.If != "Bash(git *)" {
		t.Errorf("async/asyncRewake/once/shell/if must propagate, got %+v in:\n%s", h, raw)
	}
}

// A hook spec authored against another tool's hooks directory must
// rewrite the `.<sibling>/hooks/` prefix to `.claude/hooks/` so the
// emitted settings.json points at the path inside the Claude tree.
func TestEmit_Hook_RewritesSiblingHookPathToClaude(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "h1",
			Meta: map[string]any{
				"event":   "PreToolUse",
				"matcher": "Edit",
				"command": ".codex/hooks/protect-files.sh",
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
	if !strings.Contains(string(raw), `".claude/hooks/protect-files.sh"`) {
		t.Errorf("expected sibling codex path rewritten to .claude/hooks/, got:\n%s", raw)
	}
	if strings.Contains(string(raw), ".codex/hooks/") {
		t.Errorf("emitted settings.json still references .codex/hooks/:\n%s", raw)
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
	if idxHooks < 0 || idxStatus <= idxHooks || idxPlugins <= idxStatus {
		t.Errorf("expected hooks < statusLine < enabledPlugins order, got:\n%s", s)
	}
	// Inner keys of statusLine (an overlay-owned block) must preserve
	// source order: type before command.
	statusBlock := extractBlock(s, `"statusLine":`)
	idxType := strings.Index(statusBlock, `"type":`)
	idxCmd := strings.Index(statusBlock, `"command":`)
	if idxType < 0 || idxCmd <= idxType {
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

func TestEmit_SettingsJSONIsByteStable_ComplexOverlay(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash(rm*)",
        "hooks": [
          {"type": "command", "command": "echo blocked"}
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "echo status"
  },
  "enabledPlugins": {"plugin-a": true}
}
`
	if err := os.WriteFile(overlayPath, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "echo edited",
		}},
		{Kind: spec.KindHook, Name: "h2", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Write", "command": "echo wrote",
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit 1: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit 2: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("complex settings.json drifts on second sync.\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestEmit_SettingsJSONIsByteStableNoOverlay(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "echo hi",
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit 1: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit 2: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("settings.json drifts on second sync without overlay.\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestEmit_SettingsJSONIsByteStableAcrossSyncs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{
  "statusLine": {"type": "command", "command": "echo status"}
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
		t.Fatalf("emit 1: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit 2: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("settings.json drifts on second sync (doctor/sync loop).\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestEmit_CapturePreservesOverlay regresses #215. The capture path
// (used by `sync --check` and `doctor`) must read the overlay just
// like a real sync so the captured bytes match the on-disk file.
// Previously `loadSettingsOverlay` short-circuited on emit.IsCapturing()
// and dropped overlay-supplied keys (statusLine, enabledPlugins), causing
// `doctor --fix` to silently delete user data and `doctor` to report
// false drift right after a clean sync.
func TestEmit_CapturePreservesOverlay(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{
  "enabledPlugins": {"plugin-a": true},
  "statusLine": {"type": "command", "command": "echo status"}
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

	// Real sync first, capture disk bytes.
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("sync emit: %v", err)
	}
	disk, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Capture-mode emit, compare against disk.
	emit.StartCapture()
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		emit.StopCapture()
		t.Fatalf("capture emit: %v", err)
	}
	files := emit.StopCapture()
	var captured string
	for _, f := range files {
		if filepath.Base(f.Path) == "settings.json" {
			captured = f.Content
			break
		}
	}
	if captured == "" {
		t.Fatalf("capture produced no settings.json; files=%+v", files)
	}
	if string(disk) != captured {
		t.Errorf("capture diverges from disk\nDISK:\n%s\nCAPTURE:\n%s", disk, captured)
	}
	for _, key := range []string{`"enabledPlugins"`, `"statusLine"`, `"hooks"`} {
		if !strings.Contains(captured, key) {
			t.Errorf("captured output missing %s:\n%s", key, captured)
		}
	}
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

// Remote (http/sse) servers carry an explicit type in .mcp.json; stdio
// stays type-less (it is the inferred default).
func TestEmit_MCP_RemoteCarriesType_StdioDoesNot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://mcp.example.com/mcp"}},
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
	servers := parsed["mcpServers"].(map[string]any)
	remote := servers["remote"].(map[string]any)
	if remote["type"] != "http" {
		t.Errorf("remote server missing type=http: %s", raw)
	}
	local := servers["local"].(map[string]any)
	if _, ok := local["type"]; ok {
		t.Errorf("stdio server should not carry type: %s", raw)
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

// outputs.claude.provenance-header=false must suppress the header in
// the legacy concatenated rules-file layout too, not just per-file
// emits. Regression for #276.
func TestEmit_ProvenanceHeaderToggleOff_SuppressesLegacyRulesFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	off := false
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {RulesFile: "CLAUDE.md", ProvenanceHeader: &off},
	}}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	defer emit.ProvenanceFor(cfg, "claude")()
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if strings.Contains(string(got), "Generated by agnostic-ai") {
		t.Errorf("CLAUDE.md carries header despite toggle off:\n%s", got)
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

// A captured hook-event-order sidecar wins over the canonical lifecycle
// order, so a user who authored PostToolUse before PreToolUse keeps that
// order through a sync. Without the sidecar, events fall back to the
// canonical lifecycle.
func TestEmit_SettingsJSON_HookEventOrderRespectsCapturedSidecar(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai/overlays"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.hook-events.json"),
		[]byte(`["PostToolUse","PreToolUse"]`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "post", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "echo post",
		}},
		{Kind: spec.KindHook, Name: "pre", Meta: map[string]any{
			"event": "PreToolUse", "matcher": "Bash", "command": "echo pre",
		}},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	postPos := strings.Index(out, `"PostToolUse"`)
	prePos := strings.Index(out, `"PreToolUse"`)
	if postPos == -1 || prePos == -1 {
		t.Fatalf("both events expected in:\n%s", out)
	}
	if postPos > prePos {
		t.Errorf("expected PostToolUse before PreToolUse per sidecar; got Post@%d Pre@%d\n%s",
			postPos, prePos, out)
	}
}

// Round-trip a hand-authored 4-space settings.json through emit:
// detected indent applies to the rewritten file. Without the detection
// the file would normalize to 2-space, breaking sync --check byte
// equality for users on 4-space style.
func TestEmit_SettingsJSON_PreservesSourceIndent(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	overlay := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlay), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte(`{
    "statusLine": {
        "type": "command",
        "command": "/bin/echo"
    }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, `    "statusLine"`) {
		t.Errorf("expected 4-space indent preserved in:\n%s", out)
	}
	if strings.Contains(out, `  "statusLine"`) && !strings.Contains(out, `    "statusLine"`) {
		t.Errorf("settings.json normalized to 2-space:\n%s", out)
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

func TestGitignoreHints_ReturnsLocalArtifacts(t *testing.T) {
	got := New().GitignoreHints(&config.Config{})
	want := []string{".claude/agent-memory/", ".claude/settings.local.json"}
	if len(got) != len(want) {
		t.Fatalf("GitignoreHints = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GitignoreHints[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGitignoreHints_HonorsDirOverride(t *testing.T) {
	cfg := &config.Config{Outputs: map[string]config.Output{"claude": {Dir: "vendor/.claude"}}}
	got := New().GitignoreHints(cfg)
	want := []string{"vendor/.claude/agent-memory/", "vendor/.claude/settings.local.json"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GitignoreHints[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
