package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportGlobal_KiroHookRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".kiro", "hooks", "guard.json")
	native := `{"version":"v1","hooks":[{"name":"guard","trigger":"PreToolUse","matcher":"write","action":{"type":"command","command":"guard.sh"},"timeout":0}]}`
	mustWriteGlobalTest(t, path, native)
	out, warnings, err := runImportGlobalTest("kiro")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(source, "hooks", "guard.yaml")); err != nil {
		t.Fatalf("%v; output %s; warnings %s", err, out, warnings)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != native {
		t.Errorf("native bytes changed: %s", raw)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runImportGlobalTest("kiro"); err != nil {
		t.Fatal(err)
	}
}

func TestImportGlobal_KiroSkipsHookFilesItCannotPreserve(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".kiro", "hooks", "mixed.json")
	native := `{"version":"v1","hooks":[{"name":"mixed","trigger":"Stop","action":{"type":"command","command":"first"}},{"name":"mixed-2","trigger":"PreToolUse","action":{"type":"command","command":"second"}}]}`
	mustWriteGlobalTest(t, path, native)
	_, warnings, err := runImportGlobalTest("kiro")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, "skipped hook file") {
		t.Errorf("missing skip reason: %s", warnings)
	}
	if _, err := os.Stat(filepath.Join(source, "hooks")); !os.IsNotExist(err) {
		t.Errorf("import wrote partial hook specs: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != native {
		t.Errorf("native file changed: %s", raw)
	}
}
