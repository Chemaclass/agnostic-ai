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

func TestSyncGlobal_ClaudeMCPCreatesPrivateFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	info, err := os.Stat(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestSyncGlobal_DisabledMCPStaysOutOfUserFiles(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP+"disabled: true\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,cursor,copilot")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	for _, rel := range []string{".claude.json", ".cursor/mcp.json", ".copilot/mcp-config.json"} {
		if data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(rel))); err == nil && strings.Contains(string(data), "docs") {
			t.Errorf("%s holds a disabled server:\n%s", rel, data)
		}
	}
	if !strings.Contains(warnings, "disabled") {
		t.Errorf("the dropped server must be noted:\n%s", warnings)
	}
}

func TestSyncGlobal_MCPMovesWithConfigRoot(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	oldPath := filepath.Join(home, ".claude.json")
	mustWriteGlobalTest(t, oldPath, "{\n  \"userID\": \"abc\"\n}\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	dir := filepath.Join(home, "claude-config")
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after move: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, oldPath); got != "{\n  \"userID\": \"abc\"\n}\n" {
		t.Errorf("the old file must lose the server:\n%s", got)
	}
	if got := readGlobalTest(t, filepath.Join(dir, ".claude.json")); !strings.Contains(got, "docs-mcp") {
		t.Errorf("the new file must get it:\n%s", got)
	}
}

func TestApplyGlobalChanges_StopsWhenFileChangedSincePlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	mustWriteGlobalTest(t, path, "{\"a\": 2}\n")
	w := globalWrite{path: path, data: []byte("{\"a\": 1, \"b\": 1}\n"), mode: 0o600, planned: &diskSnapshot{data: []byte("{\"a\": 1}\n")}}
	if _, err := applyGlobalChanges([]globalWrite{w}, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Fatalf("err = %v", err)
	}
	if got := readGlobalTest(t, path); got != "{\"a\": 2}\n" {
		t.Errorf("the tool's write must survive: %q", got)
	}
}

func TestApplyGlobalChanges_BackupKeepsSourceMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	mustWriteGlobalTest(t, path, "{}\n")
	mustWriteGlobalTest(t, path+".bak", "old\n")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	w := globalWrite{path: path, data: []byte("{\"a\": 1}\n"), mode: 0o600}
	if _, err := applyGlobalChanges([]globalWrite{w}, nil, true, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf(".bak mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestSyncGlobal_OpenHandsMCPReachesUserFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mcps := filepath.Join(source, "mcps")
	mustWriteGlobalTest(t, filepath.Join(mcps, "docs.yaml"), globalDocsMCP+"env:\n  DEBUG: \"true\"\n")
	mustWriteGlobalTest(t, filepath.Join(mcps, "notion.yaml"), "name: notion\ntype: http\nurl: https://mcp.notion.com/mcp\nauth: oauth\n")
	if _, w, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	path := filepath.Join(home, ".openhands", "mcp.json")
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	docs, notion := doc["mcpServers"]["docs"], doc["mcpServers"]["notion"]
	if docs["command"] != "docs-mcp" || docs["env"] == nil || docs["type"] != nil {
		t.Errorf("docs = %v", docs)
	}
	if notion["url"] != "https://mcp.notion.com/mcp" || notion["transport"] != "http" || notion["auth"] != "oauth" {
		t.Errorf("notion = %v", notion)
	}
	before := readGlobalTest(t, path)
	if err := os.RemoveAll(mcps); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(source, "state")); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runImportGlobalTest("openhands"); err != nil {
		t.Fatalf("import: %v\n%s", err, w)
	}
	if _, w, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("mcp.json changed after import:\n%s\nwant:\n%s", got, before)
	}
}

// `openhands mcp add` rewrites the whole file with every fastmcp field,
// nulls and `enabled` included; that must not read as a hand edit.
func TestSyncGlobal_OpenHandsAdoptsItsOwnRewrite(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "mcps", "docs.yaml")
	mustWriteGlobalTest(t, spec, globalDocsMCP)
	if _, w, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	path := filepath.Join(home, ".openhands", "mcp.json")
	rewritten := `{
  "mcpServers": {
    "docs": {"args": ["--stdio"], "authentication": null, "command": "docs-mcp", "cwd": null, "description": null, "enabled": true, "env": {}, "icon": null, "keep_alive": null, "timeout": null, "transport": "stdio", "type": null},
    "web": {"url": "https://x.test/sse", "enabled": true, "headers": {}}
  }
}
`
	mustWriteGlobalTest(t, path, rewritten)
	if _, w, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatalf("sync after the CLI rewrite: %v\n%s", err, w)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); strings.Contains(got, "docs-mcp") || !strings.Contains(got, "x.test") {
		t.Errorf("removal must take only the synced server:\n%s", got)
	}
}

func TestSyncGlobal_OpenHandsRemoteKeepsHeadersAndFollowsPersistenceDir(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "api.yaml"), "name: api\ntype: http\nurl: https://api.test/mcp\nheaders:\n  Authorization: Bearer t\nenv:\n  K: v\n")
	dir := filepath.Join(home, "oh")
	t.Setenv("OPENHANDS_PERSISTENCE_DIR", dir)
	if _, w, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(dir, "mcp.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	api := doc["mcpServers"]["api"]
	if headers, _ := api["headers"].(map[string]any); headers["Authorization"] != "Bearer t" || api["env"] != nil {
		t.Errorf("api = %v", api)
	}
}

func TestImportGlobal_OpenHandsCLIShapesRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".openhands", "mcp.json")
	body := `{"mcpServers": {"s": {"url": "https://x.test/sse", "transport": "sse"}, "p": {"command": "py", "args": ["-m", "t"], "transport": "stdio"}, "u": {"url": "https://x.test/mcp"}, "h": {"url": "https://x.test/mcp", "transport": "http", "headers": {"A": "b"}}}}` + "\n"
	mustWriteGlobalTest(t, path, body)
	_, warnings, err := runImportGlobalTest("openhands")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, name := range []string{"s", "p", "u", "h"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Errorf("%s not imported: %v\n%s", name, err, warnings)
		}
	}
	if _, w, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != body {
		t.Errorf("mcp.json changed after import:\n%s", got)
	}
}

func TestSyncGlobal_AugmentUserSettingsGetHooksMCPAndKeys(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "start.yaml"), "event: SessionStart\nmatcher: ignored\ncommand: ~/bin/start.sh\ntimeout: 5\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "edit.yaml"), "event: PostToolUse\nmatcher: str-replace-editor\ncommand: ~/bin/fmt.sh\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "x-augment:\n  shell: zsh\n")
	path := filepath.Join(home, ".augment", "settings.json")
	mustWriteGlobalTest(t, path, "{\n  \"indexing\": true\n}\n")

	_, warnings, err := runGlobalAgentTest("--only", "augment")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	start, _ := hooks["SessionStart"].([]any)
	edit, _ := hooks["PostToolUse"].([]any)
	if len(start) != 1 || len(edit) != 1 {
		t.Fatalf("hooks = %v", hooks)
	}
	startGroup := start[0].(map[string]any)
	if _, has := startGroup["matcher"]; has {
		t.Errorf("a session event takes no matcher: %v", startGroup)
	}
	handler := startGroup["hooks"].([]any)[0].(map[string]any)
	if handler["timeout"] != float64(5000) || handler["command"] != "~/bin/start.sh" {
		t.Errorf("handler = %v", handler)
	}
	if edit[0].(map[string]any)["matcher"] != "str-replace-editor" {
		t.Errorf("tool hook = %v", edit[0])
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers["docs"] == nil || doc["shell"] != "zsh" || doc["indexing"] != true {
		t.Errorf("settings.json = %v", doc)
	}
	if _, _, err := runGlobalAgentTest("--only", "augment", "--check"); err != nil {
		t.Fatalf("check: %v", err)
	}

	for _, dir := range []string{"mcps", "hooks", "settings"} {
		if err := os.RemoveAll(filepath.Join(source, dir)); err != nil {
			t.Fatal(err)
		}
	}
	if _, w, err := runGlobalAgentTest("--only", "augment"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != "{\n  \"indexing\": true\n}\n" {
		t.Errorf("removal must restore settings.json:\n%s", got)
	}
}

func TestSyncGlobal_AugmentAdoptsEmptyMatcherSessionHook(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "start.yaml"), "event: SessionStart\ncommand: ~/s.sh\n")
	path := filepath.Join(home, ".augment", "settings.json")
	existing := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"~/s.sh"}]}]}}` + "\n"
	mustWriteGlobalTest(t, path, existing)
	if _, w, err := runGlobalAgentTest("--only", "augment"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != existing {
		t.Errorf("the hand-written hook must stay as written:\n%s", got)
	}
}

func TestSyncGlobal_WarpMCPPreservesUserServersAndRemovesOwnedEntries(t *testing.T) {
	home, source := globalAgentTestHome(t)
	stdio := filepath.Join(source, "mcps", "docs.yaml")
	remote := filepath.Join(source, "mcps", "api.yaml")
	mustWriteGlobalTest(t, stdio, globalDocsMCP+"cwd: /src\nenv:\n  DEBUG: !literal true\n")
	mustWriteGlobalTest(t, remote, "name: api\ntype: http\nurl: https://api.test/mcp\nheaders:\n  X-Test: !literal example\n")
	path := filepath.Join(home, ".warp", ".mcp.json")
	before := "{\n  \"mcpServers\": {\n    \"mine\": {\"command\": \"mine\", \"args\": []}\n  }\n}\n"
	mustWriteGlobalTest(t, path, before)
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	got := readGlobalTest(t, path)
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatal(err)
	}
	docs, api := doc["mcpServers"]["docs"], doc["mcpServers"]["api"]
	if docs["command"] != "docs-mcp" || docs["working_directory"] != "/src" || docs["args"] == nil || docs["env"] == nil || docs["type"] != nil || docs["cwd"] != nil {
		t.Errorf("stdio = %v", docs)
	}
	if api["url"] != "https://api.test/mcp" || api["headers"] == nil || api["type"] != nil || api["working_directory"] != nil {
		t.Errorf("remote = %v", api)
	}
	if doc["mcpServers"]["mine"]["command"] != "mine" {
		t.Errorf("user server changed: %s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "warp", "--check"); err != nil {
		t.Fatalf("check: %v", err)
	}
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, w)
	}
	if next := readGlobalTest(t, path); next != got {
		t.Errorf("second sync changed output: %s", next)
	}
	for _, p := range []string{stdio, remote} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("remove: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("removal changed user file: %s", got)
	}
}

func TestSyncGlobal_WarpMCPConflictsAndAdoptsEqualServer(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	path := filepath.Join(home, ".warp", ".mcp.json")
	before := `{ "mcpServers": {"docs": {"command": "other", "args": []}} }` + "\n"
	mustWriteGlobalTest(t, path, before)
	if _, _, err := runGlobalAgentTest("--only", "warp"); err == nil || !strings.Contains(err.Error(), "mcpServers.docs") || !strings.Contains(err.Error(), "--backup") {
		t.Fatalf("expected conflict: %v", err)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("conflict changed file: %s", got)
	}
	equal := `{ "mcpServers": {"docs": {"command": "docs-mcp", "args": ["--stdio"]}} }` + "\n"
	mustWriteGlobalTest(t, path, equal)
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("adopt: %v\n%s", err, w)
	} else if !strings.Contains(w, "adopted "+path) {
		t.Errorf("adoption not named: %s", w)
	}
}

func TestSyncGlobal_WarpMCPPreviewAndTargetFilters(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "other.yaml"), "name: other\ncommand: other\ntargets: [codex]\n")
	path := filepath.Join(home, ".warp", ".mcp.json")
	out, w, err := runGlobalAgentTest("--only", "warp", "--dry-run")
	if err != nil {
		t.Fatalf("preview: %v\n%s", err, w)
	}
	if !strings.Contains(out, "dry-run: write "+path) {
		t.Errorf("preview missing file: %s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("preview wrote file: %v", err)
	}
	if _, w, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("cursor sync: %v\n%s", err, w)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("excluded target wrote file: %v", err)
	}
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("warp sync: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); strings.Contains(got, "other") || !strings.Contains(got, "docs-mcp") {
		t.Errorf("target filtering: %s", got)
	}
}

func TestSyncGlobal_WarpDisabledMCPIsNotStartedGlobally(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP+"disabled: true\n")
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	} else if !strings.Contains(w, "disabled") {
		t.Errorf("disabled warning missing: %s", w)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".warp", ".mcp.json")); err == nil && strings.Contains(string(got), "docs") {
		t.Errorf("disabled server emitted: %s", got)
	}
}

func TestImportGlobal_WarpMCPRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".warp", ".mcp.json")
	before := `{ "mcpServers": {"docs": {"command": "docs-mcp", "args": [], "working_directory": "/src"}, "api": {"url": "https://api.test/mcp"}} }` + "\n"
	mustWriteGlobalTest(t, path, before)
	if _, w, err := runImportGlobalTest("warp"); err != nil {
		t.Fatalf("import: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml")); !strings.Contains(got, "cwd: /src") || strings.Contains(got, "working_directory") {
		t.Errorf("stdio import: %s", got)
	}
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("round trip changed file: %s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "warp", "--check"); err != nil {
		t.Fatalf("check: %v", err)
	}
}
