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

func TestSyncGlobal_ApprovalsReviewEveryNativeReviewer(t *testing.T) {
	for _, tool := range slices.Sorted(maps.Keys(nativeReviewers)) {
		reviewer := nativeReviewers[tool]
		t.Run(tool, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			t.Setenv(config.EnvApprovals, "")
			if reviewer.overrideEnv != "" {
				t.Setenv(reviewer.overrideEnv, "")
			}
			g := globalTargets[tool]
			path := g.path(home, g.settings.path)
			writeHomeApprovals(t, source, "approvals:\n  tools:\n    "+tool+": review\n")

			if _, warnings, err := runGlobalAgentTest("--only", tool); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}

			if have, ok := reviewerKey(t, path, g.settings.format, reviewer.key); !ok || !sameSetting(have, reviewer.value) {
				t.Errorf("%s in %s = %v, want %v", reviewer.key, path, have, reviewer.value)
			}
			s := toolStatus(t, approvalsStatusJSON(t), tool)
			if s.Effective != config.ApprovalsReview || s.Route != reviewer.name || len(s.Notes) != 0 {
				t.Errorf("status: %+v", s)
			}

			writeHomeApprovals(t, source, "")
			if _, warnings, err := runGlobalAgentTest("--only", tool); err != nil {
				t.Fatalf("sync without review: %v\n%s", err, warnings)
			}
			if have, ok := reviewerKey(t, path, g.settings.format, reviewer.key); ok {
				t.Errorf("removing review left %s = %v", reviewer.key, have)
			}
		})
	}
}

func TestSyncGlobal_ApprovalsReviewYieldsToTargetKeys(t *testing.T) {
	for _, c := range []struct{ tool, spec string }{
		{"codebuddy", "permissions:\n  default-mode: plan\n"},
		{"codex", "x-codex:\n  approvals_reviewer: user\n"},
		{"qoder", "x-qoder:\n  general:\n    defaultPermissionMode: default\n"},
		{"qwen", "x-qwen:\n  tools:\n    approvalMode: default\n"},
	} {
		t.Run(c.tool, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			g := globalTargets[c.tool]
			writeFile(t, filepath.Join(source, "settings", "defaults.yaml"), c.spec)
			writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

			if _, warnings, err := runGlobalAgentTest("--only", c.tool); err != nil {
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

func TestSyncGlobal_ApprovalsReviewLeavesQoderLegacyMode(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".qoder", "settings.json")
	original := `{"tools": {"approvalMode": "default"}}` + "\n"
	writeFile(t, path, original)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    qoder: review\n")

	_, warnings, err := runGlobalAgentTest("--only", "qoder")
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
