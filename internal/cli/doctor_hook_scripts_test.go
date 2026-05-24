package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReportDivergentHookScripts_FlagsDifferentBodies(t *testing.T) {
	root := t.TempDir()
	mustWriteHookScript(t, root, "claude", "protect-files.sh", "echo claude\n")
	mustWriteHookScript(t, root, "codex", "protect-files.sh", "echo codex (different)\n")

	cmd := newSilentCobra()
	drift, err := reportDivergentHookScripts(cmd, root)
	if err != nil {
		t.Fatal(err)
	}
	if !drift {
		t.Error("expected drift=true for divergent bodies")
	}
	out := cmd.OutOrStderr().(*bytes.Buffer).String()
	for _, want := range []string{
		"✗ divergent hook script: scripts/{claude,codex}/protect-files.sh",
		"claude  variant:",
		"codex   variant:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q in:\n%s", want, out)
		}
	}
}

func TestReportDivergentHookScripts_SilentOnIdenticalBodies(t *testing.T) {
	root := t.TempDir()
	body := "echo same\n"
	mustWriteHookScript(t, root, "claude", "protect-files.sh", body)
	mustWriteHookScript(t, root, "codex", "protect-files.sh", body)

	cmd := newSilentCobra()
	drift, err := reportDivergentHookScripts(cmd, root)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Error("expected drift=false for identical bodies")
	}
	out := cmd.OutOrStderr().(*bytes.Buffer).String()
	if !strings.Contains(out, "✓ no divergent hook scripts") {
		t.Errorf("expected clean line, got:\n%s", out)
	}
}

func TestReportDivergentHookScripts_NoScriptsDir(t *testing.T) {
	root := t.TempDir()

	cmd := newSilentCobra()
	drift, err := reportDivergentHookScripts(cmd, root)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Error("expected drift=false when .agnostic-ai/scripts/ is absent")
	}
	out := cmd.OutOrStderr().(*bytes.Buffer).String()
	if !strings.Contains(out, "(no per-tool hook scripts captured)") {
		t.Errorf("expected empty-stash notice, got:\n%s", out)
	}
}

// A script appearing under only one tool is not divergence — silent.
func TestReportDivergentHookScripts_SingleToolNotFlagged(t *testing.T) {
	root := t.TempDir()
	mustWriteHookScript(t, root, "codex", "lone.sh", "echo lone\n")

	cmd := newSilentCobra()
	drift, err := reportDivergentHookScripts(cmd, root)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Error("single-tool script should not register as divergence")
	}
}

func mustWriteHookScript(t *testing.T, root, tool, basename, body string) {
	t.Helper()
	dir := filepath.Join(root, agnosticScriptsDir, tool)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, basename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSilentCobra() *cobra.Command {
	c := &cobra.Command{}
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	return c
}
