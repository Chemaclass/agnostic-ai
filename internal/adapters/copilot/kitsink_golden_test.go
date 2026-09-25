package copilot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for
// a copilot sync over the canonical kit-sink bundle (3 rules + 3
// agents + 3 skills + 3 MCPs) with chatmodes-dir opted in.
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/copilot/ -run TestKitSink_GoldenSnapshot
func TestKitSink_GoldenSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"copilot": {ChatmodesDir: ".github/chatmodes"},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	testutil.AssertGoldenTree(t, dir, expectedDir)
}
