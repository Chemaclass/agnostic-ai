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

func TestKimiRoundTrip_NativeProject(t *testing.T) {
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
		"agnostic-ai.yaml":                       "version: 1\ntargets: [kimi]\noutputs:\n  kimi:\n    conf-file: profile/config.toml\n",
		".kimi-code/AGENTS.md":                   "# Project\n\nUse small changes.\n",
		"backend/.kimi-code/AGENTS.md":           "Return contextual errors.\n",
		".kimi-code/agents/reviewer.md":          "---\nname: reviewer\ndescription: Review patches\ntools: []\n---\nReview changes.\n",
		".kimi-code/skills/review/SKILL.md":      "---\nname: review\ndescription: Review patches\ndisableModelInvocation: true\n---\nRead reference.txt.\n",
		".kimi-code/skills/review/reference.txt": "Check errors.\n",
		".kimi-code/mcp.json":                    "{\"mcpServers\":{\"docs\":{\"transport\":\"sse\",\"url\":\"https://example.com/mcp\",\"enabled\":false,\"enabledTools\":[]}}}\n",
		"profile/config.toml":                    "default_model='existing-model'\n[[hooks]]\nevent='PreToolUse'\ncommand='check'\n",
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
	run("import", "kimi")
	run("sync", "-t", "kimi")
	run("sync", "--check", "-t", "kimi")
	run("lint")
	for path, want := range map[string]string{".kimi-code/AGENTS.md": "Use small changes.", "backend/.kimi-code/AGENTS.md": "Return contextual errors.", ".kimi-code/agents/reviewer.md": "tools: []", ".kimi-code/skills/review/reference.txt": "Check errors.\n", ".kimi-code/skills/review/SKILL.md": "disableModelInvocation: true", ".kimi-code/mcp.json": "\"enabled\": false", "profile/config.toml": `event = "PreToolUse"`} {
		data, err := os.ReadFile(path)
		must(t, err)
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing %q: %s", path, want, data)
		}
	}
}
