package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An environment spec's setup, cleanup, and dev-commands become the
// [setup] and [cleanup] scripts and the [[actions]] of the Codex app's
// local environment (#1393).
func TestEmit_EnvironmentWritesSetupCleanupAndActions(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "Acme", Path: "environments/acme.yaml",
		Meta: map[string]any{
			"name":          "Acme",
			"setup":         "bash scripts/setup-worktree.bash",
			"setup-windows": []any{"npm ci", "copy .env.example .env"},
			"cleanup":       "bash scripts/stop-preview.bash",
			"dev-commands": []any{
				map[string]any{"name": "Dashboard", "command": "bash scripts/run-preview.bash"},
				map[string]any{"name": "Tests", "icon": "test", "command": []any{"go", "test", "./..."}},
				map[string]any{"name": "Docs", "command": []any{"pnpm", "--filter", "my docs", "dev"}},
			},
		},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".codex", "environments", "environment.toml"))
	if err != nil {
		t.Fatalf("read environment.toml: %v", err)
	}
	if !header.Has(string(raw)) {
		t.Errorf("no provenance header:\n%s", raw)
	}
	got := strings.TrimLeft(header.Strip(string(raw)), "\n")
	want := `version = 1
name = "Acme"

[setup]
script = "bash scripts/setup-worktree.bash"

[setup.win32]
script = """
npm ci
copy .env.example .env
"""

[cleanup]
script = "bash scripts/stop-preview.bash"

[[actions]]
name = "Dashboard"
icon = "run"
command = "bash scripts/run-preview.bash"

[[actions]]
name = "Tests"
icon = "test"
command = "go test ./..."

[[actions]]
name = "Docs"
icon = "run"
command = "pnpm --filter 'my docs' dev"
`
	if got != want {
		t.Errorf("environment.toml:\n%s\nwant:\n%s", got, want)
	}
}

// The last spec that sets a field wins, and outputs.codex.environment-file
// moves the file.
func TestEmit_EnvironmentLastSpecWinsAndPathOverrides(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"codex": {EnvironmentFile: "env/local.toml"}}}
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindEnvironment, Name: "a", Meta: map[string]any{"name": "a", "setup": "make a", "cleanup": "make clean"}},
		{Kind: spec.KindEnvironment, Name: "b", Meta: map[string]any{"name": "b", "setup": "make b"}},
	})
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "env", "local.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name = "b"`, `script = "make b"`, `script = "make clean"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in:\n%s", want, raw)
		}
	}
	if strings.Contains(string(raw), "make a") {
		t.Errorf("an earlier spec's setup survived:\n%s", raw)
	}
}

// A spec with nothing Codex can run writes no file.
func TestEmit_EnvironmentWithoutScriptsOrActionsWritesNoFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "cloud", Meta: map[string]any{"name": "cloud", "install": "npm ci"},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex", "environments")); !os.IsNotExist(err) {
		t.Errorf(".codex/environments written for an install-only spec: %v", err)
	}
}

// A dev command's cwd becomes a `cd` before the action's command, since a
// Codex action has no cwd field and runs from the project root.
func TestEmit_EnvironmentActionRunsFromCwd(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "dev",
		Meta: map[string]any{"dev-commands": []any{
			map[string]any{"name": "Docs", "command": "pnpm dev:docs", "cwd": "apps/docs"},
			map[string]any{"name": "Site", "command": "npm start", "cwd": "my site"},
			map[string]any{"name": "Root", "command": "make dev", "cwd": "."},
			map[string]any{"name": "Seed", "command": "npm ci\nnpm run seed", "cwd": "api"},
		}},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".codex", "environments", "environment.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`command = "cd apps/docs && pnpm dev:docs"`,
		`command = "cd 'my site' && npm start"`,
		`command = "make dev"`,
		"command = \"\"\"\ncd api || exit 1\nnpm ci\nnpm run seed\n\"\"\"",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in:\n%s", want, raw)
		}
	}
}
