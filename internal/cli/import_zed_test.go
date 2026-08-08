package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromZed_NoSources(t *testing.T) {
	dir := t.TempDir()
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when .rules absent: %v", err)
	}
}

func TestImportFromZed_MirrorsRulesFile(t *testing.T) {
	dir := t.TempDir()
	body := "# Project rules\n\n## rule-a\n\nbody.\n"
	writeFile(t, filepath.Join(dir, zedMainFile), body)
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical. got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "rule-a.md")); err != nil {
		t.Errorf("missing sliced rule rule-a.md: %v", err)
	}
}

func TestImportFromZed_ImportsTasksAsHooks(t *testing.T) {
	dir := t.TempDir()
	tasks := `[
  {"label": "fmt — gofmt the repo", "command": "sh", "args": ["-c", "gofmt -w ."]}
]`
	writeFile(t, filepath.Join(dir, zedTasksFile), tasks)
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hooks", "fmt.yaml"))
	if err != nil {
		t.Fatalf("missing hooks/fmt.yaml: %v", err)
	}
	for _, want := range []string{"event: OnDemand", "command: gofmt -w .", "description: gofmt the repo"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in hook file: %s", want, got)
		}
	}
}

// Round-trip for #539: task fields beyond label/command/args must
// survive an import under x-zed so a sync -> import -> sync cycle does
// not silently drop them.
func TestImportFromZed_TaskExtraFieldsRoundTripUnderXZed(t *testing.T) {
	dir := t.TempDir()
	tasks := `[
  {"label": "fmt", "command": "sh", "args": ["-c", "gofmt -w ."], "cwd": "$ZED_WORKTREE_ROOT", "shell": "bash"}
]`
	writeFile(t, filepath.Join(dir, zedTasksFile), tasks)
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hooks", "fmt.yaml"))
	if err != nil {
		t.Fatalf("missing hooks/fmt.yaml: %v", err)
	}
	for _, want := range []string{"x-zed:", "cwd: $ZED_WORKTREE_ROOT", "shell: bash"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in hook file: %s", want, got)
		}
	}
}

// A task with only the known fields must not gain an empty x-zed block.
func TestImportFromZed_NoXZedBlockForPlainTask(t *testing.T) {
	dir := t.TempDir()
	tasks := `[{"label": "fmt", "command": "sh", "args": ["-c", "gofmt -w ."]}]`
	writeFile(t, filepath.Join(dir, zedTasksFile), tasks)
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hooks", "fmt.yaml"))
	if err != nil {
		t.Fatalf("missing hooks/fmt.yaml: %v", err)
	}
	if strings.Contains(string(got), "x-zed") {
		t.Errorf("unexpected x-zed block for a plain task:\n%s", got)
	}
}

func TestImportFromZed_ImportsContextServers(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "context_servers": {
    "fs": {"command": {"path": "fs-server", "args": ["--root", "."]}, "settings": {}}
  }
}`
	writeFile(t, filepath.Join(dir, zedSettings), settings)
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/fs.yaml: %v", err)
	}
	for _, want := range []string{"command: fs-server", "args:", "--root"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in mcp file: %s", want, got)
		}
	}
}

// TestImportFromZed_ImportsFlatStdioContextServer covers the shape Zed
// documents and the shape this repo's own zed adapter emits: `command`
// as a plain string with `args` and `env` as siblings.
//
// The importer previously accepted only a nested `command: {path, args,
// env}` object, so importing a `.zed/settings.json` that agnostic-ai had
// just written silently dropped every stdio server (#546).
func TestImportFromZed_ImportsFlatStdioContextServer(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "context_servers": {
    "fs": {"command": "fs-server", "args": ["--root", "."], "env": {"TOKEN": "x"}}
  }
}`
	writeFile(t, filepath.Join(dir, zedSettings), settings)
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/fs.yaml: %v", err)
	}
	for _, want := range []string{"command: fs-server", "args:", "--root", "TOKEN"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in mcp file: %s", want, got)
		}
	}
}

// TestImportFromZed_ImportsRemoteContextServer covers Zed's remote shape
// (`url` plus optional `headers`). The importer required a `command` key
// and skipped anything without one, so remote servers were dropped too.
func TestImportFromZed_ImportsRemoteContextServer(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "context_servers": {
    "remote": {"url": "https://example.test/mcp", "headers": {"Authorization": "Bearer t"}}
  }
}`
	writeFile(t, filepath.Join(dir, zedSettings), settings)
	if err := importFromZed(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "remote.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/remote.yaml: %v", err)
	}
	for _, want := range []string{"https://example.test/mcp", "Authorization"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in mcp file: %s", want, got)
		}
	}
}
