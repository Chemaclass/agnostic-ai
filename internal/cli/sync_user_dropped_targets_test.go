package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncUser_TargetDroppedFromConfigLosesItsSkillsAndAgents(t *testing.T) {
	home, source := userAgentTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	writeFile(t, filepath.Join(source, "skills", "old-name", "SKILL.md"), "---\nname: old-name\ndescription: Use for tests\n---\nBody.\n")
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review changes\n---\nReview.\n")
	writeFile(t, config, "version: 1\ntargets: [codex, cursor, goose]\n")
	if _, _, err := runUserAgentTest(); err != nil {
		t.Fatal(err)
	}
	cursorSkill := filepath.Join(home, ".cursor", "skills", "old-name", "SKILL.md")
	cursorAgent := filepath.Join(home, ".cursor", "agents", "reviewer.md")
	sharedSkill := filepath.Join(home, ".agents", "skills", "old-name", "SKILL.md")
	for _, path := range []string{cursorSkill, cursorAgent, sharedSkill} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("first sync did not write %s: %v", path, err)
		}
	}

	// -t replaces the config for one run, so cursor's files stay.
	if _, _, err := runUserAgentTest("-t", "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cursorSkill); err != nil {
		t.Fatalf("-t run removed an unselected target's skill: %v", err)
	}

	// --only keeps a dropped target's file that a configured target
	// left out of this run still owns in a shared directory.
	writeFile(t, config, "version: 1\ntargets: [codex, cursor, openhands]\n")
	if _, _, err := runUserAgentTest("--only", "cursor"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sharedSkill); err != nil {
		t.Errorf("--only run removed a shared skill an unselected target owns: %v", err)
	}
	writeFile(t, config, "version: 1\ntargets: [codex, cursor, goose]\n")
	if _, _, err := runUserAgentTest(); err != nil {
		t.Fatal(err)
	}

	// A file the user already deleted by hand must not fail the run.
	if err := os.Remove(cursorAgent); err != nil {
		t.Fatal(err)
	}
	writeFile(t, config, "version: 1\ntargets: [codex]\n")
	if _, _, err := runUserAgentTest("--check"); err == nil {
		t.Error("--check reported no drift for a dropped target's files")
	}
	if _, err := os.Stat(cursorSkill); err != nil {
		t.Fatalf("--check removed a file: %v", err)
	}
	if _, _, err := runUserAgentTest(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{cursorSkill, cursorAgent, filepath.Dir(cursorSkill)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("dropped target kept %s: %v", path, err)
		}
	}
	// goose shared ~/.agents/skills with codex, which still writes it.
	if _, err := os.Stat(sharedSkill); err != nil {
		t.Errorf("shared skill a selected target writes was removed: %v", err)
	}
	if _, _, err := runUserAgentTest("--check"); err != nil {
		t.Fatalf("check after dropping targets: %v", err)
	}
	state, err := loadUserState(filepath.Join(source, "state", "global.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"cursor", "goose"} {
		if _, ok := state.Skills[target]; ok {
			t.Errorf("state still records %s skills", target)
		}
		if _, ok := state.Agents[target]; ok {
			t.Errorf("state still records %s agents", target)
		}
	}
}
