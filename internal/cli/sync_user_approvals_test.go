package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func writeHomeApprovals(t *testing.T, source, block string) {
	t.Helper()
	writeFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\n"+block)
}

func TestSyncUser_ApprovalsReviewWritesCodexReviewerAndRemovesIt(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, path, handWrittenCodexConfig)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    codex: review\n")

	if _, warnings, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if got := readFile(t, path); !strings.Contains(got, "approvals_reviewer = \"auto_review\"\n") || !strings.Contains(got, "# Personal Codex config") {
		t.Errorf("config.toml:\n%s", got)
	}
	if _, warnings, err := runUserAgentTest("--only", "codex", "--check"); err != nil {
		t.Fatalf("check after sync: %v\n%s", err, warnings)
	}

	writeHomeApprovals(t, source, "")
	if _, warnings, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync without review: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != handWrittenCodexConfig {
		t.Errorf("removing review left:\n%s\nwant:\n%s", got, handWrittenCodexConfig)
	}
}

func TestSyncUser_ApprovalsReviewWritesClaudeAutoModeAndRemovesIt(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, path, handWrittenClaudeSettings)
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

	if _, warnings, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if mode := claudeDefaultMode(t, path); mode != "auto" {
		t.Errorf("permissions.defaultMode = %v:\n%s", mode, readFile(t, path))
	}
	writeHomeApprovals(t, source, "approvals:\n  mode: ask\n")
	if _, warnings, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync with ask: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != handWrittenClaudeSettings {
		t.Errorf("removing review left:\n%s", got)
	}
}

func TestSyncUser_ApprovalsReviewYieldsToSettingsSpecMode(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, filepath.Join(source, "settings", "defaults.yaml"), "permissions:\n  default-mode: plan\n")
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    claude: review\n")

	if _, warnings, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if mode := claudeDefaultMode(t, path); mode != "plan" {
		t.Errorf("permissions.defaultMode = %v, want the spec's plan", mode)
	}
}

func TestSyncUser_ApprovalsReviewLeavesHandSetReviewer(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".codex", "config.toml")
	original := "approvals_reviewer = \"user\"\n"
	writeFile(t, path, original)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    codex: review\n")

	_, warnings, err := runUserAgentTest("--only", "codex")
	if err != nil {
		t.Fatalf("a hand-set reviewer stopped the sync: %v\n%s", err, warnings)
	}

	if got := readFile(t, path); got != original {
		t.Errorf("config.toml:\n%s", got)
	}
	if !strings.Contains(warnings, "approvals_reviewer") || !strings.Contains(warnings, "outside agnostic-ai") {
		t.Errorf("no warning about the hand-set reviewer:\n%s", warnings)
	}
}

func TestSyncUser_ApprovalsYoloAndProjectsWriteNoReviewer(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeHomeApprovals(t, source, "approvals:\n  mode: yolo\n  projects:\n    ~/sandbox: review\n")

	if _, warnings, err := runUserAgentTest("--only", "claude,codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	for _, path := range []string{filepath.Join(home, ".codex", "config.toml"), filepath.Join(home, ".claude", "settings.json")} {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "auto_review") || strings.Contains(string(data), `"auto"`) {
			t.Errorf("%s holds a reviewer key:\n%s", path, data)
		}
	}
}

func TestSync_ProjectWritesNoNativeReviewer(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")
	dir := newProject(t)
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [claude, codex]\noutputs:\n  codex:\n    config:\n      approval-policy: on-request\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "approvals_reviewer") || strings.Contains(string(data), `"defaultMode"`) {
			t.Errorf("project sync wrote a reviewer key to %s:\n%s", path, data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func claudeDefaultMode(t *testing.T, path string) any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	permissions, _ := doc["permissions"].(map[string]any)
	return permissions["defaultMode"]
}

func TestSyncUser_ApprovalsReviewKeepsHandSetMatchingValueOnRemoval(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := `{"permissions": {"defaultMode": "auto"}}` + "\n"
	writeFile(t, path, original)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    claude: review\n")
	if _, warnings, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	writeHomeApprovals(t, source, "")
	if _, warnings, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync without review: %v\n%s", err, warnings)
	}

	if got := readFile(t, path); got != original {
		t.Errorf("removing review dropped a hand-set value:\n%s", got)
	}
}

func TestSyncUser_BrokenApprovalsWarnsAndSyncs(t *testing.T) {
	home, source := userAgentTestHome(t)
	path := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, path, handWrittenCodexConfig)
	writeHomeApprovals(t, source, "approvals:\n  mode: bogus\n")

	_, warnings, err := runUserAgentTest("--only", "codex,cursor")
	if err != nil {
		t.Fatalf("an invalid approvals block stopped the sync: %v\n%s", err, warnings)
	}

	if !strings.Contains(warnings, "no native reviewer key") {
		t.Errorf("no warning:\n%s", warnings)
	}
	if got := readFile(t, path); strings.Contains(got, "approvals_reviewer") {
		t.Errorf("config.toml:\n%s", got)
	}
}

func TestApprovalsStatus_NotesTheKeySyncWroteAfterAsk(t *testing.T) {
	_, source := userAgentTestHome(t)
	t.Setenv(config.EnvApprovals, "")
	writeHomeApprovals(t, source, "targets: [claude, codex]\napprovals:\n  mode: review\n")
	if _, warnings, err := runUserAgentTest(); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	out, err := runCLI(t, "approvals", "ask")
	if err != nil {
		t.Fatalf("approvals ask: %v\n%s", err, out)
	}
	if !strings.Contains(out, "sync --scope user") {
		t.Errorf("approvals ask hides the sync step:\n%s", out)
	}
	got := approvalsStatusJSON(t)
	for _, tool := range []string{"claude", "codex"} {
		s := toolStatus(t, got, tool)
		if s.Effective != config.ApprovalsAsk || len(s.Notes) != 1 || !strings.Contains(s.Notes[0], "until sync --scope user removes") {
			t.Errorf("%s: %+v", tool, s)
		}
	}
}

func TestSyncUser_ApprovalsHookOutlastsTheCompanionHold(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeHomeApprovals(t, source, "builtins: [approvals]\n")

	if _, warnings, err := runUserAgentTest("--only", "claude,codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	handlerOf := func(path string) (float64, string) {
		t.Helper()
		var file struct {
			Hooks struct {
				PermissionRequest []struct {
					Hooks []struct {
						Command       string  `json:"command"`
						Timeout       float64 `json:"timeout"`
						StatusMessage string  `json:"statusMessage"`
					} `json:"hooks"`
				} `json:"PermissionRequest"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal([]byte(readFile(t, path)), &file); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, group := range file.Hooks.PermissionRequest {
			for _, h := range group.Hooks {
				if strings.Contains(h.Command, approvalHookCommand) {
					return h.Timeout, h.StatusMessage
				}
			}
		}
		t.Fatalf("%s has no approvals hook:\n%s", path, readFile(t, path))
		return 0, ""
	}
	if got, _ := handlerOf(filepath.Join(home, ".claude", "settings.json")); got != 310 {
		t.Errorf("Claude Code hook timeout = %v, want 310", got)
	}
	got, status := handlerOf(filepath.Join(home, ".codex", "hooks.json"))
	if got != 70 || !strings.Contains(status, "agnostic-ai companion") {
		t.Errorf("Codex hook timeout = %v, status %q; want 70 and a line naming the companion", got, status)
	}
}
