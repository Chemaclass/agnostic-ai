package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An imported Claude model alias stays with Claude, so Codex falls back
// to its own default model (#1431).
func TestImportClaude_ModelAliasStaysOutOfCodex(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	files := map[string]string{
		"agnostic-ai.yaml":           "version: 1\ntargets: [claude, codex]\n",
		".claude/agents/reviewer.md": "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: sonnet\n---\n\nReview.\n",
	}
	for name, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	runCmd(t, "import", "claude")
	runCmd(t, "sync")
	runCmd(t, "lint", "--strict")
	runCmd(t, "sync", "--check")

	assertContains(t, filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"), "\nmodel: {claude: sonnet}\n")
	assertContains(t, filepath.Join(dir, ".claude/agents/reviewer.md"), "\nmodel: sonnet\n")
	assertAbsent(t, filepath.Join(dir, ".codex/agents/reviewer.toml"), "model")
}

// A hand-written Claude alias shared with Codex fails the sync under
// `on-unsupported: error` (#1431).
func TestSync_SharedClaudeModelFailsUnderOnUnsupportedError(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	files := map[string]string{
		"agnostic-ai.yaml":                "version: 1\ntargets: [claude, codex]\non-unsupported: error\n",
		".agnostic-ai/agents/reviewer.md": "---\nname: reviewer\ndescription: Reviews diffs.\nmodel: sonnet\n---\n\nReview.\n",
	}
	for name, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	runCmdExpectErr(t, "sync")
}
