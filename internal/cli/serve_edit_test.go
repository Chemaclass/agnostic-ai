package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func editHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	source := filepath.Join(home, "source")
	t.Setenv("AGNOSTIC_AI_HOME", source)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "# Global targets.\ntargets:\n  - claude\n")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "native.yaml"), "# Native keys.\nx-claude:\n  verbose: false\n")
	return source
}

func postEdit(t *testing.T, h http.Handler, path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4310"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEditHandler_SwitchesABuiltinAndKeepsComments(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	config := filepath.Join(source, "agnostic-ai.yaml")

	if rec := postEdit(t, h, editBuiltinPath, `{"audience":"shared","name":"memory","enabled":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), "# Global targets.") || !strings.Contains(string(data), "builtins:\n  - memory") {
		t.Errorf("after enabling:\n%s", data)
	}
	if rec := postEdit(t, h, editBuiltinPath, `{"audience":"shared","name":"memory","enabled":false}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d %s", rec.Code, rec.Body)
	}
	data, _ = os.ReadFile(config)
	if strings.Contains(string(data), "builtins") || !strings.Contains(string(data), "- claude") {
		t.Errorf("after disabling, want the emptied list gone and targets kept:\n%s", data)
	}
	if rec := postEdit(t, h, editBuiltinPath, `{"audience":"shared","name":"not-a-builtin","enabled":true}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown built-in = %d, want 400", rec.Code)
	}
}

func TestEditHandler_FlipsABooleanInASettingsSpec(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	if rec := postEdit(t, h, editValuePath, `{"audience":"shared","path":"settings/native.yaml","key":"x-claude.verbose","value":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("flip = %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(filepath.Join(source, "settings", "native.yaml"))
	if !strings.Contains(string(data), "verbose: true") || !strings.Contains(string(data), "# Native keys.") {
		t.Errorf("after flipping:\n%s", data)
	}
	if rec := postEdit(t, h, editValuePath, `{"audience":"shared","path":"../outside.yaml","key":"a","value":true}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a path outside the settings specs = %d, want 400", rec.Code)
	}
}

func TestEditHandler_RefusesRequestsAnotherSiteCouldSend(t *testing.T) {
	source := editHome(t)
	h := newEditHandler(source, true)
	body := `{"audience":"shared","name":"memory","enabled":true}`
	cases := map[string]struct {
		mutate func(*http.Request)
		want   int
	}{
		"form post":    {func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		"foreign host": {func(r *http.Request) { r.Host = "attacker.example:4310" }, http.StatusMisdirectedRequest},
		"foreign page": {func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") }, http.StatusForbidden},
		"get":          {func(r *http.Request) { r.Method = http.MethodGet }, http.StatusMethodNotAllowed},
	}
	for name, c := range cases {
		if rec := postEdit(t, h, editBuiltinPath, body, c.mutate); rec.Code != c.want {
			t.Errorf("%s = %d, want %d", name, rec.Code, c.want)
		}
	}
	data, _ := os.ReadFile(filepath.Join(source, "agnostic-ai.yaml"))
	if strings.Contains(string(data), "memory") {
		t.Errorf("a refused request edited the config:\n%s", data)
	}
	if rec := postEdit(t, h, editBuiltinPath, body, func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:4310") }); rec.Code != http.StatusOK {
		t.Errorf("a same-origin page = %d, want 200", rec.Code)
	}
}

func TestEditHandler_RestoresAFileThatNoLongerLoads(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := &editHandler{root: source, global: true}
	file := filepath.Join(source, "settings", "native.yaml")
	original, _ := os.ReadFile(file)
	problems, err := h.applyValidated(file, func([]byte) ([]byte, error) { return []byte("x-claude: [unclosed\n"), nil })
	if err == nil || len(problems) == 0 {
		t.Fatalf("err = %v, problems = %v, want a refused edit", err, problems)
	}
	if now, _ := os.ReadFile(file); string(now) != string(original) {
		t.Errorf("file not restored:\n%s", now)
	}
}

func TestSetBool_RefusesAFieldThatIsNotABoolean(t *testing.T) {
	if _, err := setBool([]byte("a:\n  b: text\n"), "a.b", true); err == nil || !strings.Contains(err.Error(), "not a boolean") {
		t.Errorf("err = %v", err)
	}
	if _, err := setBool([]byte("a: true\n"), "a.missing", true); err == nil {
		t.Error("a missing key was accepted")
	}
}
