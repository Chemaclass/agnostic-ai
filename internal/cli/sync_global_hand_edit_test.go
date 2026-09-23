package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syncedClaudeInstructions(t *testing.T) (home, source, path string) {
	t.Helper()
	home, source = globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "# Agreements\n\n- Rule one.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	return home, source, filepath.Join(home, ".claude", "CLAUDE.md")
}

func editGlobalFile(t *testing.T, path, from, to string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), from, to, 1)
	if edited == string(data) {
		t.Fatalf("%s has no %q to edit:\n%s", path, from, data)
	}
	mustWriteGlobalTest(t, path, edited)
	return edited
}

func TestSyncGlobal_HandEditedBlockStopsTheRun(t *testing.T) {
	home, _, path := syncedClaudeInstructions(t)
	edited := editGlobalFile(t, path, "- Rule one.", "- Rule one, edited by hand.")
	skill := filepath.Join(home, ".claude", "skills", "new", "SKILL.md")
	mustWriteGlobalTest(t, filepath.Join(os.Getenv("AGNOSTIC_AI_HOME"), "skills", "new", "SKILL.md"), "---\nname: new\ndescription: New\n---\nNew.\n")

	_, _, err := runGlobalAgentTest("--only", "claude")
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "--backup") {
		t.Fatalf("a hand edit must stop the run and name the file and --backup, got %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != edited {
		t.Errorf("the hand edit was overwritten:\n%s", data)
	}
	if _, statErr := os.Stat(skill); !os.IsNotExist(statErr) {
		t.Errorf("no planned write may land after the stop, stat err = %v", statErr)
	}
}

func TestSyncGlobal_BackupOverwritesHandEditAndKeepsIt(t *testing.T) {
	_, _, path := syncedClaudeInstructions(t)
	edited := editGlobalFile(t, path, "- Rule one.", "- Rule one, edited by hand.")

	if _, _, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("--backup must overwrite a hand edit: %v", err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != edited {
		t.Errorf("backup must hold the hand edit, err = %v:\n%s", err, backup)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "edited by hand") {
		t.Errorf("--backup must still write the source version:\n%s", data)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Errorf("the next sync after --backup must not report the old edit: %v", err)
	}
}

func TestSyncGlobal_TextOutsideBlockAndSourceChangesSync(t *testing.T) {
	_, source, path := syncedClaudeInstructions(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, path, "My own notes.\n\n"+string(data))
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "# Agreements\n\n- Rule two.\n")

	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("user text outside the block and a source change are not hand edits: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "My own notes.") || !strings.Contains(string(data), "- Rule two.") {
		t.Errorf("sync must keep user text and apply the source change:\n%s", data)
	}
}

func TestSyncGlobal_HandEditedRemovedSkillStopsTheRun(t *testing.T) {
	home, source := globalAgentTestHome(t)
	src := filepath.Join(source, "skills", "tidy", "SKILL.md")
	mustWriteGlobalTest(t, src, "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md")
	editGlobalFile(t, out, "Tidy.\n", "Tidy, my way.\n")
	if err := os.RemoveAll(filepath.Dir(src)); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGlobalAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), out) {
		t.Fatalf("removing a hand-edited file must stop the run, got %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("the hand-edited file must survive: %v", err)
	}
}

func TestSyncGlobal_StateWithoutSumsDoesNotStop(t *testing.T) {
	_, source, path := syncedClaudeInstructions(t)
	statePath := filepath.Join(source, "state", "global.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state["sums"]; !ok {
		t.Fatalf("state records no sums:\n%s", data)
	}
	delete(state, "sums")
	state["version"] = 4
	older, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, statePath, string(older))
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("an older state holding the same ownership is not drift: %v", err)
	}
	editGlobalFile(t, path, "- Rule one.", "- Rule one, edited by hand.")

	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Errorf("an older state has no record to compare, so sync proceeds as before: %v", err)
	}
}
