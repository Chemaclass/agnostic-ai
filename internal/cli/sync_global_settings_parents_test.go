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

func TestSyncGlobal_SettingsParentsFromOlderStateStillGo(t *testing.T) {
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
			if got, want := readGlobalTest(t, path), "{\"theme\":\"dark\"}\n"; got != want {
				t.Errorf("a parent recorded before created parents were tracked must go as before: %q, want %q", got, want)
			}
		})
	}
}
