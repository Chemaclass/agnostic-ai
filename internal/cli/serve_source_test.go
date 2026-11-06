package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

func sourceProject(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(envUserGlobalRoot, t.TempDir())
	budgetProject(t, "targets: [claude]\n")
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\ndescription: Code style.\nglobs: ['**/*.go', '**/*.ts']\n---\nKeep functions small.\n")
	srv := httptest.NewServer(newServeMux(nil, authority.HandlerOptions{Root: dir}))
	t.Cleanup(srv.Close)
	return dir, srv
}

func absSlash(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(abs)
}

func getSource(t *testing.T, srv *httptest.Server, path string, status int) sourceView {
	t.Helper()
	target := srv.URL + sourceViewPath
	if path != "" {
		target += "?path=" + url.QueryEscape(path)
	}
	resp, err := http.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != status {
		t.Fatalf("%s: status %d, want %d", path, resp.StatusCode, status)
	}
	var view sourceView
	if status == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
			t.Fatal(err)
		}
	}
	return view
}

func TestServeSource_ReturnsARuleWithItsFrontmatterInFileOrder(t *testing.T) {
	dir, srv := sourceProject(t)
	path := absSlash(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"))
	view := getSource(t, srv, path, http.StatusOK)
	var keys []string
	for _, f := range view.Frontmatter {
		keys = append(keys, f.Key+"="+f.Value)
	}
	if view.Path != path || view.Kind != "rule" || view.Body != "Keep functions small.\n" || view.Truncated {
		t.Errorf("view: %+v", view)
	}
	if got := strings.Join(keys, "|"); got != "name=style|description=Code style.|globs=**/*.go, **/*.ts" {
		t.Errorf("frontmatter: %s", got)
	}
	info, _ := os.Stat(path)
	if view.Bytes != info.Size() {
		t.Errorf("bytes: %d, want %d", view.Bytes, info.Size())
	}
	if instructions := getSource(t, srv, absSlash(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")), http.StatusOK); instructions.Kind != "instructions" || instructions.Body != "Root guidance.\n" || len(instructions.Frontmatter) != 0 {
		t.Errorf("instructions: %+v", instructions)
	}
}

func TestServeSource_RefusesPathsOutsideTheLoadedConfiguration(t *testing.T) {
	dir, srv := sourceProject(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	mustWriteFile(t, outside, "---\nname: secret\n---\nOutside.\n")
	mustWriteFile(t, filepath.Join(dir, "README.md"), "Readme.\n")
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "Generated.\n")
	rule := absSlash(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"))
	for name, path := range map[string]string{
		"no path":           "",
		"relative path":     ".agnostic-ai/rules/style.md",
		"traversal":         absSlash(t, filepath.Join(dir, ".agnostic-ai", "rules")) + "/../rules/style.md",
		"file outside":      absSlash(t, outside),
		"project file":      absSlash(t, filepath.Join(dir, "README.md")),
		"generated file":    absSlash(t, filepath.Join(dir, "CLAUDE.md")),
		"config file":       absSlash(t, filepath.Join(dir, "agnostic-ai.yaml")),
		"missing spec":      absSlash(t, filepath.Join(dir, ".agnostic-ai", "rules", "gone.md")),
		"trailing slash":    rule + "/",
		"doubled separator": strings.Replace(rule, "/rules/", "//rules/", 1),
	} {
		t.Run(name, func(t *testing.T) { getSource(t, srv, path, http.StatusNotFound) })
	}
}

func TestServeSource_RefusesASpecWhoseSymlinkLeadsOutside(t *testing.T) {
	dir, srv := sourceProject(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	mustWriteFile(t, outside, "---\nname: linked\n---\nOutside.\n")
	link := filepath.Join(dir, ".agnostic-ai", "rules", "linked.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if files, _, err := sourceFiles(false); err != nil || files[absSlash(t, link)] != "rule" {
		t.Fatalf("the loader should list the linked rule: %v %v", files, err)
	}
	getSource(t, srv, absSlash(t, link), http.StatusNotFound)
}

func TestServeSource_RefusesABuiltinSpec(t *testing.T) {
	dir, srv := sourceProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nbuiltins: [memory]\n")
	_, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range b.Skills {
		if e.Layer == "builtin" && e.Path != "" {
			found = true
			getSource(t, srv, absSlash(t, e.Path), http.StatusNotFound)
		}
	}
	if !found {
		t.Fatal("memory built-in loaded no skill")
	}
}

func TestServeSource_GuardsMethodHostAndOrigin(t *testing.T) {
	dir, srv := sourceProject(t)
	target := srv.URL + sourceViewPath + "?path=" + url.QueryEscape(absSlash(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md")))
	for _, tc := range []struct {
		name   string
		method string
		host   string
		origin string
		status int
	}{
		{name: "post", method: http.MethodPost, status: http.StatusMethodNotAllowed},
		{name: "foreign host", method: http.MethodGet, host: "attacker.example", status: http.StatusMisdirectedRequest},
		{name: "foreign origin", method: http.MethodGet, origin: "http://attacker.example", status: http.StatusForbidden},
		{name: "own origin", method: http.MethodGet, origin: srv.URL, status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.host != "" {
				req.Host = tc.host
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.status)
			}
		})
	}
}

func TestServeSource_HandlerGuardsOriginWithoutTheOuterServer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+sourceViewPath+"?path=/x.md", nil)
	req.Header.Set("Origin", "http://attacker.example")
	rec := httptest.NewRecorder()
	newSourceHandler(false).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", rec.Code)
	}
}

func TestServeSource_CutsALargeBodyAndSaysSo(t *testing.T) {
	dir, srv := sourceProject(t)
	body := strings.Repeat("é", sourceBodyLimit)
	path := filepath.Join(dir, ".agnostic-ai", "rules", "large.md")
	mustWriteFile(t, path, "---\nname: large\n---\n"+body)
	view := getSource(t, srv, absSlash(t, path), http.StatusOK)
	if !view.Truncated || len(view.Body) > sourceBodyLimit || !strings.HasPrefix(view.Body, "éé") || strings.ContainsRune(view.Body, '�') {
		t.Errorf("truncated %v, body length %d", view.Truncated, len(view.Body))
	}
	if view.Bytes != int64(len("---\nname: large\n---\n")+len(body)) || len(view.Frontmatter) != 1 {
		t.Errorf("bytes %d, frontmatter %+v", view.Bytes, view.Frontmatter)
	}
}

func TestServeSource_ServesGlobalAndLocalFilesInTheGlobalView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv(envUserGlobalRoot, home)
	project := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(project, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nProject rule.\n")
	mustWriteFile(t, filepath.Join(home, "AGNOSTIC_AI.md"), "Global guidance.\n")
	mustWriteFile(t, filepath.Join(home, "local", "AGNOSTIC_AI.md"), "Local guidance.\n")
	mustWriteFile(t, filepath.Join(home, "local", "rules", "mine.md"), "---\nname: mine\n---\nLocal rule.\n")
	srv := httptest.NewServer(newServeMux(nil, authority.HandlerOptions{Root: home, Global: true}))
	defer srv.Close()
	if view := getSource(t, srv, absSlash(t, filepath.Join(home, "local", "rules", "mine.md")), http.StatusOK); view.Kind != "rule" || view.Body != "Local rule.\n" {
		t.Errorf("local rule: %+v", view)
	}
	if view := getSource(t, srv, absSlash(t, filepath.Join(home, "local", "AGNOSTIC_AI.md")), http.StatusOK); view.Kind != "instructions" {
		t.Errorf("local instructions: %+v", view)
	}
	getSource(t, srv, absSlash(t, filepath.Join(project, ".agnostic-ai", "rules", "style.md")), http.StatusNotFound)
}

func TestServeSource_ServesEveryContextRowTheOverviewCanOpen(t *testing.T) {
	dir, srv := sourceProject(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews.\ntools: [Read, Grep]\n---\nReview.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "deploy", "SKILL.md"), "---\nname: deploy\ndescription: Deploys.\n---\nDeploy.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "local", "rules", "mine.md"), "---\nname: mine\n---\nLocal rule.\n")
	resp, err := http.Get(srv.URL + overview.OverviewPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var view overview.View
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, s := range view.Sections {
		if s.ID != overview.SectionContext {
			continue
		}
		for _, item := range s.Items {
			if item.OpenPath == "" {
				continue
			}
			kinds[item.Kind] = true
			if got := getSource(t, srv, item.OpenPath, http.StatusOK); got.Kind != item.Kind {
				t.Errorf("%s: kind %q, want %q", item.OpenPath, got.Kind, item.Kind)
			}
		}
	}
	for _, kind := range []string{"instructions", "rule", "agent", "skill"} {
		if !kinds[kind] {
			t.Errorf("no %s row to open: %v", kind, kinds)
		}
	}
}
