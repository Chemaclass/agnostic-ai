package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSyncUser_PermissionPreviewsHideCredentials(t *testing.T) {
	for _, rule := range []string{
		`Bash(curl -H "Authorization: Bearer ghp_abcdefghijklmnop1234" https://example.test)`,
		`Bash(curl -H 'X-Api-Key: TOPSECRET123' https://example.test)`,
	} {
		for _, action := range []string{"add", "remove", "adopt"} {
			for _, mode := range []string{"plan", "dry-run", "json", "diff", "apply"} {
				t.Run(action+"/"+mode+"/"+rule[13:18], func(t *testing.T) {
					home, source := userAgentTestHome(t)
					path := filepath.Join(home, ".claude", "settings.json")
					rules := filepath.Join(source, "local", "settings", "private.yaml")
					writeFile(t, path, "{}\n")
					writeFile(t, rules, "permissions:\n  allow: ["+jsonString(rule)+"]\n")
					if action == "adopt" {
						writeFile(t, path, `{"permissions":{"allow":[`+jsonString(rule)+"]}}\n")
					}
					if action == "remove" {
						if _, _, err := runUserAgentTest("--only", "claude"); err != nil {
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
					out, warnings, err := runUserAgentTest(args...)
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
						if !reflect.DeepEqual(readUserPermissionState(t, source), want) {
							t.Error("the full rule was not recorded correctly")
						}
					}
				})
			}
		}
	}
}

func TestImportUser_PermissionRulesDetectCredentialHeaders(t *testing.T) {
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
			home, source := userAgentTestHome(t)
			rule := "Bash(curl " + header + " https://example.test)"
			writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":["Bash(ls:*)",`+jsonString(rule)+"]}}\n")
			out, warnings, err := runImportUserTest("claude")
			if err != nil {
				t.Fatal(err)
			}
			got := readFile(t, filepath.Join(source, filepath.FromSlash(userImportPermissionsSpec)))
			if strings.Contains(got+out+warnings, "TOPSECRET123") {
				t.Error("credential was imported or displayed")
			}
			if !strings.Contains(warnings, "holds a credential") || !strings.Contains(got, "shell(ls:*)") {
				t.Error("credential rule was not skipped with a warning")
			}
		})
	}
}

func TestImportUser_PermissionRulesKeepSafeHeadersAndReferences(t *testing.T) {
	for _, header := range []string{`-H 'Accept: application/json'`, `-H 'X-Api-Key: ${TOKEN}'`, `--header='X-Api-Key: $TOKEN'`} {
		t.Run(header, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			rule := "Bash(curl " + header + " https://example.test)"
			writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":[`+jsonString(rule)+"]}}\n")
			_, warnings, err := runImportUserTest("claude")
			if err != nil {
				t.Fatal(err)
			}
			got := readFile(t, filepath.Join(source, filepath.FromSlash(userImportPermissionsSpec)))
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

func TestSyncUser_NativePermissionShapeErrorsPreserveOwnedDenials(t *testing.T) {
	for name, invalid := range map[string]string{
		"native outer parent": "x-claude: broken\n",
		"native outer list":   "x-claude: [broken]\n",
		"portable parent":     "permissions: broken\n",
		"portable list":       "permissions: [broken]\n",
		"parent":              "x-claude:\n  permissions: broken\n",
		"null parent":         "x-claude:\n  permissions: null\n",
		"list":                "x-claude:\n  permissions:\n    deny: Bash(rm:*)\n",
		"null list":           "x-claude:\n  permissions:\n    deny: null\n",
		"item":                "x-claude:\n  permissions:\n    deny: [{Bash: rm}]\n",
	} {
		t.Run(name, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			rules := filepath.Join(source, "settings", "rules.yaml")
			writeFile(t, rules, "x-claude:\n  permissions:\n    deny: [\"Bash(rm:*)\"]\n")
			if _, _, err := runUserAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			state := filepath.Join(source, "state", "global.json")
			before, beforeState := readFile(t, path), readFile(t, state)
			writeFile(t, rules, invalid)
			if _, _, err := runUserAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), rules) {
				t.Errorf("malformed native permissions must stop sync: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != before || readFile(t, state) != beforeState {
				t.Error("settings or state changed after a malformed native permission")
			}
		})
	}
}

func TestImportUser_PermissionRulesDetectCredentialAssignmentsAndNestedShells(t *testing.T) {
	const numericCredential = "98765432101234567890"
	nested := `curl -H "X-Api-Key: test-api-key" https://example.test`
	for range 6 {
		nested = "sh -c '" + strings.ReplaceAll(nested, "'", "'\"'\"'") + "'"
	}
	for name, command := range map[string]string{
		"assignment":                "API_KEY=" + numericCredential + " curl https://example.test",
		"env argument":              `env API_KEY=test-api-key curl https://example.test`,
		"assignment default":        `API_KEY=${TOKEN:-test-api-key} curl https://example.test`,
		"nested shell":              `sh -c 'curl -H "X-Api-Key: test-api-key" https://example.test'`,
		"nested double quotes":      `sh -c "curl -H \"X-Api-Key: test-api-key\" https://example.test"`,
		"nested login shell":        "bash -lc 'env API_KEY=" + numericCredential + " curl https://example.test'",
		"nested beyond depth bound": nested,
	} {
		t.Run(name, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			// A short numeric path must not count as a leaked credential.
			configDir := filepath.Join(home, "12345")
			t.Setenv("CLAUDE_CONFIG_DIR", configDir)
			rule := "Bash(" + command + ")"
			writeFile(t, filepath.Join(configDir, "settings.json"), `{"permissions":{"allow":["Bash(ls:*)",`+jsonString(rule)+"]}}\n")
			out, warnings, err := runImportUserTest("claude")
			if err != nil {
				t.Fatal(err)
			}
			got := readFile(t, filepath.Join(source, filepath.FromSlash(userImportPermissionsSpec)))
			if strings.Contains(got+out+warnings, "test-api-key") || strings.Contains(got+out+warnings, numericCredential) {
				t.Error("credential assignment or nested command was imported or displayed")
			}
			if !strings.Contains(warnings, "holds a credential") || !strings.Contains(got, "shell(ls:*)") {
				t.Error("credential command was not skipped with a warning")
			}
		})
	}
}

// A rule in another tool's spelling, such as `PowerShell(...)`, reads
// its command as shell too (#2737).
func TestImportUser_PermissionRulesDetectCredentialInAnyToolSpelling(t *testing.T) {
	home, source := userAgentTestHome(t)
	rule := `PowerShell(curl -H "X-Api-Key: test-api-key" https://example.test)`
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":["Read(src/**)",`+jsonString(rule)+"]}}\n")
	out, warnings, err := runImportUserTest("claude")
	if err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(source, filepath.FromSlash(userImportPermissionsSpec)))
	if strings.Contains(got+out+warnings, "test-api-key") {
		t.Error("credential rule was imported or displayed")
	}
	if !strings.Contains(warnings, "holds a credential") || !strings.Contains(got, "read(src/**)") {
		t.Errorf("credential rule was not skipped with a warning:\n%s\n%s", got, warnings)
	}
}

func TestImportUser_PermissionRulesKeepAssignmentAndNestedReferences(t *testing.T) {
	for name, command := range map[string]string{
		"assignment":         `API_KEY=${TOKEN} curl https://example.test`,
		"env argument":       `env API_KEY=$TOKEN curl https://example.test`,
		"nested shell":       `sh -c 'curl -H "X-Api-Key: ${TOKEN}" https://example.test'`,
		"nested login shell": `bash -lc 'env API_KEY=${TOKEN} curl https://example.test'`,
	} {
		t.Run(name, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			rule := "Bash(" + command + ")"
			writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":[`+jsonString(rule)+"]}}\n")
			_, warnings, err := runImportUserTest("claude")
			if err != nil {
				t.Fatal(err)
			}
			var imported struct{ Permissions map[string][]string }
			got := readFile(t, filepath.Join(source, filepath.FromSlash(userImportPermissionsSpec)))
			if err := yaml.Unmarshal([]byte(got), &imported); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(imported.Permissions["allow"], []string{"shell(" + command + ")"}) || strings.Contains(warnings, "holds a credential") {
				t.Error("safe assignment or nested reference was not imported unchanged")
			}
		})
	}
}

func TestSyncUser_UnsupportedPermissionRulesHonorPolicy(t *testing.T) {
	for _, rule := range []string{"delete", "Delete"} {
		for _, mode := range []string{"error", "warn", "silent"} {
			t.Run(rule+"/"+mode, func(t *testing.T) {
				home, source := userAgentTestHome(t)
				path := filepath.Join(home, ".claude", "settings.json")
				rules := filepath.Join(source, "settings", "rules.yaml")
				writeFile(t, path, userClaudeSettings)
				if _, _, err := runUserAgentTest("--only", "claude"); err != nil {
					t.Fatal(err)
				}
				state := filepath.Join(source, "state", "global.json")
				before, beforeState := readFile(t, path), readFile(t, state)
				writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\non-unsupported: "+mode+"\n")
				writeFile(t, rules, "permissions:\n  deny: ["+rule+"]\n")
				_, warnings, err := runUserAgentTest("--only", "claude")
				if mode == "error" {
					if err == nil || !strings.Contains(err.Error(), "permissions.deny") {
						t.Errorf("unsupported denial must stop sync: %v", err)
					}
					if readFile(t, path) != before || readFile(t, state) != beforeState {
						t.Error("settings or state changed after unsupported-rule error")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if readFile(t, path) != before || len(readUserPermissionState(t, source)) != 0 {
					t.Error("an unsupported Delete rule was written or recorded")
				}
				if noted := strings.Contains(warnings, "no native"); noted != (mode == "warn") {
					t.Errorf("unsupported-policy warning mismatch: %s", warnings)
				}
			})
		}
	}
}

func TestImportUser_PermissionOwnershipFollowsTheSettingsPath(t *testing.T) {
	home, source := userAgentTestHome(t)
	a := filepath.Join(home, "claude-a")
	b := filepath.Join(home, "claude-b")
	t.Setenv("CLAUDE_CONFIG_DIR", a)
	writeFile(t, filepath.Join(a, "settings.json"), userClaudeSettings)
	rules := filepath.Join(source, "settings", "rules.yaml")
	writeFile(t, rules, "permissions:\n  allow: [\"Bash(git status)\"]\n")
	if _, _, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	beforeA := readFile(t, filepath.Join(a, "settings.json"))
	if err := os.Remove(rules); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", b)
	writeFile(t, filepath.Join(b, "settings.json"), `{"permissions":{"allow":["Bash(git status)"]}}`+"\n")
	beforeB := readFile(t, filepath.Join(b, "settings.json"))
	if _, _, err := runImportUserTest("claude"); err != nil {
		t.Fatal(err)
	}
	imported, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(userImportPermissionsSpec)))
	if err != nil || !strings.Contains(string(imported), "shell(git status)") {
		t.Errorf("independent rule at the new path must be imported: %v", err)
	}
	if readFile(t, filepath.Join(a, "settings.json")) != beforeA || readFile(t, filepath.Join(b, "settings.json")) != beforeB {
		t.Error("import changed native settings")
	}
	if err != nil {
		return
	}
	if _, _, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(b, "settings.json")) != beforeB {
		t.Error("round trip changed the independent rule")
	}
	if readFile(t, filepath.Join(a, "settings.json")) != userClaudeSettings {
		t.Error("round trip did not preserve unrelated rules at the old path")
	}
}

func TestImportUser_MalformedPermissionRulesLeaveTheHomeUnchanged(t *testing.T) {
	for name, permissions := range map[string]string{
		"parent":      `"broken"`,
		"null parent": `null`,
		"list":        `{"deny":"Read"}`,
		"null list":   `{"deny":null}`,
		"item":        `{"deny":[{"Read":".env"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			writeFile(t, path, `{"model":"opus","permissions":`+permissions+"}\n")
			writeFile(t, filepath.Join(source, "settings", "existing.yaml"), "model: sonnet\n")
			writeFile(t, filepath.Join(source, "state", "global.json"), "{\"version\":8}\n")
			before, beforeHome := readFile(t, path), snapshotTree(t, source)
			if _, _, err := runImportUserTest("claude"); err == nil || !strings.Contains(err.Error(), "permissions") {
				t.Errorf("malformed native permissions must stop import: %v", err)
			}
			if readFile(t, path) != before || !reflect.DeepEqual(snapshotTree(t, source), beforeHome) {
				t.Error("native settings, state, or home specs changed after import failed")
			}
		})
	}
}
