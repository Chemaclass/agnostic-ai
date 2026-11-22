package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
	"github.com/chemaclass/agnostic-ai/internal/authority"
)

const (
	// sessionHopPath is where the unlock file sends the browser.
	sessionHopPath    = "/unlock"
	sessionUnlockPath = "/api/session/unlock"
	sessionOfferPath  = "/api/session/offer"
	// sessionHeader carries the web session key on loosening settings
	// writes and on a sync.
	sessionHeader = "X-Agnostic-Ai-Session"
	// sessionCodeLife is how long an unlock file and its code work.
	sessionCodeLife = 60 * time.Second
)

// webSession is the proof a browser tab gives that the person opened it
// with view: the server draws a key, and only a page opened through the
// unlock file, which only this account can read, gets it.
type webSession struct {
	key  string
	addr string
	// dir holds the unlock files, mode 0700.
	dir string
	now func() time.Time

	mu      sync.Mutex
	code    string
	expires time.Time
	timer   *time.Timer
}

// newWebSession draws a key for the server at addr. Nothing is written
// until offer.
func newWebSession(addr string) (*webSession, error) {
	dir, err := sessionDir()
	if err != nil {
		return nil, err
	}
	key, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	return &webSession{key: key, addr: addr, dir: dir, now: time.Now}, nil
}

// sessionDir is the folder of the unlock files, under the user config
// folder the companion uses too.
func sessionDir() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the user config folder: %w", err)
	}
	return filepath.Join(config, "agnostic-ai", "unlock"), nil
}

// sessionFilePath is the unlock file of the server at addr, so a later
// view finds it after asking that server for a new one.
func sessionFilePath(dir, addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "0"
	}
	return filepath.Join(dir, "unlock-"+port+".html")
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("draw a random key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// allows reports whether r carries the session key.
func (s *webSession) allows(r *http.Request) bool {
	if s == nil {
		return false
	}
	got := r.Header.Get(sessionHeader)
	return len(got) == len(s.key) && subtle.ConstantTimeCompare([]byte(got), []byte(s.key)) == 1
}

// offer writes a new unlock file with a fresh one-time code and returns
// its path. A code it replaces stops working. The file goes away once
// used, or after sessionCodeLife.
func (s *webSession) offer() (string, error) {
	code, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("%s: %w", s.dir, err)
	}
	// A folder made earlier with a looser mode is tightened.
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("%s: %w", s.dir, err)
	}
	path := sessionFilePath(s.dir, s.addr)
	target := html.EscapeString("http://" + s.addr + sessionHopPath + "#unlock=" + code)
	page := "<!doctype html>\n<meta charset=\"utf-8\">\n<meta name=\"referrer\" content=\"no-referrer\">\n" +
		"<meta http-equiv=\"refresh\" content=\"0;url=" + target + "\">\n<title>agnostic-ai</title>\n" +
		"<a href=\"" + target + "\">Open agnostic-ai</a>\n"
	s.mu.Lock()
	defer s.mu.Unlock()
	// Replace, not Write: a leftover file keeps no looser mode and no link.
	if err := atomicfile.Replace(path, []byte(page), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	s.code, s.expires = code, s.now().Add(sessionCodeLife)
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(sessionCodeLife, func() { s.expire(code) })
	return path, nil
}

// expire ends code and removes its file, unless a newer offer replaced it.
func (s *webSession) expire(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code != code {
		return
	}
	s.code = ""
	_ = os.Remove(sessionFilePath(s.dir, s.addr))
}

// unlock trades a live code for the key, once.
func (s *webSession) unlock(code string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	live := s.code != "" && s.now().Before(s.expires)
	if !live || len(code) != len(s.code) || subtle.ConstantTimeCompare([]byte(code), []byte(s.code)) != 1 {
		return "", false
	}
	s.code = ""
	_ = os.Remove(sessionFilePath(s.dir, s.addr))
	return s.key, true
}

// close removes an unlock file that is still waiting.
func (s *webSession) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = ""
	if s.timer != nil {
		s.timer.Stop()
	}
	_ = os.Remove(sessionFilePath(s.dir, s.addr))
}

type sessionUnlockRequest struct {
	Code string `json:"code"`
}

type sessionUnlockReply struct {
	Key string `json:"key"`
}

// sessionHandler serves the unlock exchange and the offer of a new
// unlock file. A project or API-only server has no session and answers
// 404.
type sessionHandler struct {
	session *webSession
}

func (h sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.session == nil {
		http.NotFound(w, r)
		return
	}
	// Only the server's own origin, not one --allow-origin added.
	if status, message := editRequestGuard(r.WithContext(context.Background())); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case sessionUnlockPath:
		var req sessionUnlockRequest
		if err := decodeOneJSON(w, r, 1<<10, &req); err != nil || req.Code == "" {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: `send {"code"} from the unlock link`})
			return
		}
		key, ok := h.session.unlock(req.Code)
		if !ok {
			writeEditJSON(w, http.StatusForbidden, editError{Error: "This unlock link was used or has expired. Run agnostic-ai view --scope user again."})
			return
		}
		writeEditJSON(w, http.StatusOK, sessionUnlockReply{Key: key})
	case sessionOfferPath:
		if _, err := h.session.offer(); err != nil {
			writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// unlockHopScript forwards a well-formed code to the app and nothing else.
const unlockHopScript = `if(/^#unlock=[0-9a-f]{64}$/.test(location.hash))location.replace("/"+location.hash);else document.body.textContent="Run agnostic-ai view --scope user to open agnostic-ai."`

var unlockHopPage = []byte("<!doctype html>\n<meta charset=\"utf-8\">\n<title>agnostic-ai</title>\n<body>\n<script>" + unlockHopScript + "</script>\n")

var unlockHopPolicy = func() string {
	sum := sha256.Sum256([]byte(unlockHopScript))
	return "default-src 'none'; script-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; frame-ancestors 'none'"
}()

// unlockHop serves the page the unlock file opens. A navigation from a
// file is cross-site, so the origin guard would refuse the app itself;
// this page holds no data and moves to the app from its own origin.
func unlockHop(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != sessionHopPath || r.Method != http.MethodGet || !authority.LoopbackHost(r.Host) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", unlockHopPolicy)
		_, _ = w.Write(unlockHopPage)
	})
}

// offerSession asks the view server at addr for a new unlock file and
// returns its path. An older server without sessions returns an error.
func offerSession(ctx context.Context, addr string) (string, error) {
	dir, err := sessionDir()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, viewProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+sessionOfferPath, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return "", errors.New(resp.Status)
	}
	return sessionFilePath(dir, addr), nil
}
