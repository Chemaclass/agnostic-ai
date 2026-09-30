package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// settings.json merges into what is on disk, so a protected rule must
// leave when its block does, and import must not bring it back (#1497).
func TestProtectedPaths_RemovingTheBlockRemovesTheRulesAndImportSkipsThem(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	spec := filepath.Join(dir, ".agnostic-ai", "settings", "protected.yaml")
	must(t, os.MkdirAll(filepath.Dir(spec), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\n"), 0o644))
	must(t, os.WriteFile(spec, []byte("protected:\n  paths: [composer.lock]\n  decision: deny\n"), 0o644))

	runCmd(t, "sync")
	settings := filepath.Join(dir, ".claude", "settings.json")
	if body := readFile(t, settings); !strings.Contains(body, "Edit(/composer.lock)") {
		t.Fatalf("sync did not write the protected rule:\n%s", body)
	}

	must(t, os.WriteFile(spec, []byte("protected:\n  paths: [composer.lock]\n  decision: ask\n"), 0o644))
	runCmd(t, "sync")
	body := readFile(t, settings)
	if strings.Count(body, "Edit(/composer.lock)") != 1 || !strings.Contains(body, `"ask"`) || strings.Contains(body, `"deny"`) {
		t.Fatalf("deny to ask left the rule in the wrong list:\n%s", body)
	}

	must(t, os.Remove(spec))
	runCmd(t, "sync")
	if body := readFile(t, settings); strings.Contains(body, "Edit(/") {
		t.Fatalf("removing the block kept its rule:\n%s", body)
	}
	runCmd(t, "sync", "--check")

	runCmd(t, "import", "claude")
	for rel, body := range sourceSnapshot(t, filepath.Join(dir, ".agnostic-ai")) {
		if strings.Contains(body, "Edit(/") {
			t.Errorf("import brought a protected rule back in %s:\n%s", rel, body)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
