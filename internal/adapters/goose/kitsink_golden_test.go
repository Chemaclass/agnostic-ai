package goose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for a
// goose sync over the canonical kit-sink bundle (3 rules + 3 skills)
// with the rules-file opted in, so the golden tree captures the
// `.goosehints` document alongside the unconditional skill folders
// (see TestEmit_NoRulesFileByDefault and
// TestEmit_Skill_WritesSkillFolderWithoutRulesFileOptIn in
// goose_test.go for the un-opted-in default).
//
// Diff regressions (merged-document reorder, heading drift) trip this
// test and must be acknowledged by either fixing the adapter or
// regenerating the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/goose/ -run TestKitSink_GoldenSnapshot
func TestKitSink_GoldenSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{"goose": {RulesFile: ".goosehints"}},
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	testutil.AssertGoldenTree(t, dir, expectedDir)
}
