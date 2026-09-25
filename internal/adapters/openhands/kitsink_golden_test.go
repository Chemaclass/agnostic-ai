package openhands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for an
// openhands sync over the canonical kit-sink bundle (4 rules + 3
// skills): r1-r3 stay off disk (always-on, they reach OpenHands only
// through the shared AGENTS.md entry-point), while r4 (path-triggered,
// `globs: src/**`) lands at `.agents/skills/r4/SKILL.md`.
//
// Diff regressions (frontmatter key reorder, skill folder layout
// drift) trip this test and must be acknowledged by either fixing the
// adapter or regenerating the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/openhands/ -run TestKitSink_GoldenSnapshot
func TestKitSink_GoldenSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	testutil.AssertGoldenTree(t, dir, expectedDir)
}
