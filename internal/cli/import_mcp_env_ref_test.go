package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The #1619 reproduction: a literal token in `.mcp.json` must not reach
// a spec meant to be committed.
func TestImportClaudeMCP_LiteralsBecomeReferences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers": {
  "gh": {"command": "gh-mcp", "env": {"GITHUB_TOKEN": "ghp_example", "KEEP": "${KEEP:-x}"}},
  "my-api": {"type": "http", "url": "https://api.example.com/mcp", "headers": {"Authorization": "Bearer sk-live", "X-Api-Key": "k1", "X-Team": "Bearer ${TEAM}"}}
}}`)
	log := captureLog(t)
	if err := os.MkdirAll(filepath.Join(dir, "mcps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := importClaudeMCP(dir, filepath.Join(dir, "mcps")); err != nil {
		t.Fatal(err)
	}
	gh := readFile(t, filepath.Join(dir, "mcps", "gh.yaml"))
	api := readFile(t, filepath.Join(dir, "mcps", "my-api.yaml"))
	for _, secret := range []string{"ghp_example", "sk-live", "k1"} {
		if strings.Contains(gh+api, secret) {
			t.Errorf("literal %q reached a spec:\n%s\n%s", secret, gh, api)
		}
	}
	for doc, wants := range map[string][]string{
		gh:  {"GITHUB_TOKEN: ${GITHUB_TOKEN}", "KEEP: ${KEEP:-x}"},
		api: {"Authorization: Bearer ${MY_API_AUTHORIZATION}", "X-Api-Key: ${MY_API_X_API_KEY}", "X-Team: Bearer ${TEAM}"},
	} {
		for _, want := range wants {
			if !strings.Contains(doc, want) {
				t.Errorf("missing %q in:\n%s", want, doc)
			}
		}
	}
	out := log.String()
	for _, want := range []string{
		"MCP server gh: env GITHUB_TOKEN now reads ${GITHUB_TOKEN}; set GITHUB_TOKEN",
		"MCP server my-api: headers Authorization now reads Bearer ${MY_API_AUTHORIZATION}; set MY_API_AUTHORIZATION",
		"MCP server my-api: headers X-Api-Key now reads ${MY_API_X_API_KEY}; set MY_API_X_API_KEY",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "hint:") != 1 {
		t.Errorf("want one export hint:\n%s", out)
	}
	if strings.Contains(out, "KEEP") || strings.Contains(out, "TEAM") {
		t.Errorf("a value that is already a reference is not reported:\n%s", out)
	}
}

func TestImportMCP_ReadsEachTargetFormBack(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, windsurfMCPFile), `{"mcpServers": {"w": {"command": "w", "env": {"TOKEN": "${env:TOKEN}"}, "headers": {"Authorization": "Bearer ${env:API_KEY}"}}}}`)
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"mcp": {"o": {"type": "local", "command": ["o"], "environment": {"TOKEN": "{env:TOKEN}"}}}}`)
	captureLog(t)
	if err := os.MkdirAll(filepath.Join(dir, "mcps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := importWindsurfMCP(dir, filepath.Join(dir, "mcps")); err != nil {
		t.Fatal(err)
	}
	if _, err := importOpencodeMCP(dir, filepath.Join(dir, "mcps")); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		"w.yaml": "TOKEN: ${TOKEN}",
		"o.yaml": "TOKEN: ${TOKEN}",
	} {
		if got := readFile(t, filepath.Join(dir, "mcps", file)); !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", file, want, got)
		}
	}
	if got := readFile(t, filepath.Join(dir, "mcps", "w.yaml")); !strings.Contains(got, "Authorization: Bearer ${API_KEY}") {
		t.Errorf("windsurf header not read back:\n%s", got)
	}
}

// Sync, then import the native file back: the specs must say the same
// references, and a second sync must write the same bytes.
func TestImportMCP_EnvRefsRoundTrip(t *testing.T) {
	for target, tc := range map[string]struct{ native, form string }{
		"claude":   {".mcp.json", `"${GITHUB_TOKEN}"`},
		"codex":    {".codex/config.toml", `env_vars = ["GITHUB_TOKEN"]`},
		"windsurf": {".devin/mcp_config.json", `"${env:GITHUB_TOKEN}"`},
	} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			captureLog(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
			gh := "name: gh\ncommand: gh-mcp\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\n"
			api := "name: api\ntype: http\nurl: https://api.example.com/mcp\nheaders:\n  Authorization: Bearer ${API_KEY}\n  X-Team: ${TEAM_ID}\n"
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), gh)
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "api.yaml"), api)

			execCLI(t, "sync", "-t", target)
			first := snapshotEmitted(t, dir)[tc.native]
			if !strings.Contains(first, tc.form) {
				t.Fatalf("%s lacks %s:\n%s", tc.native, tc.form, first)
			}
			if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "import", target)

			for file, wants := range map[string][]string{
				"gh.yaml":  {"GITHUB_TOKEN: ${GITHUB_TOKEN}"},
				"api.yaml": {"Authorization: Bearer ${API_KEY}", "X-Team: ${TEAM_ID}"},
			} {
				got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", file))
				for _, want := range wants {
					if !strings.Contains(got, want) {
						t.Errorf("%s lacks %q:\n%s", file, want, got)
					}
				}
			}
			execCLI(t, "sync", "-t", target)
			if second := snapshotEmitted(t, dir)[tc.native]; second != first {
				t.Errorf("%s changed after import:\n%s\nwant:\n%s", tc.native, second, first)
			}
		})
	}
}

func TestImportGlobal_CursorEnvRefsRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mcpDir := filepath.Join(source, "mcps")
	mustWriteGlobalTest(t, filepath.Join(mcpDir, "gh.yaml"), "name: gh\ncommand: gh-mcp\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\n")
	if _, warnings, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	cursorPath := filepath.Join(home, ".cursor", "mcp.json")
	want := readGlobalTest(t, cursorPath)
	if !strings.Contains(want, `"GITHUB_TOKEN":"${env:GITHUB_TOKEN}"`) {
		t.Fatalf("cursor reads ${env:NAME}:\n%s", want)
	}
	if err := os.RemoveAll(mcpDir); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runImportGlobalTest(); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(mcpDir, "gh.yaml")); !strings.Contains(got, "GITHUB_TOKEN: ${GITHUB_TOKEN}") {
		t.Errorf("import must read ${env:NAME} back:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, cursorPath); got != want {
		t.Errorf("mcp.json changed after import:\n%s\nwant:\n%s", got, want)
	}
}
