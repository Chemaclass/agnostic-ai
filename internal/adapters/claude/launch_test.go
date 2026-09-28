package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An environment spec's dev-commands become .claude/launch.json preview
// servers (#1340).
func TestEmit_DevCommandsWriteLaunchJSON(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "dev", Path: "environments/dev.yaml",
		Meta: map[string]any{
			"name": "dev",
			"dev-commands": []any{
				map[string]any{"name": "Dashboard", "command": "bash scripts/run-preview.bash", "port": 5555, "auto-port": true},
				map[string]any{"name": "Docs", "command": []any{"pnpm", "dev:mintlify"}, "port": 3000, "cwd": "apps/docs",
					"env": map[string]any{"NODE_ENV": "development"}},
				map[string]any{"name": "Watch", "command": "pnpm build --watch | tee build.log"},
			},
		},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(cwd, ".claude", "launch.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "configurations": [
    {
      "autoPort": true,
      "name": "Dashboard",
      "port": 5555,
      "runtimeArgs": [
        "scripts/run-preview.bash"
      ],
      "runtimeExecutable": "bash"
    },
    {
      "cwd": "apps/docs",
      "env": {
        "NODE_ENV": "development"
      },
      "name": "Docs",
      "port": 3000,
      "runtimeArgs": [
        "dev:mintlify"
      ],
      "runtimeExecutable": "pnpm"
    },
    {
      "name": "Watch",
      "runtimeArgs": [
        "-c",
        "pnpm build --watch | tee build.log"
      ],
      "runtimeExecutable": "sh"
    }
  ],
  "version": "0.0.1"
}
`
	if string(got) != want {
		t.Errorf("launch.json:\n%s\nwant:\n%s", got, want)
	}
}

// A spec with no dev-commands writes no launch.json.
func TestEmit_NoDevCommandsNoLaunchJSON(t *testing.T) {
	cwd := t.TempDir()
	testutil.Chdir(t, cwd)
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindEnvironment, Name: "dev", Path: "environments/dev.yaml",
		Meta: map[string]any{"name": "dev", "install": "npm ci"},
	}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".claude", "launch.json")); !os.IsNotExist(err) {
		t.Errorf("launch.json written: %v", err)
	}
}

// Scalar env values and a numeric port string reach launch.json, since
// YAML reads `PORT: 3000` as a number.
func TestEmit_DevCommandScalarEnvAndPortString(t *testing.T) {
	got := launchConfiguration(map[string]any{
		"name": "web", "command": "npm start", "port": "4000",
		"env": map[string]any{"PORT": 3000, "DEBUG": true, "NAME": "x"},
	})
	env, _ := got["env"].(map[string]string)
	if env["PORT"] != "3000" || env["DEBUG"] != "true" || env["NAME"] != "x" {
		t.Errorf("env = %v", got["env"])
	}
	if got["port"] != 4000 {
		t.Errorf("port = %v", got["port"])
	}
}
