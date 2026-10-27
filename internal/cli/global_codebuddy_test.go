package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodebuddy_GlobalFilesRestoreOriginalBytes(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".codebuddy/settings.json")
	mcp := filepath.Join(home, ".codebuddy/mcp.json")
	beforeSettings := "{\n  \"nativeControl\" : true,\n  \"hooks\": {\n    \"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"echo mine\"}]}]\n  }\n}\n"
	beforeMCP := "{\n  \"projects\": {\"/my/project\": {\"localServer\":true}},\n  \"mcpServers\": {\n    \"mine\": {\"type\": \"stdio\", \"command\": \"mine\"}\n  }\n}\n"
	mustWriteGlobalTest(t, settings, beforeSettings)
	mustWriteGlobalTest(t, mcp, beforeMCP)
	if err := os.Chmod(mcp, 0600); err != nil {
		t.Fatal(err)
	}
	specs := map[string]string{"settings/defaults.yaml": "model: lite\neffort: high\npermissions:\n  default-mode: plan\n  deny: [shell(rm *)]\n", "hooks/guard.yaml": "event: Stop\ncommand: echo\nargs: ['a b']\n", "mcps/docs.yaml": globalDocsMCP, "agents/reviewer.md": "---\nname: reviewer\ndescription: Review\ncan: [read]\n---\nReview changes.\n", "skills/review/SKILL.md": "---\nname: review\ndescription: Review\n---\nRead references/guide.txt.\n", "skills/review/references/guide.txt": "Use complete diff.\n"}
	for p, b := range specs {
		mustWriteGlobalTest(t, filepath.Join(source, p), b)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	got := readGlobalTest(t, settings)
	for _, want := range []string{"nativeControl", "echo mine", "echo 'a b'", "reasoningEffort", "defaultMode", "Bash(rm *)"} {
		if !strings.Contains(got, want) {
			t.Errorf("settings missing %q: %s", want, got)
		}
	}
	got = readGlobalTest(t, mcp)
	for _, want := range []string{"projects", "localServer", "\"type\": \"stdio\"", "docs-mcp"} {
		if !strings.Contains(got, want) {
			t.Errorf("mcp missing %q: %s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".codebuddy/.mcp.json")); !os.IsNotExist(err) {
		t.Errorf("legacy file was shadowed: %v", err)
	}
	info, err := os.Stat(mcp)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("private mode changed: %v", info.Mode())
	}
	if got := readGlobalTest(t, filepath.Join(home, ".codebuddy/skills/review/references/guide.txt")); got != "Use complete diff.\n" {
		t.Fatal(got)
	}
	if got := readGlobalTest(t, filepath.Join(home, ".codebuddy/agents/reviewer.md")); !strings.Contains(got, "tools: Read") {
		t.Fatal(got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, warnings)
	}
	for p := range specs {
		if err := os.Remove(filepath.Join(source, p)); err != nil {
			t.Fatal(err)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("remove: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, settings); got != beforeSettings {
		t.Errorf("settings bytes changed:\n%q\nwant:\n%q", got, beforeSettings)
	}
	if got := readGlobalTest(t, mcp); got != beforeMCP {
		t.Errorf("mcp bytes changed:\n%q\nwant:\n%q", got, beforeMCP)
	}
}
func TestCodebuddy_GlobalCreatesPreferredPrivateMCPAndRemovesIt(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".codebuddy/.mcp.json")
	spec := filepath.Join(source, "mcps/docs.yaml")
	mustWriteGlobalTest(t, spec, globalDocsMCP)
	if _, w, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("created mode: %v", info.Mode())
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("remove: %v\n%s", err, w)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("created file remains: %v", err)
	}
}
func TestCodebuddy_GlobalUnsupportedServerDoesNotDeleteManagedEntries(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "mcps/docs.yaml")
	mustWriteGlobalTest(t, spec, globalDocsMCP)
	if _, w, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	path := filepath.Join(home, ".codebuddy/.mcp.json")
	before := readGlobalTest(t, path)
	mustWriteGlobalTest(t, spec, globalDocsMCP+"disabled: true\n")
	if _, _, err := runGlobalAgentTest("--only", "codebuddy"); err == nil {
		t.Fatal("unsupported disabled server enabled or removed")
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("invalid renderer changed global servers: %s", got)
	}
}
func TestCodebuddy_ImportGlobalUsesLegacyAndPreservesNativeMCPExtras(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".codebuddy/mcp.json")
	mustWriteGlobalTest(t, path, `{"projects":{"mine":{}},"mcpServers":{"docs":{"type":"stdio","command":"docs-mcp","defer_loading":true,"tools":{"read":{"defer_loading":false}}}}}`)
	if _, w, err := runImportGlobalTest("codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, w)
	}
	got := readGlobalTest(t, filepath.Join(source, "mcps/docs.yaml"))
	for _, want := range []string{"x-codebuddy:", "defer_loading:", "tools:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	before := readGlobalTest(t, path)
	if _, w, err := runGlobalAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("adopt: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Fatalf("import/adopt rewrote native file: %s", got)
	}
	if _, w, err := runGlobalAgentTest("--only", "codebuddy", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, w)
	}
}

func TestCodebuddy_GlobalNativeConditionalRuleIsRejected(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nx-codebuddy:\n  alwaysApply: false\n  paths: ['*.go']\n---\nGo only.\n")
	if _, _, err := runGlobalAgentTest("--only", "codebuddy"); err == nil {
		t.Fatal("conditional rule broadened globally")
	}
}

func TestCodebuddy_GlobalPromptHookIsRejected(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks/stop-check.yaml"), "name: stop-check\nevent: Stop\ntype: prompt\nprompt: check\n")
	if _, _, err := runGlobalAgentTest("--only", "codebuddy"); err == nil || !strings.Contains(err.Error(), "user settings take only command hooks") {
		t.Fatalf("prompt hook dropped from user settings without an error: %v", err)
	}
}
