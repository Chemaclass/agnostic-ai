package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func postCreate(t *testing.T, h http.Handler, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4310"+globalCreatePath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSwitch_OffersToCreateAMissingGlobalHomeAndStartsNoServer(t *testing.T) {
	home := filepath.Join(t.TempDir(), "missing")
	started := false
	h := newSwitchHandler(&switchPeer{root: home, global: true, dir: t.TempDir()}, freeLoopbackAddrs(t, 1), probeViewServer, func(switchPeer, string) (*peerProcess, error) {
		started = true
		return nil, nil
	})
	defer h.close()
	rec, _ := postSwitch(t, h)
	var body switchMissingReply
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || !body.Missing || body.Root != home || body.Error == "" {
		t.Errorf("switch = %d %s", rec.Code, rec.Body)
	}
	if started {
		t.Error("a server started for a missing home")
	}
}

func TestSwitch_EmptyGlobalHomeCountsAsMissing(t *testing.T) {
	h := newSwitchHandler(&switchPeer{root: t.TempDir(), global: true}, nil, probeViewServer, nil)
	defer h.close()
	if rec, _ := postSwitch(t, h); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"missing":true`) {
		t.Errorf("switch = %d %s", rec.Code, rec.Body)
	}
}

func TestSwitch_DanglingGlobalHomeLinkIsAPlainErrorNamingTheTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone"), link); err != nil {
		t.Fatal(err)
	}
	h := newSwitchHandler(&switchPeer{root: link, global: true}, nil, probeViewServer, nil)
	defer h.close()
	rec, _ := postSwitch(t, h)
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), `"missing"`) || !strings.Contains(rec.Body.String(), "gone") {
		t.Errorf("switch = %d %s", rec.Code, rec.Body)
	}
}

func TestGlobalCreate_WritesTheStarterFilesAndRunsGitInit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := filepath.Join(t.TempDir(), "home")
	rec := postCreate(t, newGlobalCreateHandler(root), `{"git":true}`, nil)
	var reply globalCreateReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if reply.Root != root || reply.Git != "initialized" || len(reply.Created) != 3 {
		t.Errorf("reply = %+v", reply)
	}
	if got := readText(t, filepath.Join(root, "agnostic-ai.yaml")); got != "version: 1\n" {
		t.Errorf("agnostic-ai.yaml = %q", got)
	}
	if got := readText(t, filepath.Join(root, ".gitignore")); got != "/local/\n" {
		t.Errorf(".gitignore = %q", got)
	}
	if got := readText(t, filepath.Join(root, "AGNOSTIC_AI.md")); !strings.HasPrefix(got, "# ") {
		t.Errorf("AGNOSTIC_AI.md = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Errorf("no .git: %v", err)
	}
}

func TestGlobalCreate_SkipsGitWhenNotRequestedOrMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	rec := postCreate(t, newGlobalCreateHandler(root), `{"git":false}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"git":"skipped`) {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		t.Error("git init ran without being asked")
	}

	t.Setenv("PATH", "")
	root = filepath.Join(t.TempDir(), "home")
	rec = postCreate(t, newGlobalCreateHandler(root), `{"git":true}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "skipped: git not found") {
		t.Fatalf("create without git = %d %s", rec.Code, rec.Body)
	}
	if got := readText(t, filepath.Join(root, ".gitignore")); got != "/local/\n" {
		t.Errorf(".gitignore = %q", got)
	}
}

func TestGlobalCreate_ExistingSetupIsLeftUntouched(t *testing.T) {
	root := t.TempDir()
	sentinel := filepath.Join(root, "AGNOSTIC_AI.md")
	mustWrite(t, sentinel, "mine\n")
	rec := postCreate(t, newGlobalCreateHandler(root), `{"git":true}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if got := readText(t, sentinel); got != "mine\n" {
		t.Errorf("sentinel = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "agnostic-ai.yaml")); err == nil {
		t.Error("create wrote into an existing setup")
	}
}

func TestGlobalCreate_WorksInAnEmptyExistingFolder(t *testing.T) {
	root := t.TempDir()
	if rec := postCreate(t, newGlobalCreateHandler(root), `{"git":false}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if got := readText(t, filepath.Join(root, "agnostic-ai.yaml")); got != "version: 1\n" {
		t.Errorf("agnostic-ai.yaml = %q", got)
	}
}

func TestGlobalCreate_RefusesADanglingLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	target := filepath.Join(t.TempDir(), "gone")
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if rec := postCreate(t, newGlobalCreateHandler(link), `{"git":false}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("create wrote through the dangling link")
	}
}

func TestGlobalCreate_RefusesBadRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	h := newGlobalCreateHandler(root)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+globalCreatePath, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", rec.Code)
	}
	if rec := postCreate(t, h, `{"git":true}`, func(r *http.Request) { r.Header.Set("Origin", "https://example.com") }); rec.Code != http.StatusForbidden {
		t.Errorf("foreign origin = %d", rec.Code)
	}
	if rec := postCreate(t, h, `{"git":true}`, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("wrong type = %d", rec.Code)
	}
	if rec := postCreate(t, h, `{"git":true,"root":"/tmp/elsewhere"}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("root field = %d", rec.Code)
	}
	if _, err := os.Stat(root); err == nil {
		t.Error("a refused request created the folder")
	}
}

func TestServeMux_GlobalCreateOnlyOnAProjectServer(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	opts := authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}
	post := func(mux http.Handler) int {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4310"+globalCreatePath, strings.NewReader(`{"git":false}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	home := filepath.Join(t.TempDir(), "home")
	project := newSwitchHandler(&switchPeer{root: home, global: true}, nil, probeViewServer, nil)
	defer project.close()
	global := newSwitchHandler(&switchPeer{root: src.root}, nil, probeViewServer, nil)
	defer global.close()
	if code := post(newServeMuxWithSwitch(src.load, opts, true, global)); code == http.StatusOK {
		t.Error("global server mounts the create route")
	}
	if _, err := os.Stat(home); err == nil {
		t.Error("the global server created the folder")
	}
	if code := post(newServeMuxWithSwitch(src.load, opts, true, project)); code != http.StatusOK {
		t.Errorf("project server create route = %d", code)
	}
	if _, err := os.Stat(filepath.Join(home, "agnostic-ai.yaml")); err != nil {
		t.Errorf("project server did not create the setup: %v", err)
	}
}
