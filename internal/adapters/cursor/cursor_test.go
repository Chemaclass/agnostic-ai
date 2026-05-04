package cursor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

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
	if !strings.Contains(string(got), "globs: **/*.go") {
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
