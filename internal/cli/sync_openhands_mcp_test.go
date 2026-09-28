package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Current OpenHands releases ignore a project config.toml [mcp] section
// (#1252). Sync writes none, removes the one an earlier release wrote,
// and prints one note pointing at sync --global (#1259).
func TestSync_OpenhandsMCPWritesNoConfigTOML(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [openhands]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"), "name: fs\ncommand: npx\n")
	mustWriteFile(t, filepath.Join(dir, "config.toml"),
		header.With("[mcp]\n\n[[mcp.stdio_servers]]\nname = \"fs\"\ncommand = \"npx\"\n", header.FormatTOML))
	if err := writeStateFile(dir, 1, "", "", syncLedger{outputs: []string{"config.toml"}}); err != nil {
		t.Fatal(err)
	}
	silence(t)
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "config.toml")); !os.IsNotExist(err) {
		t.Errorf("sync left config.toml in place: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(notes.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "agnostic-ai sync --global") {
		t.Errorf("want one note pointing at sync --global, got:\n%s", notes.String())
	}
}
