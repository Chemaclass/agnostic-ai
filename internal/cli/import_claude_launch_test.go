package cli

import (
	"path/filepath"
	"testing"
)

// A hand-written .claude/launch.json becomes an environment spec's
// dev-commands (#1340).
func TestImportFromClaude_ReadsLaunchJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "launch.json"), `{
  // preview servers
  "version": "0.0.1",
  "autoVerify": false,
  "configurations": [
    {"name": "Dashboard", "runtimeExecutable": "bash", "runtimeArgs": ["scripts/run-preview.bash"], "port": 5555, "autoPort": true},
    {"name": "Docs", "runtimeExecutable": "pnpm", "runtimeArgs": ["dev:mintlify"], "port": 3000, "cwd": "apps/docs", "env": {"NODE_ENV": "development"}},
    {"name": "Server", "program": "server.js", "args": ["--inspect"]},
    {"name": "Spaced", "runtimeExecutable": "node", "runtimeArgs": ["my server.js"]},
    {"name": "Piped", "runtimeExecutable": "sh", "runtimeArgs": ["-c", "pnpm build | tee log"]},
    {"name": "Remote", "url": "https://example.test"}
  ]
}`)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "dev.yaml"))
	want := `name: dev
dev-commands:
    - name: Dashboard
      command: bash scripts/run-preview.bash
      port: 5555
      auto-port: true
    - name: Docs
      command: pnpm dev:mintlify
      cwd: apps/docs
      port: 3000
      env:
        NODE_ENV: development
    - name: Server
      command: node server.js --inspect
    - name: Spaced
      command:
        - node
        - my server.js
    - name: Piped
      command: pnpm build | tee log
x-claude:
    autoVerify: false
`
	if got != want {
		t.Errorf("environments/dev.yaml:\n%s\nwant:\n%s", got, want)
	}
}
