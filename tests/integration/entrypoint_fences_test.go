package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A full sync distributes a fenced AGNOSTIC_AI.md so each target's
// entry-point file gets only the paragraphs meant for it, the source
// keeps its markers, and `sync --check` agrees byte-for-byte until a
// hand-edit introduces real drift.
func TestEntryPointFences_SyncClaudeGeminiCodex(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ntargets:\n  - claude\n  - gemini\n  - codex\ngitignore:\n  enabled: false\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/sample.md"),
		[]byte("---\nname: sample\ndescription: A sample rule.\n---\n\nAlways be terse.\n"), 0o644))
	source := "Shared line.\n\n::target claude\nClaude-only line.\n::end\n\n::target gemini\nGemini-only line.\n::end\n\n::target codex\nCodex-only line.\n::end\n"
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/AGNOSTIC_AI.md"), []byte(source), 0o644))

	runCmd(t, "sync")

	assertContains(t, filepath.Join(dir, "CLAUDE.md"), "Shared line.", "Claude-only line.")
	assertAbsent(t, filepath.Join(dir, "CLAUDE.md"), "Gemini-only line.")
	assertAbsent(t, filepath.Join(dir, "CLAUDE.md"), "Codex-only line.")

	assertContains(t, filepath.Join(dir, "GEMINI.md"), "Shared line.", "Gemini-only line.")
	assertAbsent(t, filepath.Join(dir, "GEMINI.md"), "Claude-only line.")
	assertAbsent(t, filepath.Join(dir, "GEMINI.md"), "Codex-only line.")

	assertContains(t, filepath.Join(dir, "AGENTS.md"), "Shared line.", "Codex-only line.")
	assertAbsent(t, filepath.Join(dir, "AGENTS.md"), "Claude-only line.")
	assertAbsent(t, filepath.Join(dir, "AGENTS.md"), "Gemini-only line.")

	for _, name := range []string{"CLAUDE.md", "GEMINI.md", "AGENTS.md"} {
		assertAbsent(t, filepath.Join(dir, name), "::target")
		assertAbsent(t, filepath.Join(dir, name), "::end")
	}

	assertContains(t, filepath.Join(dir, ".agnostic-ai/AGNOSTIC_AI.md"), "::target claude", "::target gemini", "::target codex", "::end")

	runCmd(t, "sync", "--check")

	geminiPath := filepath.Join(dir, "GEMINI.md")
	must(t, os.WriteFile(geminiPath, []byte("hand-edited, no longer matches sync\n"), 0o644))

	runCmdExpectErr(t, "sync", "--check")
}
