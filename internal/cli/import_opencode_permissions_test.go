package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The emitted vocabulary reverses exactly: a `*` pattern is the bare
// tool name, OpenCode's trailing ` *` prefix form is Claude's `:*`
// suffix, and `edit` reads back as Edit (#922).
func TestImportOpencode_ReadsPermissionMapBack(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	writeFile(t, filepath.Join(dir, "opencode.json"), `{
  "model": "example/model",
  "permission": {
    "bash": { "*": "ask", "git *": "allow", "rm -rf *": "deny" },
    "read": { "docs/*": "allow" },
    "websearch": "deny",
    "doom_loop": { "*": "ask" },
    "github_list_issues": "deny"
  }
}`)

	dst := filepath.Join(dir, ".agnostic-ai", "settings")
	n, err := importOpencodePermissions(dir, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected one settings spec, got %d", n)
	}
	got := readFile(t, filepath.Join(dst, "permissions-opencode.yaml"))
	for _, want := range []string{
		"- Bash", "- Bash(git:*)", "- Bash(rm -rf:*)", "- Read(docs/*)", "- WebSearch",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// A namespaced MCP key is emitted but never read back: nothing in
	// `github_list_issues` says where the server name ends and the
	// tool name starts, so a guess would invent an `mcp__` rule the
	// author never wrote. Same call kilo's importer makes.
	for _, unwanted := range []string{"doom_loop", "github", "mcp__"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected %q in:\n%s", unwanted, got)
		}
	}
}

// No `permission` key means no spec, not an empty one.
func TestImportOpencode_NoPermissionKeyWritesNothing(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "opencode.json"), `{"model":"example/model"}`)

	n, err := importOpencodePermissions(dir, filepath.Join(dir, ".agnostic-ai", "settings"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected no settings spec, got %d", n)
	}
}

// A missing `opencode.json` is not an error: import runs against trees
// that never had one.
func TestImportOpencode_MissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	n, err := importOpencodePermissions(dir, filepath.Join(dir, ".agnostic-ai", "settings"))
	if err != nil {
		t.Fatalf("a missing opencode.json must not fail the import: %v", err)
	}
	if n != 0 {
		t.Errorf("expected no settings spec, got %d", n)
	}
}
