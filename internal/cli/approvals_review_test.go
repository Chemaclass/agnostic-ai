package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func approvalsUserHome(t *testing.T) string {
	t.Helper()
	home := filepath.Dir(approvalsHome(t, ""))
	for _, g := range globalTargets {
		if g.rootEnv != "" {
			t.Setenv(g.rootEnv, "")
		}
	}
	return home
}

func TestApprovalsStatus_NamesTheNativeReviewers(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [claude, codex]\napprovals:\n  mode: review\n")
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "approvals_reviewer = \"auto_review\"\n")
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions": {"defaultMode": "auto"}}`)

	got := approvalsStatusJSON(t)

	claude, codex := toolStatus(t, got, "claude"), toolStatus(t, got, "codex")
	if claude.Effective != config.ApprovalsReview || claude.Route != "Claude Code auto mode" || len(claude.Notes) != 0 {
		t.Errorf("claude: %+v", claude)
	}
	if codex.Effective != config.ApprovalsReview || codex.Route != "Codex Auto-review" || len(codex.Notes) != 0 {
		t.Errorf("codex: %+v", codex)
	}
}

func TestApprovalsStatus_ReviewerKeyMissingAsksForSync(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [codex]\napprovals:\n  tools:\n    codex: review\n")

	codex := toolStatus(t, approvalsStatusJSON(t), "codex")

	if len(codex.Notes) != 1 || !strings.Contains(codex.Notes[0], "approvals_reviewer") || !strings.Contains(codex.Notes[0], "sync --scope user") {
		t.Errorf("codex notes: %q", codex.Notes)
	}
}

func TestApprovalsStatus_CodexApprovalPolicyNeverStopsTheReviewer(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [codex]\napprovals:\n  tools:\n    codex: review\n")
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "approval_policy = \"never\"\napprovals_reviewer = \"auto_review\"\n")

	codex := toolStatus(t, approvalsStatusJSON(t), "codex")

	if len(codex.Notes) != 1 || !strings.Contains(codex.Notes[0], `approval_policy = "never"`) {
		t.Errorf("codex notes: %q", codex.Notes)
	}
	out, err := runCLI(t, "approvals", "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `approval_policy = "never"`) {
		t.Errorf("text status hides the policy note:\n%s", out)
	}
}

func TestApprovalsStatus_ReviewerStaysOnWhereTheModeAsks(t *testing.T) {
	_, source := globalAgentTestHome(t)
	t.Setenv(config.EnvApprovals, "")
	dir := t.TempDir()
	writeHomeApprovals(t, source, "targets: [claude, codex]\napprovals:\n  mode: review\n  projects:\n    "+filepath.ToSlash(dir)+": ask\n")
	if _, warnings, err := runGlobalAgentTest(); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	t.Chdir(dir)

	got := approvalsStatusJSON(t)

	for _, tool := range []string{"claude", "codex"} {
		s := toolStatus(t, got, tool)
		if s.Effective != config.ApprovalsAsk || len(s.Notes) != 1 || !strings.Contains(s.Notes[0], "stays on here") {
			t.Errorf("%s: %+v", tool, s)
		}
	}
}

func TestApprovalsStatus_HandSetClaudeAutoIsNotFlaggedUnderAsk(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [claude]\n")
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions": {"defaultMode": "auto"}}`)

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")

	if len(claude.Notes) != 0 {
		t.Errorf("claude notes: %q", claude.Notes)
	}
}

func TestApprovalsStatus_HandSetCodexReviewerIsNamedAsOutside(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [codex]\n")
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "approvals_reviewer = \"auto_review\"\n")

	codex := toolStatus(t, approvalsStatusJSON(t), "codex")

	if len(codex.Notes) != 1 || !strings.Contains(codex.Notes[0], "outside agnostic-ai") || strings.Contains(codex.Notes[0], "sync --scope user") {
		t.Errorf("codex notes: %q", codex.Notes)
	}
}

func TestApprovalsStatus_ReviewWithTheKeyOffIsAsk(t *testing.T) {
	home := approvalsUserHome(t)
	writeHomeApprovals(t, filepath.Join(home, ".agnostic-ai"), "targets: [codex]\napprovals:\n  mode: review\n")
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "approvals_reviewer = \"user\"\n")

	codex := toolStatus(t, approvalsStatusJSON(t), "codex")

	if codex.Mode != config.ApprovalsReview || codex.Effective != config.ApprovalsAsk || len(codex.Notes) != 1 {
		t.Errorf("codex: %+v", codex)
	}
}

// writeCodexReviewerOn turns Codex Auto-review on in the user config of
// the home that holds source.
func writeCodexReviewerOn(t *testing.T, source string) {
	t.Helper()
	t.Setenv("CODEX_HOME", "")
	writeFile(t, filepath.Join(filepath.Dir(source), ".codex", "config.toml"), "approvals_reviewer = \"auto_review\"\n")
}
