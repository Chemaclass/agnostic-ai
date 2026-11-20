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

func TestSyncGlobal_ApprovalsReviewKeepsKiroOwnershipAfterUneditableFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIRO_HOME", "")
	path := filepath.Join(home, ".kiro", "settings", "permissions.yaml")
	writeFile(t, path, handWrittenKiroPermissions)
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	managed := readFile(t, path)
	writeFile(t, path, "policies: [\n")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil || !strings.Contains(warnings, "leaves") {
		t.Fatalf("sync with malformed permissions: %v\n%s", err, warnings)
	}

	writeFile(t, path, managed)
	writeHomeApprovals(t, source, "approvals:\n  mode: ask\n")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync with ask: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != handWrittenKiroPermissions {
		t.Errorf("permissions.yaml = %q, want the original rules", got)
	}
}

func TestSyncGlobal_ApprovalsReviewWaitsToMoveKiroPresetUntilOldFileIsEditable(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIRO_HOME", "")
	path := filepath.Join(home, ".kiro", "settings", "permissions.yaml")
	writeFile(t, path, handWrittenKiroPermissions)
	writeHomeApprovals(t, source, "approvals:\n  mode: review\n")

	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	managed := readFile(t, path)
	writeFile(t, path, strings.Replace(managed, "approve-for-me\n", "approve-for-me # keep\n", 1))
	newRoot := t.TempDir()
	t.Setenv("KIRO_HOME", newRoot)
	newPath := filepath.Join(newRoot, "settings", "permissions.yaml")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil || !strings.Contains(warnings, "leaves") {
		t.Fatalf("sync with uneditable preset: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("new permissions.yaml was written before removing the old preset: %v", err)
	}

	writeFile(t, path, managed)
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync after repairing old permissions: %v\n%s", err, warnings)
	}
	if got := readFile(t, path); got != handWrittenKiroPermissions {
		t.Errorf("old permissions.yaml = %q, want the original rules", got)
	}
	if got := readFile(t, newPath); got != "policies:\n  - approve-for-me\n" {
		t.Errorf("new permissions.yaml = %q, want only the reviewer preset", got)
	}
}

func TestForeignGlobalPath_RejectsAReviewerPolicyFromAnotherHome(t *testing.T) {
	home := t.TempDir()
	otherHome := t.TempDir()
	path := filepath.Join(otherHome, ".kiro", "settings", "permissions.yaml")
	state := globalState{ReviewPolicies: map[string]globalReviewPolicy{"kiro": {Path: path, Home: otherHome}}}
	if got := foreignGlobalPath(state, home); got != path {
		t.Errorf("foreign reviewer policy = %q, want %q", got, path)
	}
}
