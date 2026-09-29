package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		Kind: spec.KindEnvironment, Name: "acme", Path: "environments/acme.yaml",
		Meta: map[string]any{
			"name":          "acme",
			"install":       "pnpm install",
			"setup":         "bash scripts/setup-worktree.bash",
			"setup-windows": []any{"npm ci", "copy .env.example .env"},
		},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	worktrees := readJSONDoc(t, filepath.Join(cwd, ".cursor", "worktrees.json"))
	if got, _ := json.Marshal(worktrees["setup-worktree"]); string(got) != `["bash scripts/setup-worktree.bash"]` {
		t.Errorf("setup-worktree = %s", got)
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

// A script path set under x-cursor reaches worktrees.json as written, so
// Cursor still resolves it from .cursor/.
func TestEmit_EnvironmentKeepsWorktreeScriptPath(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "worktree", Path: "environments/worktree.yaml",
		Meta: map[string]any{
			"name":          "worktree",
			"setup-windows": "npm ci",
			"x-cursor":      map[string]any{"setup-worktree-unix": "setup-worktree-unix.sh"},
		},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(cwd, ".cursor", "worktrees.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"setup-worktree-unix\": \"setup-worktree-unix.sh\",\n  \"setup-worktree-windows\": [\n    \"npm ci\"\n  ]\n}\n"
	if string(got) != want {
		t.Errorf("worktrees.json:\n%s\nwant:\n%s", got, want)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".cursor", "environment.json")); !os.IsNotExist(err) {
		t.Errorf("environment.json written for setup keys only: %v", err)
	}
}

// A later spec that sets `setup` empty clears it: the last spec wins.
func TestEmit_EnvironmentLaterEmptySetupClears(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindEnvironment, Name: "a", Path: "environments/a.yaml",
			Meta: map[string]any{"setup": "make setup", "setup-windows": "npm ci"}},
		{Kind: spec.KindEnvironment, Name: "b", Path: "environments/b.yaml",
			Meta: map[string]any{"setup": []any{}}},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	doc := readJSONDoc(t, filepath.Join(cwd, ".cursor", "worktrees.json"))
	if _, ok := doc["setup-worktree"]; ok {
		t.Errorf("setup-worktree kept after a later empty setup: %v", doc)
	}
	if doc["setup-worktree-windows"] == nil {
		t.Errorf("setup-worktree-windows lost: %v", doc)
	}
}

// dev-commands stay out of environment.json and get a note, so a project
// that also targets Claude Code knows Cursor does not run them.
func TestEmit_EnvironmentNotesDevCommands(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "dev", Path: "environments/dev.yaml",
		Meta: map[string]any{"install": "npm ci", "dev-commands": []any{map[string]any{"name": "web", "command": "npm start"}}},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`dev-commands` on 1 environment has no effect on cursor") {
		t.Errorf("no dev-commands note:\n%s", buf.String())
	}
	env := readJSONDoc(t, filepath.Join(cwd, ".cursor", "environment.json"))
	if _, ok := env["dev-commands"]; ok {
		t.Errorf("environment.json carries dev-commands")
	}
}
