package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const overviewMarker = adapters.OverviewStartMarker

func TestWriteAgnosticEntryPoints_TargetOverviewAppendsAppendix(t *testing.T) {
	dir := testutil.TempCwd(t)
	custom := "# Project\n\nMy instructions.\n"
	writeAgnosticFile(t, custom)
	cfg := &config.Config{
		Targets: []string{"claude"},
		Sync:    config.SyncConfig{TargetOverview: true},
		Outputs: map[string]config.Output{
			"claude": {RulesDir: "custom/rules"},
		},
	}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not written: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, custom) {
		t.Errorf("CLAUDE.md missing canonical body:\n%s", got)
	}
	if !strings.Contains(got, overviewMarker) {
		t.Errorf("CLAUDE.md missing overview appendix:\n%s", got)
	}
	if !strings.Contains(got, "`custom/rules/`") {
		t.Errorf("appendix does not honor outputs.claude.rules-dir override:\n%s", got)
	}

	// The canonical source file never carries the appendix.
	src, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), overviewMarker) {
		t.Errorf("AGNOSTIC_AI.md must not carry the overview appendix:\n%s", src)
	}
}

func TestWriteAgnosticEntryPoints_TargetOverviewOffByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n")
	cfg := &config.Config{Targets: []string{"claude"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), overviewMarker) {
		t.Errorf("appendix written without sync.target-overview:\n%s", data)
	}
}

func TestWriteAgnosticEntryPoints_TargetOverviewSharedPathListsEachConsumer(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n")
	cfg := &config.Config{
		Targets: []string{"codex", "amp", "warp"},
		Sync:    config.SyncConfig{TargetOverview: true},
	}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not written: %v", err)
	}
	got := string(data)
	for _, heading := range []string{"### codex", "### amp", "### warp"} {
		if !strings.Contains(got, heading) {
			t.Errorf("AGENTS.md missing %q section:\n%s", heading, got)
		}
	}
	if !strings.Contains(got, "`.codex/agents/`") {
		t.Errorf("AGENTS.md missing codex agents location:\n%s", got)
	}
}

// Sync write and drift check must agree byte-for-byte on entry-point
// content, appendix included, so a freshly synced tree reports no drift.
func TestCollectEntryPointDrift_NoDriftAfterSyncWithOverview(t *testing.T) {
	testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nBody.\n")
	cfg := &config.Config{
		Targets: []string{"claude", "codex"},
		Sync:    config.SyncConfig{TargetOverview: true},
	}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rep, err := collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.hasDrift() {
		t.Errorf("freshly synced tree reports drift: missing=%v stale=%v",
			paths(rep.Missing), paths(rep.Stale))
	}
}

func TestMirrorMainFile_StripsTargetOverview(t *testing.T) {
	dir := testutil.TempCwd(t)
	body := "# Project\n\nBody.\n"
	appendix := adapters.RenderTargetOverview([]adapters.TargetArtifacts{{
		Target:    "claude",
		Artifacts: []adapters.NativeArtifact{{Label: "Rules", Location: ".claude/rules/"}},
	}})
	withAppendix := adapters.AppendTargetOverview(body, appendix)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(withAppendix), 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, err := mirrorMainFile(dir, "CLAUDE.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wrote {
		t.Fatal("mirrorMainFile reported nothing written")
	}
	data, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), overviewMarker) {
		t.Errorf("AGNOSTIC_AI.md still carries the overview appendix:\n%s", data)
	}
	if !strings.Contains(string(data), "Body.") {
		t.Errorf("AGNOSTIC_AI.md lost the canonical body:\n%s", data)
	}
}

func paths(files []adapters.CapturedFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}
