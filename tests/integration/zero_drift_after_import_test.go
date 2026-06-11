package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestZeroDrift_AfterClaudeImportSync seeds a real .claude/ project,
// runs import → sync, then asserts sync --check + doctor both report
// zero drift. Regression guard for #200, #215, and the new overlay
// fallback path in writeSettings (#232). If the overlay encoder or
// the from-disk fallback introduces formatting drift, sync --check
// will fail and doctor --fix becomes a perpetual write loop.
func TestZeroDrift_AfterClaudeImportSync(t *testing.T) {
	dir := t.TempDir()
	seedClaudeNative(t, dir)
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(claudeOnlyConfig), 0o644))

	runCmd(t, "import", "claude")
	runCmd(t, "sync", "-t", "claude")

	// Real test: sync --check must report zero drift immediately after sync.
	runCmd(t, "sync", "--check", "-t", "claude")

	// doctor (no --fix) must succeed too.
	runCmd(t, "doctor", "-t", "claude")

	// doctor --fix should be a no-op: nothing to reconcile, exits clean.
	beforeMtimes := snapshotMtimes(t, filepath.Join(dir, ".claude"))
	runCmd(t, "doctor", "--fix", "-t", "claude")
	afterMtimes := snapshotMtimes(t, filepath.Join(dir, ".claude"))

	for path, before := range beforeMtimes {
		after, ok := afterMtimes[path]
		if !ok {
			t.Errorf("doctor --fix removed %s", path)
			continue
		}
		if before != after {
			t.Errorf("doctor --fix rewrote %s when it should have been a no-op", path)
		}
	}
}

// TestZeroDrift_AfterCodexImportSync exercises the codex overlay
// round-trip end-to-end. A rich .codex/config.toml (model + profiles +
// history + hooks + MCPs) goes through import → sync → sync --check
// and the check must report zero drift. If the overlay re-encode
// differs byte-for-byte from the source (BurntSushi/toml whitespace
// quirks), sync --check fails here.
func TestZeroDrift_AfterCodexImportSync(t *testing.T) {
	dir := t.TempDir()
	seedRichCodexNative(t, dir)
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(codexOnlyConfig), 0o644))

	runCmd(t, "import", "codex")
	runCmd(t, "sync", "-t", "codex")

	runCmd(t, "sync", "--check", "-t", "codex")
	runCmd(t, "doctor", "-t", "codex")

	beforeMtimes := snapshotMtimes(t, filepath.Join(dir, ".codex"))
	runCmd(t, "doctor", "--fix", "-t", "codex")
	afterMtimes := snapshotMtimes(t, filepath.Join(dir, ".codex"))

	for path, before := range beforeMtimes {
		after, ok := afterMtimes[path]
		if !ok {
			t.Errorf("doctor --fix removed %s", path)
			continue
		}
		if before != after {
			t.Errorf("doctor --fix rewrote %s when it should have been a no-op", path)
		}
	}
}

// TestZeroDrift_CodexReimportAfterRulesInlined guards the round-trip
// introduced when codex began inlining rule bodies into AGENTS.md
// (#399). A sync writes the generated rules block; a subsequent
// `import codex` must strip that block instead of re-deriving rules
// from it, so the rules source stays a single file and sync --check
// still reports zero drift.
func TestZeroDrift_CodexReimportAfterRulesInlined(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(codexOnlyConfig), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "conventions.md"),
		[]byte("---\nname: conventions\ndescription: Project conventions.\n---\n\nBe terse.\n"), 0o644))

	runCmd(t, "sync", "-t", "codex")
	// AGENTS.md now carries the generated rules block.
	runCmd(t, "import", "codex")

	// Exactly one rule file survives: no duplicate re-derived from the
	// generated block.
	entries, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected 1 rule after re-import, got %d: %v", len(entries), names)
	}

	runCmd(t, "sync", "--check", "-t", "codex")
}

// TestZeroDrift_AfterClaudeAndCodexImportSync runs import + sync for
// both adapters in the same project, then sync --check. Catches drift
// triggered only when both overlays coexist (e.g. a path collision the
// per-target test wouldn't see).
func TestZeroDrift_AfterClaudeAndCodexImportSync(t *testing.T) {
	dir := t.TempDir()
	seedClaudeNative(t, dir)
	seedRichCodexNative(t, dir)
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(claudeAndCodexConfig), 0o644))

	runCmd(t, "import", "claude")
	runCmd(t, "import", "codex")
	runCmd(t, "sync")

	runCmd(t, "sync", "--check")
}

// seedRichCodexNative writes a .codex/config.toml with every key class
// the overlay capture path has to round-trip: scalars (model, sandbox),
// arrays (notify), nested tables ([history]), table-of-tables
// ([profiles.work], [profiles.oss]), and the spec-derived sections
// (hooks + mcp_servers) that should be stripped from the overlay.
func seedRichCodexNative(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("# Project\n\n## sample-rule\n\nBe terse.\n"), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".codex/config.toml"),
		[]byte(`model = "gpt-5"
sandbox = "workspace-write"
approval_policy = "on-failure"
notify = ["python3", "/etc/codex/notify.py"]

[history]
persistence = "project"

[profiles.work]
model = "gpt-5"
sandbox = "workspace-write"
approval_policy = "on-failure"

[profiles.oss]
model = "gpt-oss-20b"
model_provider = "ollama"

[model_providers.ollama]
name = "Ollama"
base_url = "http://localhost:11434"
wire_api = "openai"

[mcp_servers.fs]
command = "npx"
args = ["server-filesystem"]

[[hooks.PostToolUse]]
matcher = "Edit"
command = "echo edited"
`), 0o644))
}

// snapshotMtimes walks dir and returns a path → mtime map. Used to
// confirm `doctor --fix` is a no-op (every existing file keeps its
// original mtime).
func snapshotMtimes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	must(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		out[path] = info.ModTime().Format("2006-01-02T15:04:05.999999999Z07:00")
		return nil
	}))
	return out
}

const claudeOnlyConfig = `version: 1
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

const codexOnlyConfig = `version: 1
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

const claudeAndCodexConfig = `version: 1
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
