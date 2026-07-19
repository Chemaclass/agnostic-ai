package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestRoundTrip_ClaudeCodexClaude exercises the full chain:
//
//	claude native -> import claude -> sync -t codex -> wipe specs
//	-> import codex -> sync -t claude
//
// Every kind that both adapters support (agents, skills, rules, hooks,
// MCPs, commands) must survive the chain semantically. The test asserts
// that the final claude native config carries the same names, bodies,
// hook (event, matcher, command), MCP (name, type, command/url), and
// slash-command bodies that the original claude native config did.
//
// This is the canonical 10/10 round-trip check. If anyone regresses an
// import or emit path between claude and codex, this test breaks first.
// TestRoundTrip_CommandPlainAngleBracketsStayPlain regresses the
// `argument-hint: <ver>` drift the audit identified: a plain
// angle-bracket scalar in a source command must stay plain after
// import + sync, instead of being auto-promoted to double-quoted.
func TestRoundTrip_CommandPlainAngleBracketsStayPlain(t *testing.T) {
	dir := t.TempDir()
	source := "---\nargument-hint: <ver>\ndescription: release helper\n---\n\nbody\n"
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Project\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/commands"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/commands/release.md"), []byte(source), 0o644))
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\nsources:\n  agents: .agnostic-ai/agents\n  skills: .agnostic-ai/skills\n  rules: .agnostic-ai/rules\n  hooks: .agnostic-ai/hooks\n  mcps: .agnostic-ai/mcps\n  commands: .agnostic-ai/commands\ntargets:\n  - claude\ngitignore:\n  enabled: false\n"), 0o644))

	runCmd(t, "import", "claude")
	runCmd(t, "sync", "-t", "claude")

	got, err := os.ReadFile(filepath.Join(dir, ".claude/commands/release.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "argument-hint: <ver>\n") {
		t.Errorf("plain `argument-hint: <ver>` was re-quoted after round-trip:\n%s", got)
	}
	if strings.Contains(string(got), `"<ver>"`) {
		t.Errorf("source plain scalar was promoted to double-quoted on emit:\n%s", got)
	}
}

func TestRoundTrip_ClaudeCodexClaude(t *testing.T) {
	dir := t.TempDir()
	seedClaudeNative(t, dir)
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\nsources:\n  agents: .agnostic-ai/agents\n  skills: .agnostic-ai/skills\n  rules: .agnostic-ai/rules\n  hooks: .agnostic-ai/hooks\n  mcps: .agnostic-ai/mcps\n  commands: .agnostic-ai/commands\ntargets:\n  - claude\n  - codex\ngitignore:\n  enabled: false\n"), 0o644))

	runCmd(t, "import", "claude")
	runCmd(t, "sync", "-t", "codex")

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "agents")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "skills")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "rules")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "hooks")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "mcps")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "commands")); err != nil {
		t.Fatal(err)
	}

	runCmd(t, "import", "codex")
	runCmd(t, "sync", "-t", "claude")

	assertContains(t, filepath.Join(dir, ".claude/rules/sample-rule.md"),
		"name: sample-rule", "Be terse.")
	assertContains(t, filepath.Join(dir, ".claude/agents/reviewer.md"),
		"name: reviewer", "Review code carefully.")
	assertContains(t, filepath.Join(dir, ".claude/commands/release.md"),
		"release helper body")
	assertContains(t, filepath.Join(dir, ".mcp.json"),
		`"fs"`, `"command": "npx"`)
	assertContains(t, filepath.Join(dir, ".claude/settings.json"),
		`"PostToolUse"`, `"matcher": "Edit"`, `"command": "echo edited"`)
	assertContains(t, filepath.Join(dir, ".claude/skills/validator/SKILL.md"),
		"name: validator", "Validate yaml files.")
}

// TestRoundTrip_CodexClaudeCodex exercises the inverse chain:
//
//	codex native -> import codex -> sync -t claude -> wipe specs
//	-> import claude -> sync -t codex
//
// The codex overlay must carry every non-managed `.codex/config.toml`
// key through the chain so the final config.toml still has the
// user-authored `model`, `[profiles.*]`, `[history]` blocks.
//
// Hooks imported from codex are auto-tagged `target: codex` (per #249) so
// they intentionally do NOT leak into claude on the intermediate sync.
// That means after wiping specs and re-importing from claude, the hook
// is no longer in the bundle; the round-trip preserves overlay + MCP
// state but not the codex-scoped hook. To exercise hook round-trip the
// other test (TestRoundTrip_ClaudeCodexClaude) covers the symmetric
// claude-scoped path.
func TestRoundTrip_CodexClaudeCodex(t *testing.T) {
	dir := t.TempDir()
	seedCodexNative(t, dir)
	testutil.Chdir(t, dir)

	// commands-dir opts back into the deprecated project prompts layout
	// this chain seeds, so the imported command re-emits to codex.
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\nsources:\n  agents: .agnostic-ai/agents\n  skills: .agnostic-ai/skills\n  rules: .agnostic-ai/rules\n  hooks: .agnostic-ai/hooks\n  mcps: .agnostic-ai/mcps\n  commands: .agnostic-ai/commands\ntargets:\n  - claude\n  - codex\noutputs:\n  codex:\n    commands-dir: .codex/prompts\ngitignore:\n  enabled: false\n"), 0o644))

	runCmd(t, "import", "codex")
	runCmd(t, "sync", "-t", "claude")

	for _, sub := range []string{"agents", "skills", "rules", "hooks", "mcps", "commands"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "claude")
	runCmd(t, "sync", "-t", "codex")

	assertContains(t, filepath.Join(dir, ".codex/prompts/release.md"),
		"release helper body")
	assertContains(t, filepath.Join(dir, ".codex/config.toml"),
		`[mcp_servers.fs]`, `command = "npx"`)
	// Overlay carries first-class codex config keys through the chain.
	assertContains(t, filepath.Join(dir, ".codex/config.toml"),
		`model = "gpt-5"`, `[profiles.work]`)
	// Codex-scoped hook must not have leaked to claude during the
	// intermediate sync, so it is gone after re-import from claude.
	assertAbsent(t, filepath.Join(dir, ".codex/config.toml"),
		`[[hooks.PostToolUse]]`)
}

// assertAbsent fails when path contains substr.
func assertAbsent(t *testing.T, path, substr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.Contains(string(data), substr) {
		t.Errorf("%s unexpectedly contains %q", path, substr)
	}
}

func runCmd(t *testing.T, args ...string) {
	t.Helper()
	root := cli.NewRootCmd("test")
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v failed: %v", args, err)
	}
}

func assertContains(t *testing.T, path string, wants ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s := string(data)
	for _, want := range wants {
		if !strings.Contains(s, want) {
			t.Errorf("%s missing %q\nfull:\n%s", path, want, s)
		}
	}
}

func seedClaudeNative(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte("# Project\n\nClaude project root.\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/rules/sample-rule.md"),
		[]byte("---\nname: sample-rule\ndescription: A sample rule.\n---\n\nBe terse. Avoid filler.\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/agents"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/agents/reviewer.md"),
		[]byte("---\nname: reviewer\ndescription: code reviewer\n---\n\nReview code carefully.\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/skills/validator"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/skills/validator/SKILL.md"),
		[]byte("---\nname: validator\ndescription: validate yaml\n---\n\nValidate yaml files.\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".claude/commands"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/commands/release.md"),
		[]byte("---\ndescription: release helper\n---\n\nrelease helper body\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".claude/settings.json"),
		[]byte(`{
  "hooks": {
    "PostToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "echo edited"}]}
    ]
  }
}
`), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".mcp.json"),
		[]byte(`{
  "mcpServers": {
    "fs": {"command": "npx", "args": ["server-filesystem"]}
  }
}
`), 0o644))
}

func seedCodexNative(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("# Project\n\n## sample-rule\n\nBe terse. Avoid filler.\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agents/agents"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agents/agents/reviewer.toml"),
		[]byte(`name = "reviewer"
description = "code reviewer"
developer_instructions = """
Review code carefully.
"""
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agents/skills/validator"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agents/skills/validator/SKILL.md"),
		[]byte("---\nname: validator\ndescription: validate yaml\n---\n\nValidate yaml files.\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".codex/prompts"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".codex/prompts/release.md"),
		[]byte("---\ndescription: release helper\n---\n\nrelease helper body\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".codex/config.toml"),
		[]byte(`model = "gpt-5"

[profiles.work]
model = "gpt-5"

[mcp_servers.fs]
command = "npx"

[[hooks.PostToolUse]]
matcher = "Edit"
command = "echo edited"
`), 0o644))
}
