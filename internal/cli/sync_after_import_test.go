package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// importNestedAgentsMD sets up a codex project whose services/api holds a
// hand-authored AGENTS.md, and imports it into a scoped rule.
func importNestedAgentsMD(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [codex]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join("services", "api", "AGENTS.md")
	writeScopedFile(t, nested, "# API\n\nUse integer minor units.\n")
	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatalf("import codex: %v", err)
	}
	return nested
}

func writeScopedFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSync_AfterImportAdoptsTheNestedAgentsMDItCaptured(t *testing.T) {
	nested := importNestedAgentsMD(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync after import: %v", err)
	}
	got := readFile(t, nested)
	if !header.Has(got) || !strings.Contains(got, "Use integer minor units.") {
		t.Errorf("services/api/AGENTS.md should be the generated copy of the imported text, got:\n%s", got)
	}
}

func TestSync_AfterImportNamesTextAddedSince(t *testing.T) {
	nested := importNestedAgentsMD(t)
	edited := readFile(t, nested) + "\nRound half to even.\n"
	writeScopedFile(t, nested, edited)

	for _, args := range [][]string{{"sync"}, {"sync", "--check"}} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), `"Round half to even."`) || !strings.Contains(err.Error(), "after import") {
			t.Fatalf("%v = %v, want an error naming the text added after import", args, err)
		}
	}
	if got := readFile(t, nested); got != edited {
		t.Fatalf("a refused sync changed the file:\n%s", got)
	}
}
