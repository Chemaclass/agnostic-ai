package cli

import (
	"bytes"
	"io/fs"
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

func captureImportLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	prev := logOut
	logOut = &out
	t.Cleanup(func() { logOut = prev })
	return &out
}

func assertNoSpecHolds(t *testing.T, root string, secrets ...string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(path, string(filepath.Separator)+claudeDir+string(filepath.Separator)) {
			return err
		}
		data := readFileString(t, path)
		for _, secret := range secrets {
			if strings.Contains(data, secret) {
				t.Errorf("%s holds %q:\n%s", path, secret, data)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// The overlay is a committed spec file, so an env credential stays out
// of it, like an MCP server's (#2649).
func TestImportClaudeSettingsOverlay_LeavesOutEnvCredentials(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-ant-api03-abcdefghijklmnop",
    "OTEL_EXPORTER_OTLP_HEADERS": "x-honeycomb-team=hcaik01jabcdefghijk",
    "AWS_BEARER_TOKEN_BEDROCK": "ABSKQmVkcm9ja0FQSUtleS1hYmNk",
    "PROXY_URL": "https://user:pw123456@proxy.example.com",
    "ANTHROPIC_BASE_KEY": "${ANTHROPIC_API_KEY:-sk-ant-api03-defaultsecret99}",
    "AWS_SESSION_TOKEN": "${AWS_SESSION_TOKEN:-FQoGZXIvYXdzEJr}",
    "ANTHROPIC_API_KEY": "${ANTHROPIC_API_KEY}",
    "DISABLE_TELEMETRY": "1"
  }
}`)
	log := captureImportLog(t)

	if _, err := importClaudeSettingsOverlay(root, filepath.Join(root, "settings")); err != nil {
		t.Fatalf("import: %v", err)
	}

	assertNoSpecHolds(t, root, "sk-ant-api03-abcdefghijklmnop", "hcaik01jabcdefghijk", "ABSKQmVkcm9ja0FQSUtleS1hYmNk", "pw123456", "defaultsecret99", "FQoGZXIvYXdzEJr")
	overlay := readFileString(t, claudeOverlayPath(root))
	for _, kept := range []string{`"DISABLE_TELEMETRY": "1"`, `"ANTHROPIC_API_KEY": "${ANTHROPIC_API_KEY}"`} {
		if !strings.Contains(overlay, kept) {
			t.Errorf("overlay lost %s:\n%s", kept, overlay)
		}
	}
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "OTEL_EXPORTER_OTLP_HEADERS", "AWS_BEARER_TOKEN_BEDROCK", "PROXY_URL", "ANTHROPIC_BASE_KEY", "AWS_SESSION_TOKEN"} {
		if !strings.Contains(log.String(), "left out env "+key) {
			t.Errorf("no warning names %s:\n%s", key, log.String())
		}
	}
}

// Claude Code reads CLAUDE_CODE_CLIENT_KEY_PASSPHRASE as the passphrase
// itself, never a path, so any literal value is a secret (#2731).
func TestExcludeClaudeCredentials_LeavesOutAnyLiteralClientKeyPassphrase(t *testing.T) {
	captureLogOut(t)
	for _, value := range []string{"hunter2.txt", "./hunter2", "1", "no"} {
		if !claudeSettingsEnvLeftOut(t, "CLAUDE_CODE_CLIENT_KEY_PASSPHRASE", value) {
			t.Errorf("CLAUDE_CODE_CLIENT_KEY_PASSPHRASE=%q kept, want left out", value)
		}
	}
	if claudeSettingsEnvLeftOut(t, "CLAUDE_CODE_CLIENT_KEY_PASSPHRASE", "${PASSPHRASE}") {
		t.Error("CLAUDE_CODE_CLIENT_KEY_PASSPHRASE=${PASSPHRASE} left out, want kept")
	}
}

func TestImportClaudeSettingsOverlay_LeavesOutCredentialHelper(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{
  "apiKeyHelper": "echo sk-ant-api03-abcdefghijklmnop",
  "statusLine": {"type": "command", "command": "curl -H 'Authorization: Bearer abc123def456ghi' https://status.example.com"},
  "model": "opus"
}`)
	log := captureImportLog(t)

	if _, err := importClaudeSettingsOverlay(root, filepath.Join(root, "settings")); err != nil {
		t.Fatalf("import: %v", err)
	}

	assertNoSpecHolds(t, root, "sk-ant-api03-abcdefghijklmnop", "abc123def456ghi")
	if overlay := readFileString(t, claudeOverlayPath(root)); !strings.Contains(overlay, `"model": "opus"`) {
		t.Errorf("overlay lost model:\n%s", overlay)
	}
	for _, key := range []string{"apiKeyHelper", "statusLine"} {
		if !strings.Contains(log.String(), "left out "+key+" in ") {
			t.Errorf("no warning names %s:\n%s", key, log.String())
		}
	}
}

func TestImportClaudeSettingsOverlay_KeepsPlainSettings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{
  "apiKeyHelper": "~/bin/get-key.sh",
  "env": {"ANTHROPIC_MODEL": "opus", "CLAUDE_CODE_USE_BEDROCK": "1", "AWS_PROFILE": "dev", "ANTHROPIC_BASE_URL": "https://proxy.example.com", "AWS_SESSION_TOKEN": "${AWS_SESSION_TOKEN}"}
}`)

	if _, err := importClaudeSettingsOverlay(root, filepath.Join(root, "settings")); err != nil {
		t.Fatalf("import: %v", err)
	}
	overlay := readFileString(t, claudeOverlayPath(root))
	for _, kept := range []string{`"apiKeyHelper": "~/bin/get-key.sh"`, `"ANTHROPIC_MODEL"`, `"CLAUDE_CODE_USE_BEDROCK"`, `"AWS_PROFILE"`, `"ANTHROPIC_BASE_URL"`, `"AWS_SESSION_TOKEN"`} {
		if !strings.Contains(overlay, kept) {
			t.Errorf("overlay lost %s:\n%s", kept, overlay)
		}
	}
}

func TestImportClaudeSettingsOverlay_LeavesOutCredentialRule(t *testing.T) {
	root := t.TempDir()
	settingsDir := filepath.Join(root, "settings")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{"permissions":{"allow":[
  "Bash(curl -H 'Authorization: Bearer abc123def456ghi' https://api.example.com)",
  "Read(src/**)"
]}}`)
	log := captureImportLog(t)

	if _, err := importClaudeSettingsOverlay(root, settingsDir); err != nil {
		t.Fatalf("import: %v", err)
	}

	assertNoSpecHolds(t, root, "abc123def456ghi")
	if spec := readFileString(t, filepath.Join(settingsDir, claudePermissionsSpec+".yaml")); !strings.Contains(spec, "read(src/**)") {
		t.Errorf("spec lost the plain rule:\n%s", spec)
	}
	want := "warning: claude: left out a permissions.allow rule in " + filepath.Join(claudeDir, "settings.json") + ": it holds a credential or cannot be checked safely; add it under local/settings by hand"
	if !strings.Contains(log.String(), want) {
		t.Errorf("warning = %q, want %q", log.String(), want)
	}
}

// A header variable carries keys under names no detector knows, so any
// literal value is left out, with a warning that says it may be one.
func TestImportClaudeSettingsOverlay_LeavesOutLiteralHeaderEnv(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{"env": {
  "ANTHROPIC_CUSTOM_HEADERS": "anthropic-beta: context-1m",
  "OTEL_EXPORTER_OTLP_METRICS_HEADERS": "x-scope-orgid=tenant1",
  "OTEL_EXPORTER_OTLP_TRACES_HEADERS": "${OTEL_TRACES_HEADERS}"
}}`)
	log := captureImportLog(t)

	if _, err := importClaudeSettingsOverlay(root, filepath.Join(root, "settings")); err != nil {
		t.Fatalf("import: %v", err)
	}

	overlay := readFileString(t, claudeOverlayPath(root))
	if strings.Contains(overlay, "context-1m") || strings.Contains(overlay, "tenant1") || !strings.Contains(overlay, "${OTEL_TRACES_HEADERS}") {
		t.Errorf("overlay kept a literal header value or lost a reference:\n%s", overlay)
	}
	for _, key := range []string{"ANTHROPIC_CUSTOM_HEADERS", "OTEL_EXPORTER_OTLP_METRICS_HEADERS"} {
		if !strings.Contains(log.String(), "left out env "+key+" in "+filepath.Join(claudeDir, "settings.json")+": it may hold a credential") {
			t.Errorf("no warning names %s:\n%s", key, log.String())
		}
	}
}
