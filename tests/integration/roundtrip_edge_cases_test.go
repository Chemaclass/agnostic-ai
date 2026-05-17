package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEdgeCase_ImportClaudeTwiceOverwritesOverlay regresses the
// "second import should not double-stomp" scenario. After two
// back-to-back `import claude` runs the captured overlay must
// reflect the *current* on-disk settings.json snapshot, not append
// or merge old + new values.
func TestEdgeCase_ImportClaudeTwiceOverwritesOverlay(t *testing.T) {
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/settings.json"),
		[]byte(`{"statusLine": {"type": "command", "command": "first"}}`+"\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# P\n"), 0o644))
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(edgeCaseClaudeOnlyConfig), 0o644))

	runCmd(t, "import", "claude")

	overlay := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	first, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatalf("first overlay missing: %v", err)
	}
	if !strings.Contains(string(first), `"first"`) {
		t.Fatalf("first overlay missing original command:\n%s", first)
	}

	// Mutate native settings.json then re-import. Overlay must reflect
	// the new value with the old value gone.
	must(t, os.WriteFile(filepath.Join(dir, ".claude/settings.json"),
		[]byte(`{"statusLine": {"type": "command", "command": "second"}}`+"\n"), 0o644))

	runCmd(t, "import", "claude")

	second, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatalf("second overlay missing: %v", err)
	}
	if !strings.Contains(string(second), `"second"`) {
		t.Errorf("second overlay missing new command:\n%s", second)
	}
	if strings.Contains(string(second), `"first"`) {
		t.Errorf("second overlay still carries stale first-import command:\n%s", second)
	}
}

// TestEdgeCase_EmptyCodexConfigSkipsOverlayAndOutput exercises the
// "import codex over an empty config.toml" scenario. The overlay file
// must not be created (no surprise empty file under .agnostic-ai/),
// and a subsequent `sync -t codex` with no hook/MCP specs must not
// emit `.codex/config.toml` either.
func TestEdgeCase_EmptyCodexConfigSkipsOverlayAndOutput(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# P\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
	// Header-only / whitespace-only config.toml: nothing for the overlay
	// to capture.
	must(t, os.WriteFile(filepath.Join(dir, ".codex/config.toml"),
		[]byte("\n   \n"), 0o644))
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(edgeCaseCodexOnlyConfig), 0o644))

	runCmd(t, "import", "codex")

	overlay := filepath.Join(dir, ".agnostic-ai/overlays/codex.config.toml")
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Errorf("expected no overlay file for empty config.toml, got err=%v", err)
	}

	// Wipe codex tree and sync.
	must(t, os.RemoveAll(filepath.Join(dir, ".codex")))
	runCmd(t, "sync", "-t", "codex")

	out := filepath.Join(dir, ".codex/config.toml")
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("expected no .codex/config.toml emitted for empty specs/overlay, got err=%v", err)
	}
}

// TestEdgeCase_CodexOverlayWinsOverFirstClassConfig confirms the
// documented precedence: when both the overlay and outputs.codex.config
// declare the same key, the overlay wins on conflict and the
// first-class key is dropped. No TOML duplicate-key error.
func TestEdgeCase_CodexOverlayWinsOverFirstClassConfig(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# P\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/overlays"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/overlays/codex.config.toml"),
		[]byte(`model = "overlay-model"
sandbox = "overlay-sandbox"
`), 0o644))
	testutil.Chdir(t, dir)
	// outputs.codex.config declares the same keys with different values.
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - codex
outputs:
  codex:
    config:
      model: config-model
      sandbox: config-sandbox
gitignore:
  enabled: false
`), 0o644))

	runCmd(t, "sync", "-t", "codex")

	got, err := os.ReadFile(filepath.Join(dir, ".codex/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, `model = "overlay-model"`) {
		t.Errorf("overlay model should win; got:\n%s", body)
	}
	if !strings.Contains(body, `sandbox = "overlay-sandbox"`) {
		t.Errorf("overlay sandbox should win; got:\n%s", body)
	}
	if strings.Contains(body, `model = "config-model"`) {
		t.Errorf("first-class config-model should have been dropped; got:\n%s", body)
	}
	if strings.Contains(body, `sandbox = "config-sandbox"`) {
		t.Errorf("first-class config-sandbox should have been dropped; got:\n%s", body)
	}
}

// TestEdgeCase_MCPWithEnvAndHttpRoundTrip puts a stdio MCP server with
// `env` and a separate http MCP server (with `headers`) through claude
// → codex → claude and asserts both keys survive. The chain coverage
// in chain_roundtrip_test.go only exercised the stdio/command/args
// trio, so http + env was an untested code path.
func TestEdgeCase_MCPWithEnvAndHttpRoundTrip(t *testing.T) {
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# P\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".mcp.json"),
		[]byte(`{
  "mcpServers": {
    "fs": {
      "command": "npx",
      "args": ["server-filesystem"],
      "env": {"NODE_OPTIONS": "--max-old-space-size=4096"}
    },
    "remote": {
      "type": "http",
      "url": "https://mcp.example.com",
      "headers": {"Authorization": "Bearer ${TOKEN}"}
    }
  }
}
`), 0o644))
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(edgeCaseClaudeAndCodexConfig), 0o644))

	runCmd(t, "import", "claude")
	runCmd(t, "sync", "-t", "codex")

	codexCfg, err := os.ReadFile(filepath.Join(dir, ".codex/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(codexCfg)
	if !strings.Contains(body, `[mcp_servers.fs]`) {
		t.Errorf("codex config missing [mcp_servers.fs]:\n%s", body)
	}
	if !strings.Contains(body, "NODE_OPTIONS") {
		t.Errorf("codex config dropped stdio env on round-trip:\n%s", body)
	}
	if !strings.Contains(body, `[mcp_servers.remote]`) {
		t.Errorf("codex config missing [mcp_servers.remote]:\n%s", body)
	}
	if !strings.Contains(body, "https://mcp.example.com") {
		t.Errorf("codex config missing remote url:\n%s", body)
	}

	// Wipe specs, re-import from codex, and re-sync to claude. Both servers
	// must reappear with their env / headers.
	for _, sub := range []string{"mcps"} {
		must(t, os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)))
	}
	runCmd(t, "import", "codex")
	must(t, os.Remove(filepath.Join(dir, ".mcp.json")))
	runCmd(t, "sync", "-t", "claude")

	claudeMCP, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	mcpBody := string(claudeMCP)
	for _, want := range []string{
		`"fs"`, `"NODE_OPTIONS"`, `"remote"`, "https://mcp.example.com",
	} {
		if !strings.Contains(mcpBody, want) {
			t.Errorf("round-tripped .mcp.json missing %q:\n%s", want, mcpBody)
		}
	}
}

// TestEdgeCase_FoldedAndLiteralFrontmatterScalars exercises YAML's
// folded (>) and literal (|) block scalar styles in source frontmatter.
// They must round-trip through import → sync without being rewritten
// as plain or double-quoted strings.
func TestEdgeCase_FoldedAndLiteralFrontmatterScalars(t *testing.T) {
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# P\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/rules/folded.md"),
		[]byte(`---
name: folded
description: >
  one
  two
---

body
`), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/rules/literal.md"),
		[]byte(`---
name: literal
description: |
  line1
  line2
---

body
`), 0o644))
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(edgeCaseClaudeOnlyConfig), 0o644))

	runCmd(t, "import", "claude")
	runCmd(t, "sync", "-t", "claude")

	folded, err := os.ReadFile(filepath.Join(dir, ".claude/rules/folded.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(folded), "description: >") {
		t.Errorf("folded (>) scalar lost its style:\n%s", folded)
	}

	literal, err := os.ReadFile(filepath.Join(dir, ".claude/rules/literal.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(literal), "description: |") {
		t.Errorf("literal (|) scalar lost its style:\n%s", literal)
	}
}

// TestEdgeCase_SkillNestedAssetsRoundTrip puts a skill with nested
// assets (executable script, agents/openai.yaml, fixtures subdir)
// through claude → codex → claude and asserts every asset survives
// with the exec bit intact. Catches regressions in copyDirTree mode
// preservation and the codex skill emit path.
//
// `shared-subagents: true` is set explicitly so codex emits skills to
// `.agents/skills/` even with claude present (the default
// shared-subagents=false in mixed configs skips the codex skill tree
// to avoid duplicating `.claude/skills/`).
func TestEdgeCase_SkillNestedAssetsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/skills/runner/scripts"), 0o755))
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/skills/runner/fixtures"), 0o755))
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/skills/runner/agents"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# P\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/skills/runner/SKILL.md"),
		[]byte("---\nname: runner\ndescription: runs things\n---\n\nrun.\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/skills/runner/scripts/run.sh"),
		[]byte("#!/bin/sh\necho hello\n"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/skills/runner/agents/openai.yaml"),
		[]byte("model: gpt-5\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/skills/runner/fixtures/sample.json"),
		[]byte(`{"k": "v"}`+"\n"), 0o644))
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(edgeCaseClaudeAndCodexSharedConfig), 0o644))

	runCmd(t, "import", "claude")
	runCmd(t, "sync", "-t", "codex")

	// Verify codex received the assets including the exec bit (Unix only;
	// Windows filesystems do not surface POSIX exec bits via os.FileMode).
	codexScript := filepath.Join(dir, ".agents/skills/runner/scripts/run.sh")
	info, err := os.Stat(codexScript)
	if err != nil {
		t.Fatalf("codex skill script missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("codex skill script lost exec bit: mode=%v", info.Mode())
	}
	for _, asset := range []string{
		".agents/skills/runner/agents/openai.yaml",
		".agents/skills/runner/fixtures/sample.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, asset)); err != nil {
			t.Errorf("codex skill asset missing: %s (%v)", asset, err)
		}
	}

	// Wipe specs, re-import from codex, sync back to claude.
	must(t, os.RemoveAll(filepath.Join(dir, ".agnostic-ai/skills")))
	must(t, os.RemoveAll(filepath.Join(dir, ".claude/skills")))
	runCmd(t, "import", "codex")
	runCmd(t, "sync", "-t", "claude")

	claudeScript := filepath.Join(dir, ".claude/skills/runner/scripts/run.sh")
	info, err = os.Stat(claudeScript)
	if err != nil {
		t.Fatalf("claude skill script missing after round-trip: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("claude skill script lost exec bit after round-trip: mode=%v", info.Mode())
	}
	for _, asset := range []string{
		".claude/skills/runner/agents/openai.yaml",
		".claude/skills/runner/fixtures/sample.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, asset)); err != nil {
			t.Errorf("claude skill asset missing after round-trip: %s (%v)", asset, err)
		}
	}
}

const edgeCaseClaudeOnlyConfig = `version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - claude
gitignore:
  enabled: false
`

const edgeCaseCodexOnlyConfig = `version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - codex
gitignore:
  enabled: false
`

const edgeCaseClaudeAndCodexConfig = `version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - claude
  - codex
gitignore:
  enabled: false
`

const edgeCaseClaudeAndCodexSharedConfig = `version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - claude
  - codex
outputs:
  codex:
    shared-subagents: true
gitignore:
  enabled: false
`
