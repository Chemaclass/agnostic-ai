package cli

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestWatchAnchor_RejectsJunctionToProjectParent(t *testing.T) {
	workspace := testutil.TempCwd(t)
	project := filepath.Join(workspace, "real", "project")
	writeTestFile(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\n")
	alias := filepath.Join(workspace, "alias")
	if err := createImportSourceAlias(filepath.Dir(project), alias); err != nil {
		t.Fatal(err)
	}
	if got, ok := watchAnchor(project, filepath.Join(alias, "rules")); ok {
		t.Errorf("junction to project parent was accepted as anchor: %s", got)
	}
}
