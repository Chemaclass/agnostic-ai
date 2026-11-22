package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const sessionTestAddr = "127.0.0.1:4310"

func sessionFixture(t *testing.T) *webSession {
	t.Helper()
	userAgentTestHome(t)
	s, err := newWebSession(sessionTestAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	return s
}

var unlockTarget = regexp.MustCompile(`url=(http://[^/"]+)/unlock#unlock=`)

// openedPage is the address a browser lands on: target itself, or the
// one an unlock file sends it to, without the code.
func openedPage(target string) string {
	if strings.HasPrefix(target, "http") {
		return target
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return target
	}
	if match := unlockTarget.FindSubmatch(data); match != nil {
		return string(match[1]) + "/"
	}
	return target
}

var unlockCode = regexp.MustCompile(`#unlock=([0-9a-f]+)`)

func offeredCode(t *testing.T, s *webSession) (string, string) {
	t.Helper()
	path, err := s.offer()
	if err != nil {
		t.Fatal(err)
	}
	match := unlockCode.FindStringSubmatch(readFile(t, path))
	if match == nil {
		t.Fatalf("no unlock code in %s:\n%s", path, readFile(t, path))
	}
	return path, match[1]
}

func TestWebSession_UnlockFileIsPrivateAndPointsAtTheServer(t *testing.T) {
	s := sessionFixture(t)
	path, _ := offeredCode(t, s)
	if !strings.Contains(readFile(t, path), "http://"+sessionTestAddr+"/unlock#unlock=") {
		t.Errorf("unlock file does not open the server:\n%s", readFile(t, path))
	}
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("unlock file mode = %v, %v; want 0600", info.Mode().Perm(), err)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Errorf("unlock folder mode = %v, %v; want 0700", dir.Mode().Perm(), err)
	}
}

func TestWebSession_CodeWorksOnceAndRemovesTheFile(t *testing.T) {
	s := sessionFixture(t)
	path, code := offeredCode(t, s)
	key, ok := s.unlock(code)
	if !ok || key != s.key {
		t.Fatalf("unlock = %q, %v", key, ok)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("unlock file still there: %v", err)
	}
	if _, ok := s.unlock(code); ok {
		t.Error("a used code unlocked again")
	}
}

func TestWebSession_CodeExpiresAndANewOfferReplacesIt(t *testing.T) {
	s := sessionFixture(t)
	_, old := offeredCode(t, s)
	_, fresh := offeredCode(t, s)
	if _, ok := s.unlock(old); ok {
		t.Error("a replaced code unlocked")
	}
	s.now = func() time.Time { return time.Now().Add(sessionCodeLife + time.Second) }
	if _, ok := s.unlock(fresh); ok {
		t.Error("an expired code unlocked")
	}
	if _, ok := s.unlock(""); ok {
		t.Error("an empty code unlocked")
	}
}

func TestWebSession_AllowsOnlyItsKey(t *testing.T) {
	s := sessionFixture(t)
	req, _ := http.NewRequest(http.MethodPost, "http://"+sessionTestAddr+settingsWritePath, nil)
	if s.allows(req) {
		t.Error("no key allowed")
	}
	req.Header.Set(sessionHeader, strings.Repeat("0", len(s.key)))
	if s.allows(req) {
		t.Error("a wrong key allowed")
	}
	req.Header.Set(sessionHeader, s.key)
	if !s.allows(req) {
		t.Error("the key was refused")
	}
	var none *webSession
	if none.allows(req) {
		t.Error("a server without a session allowed a key")
	}
}

func TestSessionHandler_UnlockAndOfferRoutes(t *testing.T) {
	s := sessionFixture(t)
	h := sessionHandler{session: s}
	_, code := offeredCode(t, s)

	if rec := postEdit(t, h, sessionUnlockPath, `{"code":"`+code+`"}`, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin unlock = %d %s", rec.Code, rec.Body)
	}
	rec := postEdit(t, h, sessionUnlockPath, `{"code":"`+code+`"}`, nil)
	var reply sessionUnlockReply
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &reply) != nil || reply.Key != s.key {
		t.Fatalf("unlock = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if rec := postEdit(t, h, sessionUnlockPath, `{"code":"`+code+`"}`, nil); rec.Code != http.StatusForbidden {
		t.Errorf("second unlock = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, sessionOfferPath, `{}`, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("offer = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(sessionFilePath(s.dir, s.addr)); err != nil {
		t.Errorf("offer wrote no file: %v", err)
	}
	if rec := postEdit(t, sessionHandler{}, sessionOfferPath, `{}`, nil); rec.Code != http.StatusNotFound {
		t.Errorf("offer without a session = %d", rec.Code)
	}
}

func TestSettings_SessionKeyIsProof(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\ntargets: [claude]\n")
	s, err := newWebSession(sessionTestAddr)
	if err != nil {
		t.Fatal(err)
	}
	h.session = s
	withKey := func(r *http.Request) { r.Header.Set(sessionHeader, s.key) }

	if rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"yolo"}`, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("yolo without the key = %d %s", rec.Code, rec.Body)
	}
	rec, reply := postSetting(t, h, `{"key":"approvals.mode","value":"yolo","expected":"ask"}`, withKey)
	if rec.Code != http.StatusOK || !reply.Settings.CanLoosen {
		t.Fatalf("yolo with the key = %d %s", rec.Code, rec.Body)
	}
	entries, err := approvals.ReadLog(approvals.LogPath(source))
	if err != nil || len(entries) != 1 || entries[0].By != settingsByWeb {
		t.Errorf("decision log = %+v, %v", entries, err)
	}
	if rec := postEdit(t, h, settingsSyncPath, `{"action":"plan"}`, nil); rec.Code != http.StatusForbidden {
		t.Errorf("sync without the key = %d %s", rec.Code, rec.Body)
	}
	_, doc := getSettings(t, h, "")
	if doc.CanLoosen || doc.Approvals.Mode != config.ApprovalsYolo {
		t.Errorf("document without the key = %+v", doc)
	}
}

func TestUnlockHop_ServesAPageWithoutDataToAFileNavigation(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := unlockHop(authority.CORS(inner))
	navigate := func(path, host string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := navigate(sessionHopPath, sessionTestAddr)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "location.replace") {
		t.Fatalf("hop = %d %s", rec.Code, rec.Body)
	}
	if policy := rec.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "script-src 'sha256-") || strings.Contains(policy, "unsafe-inline") {
		t.Errorf("hop policy = %q", policy)
	}
	if rec := navigate("/", sessionTestAddr); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site app navigation = %d; the guard must still refuse it", rec.Code)
	}
	if rec := navigate(sessionHopPath, "evil.example:4310"); rec.Code == http.StatusOK {
		t.Errorf("hop on a foreign host = %d", rec.Code)
	}
}
