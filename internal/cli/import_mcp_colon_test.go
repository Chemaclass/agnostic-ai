package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// fakeWindowsDataStreams makes import treat a colon after the volume as
// Windows does, so the refusal runs on every system.
func fakeWindowsDataStreams(t *testing.T) {
	t.Helper()
	prior := namesDataStream
	namesDataStream = func(p string) bool { return strings.Contains(p[len(filepath.VolumeName(p)):], ":") }
	t.Cleanup(func() { namesDataStream = prior })
}

func mcpSpecFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "mcps"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func colonMCPClaudeProject(t *testing.T) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".claude", "rules", "style.md"), "Keep it short.\n")
	mustWrite(t, filepath.Join(dir, claudeMCPFile), `{"mcpServers": {"ns:tool": {"command": "npx"}}}`)
	return dir
}

func assertColonMCPRefused(t *testing.T, dir string, err error, out string) {
	t.Helper()
	file := filepath.Join(".agnostic-ai", "mcps", "ns:tool.yaml")
	if !errors.Is(err, errImportDataStream) || !strings.Contains(err.Error(), `"ns:tool"`) || strings.Count(err.Error(), file) != 1 {
		t.Fatalf("err = %v, want one colon refusal naming ns:tool and %s once\n%s", err, file, out)
	}
	if got := mcpSpecFiles(t, dir); len(got) != 0 {
		t.Errorf("refused import left MCP files: %v", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "style.md")); !os.IsNotExist(statErr) {
		t.Errorf("refused import kept the rule it wrote first: %v", statErr)
	}
}

// Windows stores ns:tool.yaml as a hidden stream of a file ns, which no
// later sync sees, so import refuses the name and puts back what it wrote.
func TestImport_ClaudeRefusesAColonMCPFileNameWhereItNamesAStream(t *testing.T) {
	dir := colonMCPClaudeProject(t)
	fakeWindowsDataStreams(t)
	out, err := runCLI(t, "import", "claude")
	assertColonMCPRefused(t, dir, err, out)
}

func TestImport_TwoSourcesRollBackOnAColonMCPFileName(t *testing.T) {
	dir := colonMCPClaudeProject(t)
	mustWrite(t, filepath.Join(dir, ".codex", "config.toml"), "[mcp_servers.plain]\ncommand = \"x\"\n")
	fakeWindowsDataStreams(t)
	out, err := runCLI(t, "import", "claude", "codex")
	if !errors.Is(err, errImportDataStream) || !strings.Contains(err.Error(), "import failed for: claude") {
		t.Fatalf("err = %v, want the failed sources and the colon refusal\n%s", err, out)
	}
	if got := mcpSpecFiles(t, dir); len(got) != 0 {
		t.Errorf("refused import left MCP files: %v", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "style.md")); !os.IsNotExist(statErr) {
		t.Errorf("refused import kept the rule it wrote first: %v", statErr)
	}
}

func TestImport_ClaudeColonMCPFileNameOnThisSystem(t *testing.T) {
	dir := colonMCPClaudeProject(t)
	out, err := runCLI(t, "import", "claude")
	if runtime.GOOS == "windows" {
		assertColonMCPRefused(t, dir, err, out)
		return
	}
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := mcpSpecFiles(t, dir); !slices.Equal(got, []string{"ns:tool.yaml"}) {
		t.Fatalf("mcps folder = %v, want only ns:tool.yaml", got)
	}
}

// Continue's JSON servers take their file name from the server name and
// its YAML blocks keep their own, so only a colon in the file is refused.
func TestImport_ContinueRefusesAColonMCPFileNameWhereItNamesAStream(t *testing.T) {
	for _, tt := range []struct {
		file, body, written string
	}{
		{"servers.json", `{"mcpServers": {"ns:tool": {"command": "npx"}}}`, ""},
		{"ns:tool.yaml", "name: ns:tool\ncommand: npx\n", ""},
		{"tool.yaml", "name: ns:tool\ncommand: npx\n", "tool.yaml"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.Contains(tt.file, ":") {
				t.Skip("Windows cannot hold the source file")
			}
			dir := testutil.TempCwd(t)
			silence(t)
			mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
			mustWrite(t, filepath.Join(dir, continueMCPServersDir, tt.file), tt.body)
			fakeWindowsDataStreams(t)
			out, err := runCLI(t, "import", "continue")
			got := mcpSpecFiles(t, dir)
			if tt.written != "" {
				if err != nil || !slices.Equal(got, []string{tt.written}) {
					t.Fatalf("import = %v, mcps folder = %v, want only %s\n%s", err, got, tt.written, out)
				}
				return
			}
			if !errors.Is(err, errImportDataStream) || !strings.Contains(err.Error(), `"ns:tool"`) || !strings.Contains(err.Error(), "ns:tool.yaml") {
				t.Fatalf("err = %v, want a colon refusal naming ns:tool and its file\n%s", err, out)
			}
			if len(got) != 0 {
				t.Errorf("refused import left MCP files: %v", got)
			}
		})
	}
}

// A spec writer that takes its file name from a spec name, such as a
// Kimi rule, names the refused file once.
func TestImportWriteFile_RefusesAColonNameWhereItNamesAStream(t *testing.T) {
	fakeWindowsDataStreams(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy:prod.md")
	err := writeKimiRule(dir, "deploy:prod", "Ship it.", "")
	if !errors.Is(err, errImportDataStream) || strings.Count(err.Error(), path) != 1 {
		t.Fatalf("err = %v, want the colon refusal naming %s once", err, path)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("refused write still created the file: %v", statErr)
	}
}

// One colon-named server in a home config does not stop import --global.
func TestImportGlobal_SkipsAColonMCPNameWhereItNamesAStream(t *testing.T) {
	home, source := globalAgentTestHome(t)
	config := filepath.Join(home, ".cursor", "mcp.json")
	mustWriteGlobalTest(t, config, `{"mcpServers": {"ns:tool": {"command": "x"}, "plain": {"command": "x"}}}`+"\n")
	fakeWindowsDataStreams(t)
	_, warnings, err := runImportGlobalTest("cursor")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if want := "skipped MCP server ns:tool: left out; " + config + " names it with a colon"; !strings.Contains(warnings, want) {
		t.Errorf("warnings miss %q:\n%s", want, warnings)
	}
	entries, err := os.ReadDir(filepath.Join(source, "mcps"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "plain.yaml" {
		t.Errorf("home mcps = %v, want only plain.yaml", entries)
	}
}
