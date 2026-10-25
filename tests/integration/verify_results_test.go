package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerify_RecordsAndComparesInstructionChange(t *testing.T) {
	binary := handoffBuildCLI(t)
	verifier, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERIFY_RESULTS_INTEGRATION_HELPER", "1")
	dir := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	write := func(path, body string, mode os.FileMode) {
		t.Helper()
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [codex]\nbuiltins: []\nverify:\n  command: [%q, '-test.run=^TestVerifyResultsIntegrationProcess$', AGENTS.md]\n", verifier), 0o600)
	write(".agnostic-ai/AGNOSTIC_AI.md", "Accept.\n", 0o600)
	run := func(want int, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				code = e.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if code != want {
			t.Fatalf("%v: exit %d, want %d\n%s", args, code, want, out)
		}
		return out
	}
	run(0, "sync")
	run(0, "verify", "--record", "before.json", "--evaluation", "local-suite-v1")
	write(".agnostic-ai/AGNOSTIC_AI.md", "Reject.\n", 0o600)
	run(0, "sync")
	run(23, "verify", "--record", "after.json", "--evaluation", "local-suite-v1")
	out := run(0, "verify", "compare", "before.json", "after.json", "--json")
	var comparison struct {
		Version int `json:"version"`
		Targets []struct {
			Status        string `json:"status"`
			Before, After struct {
				Fingerprint string `json:"harness_fingerprint"`
			}
		} `json:"targets"`
	}
	if err := json.Unmarshal(out, &comparison); err != nil {
		t.Fatalf("comparison: %v\n%s", err, out)
	}
	if comparison.Version != 1 || len(comparison.Targets) != 1 || comparison.Targets[0].Status != "regression" || comparison.Targets[0].Before.Fingerprint == comparison.Targets[0].After.Fingerprint {
		t.Errorf("comparison = %s", out)
	}
}

func TestVerifyResultsIntegrationProcess(t *testing.T) {
	if os.Getenv("VERIFY_RESULTS_INTEGRATION_HELPER") != "1" {
		return
	}
	var context struct {
		Version int    `json:"version"`
		Target  string `json:"target"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&context); err != nil || context.Version != 1 || context.Target != "codex" {
		os.Exit(90)
	}
	instruction, err := os.ReadFile("AGENTS.md")
	if err != nil {
		os.Exit(91)
	}
	if strings.Contains(string(instruction), "Accept.") {
		os.Exit(0)
	}
	os.Exit(23)
}
