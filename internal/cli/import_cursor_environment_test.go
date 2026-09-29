package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A hand-written .cursor/environment.json becomes an environment spec,
// keeping its // comments as YAML comments (#1394).
func TestImportFromCursor_ReadsEnvironmentJSONWithComments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "environment.json"), `{
  // Cloud agents need computer use
  "install": "bash .cursor/cloud-agent/install.sh",
  "snapshot": "snapshot-1", // rebuilt monthly
  /* Ports the preview opens.
     Keep in step with the dev server. */
  "ports": [
    // web
    {"name": "web", "port": 3000},
    {"name": "api", "port": 8080}
  ],
  "terminals": [],
  "env": {},
  "agentCanUpdateSnapshot": true,
  "user": null,
  "build": {"dockerfile": "Dockerfile", "context": "."},
}
`)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "cursor.yaml"))
	want := `name: cursor
# Cloud agents need computer use
install: bash .cursor/cloud-agent/install.sh
snapshot: snapshot-1 # rebuilt monthly
# Ports the preview opens.
# Keep in step with the dev server.
ports:
    # web
    - name: web
      port: 3000
    - name: api
      port: 8080
terminals: []
env: {}
agentCanUpdateSnapshot: true
user: null
build:
    dockerfile: Dockerfile
    context: .
`
	if got != want {
		t.Errorf("environments/cursor.yaml:\n%s\nwant:\n%s", got, want)
	}
}

func TestImportFromCursor_ReadsEnvironmentJSONWithoutComments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "environment.json"),
		`{"install": "npm ci", "start": "docker compose up -d", "ports": [3000]}`)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "cursor.yaml"))
	want := "name: cursor\ninstall: npm ci\nstart: docker compose up -d\nports:\n    - 3000\n"
	if got != want {
		t.Errorf("environments/cursor.yaml:\n%s\nwant:\n%s", got, want)
	}
}

// import, then sync, writes an environment.json with the same keys and
// values as the hand-written one.
func TestImportFromCursor_EnvironmentJSONSurvivesSync(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	original := `{
  // Cloud agents need computer use
  "install": "bash .cursor/cloud-agent/install.sh", // one script
  "snapshot": "snapshot-1",
  "ports": [{"name": "web", "port": 3000}],
  "env": {"NODE_ENV": "development"},
  "terminals": [{"name": "dev", "command": "npm run dev"}],
  "agentCanUpdateSnapshot": false
}
`
	writeFile(t, filepath.Join(".cursor", "environment.json"), original)
	want := map[string]any{
		"install":                "bash .cursor/cloud-agent/install.sh",
		"snapshot":               "snapshot-1",
		"ports":                  []any{map[string]any{"name": "web", "port": float64(3000)}},
		"env":                    map[string]any{"NODE_ENV": "development"},
		"terminals":              []any{map[string]any{"name": "dev", "command": "npm run dev"}},
		"agentCanUpdateSnapshot": false,
	}

	for _, args := range [][]string{{"import", "cursor"}, {"sync"}} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(".cursor", "environment.json"))), &got); err != nil {
		t.Fatalf("synced environment.json: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("synced environment.json = %v, want %v", got, want)
	}
}

// A file sync wrote, or an existing spec, is left alone.
func TestImportFromCursor_SkipsSyncedEnvironmentJSONAndExistingSpec(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "environment.json"), `{"install": "make setup"}`)
	own := "name: cursor\ninstall: make other\n"
	writeFile(t, filepath.Join(dir, "environments", "cursor.yaml"), own)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if got := readFileString(t, filepath.Join(dir, "environments", "cursor.yaml")); got != own {
		t.Errorf("existing spec changed:\n%s", got)
	}

	synced := t.TempDir()
	writeFile(t, filepath.Join(synced, ".cursor", "environment.json"), `{"install": "make setup"}`)
	writeFile(t, filepath.Join(synced, ".agnostic-ai", ".sync-state"), `{"outputs":[".cursor/environment.json"]}`)
	if err := importFromCursor(synced, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(synced, "environments", "cursor.yaml")); !os.IsNotExist(err) {
		t.Errorf("a synced environment.json was imported: %v", err)
	}
}

// A key the environment spec reads for itself would not reach
// environment.json again, so sync would drop it: the file stays whole.
func TestImportFromCursor_LeavesEnvironmentJSONWithAReservedKey(t *testing.T) {
	dir := t.TempDir()
	original := `{"install": "npm ci", "name": "web"}`
	writeFile(t, filepath.Join(dir, ".cursor", "environment.json"), original)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "environments", "cursor.yaml")); !os.IsNotExist(err) {
		t.Errorf("a file with a reserved key was imported: %v", err)
	}
	if got := readFileString(t, filepath.Join(dir, ".cursor", "environment.json")); got != original {
		t.Errorf("environment.json changed:\n%s", got)
	}
}

func TestImportFromCursor_ReportsInvalidEnvironmentJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "environment.json"), `{"install": `)
	if err := importFromCursor(dir, rootSources()); err == nil {
		t.Fatal("want a parse error")
	}
}
