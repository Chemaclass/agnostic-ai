package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Review specs emit one scope-located BUGBOT.md: unscoped at the repo root,
// scoped under its directory, with same-scope specs concatenated (#433).
func TestEmit_ReviewWritesBugbotPerScope(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindReview, Name: "root", Path: "reviews/root.md", Body: "root rule one"},
		{Kind: spec.KindReview, Name: "root2", Path: "reviews/root2.md", Body: "root rule two"},
		{Kind: spec.KindReview, Name: "be", Path: "reviews/backend/be.md", Scope: "backend", Body: "backend rule"},
	})
	if err := New().Emit(b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	root, err := os.ReadFile(filepath.Join(cwd, "BUGBOT.md"))
	if err != nil {
		t.Fatalf("read root BUGBOT.md: %v", err)
	}
	if !strings.Contains(string(root), "root rule one") || !strings.Contains(string(root), "root rule two") {
		t.Errorf("root BUGBOT.md missing concatenated specs:\n%s", root)
	}
	be, err := os.ReadFile(filepath.Join(cwd, "backend", "BUGBOT.md"))
	if err != nil {
		t.Fatalf("read backend BUGBOT.md: %v", err)
	}
	if !strings.Contains(string(be), "backend rule") {
		t.Errorf("backend BUGBOT.md wrong content:\n%s", be)
	}
	if strings.Contains(string(be), "root rule") {
		t.Errorf("scoped review leaked root content:\n%s", be)
	}
}

// The review output basename is overridable via outputs.cursor.review-file.
func TestEmit_ReviewFileOverride(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindReview, Name: "root", Path: "reviews/root.md", Body: "guidance"},
	})
	cfg := &config.Config{Outputs: map[string]config.Output{"cursor": {ReviewFile: "REVIEW.md"}}}
	if err := New().Emit(b, cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(cwd, "REVIEW.md"))
	if err != nil {
		t.Fatalf("expected REVIEW.md override: %v", err)
	}
	if !strings.Contains(string(got), "guidance") {
		t.Errorf("override file missing body: %s", got)
	}
	if !header.Has(string(got)) {
		t.Errorf("override file missing provenance header: %s", got)
	}
}

// A frontmatter scope that escapes the repo root must not write a file
// outside the project tree (#433 review).
func TestEmit_ReviewScopeEscapeIsBlocked(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindReview, Name: "evil", Path: "reviews/evil.md", Meta: map[string]any{"scope": "../escape"}, Body: "x"},
	})
	if err := New().Emit(b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cwd), "escape", "BUGBOT.md")); err == nil {
		t.Errorf("escaping scope wrote BUGBOT.md outside the repo root")
	}
}

// Environment specs pass through to .cursor/environment.json, merging
// top-level keys and stripping agnostic routing fields (#434).
func TestEmit_EnvironmentWritesCursorJSON(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{
		{
			Kind: spec.KindEnvironment, Name: "default", Path: "environments/default.yaml",
			Meta: map[string]any{"name": "default", "scope": "ignored", "install": "go mod download"},
		},
		{
			Kind: spec.KindEnvironment, Name: "extra", Path: "environments/extra.yaml",
			Meta: map[string]any{"terminals": []any{map[string]any{"name": "dev", "command": "make run"}}},
		},
	})
	if err := New().Emit(b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(cwd, ".cursor", "environment.json"))
	if err != nil {
		t.Fatalf("read environment.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc["install"] != "go mod download" {
		t.Errorf("install passthrough wrong: %v", doc["install"])
	}
	if _, ok := doc["terminals"]; !ok {
		t.Errorf("second spec did not merge: %s", raw)
	}
	if _, leaked := doc["name"]; leaked {
		t.Errorf("routing key name leaked into environment.json: %s", raw)
	}
	if _, leaked := doc["scope"]; leaked {
		t.Errorf("routing key scope leaked into environment.json: %s", raw)
	}
}

// A later environment spec overrides an earlier one on a key collision,
// and the x-cursor namespace resolves (override wins, other targets drop).
func TestEmit_EnvironmentMergeLastWinsAndResolvesMeta(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindEnvironment, Name: "a", Path: "environments/a.yaml", Meta: map[string]any{"install": "first"}},
		{
			Kind: spec.KindEnvironment, Name: "b", Path: "environments/b.yaml",
			Meta: map[string]any{
				"install":  "second",
				"x-cursor": map[string]any{"start": "make run"},
				"x-codex":  map[string]any{"setup": "pip install"},
			},
		},
	})
	if err := New().Emit(b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	doc := map[string]any{}
	raw, _ := os.ReadFile(filepath.Join(cwd, ".cursor", "environment.json"))
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc["install"] != "second" {
		t.Errorf("last-wins violated: install = %v, want second", doc["install"])
	}
	if doc["start"] != "make run" {
		t.Errorf("x-cursor override did not resolve: %s", raw)
	}
	if _, leaked := doc["x-cursor"]; leaked {
		t.Errorf("x-cursor namespace leaked: %s", raw)
	}
	if _, leaked := doc["x-codex"]; leaked {
		t.Errorf("x-codex namespace leaked into cursor output: %s", raw)
	}
	if _, leaked := doc["setup"]; leaked {
		t.Errorf("other target's key leaked: %s", raw)
	}
}

// The environment output path is overridable via outputs.cursor.environment-file.
func TestEmit_EnvironmentFileOverride(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindEnvironment, Name: "default", Path: "environments/default.yaml", Meta: map[string]any{"install": "x"}},
	})
	cfg := &config.Config{Outputs: map[string]config.Output{"cursor": {EnvironmentFile: ".cursor/env.custom.json"}}}
	if err := New().Emit(b, cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(cwd, ".cursor", "env.custom.json")); err != nil {
		t.Errorf("expected env.custom.json override: %v", err)
	}
}

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
