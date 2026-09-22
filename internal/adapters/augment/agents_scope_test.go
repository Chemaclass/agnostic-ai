package augment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmitAgents_WritesOnlyNativeDefinitionsToSuppliedDirectory(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)
	agents := []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "Review changes", "tools": []any{"Read"}}, Body: "Find correctness bugs."}}
	// Keep both scopes on the same renderer, including tool translation.
	agentsDir := filepath.Join(root, "personal", "agents")
	if err := New().EmitAgents(emit.NewSession(), agents, agentsDir, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(agentsDir, "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Find correctness bugs.") || !strings.Contains(string(got), "description: Review changes") {
		t.Errorf("missing agent content:\n%s", got)
	}
	files := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files++
			if path != filepath.Join(agentsDir, "reviewer.md") {
				t.Errorf("unexpected output: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files != 1 {
		t.Errorf("wrote %d files, want one agent definition", files)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(agents), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	project, err := os.ReadFile(filepath.Join(root, ".augment/agents", "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(project) {
		t.Errorf("agent-only output differs from project output:\n%s\nproject:\n%s", got, project)
	}
}
