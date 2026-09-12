package integration

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestGeminiRoundTrip_SyncImportSyncIsByteEqual is the byte-stability
// gate gemini never had. Scope: agents + commands, the two surfaces
// that share `.gemini/` and drifted apart in #748, where import read
// `.gemini/commands/` only when `.gemini/agents/` was missing and every
// command spec vanished (#750). Rules round-trip through the sync-owned
// GEMINI.md entry-point, the same carve-out codex and opencode take.
func TestGeminiRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedGeminiRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "gemini")
	first := snapshotGeminiEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no gemini output")
	}

	for _, sub := range []string{"agents", "commands"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "gemini")

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai/rules")); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{".gemini", "GEMINI.md"} {
		if err := os.RemoveAll(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "gemini")
	second := snapshotGeminiEmit(t, dir)

	firstPaths := sortedKeys(first)
	secondPaths := sortedKeys(second)
	if !equalStringSlice(firstPaths, secondPaths) {
		t.Fatalf("emit path set changed across round-trip\nfirst:  %v\nsecond: %v",
			firstPaths, secondPaths)
	}
	for _, p := range firstPaths {
		if first[p] != second[p] {
			t.Errorf("byte mismatch at %s (first=%d bytes, second=%d bytes)\n%s",
				p, len(first[p]), len(second[p]), unifiedDiffLines(first[p], second[p]))
		}
	}
}

// TestGeminiImport_KeepsCommandsWhenAgentsExist is the narrow guard for
// #750: a project carrying both a subagent and a slash command must
// import both. Before the fix the command file produced nothing and the
// summary carried no counter to show it.
func TestGeminiImport_KeepsCommandsWhenAgentsExist(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(geminiOnlyConfig), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".gemini/agents"), 0o755))
	must(t, os.MkdirAll(filepath.Join(dir, ".gemini/commands"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".gemini/agents/my-agent.md"),
		[]byte("---\nname: my-agent\ndescription: A subagent\n---\n\nAgent body.\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".gemini/commands/deploy.toml"),
		[]byte("description = \"Deploy the app\"\nprompt = \"Deploy to production\"\n"), 0o644))

	runCmd(t, "import", "gemini")

	for _, rel := range []string{".agnostic-ai/agents/my-agent.md", ".agnostic-ai/commands/deploy.md"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected %s after import: %v", rel, err)
		}
	}
}

func seedGeminiRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(geminiOnlyConfig), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\ndescription: agent "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/commands"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/commands/deploy.md"),
		[]byte("---\nname: deploy\ndescription: Deploy the app.\n---\n\nDeploy to production.\n"), 0o644))
	// alpha deliberately shares its name with an agent, legal since #733
	// moved agents out of `.gemini/commands/`. The two must stay distinct
	// through import and re-emit.
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/commands/alpha.md"),
		[]byte("---\nname: alpha\ndescription: The alpha command.\n---\n\nalpha command body\n"), 0o644))
}

func snapshotGeminiEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range []string{".gemini", "GEMINI.md"} {
		full := filepath.Join(root, r)
		info, err := os.Stat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if !info.IsDir() {
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("read %s: %v", full, err)
			}
			out[r] = string(data)
			continue
		}
		err = filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", full, err)
		}
	}
	return out
}

const geminiOnlyConfig = `version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
targets:
  - gemini
gitignore:
  enabled: false
`

func TestGeminiImport_PreservesNativeHookGroups(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(geminiOnlyConfig), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".gemini"), 0o755))
	native := `{"hooks":{"BeforeTool":[
		{"matcher":"write_file","sequential":true,"hooks":[
			{"type":"command","command":"echo first","name":"First check","timeout":1250,"description":"Validate","env":{"CHECK_MODE":"first"}},
			{"type":"command","command":"echo second","timeout":5000}
		]},
		{"matcher":"read_file","hooks":[{"type":"command","command":"echo read","timeout":5000,"env":{"CHECK_MODE":"read"}}]},
		{"matcher":"replace","hooks":[{"type":"command","command":"echo replace","timeout":1250,"name":"Replace check"}]},
		{"matcher":"read_file","sequential":false,"hooks":[{"type":"command","command":"echo read","name":"Second reader","timeout":2500}]}
	]}}`
	settingsPath := filepath.Join(dir, ".gemini/settings.json")
	must(t, os.WriteFile(settingsPath, []byte(native), 0o644))
	runCmd(t, "import", "gemini")
	// Re-import must reuse collision suffixes instead of creating more specs.
	runCmd(t, "import", "gemini")
	// Remove the native file so merge behavior cannot hide an import loss.
	must(t, os.Remove(settingsPath))
	runCmd(t, "sync", "-t", "gemini")
	actual, err := os.ReadFile(settingsPath)
	must(t, err)
	if !reflect.DeepEqual(geminiHookDefinitions(t, []byte(native)), geminiHookDefinitions(t, actual)) {
		t.Errorf("native hooks changed through import and sync:\nwant %s\ngot %s", native, actual)
	}
}

func geminiHookDefinitions(t *testing.T, raw []byte) []string {
	t.Helper()
	var settings struct {
		Hooks map[string][]map[string]any
	}
	must(t, json.Unmarshal(raw, &settings))
	var definitions []string
	for event, groups := range settings.Hooks {
		for _, group := range groups {
			encoded, err := json.Marshal(group)
			must(t, err)
			definitions = append(definitions, event+":"+string(encoded))
		}
	}
	// Source filenames sort independently of the vendor's definition order.
	// Preserve every definition, including ones with identical matchers.
	sort.Strings(definitions)
	return definitions
}
