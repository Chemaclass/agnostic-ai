package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// seedFactoryBundle writes one spec per kind the factory adapter emits:
// a rule, a droid whose tools need the three renames, a root and a
// scoped skill, a command, a multi-command hook group, an MCP server,
// and settings carrying a model and all three permission lists.
func seedFactoryBundle(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"),
		"---\nname: style\n---\n\nKeep functions short.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews diffs\nmodel: sonnet\ntools: [Read, Bash, Write, WebFetch, Grep]\neffort: high\n---\n\nReview the diff.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "release", "SKILL.md"),
		"---\nname: release\ndescription: Cut a release\n---\n\nTag and publish.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "backend", "migrate", "SKILL.md"),
		"---\nname: migrate\ndescription: Run migrations\n---\n\nRun the migrations.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "commands", "ship.md"),
		"---\ndescription: Ship it\nargument-hint: \"[version]\"\n---\n\nShip version $ARGUMENTS.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "gate.yaml"),
		"name: gate\nevent: PostToolUse\nmatcher: Edit\ncommand:\n  - gofmt -l .\n  - go vet ./...\ntimeout: 30\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"),
		"name: fs\ncommand: npx\nargs:\n  - -y\n  - \"@modelcontextprotocol/server-filesystem\"\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "remote.yaml"),
		"name: remote\ntype: http\nurl: https://example.test/mcp\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "base.yaml"),
		"model: claude-sonnet-5\npermissions:\n  allow:\n    - Bash(git status)\n    - Bash(npm run:*)\n  ask:\n    - Bash(git push:*)\n  deny:\n    - Bash(rm -rf:*)\n")
}

// TestImportFactory_RoundTripFixedPoint emits every kind the factory
// adapter writes, wipes the specs, imports the tree back, and re-emits.
// The second emit must byte-match the first.
func TestImportFactory_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	seedFactoryBundle(t, dir)

	execCLI(t, "sync", "-t", "factory")
	first := snapshotEmitted(t, dir)
	for _, want := range []string{
		".factory/droids/reviewer.md", ".agents/skills/release/SKILL.md", "backend/.factory/skills/migrate/SKILL.md",
		".factory/commands/ship.md", ".factory/hooks.json", ".factory/mcp.json", ".factory/settings.json",
	} {
		if _, ok := first[want]; !ok {
			t.Fatalf("first sync wrote no %s: %v", want, keys(first))
		}
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "factory")

	droid := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"))
	for _, want := range []string{"- Bash", "- Write", "- WebFetch"} {
		if !strings.Contains(droid, want) {
			t.Errorf("droid tools not renamed back, missing %q:\n%s", want, droid)
		}
	}
	for _, native := range []string{"Execute", "Create", "FetchUrl"} {
		if strings.Contains(droid, native) {
			t.Errorf("droid kept Factory tool ID %q:\n%s", native, droid)
		}
	}
	settings := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "factory.yaml"))
	for _, want := range []string{"model: claude-sonnet-5", "Bash(npm run:*)", "Bash(git push:*)", "Bash(rm -rf:*)"} {
		if !strings.Contains(settings, want) {
			t.Errorf("settings missing %q:\n%s", want, settings)
		}
	}

	execCLI(t, "sync", "-t", "factory")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
	if len(second) != len(first) {
		t.Errorf("re-emit wrote %d files, first emit wrote %d: %v", len(second), len(first), keys(second))
	}
}

// A hand-written droid may list a Factory category or an MCP tool ID
// the portable vocabulary cannot express. The whole list then lands
// under x-factory.tools verbatim instead of being half-translated.
func TestImportFactory_DroidWithNativeOnlyToolsKeepsThemUnderXFactory(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".factory", "droids", "scout.md"),
		"---\nname: scout\ndescription: Explores\ntools: read-only\n---\n\nLook around.\n")

	execCLI(t, "import", "factory")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "scout.md"))
	if !strings.Contains(got, "x-factory:") || !strings.Contains(got, "tools: read-only") {
		t.Errorf("native-only tools not kept under x-factory:\n%s", got)
	}
}

// Factory still loads the legacy `.factory/hooks/hooks.json` when the
// current file is absent.
func TestImportFactory_ReadsLegacyHooksFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".factory", "hooks", "hooks.json"),
		`{"PreToolUse": [{"matcher": "Execute", "hooks": [{"type": "command", "command": "./guard.sh", "timeout": 5}]}]}`)

	execCLI(t, "import", "factory")

	hooks := readDirNames(t, filepath.Join(dir, ".agnostic-ai", "hooks"))
	if len(hooks) != 1 {
		t.Fatalf("want 1 hook spec, got %v", hooks)
	}
	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", hooks[0]))
	for _, want := range []string{"event: PreToolUse", "matcher: Execute", "command: ./guard.sh", "timeout: 5"} {
		if !strings.Contains(got, want) {
			t.Errorf("legacy hook missing %q:\n%s", want, got)
		}
	}
}

func TestImportFactory_DryRunMatchesRealImport(t *testing.T) {
	assertDryRunMatchesRealImport(t, "factory", func(t *testing.T, dir string) {
		seedFactoryBundle(t, dir)
		execCLI(t, "sync", "-t", "factory")
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
			t.Fatal(err)
		}
	})
}
