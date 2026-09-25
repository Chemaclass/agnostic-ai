package augment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for an
// augment sync over the canonical kit-sink bundle (3 rules) with the
// rules-file opted in, so the golden tree captures actual content
// rather than an empty directory (see TestEmit_NoFilesByDefault in
// augment_test.go for the un-opted-in default).
//
// Diff regressions (merged-document reorder, heading drift) trip this
// test and must be acknowledged by either fixing the adapter or
// regenerating the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/augment/ -run TestKitSink_GoldenSnapshot
func TestKitSink_GoldenSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{"augment": {RulesFile: ".augment-guidelines"}},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	testutil.AssertGoldenTree(t, dir, expectedDir)
}
