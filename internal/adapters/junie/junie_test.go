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

func TestEmit_WritesRulesAndAgents(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".junie/rules/r1.md", ".junie/rules/agent-ag1.md", ".junie/rules/skill-sk1.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}

	got, err := os.ReadFile(filepath.Join(dir, ".junie/rules/r1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "rule") {
		t.Errorf("rule file missing body: %s", got)
	}
}

func TestEmit_RulesDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"junie": {RulesDir: "custom/junie-rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/junie-rules/r1.md")); err != nil {
		t.Errorf("override should write under the custom dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".junie/rules/r1.md")); !os.IsNotExist(err) {
		t.Errorf("override must not also write the default dir, err=%v", err)
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
