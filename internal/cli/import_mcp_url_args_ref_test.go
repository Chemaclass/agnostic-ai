package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Sync references in `url` and `args`, then import the native file
// back: the specs must say the same references and keep the literals,
// and a second sync must write the same bytes.
func TestImportMCP_URLArgsRefsRoundTrip(t *testing.T) {
	for target, tc := range map[string]struct{ native, url, arg string }{
		"claude":   {".mcp.json", `"https://${API_HOST}/mcp"`, `"${GH_TOKEN}"`},
		"windsurf": {".devin/mcp_config.json", `"https://${env:API_HOST}/mcp"`, `"${env:GH_TOKEN}"`},
		"opencode": {"opencode.json", `"https://{env:API_HOST}/mcp"`, `"{env:GH_TOKEN}"`},
		"continue": {".continue/mcpServers/gh.yaml", "", "${{ secrets.GH_TOKEN }}"},
	} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			captureLog(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
			gh := "name: gh\ncommand: gh-mcp\nargs:\n  - --token\n  - ${GH_TOKEN}\n  - --root\n  - ${workspaceFolder}\n  - pa55$word\n"
			api := "name: api\ntype: sse\nurl: https://${API_HOST}/mcp\n"
			plain := "name: plain\ntype: sse\nurl: https://mcp.example.com/mcp?$filter=x\n"
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), gh)
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "api.yaml"), api)
			writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "plain.yaml"), plain)

			execCLI(t, "sync", "-t", target)
			native := snapshotEmitted(t, dir)[tc.native]
			for _, want := range []string{tc.url, tc.arg, "${workspaceFolder}", "pa55$word"} {
				if !strings.Contains(native, want) {
					t.Fatalf("%s lacks %s:\n%s", tc.native, want, native)
				}
			}
			if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "import", target)

			for file, wants := range map[string][]string{
				"gh.yaml":    {"- ${GH_TOKEN}", "- ${workspaceFolder}", "- pa55$word"},
				"api.yaml":   {"url: https://${API_HOST}/mcp"},
				"plain.yaml": {"url: https://mcp.example.com/mcp?$filter=x"},
			} {
				got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", file))
				for _, want := range wants {
					if !strings.Contains(got, want) {
						t.Errorf("%s lacks %q:\n%s", file, want, got)
					}
				}
			}
			execCLI(t, "sync", "-t", target)
			if second := snapshotEmitted(t, dir)[tc.native]; second != native {
				t.Errorf("%s changed after import:\n%s\nwant:\n%s", tc.native, second, native)
			}
			execCLI(t, "sync", "--check", "-t", target)
		})
	}
}

func TestImportMCP_URLArgsLiteralsStayLiteral(t *testing.T) {
	specs, out := importMCPServers(t, "claude", map[string]any{
		"gh":  map[string]any{"command": "gh-mcp", "args": []any{"--token", "ghp_example", "--host", "$HOST"}},
		"api": map[string]any{"type": "http", "url": "https://user:pw@api.example.com/mcp"},
	})
	for name, want := range map[string]string{"gh": "- ghp_example", "api": "url: https://user:pw@api.example.com/mcp"} {
		if !strings.Contains(specs[name], want) {
			t.Errorf("import must keep a literal url or argument (%q):\n%s", want, specs[name])
		}
	}
	if !strings.Contains(specs["gh"], "- $HOST") {
		t.Errorf("claude does not expand $HOST, so import keeps it:\n%s", specs["gh"])
	}
	if strings.Contains(out, "now reads") {
		t.Errorf("import reports only env and header replacements:\n%s", out)
	}
}

func TestImportMCP_ReadsCrushURLArgsForms(t *testing.T) {
	specs, _ := importMCPServers(t, "crush", map[string]any{
		"gh": map[string]any{"command": "gh-mcp", "args": []any{"$GH_TOKEN", "pa55$word"}},
	})
	if !strings.Contains(specs["gh"], "- ${GH_TOKEN}") || !strings.Contains(specs["gh"], "- pa55$word") {
		t.Errorf("crush reads a whole-value $NAME argument back:\n%s", specs["gh"])
	}
}

func TestSync_URLArgsRefLeavesServerOutWithNote(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLog(t)
	notes := captureNotes(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [zed]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), "name: gh\ncommand: gh-mcp\nargs: [--token, \"${GH_TOKEN}\"]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "plain.yaml"), "name: plain\ncommand: npx\nargs: [-y, server]\n")
	execCLI(t, "sync", "-t", "zed")
	settings := snapshotEmitted(t, dir)[".zed/settings.json"]
	if strings.Contains(settings, "GH_TOKEN") || strings.Contains(settings, `"gh"`) {
		t.Errorf("zed expands no reference, so gh must be left out:\n%s", settings)
	}
	if !strings.Contains(settings, `"plain"`) {
		t.Errorf("a server without a reference still emits:\n%s", settings)
	}
	if got := notes.String(); !strings.Contains(got, "`args` on 1 mcp has no effect on zed (server gh reads ${GH_TOKEN} in `args`") {
		t.Errorf("no note names the server, field, and variable:\n%s", got)
	}
}

func TestImportGlobal_CursorURLArgsRefsRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mcpDir := filepath.Join(source, "mcps")
	mustWriteGlobalTest(t, filepath.Join(mcpDir, "gh.yaml"), "name: gh\ncommand: gh-mcp\nargs: [--token, \"${GH_TOKEN}\", \"${workspaceFolder}\", \"${env:workspaceFolder}\"]\n")
	mustWriteGlobalTest(t, filepath.Join(mcpDir, "api.yaml"), "name: api\ntype: http\nurl: https://${API_HOST}/mcp\n")
	if _, warnings, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	cursorPath := filepath.Join(home, ".cursor", "mcp.json")
	want := readGlobalTest(t, cursorPath)
	for _, form := range []string{`"${env:GH_TOKEN}"`, `"${workspaceFolder}"`, `"${env:workspaceFolder}"`, `"https://${env:API_HOST}/mcp"`} {
		if !strings.Contains(want, form) {
			t.Fatalf("cursor lacks %s:\n%s", form, want)
		}
	}
	if err := os.RemoveAll(mcpDir); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runImportGlobalTest(); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(mcpDir, "gh.yaml")); !strings.Contains(got, "- ${GH_TOKEN}") || !strings.Contains(got, "- ${workspaceFolder}") || !strings.Contains(got, "- ${env:workspaceFolder}") {
		t.Errorf("import must read ${env:NAME} back and keep ${workspaceFolder}:\n%s", got)
	}
	if got := readGlobalTest(t, filepath.Join(mcpDir, "api.yaml")); !strings.Contains(got, "url: https://${API_HOST}/mcp") {
		t.Errorf("import must read the url reference back:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, cursorPath); got != want {
		t.Errorf("mcp.json changed after import:\n%s\nwant:\n%s", got, want)
	}
}
