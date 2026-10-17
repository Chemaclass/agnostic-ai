package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_CopilotUserHooksUseNativeHomeAndValidate(t *testing.T) {
	for _, override := range []bool{false, true} {
		name := "default-home"
		if override {
			name = "COPILOT_HOME"
		}
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			native := filepath.Join(home, ".copilot")
			if override {
				native = filepath.Join(home, "custom copilot")
				t.Setenv("COPILOT_HOME", native)
			} else {
				t.Setenv("COPILOT_HOME", "")
			}
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\n")
			mustWriteGlobalTest(t, filepath.Join(source, "hooks", "session.yaml"), "name: session\ntarget: copilot\nevent: SessionStart\ncommand: echo ready\n")
			if _, warnings, err := runGlobalCheck("validate"); err != nil {
				t.Errorf("validate Copilot user hook: %v\n%s", err, warnings)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "copilot"); err != nil {
				t.Fatalf("sync Copilot user hook: %v\n%s", err, warnings)
			}
			path := filepath.Join(native, "hooks", "agnostic-ai.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read native user hook %s: %v", path, err)
			}
			var doc struct {
				Version int                         `json:"version"`
				Hooks   map[string][]map[string]any `json:"hooks"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("parse native hooks: %v", err)
			}
			if doc.Version != 1 || len(doc.Hooks["SessionStart"]) != 1 || doc.Hooks["SessionStart"][0]["command"] != "echo ready" {
				t.Errorf("native user hooks: %s", raw)
			}
			if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
				t.Errorf("check synced Copilot user hooks: %v", err)
			}
		})
	}
}

func TestImportGlobal_CopilotUserHookAdoptionDoesNotDuplicateCommands(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	path := filepath.Join(home, ".copilot", "hooks", "personal.json")
	native := `{"version":1,"hooks":{"SessionStart":[{"type":"command","bash":"echo personal","powershell":"Write-Output personal","timeoutSec":9}]}}`
	mustWriteGlobalTest(t, path, native)
	if _, warnings, err := runImportGlobalTest("copilot", "--dry-run"); err != nil {
		t.Fatalf("dry-run import: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(filepath.Join(source, "hooks")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote hooks: %v", err)
	}
	if _, warnings, err := runImportGlobalTest("copilot"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	specs, err := os.ReadDir(filepath.Join(source, "hooks"))
	if err != nil || len(specs) != 1 {
		t.Fatalf("imported hook specs: %v, %v", specs, err)
	}
	for _, command := range [][]string{{"validate"}, {"lint"}, {"sync"}, {"sync", "--check"}} {
		if _, warnings, err := runGlobalCheck(command...); err != nil {
			t.Fatalf("%v: %v\n%s", command, err, warnings)
		}
	}
	if _, warnings, err := runImportGlobalTest("copilot"); err != nil {
		t.Fatalf("repeated import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != native {
		t.Errorf("adoption changed handwritten native hook: %s", got)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		var doc struct {
			Hooks map[string][]map[string]any `json:"hooks"`
		}
		raw := readGlobalTest(t, filepath.Join(filepath.Dir(path), file.Name()))
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatalf("parse %s: %v", file.Name(), err)
		}
		for _, handlers := range doc.Hooks {
			for _, handler := range handlers {
				if handler["bash"] == "echo personal" && handler["powershell"] == "Write-Output personal" && handler["timeoutSec"] == float64(9) {
					count++
				}
			}
		}
	}
	if count != 1 {
		t.Errorf("imported command appears %d times across all native user files, want once", count)
	}
}

func TestImportGlobal_CopilotSkipsUnknownFieldsAndPreservesNeighbors(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	native := `{"version":1,"hooks":{"PreToolUse":[{"type":"command","command":"guard","futurePolicy":true}]}}`
	path := filepath.Join(home, ".copilot", "hooks", "future.json")
	mustWriteGlobalTest(t, path, native)
	_, warnings, err := runImportGlobalTest("copilot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, path) || !strings.Contains(warnings, "cannot be imported without changing behavior") {
		t.Errorf("missing precise skip: %s", warnings)
	}
	if _, err := os.Stat(filepath.Join(source, "hooks")); !os.IsNotExist(err) {
		t.Errorf("lossy specs written: %v", err)
	}
	if got := readGlobalTest(t, path); got != native {
		t.Errorf("unknown native fields changed: %s", got)
	}
}

func TestSyncGlobal_CopilotEditedImportRefusesBeforeWrites(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	path := filepath.Join(home, ".copilot", "hooks", "personal.json")
	mustWriteGlobalTest(t, path, `{"version":1,"hooks":{"SessionStart":[{"type":"command","command":"echo original"}]}}`)
	if _, _, err := runImportGlobalTest("copilot"); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Join(source, "hooks"))
	if err != nil || len(files) != 1 {
		t.Fatalf("imported specs: %v, %v", files, err)
	}
	specPath := filepath.Join(source, "hooks", files[0].Name())
	edited := strings.ReplaceAll(readGlobalTest(t, specPath), "echo original", "echo changed")
	mustWriteGlobalTest(t, specPath, edited)
	_, _, err = runGlobalAgentTest("--only", "copilot")
	if err == nil || !strings.Contains(err.Error(), "differs from its source file") {
		t.Fatalf("want nonduplicating refusal, got %v", err)
	}
	if got := readGlobalTest(t, path); strings.Contains(got, "echo changed") {
		t.Errorf("refusal altered original: %s", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "agnostic-ai.json")); !os.IsNotExist(err) {
		t.Errorf("refusal generated duplicate hook: %v", err)
	}
}

func TestImportGlobal_CopilotManagedHandlersPreserveAllSupportedFields(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	native := `{"version":1,"hooks":{"preToolUse":[{"type":"command","exec":"guard","args":["a b"],"matcher":"bash|edit","timeoutSec":8,"cwd":"sub","env":{"LOCAL":"value"}},{"type":"http","url":"https://example.test/hooks","headers":{"X-Local":"value"},"allowedEnvVars":["LOCAL"],"timeoutSec":7}],"SessionStart":[{"type":"prompt","prompt":"Check session","timeoutSec":5}]}}`
	path := filepath.Join(home, ".copilot", "hooks", "agnostic-ai.json")
	mustWriteGlobalTest(t, path, native)
	if _, warnings, err := runImportGlobalTest("copilot"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	files, err := os.ReadDir(filepath.Join(source, "hooks"))
	if err != nil || len(files) != 3 {
		t.Fatalf("supported native entries imported: %v, %v", files, err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != native {
		t.Errorf("native field preservation: %s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestImportGlobal_CopilotReconstructsInFreshHomeAndKeepsUserOwnedOriginal(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("COPILOT_HOME", "")
	native := `{"version":1,"hooks":{"SessionStart":[{"type":"command","command":"echo personal"}]}}`
	original := filepath.Join(home, ".copilot", "hooks", "personal.json")
	mustWriteGlobalTest(t, original, native)
	if _, _, err := runImportGlobalTest("copilot"); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(home, "fresh native home")
	t.Setenv("COPILOT_HOME", fresh)
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	got := readGlobalJSON(t, filepath.Join(fresh, "hooks", "agnostic-ai.json"))
	hooks := got["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(hooks) != 1 || hooks[0].(map[string]any)["command"] != "echo personal" {
		t.Errorf("fresh native handlers: %v", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", "")
	specs, err := os.ReadDir(filepath.Join(source, "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range specs {
		if err := os.Remove(filepath.Join(source, "hooks", file.Name())); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatal(err)
	}
	if got := readGlobalTest(t, original); got != native {
		t.Errorf("removing the spec removed user-owned hook: %s", got)
	}
}

func TestSyncGlobal_CopilotMemoryAndPortableHookUseGlobalPaths(t *testing.T) {
	home, source := globalAgentTestHome(t)
	native := filepath.Join(home, "native home")
	t.Setenv("COPILOT_HOME", native)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [copilot]\nbuiltins: [memory]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\ntarget: copilot\non: before-tool\ncommand: echo guard\n")
	if _, _, err := runGlobalAgentTest("--only", "copilot"); err != nil {
		t.Fatal(err)
	}
	doc := readGlobalJSON(t, filepath.Join(native, "hooks", "agnostic-ai.json"))
	hooks := doc["hooks"].(map[string]any)
	starts, _ := hooks["SessionStart"].([]any)
	bash := ""
	if len(starts) == 1 {
		bash, _ = starts[0].(map[string]any)["bash"].(string)
	}
	if len(starts) != 1 || !strings.Contains(bash, "hook memory --target copilot") {
		t.Errorf("memory target command: %v", starts)
	}
	before, _ := hooks["PreToolUse"].([]any)
	if len(before) != 1 {
		t.Fatalf("portable guard: %v", before)
	}
	command, _ := before[0].(map[string]any)["command"].(string)
	wrapper := filepath.ToSlash(filepath.Join(native, "hooks", "hooks", "agnostic-ai-portable-hook.sh"))
	if !strings.Contains(command, wrapper) || strings.Contains(command, ".github/hooks/") {
		t.Errorf("portable wrapper command does not use copied global file: %s", command)
	}
	if _, err := os.Stat(filepath.FromSlash(wrapper)); err != nil {
		t.Errorf("missing global wrapper: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "copilot", "--check"); err != nil {
		t.Fatal(err)
	}
}
