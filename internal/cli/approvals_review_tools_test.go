package cli

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestSyncUser_ApprovalsReviewEveryNativeReviewer(t *testing.T) {
	for _, tool := range slices.Sorted(maps.Keys(nativeReviewers)) {
		reviewer := nativeReviewers[tool]
		t.Run(tool, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			t.Setenv(config.EnvApprovals, "")
			if reviewer.overrideEnv != "" {
				t.Setenv(reviewer.overrideEnv, "")
			}
			g := userTargets[tool]
			path := g.path(home, g.settings.path)
			writeHomeApprovals(t, source, "approvals:\n  tools:\n    "+tool+": review\n")

			if _, warnings, err := runUserAgentTest("--only", tool); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}

			if have, ok := reviewerKey(t, path, g.settings.format, reviewer.key); !ok || !sameSetting(have, reviewer.value) {
				t.Errorf("%s in %s = %v, want %v", reviewer.key, path, have, reviewer.value)
			}
			s := toolStatus(t, approvalsStatusJSON(t), tool)
			var want []string
			if reviewer.reviewNote != "" {
				want = []string{reviewer.reviewNote}
			}
			if s.Effective != config.ApprovalsReview || s.Route != reviewer.name || !slices.Equal(s.Notes, want) {
				t.Errorf("status: %+v", s)
			}

			writeHomeApprovals(t, source, "")
			if _, warnings, err := runUserAgentTest("--only", tool); err != nil {
				t.Fatalf("sync without review: %v\n%s", err, warnings)
			}
			if have, ok := reviewerKey(t, path, g.settings.format, reviewer.key); ok {
				t.Errorf("removing review left %s = %v", reviewer.key, have)
			}
		})
	}
}

func TestSyncUser_ApprovalsReviewYieldsToTargetKeys(t *testing.T) {
	for _, c := range []struct{ tool, spec string }{
		{"codebuddy", "permissions:\n  default-mode: plan\n"},
		{"codex", "x-codex:\n  approvals_reviewer: user\n"},
		{"cursor", "x-cursor:\n  approvalMode: allowlist\n"},
		{"windsurf", "x-windsurf:\n  permission_mode: normal\n"},
		{"qoder", "x-qoder:\n  general:\n    defaultPermissionMode: default\n"},
		{"qwen", "x-qwen:\n  tools:\n    approvalMode: default\n"},
	} {
		t.Run(c.tool, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			g := userTargets[c.tool]
			writeFile(t, filepath.Join(source, "settings", "defaults.yaml"), c.spec)
			writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

			if _, warnings, err := runUserAgentTest("--only", c.tool); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}

			key := nativeReviewers[c.tool].key
			if have, _ := reviewerKey(t, g.path(home, g.settings.path), g.settings.format, key); sameSetting(have, nativeReviewers[c.tool].value) {
				t.Errorf("review replaced the spec's %s", key)
			}
		})
	}
}

func reviewerKey(t *testing.T, path, format, key string) (any, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	values, err := settingsValues(path, format, data)
	if err != nil {
		t.Fatal(err)
	}
	return settingsLookup(values, key, format)
}

func TestApprovalsStatus_QwenAutoIsItsDefault(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [qwen]\napprovals:\n  mode: ask\n")

	qwen := toolStatus(t, approvalsStatusJSON(t), "qwen")

	if len(qwen.Notes) != 1 || !strings.Contains(qwen.Notes[0], "trusted folder") || !strings.Contains(qwen.Notes[0], `"default"`) {
		t.Errorf("qwen notes: %q", qwen.Notes)
	}

	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [qwen]\napprovals:\n  mode: review\n")
	qwen = toolStatus(t, approvalsStatusJSON(t), "qwen")
	if qwen.Effective != config.ApprovalsReview || len(qwen.Notes) != 0 {
		t.Errorf("an unset key reads as off: %+v", qwen)
	}
}

func TestSyncUser_ApprovalsReviewLeavesQoderLegacyMode(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".qoder", "settings.json")
	original := `{"tools": {"approvalMode": "default"}}` + "\n"
	writeFile(t, path, original)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    qoder: review\n")

	_, warnings, err := runUserAgentTest("--only", "qoder")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if got := readFile(t, path); got != original {
		t.Errorf("settings.json:\n%s", got)
	}
	if !strings.Contains(warnings, "tools.approvalMode") {
		t.Errorf("no warning:\n%s", warnings)
	}
}

func TestApprovalsStatus_HandSetQoderAutoIsNamedUnderAsk(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [qoder]\n")
	writeFile(t, filepath.Join(home, ".qoder", "settings.json"), `{"general": {"defaultPermissionMode": "auto"}}`)

	qoder := toolStatus(t, approvalsStatusJSON(t), "qoder")

	if len(qoder.Notes) != 1 || !strings.Contains(qoder.Notes[0], "outside agnostic-ai") {
		t.Errorf("qoder notes: %q", qoder.Notes)
	}
}

func TestApprovalsStatus_CodeBuddyAutoModeDisabledOrMoved(t *testing.T) {
	home := approvalsUserHome(t)
	t.Setenv("CODEBUDDY_CONFIG_DIR", "")
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [codebuddy]\napprovals:\n  mode: review\n")
	writeFile(t, filepath.Join(home, ".codebuddy", "settings.json"), `{"permissions": {"defaultMode": "auto", "disableAutoMode": "disable"}}`)

	codebuddy := toolStatus(t, approvalsStatusJSON(t), "codebuddy")
	if len(codebuddy.Notes) != 1 || !strings.Contains(codebuddy.Notes[0], "disableAutoMode") {
		t.Errorf("codebuddy notes: %q", codebuddy.Notes)
	}

	writeFile(t, filepath.Join(home, ".codebuddy", "settings.json"), `{"permissions": {"defaultMode": "auto"}}`)
	t.Setenv("CODEBUDDY_CONFIG_DIR", t.TempDir())
	codebuddy = toolStatus(t, approvalsStatusJSON(t), "codebuddy")
	if len(codebuddy.Notes) != 1 || !strings.Contains(codebuddy.Notes[0], "CODEBUDDY_CONFIG_DIR") {
		t.Errorf("codebuddy notes: %q", codebuddy.Notes)
	}
}

func TestApprovalsStatus_VibeBypassStopsTheClassifier(t *testing.T) {
	home := approvalsUserHome(t)
	t.Setenv("VIBE_DEFAULT_AGENT", "")
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [vibe]\napprovals:\n  mode: review\n")
	writeFile(t, filepath.Join(home, ".vibe", "config.toml"), "default_agent = \"smart-approve\"\nbypass_tool_permissions = true\n")

	vibe := toolStatus(t, approvalsStatusJSON(t), "vibe")

	if len(vibe.Notes) != 1 || !strings.Contains(vibe.Notes[0], "bypass_tool_permissions") {
		t.Errorf("vibe notes: %q", vibe.Notes)
	}
}

func TestCursorCLIConfigPath_FollowsCursorAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CURSOR_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := cursorCLIConfigPath(home); got != filepath.Join(home, ".cursor", "cli-config.json") {
		t.Errorf("default: %s", got)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	if got := cursorCLIConfigPath(home); got != filepath.Join(home, "xdg", "cursor", "cli-config.json") {
		t.Errorf("XDG: %s", got)
	}
	t.Setenv("CURSOR_CONFIG_DIR", filepath.Join(home, "cfg"))
	if got := cursorCLIConfigPath(home); got != filepath.Join(home, "cfg", "cli-config.json") {
		t.Errorf("CURSOR_CONFIG_DIR: %s", got)
	}
}

func TestSyncUser_CursorSettingsWriteOnlyApprovalMode(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "settings", "defaults.yaml"), "x-cursor:\n  approvalMode: allowlist\n  hints: false\n")

	_, warnings, err := runUserAgentTest("--only", "cursor")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	values, ok := reviewerKey(t, cursorCLIConfigPath(os.Getenv("HOME")), "json", "hints")
	if ok {
		t.Errorf("sync wrote hints = %v", values)
	}
	if have, _ := reviewerKey(t, cursorCLIConfigPath(os.Getenv("HOME")), "json", "approvalMode"); have != "allowlist" {
		t.Errorf("approvalMode = %v", have)
	}
}

func TestSyncUser_ApprovalsReviewReplacesCursorDefault(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := cursorCLIConfigPath(home)
	original := "{\n  \"version\": 1,\n  \"approvalMode\": \"allowlist\",\n  \"hints\": true\n}\n"
	writeFile(t, path, original)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    cursor: review\n")

	if _, warnings, err := runUserAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if have, _ := reviewerKey(t, path, "json", "approvalMode"); have != "auto-review" {
		t.Errorf("approvalMode = %v:\n%s", have, readFile(t, path))
	}

	writeHomeApprovals(t, source, "")
	if _, warnings, err := runUserAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync without review: %v\n%s", err, warnings)
	}
	if have, ok := reviewerKey(t, path, "json", "approvalMode"); ok {
		t.Errorf("removing review left approvalMode = %v", have)
	}
	if have, _ := reviewerKey(t, path, "json", "hints"); have != true {
		t.Errorf("sync dropped cursor-agent's keys:\n%s", readFile(t, path))
	}
}

func TestSyncUser_CursorRejectsUnknownApprovalMode(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "settings", "defaults.yaml"), "x-cursor:\n  approvalMode: auto_review\n")

	if _, warnings, err := runUserAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if _, ok := reviewerKey(t, cursorCLIConfigPath(home), "json", "approvalMode"); ok {
		t.Error("sync wrote a value cursor-agent rejects")
	}
}

func TestSyncUser_CursorSkipsABrokenConfigItDoesNotWrite(t *testing.T) {
	home, _ := userAgentTestHome(t)
	writeFile(t, cursorCLIConfigPath(home), "{ broken")

	if _, warnings, err := runUserAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("a broken cli-config.json stopped the sync: %v\n%s", err, warnings)
	}
}

func TestUserTargets_CursorConfigDirMustBeAbsolute(t *testing.T) {
	t.Setenv("CURSOR_CONFIG_DIR", "relcfg")

	if err := userTargets["cursor"].rootError("cursor"); err == nil || !strings.Contains(err.Error(), "CURSOR_CONFIG_DIR") {
		t.Errorf("rootError = %v", err)
	}
}

func TestApprovalsStatus_DevinDangerousApprovesEverything(t *testing.T) {
	home := approvalsUserHome(t)
	t.Setenv("DEVIN_PERMISSION_MODE", "")
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [windsurf]\napprovals:\n  mode: review\n")
	writeFile(t, devinConfigPath(home), "{\"permission_mode\": \"dangerous\"}\n")

	devin := toolStatus(t, approvalsStatusJSON(t), "windsurf")

	if !slices.ContainsFunc(devin.Notes, func(n string) bool { return strings.Contains(n, "approves every call") }) {
		t.Errorf("windsurf notes: %q", devin.Notes)
	}
}

func TestSyncUser_ApprovalsReviewReplacesDevinNull(t *testing.T) {
	home, source := userAgentTestHome(t)
	t.Setenv("DEVIN_PERMISSION_MODE", "")
	path := devinConfigPath(home)
	writeFile(t, path, "{\n  \"permission_mode\": null,\n  \"theme_mode\": \"dark\"\n}\n")
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    windsurf: review\n")

	if _, warnings, err := runUserAgentTest("--only", "windsurf"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if have, _ := reviewerKey(t, path, "json", "permission_mode"); have != "smart" {
		t.Errorf("permission_mode = %v:\n%s", have, readFile(t, path))
	}

	writeHomeApprovals(t, source, "")
	if _, warnings, err := runUserAgentTest("--only", "windsurf"); err != nil {
		t.Fatalf("sync without review: %v\n%s", err, warnings)
	}
	if have, _ := reviewerKey(t, path, "json", "permission_mode"); have == "smart" {
		t.Errorf("removing review left smart:\n%s", readFile(t, path))
	}
}

func TestImportUser_SkipsAToolOwnedSettingsFile(t *testing.T) {
	home, _ := userAgentTestHome(t)
	writeFile(t, devinConfigPath(home), "{ broken")
	writeFile(t, cursorCLIConfigPath(home), "{ broken")

	if _, stderr, err := runImportUserTest("windsurf", "cursor"); err != nil && strings.Contains(err.Error(), "parse") {
		t.Fatalf("import read a file it takes nothing from: %v\n%s", err, stderr)
	}
}

func devinConfigPath(home string) string {
	g := userTargets["windsurf"]
	return g.path(home, g.settings.path)
}
