package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportGlobal_GrokDescribedServerRoundTrips(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".grok", "config.toml")
	native := "# Native notes stay here.\n[mcp_servers.docs]\ncommand = 'docs-mcp'\nargs = ['--stdio']\ndescription = 'Docs search' # ignored\n"
	mustWriteGlobalTest(t, path, native)
	_, warnings, err := runImportGlobalTest("grok")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if !strings.Contains(warnings, "grok mcp docs: description has no effect on Grok, so import left it out") {
		t.Errorf("missing import note:\n%s", warnings)
	}
	got := readGlobalTest(t, filepath.Join(source, "mcps", "docs.yaml"))
	if strings.Contains(got, "description") {
		t.Errorf("spec retains ignored description:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != native {
		t.Errorf("sync changed native config:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "grok", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, warnings)
	}
}

func TestImportGrok_DescribedProjectServerRoundTrips(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	seedGrokNative(t, dir)
	logged := importLog(t)
	path := filepath.Join(dir, ".grok", "config.toml")
	writeFile(t, path, "[mcp_servers.docs]\ncommand = 'docs-mcp'\ndescription = 'Docs search'\n")
	execCLI(t, "import", "grok")
	if !strings.Contains(logged.String(), "grok mcp docs: description has no effect on Grok, so import left it out") {
		t.Errorf("missing import note:\n%s", logged.String())
	}
	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "docs.yaml"))
	if strings.Contains(got, "description") {
		t.Errorf("spec retains ignored description:\n%s", got)
	}
	execCLI(t, "sync", "-t", "grok")
	native, _, err := readGrokConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	server, ok := native["docs"].(map[string]any)
	if !ok || server["command"] != "docs-mcp" {
		t.Fatalf("server missing after sync: %#v", native)
	}
	if _, exists := server["description"]; exists {
		t.Errorf("sync retains ignored description: %#v", server)
	}
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
}

func TestImportGlobal_GrokKeepsOtherToolsDescriptionInEitherOrder(t *testing.T) {
	for _, other := range []string{"claude", "cursor"} {
		for _, grokFirst := range []bool{false, true} {
			targets := []string{other, "grok"}
			if grokFirst {
				targets = []string{"grok", other}
			}
			t.Run(strings.Join(targets, "_"), func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				otherRel := ".claude.json"
				if other == "cursor" {
					otherRel = ".cursor/mcp.json"
				}
				paths := map[string]string{
					filepath.Join(home, ".grok/config.toml"): "[mcp_servers.fs]\ncommand = 'fs-mcp'\ndescription = 'Ignored native note'\n",
					filepath.Join(home, otherRel):            "{\"mcpServers\":{\"fs\":{\"command\":\"fs-mcp\",\"description\":\"Files\"}}}\n",
				}
				for path, native := range paths {
					mustWriteGlobalTest(t, path, native)
				}
				_, warnings, err := runImportGlobalTest(targets...)
				if err != nil {
					t.Fatalf("import: %v\n%s", err, warnings)
				}
				if strings.Contains(warnings, "defines fs differently") {
					t.Errorf("ignored description caused conflict:\n%s", warnings)
				}
				if got := readGlobalTest(t, filepath.Join(source, "mcps/fs.yaml")); !strings.Contains(got, "description: Files") {
					t.Errorf("lost supported description:\n%s", got)
				}
				if _, warnings, err := runGlobalAgentTest("--only", strings.Join(targets, ",")); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
				for path, native := range paths {
					if got := readGlobalTest(t, path); got != native {
						t.Errorf("sync changed %s:\n%s", path, got)
					}
				}
			})
		}
	}
}

func TestImportGlobal_CodexAndGrokDescriptionPreference(t *testing.T) {
	for _, targets := range [][]string{{"codex", "grok"}, {"grok", "codex"}, {"codex", "grok", "claude"}, {"grok", "codex", "claude"}, {"claude", "grok", "codex"}} {
		t.Run(strings.Join(targets, "_"), func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			paths := map[string]string{
				filepath.Join(home, ".codex/config.toml"): "[mcp_servers.fs]\ncommand = 'fs-mcp'\ndescription = 'Ignored Codex note'\nroots = [{ uri = 'file:///ignored' }]\n",
				filepath.Join(home, ".grok/config.toml"):  "[mcp_servers.fs]\ncommand = 'fs-mcp'\ndescription = 'Ignored Grok note'\n",
			}
			if len(targets) == 3 {
				paths[filepath.Join(home, ".claude.json")] = "{\"mcpServers\":{\"fs\":{\"command\":\"fs-mcp\",\"description\":\"Files\"}}}\n"
			}
			for path, native := range paths {
				mustWriteGlobalTest(t, path, native)
			}
			_, warnings, err := runImportGlobalTest(targets...)
			if err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if strings.Contains(warnings, "defines fs differently") {
				t.Errorf("ignored keys caused conflict:\n%s", warnings)
			}
			got := readGlobalTest(t, filepath.Join(source, "mcps/fs.yaml"))
			if strings.Contains(got, "file:///ignored") {
				t.Errorf("retained ignored Codex roots:\n%s", got)
			}
			if len(targets) == 3 && !strings.Contains(got, "description: Files") {
				t.Errorf("lost Claude description:\n%s", got)
			}
			if _, warnings, err := runGlobalAgentTest("--only", strings.Join(targets, ",")); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			for path, native := range paths {
				if got := readGlobalTest(t, path); got != native {
					t.Errorf("sync changed %s:\n%s", path, got)
				}
			}
		})
	}
}

func TestPreviewMCPSpec_GrokDropsIgnoredDescription(t *testing.T) {
	native := map[string]any{"command": "docs-mcp", "description": "Docs search", "roots": []any{map[string]any{"uri": "file:///workspace"}}}
	data, _, err := previewMCPSpec("grok", "docs", ".grok/config.toml", "toml", native)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "description") || !strings.Contains(string(data), "file:///workspace") {
		t.Errorf("preview changed retained fields or kept description:\n%s", data)
	}
	if native["description"] != "Docs search" {
		t.Errorf("preview mutated native server: %#v", native)
	}
}

func TestSameImportedMCPServer_GrokRootsStillCount(t *testing.T) {
	server := func(uri string) map[string]any {
		return map[string]any{"command": "fs-mcp", "roots": []any{map[string]any{"uri": uri}}}
	}
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"grok", "claude", false},
		{"claude", "grok", false},
		{"grok", "codex", true},
		{"codex", "grok", true},
	} {
		t.Run(tc.a+"_"+tc.b, func(t *testing.T) {
			if got := sameImportedMCPServer(tc.a, server("file:///a"), tc.b, server("file:///b")); got != tc.same {
				t.Errorf("sameImportedMCPServer() = %v, want %v", got, tc.same)
			}
		})
	}
}
