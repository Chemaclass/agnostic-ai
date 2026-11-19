package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportGrok_HeadersHelperRoundTrips(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [grok]\ngitignore:\n  enabled: true\n  commit: [grok:instructions]\n")
	writeFile(t, ".grok/config.toml", "[mcp_servers.docs]\nurl = 'https://example.com/mcp'\nheaders_helper = 'sh headers.sh'\n")
	execCLI(t, "import", "grok")
	got := readFile(t, ".agnostic-ai/mcps/docs.yaml")
	assertGrokImportedHeadersHelper(t, got)
	execCLI(t, "sync")
	if got := readFile(t, ".grok/config.toml"); !strings.Contains(got, `headers_helper = "sh headers.sh"`) {
		t.Errorf("helper lost:\n%s", got)
	}
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
}

func TestSyncGlobal_GrokHeadersHelperPreservesUserConfig(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "created"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			t.Setenv("GROK_HOME", "")
			path := filepath.Join(home, ".grok/config.toml")
			before := "# Keep formatting.\n[plugins]\ncustom = 'untouched' # Keep note.\n"
			if existing {
				writeFile(t, path, before)
			}
			sourcePath := filepath.Join(source, "mcps/docs.yaml")
			writeFile(t, sourcePath, "name: docs\nurl: https://example.com/mcp\nx-grok:\n  headers_helper: sh headers.sh\n")
			if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			got := readGlobalTest(t, path)
			if !strings.Contains(got, `headers_helper = "sh headers.sh"`) {
				t.Errorf("global helper lost:\n%s", got)
			}
			if existing && !strings.HasPrefix(got, before) {
				t.Errorf("native text changed:\n%s", got)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !existing && runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
				t.Errorf("mode = %o, want 600", info.Mode().Perm())
			}
			if _, warnings, err := runGlobalAgentTest("--only", "grok", "--check"); err != nil {
				t.Errorf("check: %v\n%s", err, warnings)
			}
			if err := os.Remove(sourcePath); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if existing {
				if got := readGlobalTest(t, path); got != before {
					t.Errorf("remove changed original bytes:\n%s", got)
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("created config remains: %v", err)
			}
		})
	}
}

func TestImportGlobal_GrokHeadersHelperRoundTrips(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	path := filepath.Join(home, ".grok/config.toml")
	before := "# Keep comment.\n[mcp_servers.docs]\nurl = 'https://example.com/mcp'\nheaders_helper = 'sh headers.sh' # Keep helper.\n"
	writeFile(t, path, before)
	if _, warnings, err := runImportGlobalTest("grok"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	assertGrokImportedHeadersHelper(t, readGlobalTest(t, filepath.Join(source, "mcps/docs.yaml")))
	if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("native config changed:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "grok", "--check"); err != nil {
		t.Errorf("check: %v\n%s", err, warnings)
	}
}

func assertGrokImportedHeadersHelper(t *testing.T, data string) {
	t.Helper()
	entry, err := spec.ParseYAMLBytes(spec.KindMCP, []byte(data))
	if err != nil {
		t.Errorf("parse imported MCP: %v", err)
		return
	}
	native, _ := entry.Meta["x-grok"].(map[string]any)
	if native["headers_helper"] != "sh headers.sh" {
		t.Errorf("native helper metadata: %#v", entry.Meta)
	}
	if _, exists := entry.Meta["headers_helper"]; exists {
		t.Error("native helper escaped the x-grok namespace")
	}
}
