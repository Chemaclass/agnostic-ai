package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func seedLegacyEntryPoint(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, legacyEntryPointFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A project synced before #623 carries a managed `.opencode/AGENTS.md`
// OpenCode never opens. Sync sweeps it once the entry point moves to
// the root AGENTS.md, so the tree does not keep two copies.
func TestEmit_SweepsLegacyEntryPoint(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := seedLegacyEntryPoint(t, dir, header.With("# Pointer body\n", header.FormatMarkdown))

	if err := New().Emit(emit.NewSession(), spec.Bundle{}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stale managed .opencode/AGENTS.md survived the sweep, err=%v", err)
	}
}

// The sweep is header-guarded: a hand-authored file at the same path is
// the user's, not a leftover of ours.
func TestEmit_LeavesHandAuthoredLegacyEntryPoint(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := seedLegacyEntryPoint(t, dir, "# Mine\n")

	if err := New().Emit(emit.NewSession(), spec.Bundle{}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("hand-authored .opencode/AGENTS.md must survive: %v", err)
	}
}

// Pointing `outputs.opencode.file` back at the old path is an explicit
// opt-in, so the sweep must not delete a live write.
func TestEmit_KeepsLegacyEntryPointWhenFileOverridePointsThere(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := seedLegacyEntryPoint(t, dir, header.With("# Pointer body\n", header.FormatMarkdown))

	cfg := &config.Config{Outputs: map[string]config.Output{"opencode": {File: legacyEntryPointFile}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("opted-in .opencode/AGENTS.md must survive: %v", err)
	}
}
