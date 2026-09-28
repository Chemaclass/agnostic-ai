package cli

import (
	"os"
	"path/filepath"
	"strings"
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
    {"name": "Cd", "runtimeExecutable": "sh", "runtimeArgs": ["-c", "cd x"]},
    {"name": "Lines", "runtimeExecutable": "sh", "runtimeArgs": ["-c", "a\nb"]},
    {"name": "Tsx", "runtimeExecutable": "tsx", "program": "server.ts", "args": ["--x"]},
    {"name": "Tail", "runtimeExecutable": "npm", "runtimeArgs": ["start"]}
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
    - name: Cd
      command: cd x
    - name: Lines
      command: |-
        a
        b
    - name: Tsx
      command: tsx server.ts --x
    - name: Tail
      command: npm start
x-claude:
    autoVerify: false
`
	if got != want {
		t.Errorf("environments/dev.yaml:\n%s\nwant:\n%s", got, want)
	}
}

// A configuration import cannot express keeps the whole file as written:
// sync rebuilds launch.json from the spec, so a partial import would
// drop that configuration.
func TestImportFromClaude_LeavesAPartlyImportableLaunchJSON(t *testing.T) {
	for name, config := range map[string]string{
		"url only":       `{"name": "Remote", "url": "https://example.test"}`,
		"non-string arg": `{"name": "Bad", "runtimeExecutable": "npm", "runtimeArgs": ["run", 3]}`,
	} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".claude", "launch.json"),
			`{"configurations": [{"name": "Web", "runtimeExecutable": "npm", "runtimeArgs": ["start"]}, `+config+`]}`)
		log := captureLog(t)
		if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "environments", "dev.yaml")); !os.IsNotExist(err) {
			t.Errorf("%s: environments/dev.yaml written for a partly importable launch.json", name)
		}
		if !strings.Contains(log.String(), "left .claude/launch.json as written") {
			t.Errorf("%s: summary does not say launch.json was left:\n%s", name, log.String())
		}
	}
}
