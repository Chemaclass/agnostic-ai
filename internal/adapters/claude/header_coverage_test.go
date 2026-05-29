package claude

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

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the claude adapter's
// header-coverage contract: every Markdown file the adapter writes
// must carry the agnostic-ai provenance marker. .json outputs
// (settings.json, .mcp.json) legitimately skip the header (JSON has
// no comment syntax). Hook scripts also skip — they are verbatim
// user-authored shell bodies whose shebang must stay on line 1.
//
// The kit-sink bundle covers every supported spec kind so any future
// emit path forgetting WithHeader / HeaderBlock gets caught here.
func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	skipExact := map[string]bool{
		"CLAUDE.md": true, // owned by sync, not this adapter
	}
	jsonExempt := func(p string) bool { return strings.HasSuffix(p, ".json") }
	shellExempt := func(p string) bool { return strings.HasSuffix(p, ".sh") }

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
		if skipExact[rel] {
			return nil
		}
		if strings.HasPrefix(rel, ".agnostic-ai/") {
			return nil
		}
		if jsonExempt(rel) || shellExempt(rel) {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				t.Errorf("expected non-empty exempt output %s", rel)
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

// kitSinkBundle returns a Bundle exercising every kind the claude
// adapter declares in caps.Supports with three specimens per kind.
// Hooks span multiple lifecycle events, MCPs cover stdio + http +
// disabled-with-command.
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body"},
		{Kind: spec.KindAgent, Name: "beta", Path: "agents/beta.md", Body: "beta body"},
		{Kind: spec.KindAgent, Name: "gamma", Path: "agents/gamma.md", Body: "gamma body"},
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "uno skill body"},
		{Kind: spec.KindSkill, Name: "dos", Path: "skills/dos/SKILL.md", Body: "dos skill body"},
		{Kind: spec.KindSkill, Name: "tres", Path: "skills/tres/SKILL.md", Body: "tres skill body"},
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body"},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
		{
			Kind: spec.KindHook, Name: "fmt-go",
			Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit", "command": "gofmt -w"},
		},
		{
			Kind: spec.KindHook, Name: "lint-pre",
			Meta: map[string]any{"event": "PreToolUse", "matcher": "Write", "command": "echo pre"},
		},
		{
			Kind: spec.KindHook, Name: "session-start",
			Meta: map[string]any{"event": "SessionStart", "command": "echo session"},
		},
		{Kind: spec.KindCommand, Name: "cmd-one", Path: "commands/cmd-one.md", Body: "cmd one body"},
		{Kind: spec.KindCommand, Name: "cmd-two", Path: "commands/cmd-two.md", Body: "cmd two body"},
		{Kind: spec.KindCommand, Name: "cmd-three", Path: "commands/cmd-three.md", Body: "cmd three body"},
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
