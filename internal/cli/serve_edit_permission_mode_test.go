package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

// globalModeHandler is the global view over a user configuration with
// Claude as its only tool, and the home sync --global writes into.
func globalModeHandler(t *testing.T, settings string) (*editHandler, string, string) {
	t.Helper()
	silence(t)
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	if settings != "" {
		mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), settings)
	}
	return &editHandler{root: source, global: true}, home, source
}

func TestPermissionModeEdit_GlobalSaveReachesClaudeThroughGlobalSync(t *testing.T) {
	h, home, source := globalModeHandler(t, "")
	code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": "plan", "expected": nil})
	if code != http.StatusOK || !strings.Contains(body, "settings/permissions.yaml") {
		t.Fatalf("save = %d %s", code, body)
	}
	file := filepath.Join(source, "settings", "permissions.yaml")
	if got := permissionModeIn(t, file); got != "plan" {
		t.Fatalf("default-mode = %v in %s", got, readProjectFile(t, file))
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync --global: %v\n%s", err, warnings)
	}
	var native map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(home, ".claude", "settings.json"))), &native); err != nil {
		t.Fatal(err)
	}
	if perms, _ := native["permissions"].(map[string]any); perms["defaultMode"] != "plan" {
		t.Fatalf("Claude settings = %v", native)
	}
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": nil, "expected": "plan"}); code != http.StatusOK {
		t.Fatalf("clear = %d %s", code, body)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("clearing the only value left the created spec: %v", err)
	}
}

func TestPermissionModeEdit_AddsToTheExistingPermissionsMappingAndKeepsTheRest(t *testing.T) {
	h, _, source := globalModeHandler(t, "# Team rules.\nname: defaults\npermissions:\n  allow: [read(src/**)] # Keep.\nmodel: sonnet\n")
	file := filepath.Join(source, "settings", "defaults.yaml")
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
	h, _, source := globalModeHandler(t, "name: defaults\n")
	for _, value := range []any{"yolo", "", 3, "Plan"} {
		code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": value})
		if code != http.StatusBadRequest || !strings.Contains(body, "default, manual, acceptEdits, plan, auto, dontAsk, bypassPermissions") {
			t.Errorf("%v: %d %s", value, code, body)
		}
	}
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "value": "plan", "target": "claude"}); code != http.StatusBadRequest {
		t.Errorf("per-tool save = %d %s", code, body)
	}
	if got := readProjectFile(t, filepath.Join(source, "settings", "defaults.yaml")); got != "name: defaults\n" {
		t.Fatalf("a refused save changed the file: %q", got)
	}
}

func TestPermissionModeEdit_LocalSaveWritesTheOverlay(t *testing.T) {
	h, _, source := globalModeHandler(t, "name: defaults\npermissions:\n  default-mode: default\n")
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "personal", "value": "bypassPermissions", "expected": nil}); code != http.StatusOK {
		t.Fatalf("save = %d %s", code, body)
	}
	if got := permissionModeIn(t, filepath.Join(source, "local", "settings", "defaults.yaml")); got != "bypassPermissions" {
		t.Fatalf("local = %v", got)
	}
	if got := permissionModeIn(t, filepath.Join(source, "settings", "defaults.yaml")); got != "default" {
		t.Fatalf("global changed to %v", got)
	}
}

func TestPermissionModeEdit_RefusesWhileALocalFileClearsPermissions(t *testing.T) {
	h, _, source := globalModeHandler(t, "name: defaults\npermissions:\n  allow: [read]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "defaults.yaml"), "name: defaults\npermissions: null\n")
	code, body := permissionModeRequest(t, h, map[string]any{"audience": "personal", "value": "plan"})
	if code != http.StatusBadRequest || !strings.Contains(body, "permissions: null") {
		t.Fatalf("save = %d %s", code, body)
	}
}

func TestPermissionModeEdit_ProjectRefusesBecauseOnlyGlobalSyncWritesIt(t *testing.T) {
	h := targetProject(t, "name: defaults\n")
	for _, audience := range []string{"shared", "personal"} {
		code, body := permissionModeRequest(t, h, map[string]any{"audience": audience, "value": "plan"})
		if code != http.StatusBadRequest || !strings.Contains(body, "global sync only") {
			t.Errorf("%s = %d %s", audience, code, body)
		}
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\n" || readProjectFile(t, ".agnostic-ai/local/settings/defaults.yaml") != "" {
		t.Fatalf("a refused save wrote files: %q", got)
	}
}

func TestPermissionModeEdit_RefusesASaveALaterSpecOverrides(t *testing.T) {
	h, home, source := globalModeHandler(t, "")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "a.yaml"), "name: a\npermissions:\n  allow: [read]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "z.yaml"), "name: z\npermissions:\n  default-mode: bypassPermissions\n")
	code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "path": "settings/a.yaml", "value": "plan", "expected": nil})
	if code != http.StatusConflict || !strings.Contains(body, "settings/z.yaml sets permissions.default-mode") {
		t.Fatalf("shadowed save = %d %s", code, body)
	}
	if strings.Contains(readProjectFile(t, filepath.Join(source, "settings", "a.yaml")), "default-mode") {
		t.Fatal("the refused save wrote the earlier spec")
	}
	nativeMode := func() any {
		t.Helper()
		if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
			t.Fatalf("sync --global: %v\n%s", err, warnings)
		}
		var native map[string]any
		if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(home, ".claude", "settings.json"))), &native); err != nil {
			t.Fatal(err)
		}
		perms, _ := native["permissions"].(map[string]any)
		return perms["defaultMode"]
	}
	if got := nativeMode(); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %v, want the later spec's value", got)
	}
	if code, body := permissionModeRequest(t, h, map[string]any{"audience": "shared", "path": "settings/z.yaml", "value": "plan", "expected": "bypassPermissions"}); code != http.StatusOK {
		t.Fatalf("save into the later spec = %d %s", code, body)
	}
	if got := nativeMode(); got != "plan" {
		t.Fatalf("defaultMode = %v, want plan", got)
	}
}
