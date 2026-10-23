package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func TestSessionRetention_GlobalWritesAndRestoresOwnedKeys(t *testing.T) {
	for _, target := range []string{"claude", "codebuddy", "gemini", "qoder"} {
		for _, before := range []string{"absent", "empty", "compact-empty", "formatted-empty", "different", "matching"} {
			t.Run(target+"/"+before, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				path := filepath.Join(home, "."+target, "settings.json")
				original := ""
				if before != "absent" {
					value := "3e1"
					if before == "matching" {
						value = "3650"
					}
					if target == "claude" || target == "codebuddy" {
						original = "{\n\n  \"theme\": \"dark\", \"cleanupPeriodDays\": " + value + "\n}\n"
					} else {
						age := `"\u0033\u0030d"`
						if before == "matching" {
							age = `"3650d"`
						}
						original = "{\n\n  \"general\": {\"sessionRetention\": {\"maxAge\": " + age + ", \"maxCount\": 9}}, \"theme\": \"dark\"\n}\n"
					}
					switch before {
					case "empty":
						original = "{ }\n"
					case "compact-empty":
						original = "{}"
					case "formatted-empty":
						original = " { \t }\r\n"
					}
					mustWriteGlobalTest(t, path, original)
				}
				settings := filepath.Join(source, "settings", "retention.yaml")
				mustWriteGlobalTest(t, settings, "session-retention-days: 3650\n")
				if _, warnings, err := runGlobalAgentTest("--only", target, "--backup"); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
				values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
				if err != nil {
					t.Fatal(err)
				}
				key, want := "cleanupPeriodDays", any(float64(3650))
				if target != "claude" && target != "codebuddy" {
					key, want = "general.sessionRetention.maxAge", "3650d"
				}
				if got, present := settingsLookup(values, key, "json"); !present || !sameSetting(got, want) {
					t.Errorf("native %s = %v, want %v", key, got, want)
				}
				if err := os.Remove(settings); err != nil {
					t.Fatal(err)
				}
				if _, warnings, err := runGlobalAgentTest("--only", target); err != nil {
					t.Fatalf("removal: %v\n%s", err, warnings)
				}
				if before == "absent" {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Errorf("created settings file remains: %v", err)
					}
				} else if got := readGlobalTest(t, path); got != original {
					t.Errorf("removal changed original bytes:\n%s\nwant:\n%s", got, original)
				}
			})
		}
	}
}

func TestSessionRetention_LintsPositiveIntegersOnly(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{{"1", true}, {"3650", true}, {"0", false}, {"-1", false}, {`"30d"`, false}, {`"30"`, false}, {"1.5", false}, {"true", false}, {"null", false}} {
		t.Run(tc.value, func(t *testing.T) {
			_, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "retention.yaml"), "session-retention-days: "+tc.value+"\n")
			out, err := runRoot(t, "lint", "--global")
			if strings.Contains(out, "LINT045") == tc.valid {
				t.Errorf("LINT045 valid=%t: %v\n%s", tc.valid, err, out)
			}
		})
	}
}

func TestSessionRetention_DoesNotEnableDisabledCleanup(t *testing.T) {
	for _, target := range []string{"gemini", "qoder"} {
		t.Run(target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, "."+target, "settings.json")
			original := "{\"general\":{\"sessionRetention\":{\"enabled\":false,\"maxAge\":\"7d\"}}}\n"
			mustWriteGlobalTest(t, path, original)
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "retention.yaml"), "session-retention-days: 3650\n")
			_, warnings, err := runGlobalAgentTest("--only", target)
			if err != nil || !strings.Contains(warnings, "disabled") {
				t.Errorf("missing disabled cleanup note: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("disabled cleanup changed: %s", got)
			}
		})
	}
}

func TestSessionRetention_ClaudeOverrideWinsAndWarnsForShorterAge(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\"cleanupPeriodDays\":30}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "retention.yaml"), "session-retention-days: 3650\nx-claude:\n  cleanupPeriodDays: 7\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude", "--backup")
	if err != nil || !strings.Contains(warnings, "older sessions") {
		t.Errorf("missing shorter-retention warning: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); !strings.Contains(got, "\"cleanupPeriodDays\":7") {
		t.Errorf("native override lost: %s", got)
	}
}

func TestSessionRetention_ExplainsMappingsAndUnsupportedTargets(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "retention.yaml"), "session-retention-days: 3650\n")
	out, err := runRoot(t, "explain", "--global", "settings/retention.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cleanupPeriodDays", "general.sessionRetention.maxAge"} {
		if !strings.Contains(out, fmt.Sprintf("key %q", key)) {
			t.Errorf("missing mapping %s: %s", key, out)
		}
	}
	_, warnings, err := runGlobalAgentTest("--only", "codex")
	if err != nil || !strings.Contains(warnings, "session-retention-days") || !strings.Contains(warnings, "no documented retention") {
		t.Errorf("missing unsupported-target note: %v\n%s", err, warnings)
	}
}

func TestSessionRetention_ProjectSyncLeavesNativeRetentionAlone(t *testing.T) {
	globalAgentTestHome(t)
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, gemini, qoder]\n")
	mustWriteGlobalTest(t, filepath.Join(".agnostic-ai", "settings", "retention.yaml"), "session-retention-days: 7\n")
	var notes bytes.Buffer
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	captureLogOut(t)
	_, err := runRoot(t, "sync")
	out := notes.String()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "session-retention-days") || !strings.Contains(out, "sync --global") {
		t.Errorf("missing personal-scope note: %s", out)
	}
	for _, target := range []string{"claude", "codebuddy", "gemini", "qoder"} {
		path := filepath.Join("."+target, "settings.json")
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "cleanupPeriodDays") || strings.Contains(string(data), "sessionRetention") {
			t.Errorf("project emitted retention in %s: %s", path, data)
		}
	}
}

func TestSessionRetention_RestoresExistingEmptyContainers(t *testing.T) {
	for _, target := range []string{"gemini", "qoder"} {
		for _, original := range []string{`{"general":{"sessionRetention":{}},"theme":"dark"}`, "{\n  \"general\": { }, \"theme\": \"dark\"\n}\n"} {
			t.Run(target+original, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				path := filepath.Join(home, "."+target, "settings.json")
				mustWriteGlobalTest(t, path, original)
				settings := filepath.Join(source, "settings", "retention.yaml")
				mustWriteGlobalTest(t, settings, "session-retention-days: 3650\n")
				if _, warnings, err := runGlobalAgentTest("--only", target); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
				if err := os.Remove(settings); err != nil {
					t.Fatal(err)
				}
				if _, warnings, err := runGlobalAgentTest("--only", target); err != nil {
					t.Fatalf("removal: %v\n%s", err, warnings)
				}
				if got := readGlobalTest(t, path); got != original {
					t.Errorf("original containers lost: %q, want %q", got, original)
				}
			})
		}
	}
}

func TestSessionRetention_WarnsWhenRemovalShortensOwnedRetention(t *testing.T) {
	for _, target := range []string{"claude", "codebuddy", "gemini", "qoder"} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing=%t", target, existing), func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				path := filepath.Join(home, "."+target, "settings.json")
				if existing {
					original := `{"cleanupPeriodDays":30}`
					if target != "claude" && target != "codebuddy" {
						original = `{"general":{"sessionRetention":{"maxAge":"30d"}}}`
					}
					mustWriteGlobalTest(t, path, original)
				}
				settings := filepath.Join(source, "settings", "retention.yaml")
				mustWriteGlobalTest(t, settings, "session-retention-days: 3650\n")
				if _, warnings, err := runGlobalAgentTest("--only", target, "--backup"); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
				if err := os.Remove(settings); err != nil {
					t.Fatal(err)
				}
				_, warnings, err := runGlobalAgentTest("--only", target)
				if err != nil || !strings.Contains(warnings, "older sessions") {
					t.Errorf("removal lowered retention without warning: %v\n%s", err, warnings)
				}
			})
		}
	}
}

func TestSessionRetention_RequiresANewOwnershipStateVersion(t *testing.T) {
	if globalStateVersion <= 6 {
		t.Errorf("retention origins require a state version newer than 6, got %d", globalStateVersion)
	}
	_, source := globalAgentTestHome(t)
	path := filepath.Join(source, "state", "global.json")
	mustWriteGlobalTest(t, path, `{"version":6,"files":[],"settings":{"claude":{"cleanupPeriodDays":90}}}`)
	state, err := loadGlobalState(path)
	if err != nil || state.Version != globalStateVersion || !sameSetting(state.Settings["claude"]["cleanupPeriodDays"], 90) {
		t.Errorf("supported previous state did not migrate: %+v, %v", state, err)
	}
}

func TestSessionRetention_RejectsEmptyNativeDocumentsBeforeWriting(t *testing.T) {
	for _, target := range []string{"claude", "codebuddy", "gemini", "qoder"} {
		for _, original := range []string{"", " \t\n"} {
			t.Run(target+fmt.Sprintf("/%q", original), func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				path := filepath.Join(home, "."+target, "settings.json")
				mustWriteGlobalTest(t, path, original)
				mustWriteGlobalTest(t, filepath.Join(source, "settings", "retention.yaml"), "session-retention-days: 3650\n")
				_, _, err := runGlobalAgentTest("--only", target)
				if err == nil || !strings.Contains(err.Error(), "unexpected end of JSON input") {
					t.Errorf("invalid native document must fail before retention: %v", err)
				}
				if got := readGlobalTest(t, path); got != original {
					t.Errorf("rejected native document changed: %q, want %q", got, original)
				}
			})
		}
	}
}

func TestSessionRetention_ClaudeNativeZeroRequiresManualCorrection(t *testing.T) {
	for _, backup := range []bool{false, true} {
		t.Run(fmt.Sprintf("backup=%t", backup), func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			original := "{\n  \"cleanupPeriodDays\": 0, \"theme\": \"dark\"\n}\n"
			mustWriteGlobalTest(t, path, original)
			settings := filepath.Join(source, "settings", "retention.yaml")
			mustWriteGlobalTest(t, settings, "session-retention-days: 3650\n")
			args := []string{"--only", "claude"}
			if backup {
				args = append(args, "--backup")
			}
			_, notes, err := runGlobalAgentTest(args...)
			if err != nil {
				t.Errorf("sync should preserve native zero: %v\n%s", err, notes)
			}
			if !strings.Contains(notes, "invalid") || !strings.Contains(notes, "manual") {
				t.Errorf("missing manual correction note: %s", notes)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("portable retention enabled paused native cleanup: %s", got)
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, notes, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Errorf("remove portable retention: %v\n%s", err, notes)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("removal changed untouched native zero: %s", got)
			}
		})
	}
}

func TestSessionRetention_ClaudeUserEditedZeroStaysPaused(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	settings := filepath.Join(source, "settings", "retention.yaml")
	mustWriteGlobalTest(t, path, "{\"cleanupPeriodDays\":30}\n")
	mustWriteGlobalTest(t, settings, "session-retention-days: 3650\n")
	if _, notes, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("initial managed sync: %v\n%s", err, notes)
	}
	edited := "{ \"cleanupPeriodDays\": 0, \"theme\": \"dark\" }\n"
	mustWriteGlobalTest(t, path, edited)
	_, notes, err := runGlobalAgentTest("--only", "claude", "--backup")
	if err != nil {
		t.Errorf("sync should preserve user edit: %v\n%s", err, notes)
	}
	if !strings.Contains(notes, "invalid") || !strings.Contains(notes, "manual") {
		t.Errorf("missing manual correction note: %s", notes)
	}
	if got := readGlobalTest(t, path); got != edited {
		t.Errorf("sync overwrote user zero: %s", got)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, notes, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Errorf("release ownership: %v\n%s", err, notes)
	}
	if got := readGlobalTest(t, path); got != edited {
		t.Errorf("restoration overwrote user zero: %s", got)
	}
}

func TestSessionRetention_ClaudeExplicitOverlayCanCorrectNativeZero(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\"cleanupPeriodDays\":0}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "retention.yaml"), "session-retention-days: 3650\nx-claude:\n  cleanupPeriodDays: 7\n")
	if _, notes, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("explicit native correction: %v\n%s", err, notes)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if got, present := settingsLookup(values, "cleanupPeriodDays", "json"); !present || !sameSetting(got, float64(7)) {
		t.Errorf("explicit native overlay lost: %v", got)
	}
}
