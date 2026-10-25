package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// scopedServer serves the working directory's project, or the global
// configuration with global set, as serve builds it.
func scopedServer(t *testing.T, global bool) http.Handler {
	t.Helper()
	src, err := openAuthoritySource(".", global)
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := serveScopes(src, authority.HandlerOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	return newServeMux(scopes...)
}

func serveGet(t *testing.T, h http.Handler, path string, into any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+path, nil))
	if into != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatalf("GET %s: %v\n%s", path, err, rec.Body)
		}
	}
	return rec.Code
}

type scopesListing struct {
	Default string `json:"default"`
	Scopes  []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Root string `json:"root"`
	} `json:"scopes"`
}

func TestServeScopes_ProjectServerListsTheGlobalConfigurationToo(t *testing.T) {
	home := globalOverviewHome(t)
	project, err := filepath.EvalSymlinks(demoProject(t))
	if err != nil {
		t.Fatal(err)
	}
	chdir(t, project)

	var got scopesListing
	if code := serveGet(t, scopedServer(t, false), scopesPath, &got); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if got.Default != scopeProject || len(got.Scopes) != 2 ||
		got.Scopes[0].ID != scopeProject || got.Scopes[0].Name != "demo-project" || got.Scopes[0].Root != project ||
		got.Scopes[1].ID != scopeGlobal || got.Scopes[1].Name != "" || got.Scopes[1].Root != filepath.Join(home, "source") {
		t.Errorf("scopes = %+v", got)
	}
}

func TestServeScopes_GlobalServerListsOnlyTheGlobalConfiguration(t *testing.T) {
	home := globalOverviewHome(t)
	chdir(t, t.TempDir())
	h := scopedServer(t, true)

	var got scopesListing
	if code := serveGet(t, h, scopesPath, &got); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if got.Default != scopeGlobal || len(got.Scopes) != 1 || got.Scopes[0].ID != scopeGlobal || got.Scopes[0].Root != filepath.Join(home, "source") {
		t.Errorf("scopes = %+v", got)
	}
	var refusal editError
	if code := serveGet(t, h, overview.OverviewPath+"?scope=project", &refusal); code != http.StatusBadRequest || refusal.Error != "this server reads the global configuration only; start serve in a project to read one" {
		t.Errorf("scope=project = %d %q", code, refusal.Error)
	}
}

func TestServeScopes_RefusesAnUnknownScopeOnEveryAPIRoute(t *testing.T) {
	globalOverviewHome(t)
	chdir(t, demoProject(t))
	h := scopedServer(t, false)
	for _, path := range []string{overview.OverviewPath, "/api/health", "/api/views/authority", builtinStatusPath} {
		var refusal editError
		if code := serveGet(t, h, path+"?scope=team", &refusal); code != http.StatusBadRequest || refusal.Error != `unknown scope "team"; use project or global` {
			t.Errorf("%s = %d %q", path, code, refusal.Error)
		}
	}
	rec := postEdit(t, h, editBuiltinPath+"?scope=team", `{"name":"memory","enabled":true}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("edit with an unknown scope = %d %s", rec.Code, rec.Body)
	}
}

func TestServeScopes_GlobalScopeOnAProjectServerReadsAndEditsTheGlobalSource(t *testing.T) {
	silence(t)
	home := globalOverviewHome(t)
	project := demoProject(t)
	chdir(t, project)
	h := scopedServer(t, false)
	projectConfig, _ := os.ReadFile(filepath.Join(project, "agnostic-ai.yaml"))

	var view overview.View
	if code := serveGet(t, h, overview.OverviewPath, &view); code != http.StatusOK || view.Snapshot.Project.Name != "demo-project" {
		t.Errorf("default = %d %q, want the project", code, view.Snapshot.Project.Name)
	}
	view = overview.View{}
	if code := serveGet(t, h, overview.OverviewPath+"?scope=global", &view); code != http.StatusOK || !strings.HasPrefix(view.Snapshot.Project.Name, "global (") {
		t.Errorf("scope=global = %d %q, want the global configuration", code, view.Snapshot.Project.Name)
	}
	var health struct {
		Revision string `json:"revision"`
	}
	if serveGet(t, h, "/api/health?scope=global", &health); health.Revision != view.Snapshot.Revision {
		t.Errorf("global health revision %s, overview %s", health.Revision, view.Snapshot.Revision)
	}

	if rec := postEdit(t, h, editBuiltinPath+"?scope=global", `{"name":"memory","enabled":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("global edit = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(filepath.Join(home, "source", "agnostic-ai.yaml")); !strings.Contains(string(data), "builtins:\n  - memory") {
		t.Errorf("global config after the edit:\n%s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(project, "agnostic-ai.yaml")); string(data) != string(projectConfig) {
		t.Errorf("a global edit changed the project config:\n%s", data)
	}
	if wd, _ := os.Getwd(); filepath.Base(wd) != "demo-project" {
		t.Errorf("serving the global scope left the project directory for %s", wd)
	}
}

func TestServeScopes_CavemanUndoStaysInItsScope(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteGlobalTest(t, filepath.Join(os.Getenv("AGNOSTIC_AI_HOME"), "agnostic-ai.yaml"), "targets: [claude]\n")
	h := scopedServer(t, false)

	proposal := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	if !applied.UndoAvailable {
		t.Fatalf("apply = %+v", applied)
	}

	var global builtinChangeView
	if code := serveGet(t, h, builtinStatusPath+"?scope=global", &global); code != http.StatusOK || global.Status != "idle" || global.UndoAvailable || global.UndoID != "" || global.Supported {
		t.Errorf("global change state = %d %+v, want idle with no undo", code, global)
	}
	if rec := postEdit(t, h, builtinUndoPath+"?scope=global", `{"id":"`+applied.ID+`"}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("undo in the global scope = %d %s, want refused", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude/skills/caveman/SKILL.md")); err != nil {
		t.Errorf("a refused global undo touched the project: %v", err)
	}

	var project builtinChangeView
	if serveGet(t, h, builtinStatusPath+"?scope=project", &project); !project.UndoAvailable || project.UndoID != applied.ID {
		t.Errorf("project change state = %+v, want the undo kept", project)
	}
	if undone := builtinAction(t, h, builtinUndoPath+"?scope=project", applied.ID); undone.Status != "undone" {
		t.Errorf("project undo = %+v", undone)
	}
}

func TestServe_ARelativeGlobalHomeStaysWhereServeStarted(t *testing.T) {
	home := globalOverviewHome(t)
	project := t.TempDir()
	writeBareReadProject(t, project)
	chdir(t, home)
	t.Setenv("AGNOSTIC_AI_HOME", "source")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	cmd := newServeCmdWithBrowser(func(string) error { return nil })
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--dir", project, "--addr", "127.0.0.1:0", "--api-only"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{home, resolved} {
		if strings.Contains(out.String(), "global configuration in "+filepath.Join(root, "source")+" with") {
			return
		}
	}
	t.Errorf("the global scope does not name %s:\n%s", filepath.Join(home, "source"), out.String())
}
