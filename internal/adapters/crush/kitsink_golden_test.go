package crush

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for a
// crush sync over the canonical kit-sink bundle (3 rules + 3 skills +
// 3 MCPs), even though rules never land on disk through this adapter.
//
// Diff regressions (frontmatter key reorder, MCP transport rendering
// drift, JSON merge churn) trip this test and must be acknowledged by
// either fixing the adapter or regenerating the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/crush/ -run TestKitSink_GoldenSnapshot
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
