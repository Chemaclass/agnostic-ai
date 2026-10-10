package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_AntigravityAndWarpMCPReachUserFiles(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mcps := filepath.Join(source, "mcps")
	mustWriteGlobalTest(t, filepath.Join(mcps, "docs.yaml"), globalDocsMCP+"cwd: /srv/docs\nenv:\n  DEBUG: \"1\"\n")
	mustWriteGlobalTest(t, filepath.Join(mcps, "api.yaml"), "name: api\ntype: http\nurl: https://api.test/mcp\nheaders:\n  X-Key: k\n")
	antigravityPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	antigravityBefore := "{\n  \"mcpServers\": {\n    \"mine\": {\n      \"command\": \"mine\"\n    }\n  }\n}\n"
	mustWriteGlobalTest(t, antigravityPath, antigravityBefore)
	warpPath := filepath.Join(home, ".warp", ".mcp.json")
	warpBefore := "{\n  \"mcpServers\": {\n    \"mine\": {\n      \"url\": \"https://mine.test\"\n    }\n  }\n}\n"
	mustWriteGlobalTest(t, warpPath, warpBefore)

	if _, w, err := runGlobalAgentTest("--only", "antigravity,warp"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	servers := func(path string) map[string]map[string]any {
		t.Helper()
		var doc map[string]map[string]map[string]any
		if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return doc["mcpServers"]
	}
	ag := servers(antigravityPath)
	if ag["mine"]["command"] != "mine" {
		t.Errorf("antigravity must keep the user's own server: %v", ag)
	}
	if ag["docs"]["command"] != "docs-mcp" || ag["docs"]["cwd"] != "/srv/docs" {
		t.Errorf("antigravity docs = %v", ag["docs"])
	}
	if ag["api"]["serverUrl"] != "https://api.test/mcp" || ag["api"]["url"] != nil {
		t.Errorf("antigravity remote servers use serverUrl: %v", ag["api"])
	}
	wp := servers(warpPath)
	if wp["mine"]["url"] != "https://mine.test" {
		t.Errorf("warp must keep the user's own server: %v", wp)
	}
	if wp["docs"]["command"] != "docs-mcp" || wp["docs"]["working_directory"] != "/srv/docs" || wp["docs"]["cwd"] != nil {
		t.Errorf("warp docs = %v", wp["docs"])
	}
	if wp["api"]["url"] != "https://api.test/mcp" || wp["api"]["serverUrl"] != nil {
		t.Errorf("warp api = %v", wp["api"])
	}
	if _, w, err := runGlobalAgentTest("--only", "antigravity,warp", "--check"); err != nil {
		t.Fatalf("check after sync: %v\n%s", err, w)
	}

	if err := os.RemoveAll(mcps); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "antigravity,warp"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, antigravityPath); got != antigravityBefore {
		t.Errorf("removal must restore mcp_config.json:\n%q", got)
	}
	if got := readGlobalTest(t, warpPath); got != warpBefore {
		t.Errorf("removal must restore .mcp.json:\n%q", got)
	}
}

func TestSyncGlobal_AntigravityAndWarpMCPCreateTheirFiles(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP)
	if _, w, err := runGlobalAgentTest("--only", "antigravity,warp"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	for _, rel := range []string{".gemini/config/mcp_config.json", ".warp/.mcp.json"} {
		if got := readGlobalTest(t, filepath.Join(home, filepath.FromSlash(rel))); !strings.Contains(got, "docs-mcp") {
			t.Errorf("%s: %s", rel, got)
		}
	}
	if err := os.Remove(filepath.Join(source, "mcps", "docs.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "antigravity,warp"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	for _, rel := range []string{".gemini/config/mcp_config.json", ".warp/.mcp.json"} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s: a file sync created must go once nothing is left in it (%v)", rel, err)
		}
	}
}

func TestSyncGlobal_WarpDisabledMCPStaysOutOfUserFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"), globalDocsMCP+"disabled: true\n")
	if _, w, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if data, err := os.ReadFile(filepath.Join(home, ".warp", ".mcp.json")); err == nil && strings.Contains(string(data), "docs") {
		t.Errorf("~/.warp/.mcp.json is on by default and has no disable key:\n%s", data)
	}
}

func TestImportGlobal_AntigravityAndWarpMCPShapesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		target, rel, file string
		names             []string
	}{
		{"antigravity", ".gemini/config/mcp_config.json", `{"mcpServers":{"api":{"serverUrl":"https://api.test/mcp"},"docs":{"command":"docs-mcp","args":["--stdio"],"cwd":"/srv"}}}` + "\n", []string{"api", "docs"}},
		{"warp", ".warp/.mcp.json", `{"mcpServers":{"api":{"url":"https://w.test/mcp"},"docs":{"command":"w-mcp","args":[],"working_directory":"/w"}}}` + "\n", []string{"api", "docs"}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, filepath.FromSlash(tc.rel))
			mustWriteGlobalTest(t, path, tc.file)
			if _, w, err := runImportGlobalTest(tc.target); err != nil {
				t.Fatalf("import: %v\n%s", err, w)
			}
			for _, name := range tc.names {
				if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
					t.Errorf("%s was not imported: %v", name, err)
				}
			}
			if _, w, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync after import: %v\n%s", err, w)
			}
			if got := readGlobalTest(t, path); got != tc.file {
				t.Errorf("the user file changed after import:\n%s\nwant:\n%s", got, tc.file)
			}
		})
	}
}
