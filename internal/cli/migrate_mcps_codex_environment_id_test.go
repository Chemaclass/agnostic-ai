package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only `local` maps to an executor ID; `remote` and a spec that sets
// both forms stay as written with a reason the user can act on (#2976).
func TestMigrate_MCPsCodexEnvironmentIDRenamesLocalOnly(t *testing.T) {
	dir := migrationFixture(t, mcpsCodexEnvironmentIDMigration.ID)
	silence(t)
	captureLogOut(t)
	before := map[string]string{}
	for _, name := range []string{"remote-runner", "both"} {
		before[name] = readTestFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", name+".yaml"))
	}

	out, err := runCLI(t, "migrate", "--only", "mcps")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}

	want := "# Keep this comment: migrations must not reformat MCP specs.\n" +
		"name: files\n" +
		"command: npx\n" +
		"args: [\"-y\", \"@modelcontextprotocol/server-filesystem\", \".\"]\n" +
		"environment_id:   \"local\"   # runs on this machine\n" +
		"required: true\n"
	if got := readTestFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "files.yaml")); got != want {
		t.Errorf("files.yaml =\n%s\nwant:\n%s", got, want)
	}
	for name, body := range before {
		if got := readTestFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", name+".yaml")); got != body {
			t.Errorf("%s.yaml changed:\n%s", name, got)
		}
	}
	for _, reason := range []string{"experimental_environment: remote names no executor", "sets both environment_id and experimental_environment"} {
		if !strings.Contains(out, reason) {
			t.Errorf("output misses %q:\n%s", reason, out)
		}
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
