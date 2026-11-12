package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestSync_UpgradeKeepsAMergedFileTheLedgerSpelledWithDotSlash(t *testing.T) {
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

func TestSync_UpgradeKeepsAWholeFileTheLedgerSpelledWithDotSlash(t *testing.T) {
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
