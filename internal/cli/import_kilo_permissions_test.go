package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Kilo documents `kilo.jsonc` as the shared project config:
// "Permissions are configured under the `permission` key in
// `kilo.jsonc`". A team that wrote one before adopting agnostic-ai
// keeps it, and the reverse of the emit translation is exact: a `*`
// pattern is the bare tool name, and Kilo's trailing ` *` prefix form
// is Claude's `:*` suffix (#890).
func TestImportKilo_ReadsPermissionMapBack(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	writeFile(t, filepath.Join(dir, "kilo.jsonc"), `{
  // a comment, since the vendor documents JSONC here
  "model": "openai/example",
  "permission": {
    "bash": { "*": "ask", "git *": "allow", "rm -rf *": "deny" },
    "read": { "docs/*": "allow" },
    "websearch": "deny",
    "doom_loop": { "*": "ask" },
    "github_list_issues": { "*": "allow" }
  }
}`)

	dst := filepath.Join(dir, ".agnostic-ai", "settings")
	n, err := importKiloPermissions(dir, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected one settings spec, got %d", n)
	}
	got := readFile(t, filepath.Join(dst, "permissions-kilo.yaml"))
	for _, want := range []string{
		"- Bash(git:*)", "- WebSearch", "- Bash", "- Read(docs/*)", "- Bash(rm -rf:*)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Kilo keys with no portable spelling stay where they are rather
	// than being guessed at. `github_list_issues` gives no way to tell
	// where the server name ends and the tool name starts.
	for _, unwanted := range []string{"doom_loop", "github", "mcp__"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected %q in:\n%s", unwanted, got)
		}
	}
}

// No `permission` key means no spec, not an empty one.
func TestImportKilo_NoPermissionKeyWritesNothing(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "kilo.jsonc"), `{"model":"openai/example"}`)

	n, err := importKiloPermissions(dir, filepath.Join(dir, ".agnostic-ai", "settings"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected no settings spec, got %d", n)
	}
}

// A missing `kilo.jsonc` is not an error: import runs against trees
// that never had one.
func TestImportKilo_MissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	n, err := importKiloPermissions(dir, filepath.Join(dir, ".agnostic-ai", "settings"))
	if err != nil {
		t.Fatalf("a missing kilo.jsonc must not fail the import: %v", err)
	}
	if n != 0 {
		t.Errorf("expected no settings spec, got %d", n)
	}
}
