package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An upgrade that moves a vendor alias prints the move once, so the Codex
// files that change under an unchanged spec are explained (#1578).
func TestSync_NotesAnAliasThatMovesOnce(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "team.yaml"), "model: sol\n")
	if out := syncOutput(t); strings.Contains(out, "now resolves") {
		t.Errorf("a first sync has nothing to compare:\n%s", out)
	}
	if got := readStateFile(dir).ModelAliases["codex"]["sol"]; got != "gpt-6.1-sol" {
		t.Fatalf("state records sol as %q, want gpt-6.1-sol", got)
	}
	seedModelAlias(t, dir, "codex", "sol", "gpt-6-sol")

	if out := syncOutput(t); !strings.Contains(out, "codex: sol now resolves to gpt-6.1-sol (was gpt-6-sol)") {
		t.Errorf("sync must note the moved alias:\n%s", out)
	}
	if out := syncOutput(t); strings.Contains(out, "now resolves") {
		t.Errorf("a second sync must stay quiet:\n%s", out)
	}
}

// sync --json prints no notes, so it keeps the recorded resolution for
// the next text sync to report.
func TestSyncJSON_KeepsTheAliasRecordForTheNextSync(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "team.yaml"), "model: sol\n")
	syncOutput(t)
	seedModelAlias(t, dir, "codex", "sol", "gpt-6-sol")

	if out, err := runCLI(t, "sync", "--json"); err != nil {
		t.Fatalf("sync --json: %v\n%s", err, out)
	}

	if out := syncOutput(t); !strings.Contains(out, "codex: sol now resolves to gpt-6.1-sol (was gpt-6-sol)") {
		t.Errorf("the text sync after sync --json must note the moved alias:\n%s", out)
	}
}

func seedModelAlias(t *testing.T, dir, target, alias, model string) {
	t.Helper()
	state := readStateFile(dir)
	state.ModelAliases = map[string]map[string]string{target: {alias: model}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFilePath(dir), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// --quiet prints no notes, so a moved alias waits for the next sync that
// does.
func TestSync_QuietKeepsAMovedAliasForTheNextSync(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "team.yaml"), "model: sol\n")
	syncOutput(t)
	seedModelAlias(t, dir, "codex", "sol", "gpt-6-sol")

	if out := syncOutput(t, "--quiet"); strings.Contains(out, "now resolves") {
		t.Errorf("--quiet must print no note:\n%s", out)
	}

	if out := syncOutput(t); !strings.Contains(out, "codex: sol now resolves to gpt-6.1-sol (was gpt-6-sol)") {
		t.Errorf("the sync after --quiet must note the moved alias:\n%s", out)
	}
}
