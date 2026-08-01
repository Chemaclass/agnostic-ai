package factory

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the factory adapter's
// header-coverage contract: every Markdown file the adapter writes
// must carry the agnostic-ai provenance marker, landing after the
// frontmatter block as Droid CLI requires. `.factory/mcp.json`
// legitimately skips the header (JSON has no comment syntax agnostic-ai
// emits into) but the test still asserts the file is non-empty so a
// regression that produces an empty file trips here.
func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
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
		content := string(data)
		if !header.Has(content) {
			t.Errorf("missing provenance header in %s:\n%s", rel, headFor(t, data))
		}
		if !strings.HasPrefix(content, "---\n") {
			t.Errorf("%s must start with frontmatter, got:\n%s", rel, headFor(t, data))
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

// kitSinkBundle returns a Bundle exercising every kind the factory
// adapter declares in caps.Supports (Rule, Agent, MCP) with three
// specimens per kind. Rules are included even though this adapter
// never writes them itself (see factory.go). The MCP specimens cover
// stdio, HTTP, and a genuinely `disabled: true` server: Factory's
// schema documents a real `disabled` key, unlike Claude Code, Cursor,
// and Copilot, so this fixture (unlike kilo's pre-B9 one) must
// actually set the flag to exercise the pass-through.
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body"},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Meta: map[string]any{"description": "handles alpha"}, Body: "alpha body"},
		{Kind: spec.KindAgent, Name: "beta", Path: "agents/beta.md", Meta: map[string]any{"description": "handles beta", "model": "opus"}, Body: "beta body"},
		{Kind: spec.KindAgent, Name: "gamma", Path: "agents/gamma.md", Meta: map[string]any{"description": "handles gamma", "tools": []any{"Read"}}, Body: "gamma body"},
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
			Meta: map[string]any{"command": "x", "disabled": true},
		},
	}
	return spec.NewBundle(entries)
}

// headFor returns the first line (or up to 120 bytes) of data for
// human-readable failure output.
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
