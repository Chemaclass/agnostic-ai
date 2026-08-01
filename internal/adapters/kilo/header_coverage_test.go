package kilo

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

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the kilo adapter's
// header-coverage contract: a sync that exercises every kind this
// adapter emits directly (agents, skills, MCP) must land an
// agnostic-ai provenance header on each resulting non-JSONC file. `kilo.jsonc`
// legitimately skips the header (JSON has no comment syntax agnostic-ai
// emits into) but the test still asserts the file is non-empty so a
// regression that produces an empty file trips here.
func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	jsoncExempt := func(p string) bool { return strings.HasSuffix(p, ".jsonc") }

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
		if jsoncExempt(rel) {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				t.Errorf("expected non-empty JSONC output %s", rel)
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

// kitSinkBundle returns a Bundle exercising every kind the kilo
// adapter declares in caps.Supports (Rule, Agent, Skill, MCP) with
// three specimens per kind. Rules are included even though this
// adapter never writes them itself (see kilo.go). "disabled-server"
// actually sets `disabled: true` (B9, target-audit 2026-08-01
// follow-up: the fixture was named for a server that never carried the
// flag, so the kit sink emitted with no disable state at all before
// and after the #518 schema fix; this fixture now exercises the path
// its name promises). Skill names (uno/dos/tres) match every other
// `.agents/skills/` writer's kit-sink fixture so the golden output can
// diff byte-for-byte against theirs (target-audit 2026-08-01).
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body"},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Meta: map[string]any{"description": "handles alpha"}, Body: "alpha body"},
		{Kind: spec.KindAgent, Name: "beta", Path: "agents/beta.md", Meta: map[string]any{"description": "handles beta", "model": "opus"}, Body: "beta body"},
		{Kind: spec.KindAgent, Name: "gamma", Path: "agents/gamma.md", Meta: map[string]any{"description": "handles gamma", "tools": []any{"Read"}}, Body: "gamma body"},
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
