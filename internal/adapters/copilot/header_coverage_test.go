package copilot

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the copilot adapter's
// header-coverage contract: every Markdown file the adapter writes
// must carry the agnostic-ai provenance marker. The .vscode/mcp.json
// file is JSON-exempt.
func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"copilot": {
				RulesFile:     ".github/copilot-instructions.md",
				ChatmodesDir:  ".github/chatmodes",
			},
		},
	}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	jsonExempt := func(p string) bool { return strings.HasSuffix(p, ".json") }

	var checked int
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, ".agnostic-ai/") {
			return nil
		}
		if jsonExempt(rel) {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				t.Errorf("expected non-empty JSON output %s", rel)
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !header.Has(string(data)) {
			t.Errorf("missing provenance header in %s:\n%s", rel, headFor(t, data))
			return nil
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked == 0 {
		t.Fatalf("no header-bearing files inspected; kit-sink bundle likely emitted nothing")
	}
}

// kitSinkBundle returns a Bundle exercising every kind the copilot
// adapter declares in caps.Supports (Agent, Skill, Rule, MCP) with
// three specimens per kind. MCPs cover stdio + http + disabled-with-
// command.
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body", Meta: map[string]any{"globs": "**/*.go"}},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body", Meta: map[string]any{"globs": "**/*.ts"}},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body", Meta: map[string]any{"globs": "**/*.py"}},
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body"},
		{Kind: spec.KindAgent, Name: "beta", Path: "agents/beta.md", Body: "beta body"},
		{Kind: spec.KindAgent, Name: "gamma", Path: "agents/gamma.md", Body: "gamma body"},
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "uno skill body"},
		{Kind: spec.KindSkill, Name: "dos", Path: "skills/dos/SKILL.md", Body: "dos skill body"},
		{Kind: spec.KindSkill, Name: "tres", Path: "skills/tres/SKILL.md", Body: "tres skill body"},
		{
			Kind: spec.KindMCP, Name: "stdio-server",
			Meta: map[string]any{"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-filesystem"}},
		},
		{
			Kind: spec.KindMCP, Name: "http-server",
			Meta: map[string]any{"type": "http", "url": "https://example.test/mcp"},
		},
		{
			Kind: spec.KindMCP, Name: "disabled-server",
			Meta: map[string]any{"command": "x"},
		},
	}
	return spec.NewBundle(entries)
}

func headFor(t *testing.T, data []byte) string {
	t.Helper()
	if i := strings.IndexByte(string(data), '\n'); i >= 0 && i < 120 {
		return string(data[:i])
	}
	if len(data) > 120 {
		return string(data[:120]) + "..."
	}
	return string(data)
}
