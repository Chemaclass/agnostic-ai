package qoder

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

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the qoder adapter's
// header-coverage contract: every Markdown file the adapter writes
// must carry the agnostic-ai provenance marker. `.mcp.json` legitimately
// skips the header (JSON has no comment syntax agnostic-ai emits into)
// but the test still asserts the file is non-empty so a regression that
// produces an empty file trips here.
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

// kitSinkBundle returns a Bundle exercising every kind the qoder
// adapter declares in caps.Supports (Rule, Agent, Skill, Command, MCP,
// Hook) with three specimens for rule, agent, skill, and command. The
// MCP specimens are byte-identical to claude's kit-sink MCP entries
// (same names, same Meta) so the two adapters' `.mcp.json` output can
// be diffed directly; see TestEmit_MCP_MatchesClaudeSharedFile in
// qoder_test.go. The one hook specimen sets `matcher: Bash`, Claude
// Code's own tool name and one of qoder's documented PreToolUse
// examples, demonstrating the matcher passes straight through with no
// coverage note (#629). The three command specimens exercise the
// vendor-required `description` fallback to the spec name (cmd-two
// omits it) and the `x-qoder` passthrough escape hatch (cmd-three),
// since docs.qoder.com/cli/commands documents no other native
// frontmatter key (#630).
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body"},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body"},
		{Kind: spec.KindAgent, Name: "beta", Path: "agents/beta.md", Body: "beta body"},
		{Kind: spec.KindAgent, Name: "gamma", Path: "agents/gamma.md", Body: "gamma body"},
		{Kind: spec.KindSkill, Name: "uno", Meta: map[string]any{"description": "Uno skill description."}, Body: "uno skill body"},
		{Kind: spec.KindSkill, Name: "dos", Meta: map[string]any{"description": "Dos skill description."}, Body: "dos skill body"},
		{Kind: spec.KindSkill, Name: "tres", Meta: map[string]any{"description": "Tres skill description."}, Body: "tres skill body"},
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Meta: map[string]any{"description": "cmd one"}, Body: "cmd one body"},
		{Kind: spec.KindCommand, Name: "cmd-two", Path: "commands/cmd-two.md", Body: "cmd two body"},
		{Kind: spec.KindCommand, Name: "cmd-three", Path: "commands/cmd-three.md", Meta: map[string]any{"description": "cmd three", "x-qoder": map[string]any{"tags": "release"}}, Body: "cmd three body"},
		{
			Kind: spec.KindMCP, Name: "stdio-server",
			Meta: map[string]any{
				"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-filesystem"},
				"cwd": "/srv/project", "timeout": 4500, "trust": true,
			},
		},
		{
			Kind: spec.KindMCP, Name: "http-server",
			Meta: map[string]any{
				"type": "http", "url": "https://example.test/mcp",
				"description":  "Remote MCP over HTTP",
				"includeTools": []any{"read_file"}, "excludeTools": []any{"delete_file"},
				"alwaysAllow": []any{"read_file"},
			},
		},
		{
			Kind: spec.KindMCP, Name: "disabled-server",
			Meta: map[string]any{"command": "x", "disabled": true},
		},
		{
			Kind: spec.KindHook, Name: "guard", Path: "hooks/guard.yaml",
			Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "hooks/guard.sh", "timeout": 10},
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
