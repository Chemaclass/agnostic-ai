package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func runImportUserTest(args ...string) (string, string, error) {
	var out, warnings bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&warnings)
	cmd.SetArgs(append([]string{"import", "--scope", "user"}, args...))
	err := cmd.Execute()
	return out.String(), warnings.String(), err
}

func TestImportUser_CodexConfigRoundTrips(t *testing.T) {
	home, source := userAgentTestHome(t)
	codexPath := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, codexPath, "model = \"gpt-6-luna\"\nmodel_reasoning_effort = \"high\"\n\n[mcp_servers.docs]\ncommand = \"docs-mcp\"\nargs = [\"--stdio\"]\n")

	if _, warnings, err := runImportUserTest("codex"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	settings := readFile(t, filepath.Join(source, "settings", "imported.yaml"))
	if !strings.Contains(settings, "codex: gpt-6-luna") || !strings.Contains(settings, "codex: high") {
		t.Errorf("settings spec:\n%s", settings)
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "docs.yaml")); err != nil {
		t.Fatalf("docs MCP spec: %v", err)
	}
	before := readFile(t, codexPath)
	if _, warnings, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	if got := readFile(t, codexPath); got != before {
		t.Errorf("sync after import changed config.toml:\n%s", got)
	}
	if _, _, err := runUserAgentTest("--only", "codex", "--check"); err != nil {
		t.Fatalf("check: %v", err)
	}
}

// Codex has no MCP description or roots key (#2999). Import leaves them
// out of the spec and still keeps the server, and the next sync adopts
// the hand-written table without touching config.toml.
func TestImportUser_CodexDescribedServerRoundTrips(t *testing.T) {
	home, source := userAgentTestHome(t)
	codexPath := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, codexPath, "[mcp_servers.docs]\ncommand = \"docs-mcp\"\nargs = [\"--stdio\"]\ndescription = \"Docs search\"\nroots = [{ uri = \"file:///workspace\", name = \"workspace\" }]\n")

	_, warnings, err := runImportUserTest("codex")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if !strings.Contains(warnings, "codex mcp docs: description and roots have no effect on Codex") {
		t.Errorf("import must say it left the keys out:\n%s", warnings)
	}
	got := readFile(t, filepath.Join(source, "mcps", "docs.yaml"))
	for _, unwanted := range []string{"description", "roots", "file:///workspace"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("docs.yaml must leave out %q:\n%s", unwanted, got)
		}
	}
	before := readFile(t, codexPath)
	if _, warnings, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	if got := readFile(t, codexPath); got != before {
		t.Errorf("sync after import changed config.toml:\n%s", got)
	}
	if _, _, err := runUserAgentTest("--only", "codex", "--check"); err != nil {
		t.Fatalf("check: %v", err)
	}
}

// Claude keeps an MCP description and Codex has none, so the same
// server with the same description in both homes is one server (#2999).
func TestImportUser_ClaudeAndCodexAgreeDespiteDescription(t *testing.T) {
	home, _ := userAgentTestHome(t)
	writeFile(t, filepath.Join(home, ".claude.json"), "{\"mcpServers\": {\"fs\": {\"command\": \"fs-mcp\", \"description\": \"Files\"}}}\n")
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.fs]\ncommand = \"fs-mcp\"\ndescription = \"Files\"\n")

	_, warnings, err := runImportUserTest("claude", "codex")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if strings.Contains(warnings, "defines fs differently") {
		t.Errorf("a description Codex cannot hold is no difference:\n%s", warnings)
	}
}

// When Codex is read first, a later tool's copy of the same server keeps
// the description Codex cannot hold, so the spec loses nothing and sync
// writes every home back unchanged (#2999).
func TestImportUser_LaterToolKeepsDescriptionCodexDrops(t *testing.T) {
	for _, tc := range []struct {
		other, rel, native string
	}{
		{"cursor", ".cursor/mcp.json", "{\n  \"mcpServers\": {\n    \"fs\": {\n      \"command\": \"fs-mcp\",\n      \"description\": \"Files\"\n    }\n  }\n}\n"},
		{"claude", ".claude.json", "{\"mcpServers\": {\"fs\": {\"command\": \"fs-mcp\", \"description\": \"Files\"}}}\n"},
	} {
		t.Run(tc.other, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			otherPath := filepath.Join(home, filepath.FromSlash(tc.rel))
			writeFile(t, otherPath, tc.native)
			codexPath := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, codexPath, "[mcp_servers.fs]\ncommand = \"fs-mcp\"\ndescription = \"Files\"\n")

			if _, warnings, err := runImportUserTest("codex", tc.other); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got := readFile(t, filepath.Join(source, "mcps", "fs.yaml")); !strings.Contains(got, "description: Files") {
				t.Errorf("fs.yaml lost the %s description:\n%s", tc.other, got)
			}
			codexBefore, otherBefore := readFile(t, codexPath), readFile(t, otherPath)
			if _, warnings, err := runUserAgentTest("--only", "codex,"+tc.other); err != nil {
				t.Fatalf("sync after import: %v\n%s", err, warnings)
			}
			if got := readFile(t, codexPath); got != codexBefore {
				t.Errorf("sync changed config.toml:\n%s", got)
			}
			if got := readFile(t, otherPath); got != otherBefore {
				t.Errorf("sync changed %s:\n%s", tc.rel, got)
			}
		})
	}
}

// Every JSON user file sync writes reads back through import into specs
// that render to the same bytes.
func TestImportUser_RoundTripsWhatSyncWrote(t *testing.T) {
	home, source := userAgentTestHome(t)
	targets := "claude,codex,copilot,cursor,gemini,qoder"
	settings := filepath.Join(source, "settings", "d.yaml")
	writeFile(t, settings, `model:
  claude: opus
  codex: gpt-6-luna
  copilot: gpt-6-sol
  gemini: gemini-3-pro
  qoder: qoder-max
effort:
  claude: high
  copilot: xhigh
  qoder: max
`)
	mcpDir := filepath.Join(source, "mcps")
	writeFile(t, filepath.Join(mcpDir, "docs.yaml"), "name: docs\ncommand: docs-mcp\nargs: [--stdio]\nenv:\n  TOKEN: ${DOCS_TOKEN}\n")
	writeFile(t, filepath.Join(mcpDir, "web.yaml"), "name: web\ntype: http\nurl: https://example.com/mcp\n")
	if _, warnings, err := runUserAgentTest("--only", targets); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	snapshot := map[string]string{}
	for _, rel := range []string{".claude.json", ".claude/settings.json", ".codex/config.toml", ".copilot/settings.json", ".copilot/mcp-config.json", ".cursor/mcp.json", ".gemini/settings.json", ".qoder/settings.json"} {
		snapshot[rel] = readFile(t, filepath.Join(home, filepath.FromSlash(rel)))
	}
	for _, path := range []string{settings, mcpDir, filepath.Join(source, "state")} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}

	if _, warnings, err := runImportUserTest(); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	_, warnings, err := runUserAgentTest("--only", targets)
	if err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	for rel, want := range snapshot {
		if got := readFile(t, filepath.Join(home, filepath.FromSlash(rel))); got != want {
			t.Errorf("%s changed after import:\n%s\nwant:\n%s", rel, got, want)
		}
	}
}

func TestImportUser_NeverReplacesASpec(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "model = \"gpt-6-luna\"\n")
	existing := filepath.Join(source, "settings", "imported.yaml")
	writeFile(t, existing, "effort:\n  claude: low\n")

	_, warnings, err := runImportUserTest("codex")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := readFile(t, existing); got != "effort:\n  claude: low\n" {
		t.Errorf("an existing spec must stay: %q", got)
	}
	if !strings.Contains(warnings, "skipped "+existing) {
		t.Errorf("the skip must be named:\n%s", warnings)
	}
	out, _, err := runImportUserTest("codex", "--dry-run")
	if err != nil || strings.Contains(out, "dry-run: write") {
		t.Errorf("nothing new to write: %v\n%s", err, out)
	}
	if _, _, err := runImportUserTest("aider"); err == nil {
		t.Error("a target without user settings or MCP must fail")
	}
}

func TestImportUser_LeavesWhatTheHomeProvides(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "local", "mcps", "secret.yaml"), "name: secret\ncommand: s\nenv:\n  TOKEN: sk-live-123\n")
	writeFile(t, filepath.Join(source, "local", "settings", "me.yaml"), "model:\n  codex: private\n")
	writeFile(t, filepath.Join(source, "mcps", "documentation.yaml"), "name: docs\ncommand: docs-mcp\n")
	if _, warnings, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if _, warnings, err := runImportUserTest("codex"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	for _, rel := range []string{"mcps/secret.yaml", "mcps/docs.yaml", "settings/imported.yaml"} {
		if _, err := os.Stat(filepath.Join(source, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s must not be written: %v", rel, err)
		}
	}
	_ = home
}

func TestImportUser_HandWrittenShapesRoundTrip(t *testing.T) {
	home, source := userAgentTestHome(t)
	files := map[string]string{
		".cursor/mcp.json":         `{"mcpServers": {"s": {"command": "x"}, "r": {"url": "https://x/mcp", "headers": {"A": "b"}}}}`,
		".copilot/mcp-config.json": `{"mcpServers": {"a": {"type": "local", "command": "x", "args": [], "tools": ["*"]}, "b": {"command": "y"}}}`,
		".gemini/settings.json":    `{"mcpServers": {"g": {"url": "https://x", "type": "http"}, "h": {"httpUrl": "https://y"}}}`,
	}
	for rel, body := range files {
		writeFile(t, filepath.Join(home, filepath.FromSlash(rel)), body+"\n")
	}
	if _, warnings, err := runImportUserTest("cursor", "copilot", "gemini"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	for _, name := range []string{"s", "r", "a", "b", "g", "h"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, target := range []string{"cursor", "copilot", "gemini"} {
		if _, warnings, err := runUserAgentTest("--only", target); err != nil {
			t.Fatalf("sync %s after import: %v\n%s", target, err, warnings)
		}
	}
	// Each file keeps its own servers exactly as written; the other
	// tools' imported servers join it, as home specs reach every tool.
	for rel, body := range files {
		var before, after map[string]map[string]any
		if err := json.Unmarshal([]byte(body), &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(readFile(t, filepath.Join(home, filepath.FromSlash(rel)))), &after); err != nil {
			t.Fatal(err)
		}
		for name, server := range before["mcpServers"] {
			if !reflect.DeepEqual(after["mcpServers"][name], server) {
				t.Errorf("%s: %s = %v, want %v", rel, name, after["mcpServers"][name], server)
			}
		}
	}
}

func TestImportUser_SkipsServerThatWouldNotRoundTrip(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.a]\ncommand = \"x\"\n\n[mcp_servers.a.tools.t]\napproval_mode = \"approve\"\n")
	_, warnings, err := runImportUserTest("codex")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "a.yaml")); err == nil {
		if _, w, err := runUserAgentTest("--only", "codex", "--check"); err != nil {
			t.Errorf("an imported server must round-trip: %v\n%s", err, w)
		}
		return
	}
	if !strings.Contains(warnings, "skipped MCP server a") {
		t.Errorf("a skipped server must be named:\n%s", warnings)
	}
}

func TestImportUser_OneServerAcrossToolsImportsOnce(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(home, ".copilot", "mcp-config.json"), `{"mcpServers": {"d": {"command": "x", "tools": ["*"]}}}`+"\n")
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {"d": {"command": "x"}}}`+"\n")
	_, warnings, err := runImportUserTest("copilot", "cursor")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if strings.Contains(warnings, "differently") {
		t.Errorf("one server in two spellings is not a clash:\n%s", warnings)
	}
	if got := readFile(t, filepath.Join(source, "mcps", "d.yaml")); strings.Contains(got, "tools") {
		t.Errorf("Copilot's default tools must not reach the spec:\n%s", got)
	}
	if _, w, err := runUserAgentTest("--only", "copilot,cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	for rel, want := range map[string]string{".copilot/mcp-config.json": `{"mcpServers": {"d": {"command": "x", "tools": ["*"]}}}`, ".cursor/mcp.json": `{"mcpServers": {"d": {"command": "x"}}}`} {
		if got := readFile(t, filepath.Join(home, filepath.FromSlash(rel))); got != want+"\n" {
			t.Errorf("%s: both tools must adopt the server as written:\n%s", rel, got)
		}
	}
}

func TestImportUser_RelativeRootFails(t *testing.T) {
	userAgentTestHome(t)
	t.Setenv("CODEX_HOME", "rel")
	if _, _, err := runImportUserTest("codex"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("err = %v", err)
	}
}

func TestImportUser_CopilotNativeClientSettingsRoundTrip(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	path := filepath.Join(home, ".copilot/mcp-config.json")
	original := "{\n\t\"unrelated\":true,\n\t\"mcpServers\":{\n\t\t\"remote\":{\"type\":\"http\",\"url\":\"https://example.invalid/mcp\",\"oauthClientId\":\"public-test-client\",\"timeout\":45000,\"tools\":[\"*\"]},\n\t\t\"local\":{\"command\":\"server\",\"cwd\":\"/tmp\",\"timeout\":45000,\"tools\":[\"*\"]}\n\t}\n}\n"
	writeFile(t, path, original)
	if _, warnings, err := runImportUserTest("copilot"); err != nil {
		t.Fatalf("import: %v %s", err, warnings)
	}
	for _, name := range []string{"remote", "local"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Errorf("server %s not adopted: %v", name, err)
		}
	}
	for range 2 {
		if _, warnings, err := runUserAgentTest("--only", "copilot"); err != nil {
			t.Fatalf("sync: %v %s", err, warnings)
		}
		if got := readFile(t, path); got != original {
			t.Errorf("native bytes changed: %s", got)
		}
	}
	if _, _, err := runUserAgentTest("--only", "copilot", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestImportUser_CopilotExtraServerFieldsRoundTrip(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	path := filepath.Join(home, ".copilot/mcp-config.json")
	original := "{\n\t\"mcpServers\":{\n\t\t\"remote\":{\"type\":\"http\",\"url\":\"https://example.invalid/mcp\",\"oauthClientId\":\"abc\",\"oauthScopes\":[\"read\"],\"oauthPublicClient\":false,\"oauthGrantType\":\"client_credentials\",\"deferTools\":\"never\",\"filterMapping\":\"markdown\",\"slowConnectionThresholdMs\":5000,\"oidc\":true},\n\t\t\"local\":{\"command\":\"server\",\"disableToolCache\":true,\"oidc\":true,\"slowConnectionThresholdMs\":2147483647,\"deferTools\":\"never\",\"filterMapping\":\"none\"}\n\t}\n}\n"
	writeFile(t, path, original)
	if _, warnings, err := runImportUserTest("copilot"); err != nil {
		t.Fatalf("import: %v %s", err, warnings)
	}
	for _, name := range []string{"remote", "local"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Errorf("server %s not adopted: %v", name, err)
		}
	}
	for range 2 {
		if _, warnings, err := runUserAgentTest("--only", "copilot"); err != nil {
			t.Fatalf("sync: %v %s", err, warnings)
		}
		if got := readFile(t, path); got != original {
			t.Errorf("native bytes changed: %s", got)
		}
	}
	if _, _, err := runUserAgentTest("--only", "copilot", "--check"); err != nil {
		t.Fatal(err)
	}
}
