package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func readJSONDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

// An environment spec's `setup` reaches .cursor/worktrees.json and stays
// out of environment.json (#1339).
func TestEmit_EnvironmentSetupWritesWorktreesJSON(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "kombo", Path: "environments/kombo.yaml",
		Meta: map[string]any{
			"name":          "kombo",
			"install":       "pnpm install",
			"setup":         "bash scripts/setup-worktree.bash",
			"setup-windows": []any{"npm ci", "copy .env.example .env"},
		},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	worktrees := readJSONDoc(t, filepath.Join(cwd, ".cursor", "worktrees.json"))
	if got, _ := json.Marshal(worktrees["setup-worktree-unix"]); string(got) != `["bash scripts/setup-worktree.bash"]` {
		t.Errorf("setup-worktree-unix = %s", got)
	}
	if got, _ := json.Marshal(worktrees["setup-worktree-windows"]); string(got) != `["npm ci","copy .env.example .env"]` {
		t.Errorf("setup-worktree-windows = %s", got)
	}

	env := readJSONDoc(t, filepath.Join(cwd, ".cursor", "environment.json"))
	for _, key := range []string{"setup", "setup-windows"} {
		if _, ok := env[key]; ok {
			t.Errorf("environment.json carries %q", key)
		}
	}
	if env["install"] != "pnpm install" {
		t.Errorf("install = %v", env["install"])
	}
}

// A spec with only setup keys writes no environment.json.
func TestEmit_EnvironmentSetupOnlyWritesNoEnvironmentJSON(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "wt", Path: "environments/wt.yaml",
		Meta: map[string]any{"name": "wt", "setup": "make setup"},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".cursor", "environment.json")); !os.IsNotExist(err) {
		t.Errorf("environment.json written for a setup-only spec: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".cursor", "worktrees.json")); err != nil {
		t.Errorf("worktrees.json missing: %v", err)
	}
}
