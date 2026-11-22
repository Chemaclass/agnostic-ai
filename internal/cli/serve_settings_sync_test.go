package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func decodeSyncReply[T any](t *testing.T, body []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return out
}

func TestSettingsSync_NeedsProof(t *testing.T) {
	h, _ := settingsFixture(t, "")
	ran := false
	h.runSync = func(context.Context, ...string) ([]byte, error) { ran = true; return nil, nil }
	for _, body := range []string{`{"action":"plan"}`, `{"action":"apply","id":"x"}`} {
		if rec := postEdit(t, h, settingsSyncPath, body, nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s without proof = %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := postEdit(t, h, settingsSyncPath, `{"action":"plan"}`, func(r *http.Request) {
		r.Header.Set("Origin", "https://example.com")
		withSecret(settingsTestSecret)(r)
	}); rec.Code != http.StatusForbidden {
		t.Errorf("plan from another origin = %d", rec.Code)
	}
	if ran {
		t.Error("a refused request ran a sync")
	}
}

func TestSettingsSync_PlanApplyStaleAndFailure(t *testing.T) {
	h, source := settingsFixture(t, "")
	hooks, err := codex.UserHooksPath()
	if err != nil {
		t.Fatal(err)
	}
	planJSON := `{"writes":[{"target":"codex","path":"` + filepath.ToSlash(hooks) + `","action":"create"}],"notes":[{"message":"a note"}]}`
	var applied bool
	var failure error
	h.runSync = func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "--plan") {
			return []byte(planJSON), nil
		}
		applied = true
		return []byte(planJSON), failure
	}
	rec := postEdit(t, h, settingsSyncPath, `{"action":"plan"}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d %s", rec.Code, rec.Body)
	}
	plan := decodeSyncReply[settingsSyncPlan](t, rec.Body.Bytes())
	if plan.ID == "" || len(plan.Files) != 1 || plan.Files[0].Change != "create" || len(plan.Notes) != 1 {
		t.Fatalf("plan = %s", rec.Body)
	}

	if rec := postEdit(t, h, settingsSyncPath, `{"action":"apply","id":"stale"}`, withSecret(settingsTestSecret)); rec.Code != http.StatusConflict || applied {
		t.Fatalf("stale apply = %d %s, applied %v", rec.Code, rec.Body, applied)
	}

	failure = errors.New("conflict: edited since the last user-level sync")
	rec = postEdit(t, h, settingsSyncPath, `{"action":"apply","id":"`+plan.ID+`"}`, withSecret(settingsTestSecret))
	refusal := decodeSyncReply[settingsSyncRefusal](t, rec.Body.Bytes())
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(refusal.Error, "conflict") || refusal.Command != settingsSyncCommand {
		t.Fatalf("failed apply = %d %s", rec.Code, rec.Body)
	}

	failure = nil
	rec = postEdit(t, h, settingsSyncPath, `{"action":"apply","id":"`+plan.ID+`"}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	reply := decodeSyncReply[settingsSyncReply](t, rec.Body.Bytes())
	if len(reply.Written) != 1 || !strings.Contains(strings.Join(reply.Notes, "\n"), "Open /hooks in Codex") {
		t.Errorf("apply reply = %s", rec.Body)
	}
	entries, err := approvals.ReadLog(approvals.LogPath(source))
	if err != nil || len(entries) != 1 || entries[0].Call != "sync" {
		t.Errorf("decision log = %+v, %v", entries, err)
	}
}

// TestSettingsSync_InstallsTheHookOnACleanHome runs the real user sync in
// process on a temp home: after yolo and the apply, Claude Code's user
// settings hold the approvals hook.
func TestSettingsSync_InstallsTheHookOnACleanHome(t *testing.T) {
	silence(t)
	h, _ := settingsFixture(t, "version: 1\ntargets: [claude]\n")
	if rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"yolo"}`, withSecret(settingsTestSecret)); rec.Code != http.StatusOK {
		t.Fatalf("POST yolo = %d %s", rec.Code, rec.Body)
	}
	rec := postEdit(t, h, settingsSyncPath, `{"action":"plan"}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d %s", rec.Code, rec.Body)
	}
	plan := decodeSyncReply[settingsSyncPlan](t, rec.Body.Bytes())
	settings, ok := claudeUserSettingsPath()
	if !ok {
		t.Fatal("no Claude Code settings path")
	}
	listed := false
	for _, f := range plan.Files {
		listed = listed || filepath.FromSlash(f.Path) == settings
	}
	if !listed {
		t.Fatalf("plan does not list %s: %s", settings, rec.Body)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatalf("a plan wrote %s: %v", settings, err)
	}
	rec = postEdit(t, h, settingsSyncPath, `{"action":"apply","id":"`+plan.ID+`"}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	if got := readFile(t, settings); !strings.Contains(got, approvalHookCommand) {
		t.Errorf("%s has no approvals hook:\n%s", settings, got)
	}
	reply := decodeSyncReply[settingsSyncReply](t, rec.Body.Bytes())
	for _, tool := range reply.Settings.Approvals.Tools {
		if tool.Tool == "claude" && tool.Effective != config.ApprovalsYolo {
			t.Errorf("claude after the sync = %+v", tool)
		}
	}
	if reply.Settings.Sync.Needed {
		t.Errorf("sync still needed: %v", reply.Settings.Sync.Reasons)
	}
}
