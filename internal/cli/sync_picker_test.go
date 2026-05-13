package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestTargetsMatchDefault_FullSet(t *testing.T) {
	if !targetsMatchDefault(allTargetNames()) {
		t.Error("full target list should match defaults")
	}
}

func TestTargetsMatchDefault_PartialSet(t *testing.T) {
	if targetsMatchDefault([]string{"claude", "codex"}) {
		t.Error("partial target list should not match defaults")
	}
}

func TestTargetsMatchDefault_OrderInsensitive(t *testing.T) {
	reversed := append([]string(nil), allTargetNames()...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	if !targetsMatchDefault(reversed) {
		t.Error("reversed default list should still match")
	}
}

func TestShouldPromptTargetSelection_FirstRunWithDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Targets: allTargetNames()}
	if !shouldPromptTargetSelection(dir, cfg) {
		t.Error("should prompt on first run with default targets")
	}
}

func TestShouldPromptTargetSelection_StateFilePresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFilePath(dir), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Targets: allTargetNames()}
	if shouldPromptTargetSelection(dir, cfg) {
		t.Error("should not prompt when .sync-state exists")
	}
}

func TestShouldPromptTargetSelection_TargetsNarrowed(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Targets: []string{"claude", "cursor"}}
	if shouldPromptTargetSelection(dir, cfg) {
		t.Error("should not prompt when targets already narrowed")
	}
}

func TestSelectTargetsForSync_NonTTYNoData(t *testing.T) {
	picked, err := selectTargetsForSync(bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if picked != nil {
		t.Errorf("expected nil (silent fallback), got %v", picked)
	}
}

func TestSelectTargetsForSync_PipedReader(t *testing.T) {
	in := strings.NewReader("claude,codex\n")
	picked, err := selectTargetsForSync(in, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "codex"}
	if !equalStringSlices(picked, want) {
		t.Errorf("got %v, want %v", picked, want)
	}
}

func TestSelectTargetsForSync_PipedUnknownTarget(t *testing.T) {
	in := strings.NewReader("claude,bogus\n")
	_, err := selectTargetsForSync(in, &bytes.Buffer{})
	if err == nil {
		t.Error("expected error for unknown target")
	}
}

func TestFirstSyncTargetSelection_PipedPersists(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ntargets:\n"+listTargets(allTargetNames())), 0o644); err != nil {
		t.Fatal(err)
	}

	in := strings.NewReader("claude,cursor\n")
	out := &bytes.Buffer{}
	picked, err := firstSyncTargetSelection(".", in, out)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "cursor"}
	if !equalStringSlices(picked, want) {
		t.Errorf("got %v, want %v", picked, want)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if !strings.Contains(string(data), "targets:\n  - claude\n  - cursor\n") {
		t.Errorf("config not persisted, got:\n%s", data)
	}
	if !strings.Contains(out.String(), "saved 2 target(s)") {
		t.Errorf("expected confirmation in stdout, got: %s", out.String())
	}
}

func TestFirstSyncTargetSelection_NonTTYSilentFallback(t *testing.T) {
	dir := setupFixture(t)
	picked, err := firstSyncTargetSelection(dir, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if picked != nil {
		t.Errorf("expected nil for silent fallback, got %v", picked)
	}
}

func TestSync_AllFlagSkipsPrompt(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ntargets:\n"+listTargets(allTargetNames())), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if !strings.Contains(string(data), "- claude") || !strings.Contains(string(data), "- opencode") {
		t.Errorf("--all should not narrow targets, got:\n%s", data)
	}
}

func listTargets(targets []string) string {
	var sb strings.Builder
	for _, t := range targets {
		sb.WriteString("  - ")
		sb.WriteString(t)
		sb.WriteString("\n")
	}
	return sb.String()
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
