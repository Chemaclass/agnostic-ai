package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_GoosePluginHooks(t *testing.T) {
	home, source := globalAgentTestHome(t)
	plugin := filepath.Join(home, ".agents", "plugins", "agnostic-ai")
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
	if name := readGlobalJSON(t, manifest)["name"]; name != "agnostic-ai" {
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
	hooksFile := filepath.Join(home, ".agents", "plugins", "agnostic-ai", "hooks", "hooks.json")
	mustWriteGlobalTest(t, hooksFile, `{"hooks":{}}`)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: Stop\ncommand: echo done\n")
	if _, _, err := runGlobalAgentTest("--only", "goose"); err == nil {
		t.Fatal("unmanaged hooks file overwritten")
	}
	if data, _ := os.ReadFile(hooksFile); string(data) != `{"hooks":{}}` {
		t.Errorf("unmanaged file changed: %s", data)
	}
}
