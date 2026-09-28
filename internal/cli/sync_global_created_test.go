package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncGlobal_RemovesAUserFileItCreatedOnceEmpty(t *testing.T) {
	cases := map[string]struct {
		target, file string
		specs        map[string]string
	}{
		"augment hooks and mcp":     {"augment", ".augment/settings.json", map[string]string{"hooks/start.yaml": "event: SessionStart\ncommand: ~/s.sh\n", "mcps/docs.yaml": globalDocsMCP}},
		"claude hooks and settings": {"claude", ".claude/settings.json", map[string]string{"hooks/start.yaml": "event: SessionStart\ncommand: echo hi\n", "settings/d.yaml": "model: opus\n"}},
		"gemini settings and mcp":   {"gemini", ".gemini/settings.json", map[string]string{"settings/d.yaml": "model:\n  gemini: gemini-3-pro\n", "mcps/docs.yaml": globalDocsMCP}},
		"codex settings only":       {"codex", ".codex/config.toml", map[string]string{"settings/d.yaml": "model:\n  codex: gpt-6-luna\n"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			for rel, body := range tc.specs {
				mustWriteGlobalTest(t, filepath.Join(source, filepath.FromSlash(rel)), body)
			}
			if _, w, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync: %v\n%s", err, w)
			}
			path := filepath.Join(home, filepath.FromSlash(tc.file))
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("sync must create %s: %v", tc.file, err)
			}
			for rel := range tc.specs {
				if err := os.Remove(filepath.Join(source, filepath.FromSlash(rel))); err != nil {
					t.Fatal(err)
				}
			}
			if _, w, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync after removal: %v\n%s", err, w)
			}
			if data, err := os.ReadFile(path); err == nil {
				t.Errorf("%s must be gone, holds %q", tc.file, data)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
				t.Errorf("check after removal: %v", err)
			}
		})
	}
}

func TestSyncGlobal_KeepsACreatedFileTheUserAddedTo(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "settings", "d.yaml")
	mustWriteGlobalTest(t, spec, "model: opus\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\n  \"model\": \"opus\",\n  \"theme\": \"dark\"\n}\n")
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, path); got != "{\n  \"theme\": \"dark\"\n}\n" {
		t.Errorf("the user's key must stay: %q", got)
	}
}
