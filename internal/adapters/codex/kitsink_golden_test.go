package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for a
// codex sync over the canonical kit-sink bundle. Every supported spec
// kind is exercised with 3+ specimens, hooks span multiple lifecycle
// events, MCPs cover stdio + http + disabled, and the legacy rules
// file is opted in via outputs.codex.rules-file so all six supported
// kinds land at observable paths.
//
// Diff regressions (frontmatter key reorder, hook ordering churn,
// MCP table drift, exec-policy rendering changes) trip the test and
// must be acknowledged by either fixing the adapter or regenerating
// the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/codex/ -run TestKitSink_GoldenSnapshot
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
			// commands-dir opts into the deprecated project prompts
			// layout so the snapshot keeps covering command emission.
			"codex": {RulesFile: "AGENTS-rules.md", CommandsDir: ".codex/prompts"},
		},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	testutil.AssertGoldenTree(t, dir, expectedDir, "AGENTS.md")
}
