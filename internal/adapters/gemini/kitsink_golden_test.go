package gemini

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestKitSink_GoldenSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"gemini": {
				RulesFile:            "GEMINI-rules.md",
				EmitSkillsAsCommands: true,
			},
		},
	}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.RemoveAll(expectedDir); err != nil {
			t.Fatalf("clean expected dir: %v", err)
		}
		if err := testutil.CopyEmittedTree(dir, expectedDir); err != nil {
			t.Fatalf("copy: %v", err)
		}
		t.Logf("kit-sink golden updated: %s", expectedDir)
		return
	}

	got := testutil.WalkRel(t, dir)
	want, err := testutil.LoadExpectedTree(expectedDir)
	if err != nil {
		t.Fatalf("load expected: %v", err)
	}

	gotSet := map[string]string{}
	for _, rel := range got {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		gotSet[rel] = string(data)
	}

	for rel, wantBody := range want {
		gotBody, ok := gotSet[rel]
		if !ok {
			t.Errorf("missing expected output %s (run UPDATE_GOLDEN=1 to accept)", rel)
			continue
		}
		if gotBody != wantBody {
			t.Errorf("content mismatch: %s\n--- want ---\n%s\n--- got ---\n%s",
				rel, wantBody, gotBody)
		}
	}
	for rel := range gotSet {
		if _, ok := want[rel]; !ok {
			t.Errorf("unexpected output file: %s (run UPDATE_GOLDEN=1 to accept)", rel)
		}
	}
}
