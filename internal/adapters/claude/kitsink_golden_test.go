package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for
// a claude sync over the canonical kit-sink bundle. Every supported
// spec kind is exercised with 3+ specimens, hooks span multiple
// lifecycle events, MCPs cover stdio + http + disabled-with-command.
//
// Diff regressions (frontmatter key reorder, settings.json indent
// drift, hook event ordering churn, MCP key sort changes) trip
// this test and must be acknowledged by either fixing the adapter
// or regenerating the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/claude/ -run TestKitSink_GoldenSnapshot
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

	testutil.AssertGoldenTree(t, dir, expectedDir, "CLAUDE.md")
}
