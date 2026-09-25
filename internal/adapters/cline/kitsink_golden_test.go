package cline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for
// a cline sync over the canonical kit-sink bundle (3 rules + 3
// agents + 3 skills) with workflows-dir opted in so the workflow
// emit branch lands at an observable path.
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/cline/ -run TestKitSink_GoldenSnapshot
func TestKitSink_GoldenSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"cline": {WorkflowsDir: recommendedWorkflowsDir},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	testutil.AssertGoldenTree(t, dir, expectedDir)
}
