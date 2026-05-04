package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestEmit_WritesRulesIntoClaudeMd(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule one"},
		{Kind: spec.KindRule, Name: "r2", Body: "rule two"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "rule one") || !strings.Contains(string(got), "rule two") {
		t.Errorf("rules missing: %s", got)
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
