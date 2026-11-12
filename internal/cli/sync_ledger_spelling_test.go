package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// respellLedger rewrites the ledger as releases before #3329 left it: the
// output at clean recorded under the spelling the config used.
func respellLedger(t *testing.T, clean, typed string) {
	t.Helper()
	state := readStateFile(".")
	for i, p := range state.Outputs {
		if p == clean {
			state.Outputs[i] = typed
		}
	}
	if sum, ok := state.OutputSums[clean]; ok {
		delete(state.OutputSums, clean)
		state.OutputSums[typed] = sum
	}
	if m, ok := state.Merged[clean]; ok {
		delete(state.Merged, clean)
		state.Merged[typed] = m
	}
	if err := replaceStateFile(".", state); err != nil {
		t.Fatal(err)
	}
}

func TestSync_LedgerSpelling_UpgradeKeepsAMergedFile(t *testing.T) {
	for name, userKeys := range map[string]string{"created by sync": "", "with keys the user owns": "{\"amp.theme\": \"dark\"}\n"} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [amp]\noutputs:\n  amp:\n    mcp-file: ./.amp/settings.json\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), "name: gh\ncommand: npx\nargs: [gh-mcp]\n")
			if userKeys != "" {
				mustWriteFile(t, filepath.Join(dir, ".amp", "settings.json"), userKeys)
			}
			if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			respellLedger(t, ".amp/settings.json", "./.amp/settings.json")
			if _, ok := readStateFile(".").Merged[".amp/settings.json"]; !ok {
				t.Fatal("the ledger must hold the merged ownership of .amp/settings.json")
			}

			if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("sync after the upgrade: %v\n%s", err, out)
			}

			data, err := os.ReadFile(filepath.Join(dir, ".amp", "settings.json"))
			if err != nil || !strings.Contains(string(data), "gh-mcp") {
				t.Fatalf("sync must keep the gh server (err %v):\n%s", err, data)
			}
			if userKeys != "" && !strings.Contains(string(data), "amp.theme") {
				t.Errorf("sync must keep the user's key:\n%s", data)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
		})
	}
}

func TestSync_LedgerSpelling_UpgradeKeepsAWholeFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    rules-file: ./docs/RULES.md\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nUse tabs.\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	want, _ := os.ReadFile(filepath.Join(dir, "docs", "RULES.md"))
	respellLedger(t, "docs/RULES.md", "./docs/RULES.md")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync after the upgrade: %v\n%s", err, out)
	}

	if got, err := os.ReadFile(filepath.Join(dir, "docs", "RULES.md")); err != nil || string(got) != string(want) {
		t.Errorf("sync must keep docs/RULES.md (err %v):\n%s", err, got)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Claude joins .claude and settings.json with the system separator, so on
// Windows the ledger records a backslash. The next sync must still find
// the record and keep both its key and the user's.
func TestSync_LedgerSpelling_SecondSyncKeepsAMergedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "defaults.yaml"), "model: claude-opus-4-8\n")
	mustWriteFile(t, filepath.Join(dir, ".claude", "settings.json"), "{\"theme\": \"dark\"}\n")

	for i := range 2 {
		if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
			t.Fatalf("sync %d: %v\n%s", i+1, err, out)
		}
	}

	if _, ok := readStateFile(".").Merged[filepath.Join(".claude", "settings.json")]; !ok {
		t.Errorf("the ledger must keep the merged record of %s", filepath.Join(".claude", "settings.json"))
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if !strings.Contains(string(data), "claude-opus-4-8") || !strings.Contains(string(data), "dark") {
		t.Errorf("settings.json must keep sync's key and the user's:\n%s", data)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

func TestSync_LedgerSpelling_KeepsWindowsSeparatorsAndDotDot(t *testing.T) {
	state := syncStateFile{
		Outputs:    []string{`.claude\settings.json`, "./AGENTS.md", "alias/../settings.json"},
		OutputSums: map[string]string{`.claude\settings.json`: "a", "./AGENTS.md": "b", "alias/../settings.json": "c"},
		Merged:     map[string]mergedOutput{`.claude\settings.json`: {Created: true}, "alias/../settings.json": {Created: true}},
	}

	state.mergeSpellings(t.TempDir(), func(p string) string { return config.CleanOutputPathOn(p, "windows") })

	want := []string{`.claude\settings.json`, "AGENTS.md", "alias/../settings.json"}
	if !slices.Equal(state.Outputs, want) {
		t.Errorf("outputs = %q, want %q", state.Outputs, want)
	}
	if state.OutputSums[`.claude\settings.json`] != "a" || state.OutputSums["AGENTS.md"] != "b" || state.OutputSums["alias/../settings.json"] != "c" {
		t.Errorf("sums = %v", state.OutputSums)
	}
	if _, ok := state.Merged[`.claude\settings.json`]; !ok {
		t.Errorf("merged = %v, want the backslash record kept", state.Merged)
	}
	if _, ok := state.Merged["alias/../settings.json"]; !ok {
		t.Errorf("merged = %v, want the .. record kept", state.Merged)
	}
}

func TestSync_LedgerSpelling_UnitesTheClaimsOfTwoSpellings(t *testing.T) {
	theme := adapters.MergedKey{Path: []string{"theme"}}
	model := adapters.MergedKey{Path: []string{"model"}}
	state := syncStateFile{
		Outputs: []string{"./settings.json", "settings.json"},
		Merged: map[string]mergedOutput{
			"./settings.json": {Keys: []adapters.MergedKey{theme}, Created: true},
			"settings.json":   {Keys: []adapters.MergedKey{model}},
		},
	}

	state.mergeSpellings(t.TempDir(), config.CleanOutputPath)

	got, ok := state.Merged["settings.json"]
	if !ok || len(state.Merged) != 1 {
		t.Fatalf("merged = %v, want one settings.json record", state.Merged)
	}
	if len(got.Keys) != 2 || !got.Created {
		t.Errorf("record = %+v, want both keys and created", got)
	}
	if !slices.Equal(state.Outputs, []string{"settings.json"}) {
		t.Errorf("outputs = %q", state.Outputs)
	}
}

func rawLedger(t *testing.T) syncStateFile {
	t.Helper()
	data, err := os.ReadFile(stateFilePath("."))
	if err != nil {
		t.Fatal(err)
	}
	var state syncStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

// doctor --fix releases the merged keys of a dropped target. Over an old
// ledger it must release the claim under every spelling, or a later sync
// takes back a key the user adds again.
func TestDoctorFix_LedgerSpelling_ReleasesAnOldSpellingsClaim(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [amp]\noutputs:\n  amp:\n    mcp-file: ./.amp/settings.json\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "gh.yaml"), "name: gh\ncommand: npx\nargs: [gh-mcp]\n")
	mustWriteFile(t, filepath.Join(dir, ".amp", "settings.json"), "{\"amp.theme\": \"dark\"}\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	respellLedger(t, ".amp/settings.json", "./.amp/settings.json")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\noutputs:\n  amp:\n    mcp-file: ./.amp/settings.json\n")

	out, err := runCLI(t, "doctor", "--fix")

	if data, _ := os.ReadFile(filepath.Join(dir, ".amp", "settings.json")); strings.Contains(string(data), "gh-mcp") {
		t.Fatalf("doctor --fix must take out the gh server (err %v):\n%s\n%s", err, out, data)
	}
	for p := range rawLedger(t).Merged {
		if strings.Contains(p, ".amp/settings.json") {
			t.Errorf("the ledger must not claim keys in %q after doctor --fix", p)
		}
	}
}

func TestRecordOutputSums_RewritesAnOldSpelling(t *testing.T) {
	testutil.TempCwd(t)
	if err := replaceStateFile(".", syncStateFile{Version: syncStateVersion, Outputs: []string{"./AGENTS.md"}, OutputSums: map[string]string{"./AGENTS.md": "old"}}); err != nil {
		t.Fatal(err)
	}

	if err := recordOutputSums(".", map[string]string{"AGENTS.md": "new"}); err != nil {
		t.Fatal(err)
	}

	if got := rawLedger(t).OutputSums; len(got) != 1 || got["AGENTS.md"] != "new" {
		t.Errorf("the ledger must hold one sum under AGENTS.md, got %v", got)
	}
}
