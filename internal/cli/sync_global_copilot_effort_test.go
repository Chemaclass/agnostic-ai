package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const globalEffortAgent = `---
name: reviewer
description: Review code
effort:
  copilot: xhigh
  default: high
---
Review carefully.
`

func readCopilotSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".copilot", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse settings.json: %v\n%s", err, data)
	}
	return doc
}

func copilotAgentEffort(doc map[string]any, name string) any {
	subagents, _ := doc["subagents"].(map[string]any)
	agents, _ := subagents["agents"].(map[string]any)
	agent, _ := agents[name].(map[string]any)
	return agent["effortLevel"]
}

func TestSyncGlobal_CopilotAgentEffortLandsInUserSettings(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), globalEffortAgent)
	settings := filepath.Join(home, ".copilot", "settings.json")
	mustWriteGlobalTest(t, settings, `{
  // user comment
  "theme": "dark",
  "subagents": {"agents": {"reviewer": {"model": "gpt-6-sol"}}}
}`)

	_, warnings, err := runGlobalAgentTest("--only", "copilot")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if strings.Contains(warnings, "effort") {
		t.Errorf("global sync writes the effort, so no coverage note is due:\n%s", warnings)
	}
	doc := readCopilotSettings(t, home)
	if got := copilotAgentEffort(doc, "reviewer"); got != "xhigh" {
		t.Errorf("effortLevel = %v, want xhigh", got)
	}
	agent := doc["subagents"].(map[string]any)["agents"].(map[string]any)["reviewer"].(map[string]any)
	if doc["theme"] != "dark" || agent["model"] != "gpt-6-sol" {
		t.Errorf("user settings must survive the merge: %v", doc)
	}

	first, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	second, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("second sync changed settings.json:\n%s\n---\n%s", first, second)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
		t.Errorf("check after sync: %v", err)
	}
}

func TestSyncGlobal_CopilotAgentEffortRemovalKeepsUserSettings(t *testing.T) {
	home, source := globalAgentTestHome(t)
	agent := filepath.Join(source, "agents", "reviewer.md")
	mustWriteGlobalTest(t, agent, globalEffortAgent)
	mustWriteGlobalTest(t, filepath.Join(home, ".copilot", "settings.json"), `{"theme": "dark"}`)
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatal(err)
	}

	mustWriteGlobalTest(t, agent, "---\nname: reviewer\ndescription: Review code\n---\nReview carefully.\n")
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatal(err)
	}
	doc := readCopilotSettings(t, home)
	if _, ok := doc["subagents"]; ok {
		t.Errorf("the managed effort and its now-empty parents must go: %v", doc)
	}
	if doc["theme"] != "dark" {
		t.Errorf("user settings must survive removal: %v", doc)
	}
}

func TestSyncGlobal_CopilotHandSetEffortStopsBeforeWrites(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), globalEffortAgent)
	settings := filepath.Join(home, ".copilot", "settings.json")
	original := `{"subagents": {"agents": {"reviewer": {"effortLevel": "low"}}}}`
	mustWriteGlobalTest(t, settings, original)

	_, _, err := runGlobalAgentTest("--only", "copilot")
	if err == nil || !strings.Contains(err.Error(), "effortLevel") {
		t.Fatalf("a hand-set effortLevel must stop the run, got %v", err)
	}
	data, readErr := os.ReadFile(settings)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != original {
		t.Errorf("settings.json changed despite the conflict:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(home, ".copilot", "agents", "reviewer.agent.md")); !os.IsNotExist(err) {
		t.Errorf("no other planned write may land after a conflict, stat err = %v", err)
	}
}

func TestSyncGlobal_CopilotUnsupportedEffortKeepsNote(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\neffort: max\n---\nReview.\n")

	_, warnings, err := runGlobalAgentTest("--only", "copilot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, "effort") {
		t.Errorf("an effort Copilot does not accept must still raise a note:\n%s", warnings)
	}
	if _, err := os.Stat(filepath.Join(home, ".copilot", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("nothing valid to write, so settings.json must not be created, stat err = %v", err)
	}
}
