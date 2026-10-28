package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestOpencodeV2Agent_TemperatureRoundTripsThroughRequestBody(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\nsources:\n  agents: .agnostic-ai/agents\ntargets:\n  - opencode\ngitignore:\n  enabled: false\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"),
		[]byte("---\nname: reviewer\ndescription: Reviews code\ntemperature: 0.2\nx-opencode:\n  disabled: false\n---\n\nReview.\n"), 0o644))

	agentPath := filepath.Join(dir, ".opencode/agents/reviewer.md")
	runCmd(t, "sync", "-t", "opencode")
	first, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "request:\n  body:\n    temperature: 0.2\n") || strings.Contains(string(first), "\ntemperature:") {
		t.Fatalf("V2 agent must carry temperature under request.body only:\n%s", first)
	}

	must(t, os.RemoveAll(filepath.Join(dir, ".agnostic-ai/agents")))
	runCmd(t, "import", "opencode")
	imported, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(imported), "\ntemperature:") {
		t.Errorf("import must keep request.body.temperature, not lift it to top level:\n%s", imported)
	}

	must(t, os.RemoveAll(filepath.Join(dir, ".opencode")))
	runCmd(t, "sync", "-t", "opencode")
	second, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("agent changed across import round trip\n%s", unifiedDiffLines(string(first), string(second)))
	}
}
