package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const handWrittenKiroPermissions = "# my Kiro rules\nrules:\n  - capability: shell # trusted\n    match: [\"git *\"]\n    effect: allow\npolicies:\n  - dev-shell\n"

func TestSyncGlobal_ApprovalsReviewListsKiroPresetAndRemovesIt(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIRO_HOME", "")
	path := filepath.Join(home, ".kiro", "settings", "permissions.yaml")
	writeFile(t, path, handWrittenKiroPermissions)
	writeHomeApprovals(t, source, "approvals:\n  tools:\n    kiro: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	want := strings.Replace(handWrittenKiroPermissions, "policies:\n", "policies:\n  - approve-for-me\n", 1)
	if got := readFile(t, path); got != want {
		t.Errorf("permissions.yaml:\n%s\nwant:\n%s", got, want)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
		t.Fatalf("check after sync: %v\n%s", err, warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != want {
		t.Errorf("second sync changed permissions.yaml:\n%s", got)
	}

	writeHomeApprovals(t, source, "approvals:\n  mode: ask\n")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync with ask: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != handWrittenKiroPermissions {
		t.Errorf("removing review left:\n%s\nwant:\n%s", got, handWrittenKiroPermissions)
	}
}

func TestSyncGlobal_ApprovalsReviewCreatesKiroPermissionsAndRemovesIt(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIRO_HOME", "")
	path := filepath.Join(home, ".kiro", "settings", "permissions.yaml")
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != "policies:\n  - approve-for-me\n" {
		t.Errorf("permissions.yaml:\n%s", got)
	}

	writeHomeApprovals(t, source, "")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync without review: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("permissions.yaml stays after review went: %v\n%s", err, readFile(t, path))
	}
}

func TestSyncGlobal_ApprovalsReviewLeavesAKiroPresetTheUserListed(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIRO_HOME", "")
	path := filepath.Join(home, ".kiro", "settings", "permissions.yaml")
	const listed = "policies:\n  - approve-for-me\n"
	writeFile(t, path, listed)
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}

	writeHomeApprovals(t, source, "")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync without review: %v\n%s", err, warnings)
	}

	if got := readFile(t, path); got != listed {
		t.Errorf("permissions.yaml = %q, want the user's preset kept", got)
	}
}

func TestSyncGlobal_ApprovalsReviewWarnsOnKiroPermissionsItCannotEdit(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIRO_HOME", "")
	path := filepath.Join(home, ".kiro", "settings", "permissions.yaml")
	const odd = "policies: dev-shell\n"
	writeFile(t, path, odd)
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

	_, warnings, err := runGlobalAgentTest("--only", "kiro")

	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != odd {
		t.Errorf("permissions.yaml = %q, want it untouched", got)
	}
	if !strings.Contains(warnings, "approve-for-me") || !strings.Contains(warnings, "not a list") {
		t.Errorf("warnings do not explain the skip:\n%s", warnings)
	}
}
