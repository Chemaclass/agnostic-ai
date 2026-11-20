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

func TestSyncGlobal_ApprovalsReviewWritesCodexReviewerAndRemovesIt(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, path, handWrittenCodexConfig)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    codex: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if got := readFile(t, path); !strings.Contains(got, "approvals_reviewer = \"auto_review\"\n") || !strings.Contains(got, "# Personal Codex config") {
		t.Errorf("config.toml:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codex", "--check"); err != nil {
		t.Fatalf("check after sync: %v\n%s", err, warnings)
	}

	writeHomeApprovals(t, source, "")
	if _, warnings, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync without review: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != handWrittenCodexConfig {
		t.Errorf("removing review left:\n%s\nwant:\n%s", got, handWrittenCodexConfig)
	}
}

func TestSyncGlobal_ApprovalsReviewWritesClaudeAutoModeAndRemovesIt(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, path, handWrittenClaudeSettings)
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if mode := claudeDefaultMode(t, path); mode != "auto" {
		t.Errorf("permissions.defaultMode = %v:\n%s", mode, readFile(t, path))
	}
	writeHomeApprovals(t, source, "approvals:\n  mode: ask\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync with ask: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != handWrittenClaudeSettings {
		t.Errorf("removing review left:\n%s", got)
	}
}

func TestSyncGlobal_ApprovalsReviewYieldsToSettingsSpecMode(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, filepath.Join(source, "settings", "defaults.yaml"), "permissions:\n  default-mode: plan\n")
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    claude: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	if mode := claudeDefaultMode(t, path); mode != "plan" {
		t.Errorf("permissions.defaultMode = %v, want the spec's plan", mode)
	}
}

func TestSyncGlobal_ApprovalsReviewLeavesHandSetReviewer(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".codex", "config.toml")
	original := "approvals_reviewer = \"user\"\n"
	writeFile(t, path, original)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    codex: review\n")

	_, warnings, err := runGlobalAgentTest("--only", "codex")
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

func TestSyncGlobal_ApprovalsYoloAndProjectsWriteNoReviewer(t *testing.T) {
	home, source := globalAgentTestHome(t)
	writeHomeApprovals(t, source, "approvals:\n  mode: yolo\n  projects:\n    ~/sandbox: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "claude,codex"); err != nil {
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
	_, source := globalAgentTestHome(t)
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
