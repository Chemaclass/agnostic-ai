package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
	"golang.org/x/sys/windows"
)

func TestImportAntigravity_PreviewMatchesWindowsShortPaths(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		shortRoot, shortFile bool
	}{
		{"short project and long file", true, false},
		{"long project and short file", false, true},
		{"short project and short file", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			physical := resolveImportSourceExisting(t.TempDir())
			mcp := filepath.Join(".agents", "plugins", "team", "mcp.json")
			const native = `{"mcpServers":{"docs":{"serverUrl":"https://example.com/mcp"}}}`
			mustWriteFile(t, filepath.Join(physical, mcp), native)
			short := antigravityShortPath(t, physical)
			root, file := physical, filepath.Join(physical, mcp)
			if tc.shortRoot {
				root = short
			}
			if tc.shortFile {
				file = filepath.Join(short, mcp)
			}
			testutil.Chdir(t, root)
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: %q\n", filepath.ToSlash(file)))
			before := snapshotProject(t, physical)
			for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}} {
				output := captureStdout(t, func() {
					if out, err := runRoot(t, append([]string{"import", "antigravity"}, flags...)...); err != nil {
						t.Errorf("flags %v: preview: %v\n%s", flags, err, out)
					}
				})
				if !strings.Contains(filepath.ToSlash(output), ".agnostic-ai/mcps/docs.yaml") || !strings.Contains(output, "dry-run: 1 file(s)") {
					t.Errorf("flags %v: preview missing server:\n%s", flags, output)
				}
				assertProjectUnchanged(t, before, snapshotProject(t, physical))
			}
			if out, err := runRoot(t, "import", "antigravity"); err != nil {
				t.Fatalf("real import: %v\n%s", err, out)
			}
			if got := readFile(t, filepath.Join(physical, mcp)); got != native {
				t.Errorf("real import changed native MCP: %s", got)
			}
		})
	}
}

func antigravityShortPath(t *testing.T, path string) string {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetShortPathName(name, &buffer[0], uint32(len(buffer)))
		if err != nil {
			t.Fatalf("short path for %s: %v", path, err)
		}
		if n < uint32(len(buffer)) {
			short := windows.UTF16ToString(buffer[:n])
			if strings.EqualFold(short, path) {
				t.Skip("the volume supplies no distinct 8.3 path; directory-junction coverage still runs")
			}
			return short
		}
		buffer = make([]uint16, n+1)
	}
}
