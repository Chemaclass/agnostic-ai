package cline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestGlobalHooks_GoldenSnapshot(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	hooks := []spec.Entry{
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "echo guard"}},
		{Kind: spec.KindHook, Name: "second", Meta: map[string]any{"event": "pretooluse", "command": "echo second"}},
	}
	files := GlobalHookFiles(hooks, "/cline-global/hook-scripts")
	if len(files) != 1 || files[0].Path != "PreToolUse" {
		t.Fatalf("hooks must share the canonical event file: %+v", files)
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(out, file.Path), file.Body, file.Mode); err != nil {
			t.Fatal(err)
		}
	}
	testutil.AssertGoldenTree(t, out, filepath.Join(root, "testdata", "global-hooks"))
}
