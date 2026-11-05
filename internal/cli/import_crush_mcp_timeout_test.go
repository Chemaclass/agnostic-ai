package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func crushTimeoutProject(t *testing.T, crushJSON string) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush, junie]\n")
	writeFile(t, filepath.Join(dir, "crush.json"), crushJSON)
	return dir
}

func mcpTimeoutIn(t *testing.T, path, mapKey, server string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc[mapKey][server]["timeout"]
}

// A Crush timeout is seconds; the spec field is milliseconds, so import
// scales it and sync scales it back. Junie reads milliseconds and must
// get 30000, not 30.
func TestImportCrush_MCPTimeoutSecondsRoundTrip(t *testing.T) {
	dir := crushTimeoutProject(t, `{"mcp":{"slow":{"type":"http","url":"https://x.example/mcp","timeout":30}}}`)

	execCLI(t, "import", "crush")
	specYAML := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "slow.yaml"))
	if !strings.Contains(specYAML, "timeout: 30000\n") {
		t.Fatalf("spec should hold the timeout in milliseconds:\n%s", specYAML)
	}

	execCLI(t, "sync")
	if got := mcpTimeoutIn(t, filepath.Join(dir, "crush.json"), "mcp", "slow"); got != float64(30) {
		t.Errorf("crush.json timeout = %v, want 30", got)
	}
	if got := mcpTimeoutIn(t, filepath.Join(dir, ".junie", "mcp", "mcp.json"), "mcpServers", "slow"); got != float64(30000) {
		t.Errorf("junie timeout = %v, want 30000", got)
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import and sync: %v", err)
	}
}

// Crush reads 0 as its default, which no millisecond value means, so the
// spec gets no timeout and every target keeps its own default.
func TestImportCrush_ZeroMCPTimeoutImportsNothing(t *testing.T) {
	dir := crushTimeoutProject(t, `{"mcp":{"slow":{"type":"http","url":"https://x.example/mcp","timeout":0}}}`)

	execCLI(t, "import", "crush")
	specYAML := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "slow.yaml"))
	if strings.Contains(specYAML, "timeout") {
		t.Errorf("a zero Crush timeout should not reach the spec:\n%s", specYAML)
	}
}

// Pi reads seconds too. A Pi import scales to milliseconds, so Crush and
// Pi get the same seconds back and Junie gets milliseconds.
func TestImportPi_MCPTimeoutSecondsRoundTripAcrossTargets(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi, crush, junie]\n")
	writeFile(t, filepath.Join(dir, ".pi", "mcp.json"), `{"mcpServers":{"slow":{"type":"http","url":"https://x.example/mcp","timeout":40}}}`)

	execCLI(t, "import", "pi")
	specYAML := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "slow.yaml"))
	if !strings.Contains(specYAML, "timeout: 40000\n") {
		t.Fatalf("spec should hold the timeout in milliseconds:\n%s", specYAML)
	}

	execCLI(t, "sync")
	for _, c := range []struct {
		path, key string
		want      float64
	}{
		{filepath.Join(dir, ".pi", "mcp.json"), "mcpServers", 40},
		{filepath.Join(dir, "crush.json"), "mcp", 40},
		{filepath.Join(dir, ".junie", "mcp", "mcp.json"), "mcpServers", 40000},
	} {
		if got := mcpTimeoutIn(t, c.path, c.key, "slow"); got != c.want {
			t.Errorf("%s timeout = %v, want %v", c.path, got, c.want)
		}
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import and sync: %v", err)
	}
}

func TestSecondsTimeoutToMilliseconds_RoundsFloatError(t *testing.T) {
	server := map[string]any{"timeout": 2.3}
	secondsTimeoutToMilliseconds(server)
	if server["timeout"] != 2300 {
		t.Errorf("timeout = %v, want 2300", server["timeout"])
	}
}

// Under half a millisecond rounds to 0, which Junie would read as no
// time at all, so the key is dropped.
func TestSecondsTimeoutToMilliseconds_DropsAZeroResult(t *testing.T) {
	server := map[string]any{"timeout": 0.0004}
	secondsTimeoutToMilliseconds(server)
	if _, set := server["timeout"]; set {
		t.Errorf("want no timeout, got %v", server["timeout"])
	}
}
