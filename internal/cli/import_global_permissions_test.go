package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportGlobal_HoistsClaudePermissionRules(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, `{
  "permissions": {
    "allow": ["Bash(git status)", "WebFetch(domain:example.com)"],
    "ask": ["Bash(git push:*)"],
    "deny": ["Read(./.env)"],
    "defaultMode": "auto"
  }
}
`)
	// Project settings in the working directory are never read.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(wd, ".claude", "settings.json"), `{"permissions":{"allow":["Bash(project-only)"]}}`)
	mustWriteGlobalTest(t, filepath.Join(wd, ".claude", "settings.local.json"), `{"permissions":{"allow":["Bash(local-only)"]}}`)

	out, warnings, err := runImportGlobalTest("claude", "--dry-run")
	specPath := filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec))
	if err != nil || !strings.Contains(out, "dry-run: write "+specPath) {
		t.Fatalf("dry-run must list the spec: %v\n%s\n%s", err, out, warnings)
	}
	if _, err := os.Stat(specPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write: %v", err)
	}
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	got := readGlobalTest(t, specPath)
	want := `permissions:
    allow:
        - shell(git status)
        - WebFetch(domain:example.com)
    ask:
        - shell(git push:*)
    deny:
        - read(./.env)
`
	if got != want {
		t.Errorf("permissions spec:\n%s\nwant:\n%s", got, want)
	}

	// The round trip: sync adopts every rule and changes nothing.
	before := readGlobalTest(t, path)
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("sync after import changed settings.json:\n%s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check after import and sync: %v", err)
	}
	// A second import finds nothing new.
	out, _, err = runImportGlobalTest("claude")
	if err != nil || strings.Contains(out, "wrote") || strings.Contains(out, "updated") {
		t.Errorf("a second import must add nothing: %v\n%s", err, out)
	}
}

func TestImportGlobal_PermissionRulesLeaveWhatTheHomeProvides(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":["Read","Bash(ls:*)","mcp__docs"],"deny":["Bash(rm:*)"]}}`)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "mine.yaml"), "permissions:\n  allow: [read]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "private.yaml"), "x-claude:\n  permissions:\n    deny: [\"Bash(rm:*)\"]\n    allow: [mcp__docs]\n")

	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	got := readGlobalTest(t, filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec)))
	if want := "permissions:\n    allow:\n        - shell(ls:*)\n"; got != want {
		t.Errorf("only the rule no home spec gives may be hoisted:\n%s", got)
	}
}

func TestImportGlobal_PermissionRulesLeaveWhatSyncWrote(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, `{"permissions":{"allow":["Bash(ls:*)"]}}`)
	rules := filepath.Join(source, "settings", "rules.yaml")
	mustWriteGlobalTest(t, rules, "permissions:\n  allow: [\"Bash(git status)\"]\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	// The spec goes, but no sync has removed its rule yet.
	if err := os.Remove(rules); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	got := readGlobalTest(t, filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec)))
	if want := "permissions:\n    allow:\n        - shell(ls:*)\n"; got != want {
		t.Errorf("a rule sync wrote must not be hoisted:\n%s", got)
	}
}

func TestImportGlobal_PermissionRulesAppendOnALaterImport(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, `{"permissions":{"allow":["Bash(ls:*)"]}}`)
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	specPath := filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec))
	// The user keeps notes in the spec and adds rules to the user file.
	mustWriteGlobalTest(t, specPath, "# hoisted from Claude Code\npermissions:\n  allow:\n    - Bash(ls:*)\n")
	mustWriteGlobalTest(t, path, `{"permissions":{"allow":["Bash(ls:*)","Bash(go test:*)"],"deny":["Bash(rm:*)"]}}`)

	out, _, err := runImportGlobalTest("claude", "--dry-run")
	if err != nil || !strings.Contains(out, "dry-run: update "+specPath+": add 2 permission rule(s)") {
		t.Errorf("dry-run must list the update: %v\n%s", err, out)
	}
	out, warnings, err := runImportGlobalTest("claude")
	if err != nil {
		t.Fatalf("second import: %v\n%s", err, warnings)
	}
	if !strings.Contains(out, "updated "+specPath+": added 2 permission rule(s)") || strings.Contains(warnings, "skipped") {
		t.Errorf("the spec must be updated, not skipped:\n%s\n%s", out, warnings)
	}
	want := "# hoisted from Claude Code\npermissions:\n  allow:\n    - Bash(ls:*)\n    - shell(go test:*)\n  deny:\n    - shell(rm:*)\n"
	if got := readGlobalTest(t, specPath); got != want {
		t.Errorf("permissions spec:\n%s\nwant:\n%s", got, want)
	}
	before := readGlobalTest(t, path)
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("sync after import changed settings.json:\n%s", got)
	}
}

func TestImportGlobal_PermissionRulesLeaveOutCredentials(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":["Bash(ls:*)","Bash(curl -H 'Authorization: Bearer ghp_abcdefghijklmnop1234' https://api.github.com)"]}}`)
	_, warnings, err := runImportGlobalTest("claude")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	got := readGlobalTest(t, filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec)))
	if strings.Contains(got, "ghp_") || !strings.Contains(got, "shell(ls:*)") {
		t.Errorf("a rule with a credential must stay out:\n%s", got)
	}
	if !strings.Contains(warnings, "holds a credential") || strings.Contains(warnings, "ghp_") {
		t.Errorf("the warning must name the list, never the value:\n%s", warnings)
	}
}
