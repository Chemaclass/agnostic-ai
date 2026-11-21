package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every edit endpoint that names a file or spec from the request must
// refuse one that climbs out of the project, and write nothing there.
func TestServeEdit_RefusesRequestPathsOutsideTheProject(t *testing.T) {
	userLevel := editHome(t)
	project := scopeProject(t)
	outside := t.TempDir()
	rel, err := filepath.Rel(project, filepath.Join(outside, "escape.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	escapes := map[string]string{"relative": filepath.ToSlash(rel), "absolute": filepath.Join(outside, "escape.yaml")}
	edit := newEditHandler(project, false)
	move := newSpecMoveHandler(project, false, &switchPeer{root: userLevel, userLevel: true})
	for kind, escape := range escapes {
		for _, audience := range []string{"shared", "personal"} {
			for _, tc := range []struct {
				h        http.Handler
				endpoint string
				body     map[string]any
			}{
				{edit, editValuePath, map[string]any{"audience": audience, "path": escape, "key": "x-claude.verbose", "value": true}},
				{edit, editFieldPath, map[string]any{"audience": audience, "path": escape, "key": "model", "value": "opus"}},
				{edit, editPermissionsPath, map[string]any{"action": "apply", "audience": audience, "path": escape, "key": "permissions.allow", "rules": []string{"read"}, "before": nil}},
				{edit, editProtectedPath, map[string]any{"action": "apply", "audience": audience, "path": escape, "protected": map[string]any{"paths": []string{".env"}, "decision": "deny"}}},
				{edit, editMCPPath, map[string]any{"action": "apply", "audience": audience, "name": escape, "target": "claude", "path": escape, "scope": "project"}},
				{edit, editHookPath, map[string]any{"action": "apply", "audience": audience, "name": escape, "on": "after-edit", "command": "exit 0"}},
				{move, editSpecMovePath, map[string]any{"action": "apply", "kind": "rule", "name": escape, "from": "project", "to": "global", "mode": "copy"}},
			} {
				t.Run(kind+" "+audience+" "+tc.endpoint, func(t *testing.T) {
					body, err := json.Marshal(tc.body)
					if err != nil {
						t.Fatal(err)
					}
					rec := postEdit(t, tc.h, tc.endpoint, string(body), nil)
					if rec.Code < http.StatusBadRequest {
						t.Errorf("escaping request = %d %s, want a refusal", rec.Code, rec.Body)
					}
					if entries, err := os.ReadDir(outside); err != nil || len(entries) > 0 {
						t.Errorf("escaping request wrote outside the project: %v %v", entries, err)
					}
				})
			}
		}
	}
}

// hookPreview builds the file path itself, so it must refuse a name that
// leaves the hooks folder even when the request check is skipped.
func TestHookPreview_RefusesANameOutsideTheHooksFolder(t *testing.T) {
	project := scopeProject(t)
	h := &editHandler{root: project}
	for _, name := range []string{"../escape", filepath.Join(t.TempDir(), "escape")} {
		_, file, err := h.hookPreview(hookSpecRequest{Name: name, On: "after-edit", Command: "exit 0"}, audienceShared)
		if err == nil || file != "" || !strings.Contains(err.Error(), "not a usable file name") {
			t.Errorf("hookPreview(%q) = %q, %v; want a refusal", name, file, err)
		}
	}
}
