package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_GoosePluginHooks(t *testing.T) {
	home, source := globalAgentTestHome(t)
	plugin := filepath.Join(home, ".agents", "plugins", "agnostic-ai-global")
	hooksFile := filepath.Join(plugin, "hooks", "hooks.json")
	manifest := filepath.Join(plugin, "plugin.json")
	specPath := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, specPath, "name: guard\nevent: PreToolUse\nmatcher: developer__shell\ncommand: echo guard\n")
	out, _, err := runGlobalAgentTest("--only", "goose", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, hooksFile) {
		t.Errorf("preview missing hooks file: %s", out)
	}
	if _, err := os.Stat(hooksFile); !os.IsNotExist(err) {
		t.Errorf("preview wrote hooks file: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "goose"); err != nil {
		t.Fatal(err)
	}
	doc := readGlobalJSON(t, hooksFile)
	groups, _ := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(groups) != 1 {
		t.Fatalf("hooks: %v", doc)
	}
	if name := readGlobalJSON(t, manifest)["name"]; name != "agnostic-ai-global" {
		t.Errorf("manifest name: %v", name)
	}
	if _, _, err := runGlobalAgentTest("--only", "goose", "--check"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "goose"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{hooksFile, manifest} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("removed source left %s: %v", path, err)
		}
	}
}

func TestSyncGlobal_GooseKeepsUnmanagedPlugin(t *testing.T) {
	home, source := globalAgentTestHome(t)
	hooksFile := filepath.Join(home, ".agents", "plugins", "agnostic-ai-global", "hooks", "hooks.json")
	mustWriteGlobalTest(t, hooksFile, `{"hooks":{}}`)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: Stop\ncommand: echo done\n")
	if _, _, err := runGlobalAgentTest("--only", "goose"); err == nil {
		t.Fatal("unmanaged hooks file overwritten")
	}
	if data, _ := os.ReadFile(hooksFile); string(data) != `{"hooks":{}}` {
		t.Errorf("unmanaged file changed: %s", data)
	}
}

func TestSyncGlobal_GooseHookScriptPaths(t *testing.T) {
	home, source := globalAgentTestHome(t)
	scripts := filepath.Join(home, ".agents", "plugins", "agnostic-ai-global", "hooks")
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "no-force-push.sh"), "#!/bin/sh\nexit 0\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "quoted.yaml"), "name: quoted\nevent: PreToolUse\ncommand: '\"$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/no-force-push.sh\"'\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "bare.yaml"), "name: bare\nevent: Stop\ncommand: $CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/no-force-push.sh\n")
	if _, _, err := runGlobalAgentTest("--only", "goose"); err != nil {
		t.Fatal(err)
	}
	doc := readGlobalJSON(t, filepath.Join(scripts, "hooks.json"))
	const goosePrefix = "export AGNOSTIC_AI_TARGET=goose; "
	want := map[string]string{
		"PreToolUse": goosePrefix + `"` + filepath.ToSlash(scripts) + `/no-force-push.sh"`,
		"Stop":       goosePrefix + filepath.ToSlash(scripts) + "/no-force-push.sh",
	}
	for event, command := range want {
		groups := doc["hooks"].(map[string]any)[event].([]any)
		got := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"]
		if got != command {
			t.Errorf("%s command = %v, want %s", event, got, command)
		}
	}
	if _, err := os.Stat(filepath.Join(scripts, "no-force-push.sh")); err != nil {
		t.Errorf("script missing: %v", err)
	}
}

func TestSyncGlobal_GooseKeepsProjectPluginName(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: Stop\ncommand: echo done\n")
	project := filepath.Join(home, "repo", ".agents", "plugins", "agnostic-ai", "hooks", "hooks.json")
	mustWriteGlobalTest(t, project, `{"hooks":{}}`)
	if _, _, err := runGlobalAgentTest("--only", "goose"); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(home, ".agents", "plugins")
	entries, err := os.ReadDir(user)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "agnostic-ai" {
			t.Errorf("user plugin shares the project plugin name %q", entry.Name())
		}
	}
	if data, _ := os.ReadFile(project); string(data) != `{"hooks":{}}` {
		t.Errorf("project plugin changed: %s", data)
	}
}
