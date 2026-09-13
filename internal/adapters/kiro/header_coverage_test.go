package kiro

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

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the kiro adapter's
// header-coverage contract: every Markdown file the adapter writes
// must carry the agnostic-ai provenance marker, and land after the
// frontmatter block Kiro requires as the file's first bytes. JSON
// output (`.kiro/settings/mcp.json`, `.kiro/hooks/<name>.json`) is
// exempt: JSON has no comment syntax to carry the marker in.
// `.kiroignore` still carries the marker, as a `#` comment, but is
// exempt from the frontmatter rule: it is a gitignore-syntax file, not
// one of Kiro's Markdown surfaces.
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
		if rel != defaultIgnoreFile && !strings.HasPrefix(content, "---\n") {
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

// kitSinkBundle returns a Bundle exercising every kind the kiro
// adapter declares in caps.Supports, including command hooks with an
// explicit zero timeout and a native agent action without a command.
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body", Meta: map[string]any{"globs": "**/*.go"}},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body", Meta: map[string]any{"globs": "**/*.ts"}},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body"},
		{Kind: spec.KindAgent, Name: "beta", Path: "agents/beta.md", Body: "beta body"},
		{Kind: spec.KindAgent, Name: "gamma", Path: "agents/gamma.md", Body: "gamma body"},
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "uno skill body", Meta: map[string]any{"description": "handles uno"}},
		{Kind: spec.KindSkill, Name: "dos", Path: "skills/dos/SKILL.md", Body: "dos skill body", Meta: map[string]any{"description": "handles dos"}},
		{Kind: spec.KindSkill, Name: "tres", Path: "skills/tres/SKILL.md", Body: "tres skill body", Meta: map[string]any{"description": "handles tres"}},
		{
			Kind: spec.KindHook, Name: "fmt-go",
			Meta: map[string]any{
				"event": "PostToolUse", "matcher": "Edit", "command": "gofmt -w",
				"description": "Format Go files after an edit.",
			},
		},
		{
			Kind: spec.KindHook, Name: "lint-pre",
			Meta: map[string]any{"event": "PreToolUse", "matcher": "Write", "command": "echo pre"},
		},
		{
			Kind: spec.KindHook, Name: "session-start",
			Meta: map[string]any{"event": "SessionStart", "command": "echo session", "timeout": 0},
		},
		{
			Kind: spec.KindHook, Name: "review-stop",
			Meta: map[string]any{
				"event": "Stop",
				"x-kiro": map[string]any{
					"action": map[string]any{"type": "agent", "prompt": "Check the result."},
				},
			},
		},
		{
			Kind: spec.KindMCP, Name: "stdio-server",
			Meta: map[string]any{
				"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-filesystem"},
				"autoApprove": []any{"read_file"}, "disabledTools": []any{"delete_file"},
			},
		},
		{
			Kind: spec.KindMCP, Name: "http-server",
			Meta: map[string]any{
				"type": "http", "url": "https://example.test/mcp",
				"oauth": map[string]any{
					"clientId":    "client-abc",
					"redirectUri": "http://localhost:7778/oauth/callback",
					"oauthScopes": []any{"files:read"},
				},
			},
		},
		{
			Kind: spec.KindMCP, Name: "disabled-server",
			Meta: map[string]any{"command": "x", "disabled": true},
		},
		{Kind: spec.KindIgnore, Name: "secrets", Path: "ignore/secrets.md", Body: "*.env\nsecrets/"},
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
