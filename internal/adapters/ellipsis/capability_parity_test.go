package ellipsis

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// kitSinkBundle exercises every kind the adapter declares: a root and a
// scoped review plus the settings spec that names the reviewer. Rules,
// agents, and skills are left out on purpose; Ellipsis reads none of them
// from this file.
func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		review("root", "", "Check the changed code for correctness."),
		review("backend", "backend", "Check that handlers use a repository."),
		settings(map[string]any{
			"pull_requests": map[string]any{"draft": false},
			"reviewer":      reviewer(nil),
			"output":        map[string]any{"on_fail": "request_changes"},
			"budget":        map[string]any{"session": 3},
		}),
	})
}

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	paths := testutil.WalkRel(t, dir)
	for _, k := range caps.Supports {
		found := false
		for _, p := range paths {
			if filepath.ToSlash(p) == reviewFile {
				found = true
			}
		}
		if !found {
			t.Errorf("declared kind %q in caps.Supports has no observable output (paths: %v)", k, paths)
		}
	}
	if len(caps.Supports) != 2 {
		t.Errorf("claim only the kinds this adapter writes, got %v", caps.Supports)
	}
}

func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings, got %d", got)
	}
}

func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "stdio", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt"}},
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "skill"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 3 {
		t.Errorf("expected 3 capability warnings (mcp, hook, skill), got %d", got)
	}
}

func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	checked := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !header.Has(string(data)) || !strings.HasPrefix(string(data), "# ") {
			t.Errorf("missing provenance header in %s", path)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("kit-sink bundle emitted nothing")
	}
}
