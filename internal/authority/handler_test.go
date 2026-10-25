package authority

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func demoServer(t *testing.T, opts HandlerOptions) *httptest.Server {
	t.Helper()
	load := func() (*config.Config, spec.Bundle, error) {
		cfg, err := config.Load(demoRoot)
		if err != nil {
			return nil, spec.Bundle{}, err
		}
		b, err := spec.LoadBundle(demoRoot, cfg)
		return cfg, b, err
	}
	opts.Root = demoRoot
	opts.Now = func() time.Time { return fixedNow }
	srv := httptest.NewServer(NewHandler(load, opts))
	t.Cleanup(srv.Close)
	return srv
}

func getJSON(t *testing.T, url string, out any) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return resp
}

func TestHandler_ServesTheViewForTheRequestedSubjectAndHarness(t *testing.T) {
	installedFor := ""
	srv := demoServer(t, HandlerOptions{
		ConfigFiles: func() []string { return []string{"agnostic-ai.yaml"} },
		Installed: func(target string, _ *config.Config, _ spec.Bundle) (InstalledState, error) {
			installedFor = target
			return InstalledState{State: InstalledNotGenerated, Files: []InstalledFile{}}, nil
		},
	})
	var v View
	resp := getJSON(t, srv.URL+"/api/views/authority?agentId=coding-agent&target=codex", &v)

	if resp.StatusCode != http.StatusOK || v.SchemaVersion != SchemaVersion || v.Agent.ID != "coding-agent" || v.Harness.Target != "codex" {
		t.Fatalf("status %d, view %s/%s/%s", resp.StatusCode, v.SchemaVersion, v.Agent.ID, v.Harness.Target)
	}
	if installedFor != "codex" || v.Harness.Installed.State != InstalledNotGenerated || v.Snapshot.Evidence.Installed != "checked" {
		t.Errorf("installed compared for %q, harness %+v", installedFor, v.Harness.Installed)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("content type = %q", resp.Header.Get("Content-Type"))
	}
	var session View
	getJSON(t, srv.URL+"/api/views/authority", &session)
	if session.Agent.ID != SessionSubjectID || session.Harness.Target != "claude" {
		t.Errorf("default view = %s on %s, want the session on the first target", session.Agent.ID, session.Harness.Target)
	}
}

func TestHandler_UnknownAgentOrTargetIsNotFoundAndListsTheValidIDs(t *testing.T) {
	srv := demoServer(t, HandlerOptions{})
	for query, valid := range map[string]string{
		"agentId=nobody": "session, coding-agent, data-analyst, reviewer",
		"target=nowhere": "claude, codex, cursor",
	} {
		var body struct{ Error string }
		resp := getJSON(t, srv.URL+"/api/views/authority?"+query, &body)
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(body.Error, valid) {
			t.Errorf("%s: status %d, error %q", query, resp.StatusCode, body.Error)
		}
	}
}

func TestHandler_ImpactRoundTrip(t *testing.T) {
	srv := demoServer(t, HandlerOptions{})
	var v View
	getJSON(t, srv.URL+"/api/views/authority?agentId=coding-agent", &v)

	body := `{"agentId":"coding-agent","target":"claude","revision":"` + v.Snapshot.Revision + `","changes":[{"op":"add","list":"deny","rule":"mcp:railway/deploy"}]}`
	resp, err := http.Post(srv.URL+"/api/views/authority/impact", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var res ImpactResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !res.Valid || res.BaseRevision != v.Snapshot.Revision || res.Summary.Narrowed == 0 || res.After == nil {
		t.Fatalf("status %d, result %+v", resp.StatusCode, res)
	}

	bad, err := http.Post(srv.URL+"/api/views/authority/impact", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bad.Body.Close() }()
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid JSON status = %d, want 400", bad.StatusCode)
	}
	wrong := getJSON(t, srv.URL+"/api/views/authority/impact", nil)
	if wrong.StatusCode != http.StatusMethodNotAllowed || wrong.Header.Get("Allow") != http.MethodPost {
		t.Errorf("GET impact = %d, Allow %q", wrong.StatusCode, wrong.Header.Get("Allow"))
	}
}

func TestHandler_SchemaAndHealth(t *testing.T) {
	srv := demoServer(t, HandlerOptions{ConfigFiles: func() []string { return []string{"agnostic-ai.yaml"} }})

	var schema struct {
		SchemaVersion string
		Endpoints     []struct{ Method, Path, Description string }
	}
	getJSON(t, srv.URL+"/api/views/authority/schema", &schema)
	if schema.SchemaVersion != SchemaVersion || len(schema.Endpoints) != 4 || schema.Endpoints[1].Method != http.MethodPost {
		t.Errorf("schema = %+v", schema)
	}

	var health struct {
		OK     bool
		Root   string
		Global bool
		UI     bool
	}
	getJSON(t, srv.URL+"/api/health", &health)
	if !health.OK || health.Root != canonicalHealthRoot(t, demoRoot) || health.Global || health.UI {
		t.Errorf("health = %+v", health)
	}
}

func TestHandler_HealthAnswersWithoutLoadingSpecs(t *testing.T) {
	loads := 0
	broken := func() (*config.Config, spec.Bundle, error) {
		loads++
		return nil, spec.Bundle{}, errors.New("broken config")
	}
	srv := httptest.NewServer(NewHandler(broken, HandlerOptions{Root: demoRoot, UIFiles: fstest.MapFS{}}))
	t.Cleanup(srv.Close)
	var health struct {
		OK   bool
		Root string
		UI   bool
	}
	resp := getJSON(t, srv.URL+"/api/health", &health)
	if resp.StatusCode != http.StatusOK || loads != 0 || !health.OK || !health.UI || health.Root != canonicalHealthRoot(t, demoRoot) {
		t.Errorf("health = %d %+v after %d loads", resp.StatusCode, health, loads)
	}
}

func TestHandler_AllowsLocalDevelopmentOriginsOnly(t *testing.T) {
	srv := demoServer(t, HandlerOptions{AllowedOrigins: []string{"http://localhost:5173"}})
	for origin, allowed := range map[string]bool{
		"http://localhost:5173":     true,
		"http://127.0.0.1:4173":     false,
		"http://localhost":          false,
		"https://evil.example":      false,
		"http://localhost.evil.com": false,
	} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/views/authority/schema", nil)
		req.Header.Set("Origin", origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		got := resp.Header.Get("Access-Control-Allow-Origin")
		if allowed && got != origin || !allowed && got != "" {
			t.Errorf("%s: allow-origin %q", origin, got)
		}
	}
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/views/authority/impact", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || !strings.Contains(resp.Header.Get("Access-Control-Allow-Methods"), "POST") {
		t.Errorf("preflight = %d %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Methods"))
	}
}

func TestHandler_ServesTheClientWithAFallbackToIndex(t *testing.T) {
	ui := t.TempDir()
	for path, body := range map[string]string{"index.html": "<!doctype html><title>app</title>", "assets/app.js": "console.log(1)"} {
		full := filepath.Join(ui, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := demoServer(t, HandlerOptions{UIDir: ui})
	for path, want := range map[string]string{
		"/assets/app.js":            "console.log(1)",
		"/agents/coding-agent":      "<title>app</title>",
		"/":                         "<title>app</title>",
		"/api/views/does-not-exist": `"error"`,
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if !strings.Contains(string(body), want) {
			t.Errorf("%s = %d %q, want %q", path, resp.StatusCode, body, want)
		}
		if strings.HasPrefix(path, "/api/") && resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestHandler_MissingAssetsAreNotHTML(t *testing.T) {
	ui := t.TempDir()
	if err := os.WriteFile(filepath.Join(ui, "index.html"), []byte("<!doctype html><title>app</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := demoServer(t, HandlerOptions{UIDir: ui})
	for _, path := range []string{"/assets/missing.js", "/missing.css", "/favicon.ico", "/api/missing"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNotFound || strings.Contains(string(body), "<title>app</title>") {
			t.Errorf("%s: status %d, body %q", path, resp.StatusCode, body)
		}
	}
}

func TestHandler_ServesEmbeddedFilesAndClientRoutes(t *testing.T) {
	files := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><title>embedded</title>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	srv := demoServer(t, HandlerOptions{UIFiles: files})
	for _, check := range []struct {
		method string
		path   string
		status int
		body   string
	}{
		{http.MethodGet, "/", http.StatusOK, "<title>embedded</title>"},
		{http.MethodGet, "/context/details", http.StatusOK, "<title>embedded</title>"},
		{http.MethodGet, "/assets/app.js", http.StatusOK, "console.log(1)"},
		{http.MethodHead, "/assets/app.js", http.StatusOK, ""},
		{http.MethodGet, "/assets/", http.StatusNotFound, "404"},
		{http.MethodGet, "/missing.css", http.StatusNotFound, "404"},
		{http.MethodPost, "/", http.StatusMethodNotAllowed, "use GET or HEAD"},
	} {
		req, err := http.NewRequest(check.method, srv.URL+check.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != check.status || !strings.Contains(string(body), check.body) {
			t.Errorf("%s %s: status %d, body %q", check.method, check.path, resp.StatusCode, body)
		}
		if check.method == http.MethodHead && len(body) != 0 {
			t.Errorf("HEAD returned a body: %q", body)
		}
	}
}

func canonicalHealthRoot(t *testing.T, root string) string {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestHandler_HealthIdentifiesGlobalScope(t *testing.T) {
	srv := demoServer(t, HandlerOptions{Global: true})
	var health struct {
		Root   string
		Global bool
	}
	getJSON(t, srv.URL+"/api/health", &health)
	if !health.Global || health.Root != canonicalHealthRoot(t, demoRoot) {
		t.Errorf("health = %+v", health)
	}
}
