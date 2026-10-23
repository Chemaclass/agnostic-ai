package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestPiRoundTrip_NativeProject(t *testing.T) {
	repository, err := filepath.Abs("../..")
	must(t, err)
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := testutil.TempCwd(t)
	for path, body := range map[string]string{
		"agnostic-ai.yaml":                "version: 1\ntargets: [pi]\n",
		"AGENTS.md":                       "# Project\n\nUse small changes.\n",
		"backend/AGENTS.md":               "Return contextual errors.\n",
		".pi/skills/review/SKILL.md":      "---\nname: review\ndescription: Review patches\n---\n\nRead reference.txt.\n",
		".pi/skills/review/reference.txt": "Check errors.\n",
		".pi/prompts/check.md":            "---\ndescription: Check a file\nargument-hint: <path>\n---\n\nCheck $ARGUMENTS and $1.\n",
		".pi/mcp.json":                    "{\"mcpServers\":{\"local\":{\"command\":\"server\",\"enabled\":false,\"cwd\":\"tools\"},\"remote\":{\"url\":\"https://example.com/mcp\",\"headers\":{\"Authorization\":\"${TOKEN}\"}}}}\n",
		".pi/settings.json":               "{\"defaultModel\":\"model-a\",\"defaultThinkingLevel\":\"high\",\"theme\":\"light\"}\n",
	} {
		must(t, os.MkdirAll(filepath.Dir(path), 0755))
		must(t, os.WriteFile(path, []byte(body), 0644))
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "pi")
	run("sync", "-t", "pi")
	run("sync", "--check", "-t", "pi")
	run("lint")
	for path, want := range map[string]string{"AGENTS.md": "Use small changes.", "backend/AGENTS.md": "Return contextual errors.", ".pi/skills/review/reference.txt": "Check errors.\n", ".pi/prompts/check.md": "Check $ARGUMENTS and $1.", ".pi/mcp.json": "\"enabled\": false", ".pi/settings.json": "\"theme\": \"light\""} {
		data, err := os.ReadFile(path)
		must(t, err)
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing %q: %s", path, want, data)
		}
	}
}
