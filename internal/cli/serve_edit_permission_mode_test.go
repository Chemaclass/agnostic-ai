package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func permissionModeRequest(t *testing.T, h http.Handler, body map[string]any) (int, string) {
	t.Helper()
	body["key"] = "permissions.default-mode"
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := postEdit(t, h, "/api/edit/field", string(data), nil)
	return rec.Code, rec.Body.String()
}

func permissionModeIn(t *testing.T, file string) any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(readProjectFile(t, file)), &doc); err != nil {
		t.Fatal(err)
	}
	perms, _ := doc["permissions"].(map[string]any)
	return perms["default-mode"]
}

func TestPermissionModeEdit_CreatesThePermissionsSpecWhenNoSettingsExist(t *testing.T) {
	h := targetProject(t, "")
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": "plan", "expected": nil}); code != http.StatusOK || !strings.Contains(body, ".agnostic-ai/settings/permissions.yaml") {
		t.Fatalf("save = %d %s", code, body)
	}
	file := ".agnostic-ai/settings/permissions.yaml"
	if got := permissionModeIn(t, file); got != "plan" {
		t.Fatalf("default-mode = %v in %s", got, readProjectFile(t, file))
	}
	if readProjectFile(t, ".agnostic-ai/settings/defaults.yaml") != "" {
		t.Fatal("the mode went to defaults.yaml instead of the permissions spec")
	}
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": nil, "expected": "plan"}); code != http.StatusOK {
		t.Fatalf("clear = %d %s", code, body)
	}
	if got := readProjectFile(t, file); got != "" {
		t.Fatalf("clearing the only value left the created spec: %q", got)
	}
}

func TestPermissionModeEdit_AddsToTheExistingPermissionsMappingAndKeepsTheRest(t *testing.T) {
	original := "# Team rules.\nname: defaults\npermissions:\n  allow: [read(src/**)] # Keep.\nmodel: sonnet\n"
	h := targetProject(t, original)
	file := ".agnostic-ai/settings/defaults.yaml"
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": "acceptEdits", "expected": nil}); code != http.StatusOK {
		t.Fatalf("save = %d %s", code, body)
	}
	got := readProjectFile(t, file)
	if permissionModeIn(t, file) != "acceptEdits" || !strings.Contains(got, "  allow: [read(src/**)] # Keep.\n") || !strings.Contains(got, "model: sonnet\n") {
		t.Fatalf("file = %q", got)
	}
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": "plan", "expected": "default"}); code != http.StatusConflict {
		t.Fatalf("stale save = %d %s", code, body)
	}
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": "plan", "expected": "acceptEdits"}); code != http.StatusOK {
		t.Fatalf("update = %d %s", code, body)
	}
	if permissionModeIn(t, file) != "plan" {
		t.Fatalf("file = %q", readProjectFile(t, file))
	}
}

func TestPermissionModeEdit_RejectsModesClaudeCodeDoesNotTake(t *testing.T) {
	h := targetProject(t, "name: defaults\n")
	for _, value := range []any{"yolo", "", 3, "Plan"} {
		code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": value})
		if code != http.StatusBadRequest || !strings.Contains(body, "default, manual, acceptEdits, plan, auto, dontAsk, bypassPermissions") {
			t.Errorf("%v: %d %s", value, code, body)
		}
	}
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": "plan", "target": "claude"}); code != http.StatusBadRequest {
		t.Errorf("per-tool save = %d %s", code, body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\n" {
		t.Fatalf("a refused save changed the file: %q", got)
	}
}

func TestPermissionModeEdit_LocalSaveWritesTheOverlay(t *testing.T) {
	h := targetProject(t, "name: defaults\npermissions:\n  default-mode: default\n")
	isolateGit(t)
	git(t, h.root, "init", "-q")
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "personal", "value": "bypassPermissions", "expected": nil}); code != http.StatusOK {
		t.Fatalf("save = %d %s", code, body)
	}
	if got := permissionModeIn(t, ".agnostic-ai/local/settings/defaults.yaml"); got != "bypassPermissions" {
		t.Fatalf("local = %v", got)
	}
	if got := permissionModeIn(t, ".agnostic-ai/settings/defaults.yaml"); got != "default" {
		t.Fatalf("shared changed to %v", got)
	}
}

func TestPermissionModeEdit_RefusesWhileALocalFileClearsPermissions(t *testing.T) {
	h := targetProject(t, "name: defaults\npermissions:\n  allow: [read]\n")
	mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\npermissions: null\n")
	code, body := permissionModeRequest(t, h, map[string]any{"audience": "personal", "value": "plan"})
	if code != http.StatusBadRequest || !strings.Contains(body, "permissions: null") {
		t.Fatalf("save = %d %s", code, body)
	}
}
