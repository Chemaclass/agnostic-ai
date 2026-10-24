package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// demoProject copies the Authority demo project into a fresh directory
// and returns it, so a test never touches the checked-in copy.
func demoProject(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "demo-project")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "authority", "testdata", "demo-project"))); err != nil {
		t.Fatalf("copy demo project: %v", err)
	}
	return dir
}

func TestInstalledState_FreshCopyIsNotGenerated(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	state, err := installedState("claude", cfg, b, ".")
	if err != nil {
		t.Fatal(err)
	}
	if state.State != authority.InstalledNotGenerated || len(state.Files) == 0 || state.CheckedAt == "" {
		t.Fatalf("state = %+v", state)
	}
	for _, f := range state.Files {
		if f.State != authority.InstalledNotGenerated {
			t.Errorf("%s = %s on a fresh copy", f.Path, f.State)
		}
	}
	if _, err := os.Stat(".claude"); !os.IsNotExist(err) {
		t.Errorf("comparing wrote generated files: %v", err)
	}
}
