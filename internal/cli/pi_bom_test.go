package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const piBOM = "\xef\xbb\xbf"

func TestPiBOM_ProjectImportAndInPlaceSync(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [pi]\n")
	native := filepath.Join(dir, ".pi/mcp.json")
	writeFile(t, native, piBOM+`{"mcpServers":{"tools":`+piOrderedServer+`}}`)
	settings := filepath.Join(dir, ".pi/settings.json")
	writeFile(t, settings, piBOM+`{"defaultModel":"model-a","theme":"light"}`)
	execCLI(t, "import", "pi")
	requirePiOrder(t, readFile(t, filepath.Join(dir, ".agnostic-ai/mcps/tools.yaml")))
	execCLI(t, "sync")
	for _, path := range []string{native, settings} {
		got := readFile(t, path)
		if !strings.HasPrefix(got, piBOM) {
			t.Errorf("%s lost leading BOM: %q", path, got)
		}
	}
	requirePiOrder(t, readFile(t, native))
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
	edited := piBOM + `{"mcpServers":{"tools":` + piReversedServer + `}}`
	writeFile(t, native, edited)
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai/mcps/tools.yaml")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync")
	if got := readFile(t, native); got != edited {
		t.Errorf("BOM ledger release changed a user reorder: %q", got)
	}
}

func TestPiBOM_GlobalImportAndAdoptionKeepOriginalBytes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	native := filepath.Join(home, ".pi/agent/mcp.json")
	before := piBOM + "{\n // keep native formatting\n \"mcpServers\": {\"tools\":" + piOrderedServer + "}\n}\n"
	mustWriteGlobalTest(t, native, before)
	settings := filepath.Join(home, ".pi/agent/settings.json")
	beforeSettings := piBOM + "{\n \"defaultModel\": \"model-a\",\n \"theme\": \"light\"\n}\n"
	mustWriteGlobalTest(t, settings, beforeSettings)
	if _, warn, err := runImportGlobalTest("pi"); err != nil {
		t.Fatalf("import: %v %s", err, warn)
	}
	requirePiOrder(t, readGlobalTest(t, filepath.Join(source, "mcps/tools.yaml")))
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("adopt: %v %s", err, warn)
	}
	if got := readGlobalTest(t, native); got != before {
		t.Errorf("adoption changed MCP bytes: %q", got)
	}
	if got := readGlobalTest(t, settings); got != beforeSettings {
		t.Errorf("adoption changed settings bytes: %q", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestPiBOM_GlobalOwnedUpdateAndRemovalRestoreOriginalBytes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	native := filepath.Join(home, ".pi/agent/mcp.json")
	before := piBOM + "{\r\n // keep this comment\r\n \"mcpServers\": {\r\n  \"mine\": {\"command\": \"mine\"}\r\n }\r\n}\r\n"
	mustWriteGlobalTest(t, native, before)
	if err := os.Chmod(native, 0600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(source, "mcps/tools.yaml")
	mustWriteGlobalTest(t, specPath, piOrderedSpec)
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("add: %v %s", err, warn)
	}
	requirePiOrder(t, readGlobalTest(t, native))
	mustWriteGlobalTest(t, specPath, strings.Replace(piOrderedSpec, "command: tools-mcp", "command: updated-mcp", 1))
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("update: %v %s", err, warn)
	}
	got := readGlobalTest(t, native)
	if !strings.HasPrefix(got, piBOM) || !strings.Contains(got, "updated-mcp") {
		t.Errorf("owned update changed prefix: %q", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "pi", "--check"); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(native)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("private mode changed: %o", info.Mode().Perm())
		}
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, warn, err := runGlobalAgentTest("--only", "pi"); err != nil {
		t.Fatalf("remove: %v %s", err, warn)
	}
	if got := readGlobalTest(t, native); got != before {
		t.Errorf("add-remove did not restore original BOM/comments/bytes: %q", got)
	}
}

func TestPiBOM_GlobalStaleReadStopsBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	native := filepath.Join(dir, "mcp.json")
	before := []byte(piBOM + `{"mcpServers":{"tools":` + piOrderedServer + `}}`)
	changed := append(bytes.Clone(before), ' ')
	mustWriteGlobalTest(t, native, string(changed))
	write := globalWrite{path: native, data: []byte(piBOM + `{}`), mode: 0600, planned: &diskSnapshot{data: before}}
	if _, err := applyGlobalChanges([]globalWrite{write}, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Errorf("stale BOM snapshot did not stop: %v", err)
	}
	if got := readGlobalTest(t, native); got != string(changed) {
		t.Errorf("concurrent edit changed: %q", got)
	}
}
