package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// userClaudeSettings holds an existing user-level permission rule.
const userClaudeSettings = `{
  "theme": "dark",
  "permissions": {
    "allow": [
      "Bash(ls:*)"
    ],
    "defaultMode": "auto"
  }
}
`

func readGlobalPermissionState(t *testing.T, source string) map[string][]string {
	t.Helper()
	var state globalState
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(source, "state", "global.json"))), &state); err != nil {
		t.Fatal(err)
	}
	return state.PermissionRules["claude"]
}

func TestSyncGlobal_PermissionRulesAppendMissingSpecRules(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, userClaudeSettings)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "rules.yaml"), "permissions:\n  allow: [\"Bash(git status)\", \"Bash(ls:*)\"]\n  deny: [\"Bash(rm -rf:*)\"]\n")

	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	want := `{
  "theme": "dark",
  "permissions": {
    "allow": [
      "Bash(ls:*)",
      "Bash(git status)"
    ],
    "defaultMode": "auto",
    "deny": [
      "Bash(rm -rf:*)"
    ]
  }
}
`
	if got := readGlobalTest(t, path); got != want {
		t.Errorf("settings.json:\n%s\nwant:\n%s", got, want)
	}
	recorded := readGlobalPermissionState(t, source)
	wantState := map[string][]string{"allow": {"Bash(git status)", "Bash(ls:*)"}, "deny": {"Bash(rm -rf:*)"}}
	if !reflect.DeepEqual(recorded, wantState) {
		t.Errorf("state = %v, want %v", recorded, wantState)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check after sync: %v", err)
	}
}

func TestSyncGlobal_PermissionRulesRemoveOnlyWhatSyncWrote(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, userClaudeSettings)
	rules := filepath.Join(source, "settings", "rules.yaml")
	mustWriteGlobalTest(t, rules, "permissions:\n  allow: [Read, \"Bash(git status)\"]\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	// The tool adds a rule of its own after the sync.
	synced := readGlobalTest(t, path)
	mustWriteGlobalTest(t, path, strings.Replace(synced, `"Bash(git status)"`, `"Bash(git status)",`+"\n"+`      "WebFetch"`, 1))

	mustWriteGlobalTest(t, rules, "permissions:\n  allow: [\"Bash(git status)\"]\n")
	out, _, err := runGlobalAgentTest("--only", "claude", "--plan")
	if err != nil || !strings.Contains(out, `remove "Read" from permissions.allow`) {
		t.Errorf("plan must name the removal: %v\n%s", err, out)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after dropping a rule: %v\n%s", err, warnings)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	allow := doc["permissions"].(map[string]any)["allow"]
	if want := []any{"Bash(ls:*)", "Bash(git status)", "WebFetch"}; !reflect.DeepEqual(allow, want) {
		t.Errorf("allow = %v, want %v", allow, want)
	}

	// With no rules left in the specs, the user's own rules stay.
	if err := os.Remove(rules); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after removing the spec: %v\n%s", err, warnings)
	}
	want := strings.Replace(userClaudeSettings, `"Bash(ls:*)"`, `"Bash(ls:*)",`+"\n"+`      "WebFetch"`, 1)
	if got := readGlobalTest(t, path); got != want {
		t.Errorf("settings.json:\n%s\nwant:\n%s", got, want)
	}
	if recorded := readGlobalPermissionState(t, source); recorded != nil {
		t.Errorf("no rules may stay recorded: %v", recorded)
	}
}

func TestSyncGlobal_PermissionRulesCheckReportsDrift(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, userClaudeSettings)
	rules := filepath.Join(source, "settings", "rules.yaml")
	mustWriteGlobalTest(t, rules, "permissions:\n  allow: [Read]\n")

	// A spec rule the file lacks.
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("a missing spec rule must drift: %v", err)
	}
	out, _, err := runGlobalAgentTest("--only", "claude", "--plan")
	if err != nil || !strings.Contains(out, `add "Read" to permissions.allow`) {
		t.Errorf("plan must name the addition: %v\n%s", err, out)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check after sync: %v", err)
	}

	// A rule removed from the file by hand.
	synced := readGlobalTest(t, path)
	mustWriteGlobalTest(t, path, userClaudeSettings)
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err == nil {
		t.Error("a spec rule removed by hand must drift")
	}
	mustWriteGlobalTest(t, path, synced)

	// A recorded rule the specs dropped, still in the file.
	mustWriteGlobalTest(t, rules, "model: opus\n")
	out, _, err = runGlobalAgentTest("--only", "claude", "--check", "--diff")
	if err == nil || !strings.Contains(out, `-      "Read"`) {
		t.Errorf("a dropped recorded rule must drift: %v\n%s", err, out)
	}
}

func TestSyncGlobal_PermissionRulesAdoptARuleAlreadyThere(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, userClaudeSettings)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "rules.yaml"), "permissions:\n  allow: [\"Bash(ls:*)\"]\n")

	_, warnings, err := runGlobalAgentTest("--only", "claude")
	if err != nil {
		t.Fatalf("a rule already there is no conflict: %v\n%s", err, warnings)
	}
	if !strings.Contains(warnings, `adopted `+path+` permissions.allow "Bash(ls:*)"`) {
		t.Errorf("the adoption must be named:\n%s", warnings)
	}
	if got := readGlobalTest(t, path); got != userClaudeSettings {
		t.Errorf("adopting must not edit the file:\n%s", got)
	}
	if recorded := readGlobalPermissionState(t, source); !reflect.DeepEqual(recorded, map[string][]string{"allow": {"Bash(ls:*)"}}) {
		t.Errorf("the adopted rule must be recorded: %v", recorded)
	}
}

func TestSyncGlobal_PermissionRulesMergeTargetBlock(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, userClaudeSettings)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "a.yaml"), "permissions:\n  allow: [Read]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "b.yaml"), "x-claude:\n  permissions:\n    allow: [\"mcp__docs\"]\n    ask: [\"Bash(git push:*)\"]\n")

	_, warnings, err := runGlobalAgentTest("--only", "claude")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if strings.Contains(warnings, "x-claude.permissions") {
		t.Errorf("rule lists under x-claude are written, not noted:\n%s", warnings)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"allow": []any{"Bash(ls:*)", "Read", "mcp__docs"}, "ask": []any{"Bash(git push:*)"}, "defaultMode": "auto"}
	if got := doc["permissions"]; !reflect.DeepEqual(got, want) {
		t.Errorf("permissions = %v, want %v", got, want)
	}
}

func TestSyncGlobal_PermissionRulesCreateAndRemoveTheFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	rules := filepath.Join(source, "settings", "rules.yaml")
	mustWriteGlobalTest(t, rules, "permissions:\n  deny: [\"Bash(rm -rf:*)\"]\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); !strings.Contains(got, `"Bash(rm -rf:*)"`) {
		t.Fatalf("settings.json:\n%s", got)
	}
	if err := os.Remove(rules); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after removing the spec: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a settings file sync created goes once empty: %v", err)
	}
}

func TestSyncGlobal_PermissionRulesStopOnAnUnreadableRule(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "rules.yaml"), "permissions:\n  deny: [\"write(.env)\"]\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err == nil {
		t.Error("an unreadable deny rule must stop the sync")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("nothing may be written: %v", err)
	}
}

func TestExplainGlobal_PermissionRulesNameTheirList(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "rules.yaml"), "permissions:\n  deny: [\"Bash(rm:*)\"]\n")
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"explain", "--global", "settings/rules.yaml"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explain: %v\n%s", err, out.String())
	}
	want := filepath.ToSlash(filepath.Join(home, ".claude", "settings.json")) + ` (key "permissions.deny")`
	if !strings.Contains(out.String(), want) {
		t.Errorf("explain must name the list:\n%s", out.String())
	}
}
