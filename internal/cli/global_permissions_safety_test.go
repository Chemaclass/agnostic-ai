package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSyncGlobal_PermissionPreviewsHideCredentials(t *testing.T) {
	for _, rule := range []string{
		`Bash(curl -H "Authorization: Bearer ghp_abcdefghijklmnop1234" https://example.test)`,
		`Bash(curl -H 'X-Api-Key: TOPSECRET123' https://example.test)`,
	} {
		for _, action := range []string{"add", "remove", "adopt"} {
			for _, mode := range []string{"plan", "dry-run", "json", "diff", "apply"} {
				t.Run(action+"/"+mode+"/"+rule[13:18], func(t *testing.T) {
					home, source := globalAgentTestHome(t)
					path := filepath.Join(home, ".claude", "settings.json")
					rules := filepath.Join(source, "local", "settings", "private.yaml")
					mustWriteGlobalTest(t, path, "{}\n")
					mustWriteGlobalTest(t, rules, "permissions:\n  allow: ["+jsonString(rule)+"]\n")
					if action == "adopt" {
						mustWriteGlobalTest(t, path, `{"permissions":{"allow":[`+jsonString(rule)+"]}}\n")
					}
					if action == "remove" {
						if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
							t.Fatal(err)
						}
						if err := os.Remove(rules); err != nil {
							t.Fatal(err)
						}
					}
					args := []string{"--only", "claude"}
					switch mode {
					case "diff":
						args = append(args, "--check", "--diff")
					case "apply":
					default:
						args = append(args, "--"+mode)
					}
					out, warnings, err := runGlobalAgentTest(args...)
					if err != nil && mode != "diff" {
						t.Fatalf("sync: %v", err)
					}
					shown := out + warnings
					if strings.Contains(shown, "ghp_abcdefghijklmnop1234") || strings.Contains(shown, "TOPSECRET123") {
						t.Error("permission credential appeared in output")
					}
					if mode == "apply" || mode == "json" {
						want := map[string][]string{"allow": {rule}}
						if action == "remove" {
							want = nil
						}
						if !reflect.DeepEqual(readGlobalPermissionState(t, source), want) {
							t.Error("the full rule was not recorded correctly")
						}
					}
				})
			}
		}
	}
}

func TestImportGlobal_PermissionRulesDetectCredentialHeaders(t *testing.T) {
	for _, header := range []string{
		`-H 'X-Api-Key: TOPSECRET123'`,
		`--header "X-Api-Key: TOPSECRET123"`,
		`--header='X-Api-Key: TOPSECRET123'`,
		`-H'X-Api-Key: TOPSECRET123'`,
		`-H 'X-Api-Key: ${TOKEN:-TOPSECRET123}'`,
		`-H 'X-Api-Key: test-api-key'`,
		`--header='X-Api-Key: 12345'`,
		`--api-key test-api-key`,
		`--api-key=12345`,
	} {
		t.Run(header, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			rule := "Bash(curl " + header + " https://example.test)"
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":["Bash(ls:*)",`+jsonString(rule)+"]}}\n")
			out, warnings, err := runImportGlobalTest("claude")
			if err != nil {
				t.Fatal(err)
			}
			got := readGlobalTest(t, filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec)))
			if strings.Contains(got+out+warnings, "TOPSECRET123") {
				t.Error("credential was imported or displayed")
			}
			if !strings.Contains(warnings, "holds a credential") || !strings.Contains(got, "shell(ls:*)") {
				t.Error("credential rule was not skipped with a warning")
			}
		})
	}
}

func TestImportGlobal_PermissionRulesKeepSafeHeadersAndReferences(t *testing.T) {
	for _, header := range []string{`-H 'Accept: application/json'`, `-H 'X-Api-Key: ${TOKEN}'`, `--header='X-Api-Key: $TOKEN'`} {
		t.Run(header, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			rule := "Bash(curl " + header + " https://example.test)"
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":[`+jsonString(rule)+"]}}\n")
			_, warnings, err := runImportGlobalTest("claude")
			if err != nil {
				t.Fatal(err)
			}
			got := readGlobalTest(t, filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec)))
			var imported struct{ Permissions map[string][]string }
			if err := yaml.Unmarshal([]byte(got), &imported); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(imported.Permissions["allow"], []string{strings.Replace(rule, "Bash(", "shell(", 1)}) || strings.Contains(warnings, "holds a credential") {
				t.Errorf("safe header was not imported: %s", warnings)
			}
		})
	}
}

func TestSyncGlobal_NativePermissionShapeErrorsPreserveOwnedDenials(t *testing.T) {
	for name, invalid := range map[string]string{
		"parent":      "x-claude:\n  permissions: broken\n",
		"null parent": "x-claude:\n  permissions: null\n",
		"list":        "x-claude:\n  permissions:\n    deny: Bash(rm:*)\n",
		"null list":   "x-claude:\n  permissions:\n    deny: null\n",
		"item":        "x-claude:\n  permissions:\n    deny: [{Bash: rm}]\n",
	} {
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			rules := filepath.Join(source, "settings", "rules.yaml")
			mustWriteGlobalTest(t, rules, "x-claude:\n  permissions:\n    deny: [\"Bash(rm:*)\"]\n")
			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			state := filepath.Join(source, "state", "global.json")
			before, beforeState := readGlobalTest(t, path), readGlobalTest(t, state)
			mustWriteGlobalTest(t, rules, invalid)
			if _, _, err := runGlobalAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), "x-claude.permissions") {
				t.Errorf("malformed native permissions must stop sync: %v", err)
			}
			if readGlobalTest(t, path) != before || readGlobalTest(t, state) != beforeState {
				t.Error("settings or state changed after a malformed native permission")
			}
		})
	}
}

func TestSyncGlobal_UnsupportedPermissionRulesHonorPolicy(t *testing.T) {
	for _, rule := range []string{"delete", "Delete"} {
		for _, mode := range []string{"error", "warn", "silent"} {
			t.Run(rule+"/"+mode, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				path := filepath.Join(home, ".claude", "settings.json")
				rules := filepath.Join(source, "settings", "rules.yaml")
				mustWriteGlobalTest(t, path, userClaudeSettings)
				if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
					t.Fatal(err)
				}
				state := filepath.Join(source, "state", "global.json")
				before, beforeState := readGlobalTest(t, path), readGlobalTest(t, state)
				mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\non-unsupported: "+mode+"\n")
				mustWriteGlobalTest(t, rules, "permissions:\n  deny: ["+rule+"]\n")
				_, warnings, err := runGlobalAgentTest("--only", "claude")
				if mode == "error" {
					if err == nil || !strings.Contains(err.Error(), "permissions.deny") {
						t.Errorf("unsupported denial must stop sync: %v", err)
					}
					if readGlobalTest(t, path) != before || readGlobalTest(t, state) != beforeState {
						t.Error("settings or state changed after unsupported-rule error")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if readGlobalTest(t, path) != before || len(readGlobalPermissionState(t, source)) != 0 {
					t.Error("an unsupported Delete rule was written or recorded")
				}
				if noted := strings.Contains(warnings, "no native"); noted != (mode == "warn") {
					t.Errorf("unsupported-policy warning mismatch: %s", warnings)
				}
			})
		}
	}
}

func TestImportGlobal_PermissionOwnershipFollowsTheSettingsPath(t *testing.T) {
	home, source := globalAgentTestHome(t)
	a := filepath.Join(home, "claude-a")
	b := filepath.Join(home, "claude-b")
	t.Setenv("CLAUDE_CONFIG_DIR", a)
	mustWriteGlobalTest(t, filepath.Join(a, "settings.json"), userClaudeSettings)
	rules := filepath.Join(source, "settings", "rules.yaml")
	mustWriteGlobalTest(t, rules, "permissions:\n  allow: [\"Bash(git status)\"]\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	beforeA := readGlobalTest(t, filepath.Join(a, "settings.json"))
	if err := os.Remove(rules); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", b)
	mustWriteGlobalTest(t, filepath.Join(b, "settings.json"), `{"permissions":{"allow":["Bash(git status)"]}}`+"\n")
	beforeB := readGlobalTest(t, filepath.Join(b, "settings.json"))
	if _, _, err := runImportGlobalTest("claude"); err != nil {
		t.Fatal(err)
	}
	imported, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(globalImportPermissionsSpec)))
	if err != nil || !strings.Contains(string(imported), "shell(git status)") {
		t.Errorf("independent rule at the new path must be imported: %v", err)
	}
	if readGlobalTest(t, filepath.Join(a, "settings.json")) != beforeA || readGlobalTest(t, filepath.Join(b, "settings.json")) != beforeB {
		t.Error("import changed native settings")
	}
	if err != nil {
		return
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if readGlobalTest(t, filepath.Join(b, "settings.json")) != beforeB {
		t.Error("round trip changed the independent rule")
	}
	if readGlobalTest(t, filepath.Join(a, "settings.json")) != userClaudeSettings {
		t.Error("round trip did not preserve unrelated rules at the old path")
	}
}

func TestImportGlobal_MalformedPermissionRulesLeaveTheHomeUnchanged(t *testing.T) {
	for name, permissions := range map[string]string{
		"parent":      `"broken"`,
		"null parent": `null`,
		"list":        `{"deny":"Read"}`,
		"null list":   `{"deny":null}`,
		"item":        `{"deny":[{"Read":".env"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			mustWriteGlobalTest(t, path, `{"model":"opus","permissions":`+permissions+"}\n")
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "existing.yaml"), "model: sonnet\n")
			mustWriteGlobalTest(t, filepath.Join(source, "state", "global.json"), "{\"version\":8}\n")
			before, beforeHome := readGlobalTest(t, path), snapshotTree(t, source)
			if _, _, err := runImportGlobalTest("claude"); err == nil || !strings.Contains(err.Error(), "permissions") {
				t.Errorf("malformed native permissions must stop import: %v", err)
			}
			if readGlobalTest(t, path) != before || !reflect.DeepEqual(snapshotTree(t, source), beforeHome) {
				t.Error("native settings, state, or home specs changed after import failed")
			}
		})
	}
}
