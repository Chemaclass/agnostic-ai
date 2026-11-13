package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
	for _, want := range []string{"x-factory:", "- Execute", "- Create", "- FetchUrl", "- Grep"} {
		if !strings.Contains(droid, want) {
			t.Errorf("native-only droid list lost %q:\n%s", want, droid)
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

// A `readonly: true` agent emits `tools: read-only` (#1162). That
// category string has no portable spelling, so import keeps it under
// x-factory.tools rather than guessing it means readonly: true back;
// re-emitting the imported spec must still reach the same droid file,
// since x-factory.tools wins outright over readonly on the way out.
func TestImportFactory_ReadonlyDroidRoundTripsThroughXFactoryTools(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "scout.md"),
		"---\nname: scout\ndescription: Explores\nreadonly: true\n---\n\nLook around.\n")

	execCLI(t, "sync", "-t", "factory")
	first := snapshotEmitted(t, dir)
	droidBefore := first[".factory/droids/scout.md"]
	if !strings.Contains(droidBefore, "tools: read-only") {
		t.Fatalf("first sync did not emit tools: read-only:\n%s", droidBefore)
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "factory")

	got := readFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "scout.md"))
	if strings.Contains(got, "readonly:") {
		t.Errorf("expected tools: read-only to stay under x-factory.tools, not readonly:true, got:\n%s", got)
	}
	if !strings.Contains(got, "x-factory:") || !strings.Contains(got, "tools: read-only") {
		t.Errorf("expected tools: read-only to round-trip under x-factory:\n%s", got)
	}

	execCLI(t, "sync", "-t", "factory")
	second := snapshotEmitted(t, dir)
	assertEmittedEqual(t, first, second)
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

func TestImportFactory_MCPEnabledToolsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	servers := map[string]map[string]any{
		"local":  {"type": "stdio", "command": "server", "enabledTools": []any{"safe", "shared"}, "disabledTools": []any{"shared"}},
		"remote": {"type": "http", "url": "https://example.test/mcp", "enabledTools": []any{"safe"}, "disabledTools": []any{"destroy"}},
		"empty":  {"type": "sse", "url": "https://example.test/sse", "enabledTools": []any{}, "disabledTools": []any{}},
	}
	native, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".factory", "mcp.json"), string(native))
	execCLI(t, "import", "factory")
	for name := range servers {
		imported := readFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", name+".yaml"))
		if !strings.Contains(imported, "x-factory:") || !strings.Contains(imported, "    enabledTools:") {
			t.Errorf("%s must import enabledTools under x-factory:\n%s", name, imported)
		}
	}
	execCLI(t, "lint", "--strict")
	execCLI(t, "sync")
	var got struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".factory", "mcp.json"))), &got); err != nil {
		t.Fatal(err)
	}
	for name, want := range servers {
		for _, key := range []string{"enabledTools", "disabledTools"} {
			if !reflect.DeepEqual(got.Servers[name][key], want[key]) {
				t.Errorf("%s %s = %#v, want %#v", name, key, got.Servers[name][key], want[key])
			}
		}
	}
	execCLI(t, "sync", "--check")
}

const factoryRegexHooks = `{"PreToolUse": [{"matcher": "Execute", "commandRegex": "^git ", "hooks": [{"type": "command", "command": "./guard.sh"}]}]}`

func factoryHooksJSON(t *testing.T, dir string) string {
	t.Helper()
	return readFile(t, filepath.Join(dir, ".factory", "hooks.json"))
}

func TestImportFactory_KeepsCommandRegexThroughSync(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"), factoryRegexHooks)

	execCLI(t, "import", "factory")
	execCLI(t, "sync")
	first := factoryHooksJSON(t, dir)
	if strings.Count(first, `"matcher"`) != 1 || !strings.Contains(first, `"commandRegex": "^git "`) {
		t.Fatalf("want one group with its commandRegex, got:\n%s", first)
	}

	execCLI(t, "import", "factory")
	execCLI(t, "sync")
	if got := factoryHooksJSON(t, dir); got != first {
		t.Errorf("second round trip changed the file:\n%s\nwant\n%s", got, first)
	}
}

func TestImportFactoryHooks_NamesAreStableAcrossRuns(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"), factoryRegexHooks)
	var runs [2][]string
	for i := range runs {
		dst := filepath.Join(dir, "out", string(rune('a'+i)))
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := importFactoryHooks(dir, dst); err != nil {
			t.Fatal(err)
		}
		runs[i] = readDirNames(t, dst)
	}
	if len(runs[0]) != 1 || strings.Join(runs[0], ",") != strings.Join(runs[1], ",") {
		t.Errorf("spec names differ between runs: %v and %v", runs[0], runs[1])
	}
}

func TestImportFactory_NonStringCommandRegexIsIgnored(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"),
		`{"PreToolUse": [{"matcher": "Execute", "commandRegex": 5, "hooks": [{"type": "command", "command": "./guard.sh"}]}]}`)
	execCLI(t, "import", "factory")
	names := readDirNames(t, filepath.Join(dir, ".agnostic-ai", "hooks"))
	if len(names) != 1 || strings.Contains(readFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", names[0])), "x-factory") {
		t.Errorf("want one hook spec without x-factory, got %v", names)
	}
}

func TestImportFactory_NewRegexGroupBesideSyncedSpecIsImported(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"), factoryRegexHooks)
	execCLI(t, "import", "factory")
	execCLI(t, "sync")

	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"),
		`{"PreToolUse": [{"matcher": "Execute", "commandRegex": "^git ", "hooks": [{"type": "command", "command": "./guard.sh"}]}, {"matcher": "Execute", "commandRegex": "^npm ", "hooks": [{"type": "command", "command": "./guard.sh"}]}]}`)
	execCLI(t, "import", "factory")
	execCLI(t, "sync")

	if names := readDirNames(t, filepath.Join(dir, ".agnostic-ai", "hooks")); len(names) != 2 {
		t.Errorf("want a second spec for the new regex, got %v", names)
	}
	got := factoryHooksJSON(t, dir)
	if !strings.Contains(got, `"^npm "`) || strings.Count(got, `"^git "`) != 1 {
		t.Errorf("want one group per regex:\n%s", got)
	}
}

func TestImportFactory_GroupWithoutCommandRegexImportsPlain(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"),
		`{"PreToolUse": [{"matcher": "Execute", "hooks": [{"type": "command", "command": "./guard.sh"}]}]}`)
	execCLI(t, "import", "factory")
	execCLI(t, "sync")

	for _, name := range readDirNames(t, filepath.Join(dir, ".agnostic-ai", "hooks")) {
		if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", name)); strings.Contains(got, "x-factory") {
			t.Errorf("a group with no commandRegex must not gain x-factory:\n%s", got)
		}
	}
	got := factoryHooksJSON(t, dir)
	if strings.Contains(got, "commandRegex") || strings.Count(got, `"matcher"`) != 1 {
		t.Errorf("want one plain group:\n%s", got)
	}
}

func TestImportFactory_SameCommandWithDifferentRegexesKeepsBoth(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"),
		`{"PreToolUse": [{"matcher": "Execute", "commandRegex": "^git ", "hooks": [{"type": "command", "command": "./guard.sh"}]}, {"matcher": "Execute", "commandRegex": "^npm ", "hooks": [{"type": "command", "command": "./guard.sh"}]}]}`)
	execCLI(t, "import", "factory")
	execCLI(t, "sync")

	got := factoryHooksJSON(t, dir)
	if !strings.Contains(got, `"^git "`) || !strings.Contains(got, `"^npm "`) || strings.Count(got, `"matcher"`) != 2 {
		t.Errorf("both regex groups must survive:\n%s", got)
	}
}

func TestImportFactory_HandNamedRegexSpecOwnsItsNativeGroup(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [factory]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "git-guard.yaml"),
		"name: git-guard\nevent: PreToolUse\nmatcher: Execute\ncommand: ./guard.sh\ntarget: factory\nx-factory:\n  commandRegex: '^git '\n")
	execCLI(t, "sync")
	want := factoryHooksJSON(t, dir)
	if !strings.Contains(want, `"^git "`) {
		t.Fatalf("sync did not write the regex:\n%s", want)
	}

	execCLI(t, "import", "factory")
	if names := readDirNames(t, filepath.Join(dir, ".agnostic-ai", "hooks")); len(names) != 1 || names[0] != "git-guard.yaml" {
		t.Errorf("import must not add a spec for the synced group, got %v", names)
	}
	execCLI(t, "sync")
	if got := factoryHooksJSON(t, dir); got != want {
		t.Errorf("sync after import changed the file:\n%s\nwant\n%s", got, want)
	}

	writeFile(t, filepath.Join(dir, ".factory", "hooks.json"),
		`{"PreToolUse": [{"matcher": "Execute", "commandRegex": "^git ", "hooks": [{"type": "command", "command": "./guard.sh"}]}, {"matcher": "Execute", "hooks": [{"type": "command", "command": "./guard.sh"}]}]}`)
	execCLI(t, "import", "factory")
	if names := readDirNames(t, filepath.Join(dir, ".agnostic-ai", "hooks")); len(names) != 2 {
		t.Errorf("the hand-written group without a regex must be imported, got %v", names)
	}
}
