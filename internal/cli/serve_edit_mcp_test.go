package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type mcpEditTestReply struct {
	OK          bool     `json:"ok"`
	File        string   `json:"file"`
	Spec        string   `json:"spec"`
	Variables   []string `json:"variables"`
	Targets     []string `json:"targets"`
	SyncCommand string   `json:"syncCommand"`
	Expected    string   `json:"expected"`
}

func mcpEditGlobal(t *testing.T, native string) (string, string) {
	t.Helper()
	silence(t)
	home := nativeMCPHome(t)
	source := filepath.Join(home, "source")
	mustWrite(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, cursor]\n")
	mustWrite(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared instructions.\n")
	path := filepath.Join(home, ".claude.json")
	mustWrite(t, path, native)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return source, path
}

func mcpEditRequest(t *testing.T, h http.Handler, request map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return postEdit(t, h, "/api/edit/mcp", string(body), nil)
}

func mcpEditIdentity(name, audience, scope, path string) map[string]any {
	return map[string]any{"action": "preview", "audience": audience, "name": name, "target": "claude", "path": path, "scope": scope}
}

func mcpEditPreview(t *testing.T, h http.Handler, request map[string]any) mcpEditTestReply {
	t.Helper()
	rec := mcpEditRequest(t, h, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body)
	}
	var response mcpEditTestReply
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Expected == "" {
		t.Fatal("preview lacks its expected revision")
	}
	return response
}

func mcpEditApply(t *testing.T, h http.Handler, request map[string]any, preview mcpEditTestReply) {
	t.Helper()
	request["action"], request["expected"] = "apply", preview.Expected
	rec := mcpEditRequest(t, h, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	var response mcpEditTestReply
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Errorf("apply lacks ok: %s", rec.Body)
	}
}

func TestServeEditMCP_GlobalPreviewAndApplyOnlyTheChosenServer(t *testing.T) {
	native := "{\n  \"mcpServers\": {\n    \"notion\": {\"type\": \"http\", \"url\": \"https://mcp.notion.com/mcp\"},\n    \"other\": {\"command\": \"other-mcp\"}\n  },\n  \"keep\": true\n}\n"
	source, path := mcpEditGlobal(t, native)
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, t.TempDir())
	h := newEditHandler(source, true)
	request := mcpEditIdentity("notion", "shared", "user", "~/.claude.json")
	preview := mcpEditPreview(t, h, request)
	if preview.File != "mcps/notion.yaml" || preview.SyncCommand != "agnostic-ai sync --global" {
		t.Errorf("preview destination = %q, sync = %q", preview.File, preview.SyncCommand)
	}
	if !slices.Contains(preview.Targets, "claude") || !slices.Contains(preview.Targets, "cursor") {
		t.Errorf("preview targets = %v", preview.Targets)
	}
	if _, err := os.Stat(filepath.Join(source, preview.File)); !os.IsNotExist(err) {
		t.Errorf("preview wrote a spec: %v", err)
	}
	mcpEditApply(t, h, request, preview)
	data, err := os.ReadFile(filepath.Join(source, "mcps", "notion.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc) != 3 || doc["name"] != "notion" || doc["type"] != "http" || doc["url"] != "https://mcp.notion.com/mcp" {
		t.Errorf("spec = %s", data)
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "other.yaml")); !os.IsNotExist(err) {
		t.Errorf("another server was imported: %v", err)
	}
	scope, err := loadCheckScope(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.bundle.MCPs) != 1 || scope.bundle.MCPs[0].Name != "notion" {
		t.Errorf("reloaded MCP specs = %v", scope.bundle.MCPs)
	}
	out, warnings, err := runGlobalAgentTest("--dry-run")
	if err != nil {
		t.Fatalf("sync --global --dry-run: %v\n%s\n%s", err, out, warnings)
	}
	for _, targetPath := range []string{".claude.json", ".cursor/mcp.json"} {
		if !strings.Contains(filepath.ToSlash(out), targetPath) {
			t.Errorf("dry run lacks %s: %s", targetPath, out)
		}
	}
	if got := readOptional(t, path); got != native {
		t.Errorf("native file changed: %s", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != beforeInfo.Mode().Perm() {
		t.Errorf("native mode = %o, want original %o", info.Mode().Perm(), beforeInfo.Mode().Perm())
	}
}

func TestServeEditMCP_RewritesEnvCredentialBeforePreviewAndApply(t *testing.T) {
	const token = "ghp_synthetic123"
	source, path := mcpEditGlobal(t, `{"mcpServers":{"github":{"command":"github-mcp","env":{"GITHUB_TOKEN":"`+token+`"}}}}`)
	native := readOptional(t, path)
	h := newEditHandler(source, true)
	request := mcpEditIdentity("github", "shared", "user", "~/.claude.json")
	preview := mcpEditPreview(t, h, request)
	if strings.Contains(preview.Spec, token) || !strings.Contains(preview.Spec, "GITHUB_TOKEN: ${GITHUB_TOKEN}") || !slices.Contains(preview.Variables, "GITHUB_TOKEN") {
		t.Errorf("credential preview = %#v", preview)
	}
	mcpEditApply(t, h, request, preview)
	if got := readOptional(t, filepath.Join(source, preview.File)); got != preview.Spec || strings.Contains(got, token) {
		t.Errorf("written spec differs from safe preview: %s", got)
	}
	if got := readOptional(t, path); got != native {
		t.Error("native credentials changed")
	}
}

func TestServeEditMCP_RefusesAnArgsCredentialImportCannotRewrite(t *testing.T) {
	source, path := mcpEditGlobal(t, `{"mcpServers":{"unsafe":{"command":"sh","args":["-c","exec npx mcp-remote https://x.example/mcp?token='T0KEN'"]}}}`)
	native := readOptional(t, path)
	h := newEditHandler(source, true)
	request := mcpEditIdentity("unsafe", "shared", "user", "~/.claude.json")
	for _, action := range []string{"preview", "apply"} {
		request["action"] = action
		rec := mcpEditRequest(t, h, request)
		if rec.Code < 400 || strings.Contains(rec.Body.String(), "T0KEN") {
			t.Errorf("%s = %d %s", action, rec.Code, rec.Body)
		}
		if action == "preview" && !strings.Contains(rec.Body.String(), "args[1] holds a credential import cannot rewrite") {
			t.Errorf("refusal differs from import: %s", rec.Body)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "unsafe.yaml")); !os.IsNotExist(err) {
		t.Errorf("refusal wrote a spec: %v", err)
	}
	if got := readOptional(t, path); got != native {
		t.Error("refusal changed native file")
	}
}

func TestServeEditMCP_RefusesAnExistingSpecByName(t *testing.T) {
	source, _ := mcpEditGlobal(t, `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`)
	file := filepath.Join(source, "mcps", "different-file.yaml")
	before := "name: notion\ncommand: existing-mcp\n"
	mustWrite(t, file, before)
	rec := mcpEditRequest(t, newEditHandler(source, true), mcpEditIdentity("notion", "shared", "user", "~/.claude.json"))
	if rec.Code < 400 || !strings.Contains(rec.Body.String(), "A spec named notion already exists") {
		t.Errorf("existing spec = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, file); got != before {
		t.Error("existing spec changed")
	}
}

func TestServeEditMCP_RefusesASpecCreatedAfterPreview(t *testing.T) {
	source, _ := mcpEditGlobal(t, `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`)
	h := newEditHandler(source, true)
	request := mcpEditIdentity("notion", "shared", "user", "~/.claude.json")
	preview := mcpEditPreview(t, h, request)
	file := filepath.Join(source, preview.File)
	before := "name: notion\ncommand: existing-mcp\n"
	mustWrite(t, file, before)
	request["action"], request["expected"] = "apply", preview.Expected
	rec := mcpEditRequest(t, h, request)
	if rec.Code < 400 || !strings.Contains(rec.Body.String(), "A spec named notion already exists") {
		t.Errorf("existing destination = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, file); got != before {
		t.Error("concurrent spec changed")
	}
}

func TestServeEditMCP_ProjectPersonalSaveStaysOutOfGit(t *testing.T) {
	nativeMCPHome(t)
	project := gitScopeProject(t)
	const native = `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`
	mustWrite(t, ".mcp.json", native)
	git(t, project, "add", ".mcp.json")
	git(t, project, "commit", "-qm", "native server")
	h := newEditHandler(project, false)
	request := mcpEditIdentity("notion", "personal", "project", ".mcp.json")
	preview := mcpEditPreview(t, h, request)
	if preview.File != ".agnostic-ai/local/mcps/notion.yaml" || preview.SyncCommand != "agnostic-ai sync" {
		t.Errorf("personal preview = %#v", preview)
	}
	mcpEditApply(t, h, request, preview)
	assertGitHides(t, project, preview.File)
	if got := readOptional(t, ".mcp.json"); got != native {
		t.Error("project native file changed")
	}
	if got := readOptional(t, filepath.Join(project, preview.File)); got != preview.Spec {
		t.Errorf("personal spec = %s", got)
	}
}

func TestServeEditMCP_GlobalPersonalSaveStaysOutOfGit(t *testing.T) {
	source, _ := mcpEditGlobal(t, `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`)
	isolateGit(t)
	git(t, source, "init", "-q")
	git(t, source, "add", ".")
	git(t, source, "commit", "-qm", "base")
	h := newEditHandler(source, true)
	request := mcpEditIdentity("notion", "personal", "user", "~/.claude.json")
	preview := mcpEditPreview(t, h, request)
	if preview.File != "local/mcps/notion.yaml" {
		t.Errorf("personal destination = %q", preview.File)
	}
	mcpEditApply(t, h, request, preview)
	assertGitHides(t, source, preview.File)
}

func TestServeEditMCP_ProjectPageImportsAUserServerToProjectSpecs(t *testing.T) {
	source, nativePath := mcpEditGlobal(t, `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`)
	project := scopeProject(t)
	h := newEditHandler(project, false)
	request := mcpEditIdentity("notion", "shared", "user", "~/.claude.json")
	preview := mcpEditPreview(t, h, request)
	if preview.File != ".agnostic-ai/mcps/notion.yaml" {
		t.Errorf("project destination = %q", preview.File)
	}
	mcpEditApply(t, h, request, preview)
	if got := readOptional(t, filepath.Join(project, preview.File)); got != preview.Spec {
		t.Errorf("project spec = %s", got)
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "notion.yaml")); !os.IsNotExist(err) {
		t.Errorf("project save wrote a home spec: %v", err)
	}
	if got := readOptional(t, nativePath); !strings.Contains(got, "mcp.notion.com") {
		t.Error("native user server changed")
	}
}

func TestServeEditMCP_RefusesAnApplyWithoutPreviewOrAfterNativeChange(t *testing.T) {
	source, path := mcpEditGlobal(t, `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`)
	h := newEditHandler(source, true)
	request := mcpEditIdentity("notion", "shared", "user", "~/.claude.json")
	request["action"] = "apply"
	if rec := mcpEditRequest(t, h, request); rec.Code < 400 {
		t.Errorf("apply without preview = %d %s", rec.Code, rec.Body)
	}
	request["action"] = "preview"
	preview := mcpEditPreview(t, h, request)
	mustWrite(t, path, `{"mcpServers":{"notion":{"type":"http","url":"https://changed.example/mcp"}}}`)
	request["action"], request["expected"] = "apply", preview.Expected
	if rec := mcpEditRequest(t, h, request); rec.Code != http.StatusConflict {
		t.Errorf("stale apply = %d %s, want 409", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(source, preview.File)); !os.IsNotExist(err) {
		t.Errorf("stale preview wrote a spec: %v", err)
	}
}

func TestServeEditMCP_PreviewListsShellVariablesOnly(t *testing.T) {
	source, _ := mcpEditGlobal(t, `{"mcpServers":{"workspace":{"command":"node","args":["${workspaceFolder}/mcp.js","${API_TOKEN}"]}}}`)
	preview := mcpEditPreview(t, newEditHandler(source, true), mcpEditIdentity("workspace", "shared", "user", "~/.claude.json"))
	if !slices.Equal(preview.Variables, []string{"API_TOKEN"}) {
		t.Errorf("variables to set = %v, want only API_TOKEN", preview.Variables)
	}
	if !strings.Contains(preview.Spec, "${workspaceFolder}/mcp.js") || !strings.Contains(preview.Spec, "${API_TOKEN}") {
		t.Errorf("preview changed references: %s", preview.Spec)
	}
}

func TestServeEditMCP_RefusesRequestsAnotherSiteCouldSend(t *testing.T) {
	source, nativePath := mcpEditGlobal(t, `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`)
	native := readOptional(t, nativePath)
	h := newEditHandler(source, true)
	body, err := json.Marshal(mcpEditIdentity("notion", "shared", "user", "~/.claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://other.example") }, http.StatusForbidden},
		{"form", func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }, http.StatusUnsupportedMediaType},
		{"foreign host", func(r *http.Request) { r.Host = "other.example:4310" }, http.StatusMisdirectedRequest},
		{"GET", func(r *http.Request) { r.Method = http.MethodGet }, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postEdit(t, h, "/api/edit/mcp", string(body), tc.mutate)
			if rec.Code != tc.status {
				t.Errorf("guard = %d %s, want %d", rec.Code, rec.Body, tc.status)
			}
		})
	}
	if got := readOptional(t, nativePath); got != native {
		t.Error("refused request changed native file")
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "notion.yaml")); !os.IsNotExist(err) {
		t.Errorf("refused request wrote a spec: %v", err)
	}
}

func TestServeEditMCP_UsesTheProjectsConfiguredMCPSource(t *testing.T) {
	nativeMCPHome(t)
	project := scopeProject(t)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  mcps: shared/servers\n")
	mustWrite(t, ".mcp.json", `{"mcpServers":{"notion":{"type":"http","url":"https://mcp.notion.com/mcp"}}}`)
	h := newEditHandler(project, false)
	request := mcpEditIdentity("notion", "shared", "project", ".mcp.json")
	preview := mcpEditPreview(t, h, request)
	if preview.File != "shared/servers/notion.yaml" {
		t.Errorf("configured destination = %q", preview.File)
	}
	mcpEditApply(t, h, request, preview)
	if got := readOptional(t, "shared/servers/notion.yaml"); got != preview.Spec {
		t.Errorf("configured source spec = %s", got)
	}
	if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "mcps", "notion.yaml")); !os.IsNotExist(err) {
		t.Errorf("configured save wrote the default source: %v", err)
	}
}

func TestServeEditMCP_ImportsClaudeLocalServersForTheServedProject(t *testing.T) {
	home := nativeMCPHome(t)
	project := scopeProject(t)
	resolved, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	native, err := json.Marshal(map[string]any{
		"projects": map[string]any{
			resolved: map[string]any{"mcpServers": map[string]any{
				"local-db": map[string]any{"command": "database-mcp", "args": []string{"--local"}},
			}},
			filepath.Join(home, "other-project"): map[string]any{"mcpServers": map[string]any{
				"elsewhere": map[string]any{"command": "other-mcp"},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	nativePath := filepath.Join(home, ".claude.json")
	mustWrite(t, nativePath, string(native))
	h := newEditHandler(project, false)
	request := mcpEditIdentity("local-db", "shared", "project", "~/.claude.json")
	preview := mcpEditPreview(t, h, request)
	if preview.File != ".agnostic-ai/mcps/local-db.yaml" || !strings.Contains(preview.Spec, "command: database-mcp") {
		t.Errorf("local-scope preview = %#v", preview)
	}
	mcpEditApply(t, h, request, preview)
	if got := readOptional(t, filepath.Join(project, preview.File)); got != preview.Spec {
		t.Errorf("local-scope spec = %s", got)
	}
	if got := readOptional(t, nativePath); got != string(native) {
		t.Error("local-scope import changed Claude's file")
	}
	if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "mcps", "elsewhere.yaml")); !os.IsNotExist(err) {
		t.Errorf("another project's server was imported: %v", err)
	}
}
