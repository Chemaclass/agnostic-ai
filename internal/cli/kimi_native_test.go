package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportKimi_NativeRoundTrip(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\noutputs:\n  kimi:\n    conf-file: profile/config.toml\n")
	for path, body := range map[string]string{
		".kimi-code/AGENTS.md":                  "# Project\n\nUse small changes.\n",
		"backend/.kimi-code/AGENTS.md":          "Keep explicit errors.\n",
		".kimi-code/agents/reviewer.md":         "---\nname: reviewer\ndescription: Review changes\ntools: []\ndisallowedTools: [Bash]\n---\nReview the patch.\n",
		".kimi-code/skills/check/SKILL.md":      "---\nname: check\ndescription: Check changes\ndisableModelInvocation: true\narguments: [path]\n---\nCheck the patch.\n",
		".kimi-code/skills/check/reference.txt": "reference bytes\n",
		".kimi-code/skills/manual.md":           "---\nname: manual\ndescription: A manual skill\ndisable_model_invocation: true\n---\nManual body.\n",
		".kimi-code/mcp.json":                   "{\"mcpServers\":{\"docs\":{\"transport\":\"sse\",\"url\":\"https://example.com/mcp\",\"enabled\":false,\"enabledTools\":[],\"bearerTokenEnvVar\":\"TOKEN\"},\"literal\":{\"command\":\"server\",\"env\":{\"MESSAGE\":\"${LITERAL}\"}}}}\n",
		"profile/config.toml":                   "default_model='existing-model'\n[thinking]\neffort='high'\n[[hooks]]\nevent='PreToolUse'\ncommand='check'\nmatcher='Bash'\ntimeout=5\n",
	} {
		writeFile(t, path, body)
	}
	execCLI(t, "import", "kimi")
	execCLI(t, "sync", "-t", "kimi")
	for path, want := range map[string]string{".kimi-code/AGENTS.md": "Use small changes.", "backend/.kimi-code/AGENTS.md": "Keep explicit errors.", ".kimi-code/agents/reviewer.md": "tools: []", ".kimi-code/skills/check/SKILL.md": "disableModelInvocation: true", ".kimi-code/skills/check/reference.txt": "reference bytes\n", ".kimi-code/skills/manual/SKILL.md": "disableModelInvocation: true", "profile/config.toml": "event = \"PreToolUse\""} {
		if got := readFile(t, filepath.Join(dir, path)); !strings.Contains(got, want) {
			t.Errorf("%s missing %q: %s", path, want, got)
		}
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, ".kimi-code/mcp.json")), &doc); err != nil {
		t.Fatal(err)
	}
	servers := doc["mcpServers"].(map[string]any)
	docs := servers["docs"].(map[string]any)
	if docs["enabled"] != false || docs["transport"] != "sse" || docs["bearerTokenEnvVar"] != "TOKEN" {
		t.Errorf("native server changed: %#v", docs)
	}
	if got := servers["literal"].(map[string]any)["env"].(map[string]any)["MESSAGE"]; got != "${LITERAL}" {
		t.Errorf("literal changed: %v", got)
	}
	execCLI(t, "sync", "--check", "-t", "kimi")
	execCLI(t, "lint")
}
func TestSyncGlobal_KimiRestoresConfigAndUserHooks(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "current-kimi")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "config.toml")
	original := "# user settings\n[thinking]\neffort='low'\nenabled=true\n[[hooks]]\nevent='Stop'\ncommand='user-hook'\n"
	mustWriteGlobalTest(t, path, original)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "settings/kimi.yaml"), "model:\n  kimi: existing-model\nx-kimi:\n  telemetry: false\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks/guard.yaml"), "name: guard\ntarget: kimi\nevent: PreToolUse\ncommand: check\nmatcher: Bash\ntimeout: 5\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents/reviewer.md"), "---\nname: reviewer\ndescription: Review changes\ncan: []\n---\nReview changes.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/docs.yaml"), "name: docs\ncommand: server\ndisabled: true\n")
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("sync: %v %s", err, w)
	}
	got := readGlobalTest(t, path)
	for _, want := range []string{original, "default_model = \"existing-model\"", "telemetry = false", "event = \"PreToolUse\""} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "name =") {
		t.Errorf("hook name is not a native field: %s", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Error("private mode widened")
	}
	if _, _, err := runGlobalAgentTest("--only", "kimi", "--check"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"settings/kimi.yaml", "hooks/guard.yaml", "agents/reviewer.md", "mcps/docs.yaml"} {
		if err := os.Remove(filepath.Join(source, f)); err != nil {
			t.Fatal(err)
		}
	}
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("remove: %v %s", err, w)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("config bytes changed: got %q want %q", got, original)
	}
	if _, err := os.Stat(filepath.Join(home, ".kimi")); !os.IsNotExist(err) {
		t.Errorf("legacy home touched: %v", err)
	}
}

func TestImportGlobal_KimiAdoptsNativeHooksAndMCP(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	root := filepath.Join(home, "kimi-current")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "config.toml")
	before := "# mine\ndefault_model='existing-model'\n[[hooks]]\nevent='Stop'\ncommand='check'\ntimeout=5\n"
	mcp := filepath.Join(root, "mcp.json")
	beforeMCP := "{\"mcpServers\":{\"docs\":{\"transport\":\"sse\",\"url\":\"https://example.com/mcp\",\"enabled\":false,\"enabledTools\":[]}}}\n"
	mustWriteGlobalTest(t, path, before)
	mustWriteGlobalTest(t, mcp, beforeMCP)
	if _, w, err := runImportGlobalTest("kimi"); err != nil {
		t.Fatalf("import: %v %s", err, w)
	}
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("adopt: %v %s", err, w)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("config changed on adoption: %q", got)
	}
	if got := readGlobalTest(t, mcp); got != beforeMCP {
		t.Errorf("MCP changed on adoption: %q", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "kimi", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_KimiEmptyMCPFilterIsNotUnrestricted(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "kimi")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "mcp.json")
	before := `{"mcpServers":{"docs":{"transport":"stdio","command":"server"}}}` + "\n"
	mustWriteGlobalTest(t, path, before)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/docs.yaml"), "name: docs\ncommand: server\nx-kimi:\n  enabledTools: []\n")
	if _, _, err := runGlobalAgentTest("--only", "kimi"); err == nil {
		t.Fatal("empty allowlist was adopted as unrestricted user server")
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("conflict changed user server: %s", got)
	}
}

func TestImportKimi_RefusesCredentialBearingMetadata(t *testing.T) {
	for _, path := range []string{".kimi-code/agents/reviewer.md", ".kimi-code/skills/reviewer/SKILL.md", "profile/config.toml"} {
		t.Run(path, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\noutputs:\n  kimi:\n    conf-file: profile/config.toml\n")
			body := "---\nname: reviewer\ndescription: Review changes\napi_key: sk-live-abcdef1234567890\n---\nReview changes.\n"
			if strings.HasSuffix(path, ".toml") {
				body = "api_key='sk-live-abcdef1234567890'\n"
			}
			writeFile(t, path, body)
			if _, err := runCLI(t, "import", "kimi"); err == nil {
				t.Fatal("credential-bearing native metadata imported")
			}
		})
	}
}
func TestSyncGlobal_KimiCreatedConfigRemovedAndEffortLeftAlone(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "kimi")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "config.toml")
	specPath := filepath.Join(source, "settings/kimi.yaml")
	mustWriteGlobalTest(t, specPath, "model: existing-model\neffort: high\nx-kimi:\n  telemetry: false\n  thinking:\n    enabled: false\n")
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("create: %v %s", err, w)
	}
	got := readGlobalTest(t, path)
	if strings.Contains(got, "thinking") || strings.Contains(got, "effort") {
		t.Errorf("global nested table was owned: %s", got)
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("remove: %v %s", err, w)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("created settings remains: %v", err)
	}
}
func TestImportKimi_FolderSkillWinsAndFlatDescriptionMatchesNative(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\n")
	writeFile(t, ".kimi-code/skills/review/SKILL.md", "---\nname: review\ndescription: Folder description\n---\nFolder body.\n")
	writeFile(t, ".kimi-code/skills/review.md", "Flat body.\n")
	writeFile(t, ".kimi-code/skills/plain.md", "Plain skill description.\nMore text.\n")
	execCLI(t, "import", "kimi")
	execCLI(t, "sync", "-t", "kimi")
	if got := readFile(t, ".kimi-code/skills/review/SKILL.md"); !strings.Contains(got, "Folder body.") || strings.Contains(got, "Flat body.") {
		t.Errorf("flat skill replaced folder: %s", got)
	}
	if got := readFile(t, ".kimi-code/skills/plain/SKILL.md"); !strings.Contains(got, "description: Plain skill description.") {
		t.Errorf("native flat description changed: %s", got)
	}
}
