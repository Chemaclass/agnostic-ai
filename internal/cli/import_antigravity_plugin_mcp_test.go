package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportAntigravity_PluginMCPFileRoundTrips(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: .agents/plugins/team/mcp.json\n")
	mcpPath := filepath.FromSlash(".agents/plugins/team/mcp.json")
	manifestPath := filepath.FromSlash(".agents/plugins/team/plugin.json")
	mcp := "{\n  \"mcpServers\": {\n    \"docs\": {\n      \"serverUrl\": \"https://example.com/mcp\"\n    },\n    \"fs\": {\n      \"args\": [\n        \"--root\",\n        \".\"\n      ],\n      \"command\": \"fs-server\"\n    }\n  }\n}\n"
	manifest := "{\n  \"name\": \"team\",\n  \"version\": \"2.0.0\"\n}\n"
	mustWriteFile(t, mcpPath, mcp)
	mustWriteFile(t, manifestPath, manifest)

	if out, err := runRoot(t, "import", "antigravity"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	for _, name := range []string{"docs", "fs"} {
		if !exists(filepath.Join(".agnostic-ai", "mcps", name+".yaml")) {
			t.Errorf("import skipped plugin MCP server %q", name)
		}
	}
	if exists(filepath.FromSlash(".agents/mcp_config.json")) {
		t.Fatal("import created the standalone MCP file")
	}

	runSyncOK(t)
	if got := readFile(t, mcpPath); got != mcp {
		t.Errorf("sync changed plugin mcp.json:\n%s", got)
	}
	if got := readFile(t, manifestPath); got != manifest {
		t.Errorf("sync changed plugin.json:\n%s", got)
	}
	if _, err := os.Stat(mcpPath + ".bak"); !os.IsNotExist(err) {
		t.Errorf("unexpected mcp.json backup: %v", err)
	}
	runSyncOK(t, "--check")
}

func TestImportAntigravity_AbsolutePluginMCPFileRoundTrips(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	mcpPath := filepath.Join(root, ".agents", "plugins", "team", "mcp.json")
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(mcpPath)))
	mcp := "{\n  \"mcpServers\": {\n    \"docs\": {\n      \"serverUrl\": \"https://example.com/mcp\"\n    }\n  }\n}\n"
	mustWriteFile(t, mcpPath, mcp)

	if out, err := runRoot(t, "import", "antigravity"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(".agnostic-ai", "mcps", "docs.yaml"))
	if err != nil {
		t.Fatalf("read imported docs server: %v", err)
	}
	if !strings.Contains(string(data), "url: https://example.com/mcp") {
		t.Errorf("imported docs server lost its URL:\n%s", data)
	}

	runSyncOK(t)
	if got := readFile(t, mcpPath); got != mcp {
		t.Errorf("sync changed plugin mcp.json:\n%s", got)
	}
	runSyncOK(t, "--check")
}

func TestImportAntigravity_RejectsAbsoluteMCPFileOutsideProject(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	mcpPath := filepath.Join(t.TempDir(), "mcp.json")
	mustWriteFile(t, mcpPath, `{"mcpServers":{"outside":{"command":"outside-server"}}}`)
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(mcpPath)))
	if out, err := runRoot(t, "import", "antigravity"); err == nil || !strings.Contains(err.Error(), "outside the project") {
		t.Errorf("import error = %v, want rejection of path outside the project\n%s", err, out)
	}
	if exists(filepath.Join(".agnostic-ai", "mcps", "outside.yaml")) {
		t.Error("imported MCP server from outside the project")
	}
}

func TestImportAntigravity_DryRunListsAbsolutePluginMCPFile(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	mcpPath := filepath.Join(root, ".agents", "plugins", "team", "mcp.json")
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(mcpPath)))
	mustWriteFile(t, mcpPath, `{"mcpServers":{"docs":{"serverUrl":"https://example.com/mcp"}}}`)
	before := snapshotProject(t, root)

	out := captureStdout(t, func() {
		if out, err := runRoot(t, "import", "antigravity", "--dry-run"); err != nil {
			t.Errorf("import --dry-run: %v\n%s", err, out)
		}
	})
	if want := "would write " + filepath.Join(".agnostic-ai", "mcps", "docs.yaml"); !strings.Contains(out, want) {
		t.Errorf("preview missing %q:\n%s", want, out)
	}
	if !strings.Contains(out, "dry-run: 1 file(s) would be written") {
		t.Errorf("preview missing one-file summary:\n%s", out)
	}
	assertProjectUnchanged(t, before, snapshotProject(t, root))
}

func TestImportAntigravity_DryRunRejectsAbsoluteMCPFileOutsideProject(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	mcpPath := filepath.Join(t.TempDir(), "mcp.json")
	mcp := `{"mcpServers":{"outside":{"command":"outside-server"}}}`
	mustWriteFile(t, mcpPath, mcp)
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(mcpPath)))
	before := snapshotProject(t, root)

	out := captureStdout(t, func() {
		if out, err := runRoot(t, "import", "antigravity", "--dry-run"); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Errorf("import error = %v, want rejection of path outside the project\n%s", err, out)
		}
	})
	if strings.Contains(out, "would write ") || !strings.Contains(out, "dry-run: 0 file(s) would be written") {
		t.Errorf("preview should list no files:\n%s", out)
	}
	assertProjectUnchanged(t, before, snapshotProject(t, root))
	if got := readFile(t, mcpPath); got != mcp {
		t.Errorf("dry-run changed external MCP file:\n%s", got)
	}
}

func TestImportAntigravity_RejectsRelativeMCPFileOutsideProject(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: ../mcp.json\n")
	if out, err := runRoot(t, "import", "antigravity"); err == nil || !strings.Contains(err.Error(), "outputs.antigravity.mcp-file") {
		t.Errorf("import error = %v, want rejection naming outputs.antigravity.mcp-file\n%s", err, out)
	}
}
