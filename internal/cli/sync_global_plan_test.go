package cli

import (
	"encoding/json"
	"fmt"
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

func TestSyncGlobal_PlanCheckGatesWithoutWriting(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", jsonOut), func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			input := filepath.Join(source, "AGNOSTIC_AI.md")
			mustWriteGlobalTest(t, input, "Be brief.\n")
			args := []string{"--only", "claude", "--plan", "--check"}
			if jsonOut {
				args = append(args, "--json")
			}
			out, _, err := runGlobalAgentTest(args...)
			if err == nil || !strings.Contains(err.Error(), "drift detected") {
				t.Errorf("missing output: %v\n%s", err, out)
			}
			if jsonOut {
				var plan globalPlanJSON
				if err := json.Unmarshal([]byte(out), &plan); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if plan.Command != "sync --global --plan" || len(plan.Writes) == 0 || plan.Writes[0].Action != "create" {
					t.Errorf("plan = %+v", plan)
				}
			} else if !strings.Contains(out, "create ") {
				t.Errorf("plan: %s", out)
			}
			if _, err := os.Stat(filepath.Join(source, "state")); !os.IsNotExist(err) {
				t.Errorf("preview created state: %v", err)
			}
			output := filepath.Join(home, ".claude", "CLAUDE.md")
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Errorf("preview created output: %v", err)
			}
			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("sync: %v", err)
			}
			if out, _, err := runGlobalAgentTest(args...); err != nil {
				t.Errorf("clean: %v\n%s", err, out)
			}
			before := readGlobalTest(t, output)
			state := readGlobalTest(t, filepath.Join(source, "state", "global.json"))
			mustWriteGlobalTest(t, input, "Be short.\n")
			if out, _, err := runGlobalAgentTest(args...); err == nil || !strings.Contains(err.Error(), "drift detected") {
				t.Errorf("changed source: %v\n%s", err, out)
			}
			if got := readGlobalTest(t, output); got != before {
				t.Error("preview changed output")
			}
			if got := readGlobalTest(t, filepath.Join(source, "state", "global.json")); got != state {
				t.Error("preview changed state")
			}
			if _, err := os.Stat(output + ".bak"); !os.IsNotExist(err) {
				t.Errorf("preview created backup: %v", err)
			}
		})
	}
}

func TestSyncGlobal_PlanCheckPreservesLoadFailure(t *testing.T) {
	_, source := globalAgentTestHome(t)
	path := filepath.Join(source, "settings", "bad.yaml")
	mustWriteGlobalTest(t, path, "model: [\n")
	for _, args := range [][]string{{"--plan", "--check"}, {"--plan", "--check", "--json"}} {
		_, _, err := runGlobalAgentTest(args...)
		if err == nil || strings.Contains(err.Error(), "drift detected") || !strings.Contains(err.Error(), "bad.yaml") {
			t.Errorf("load failure: %v", err)
		}
	}
}

func TestSyncGlobal_PlanRejectsDryRun(t *testing.T) {
	globalAgentTestHome(t)
	if _, _, err := runGlobalAgentTest("--plan", "--dry-run"); err == nil {
		t.Error("--plan with --dry-run must fail")
	}
}

func TestSyncGlobal_JSONRunPrunesEmptySkillFolder(t *testing.T) {
	home, source := globalAgentTestHome(t)
	skill := filepath.Join(source, "skills", "foo", "SKILL.md")
	mustWriteGlobalTest(t, skill, "---\nname: foo\ndescription: Foo\n---\nFoo.\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if err := os.RemoveAll(filepath.Dir(skill)); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "claude", "--json"); err != nil {
		t.Fatalf("sync --json: %v\n%s", err, w)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "foo")); !os.IsNotExist(err) {
		t.Errorf("the emptied skill folder must go: %v", err)
	}
}

func TestSyncGlobal_PlanMarksHandEdit(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be brief.\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	claudeMD := filepath.Join(home, ".claude", "CLAUDE.md")
	data := readGlobalTest(t, claudeMD)
	mustWriteGlobalTest(t, claudeMD, strings.Replace(data, "Be brief.", "Be terse.", 1))
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be short.\n")
	out, _, err := runGlobalAgentTest("--only", "claude", "--plan")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(out, "conflict: edited since the last global sync") {
		t.Errorf("plan must mark the hand edit:\n%s", out)
	}
}

func TestSyncGlobal_PlanCheckDoesNotGateOnOwnershipOnly(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be brief.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(source, "state", "global.json")
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--plan", "--check"}, {"--plan", "--check", "--json"}} {
		out, _, err := runGlobalAgentTest(append([]string{"--only", "claude"}, args...)...)
		if err != nil {
			t.Errorf("ownership-only plan: %v\n%s", err, out)
		}
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("preview restored state: %v", err)
	}
}
