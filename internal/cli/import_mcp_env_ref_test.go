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
  "gh": {"command": "gh-mcp", "env": {"GITHUB_TOKEN": "ghp_example", "KEEP": "${KEEP}"}},
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
		gh:  {"GITHUB_TOKEN: ${GITHUB_TOKEN}", "KEEP: ${KEEP}"},
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

// importMCPServers runs the shared writer for target and returns each
// written spec by server name plus the printed lines.
func importMCPServers(t *testing.T, target string, servers map[string]any) (map[string]string, string) {
	t.Helper()
	dir := t.TempDir()
	log := captureLog(t)
	if _, err := writeMCPYAMLs(target, servers, dir); err != nil {
		t.Fatal(err)
	}
	specs := map[string]string{}
	for name := range servers {
		specs[name] = readFile(t, filepath.Join(dir, name+".yaml"))
	}
	return specs, log.String()
}

func TestImportMCP_StripsADefaultValue(t *testing.T) {
	specs, out := importMCPServers(t, "claude", map[string]any{
		"gh": map[string]any{"command": "gh-mcp", "env": map[string]any{"API_KEY": "${VAR:-sk-live-123}", "EMPTY": "${E:-}"}},
	})
	if strings.Contains(specs["gh"]+out, "sk-live-123") {
		t.Errorf("a default value reached the spec or the output:\n%s\n%s", specs["gh"], out)
	}
	for _, want := range []string{"API_KEY: ${VAR}", "EMPTY: ${E:-}"} {
		if !strings.Contains(specs["gh"], want) {
			t.Errorf("missing %q:\n%s", want, specs["gh"])
		}
	}
	if !strings.Contains(out, "MCP server gh: env API_KEY now reads ${VAR} without its default; set VAR") {
		t.Errorf("output does not name the variable:\n%s", out)
	}
}

func TestImportMCP_VariableNamesNeverCollide(t *testing.T) {
	specs, _ := importMCPServers(t, "claude", map[string]any{
		"a":   map[string]any{"command": "a", "env": map[string]any{"API_KEY": "one"}},
		"b":   map[string]any{"command": "b", "env": map[string]any{"API_KEY": "two"}},
		"c":   map[string]any{"url": "https://c", "env": map[string]any{"TOKEN": "same"}, "headers": map[string]any{"d-X": "p"}},
		"c-d": map[string]any{"url": "https://cd", "env": map[string]any{"TOKEN": "same"}, "headers": map[string]any{"X": "q"}},
		"e":   map[string]any{"command": "e", "env": map[string]any{"GH": "${GH}"}},
		"f":   map[string]any{"command": "f", "env": map[string]any{"GH": "lit"}},
		"foo": map[string]any{"url": "https://foo", "headers": map[string]any{"x-a": "1", "x_a": "2"}},
	})
	for name, wants := range map[string][]string{
		"a":   {"API_KEY: ${A_API_KEY}"},
		"b":   {"API_KEY: ${B_API_KEY}"},
		"c":   {"TOKEN: ${TOKEN}", "d-X: ${C_D_X}"},
		"c-d": {"TOKEN: ${TOKEN}", "X: ${C_D_X_2}"},
		"f":   {"GH: ${F_GH}"},
		"foo": {"x-a: ${FOO_X_A}", "x_a: ${FOO_X_A_2}"},
	} {
		for _, want := range wants {
			if !strings.Contains(specs[name], want) {
				t.Errorf("%s lacks %q:\n%s", name, want, specs[name])
			}
		}
	}
}

func TestImportMCP_ReadsGeminiAndCrushForms(t *testing.T) {
	specs, out := importMCPServers(t, "gemini", map[string]any{
		"g": map[string]any{"command": "g", "env": map[string]any{"A": "%WIN_TOKEN%", "B": "$MY_KEY", "PASS": "pa55$word"}},
	})
	for _, want := range []string{"A: ${WIN_TOKEN}", "B: ${MY_KEY}", "PASS: ${PASS}"} {
		if !strings.Contains(specs["g"], want) {
			t.Errorf("missing %q:\n%s", want, specs["g"])
		}
	}
	if strings.Contains(specs["g"], "pa55") || !strings.Contains(out, "env PASS now reads ${PASS}") {
		t.Errorf("a `$` inside a literal is not a reference:\n%s\n%s", specs["g"], out)
	}
	if strings.Contains(out, "env A ") || strings.Contains(out, "env B ") {
		t.Errorf("a value that already reads a variable is not replaced:\n%s", out)
	}

	specs, out = importMCPServers(t, "crush", map[string]any{
		"c": map[string]any{"command": "c", "env": map[string]any{"TOKEN": "$(op read op://vault/item)"}},
	})
	if !strings.Contains(specs["c"], "TOKEN: ${TOKEN}") {
		t.Errorf("a command value becomes a reference:\n%s", specs["c"])
	}
	if !strings.Contains(out, "(the value ran op)") || strings.Contains(out, "vault") {
		t.Errorf("output must name the command and nothing else:\n%s", out)
	}
}

func TestImportMCP_TextAroundAReferenceIsReplacedWhole(t *testing.T) {
	specs, out := importMCPServers(t, "claude", map[string]any{
		"db": map[string]any{
			"url":     "https://db.example.com/mcp",
			"env":     map[string]any{"DB_URL": "postgres://u:hunter2@${HOST}/db", "PAIR": "${HOST}:${PORT}"},
			"headers": map[string]any{"Authorization": "Bearer sk-1 ${EXTRA}", "X-Ref": "Bearer ${TOKEN}"},
		},
	})
	for _, secret := range []string{"hunter2", "sk-1"} {
		if strings.Contains(specs["db"]+out, secret) {
			t.Errorf("literal %q survived:\n%s\n%s", secret, specs["db"], out)
		}
	}
	for _, want := range []string{"DB_URL: ${DB_URL}", "PAIR: ${PAIR}", "Authorization: Bearer ${DB_AUTHORIZATION}", "X-Ref: Bearer ${TOKEN}"} {
		if !strings.Contains(specs["db"], want) {
			t.Errorf("missing %q:\n%s", want, specs["db"])
		}
	}
	if !strings.Contains(out, "env DB_URL now reads ${DB_URL}; set DB_URL") {
		t.Errorf("no set line for the replaced value:\n%s", out)
	}
}

func TestImportMCP_UnknownTokenIsReplacedWhole(t *testing.T) {
	specs, out := importMCPServers(t, "copilot", map[string]any{
		"vs": map[string]any{"command": "vs", "env": map[string]any{"API_KEY": "${input:api-key}", "CFG": "${workspaceFolder}/cfg"}},
	})
	for _, want := range []string{"API_KEY: ${API_KEY}", "CFG: ${CFG}"} {
		if !strings.Contains(specs["vs"], want) {
			t.Errorf("missing %q:\n%s", want, specs["vs"])
		}
	}
	if strings.Contains(specs["vs"], "input:") {
		t.Errorf("an unknown token reached the spec:\n%s", specs["vs"])
	}
	if !strings.Contains(out, "env API_KEY now reads ${API_KEY}; set API_KEY (the value prompted for input api-key)") {
		t.Errorf("output does not name the prompt:\n%s", out)
	}
}
