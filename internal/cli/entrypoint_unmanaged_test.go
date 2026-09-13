package cli

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestWriteAgnosticEntryPoints_UnmanagedFileKeptAndNoWarning(t *testing.T) {
	dir := testutil.TempCwd(t)
	const mine = "# My CLAUDE.md\n\nKeep me.\n"
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), mine)
	writeAgnosticFile(t, "# Pointer body\n")
	buf := captureLog(t)
	cfg := &config.Config{Targets: []string{"claude"}}
	cfg.Sync.Unmanaged = []string{"CLAUDE.md"}
	sess := adapters.NewSession()
	sess.SetUnmanaged(cfg.Sync.Unmanaged)

	if err := writeAgnosticEntryPoints(sess, cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(dir, "CLAUDE.md")); got != mine {
		t.Errorf("user-owned CLAUDE.md rewritten:\n%s", got)
	}
	if strings.Contains(buf.String(), "appears hand-authored") {
		t.Errorf("no hand-authored warning expected for a user-owned file, got:\n%s", buf.String())
	}
}

func TestEntryPointPaths_ExcludesUnmanaged(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude", "codex"}}
	cfg.Sync.Unmanaged = []string{"CLAUDE.md"}

	got := entryPointPaths(cfg, cfg.Targets)

	want := []string{adapters.AgnosticEntryPointPath, "AGENTS.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entryPointPaths = %q, want %q", got, want)
	}
}
