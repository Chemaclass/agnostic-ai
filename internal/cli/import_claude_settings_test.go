package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Claude's permission lists become a portable settings spec, so lint and
// the other targets' coverage notes see them. Keys with no portable form
// stay in the overlay (#1329).
func TestImportClaudeSettingsOverlay_MovesPermissionListsToSpec(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{
  "statusLine": {"type": "command", "command": "echo hi"},
  "permissions": {
    "defaultMode": "acceptEdits",
    "allow": ["Bash(git log * --oneline:*)", "Read(src/**)"],
    "ask": ["Bash(git push:*)"],
    "deny": ["Bash(git push origin +*:*)"]
  }
}`)
	settingsDir := filepath.Join(root, "settings")

	got, err := importClaudeSettingsOverlay(root, settingsDir)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !got.seeded || !got.permissionsMoved {
		t.Fatalf("got %+v, want the overlay seeded and permissions moved", got)
	}

	var doc struct {
		Permissions map[string][]string `yaml:"permissions"`
		Target      any                 `yaml:"target"`
	}
	if err := yaml.Unmarshal([]byte(readFileString(t, filepath.Join(settingsDir, claudePermissionsSpec+".yaml"))), &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	want := map[string][]string{
		"allow": {"shell(git log * --oneline:*)", "read(src/**)"},
		"ask":   {"shell(git push:*)"},
		"deny":  {"shell(git push origin +*:*)"},
	}
	for list, rules := range want {
		if strings.Join(doc.Permissions[list], ",") != strings.Join(rules, ",") {
			t.Errorf("%s = %v, want %v", list, doc.Permissions[list], rules)
		}
	}
	if doc.Target != nil {
		t.Errorf("spec is pinned to %v, want it portable", doc.Target)
	}

	overlay := readFileString(t, claudeOverlayPath(root))
	for _, gone := range []string{"allow", "ask", "deny"} {
		if strings.Contains(overlay, `"`+gone+`"`) {
			t.Errorf("overlay still carries %s:\n%s", gone, overlay)
		}
	}
	for _, kept := range []string{`"defaultMode": "acceptEdits"`, `"statusLine"`} {
		if !strings.Contains(overlay, kept) {
			t.Errorf("overlay lost %s:\n%s", kept, overlay)
		}
	}
}

// A settings.json holding only permission lists leaves no overlay behind.
func TestImportClaudeSettingsOverlay_DropsEmptiedPermissions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{"permissions":{"deny":["Read(.env)"]}}`)

	got, err := importClaudeSettingsOverlay(root, filepath.Join(root, "settings"))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got.seeded || !got.permissionsMoved {
		t.Fatalf("got %+v, want only the permissions moved", got)
	}
	if _, err := os.Stat(claudeOverlayPath(root)); !os.IsNotExist(err) {
		t.Errorf("overlay written for a permissions-only settings.json: %v", err)
	}
}

// Sync writes every settings spec's rules into settings.json. A re-import
// must not copy a rule another spec owns into the portable one: a rule
// pinned to Claude would then reach every target.
func TestImportClaudeSettingsOverlay_SkipsRulesAnotherSpecOwns(t *testing.T) {
	root := t.TempDir()
	settingsDir := filepath.Join(root, "settings")
	writeFile(t, filepath.Join(settingsDir, "claude-only.yaml"), "target: claude\npermissions:\n  deny: [\"Bash(rm:*)\"]\n")
	writeFile(t, filepath.Join(settingsDir, "hatch.yaml"), "x-claude:\n  permissions:\n    ask: [\"WebFetch\"]\n")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"permissions":{"deny":["Bash(rm:*)","Read(.env)"],"ask":["WebFetch"]}}`)

	if _, err := importClaudeSettingsOverlay(root, settingsDir); err != nil {
		t.Fatalf("import: %v", err)
	}
	spec := readFileString(t, filepath.Join(settingsDir, claudePermissionsSpec+".yaml"))
	if strings.Contains(spec, "Bash(rm:*)") || strings.Contains(spec, "shell(rm:*)") || strings.Contains(spec, "WebFetch") {
		t.Errorf("spec copied a rule another spec owns:\n%s", spec)
	}
	if !strings.Contains(spec, "read(.env)") {
		t.Errorf("spec lost the Claude-only rule:\n%s", spec)
	}
}

// A spec that does not reach Claude writes nothing into settings.json, so
// the same rule there is Claude's own and moves into the portable spec.
func TestImportClaudeSettingsOverlay_KeepsRulesASpecForAnotherTargetAlsoLists(t *testing.T) {
	root := t.TempDir()
	settingsDir := filepath.Join(root, "settings")
	writeFile(t, filepath.Join(settingsDir, "codex-only.yaml"), "target: codex\npermissions:\n  deny: [\"Bash(rm:*)\"]\n")
	writeFile(t, filepath.Join(settingsDir, "not-claude.yaml"), "targets-exclude: [claude]\npermissions:\n  ask: [\"WebFetch\"]\n")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"permissions":{"deny":["Bash(rm:*)"],"ask":["WebFetch"]}}`)

	if _, err := importClaudeSettingsOverlay(root, settingsDir); err != nil {
		t.Fatalf("import: %v", err)
	}
	spec := readFileString(t, filepath.Join(settingsDir, claudePermissionsSpec+".yaml"))
	if !strings.Contains(spec, "shell(rm:*)") || !strings.Contains(spec, "WebFetch") {
		t.Errorf("spec dropped a rule only a spec for another target lists:\n%s", spec)
	}
}
