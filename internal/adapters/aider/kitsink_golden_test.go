package aider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for an
// aider sync over the canonical kit-sink bundle (3 rules + 3 agents +
// 3 skills) with both legacy outputs opted in: rules-file at
// CONVENTIONS.md and conf-file at .aider.conf.yml.
//
// Diff regressions (frontmatter key reorder, agent section prefix
// drift, YAML key sort changes, conf-file header churn) trip this
// test and must be acknowledged by either fixing the adapter or
// regenerating the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/aider/ -run TestKitSink_GoldenSnapshot
func TestKitSink_GoldenSnapshot(t *testing.T) {
	// Resolve the package-relative testdata path before TempCwd
	// switches the working directory; otherwise the relative path
	// would target the temp dir.
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {
				RulesFile: "CONVENTIONS.md",
				ConfFile:  ".aider.conf.yml",
				Model:     "gpt-4o",
				WeakModel: "gpt-4o-mini",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	testutil.AssertGoldenTree(t, dir, expectedDir)
}
