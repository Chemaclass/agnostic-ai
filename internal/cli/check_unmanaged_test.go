package cli

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCollectEntryPointDrift_IgnoresUnmanaged(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Pointer body\n")
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "hand-owned\n")
	cfg := &config.Config{Targets: []string{"claude", "codex"}}
	cfg.Sync.Unmanaged = []string{"CLAUDE.md"}

	rep, err := collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range append(rep.Missing, rep.Stale...) {
		if f.Path == "CLAUDE.md" {
			t.Errorf("user-owned CLAUDE.md reported as drift: %+v", rep)
		}
	}
	if len(rep.Missing) != 1 || rep.Missing[0].Path != "AGENTS.md" {
		t.Errorf("managed AGENTS.md should still be missing, got %+v", rep.Missing)
	}
}

func TestDoctor_PrintsUserOwnedSection(t *testing.T) {
	setupUnmanagedFixture(t)
	silence(t)
	captureLog(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	buf := &strings.Builder{}
	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor"})
	root.SetOut(buf)
	root.SetErr(io.Discard)

	if err := root.Execute(); err != nil {
		t.Errorf("doctor must not count a user-owned path as drift: %v\n%s", err, buf.String())
	}

	want := "User-owned (sync.unmanaged):\n  ~ " + unmanagedAgentPath + "\n"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("doctor output missing %q:\n%s", want, buf.String())
	}
}

func TestReportUnmanagedConfig_SkipsUserOwnedPaths(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "# Hand-written\n")
	mustWriteFile(t, filepath.Join(dir, ".cursor", "rules", "legacy.mdc"), "old rule\n")
	cfg := &config.Config{}
	cfg.Sync.Unmanaged = []string{"CLAUDE.md"}
	buf := &strings.Builder{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)

	n := reportUnmanagedConfig(cmd, dir, cfg)

	if n != 1 {
		t.Errorf("findings = %d, want 1:\n%s", n, buf.String())
	}
	if strings.Contains(buf.String(), "CLAUDE.md") {
		t.Errorf("user-owned CLAUDE.md listed as unmanaged config:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), ".cursor/rules/legacy.mdc") {
		t.Errorf("hand-authored cursor rule missing:\n%s", buf.String())
	}
}
