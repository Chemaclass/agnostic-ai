package cli

import (
	"encoding/json"
	"io"
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

func TestServeSource_ServesRulesFromAnAbsoluteSourceOutsideTheProject(t *testing.T) {
	dir, srv := sourceProject(t)
	external := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nsources:\n  rules: "+filepath.ToSlash(external)+"\n")
	rule := filepath.Join(external, "team.md")
	mustWriteFile(t, rule, "---\nname: team\n---\nTeam rule.\n")
	if view := getSource(t, srv, absSlash(t, rule), http.StatusOK); view.Kind != "rule" || view.Body != "Team rule.\n" {
		t.Errorf("view: %+v", view)
	}
	outside := filepath.Join(t.TempDir(), "secret.md")
	mustWriteFile(t, outside, "---\nname: linked\n---\nOutside.\n")
	link := filepath.Join(external, "linked.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	getSource(t, srv, absSlash(t, link), http.StatusNotFound)
}

func TestServeSource_ServesRulesFromALinkedSourceFolder(t *testing.T) {
	dir, srv := sourceProject(t)
	external := t.TempDir()
	rule := filepath.Join(external, "team.md")
	mustWriteFile(t, rule, "---\nname: team\n---\nTeam rule.\n")
	rules := filepath.Join(dir, ".agnostic-ai", "rules")
	if err := os.RemoveAll(rules); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, rules); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if view := getSource(t, srv, absSlash(t, filepath.Join(rules, "team.md")), http.StatusOK); view.Body != "Team rule.\n" {
		t.Errorf("view: %+v", view)
	}
	outside := filepath.Join(t.TempDir(), "secret.md")
	mustWriteFile(t, outside, "---\nname: linked\n---\nOutside.\n")
	if err := os.Symlink(outside, filepath.Join(external, "linked.md")); err != nil {
		t.Fatal(err)
	}
	getSource(t, srv, absSlash(t, filepath.Join(rules, "linked.md")), http.StatusNotFound)
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
		site   string
		status int
	}{
		{name: "post", method: http.MethodPost, status: http.StatusMethodNotAllowed},
		{name: "foreign host", method: http.MethodGet, host: "attacker.example", status: http.StatusMisdirectedRequest},
		{name: "foreign origin", method: http.MethodGet, origin: "http://attacker.example", status: http.StatusForbidden},
		{name: "cross-site fetch without an origin", method: http.MethodGet, site: "cross-site", status: http.StatusForbidden},
		{name: "head", method: http.MethodHead, status: http.StatusOK},
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
			if tc.site != "" {
				req.Header.Set("Sec-Fetch-Site", tc.site)
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

func TestServeSource_RefusesARawEncodedTraversal(t *testing.T) {
	dir, srv := sourceProject(t)
	resp, err := http.Get(srv.URL + sourceViewPath + "?path=" + absSlash(t, dir) + "/.agnostic-ai/rules/%2e%2e/rules/style.md")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

func TestServeSource_RefusesASkillFolderLinkedOutside(t *testing.T) {
	dir, srv := sourceProject(t)
	outside := filepath.Join(t.TempDir(), "deploy")
	mustWriteFile(t, filepath.Join(outside, "SKILL.md"), "---\nname: deploy\ndescription: Deploys.\n---\nOutside.\n")
	link := filepath.Join(dir, ".agnostic-ai", "skills", "deploy")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	getSource(t, srv, absSlash(t, filepath.Join(link, "SKILL.md")), http.StatusNotFound)
}

func TestServeSource_HidesFrontmatterThatDoesNotParse(t *testing.T) {
	dir, srv := sourceProject(t)
	broken := "---\npassword: hunter2short\nbad: [\n---\nKeep functions small.\n"
	instructions := filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")
	mustWriteFile(t, instructions, broken)
	if view := getSource(t, srv, absSlash(t, instructions), http.StatusOK); view.Body != "Keep functions small.\n" || len(view.Frontmatter) != 0 {
		t.Errorf("instructions: %+v", view)
	}
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "style.md")
	mustWriteFile(t, rule, broken)
	resp, err := http.Get(srv.URL + sourceViewPath + "?path=" + url.QueryEscape(absSlash(t, rule)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(data), "hunter2short") {
		t.Errorf("status %d leaked the value: %s", resp.StatusCode, data)
	}
}

func TestServeSource_ShowsEachNestedFieldWithTheOverviewRedaction(t *testing.T) {
	dir, srv := sourceProject(t)
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "nested.md")
	mustWriteFile(t, rule, "---\nname: nested\nmetadata:\n  type: user\n  api_key: hunter2short\n---\nBody.\n")
	view := getSource(t, srv, absSlash(t, rule), http.StatusOK)
	var got []string
	for _, f := range view.Frontmatter {
		got = append(got, f.Key+"="+f.Value)
	}
	if strings.Join(got, "|") != "name=nested|metadata.api_key=<redacted>|metadata.type=user" {
		t.Errorf("frontmatter: %v", got)
	}
}

func TestServeSource_NeverShowsFrontmatterThatClosesPastTheBodyLimit(t *testing.T) {
	dir, srv := sourceProject(t)
	get := func(path string) (sourceView, string) {
		t.Helper()
		resp, err := http.Get(srv.URL + sourceViewPath + "?path=" + url.QueryEscape(absSlash(t, path)))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", resp.StatusCode, data)
		}
		var view sourceView
		if err := json.Unmarshal(data, &view); err != nil {
			t.Fatal(err)
		}
		return view, string(data)
	}
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "big.md")
	mustWriteFile(t, rule, "---\nname: big\napi_key: hunter2short\npad: "+strings.Repeat("a", sourceBodyLimit+1024)+"\n---\nBody.\n")
	view, raw := get(rule)
	if strings.Contains(raw, "hunter2short") || view.Body != "Body.\n" || view.Truncated {
		t.Errorf("closing past the body limit: truncated %v, body %q", view.Truncated, view.Body)
	}
	instructions := filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")
	mustWriteFile(t, instructions, "---\napi_key: hunter2short\npad: "+strings.Repeat("a", sourceFrontmatterLimit+sourceBodyLimit)+"\n---\nBody.\n")
	view, raw = get(instructions)
	if strings.Contains(raw, "hunter2short") || view.Body != "" || len(view.Frontmatter) != 0 || !view.Truncated {
		t.Errorf("closing past the read limit: truncated %v, body length %d, frontmatter %d", view.Truncated, len(view.Body), len(view.Frontmatter))
	}
}

func TestServeSource_ShowsAHookAsRedactedFieldsAndNoRawText(t *testing.T) {
	dir, srv := sourceProject(t)
	hook := filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml")
	mustWriteFile(t, hook, "name: guard\non: before-tool\nmatch: shell\ncommand: API_TOKEN=sk-abcdefghijklmnop ./guard.sh\ntimeout: 10\nenv:\n  password: hunter2short\n")
	resp, err := http.Get(srv.URL + sourceViewPath + "?path=" + url.QueryEscape(absSlash(t, hook)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, data)
	}
	if strings.Contains(string(data), "sk-abcdefghijklmnop") || strings.Contains(string(data), "hunter2short") {
		t.Fatalf("leaked a secret: %s", data)
	}
	var view sourceView
	if err := json.Unmarshal(data, &view); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range view.Frontmatter {
		got = append(got, f.Key+"="+f.Value)
	}
	if view.Kind != "hook" || view.Body != "" || strings.Join(got, "|") != "name=guard|on=before-tool|match=shell|command=<redacted>|timeout=10|env.password=<redacted>" {
		t.Errorf("kind %q, body %q, fields %v", view.Kind, view.Body, got)
	}
}

func TestServeSource_HidesAHookThatDoesNotParseOrIsCut(t *testing.T) {
	dir, srv := sourceProject(t)
	broken := filepath.Join(dir, ".agnostic-ai", "hooks", "broken.yaml")
	mustWriteFile(t, broken, "name: broken\non: stop\ncommand: echo hunter2short\nbad: [\n")
	large := filepath.Join(dir, ".agnostic-ai", "hooks", "large.yaml")
	mustWriteFile(t, large, "name: large\non: stop\ncommand: echo hunter2short\npad: "+strings.Repeat("a", sourceFrontmatterLimit+sourceBodyLimit)+"\n")
	for _, path := range []string{broken, large} {
		resp, err := http.Get(srv.URL + sourceViewPath + "?path=" + url.QueryEscape(absSlash(t, path)))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if strings.Contains(string(data), "hunter2short") {
			t.Errorf("%s: status %d leaked the command: %.200s", filepath.Base(path), resp.StatusCode, data)
		}
	}
	mustWriteFile(t, broken, "name: broken\non: stop\ncommand: echo ok\n")
	view := getSource(t, srv, absSlash(t, large), http.StatusOK)
	if !view.Truncated || len(view.Frontmatter) != 0 || view.Body != "" {
		t.Errorf("cut hook: truncated %v, fields %d, body length %d", view.Truncated, len(view.Frontmatter), len(view.Body))
	}
}

func TestServeSource_ServesEveryLifecycleRowTheOverviewCanOpen(t *testing.T) {
	dir, srv := sourceProject(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\non: before-tool\nmatch: shell\ncommand: ./guard.sh\n")
	resp, err := http.Get(srv.URL + overview.OverviewPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var view overview.View
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	opened := 0
	for _, s := range view.Sections {
		if s.ID != overview.SectionLifecycle {
			continue
		}
		for _, item := range s.Items {
			if item.OpenPath == "" {
				continue
			}
			opened++
			if got := getSource(t, srv, item.OpenPath, http.StatusOK); got.Kind != "hook" {
				t.Errorf("%s: kind %q", item.OpenPath, got.Kind)
			}
		}
	}
	if opened == 0 {
		t.Error("no hook row to open")
	}
}

func TestServeSource_RefusesABuiltinHook(t *testing.T) {
	dir, srv := sourceProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nbuiltins: [memory]\n")
	_, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range b.Hooks {
		if e.Layer == "builtin" && e.Path != "" {
			found = true
			getSource(t, srv, absSlash(t, e.Path), http.StatusNotFound)
		}
	}
	if !found {
		t.Fatal("memory built-in loaded no hook")
	}
}
