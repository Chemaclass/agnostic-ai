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

func colonMCPProject(t *testing.T, targets string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "mcps", "ns-tool.yaml"), "name: ns:tool\ncommand: npx\n")
	return dir
}

// Import reuses the spec that holds the server name, whatever its file
// is called, so a re-import never adds a second spec for one server.
func TestImport_ClaudeReusesTheSpecHoldingTheServerName(t *testing.T) {
	dir := colonMCPProject(t, "claude")
	mustSync(t)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := mcpSpecFiles(t, dir); !slices.Equal(got, []string{"ns-tool.yaml"}) {
		t.Fatalf("mcps folder = %v, want only ns-tool.yaml", got)
	}
}

func TestImport_ContinueReusesTheSpecHoldingTheServerName(t *testing.T) {
	for _, tt := range []struct {
		file, body string
	}{
		{"servers.json", `{"mcpServers": {"ns:tool": {"command": "npx"}}}`},
		{"tool.yaml", "name: ns:tool\ncommand: npx\n"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			dir := colonMCPProject(t, "claude")
			mustWrite(t, filepath.Join(dir, continueMCPServersDir, tt.file), tt.body)
			if out, err := runCLI(t, "import", "continue", "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			if got := mcpSpecFiles(t, dir); !slices.Equal(got, []string{"ns-tool.yaml"}) {
				t.Fatalf("mcps folder = %v, want only ns-tool.yaml", got)
			}
		})
	}
}

// Windows stores ns:tool.yaml as a hidden stream of a file ns, which no
// later sync sees, so import refuses the name before writing anything.
func TestImport_ClaudeRefusesAColonMCPFileNameOnWindowsOnly(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".claude", "rules", "style.md"), "Keep it short.\n")
	mustWrite(t, filepath.Join(dir, claudeMCPFile), `{"mcpServers": {"ns:tool": {"command": "npx"}}}`)
	out, err := runCLI(t, "import", "claude")
	if runtime.GOOS != "windows" {
		if err != nil {
			t.Fatalf("import: %v\n%s", err, out)
		}
		if got := mcpSpecFiles(t, dir); !slices.Equal(got, []string{"ns:tool.yaml"}) {
			t.Fatalf("mcps folder = %v, want only ns:tool.yaml", got)
		}
		return
	}
	file := filepath.Join(".agnostic-ai", "mcps", "ns:tool.yaml")
	if !errors.Is(err, errImportDataStream) || !strings.Contains(err.Error(), `"ns:tool"`) || !strings.Contains(err.Error(), file) {
		t.Fatalf("err = %v, want a colon refusal naming ns:tool and %s\n%s", err, file, out)
	}
	if got := mcpSpecFiles(t, dir); len(got) != 0 {
		t.Errorf("refused import left MCP files: %v", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "style.md")); !os.IsNotExist(statErr) {
		t.Errorf("refused import kept the rule it wrote first: %v", statErr)
	}
}

func TestImportWriteFile_RefusesAColonNameOnWindowsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy:prod.md")
	err := importWriteFile(path, []byte("x"), 0o644)
	if runtime.GOOS != "windows" {
		if err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	if !errors.Is(err, errImportDataStream) {
		t.Fatalf("err = %v, want the colon refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(path), "deploy")); !os.IsNotExist(statErr) {
		t.Errorf("refused write still created a file: %v", statErr)
	}
}
