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

func TestImportAntigravity_PreviewMatchesImportThroughProjectAlias(t *testing.T) {
	root := resolveImportSourceExisting(t.TempDir())
	alias := filepath.Join(t.TempDir(), "project-alias")
	testutil.DirectoryAlias(t, root, alias)
	testutil.Chdir(t, alias)
	t.Setenv("PWD", alias)
	silence(t)
	mcpPath := filepath.Join(root, ".agents", "plugins", "team", "mcp.json")
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(mcpPath)))
	const mcp = `{"mcpServers":{"docs":{"serverUrl":"https://example.com/mcp"}}}`
	mustWriteFile(t, mcpPath, mcp)
	before := snapshotProject(t, root)
	for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}} {
		output := captureStdout(t, func() {
			if out, err := runRoot(t, append([]string{"import", "antigravity"}, flags...)...); err != nil {
				t.Errorf("flags %v: preview: %v\n%s", flags, err, out)
			}
		})
		if !strings.Contains(filepath.ToSlash(output), ".agnostic-ai/mcps/docs.yaml") || !strings.Contains(output, "dry-run: 1 file(s)") {
			t.Errorf("flags %v: preview missing imported server:\n%s", flags, output)
		}
		assertProjectUnchanged(t, before, snapshotProject(t, root))
	}
	if out, err := runRoot(t, "import", "antigravity"); err != nil {
		t.Fatalf("real import: %v\n%s", err, out)
	}
	if data := readFile(t, filepath.Join(root, ".agnostic-ai", "mcps", "docs.yaml")); !strings.Contains(data, "url: https://example.com/mcp") {
		t.Errorf("real import lost server: %s", data)
	}
	if got := readFile(t, mcpPath); got != mcp {
		t.Errorf("import changed native MCP: %s", got)
	}
}

func TestImportAntigravity_ProjectPathUsesPhysicalRoot(t *testing.T) {
	root := resolveImportSourceExisting(t.TempDir())
	alias := filepath.Join(t.TempDir(), "project-alias")
	testutil.DirectoryAlias(t, root, alias)
	path := filepath.Join(root, ".agents", "plugins", "team", "mcp.json")
	mustWriteFile(t, path, `{"mcpServers":{}}`)
	got, checkRoot, ok := importProjectSourcePath(alias, path)
	if !ok {
		t.Fatal("project file rejected through directory alias")
	}
	if checkRoot != root || got != path {
		t.Errorf("mapped file and root = %q, %q, want %q, %q", got, checkRoot, path, root)
	}
}

func TestImportAntigravity_PreviewRejectsOutsideMCPThroughSourceAlias(t *testing.T) {
	root := resolveImportSourceExisting(t.TempDir())
	testutil.Chdir(t, root)
	silence(t)
	external := resolveImportSourceExisting(t.TempDir())
	mcpPath := filepath.Join(external, "mcp.json")
	mustWriteFile(t, mcpPath, `{"mcpServers":{"outside":{"command":"outside-server"}}}`)
	if err := os.MkdirAll(".agents", 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.DirectoryAlias(t, external, filepath.Join(root, ".agents", "shared"))
	configuration := fmt.Sprintf("version: 1\ntargets: [antigravity]\nsources:\n  rules: %q\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(external), filepath.ToSlash(mcpPath))
	mustWriteFile(t, "agnostic-ai.yaml", configuration)
	outsideBefore := snapshotProject(t, external)
	for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}, nil} {
		output := captureStdout(t, func() {
			if out, err := runRoot(t, append([]string{"import", "antigravity"}, flags...)...); err == nil || !strings.Contains(err.Error(), "outside the project") {
				t.Errorf("flags %v: error = %v, want outside-project refusal\n%s", flags, err, out)
			}
		})
		if strings.Contains(filepath.ToSlash(output), ".agnostic-ai/mcps/outside.yaml") {
			t.Errorf("flags %v: preview proposed outside server:\n%s", flags, output)
		}
		if got := readFile(t, "agnostic-ai.yaml"); got != configuration {
			t.Errorf("flags %v: changed config: %s", flags, got)
		}
		if len(flags) > 0 && exists(".agnostic-ai") || exists(filepath.Join(".agnostic-ai", "mcps", "outside.yaml")) {
			t.Errorf("flags %v: created portable specs", flags)
		}
		if got := resolveImportSourceExisting(filepath.Join(root, ".agents", "shared")); got != external {
			t.Errorf("flags %v: source alias changed to %s", flags, got)
		}
		assertProjectUnchanged(t, outsideBefore, snapshotProject(t, external))
	}
}

func TestImportAntigravity_PreviewKeepsMissingMCPThroughProjectAliasAbsent(t *testing.T) {
	root := resolveImportSourceExisting(t.TempDir())
	alias := filepath.Join(t.TempDir(), "project-alias")
	testutil.DirectoryAlias(t, root, alias)
	testutil.Chdir(t, alias)
	t.Setenv("PWD", alias)
	silence(t)
	mcpPath := filepath.Join(root, ".agents", "plugins", "missing", "mcp.json")
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(mcpPath)))
	before := snapshotProject(t, root)
	for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}} {
		output := captureStdout(t, func() {
			if out, err := runRoot(t, append([]string{"import", "antigravity"}, flags...)...); err != nil {
				t.Errorf("flags %v: preview: %v\n%s", flags, err, out)
			}
		})
		if !strings.Contains(output, "dry-run: 0 file(s)") {
			t.Errorf("flags %v: preview proposed a missing file:\n%s", flags, output)
		}
		assertProjectUnchanged(t, before, snapshotProject(t, root))
	}
}

func TestImportAntigravity_PreviewRejectsMCPAliasIntoGit(t *testing.T) {
	root := resolveImportSourceExisting(t.TempDir())
	testutil.Chdir(t, root)
	silence(t)
	git := filepath.Join(root, ".git", "private")
	mustWriteFile(t, filepath.Join(git, "mcp.json"), `{"mcpServers":{"private":{"command":"private-server"}}}`)
	alias := filepath.Join(root, "plugin-alias")
	testutil.DirectoryAlias(t, git, alias)
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(filepath.Join(alias, "mcp.json"))))
	gitBefore := snapshotProject(t, git)
	for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}, nil} {
		if out, err := runRoot(t, append([]string{"import", "antigravity"}, flags...)...); err == nil || !strings.Contains(err.Error(), "inside .git") {
			t.Errorf("flags %v: error = %v, want .git refusal\n%s", flags, err, out)
		}
		assertProjectUnchanged(t, gitBefore, snapshotProject(t, git))
		if len(flags) > 0 && exists(".agnostic-ai") || exists(filepath.Join(".agnostic-ai", "mcps", "private.yaml")) {
			t.Errorf("flags %v: imported a Git-private MCP", flags)
		}
	}
}
