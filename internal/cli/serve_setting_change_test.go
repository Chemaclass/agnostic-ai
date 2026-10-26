package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
)

func settingProposal(t *testing.T, h http.Handler, scope editScope, setting map[string]any, want int) builtinChangeView {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"scope": scope, "setting": setting})
	rec := postEdit(t, h, builtinPreviewPath, string(body), nil)
	if rec.Code != want {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	var result builtinChangeView
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	return result
}

func builtinStatus(t *testing.T, h http.Handler) builtinChangeView {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+builtinStatusPath, nil))
	var result builtinChangeView
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}
	return result
}

func plannedChanges(view builtinChangeView) []string {
	var paths []string
	for _, change := range view.Changes {
		paths = append(paths, change.Action+" "+change.Path)
	}
	return paths
}

func TestSettingChanges_PreviewApplyUndoEffortRestoresProject(t *testing.T) {
	root := builtinChangeProject(t)
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := settingProposal(t, h, scopeTeam, map[string]any{"key": "effort", "value": "high", "expected": nil}, http.StatusOK)
	if err := before.check(root); err != nil {
		t.Fatalf("preview wrote project: %v", err)
	}
	paths := plannedChanges(proposal)
	for _, want := range []string{"create .agnostic-ai/settings/defaults.yaml", "update .agnostic-ai/.sync-state"} {
		if !slices.Contains(paths, want) {
			t.Errorf("preview changes = %v, want %q", paths, want)
		}
	}
	if !slices.ContainsFunc(paths, func(p string) bool { return strings.Contains(p, ".claude/") }) || !slices.ContainsFunc(paths, func(p string) bool { return strings.Contains(p, ".codex/") }) {
		t.Errorf("preview changes = %v, want Claude Code and Codex output", paths)
	}
	if proposal.Key != "effort" || proposal.Title != "effort set to high" || proposal.Config != ".agnostic-ai/settings/defaults.yaml" || proposal.Scope != "team" {
		t.Errorf("proposal = %+v", proposal)
	}
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	if applied.Status != "applied" || applied.UndoTitle != "effort set to high" {
		t.Fatalf("apply = %+v", applied)
	}
	if output, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, output)
	}
	if data, _ := os.ReadFile(".agnostic-ai/settings/defaults.yaml"); !strings.Contains(string(data), "effort: high") {
		t.Errorf("defaults.yaml = %q", data)
	}
	undone := builtinAction(t, h, builtinUndoPath, applied.ID)
	if undone.Status != "undone" {
		t.Fatalf("undo = %+v", undone)
	}
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("undo did not restore original project bytes and modes")
	}
}

func TestSettingChanges_StaleExpectedAndInterveningEditsAreConflicts(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, ".agnostic-ai/settings/defaults.yaml", "name: defaults\neffort: low\n")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	settingProposal(t, h, scopeTeam, map[string]any{"key": "effort", "value": "high", "expected": "medium"}, http.StatusConflict)

	proposal := settingProposal(t, h, scopeTeam, map[string]any{"key": "effort", "value": "high", "expected": "low"}, http.StatusOK)
	mustWriteFile(t, ".agnostic-ai/settings/defaults.yaml", "name: defaults\neffort: medium\n")
	if rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("apply after an intervening edit: %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(".agnostic-ai/settings/defaults.yaml"); string(data) != "name: defaults\neffort: medium\n" {
		t.Errorf("stale apply wrote %q", data)
	}
}

func TestSettingChanges_PersonalResetRemovesCreatedFileAndSharesUndo(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	builtin := builtinProposal(t, h, true)
	builtinAction(t, h, builtinApplyPath, builtin.ID)

	personal := settingProposal(t, h, scopeLocal, map[string]any{"key": "model", "value": "opus", "expected": nil}, http.StatusOK)
	if !slices.Contains(plannedChanges(personal), "create .agnostic-ai/local/settings/defaults.yaml") {
		t.Fatalf("personal preview = %v", plannedChanges(personal))
	}
	applied := builtinAction(t, h, builtinApplyPath, personal.ID)
	if applied.UndoTitle != "model set to opus for you" {
		t.Errorf("undo names %q", applied.UndoTitle)
	}
	if output, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, output)
	}

	reset := settingProposal(t, h, scopeLocal, map[string]any{"key": "model", "value": nil, "expected": "opus"}, http.StatusOK)
	if !slices.Contains(plannedChanges(reset), "delete .agnostic-ai/local/settings/defaults.yaml") {
		t.Fatalf("reset preview = %v", plannedChanges(reset))
	}
	if reset.UndoTitle != "model set to opus for you" {
		t.Errorf("preview hides the last applied change: %+v", reset)
	}
	reverted := builtinAction(t, h, builtinApplyPath, reset.ID)
	if _, err := os.Stat(".agnostic-ai/local/settings/defaults.yaml"); !os.IsNotExist(err) {
		t.Errorf("reset kept the created personal file: %v", err)
	}
	if output, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check after reset: %v\n%s", err, output)
	}
	builtinAction(t, h, builtinUndoPath, reverted.ID)
	if data, _ := os.ReadFile(".agnostic-ai/local/settings/defaults.yaml"); !strings.Contains(string(data), "model: opus") {
		t.Errorf("undo of the reset did not restore the personal file: %q", data)
	}
	if status := builtinStatus(t, h); status.UndoAvailable {
		t.Errorf("undo stays available after undoing the last change: %+v", status)
	}
}

func TestSettingChanges_RequestBoundaries(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	for _, body := range []string{
		`{"scope":"team","setting":{"key":"session-retention-days","value":3}}`,
		`{"scope":"team","setting":{"key":"effort"}}`,
		`{"scope":"team","name":"caveman","enabled":true,"setting":{"key":"effort","value":"high"}}`,
		`{"scope":"team","setting":{"key":"effort","value":"high","other":1}}`,
		`{"id":"x","setting":{"key":"effort","value":"high"}}`,
	} {
		if rec := postEdit(t, h, builtinPreviewPath, body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := postEdit(t, h, builtinApplyPath, `{"id":"x","setting":{"key":"effort","value":"high"}}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("apply with a setting: %d", rec.Code)
	}
	settingProposal(t, h, scopeTeam, map[string]any{"key": "model", "value": " "}, http.StatusUnprocessableEntity)
	if _, err := os.Stat(".agnostic-ai/settings/defaults.yaml"); !os.IsNotExist(err) {
		t.Errorf("refused previews wrote a file: %v", err)
	}
}

func TestSettingChanges_PendingSourceEditsAreNamedAndWriteFailureRestores(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Review changes.\n---\nReview one change.\n")
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := settingProposal(t, h, scopeTeam, map[string]any{"key": "model", "value": "opus"}, http.StatusOK)
	if !slices.Contains(proposal.Findings, "This preview also applies pending source changes.") {
		t.Errorf("findings = %v", proposal.Findings)
	}
	if !slices.ContainsFunc(plannedChanges(proposal), func(p string) bool { return strings.Contains(p, "agents/reviewer") }) {
		t.Errorf("preview leaves out the pending agent output: %v", plannedChanges(proposal))
	}
	count := 0
	h.beforeWrite = func(string) error {
		count++
		if count == 2 {
			return errors.New("injected write failure")
		}
		return nil
	}
	if rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil); rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "restored") {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("failed apply was not restored")
	}
}
