package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const globalDocsMCP = "name: docs\ncommand: docs-mcp\nargs: [--stdio]\n"

func TestSyncGlobal_MCPServersReachUserFiles(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "mcps", "docs.yaml")
	mustWriteGlobalTest(t, spec, globalDocsMCP)
	codexPath := filepath.Join(home, ".codex", "config.toml")
	codexBefore := "model = \"gpt-6-luna\"\n\n[mcp_servers.mine]\ncommand = \"mine\"\n"
	mustWriteGlobalTest(t, codexPath, codexBefore)
	cursorPath := filepath.Join(home, ".cursor", "mcp.json")
	cursorBefore := "{\n  \"mcpServers\": {\n    \"mine\": {\n      \"command\": \"mine\"\n    }\n  }\n}\n"
	mustWriteGlobalTest(t, cursorPath, cursorBefore)

	_, warnings, err := runGlobalAgentTest("--only", "codex,cursor,gemini,qoder,copilot,claude")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	wantCodex := codexBefore + "\n[mcp_servers.docs]\ncommand = \"docs-mcp\"\nargs = [\"--stdio\"]\n"
	if got := readGlobalTest(t, codexPath); got != wantCodex {
		t.Errorf("config.toml = %q, want %q", got, wantCodex)
	}
	server := func(path string) map[string]any {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		servers, _ := doc["mcpServers"].(map[string]any)
		docs, _ := servers["docs"].(map[string]any)
		return docs
	}
	for _, path := range []string{
		cursorPath,
		filepath.Join(home, ".gemini", "settings.json"),
		filepath.Join(home, ".qoder", "settings.json"),
		filepath.Join(home, ".copilot", "mcp-config.json"),
	} {
		if got := server(path); got["command"] != "docs-mcp" {
			t.Errorf("%s: docs server = %v", path, got)
		}
	}
	if tools := server(filepath.Join(home, ".copilot", "mcp-config.json"))["tools"]; tools == nil {
		t.Errorf("Copilot requires a tools list on every server")
	}
	if got := server(filepath.Join(home, ".claude.json")); got["command"] != "docs-mcp" {
		t.Errorf("claude docs server = %v", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "codex,cursor,gemini,qoder,copilot", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}

	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codex,cursor,gemini,qoder,copilot"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, codexPath); got != codexBefore {
		t.Errorf("removal must restore config.toml:\n%q", got)
	}
	if got := readGlobalTest(t, cursorPath); got != cursorBefore {
		t.Errorf("removal must restore mcp.json:\n%q", got)
	}
}

func TestSyncGlobal_MCPHandWrittenServerConflicts(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	codexPath := filepath.Join(home, ".codex", "config.toml")
	mustWriteGlobalTest(t, codexPath, "[mcp_servers.docs]\ncommand = \"other\"\n")

	_, _, err := runGlobalAgentTest("--only", "codex")
	if err == nil || !strings.Contains(err.Error(), "[mcp_servers.docs]") || !strings.Contains(err.Error(), "--backup") {
		t.Fatalf("a different hand-written server must stop the run: %v", err)
	}
	mustWriteGlobalTest(t, codexPath, "[mcp_servers.docs]\ncommand = \"docs-mcp\"\nargs = [\"--stdio\"]\n")
	_, warnings, err := runGlobalAgentTest("--only", "codex")
	if err != nil {
		t.Fatalf("an equal server must be adopted: %v", err)
	}
	if !strings.Contains(warnings, "adopted "+codexPath+" [mcp_servers.docs]") {
		t.Errorf("adoption must be named:\n%s", warnings)
	}
}

func TestEditTOMLTables_ReplacesSubtablesInPlace(t *testing.T) {
	in := "[mcp_servers.docs]\ncommand = \"old\"\n\n[mcp_servers.docs.env]\nA = \"1\"\n\n[tui]\ntheme = \"dark\"\n"
	got, err := editTOMLTables("c.toml", []byte(in), "mcp_servers", []string{"docs"}, map[string]string{"docs": "[mcp_servers.docs]\ncommand = \"new\"\n"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "[mcp_servers.docs]\ncommand = \"new\"\n\n[tui]\ntheme = \"dark\"\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSyncGlobal_MCPKeepsPrivateFileMode(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	path := filepath.Join(home, ".cursor", "mcp.json")
	mustWriteGlobalTest(t, path, "{}\n")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestSyncGlobal_RemovingSymlinkedFileRemovesItsTarget(t *testing.T) {
	home, source := globalAgentTestHome(t)
	instructions := filepath.Join(source, "AGNOSTIC_AI.md")
	mustWriteGlobalTest(t, instructions, "Be brief.\n")
	real := filepath.Join(home, "dotfiles", "CLAUDE.md")
	mustWriteGlobalTest(t, real, "")
	link := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if err := os.Remove(instructions); err != nil {
		t.Fatal(err)
	}
	_, warnings, err := runGlobalAgentTest("--only", "claude")
	if err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, warnings)
	}
	for _, path := range []string{real, link} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s must be gone: %v", path, err)
		}
	}
	if !strings.Contains(warnings, "removed ") || !strings.Contains(warnings, filepath.Join("dotfiles", "CLAUDE.md")+" and its symlink "+link) {
		t.Errorf("the removal must be named:\n%s", warnings)
	}
}

func TestSyncGlobal_ClaudeMCPKeepsAppState(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "mcps", "docs.yaml")
	mustWriteGlobalTest(t, spec, globalDocsMCP)
	path := filepath.Join(home, ".claude.json")
	before := "{\n  \"userID\": \"abc\",\n  \"projects\": {\n    \"/src\": {\"hasTrustDialogAccepted\": true}\n  },\n  \"mcpServers\": {\n    \"mine\": {\"type\": \"stdio\", \"command\": \"mine\"}\n  }\n}\n"
	mustWriteGlobalTest(t, path, before)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	got := readGlobalTest(t, path)
	if !strings.HasPrefix(got, "{\n  \"userID\": \"abc\",\n  \"projects\": {\n    \"/src\": {\"hasTrustDialogAccepted\": true}\n  },") || !strings.Contains(got, `"docs": {`) {
		t.Errorf(".claude.json:\n%s", got)
	}
	if info, err := os.Stat(path); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("removal must restore .claude.json:\n%s", got)
	}
}

func TestSyncGlobal_ClaudeMCPFollowsConfigDir(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	dir := filepath.Join(home, "claude-config")
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, filepath.Join(dir, ".claude.json")); !strings.Contains(got, "docs-mcp") {
		t.Errorf("config dir .claude.json:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(err) {
		t.Errorf("~/.claude.json must not be written: %v", err)
	}
}
