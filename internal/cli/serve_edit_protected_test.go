package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func protectedRequest(t *testing.T, h http.Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return postEdit(t, h, "/api/edit/protected", string(data), nil)
}

func protectedPreview(t *testing.T, h http.Handler, body map[string]any) map[string]any {
	t.Helper()
	body["action"] = "preview"
	rec := protectedRequest(t, h, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func protectedApply(t *testing.T, h http.Handler, body, preview map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body["action"], body["expected"] = "apply", preview["expected"]
	return protectedRequest(t, h, body)
}

func protectedIn(t *testing.T, file string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(readProjectFile(t, file)), &doc); err != nil {
		t.Fatal(err)
	}
	block, _ := doc["protected"].(map[string]any)
	return block
}

func TestProtectedEdit_PreviewsThenWritesTheCanonicalBlock(t *testing.T) {
	h := targetProject(t, "")
	file := ".agnostic-ai/settings/permissions.yaml"
	body := map[string]any{"audience": "shared", "protected": map[string]any{"paths": []string{".github/**", " composer.lock "}, "decision": "deny", "reason": "CI changes  on purpose."}, "before": nil}
	preview := protectedPreview(t, h, body)
	if readProjectFile(t, file) != "" {
		t.Fatal("preview wrote the file")
	}
	if preview["schemaVersion"] != "protected-edit/v1" || preview["file"] != file || preview["syncCommand"] != "agnostic-ai sync" || !strings.Contains(preview["diff"].(string), "composer.lock") {
		t.Fatalf("preview = %v", preview)
	}
	if rec := protectedApply(t, h, body, preview); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	got := readProjectFile(t, file)
	if !strings.Contains(got, "protected:\n  paths:\n    - .github/**\n    - composer.lock\n  decision: deny\n  reason: CI changes on purpose.\n") {
		t.Fatalf("file = %q", got)
	}
	replace := map[string]any{"audience": "shared", "protected": map[string]any{"paths": []string{"migrations/"}, "decision": "ask", "reason": ""}, "before": map[string]any{"paths": []string{".github/**", "composer.lock"}, "decision": "deny", "reason": "CI changes on purpose."}}
	if rec := protectedApply(t, h, replace, protectedPreview(t, h, replace)); rec.Code != http.StatusOK {
		t.Fatalf("replace = %d %s", rec.Code, rec.Body)
	}
	if block := protectedIn(t, file); len(block) != 2 || block["decision"] != "ask" || block["paths"].([]any)[0] != "migrations/" {
		t.Fatalf("replaced block = %v in %q", block, readProjectFile(t, file))
	}
	remove := map[string]any{"audience": "shared", "protected": nil}
	if rec := protectedApply(t, h, remove, protectedPreview(t, h, remove)); rec.Code != http.StatusOK {
		t.Fatalf("remove = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "" {
		t.Fatalf("removing the only block left the created spec: %q", got)
	}
}

func TestProtectedEdit_KeepsOtherValuesInAnExistingSpec(t *testing.T) {
	original := "# Team rules.\nname: defaults\npermissions:\n  allow: [read] # Keep.\nprotected:\n  paths: [old]\nmodel: sonnet\n"
	h := targetProject(t, original)
	file := ".agnostic-ai/settings/defaults.yaml"
	body := map[string]any{"audience": "shared", "protected": map[string]any{"paths": []string{"vendor"}, "decision": "ask", "reason": "Vendored."}}
	if rec := protectedApply(t, h, body, protectedPreview(t, h, body)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	got := readProjectFile(t, file)
	for _, kept := range []string{"# Team rules.\nname: defaults\n", "  allow: [read] # Keep.\n", "model: sonnet\n"} {
		if !strings.Contains(got, kept) {
			t.Errorf("lost %q in %q", kept, got)
		}
	}
	if strings.Contains(got, "old") || protectedIn(t, file)["reason"] != "Vendored." {
		t.Fatalf("file = %q", got)
	}
}

func TestProtectedEdit_RejectsStaleAndInvalidSaves(t *testing.T) {
	h := targetProject(t, "name: defaults\n")
	file := ".agnostic-ai/settings/defaults.yaml"
	body := map[string]any{"audience": "shared", "protected": map[string]any{"paths": []string{"vendor"}, "decision": "ask", "reason": ""}}
	preview := protectedPreview(t, h, body)
	mustWrite(t, file, "name: defaults\nmodel: sonnet\n")
	if rec := protectedApply(t, h, body, preview); rec.Code != http.StatusConflict {
		t.Fatalf("stale apply = %d %s", rec.Code, rec.Body)
	}
	stale := map[string]any{"action": "preview", "audience": "shared", "protected": body["protected"], "before": map[string]any{"paths": []string{"x"}, "decision": "ask", "reason": ""}}
	if rec := protectedRequest(t, h, stale); rec.Code != http.StatusConflict {
		t.Fatalf("stale draft = %d %s", rec.Code, rec.Body)
	}
	for name, block := range map[string]any{
		"no paths":     map[string]any{"paths": []string{}, "decision": "ask", "reason": ""},
		"bad decision": map[string]any{"paths": []string{"a"}, "decision": "block", "reason": ""},
		"braces":       map[string]any{"paths": []string{"src/{a,b}"}, "decision": "ask", "reason": ""},
		"outside":      map[string]any{"paths": []string{"../secret"}, "decision": "ask", "reason": ""},
		"two lines":    map[string]any{"paths": []string{"a\nb"}, "decision": "ask", "reason": ""},
	} {
		rec := protectedRequest(t, h, map[string]any{"action": "preview", "audience": "shared", "protected": block})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s", name, rec.Code, rec.Body)
		}
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel: sonnet\n" {
		t.Fatalf("refused saves changed the file: %q", got)
	}
}

func TestProtectedEdit_LocalSaveWritesEveryKeySoTheProjectReasonDoesNotLeak(t *testing.T) {
	h := targetProject(t, "name: defaults\nprotected:\n  paths: [.github/**]\n  reason: CI only.\n")
	isolateGit(t)
	git(t, h.root, "init", "-q")
	body := map[string]any{"audience": "personal", "protected": map[string]any{"paths": []string{"notes/"}, "decision": "deny", "reason": ""}, "before": nil}
	preview := protectedPreview(t, h, body)
	local := ".agnostic-ai/local/settings/defaults.yaml"
	if preview["file"] != local {
		t.Fatalf("preview = %v", preview)
	}
	if rec := protectedApply(t, h, body, preview); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	block := protectedIn(t, local)
	if reason, set := block["reason"]; !set || reason != "" || block["decision"] != "deny" {
		t.Fatalf("local block = %v in %q", block, readProjectFile(t, local))
	}
	if !strings.Contains(readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"), "CI only.") {
		t.Fatal("the local save changed the project spec")
	}
}

func TestProtectedEdit_GlobalViewRefuses(t *testing.T) {
	root := t.TempDir()
	h := &editHandler{root: root, global: true}
	rec := protectedRequest(t, h, map[string]any{"action": "preview", "audience": "shared", "protected": map[string]any{"paths": []string{"a"}, "decision": "ask", "reason": ""}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sync --global does not write protected paths") {
		t.Fatalf("global = %d %s", rec.Code, rec.Body)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Fatal("global refusal wrote files")
	}
}

func TestProtectedEdit_ComparesBeforeAsThePageShowsIt(t *testing.T) {
	h := targetProject(t, "name: defaults\nprotected:\n  paths: [.github/**]\n")
	isolateGit(t)
	git(t, h.root, "init", "-q")
	body := map[string]any{"audience": "personal", "protected": map[string]any{"paths": []string{"notes/"}, "decision": "ask", "reason": ""}, "before": nil}
	if rec := protectedApply(t, h, body, protectedPreview(t, h, body)); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	shown := map[string]any{"paths": []string{"notes/"}, "decision": "ask", "reason": ""}
	reset := map[string]any{"audience": "personal", "protected": nil, "before": shown}
	if rec := protectedApply(t, h, reset, protectedPreview(t, h, reset)); rec.Code != http.StatusOK {
		t.Fatalf("reset after an empty local reason = %d %s", rec.Code, rec.Body)
	}
	mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\nprotected:\n  decision: deny\n")
	partial := map[string]any{"audience": "personal", "protected": nil, "before": map[string]any{"paths": []string{}, "decision": "deny", "reason": ""}}
	preview := protectedPreview(t, h, partial)
	if before, _ := preview["before"].(map[string]any); before == nil || before["paths"] == nil {
		t.Fatalf("partial block preview = %v", preview)
	}
}

func TestProtectedEdit_RefusesValuesThePageHides(t *testing.T) {
	h := targetProject(t, "name: defaults\nprotected:\n  paths: [src/AbstractSingletonProxyFactoryBeanImpl.java]\n")
	hidden := map[string]any{"action": "preview", "audience": "shared", "protected": map[string]any{"paths": []string{"<redacted>"}, "decision": "ask", "reason": ""}}
	if rec := protectedRequest(t, h, hidden); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "hidden value") {
		t.Fatalf("hidden draft = %d %s", rec.Code, rec.Body)
	}
	replace := map[string]any{"action": "preview", "audience": "shared", "protected": map[string]any{"paths": []string{"vendor"}, "decision": "ask", "reason": ""}}
	if rec := protectedRequest(t, h, replace); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "page hides") {
		t.Fatalf("hidden source = %d %s", rec.Code, rec.Body)
	}
}
