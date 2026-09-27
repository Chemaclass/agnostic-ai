package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type globalPlanJSON struct {
	Command string `json:"command"`
	Writes  []struct {
		Target string   `json:"target"`
		Path   string   `json:"path"`
		Action string   `json:"action"`
		Keys   []string `json:"keys"`
	} `json:"writes"`
}

func TestSyncGlobal_PlanJSONListsSettingsKeysAndWritesNothing(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model:\n  codex: gpt-6-luna\n")
	codexPath := filepath.Join(home, ".codex", "config.toml")
	mustWriteGlobalTest(t, codexPath, "[tui]\ntheme = \"dark\"\n")

	out, _, err := runGlobalAgentTest("--only", "codex", "--plan", "--json")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	var plan globalPlanJSON
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	var found bool
	for _, w := range plan.Writes {
		if w.Path == filepath.ToSlash(codexPath) {
			found = true
			if w.Action != "update" || w.Target != "codex" || len(w.Keys) != 1 || !strings.Contains(w.Keys[0], `set model = "gpt-6-luna"`) {
				t.Errorf("record = %+v", w)
			}
		}
	}
	if !found || plan.Command != "sync --global --plan" {
		t.Errorf("plan = %+v", plan)
	}
	if got := readGlobalTest(t, codexPath); got != "[tui]\ntheme = \"dark\"\n" {
		t.Errorf("plan must write nothing: %q", got)
	}
	if _, err := os.Stat(filepath.Join(source, "state")); !os.IsNotExist(err) {
		t.Errorf("plan must not record state: %v", err)
	}

	text, _, err := runGlobalAgentTest("--only", "codex", "--plan")
	if err != nil || !strings.Contains(text, "update "+filepath.ToSlash(codexPath)+" [codex]") || !strings.Contains(text, `  set model = "gpt-6-luna"`) {
		t.Errorf("text plan: %v\n%s", err, text)
	}

	if _, _, err := runGlobalAgentTest("--only", "codex", "--json"); err != nil {
		t.Fatalf("sync --json: %v", err)
	}
	after, _, err := runGlobalAgentTest("--only", "codex", "--plan")
	if err != nil || strings.TrimSpace(after) != "No changes." {
		t.Errorf("plan after sync: %v\n%s", err, after)
	}
	checkOut, _, err := runGlobalAgentTest("--only", "codex", "--check", "--json")
	if err != nil {
		t.Errorf("check --json after sync: %v\n%s", err, checkOut)
	}
}

func TestSyncGlobal_CheckJSONReportsDrift(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model:\n  codex: gpt-6-luna\n")
	out, _, err := runGlobalAgentTest("--only", "codex", "--check", "--json")
	if err == nil {
		t.Fatal("drift must fail")
	}
	if !strings.Contains(out, `"action": "missing"`) {
		t.Errorf("check json:\n%s", out)
	}
}

func TestSyncGlobal_PlanRejectsCheck(t *testing.T) {
	globalAgentTestHome(t)
	if _, _, err := runGlobalAgentTest("--plan", "--check"); err == nil {
		t.Error("--plan with --check must fail")
	}
}
