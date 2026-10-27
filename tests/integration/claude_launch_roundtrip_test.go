package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A hand-written .claude/launch.json with a url-only configuration, which
// attaches the preview to a server already running, survives import and
// sync byte for byte, and lint and `sync --check` stay clean (#2579).
func TestClaudeLaunchRoundTrip_URLOnlyConfigurationStaysUnchanged(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  environments: .agnostic-ai/environments
targets:
  - claude
gitignore:
  enabled: false
`), 0o644))
	launch := `{
  "configurations": [
    {
      "name": "app",
      "url": "https://app.localhost:3000"
    },
    {
      "name": "web",
      "port": 3000,
      "runtimeArgs": [
        "run",
        "dev"
      ],
      "runtimeExecutable": "npm"
    }
  ],
  "version": "0.0.1"
}
`
	launchPath := filepath.Join(dir, ".claude", "launch.json")
	must(t, os.MkdirAll(filepath.Dir(launchPath), 0o755))
	must(t, os.WriteFile(launchPath, []byte(launch), 0o644))

	runCmd(t, "import", "claude")
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "environments", "dev.yaml")); err != nil {
		t.Fatalf("import wrote no environment spec: %v", err)
	}

	runCmd(t, "sync", "-t", "claude")
	got, err := os.ReadFile(launchPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != launch {
		t.Errorf("launch.json after import and sync:\n%s\nwant:\n%s", got, launch)
	}
	runCmd(t, "sync", "--check")
	runCmd(t, "lint")
}
