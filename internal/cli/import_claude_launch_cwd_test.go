package cli

import (
	"path/filepath"
	"testing"
)

// Claude Code documents `${workspaceFolder}` as the project root, so a
// hand-written cwd that starts with it imports as the portable
// project-relative path, and the project root itself as no cwd (#1400).
func TestImportFromClaude_StripsWorkspaceFolderFromCwd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "launch.json"), `{
  "version": "0.0.1",
  "configurations": [
    {"name": "Docs", "runtimeExecutable": "pnpm", "runtimeArgs": ["dev:mintlify"], "cwd": "${workspaceFolder}/apps/docs"},
    {"name": "Root", "runtimeExecutable": "npm", "runtimeArgs": ["start"], "cwd": "${workspaceFolder}"},
    {"name": "Slash", "runtimeExecutable": "npm", "runtimeArgs": ["test"], "cwd": "${workspaceFolder}/"},
    {"name": "Plain", "runtimeExecutable": "npm", "runtimeArgs": ["run", "web"], "cwd": "apps/web"},
    {"name": "Other", "runtimeExecutable": "npm", "runtimeArgs": ["run", "api"], "cwd": "${workspaceFolderBasename}/api"}
  ]
}`)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "dev.yaml"))
	want := `name: dev
dev-commands:
    - name: Docs
      command: pnpm dev:mintlify
      cwd: apps/docs
    - name: Root
      command: npm start
    - name: Slash
      command: npm test
    - name: Plain
      command: npm run web
      cwd: apps/web
    - name: Other
      command: npm run api
      cwd: ${workspaceFolderBasename}/api
`
	if got != want {
		t.Errorf("environments/dev.yaml:\n%s\nwant:\n%s", got, want)
	}
}
