package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_SettingsAddThenRemoveKeepsParentsTheUserWrote(t *testing.T) {
	cases := []struct {
		name, original, spec string
	}{
		{"empty parent", "{\"env\":{}}\n", "x-claude:\n  env:\n    DEMO: hello\n"},
		{"spaced empty parent", "{\n  \"env\": { },\n  \"theme\": \"dark\"\n}\n", "x-claude:\n  env:\n    DEMO: hello\n"},
		{"nested empty parents", "{\"demo\":{\"outer\":{}}}\n", "x-claude:\n  demo:\n    outer:\n      inner:\n        value: 1\n"},
		{"parent sync creates under one the user wrote", "{\"demo\":{}}\n", "x-claude:\n  demo:\n    outer:\n      value: 1\n"},
		{"several managed children", "{\"env\":{}}\n", "x-claude:\n  env:\n    A: one\n    B: two\n"},
		{"surviving user child", "{\"env\":{\"MINE\":\"x\"}}\n", "x-claude:\n  env:\n    A: one\n    B: two\n"},
		{"parent sync creates", "{\"theme\":\"dark\"}\n", "x-claude:\n  env:\n    A: one\n  demo:\n    outer:\n      value: 1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			mustWriteGlobalTest(t, path, tc.original)
			settings := filepath.Join(source, "settings", "defaults.yaml")
			mustWriteGlobalTest(t, settings, "name: defaults\n"+tc.spec)
			if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			added := readGlobalTest(t, path)
			if added == tc.original {
				t.Fatalf("the managed settings must land:\n%s", added)
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("removal: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != tc.original {
				t.Errorf("add then remove must restore the original bytes\nwant: %q\ngot:  %q\nafter add:\n%s", tc.original, got, added)
			}
		})
	}
}

func TestSyncGlobal_SettingsRemovalDropsTheFileSyncCreated(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	settings := filepath.Join(source, "settings", "defaults.yaml")
	mustWriteGlobalTest(t, settings, "name: defaults\nx-claude:\n  env:\n    DEMO: hello\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if data, err := os.ReadFile(path); !os.IsNotExist(err) {
		t.Errorf("the settings file sync created must go once empty, got %q, %v", data, err)
	}
}

func TestSyncGlobal_SettingsPartialRemovalKeepsTheParentForTheRest(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"theme\":\"dark\"}\n"
	mustWriteGlobalTest(t, path, original)
	first := filepath.Join(source, "settings", "a.yaml")
	second := filepath.Join(source, "settings", "b.yaml")
	mustWriteGlobalTest(t, first, "name: a\nx-claude:\n  env:\n    A: one\n")
	mustWriteGlobalTest(t, second, "name: b\nx-claude:\n  env:\n    B: two\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	for _, spec := range []string{first, second} {
		if err := os.Remove(spec); err != nil {
			t.Fatal(err)
		}
		if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
			t.Fatalf("removal: %v\n%s", err, warnings)
		}
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("a parent sync created must go with its last managed child: %q, want %q", got, original)
	}
}

func TestSyncGlobal_SettingsParentOwnershipSurvivesEmptyObjectBecomingChild(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"theme\":\"dark\"}\n"
	mustWriteGlobalTest(t, path, original)
	settings := filepath.Join(source, "settings", "defaults.yaml")
	for _, value := range []string{"env: {}", "env:\n    DEMO: hello"} {
		mustWriteGlobalTest(t, settings, "name: defaults\nx-claude:\n  "+value+"\n")
		if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
			t.Fatalf("sync %q: %v\n%s", value, err, warnings)
		}
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("add then remove must restore the original bytes: %q, want %q", got, original)
	}
}

func TestSyncGlobal_AdoptedEmptyObjectSurvivesBecomingChild(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"env\":{}}\n"
	mustWriteGlobalTest(t, path, original)
	settings := filepath.Join(source, "settings", "defaults.yaml")
	for _, value := range []string{"env: {}", "env:\n    DEMO: hello"} {
		mustWriteGlobalTest(t, settings, "name: defaults\nx-claude:\n  "+value+"\n")
		if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
			t.Fatalf("sync %q: %v\n%s", value, err, warnings)
		}
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("add then remove must restore the adopted object: %q, want %q", got, original)
	}
}

func TestSyncGlobal_SettingsRemovalKeepsAParentTheUserFilled(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{}\n")
	settings := filepath.Join(source, "settings", "defaults.yaml")
	mustWriteGlobalTest(t, settings, "name: defaults\nx-claude:\n  env:\n    DEMO: hello\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, path, "{\"env\":{\"DEMO\":\"hello\",\"MINE\":\"x\"}}\n")
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if got, want := readGlobalTest(t, path), "{\"env\":{\"MINE\":\"x\"}}\n"; got != want {
		t.Errorf("the user's key must stay: %q, want %q", got, want)
	}
}

func TestSyncGlobal_SettingsParentsFromOlderStateStayWhenOwnershipIsUnknown(t *testing.T) {
	for _, first := range [][]string{nil, {"--only", "codex"}} {
		t.Run(strings.Join(first, " "), func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			mustWriteGlobalTest(t, path, "{\"theme\":\"dark\",\"env\":{\"DEMO\":\"hello\"}}\n")
			recorded, err := json.Marshal(path)
			if err != nil {
				t.Fatal(err)
			}
			mustWriteGlobalTest(t, filepath.Join(source, "state", "global.json"), `{"version":8,"files":[],"settings":{"claude":{"env.DEMO":"hello"}},"settingsPaths":{"claude":`+string(recorded)+`}}`)
			if first != nil {
				if _, warnings, err := runGlobalAgentTest(first...); err != nil {
					t.Fatalf("sync of another target: %v\n%s", err, warnings)
				}
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("removal: %v\n%s", err, warnings)
			}
			if got, want := readGlobalTest(t, path), "{\"theme\":\"dark\",\"env\":{}}\n"; got != want {
				t.Errorf("a parent with unknown ownership must stay: %q, want %q", got, want)
			}
		})
	}
}

func TestSyncGlobal_PermissionParentsFromOlderStateStayWhenOwnershipIsUnknown(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\"permissions\":{\"allow\":[\"Read\"]}}\n")
	recorded, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "state", "global.json"), `{"version":8,"files":[],"permissionRules":{"claude":{"allow":["Read"]}},"settingsPaths":{"claude":`+string(recorded)+`}}`)
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if got, want := readGlobalTest(t, path), "{\"permissions\":{}}\n"; got != want {
		t.Errorf("a parent with unknown ownership must stay: %q, want %q", got, want)
	}
}

func TestSyncGlobal_SettingsListsKeepParentsTheUserWrote(t *testing.T) {
	cases := []struct {
		name, target, original string
		steps                  []string
	}{
		{"permission rules in an empty parent", "claude", "{\"permissions\":{}}\n", []string{"permissions:\n  allow: [Read]\n"}},
		{"permission rules and mode in an empty parent", "claude", "{\"permissions\":{}}\n", []string{"permissions:\n  allow: [Read]\n  default-mode: plan\n"}},
		{"mode added under a parent the rules created", "claude", "{\"theme\":\"dark\"}\n", []string{"permissions:\n  allow: [Read]\n", "permissions:\n  allow: [Read]\n  default-mode: plan\n", "permissions:\n  default-mode: plan\n"}},
		{"rules added under a parent the mode created", "claude", "{\"theme\":\"dark\"}\n", []string{"permissions:\n  default-mode: plan\n", "permissions:\n  allow: [Read]\n  default-mode: plan\n", "permissions:\n  allow: [Read]\n"}},
		{"item list in an empty parent", "qwen", "{\"mcp\":{}}\n", []string{"x-qwen:\n  mcp:\n    excluded: [a]\n"}},
		{"key added under a parent the item list created", "qwen", "{\"theme\":\"dark\"}\n", []string{"x-qwen:\n  mcp:\n    excluded: [a]\n", "x-qwen:\n  mcp:\n    excluded: [a]\n    allowed: [b]\n", "x-qwen:\n  mcp:\n    allowed: [b]\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, "."+tc.target, "settings.json")
			mustWriteGlobalTest(t, path, tc.original)
			settings := filepath.Join(source, "settings", "defaults.yaml")
			for _, step := range tc.steps {
				mustWriteGlobalTest(t, settings, "name: defaults\n"+step)
				if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
			}
			added := readGlobalTest(t, path)
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("removal: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != tc.original {
				t.Errorf("add then remove must restore the original bytes\nwant: %q\ngot:  %q\nafter add:\n%s", tc.original, got, added)
			}
		})
	}
}
