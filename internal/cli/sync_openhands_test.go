package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_OpenhandsMovesOwnedScopedRuleOutOfSharedSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	seedOpenhandsBundle(t, dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [openhands, codex]\n")
	execCLI(t, "sync")

	const flat = ".agents/skills/api.md"
	const old = ".agents/skills/api/SKILL.md"
	const skill = ".agents/skills/release/SKILL.md"
	const manual = ".agents/skills/manual/SKILL.md"
	const manualRule = "---\npaths: [docs/**]\n---\n\nCheck docs links.\n"
	initialRule := readFile(t, flat)
	initialSkill := readFile(t, skill)
	legacyRule := strings.Replace(initialRule, "name: path-rule/api\n", "", 1)
	writeFile(t, old, legacyRule)
	writeFile(t, manual, manualRule)
	if err := os.Remove(flat); err != nil {
		t.Fatal(err)
	}
	editLedger(t, func(state map[string]any) {
		// The ledger keeps each path in the separator sync wrote it with.
		outputs := state["outputs"].([]any)
		for i, path := range outputs {
			if filepath.ToSlash(path.(string)) == flat {
				outputs[i] = filepath.FromSlash(old)
			}
		}
		sums := state["output_sums"].(map[string]any)
		sums[filepath.FromSlash(old)] = sha256Hex([]byte(legacyRule))
		delete(sums, flat)
		delete(sums, filepath.FromSlash(flat))
	})

	execCLI(t, "sync")
	if _, err := os.Stat(".agents/skills/api"); !os.IsNotExist(err) {
		t.Errorf("expected the old generated rule folder to be removed, got %v", err)
	}
	if got := readFile(t, flat); got != initialRule {
		t.Errorf("migrated scoped rule changed contents:\n%s", got)
	}
	if got := readFile(t, skill); got != initialSkill {
		t.Errorf("ordinary skill changed during migration:\n%s", got)
	}
	if got := readFile(t, manual); got != manualRule {
		t.Errorf("hand-written rule changed during migration:\n%s", got)
	}
	execCLI(t, "sync", "--check")
}
