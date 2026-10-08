package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReportInstalledCLIs_ListsToolsInNameOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries need a Unix executable bit")
	}
	bin := t.TempDir()
	for _, name := range []string{"zed", "aider", "opencode", "claude", "codex"} {
		mustWriteFile(t, filepath.Join(bin, name), "#!/bin/sh\n")
		if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	for range 5 {
		var out bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&out)
		reportInstalledCLIs(cmd)

		var listed []string
		for _, line := range strings.Split(out.String(), "\n") {
			if name, _, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "✓ "), " →"); ok {
				listed = append(listed, name)
			}
		}
		if want := []string{"aider", "claude", "codex", "opencode", "zed"}; !slices.Equal(listed, want) {
			t.Fatalf("listed %v, want %v", listed, want)
		}
	}
}
