package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func permissionRequest(t *testing.T, h http.Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return postEdit(t, h, "/api/edit/permissions", string(data), nil)
}

func permissionPreviewRequest(t *testing.T, h http.Handler, body map[string]any) map[string]any {
	t.Helper()
	body["action"] = "preview"
	rec := permissionRequest(t, h, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func permissionApplyRequest(t *testing.T, h http.Handler, body, preview map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body["action"], body["expected"] = "apply", preview["expected"]
	return permissionRequest(t, h, body)
}

func TestPermissionEdit_PreviewsThenSavesOnlyTheSelectedList(t *testing.T) {
	original := "# Team rules.\nname: defaults\npermissions:\n  allow: [read(src/**)] # Keep this comment.\n  deny: [shell(rm -rf *)]\nmodel: sonnet # Keep model.\n"
	h := targetProject(t, original)
	file := ".agnostic-ai/settings/defaults.yaml"
	body := map[string]any{"audience": "shared", "key": "permissions.allow", "rules": []string{"read(src/**)", "shell(go test:*)"}, "before": []string{"read(src/**)"}}
	proposed := permissionPreviewRequest(t, h, body)
	if got := readProjectFile(t, file); got != original {
		t.Fatal("preview changed the source")
	}
	if proposed["schemaVersion"] != "permission-edit/v1" || proposed["file"] != file || proposed["syncCommand"] != "agnostic-ai sync" || !strings.Contains(proposed["diff"].(string), "shell(go test:*)") {
		t.Fatalf("preview = %v", proposed)
	}
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	got := readProjectFile(t, file)
	for _, kept := range []string{"# Team rules.\nname: defaults\n", "# Keep this comment.", "  deny: [shell(rm -rf *)]\nmodel: sonnet # Keep model.\n"} {
		if !strings.Contains(got, kept) {
			t.Errorf("lost unrelated text %q in %q", kept, got)
		}
	}
	if !strings.Contains(got, "shell(go test:*)") {
		t.Errorf("rule not saved: %s", got)
	}
}

func TestPermissionEdit_PersonalSaveInheritsOtherListsAndResetRestoresShared(t *testing.T) {
	h := targetProject(t, "name: defaults\npermissions:\n  allow: [read]\n  deny: [shell(rm -rf *)]\n")
	isolateGit(t)
	git(t, h.root, "init", "-q")
	git(t, h.root, "add", ".")
	git(t, h.root, "commit", "-qm", "base")
	beforeExclude := excludeOf(t, h.root)
	body := map[string]any{"audience": "personal", "key": "permissions.allow", "rules": []string{"read(src/**)"}, "before": nil}
	proposed := permissionPreviewRequest(t, h, body)
	local := ".agnostic-ai/local/settings/defaults.yaml"
	if proposed["file"] != local || readProjectFile(t, local) != "" || excludeOf(t, h.root) != beforeExclude {
		t.Fatalf("preview changed files or Git: %v", proposed)
	}
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != 200 {
		t.Fatalf("personal apply = %d %s", rec.Code, rec.Body)
	}
	assertGitHides(t, h.root, local)
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	perms := scope.bundle.Settings[0].Meta["permissions"].(map[string]any)
	if got := perms["deny"].([]any)[0]; got != "shell(rm -rf *)" {
		t.Errorf("inherited deny = %v", got)
	}
	body["rules"], body["before"] = nil, []string{"read(src/**)"}
	proposed = permissionPreviewRequest(t, h, body)
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != 200 {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, local); got != "" {
		t.Errorf("reset retained created overlay: %q", got)
	}
}

func TestPermissionEdit_RefusesStaleListsSourcesAndNewPersonalOverrides(t *testing.T) {
	h := targetProject(t, "name: defaults\npermissions:\n  allow: [read]\n")
	body := map[string]any{"action": "preview", "audience": "shared", "key": "permissions.allow", "rules": []string{"edit"}, "before": []string{"shell"}}
	if rec := permissionRequest(t, h, body); rec.Code != 409 {
		t.Fatalf("stale list = %d %s", rec.Code, rec.Body)
	}
	body["before"] = []string{"read"}
	proposed := permissionPreviewRequest(t, h, body)
	file := ".agnostic-ai/settings/defaults.yaml"
	mustWrite(t, file, "# Later comment.\nname: defaults\npermissions:\n  allow: [read]\n")
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != 409 {
		t.Fatalf("changed raw source = %d %s", rec.Code, rec.Body)
	}
	proposed = permissionPreviewRequest(t, h, body)
	mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\npermissions:\n  allow: [edit]\n")
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != 409 {
		t.Fatalf("new personal override = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); !strings.Contains(got, "allow: [read]") {
		t.Error("refused apply changed the shared rules")
	}
}

func TestPermissionEdit_RequiresAConfiguredSourceAndExplicitChoice(t *testing.T) {
	h := targetProject(t, "name: defaults\npermissions:\n  allow: [read]\n")
	mustWrite(t, ".agnostic-ai/settings/other.yaml", "name: other\neffort: high\n")
	body := map[string]any{"action": "preview", "audience": "shared", "key": "permissions.ask", "rules": []string{"shell"}, "before": nil}
	if rec := permissionRequest(t, h, body); rec.Code != 400 || !strings.Contains(rec.Body.String(), "choose a path") {
		t.Fatalf("ambiguous source = %d %s", rec.Code, rec.Body)
	}
	body["path"] = "../outside.yaml"
	if rec := permissionRequest(t, h, body); rec.Code != 400 {
		t.Fatalf("arbitrary path = %d %s", rec.Code, rec.Body)
	}
	body["path"] = ".agnostic-ai/settings/defaults.yaml"
	permissionPreviewRequest(t, h, body)
}

func TestPermissionEdit_ValidatesGrammarAndKeepsCredentialsOutOfResponses(t *testing.T) {
	h := targetProject(t, "name: defaults\n")
	for _, rule := range []string{"shell(", "shell(curl -H 'Authorization: Bearer abcdefghijklmnop1234567890')", "read\nedit"} {
		body := map[string]any{"action": "preview", "audience": "shared", "key": "permissions.allow", "rules": []string{rule}, "before": nil}
		rec := permissionRequest(t, h, body)
		if rec.Code != 400 || strings.Contains(rec.Body.String(), "abcdefghijklmnop1234567890") {
			t.Errorf("invalid rule = %d %s", rec.Code, rec.Body)
		}
	}
}

func TestPermissionEdit_CreatesConfiguredProjectAndGlobalSources(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "global"}[global], func(t *testing.T) {
			var h *editHandler
			var file string
			if global {
				root := editHome(t)
				if err := os.Remove(filepath.Join(root, "settings", "native.yaml")); err != nil {
					t.Fatal(err)
				}
				h, file = &editHandler{root: root, global: true}, filepath.Join(root, "settings", "permissions.yaml")
			} else {
				h = targetProject(t, "")
				mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  settings: config/policies\n")
				file = filepath.Join(h.root, "config", "policies", "permissions.yaml")
			}
			body := map[string]any{"audience": "shared", "key": "permissions.deny", "rules": []string{"shell(rm -rf *)"}, "before": nil}
			proposed := permissionPreviewRequest(t, h, body)
			if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != 200 {
				t.Fatalf("create = %d %s", rec.Code, rec.Body)
			}
			if got := readProjectFile(t, file); !strings.Contains(got, "name: permissions") || !strings.Contains(got, "shell(rm -rf *)") {
				t.Errorf("created source = %q", got)
			}
		})
	}
}

func TestPermissionEdit_RollsBackAnInvalidSave(t *testing.T) {
	original := "name: defaults\npermissions:\n  allow: [read]\n"
	h := targetProject(t, original)
	body := map[string]any{"audience": "shared", "key": "permissions.allow", "rules": []string{"edit"}, "before": []string{"read"}}
	proposed := permissionPreviewRequest(t, h, body)
	calls := 0
	h.validate = func() ([]string, error) {
		calls++
		if calls > 1 {
			return []string{"permissions changed into an invalid state"}, nil
		}
		return nil, nil
	}
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid apply = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != original {
		t.Errorf("invalid save was not restored: %q", got)
	}
}

func TestPermissionEdit_RechecksSourcesInsideTheAtomicSave(t *testing.T) {
	h := targetProject(t, "name: defaults\npermissions:\n  allow: [read]\n")
	body := map[string]any{"audience": "shared", "key": "permissions.allow", "rules": []string{"edit"}, "before": []string{"read"}}
	proposed := permissionPreviewRequest(t, h, body)
	h.validate = func() ([]string, error) {
		mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\npermissions:\n  allow: [shell]\n")
		return nil, nil
	}
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != http.StatusConflict {
		t.Fatalf("racing override = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); !strings.Contains(got, "allow: [read]") {
		t.Error("racing override allowed a shared save")
	}
}

func TestPermissionEdit_UsesExistingOriginAndMethodGuards(t *testing.T) {
	h := targetProject(t, "name: defaults\n")
	body := `{"action":"preview","audience":"shared","key":"permissions.allow","rules":["read"]}`
	for _, tc := range []struct {
		name   string
		change func(*http.Request)
		want   int
	}{
		{"cross origin", func(r *http.Request) { r.Header.Set("Origin", "https://elsewhere.example") }, http.StatusForbidden},
		{"wrong method", func(r *http.Request) { r.Method = http.MethodGet }, http.StatusMethodNotAllowed},
		{"wrong type", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rec := postEdit(t, h, "/api/edit/permissions", body, tc.change); rec.Code != tc.want {
				t.Errorf("guard = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestPermissionEdit_ResetsAPersonalNullWithoutChangingOtherKeys(t *testing.T) {
	h := targetProject(t, "name: defaults\npermissions:\n  allow: [read]\n")
	local := ".agnostic-ai/local/settings/defaults.yaml"
	mustWrite(t, local, "# Mine.\nname: defaults\npermissions:\n  allow: null\neffort: low\n")
	body := map[string]any{"audience": "personal", "key": "permissions.allow", "rules": nil, "before": nil}
	proposed := permissionPreviewRequest(t, h, body)
	if rec := permissionApplyRequest(t, h, body, proposed); rec.Code != http.StatusOK {
		t.Fatalf("reset null = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, local); got != "# Mine.\nname: defaults\neffort: low\n" {
		t.Errorf("reset changed unrelated keys: %q", got)
	}
}

func TestPermissionEdit_RefusesWholePersonalPermissionsClearForBothAudiences(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "global"}[global], func(t *testing.T) {
			original := "name: defaults\npermissions:\n  allow: [read]\n"
			personal := "# Keep my clear.\nname: defaults\npermissions: null\n"
			var h *editHandler
			shared, local := ".agnostic-ai/settings/defaults.yaml", ".agnostic-ai/local/settings/defaults.yaml"
			if global {
				root := editHome(t)
				h = &editHandler{root: root, global: true}
				shared, local = "settings/native.yaml", "local/settings/defaults.yaml"
			} else {
				h = targetProject(t, original)
			}
			sharedFile, localFile := filepath.Join(h.root, filepath.FromSlash(shared)), filepath.Join(h.root, filepath.FromSlash(local))
			mustWrite(t, sharedFile, original)
			mustWrite(t, localFile, personal)
			for _, audience := range []string{"shared", "personal"} {
				body := map[string]any{"action": "preview", "audience": audience, "path": shared, "key": "permissions.allow", "rules": []string{"edit"}}
				rec := permissionRequest(t, h, body)
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Remove permissions: null") || !strings.Contains(rec.Body.String(), local) {
					t.Errorf("%s whole clear = %d %s", audience, rec.Code, rec.Body)
				}
			}
			if readProjectFile(t, sharedFile) != original || readProjectFile(t, localFile) != personal {
				t.Error("refused permission edit changed source files")
			}
		})
	}
}
