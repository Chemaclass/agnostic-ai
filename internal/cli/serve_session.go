package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
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
	// sessionCodesKept caps the codes waiting at once; a new offer past it
	// ends the oldest, so offers from another program cannot pile up.
	sessionCodesKept = 8
)

// unlockFilesPrivate says whether a file mode keeps an unlock file from
// other accounts. On Windows the file would keep its folder's access
// rules, so no unlock file is written and the page stays locked.
var unlockFilesPrivate = runtime.GOOS != "windows"

// webSession is the proof a browser tab gives that the person opened it
// with view: the server draws a key, and only a page opened through the
// unlock file, which only this account can read, gets it.
type webSession struct {
	key  string
	addr string
	// dir holds the unlock files, mode 0700.
	dir string
	now func() time.Time

	mu sync.Mutex
	// pending holds the live codes, oldest first. Each works once, with
	// its own file and expiry, so a stray offer cannot end a real launch.
	pending []*pendingCode
}

type pendingCode struct {
	code, file string
	expires    time.Time
	timer      *time.Timer
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
	// Files a server on this port left when it was killed hold dead codes.
	if stale, err := filepath.Glob(filepath.Join(dir, filepath.Base(sessionFilePath(dir, addr, "*")))); err == nil {
		for _, file := range stale {
			_ = os.Remove(file)
		}
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

var unlockFileName = regexp.MustCompile(`^unlock-[0-9]+-[0-9a-f]{12}\.html$`)

// sessionFilePath names one unlock file of the server at addr.
func sessionFilePath(dir, addr, id string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "0"
	}
	return filepath.Join(dir, "unlock-"+port+"-"+id+".html")
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
// its path. The file goes away once used, or after sessionCodeLife.
func (s *webSession) offer() (string, error) {
	code, err := randomHex(32)
	if err != nil {
		return "", err
	}
	id, err := randomHex(6)
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
	path := sessionFilePath(s.dir, s.addr, id)
	target := html.EscapeString("http://" + s.addr + sessionHopPath + "#unlock=" + code)
	page := "<!doctype html>\n<meta charset=\"utf-8\">\n<meta name=\"referrer\" content=\"no-referrer\">\n" +
		"<meta http-equiv=\"refresh\" content=\"0;url=" + target + "\">\n<title>agnostic-ai</title>\n" +
		"<a href=\"" + target + "\">Open agnostic-ai</a>\n"
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := atomicfile.Replace(path, []byte(page), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	if len(s.pending) >= sessionCodesKept {
		s.drop(s.pending[0])
	}
	entry := &pendingCode{code: code, file: path, expires: s.now().Add(sessionCodeLife)}
	entry.timer = time.AfterFunc(sessionCodeLife, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.drop(entry)
	})
	s.pending = append(s.pending, entry)
	return path, nil
}

// drop ends one code and removes its file. The caller holds mu.
func (s *webSession) drop(entry *pendingCode) {
	i := slices.Index(s.pending, entry)
	if i < 0 {
		return
	}
	s.pending = slices.Delete(s.pending, i, i+1)
	entry.timer.Stop()
	_ = os.Remove(entry.file)
}

// unlock trades a live code for the key, once.
func (s *webSession) unlock(code string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, entry := range s.pending {
		if len(code) == len(entry.code) && subtle.ConstantTimeCompare([]byte(code), []byte(entry.code)) == 1 {
			s.drop(entry)
			if !now.Before(entry.expires) {
				return "", false
			}
			return s.key, true
		}
	}
	return "", false
}

// close removes the unlock files still waiting.
func (s *webSession) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.pending) > 0 {
		s.drop(s.pending[0])
	}
}

type sessionUnlockRequest struct {
	Code string `json:"code"`
}

type sessionUnlockReply struct {
	Key string `json:"key"`
}

type sessionOfferReply struct {
	File string `json:"file"`
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
		file, err := h.session.offer()
		if err != nil {
			writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
			return
		}
		writeEditJSON(w, http.StatusOK, sessionOfferReply{File: file})
	default:
		http.NotFound(w, r)
	}
}

// unlockHopScript forwards a well-formed code to the app and nothing else.
const unlockHopScript = `var w=navigator.serviceWorker;if(w&&w.controller){w.getRegistrations().then(function(r){r.forEach(function(x){x.unregister()})});document.body.textContent="A service worker controlled this address, so the page stays locked. It is removed now: close this tab and run agnostic-ai view --scope user again."}else if(/^#unlock=[0-9a-f]{64}$/.test(location.hash))location.replace("/"+location.hash);else document.body.textContent="Run agnostic-ai view --scope user to open agnostic-ai."`

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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New(resp.Status)
	}
	var reply sessionOfferReply
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&reply); err != nil || reply.File == "" {
		return "", errors.New("the server sent no unlock file")
	}
	// Another program may hold the port; open only an unlock file of ours.
	dir, err := sessionDir()
	if err != nil {
		return "", err
	}
	_, port, _ := net.SplitHostPort(addr)
	file := filepath.Clean(reply.File)
	if filepath.Dir(file) != filepath.Clean(dir) || !unlockFileName.MatchString(filepath.Base(file)) || !strings.HasPrefix(filepath.Base(file), "unlock-"+port+"-") {
		return "", errors.New("the server sent a file outside the unlock folder")
	}
	return file, nil
}
