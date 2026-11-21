package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCodebuddy_UserFilesRestoreOriginalBytes(t *testing.T) {
	home, source := userAgentTestHome(t)
	settings := filepath.Join(home, ".codebuddy/settings.json")
	mcp := filepath.Join(home, ".codebuddy/mcp.json")
	beforeSettings := "{\n  \"nativeControl\" : true,\n  \"hooks\": {\n    \"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"echo mine\"}]}]\n  }\n}\n"
	beforeMCP := "{\n  \"projects\": {\"/my/project\": {\"localServer\":true}},\n  \"mcpServers\": {\n    \"mine\": {\"type\": \"stdio\", \"command\": \"mine\"}\n  }\n}\n"
	writeFile(t, settings, beforeSettings)
	writeFile(t, mcp, beforeMCP)
	if err := os.Chmod(mcp, 0600); err != nil {
		t.Fatal(err)
	}
	specs := map[string]string{"settings/defaults.yaml": "model: lite\neffort: high\npermissions:\n  default-mode: plan\n  deny: [shell(rm *)]\n", "hooks/guard.yaml": "event: Stop\ncommand: echo\nargs: ['a b']\n", "mcps/docs.yaml": userDocsMCP, "agents/reviewer.md": "---\nname: reviewer\ndescription: Review\ncan: [read]\n---\nReview changes.\n", "skills/review/SKILL.md": "---\nname: review\ndescription: Review\n---\nRead references/guide.txt.\n", "skills/review/references/guide.txt": "Use complete diff.\n"}
	for p, b := range specs {
		writeFile(t, filepath.Join(source, p), b)
	}
	if _, warnings, err := runUserAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	got := readFile(t, settings)
	for _, want := range []string{"nativeControl", "echo mine", "echo 'a b'", "reasoningEffort", "defaultMode", "Bash(rm *)"} {
		if !strings.Contains(got, want) {
			t.Errorf("settings missing %q: %s", want, got)
		}
	}
	got = readFile(t, mcp)
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
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("private mode changed: %v", info.Mode())
	}
	if got := readFile(t, filepath.Join(home, ".codebuddy/skills/review/references/guide.txt")); got != "Use complete diff.\n" {
		t.Fatal(got)
	}
	if got := readFile(t, filepath.Join(home, ".codebuddy/agents/reviewer.md")); !strings.Contains(got, "tools: Read") {
		t.Fatal(got)
	}
	if _, warnings, err := runUserAgentTest("--only", "codebuddy", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, warnings)
	}
	for p := range specs {
		if err := os.Remove(filepath.Join(source, p)); err != nil {
			t.Fatal(err)
		}
	}
	if _, warnings, err := runUserAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("remove: %v\n%s", err, warnings)
	}
	if got := readFile(t, settings); got != beforeSettings {
		t.Errorf("settings bytes changed:\n%q\nwant:\n%q", got, beforeSettings)
	}
	if got := readFile(t, mcp); got != beforeMCP {
		t.Errorf("mcp bytes changed:\n%q\nwant:\n%q", got, beforeMCP)
	}
}
func TestCodebuddy_UserCreatesPreferredPrivateMCPAndRemovesIt(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".codebuddy/.mcp.json")
	spec := filepath.Join(source, "mcps/docs.yaml")
	writeFile(t, spec, userDocsMCP)
	if _, w, err := runUserAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("created mode: %v", info.Mode())
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runUserAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("remove: %v\n%s", err, w)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("created file remains: %v", err)
	}
}
func TestCodebuddy_UserUnsupportedServerDoesNotDeleteManagedEntries(t *testing.T) {
	home, source := userAgentTestHome(t)
	spec := filepath.Join(source, "mcps/docs.yaml")
	writeFile(t, spec, userDocsMCP)
	if _, w, err := runUserAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	path := filepath.Join(home, ".codebuddy/.mcp.json")
	before := readFile(t, path)
	writeFile(t, spec, userDocsMCP+"disabled: true\n")
	if _, _, err := runUserAgentTest("--only", "codebuddy"); err == nil {
		t.Fatal("unsupported disabled server enabled or removed")
	}
	if got := readFile(t, path); got != before {
		t.Errorf("invalid renderer changed user servers: %s", got)
	}
}
func TestCodebuddy_ImportUserUsesLegacyAndPreservesNativeMCPExtras(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".codebuddy/mcp.json")
	writeFile(t, path, `{"projects":{"mine":{}},"mcpServers":{"docs":{"type":"stdio","command":"docs-mcp","defer_loading":true,"tools":{"read":{"defer_loading":false}}}}}`)
	if _, w, err := runImportUserTest("codebuddy"); err != nil {
		t.Fatalf("import: %v\n%s", err, w)
	}
	got := readFile(t, filepath.Join(source, "mcps/docs.yaml"))
	for _, want := range []string{"x-codebuddy:", "defer_loading:", "tools:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	before := readFile(t, path)
	if _, w, err := runUserAgentTest("--only", "codebuddy"); err != nil {
		t.Fatalf("adopt: %v\n%s", err, w)
	}
	if got := readFile(t, path); got != before {
		t.Fatalf("import/adopt rewrote native file: %s", got)
	}
	if _, w, err := runUserAgentTest("--only", "codebuddy", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, w)
	}
}

func TestCodebuddy_UserNativeConditionalRuleIsRejected(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nx-codebuddy:\n  alwaysApply: false\n  paths: ['*.go']\n---\nGo only.\n")
	if _, _, err := runUserAgentTest("--only", "codebuddy"); err == nil {
		t.Fatal("conditional rule broadened globally")
	}
}

func TestCodebuddy_UserPromptHookIsRejected(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "hooks/stop-check.yaml"), "name: stop-check\nevent: Stop\ntype: prompt\nprompt: check\n")
	if _, _, err := runUserAgentTest("--only", "codebuddy"); err == nil || !strings.Contains(err.Error(), "user settings take only command hooks") {
		t.Fatalf("prompt hook dropped from user settings without an error: %v", err)
	}
}
