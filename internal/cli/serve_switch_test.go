package cli

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func fakePeer(t *testing.T, addr string, identity viewServer) *peerProcess {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == authority.HealthPath {
			_ = json.NewEncoder(w).Encode(identity)
		}
	})}
	done := make(chan struct{})
	go func() { _ = srv.Serve(ln); close(done) }()
	var once sync.Once
	stop := func() { once.Do(func() { _ = srv.Close() }) }
	t.Cleanup(stop)
	return &peerProcess{done: done, stop: stop}
}

type startedPeer struct {
	addr  string
	other switchPeer
}

func postSwitch(t *testing.T, h http.Handler) (*httptest.ResponseRecorder, switchReply) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, switchPath, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var reply switchReply
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
			t.Fatalf("reply %s: %v", rec.Body, err)
		}
	}
	return rec, reply
}

func TestSwitch_StartsTheGlobalServerOnTheFirstFreePort(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	ports := freeLoopbackAddrs(t, 2)
	var started []startedPeer
	h := newSwitchHandler(&switchPeer{root: home, global: true, dir: project}, ports, probeViewServer, func(other switchPeer, addr string) (*peerProcess, error) {
		started = append(started, startedPeer{addr, other})
		return fakePeer(t, addr, viewServer{OK: true, Root: home, Global: true, UI: true, ProjectRoot: project}), nil
	})
	defer h.close()

	rec, reply := postSwitch(t, h)
	if rec.Code != http.StatusOK || !reply.Started || !reply.Global || reply.URL != "http://"+ports[0]+"/" {
		t.Fatalf("switch = %d %s", rec.Code, rec.Body)
	}
	if len(started) != 1 || started[0].addr != ports[0] || !started[0].other.global || started[0].other.dir != project {
		t.Errorf("started = %+v", started)
	}

	rec, reply = postSwitch(t, h)
	if rec.Code != http.StatusOK || reply.Started || reply.URL != "http://"+ports[0]+"/" || len(started) != 1 {
		t.Errorf("second switch = %d %s, started %d servers", rec.Code, rec.Body, len(started))
	}
}

func TestSwitch_ReusesARunningServerForTheOtherConfiguration(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	ports := freeLoopbackAddrs(t, 3)
	fakePeer(t, ports[0], viewServer{OK: true, Root: t.TempDir(), UI: true})
	fakePeer(t, ports[1], viewServer{OK: true, Root: home, Global: true, ProjectRoot: project})
	fakePeer(t, ports[2], viewServer{OK: true, Root: home, Global: true, UI: true, ProjectRoot: project})
	h := newSwitchHandler(&switchPeer{root: home, global: true, dir: project}, ports, probeViewServer, func(switchPeer, string) (*peerProcess, error) {
		t.Error("started a server while one runs")
		return nil, errors.New("unexpected")
	})
	defer h.close()

	rec, reply := postSwitch(t, h)
	if rec.Code != http.StatusOK || reply.Started || reply.URL != "http://"+ports[2]+"/" {
		t.Fatalf("switch = %d %s", rec.Code, rec.Body)
	}
}

func TestSwitch_StartsItsOwnGlobalServerWhenTheRunningOneLinksToAnotherProject(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	ports := freeLoopbackAddrs(t, 3)
	fakePeer(t, ports[0], viewServer{OK: true, Root: home, Global: true, UI: true, ProjectRoot: t.TempDir()})
	fakePeer(t, ports[1], viewServer{OK: true, Root: home, Global: true, UI: true})
	var started []string
	h := newSwitchHandler(&switchPeer{root: home, global: true, dir: project}, ports, probeViewServer, func(_ switchPeer, addr string) (*peerProcess, error) {
		started = append(started, addr)
		return fakePeer(t, addr, viewServer{OK: true, Root: home, Global: true, UI: true, ProjectRoot: project}), nil
	})
	defer h.close()

	rec, reply := postSwitch(t, h)
	if rec.Code != http.StatusOK || !reply.Started || reply.URL != "http://"+ports[2]+"/" || !slices.Equal(started, ports[2:]) {
		t.Fatalf("switch = %d %s, started %v", rec.Code, rec.Body, started)
	}
}

func TestPeerServerCommand_UsesFixedArguments(t *testing.T) {
	args, dir := peerServerCommand(switchPeer{root: "/home/.agnostic-ai", global: true, dir: "/work/repo"}, "127.0.0.1:4311")
	if !slices.Equal(args, []string{"serve", "--addr", "127.0.0.1:4311", "--global"}) || dir != "/work/repo" {
		t.Errorf("global = %v in %q", args, dir)
	}
	args, dir = peerServerCommand(switchPeer{root: "/work/repo"}, "127.0.0.1:4312")
	if !slices.Equal(args, []string{"serve", "--addr", "127.0.0.1:4312", "--dir", "/work/repo"}) || dir != "" {
		t.Errorf("project = %v in %q", args, dir)
	}
}

func TestSwitch_TriesTheNextPortWhenAStartedServerExits(t *testing.T) {
	project := t.TempDir()
	ports := freeLoopbackAddrs(t, 2)
	var tried []string
	h := newSwitchHandler(&switchPeer{root: project}, ports, probeViewServer, func(_ switchPeer, addr string) (*peerProcess, error) {
		tried = append(tried, addr)
		if addr == ports[0] {
			done := make(chan struct{})
			close(done)
			return &peerProcess{done: done, stop: func() {}}, nil
		}
		return fakePeer(t, addr, viewServer{OK: true, Root: project, UI: true}), nil
	})
	defer h.close()

	rec, reply := postSwitch(t, h)
	if rec.Code != http.StatusOK || reply.URL != "http://"+ports[1]+"/" || reply.Global || !slices.Equal(tried, ports) {
		t.Fatalf("switch = %d %s, tried %v", rec.Code, rec.Body, tried)
	}
}

func TestSwitch_ReportsTheCommandWhenNoServerStarts(t *testing.T) {
	home := t.TempDir()
	ports := freeLoopbackAddrs(t, 1)
	h := newSwitchHandler(&switchPeer{root: home, global: true}, ports, probeViewServer, func(switchPeer, string) (*peerProcess, error) {
		return nil, errors.New("no executable")
	})
	defer h.close()

	rec, _ := postSwitch(t, h)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "agnostic-ai ui --global") {
		t.Errorf("switch = %d %s", rec.Code, rec.Body)
	}
}

func TestSwitch_GlobalServerWithoutAProjectHasNoWayBack(t *testing.T) {
	h := newSwitchHandler(nil, freeLoopbackAddrs(t, 1), probeViewServer, func(switchPeer, string) (*peerProcess, error) {
		t.Error("started a server with no project")
		return nil, errors.New("unexpected")
	})
	defer h.close()

	rec, _ := postSwitch(t, h)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "agnostic-ai ui") {
		t.Errorf("switch = %d %s", rec.Code, rec.Body)
	}
}

func TestSwitch_RefusesOtherMethodsAndBodyTypes(t *testing.T) {
	h := newSwitchHandler(nil, nil, probeViewServer, nil)
	defer h.close()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, switchPath, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, switchPath, strings.NewReader("a=b")))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form POST = %d", rec.Code)
	}
}

func TestSwitch_CloseStopsTheServersItStarted(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	ports := freeLoopbackAddrs(t, 1)
	var peer *peerProcess
	h := newSwitchHandler(&switchPeer{root: home, global: true, dir: project}, ports, probeViewServer, func(_ switchPeer, addr string) (*peerProcess, error) {
		peer = fakePeer(t, addr, viewServer{OK: true, Root: home, Global: true, UI: true, ProjectRoot: project})
		return peer, nil
	})
	if rec, _ := postSwitch(t, h); rec.Code != http.StatusOK {
		t.Fatalf("switch = %d %s", rec.Code, rec.Body)
	}
	h.close()
	select {
	case <-peer.done:
	case <-time.After(2 * time.Second):
		t.Error("the started server still runs after close")
	}
}

func TestServeMux_SwitchKeepsTheOriginGuard(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	h := newSwitchHandler(nil, nil, probeViewServer, nil)
	defer h.close()
	srv := httptest.NewServer(newServeMuxWithSwitch(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}, true, h))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+switchPath, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site switch = %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+switchPath, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("same-origin switch without a peer = %d", resp.StatusCode)
	}
}

func TestSwitchPeerFor_ProjectServerPointsAtTheUserConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", home)
	project := t.TempDir()
	peer := switchPeerFor(project, false)
	if peer == nil || !peer.global || !sameProjectRoot(peer.root, home) || !sameProjectRoot(peer.dir, project) {
		t.Errorf("peer = %+v", peer)
	}
}

func TestSwitchPeerFor_GlobalServerPointsBackAtTheProjectItStartedIn(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	nested := filepath.Join(project, "src")
	mustWrite(t, filepath.Join(nested, "main.go"), "package main\n")

	peer := switchPeerFor(nested, true)
	if peer == nil || peer.global || !sameProjectRoot(peer.root, project) {
		t.Errorf("peer = %+v", peer)
	}
	if got := switchPeerFor(t.TempDir(), true); got != nil {
		t.Errorf("outside a project, peer = %+v", got)
	}
}

func TestSwitch_RefusesOnceClosed(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	h := newSwitchHandler(&switchPeer{root: home, global: true, dir: project}, freeLoopbackAddrs(t, 1), probeViewServer, func(switchPeer, string) (*peerProcess, error) {
		t.Error("started a server after close")
		return nil, errors.New("unexpected")
	})
	h.close()
	if rec, _ := postSwitch(t, h); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("switch after close = %d %s", rec.Code, rec.Body)
	}
}

func TestSwitch_ReusesAGlobalServerItStartedWhoseHealthNamesNoProject(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	ports := freeLoopbackAddrs(t, 2)
	var started []string
	h := newSwitchHandler(&switchPeer{root: home, global: true, dir: project}, ports, probeViewServer, func(_ switchPeer, addr string) (*peerProcess, error) {
		started = append(started, addr)
		return fakePeer(t, addr, viewServer{OK: true, Root: home, Global: true, UI: true}), nil
	})
	defer h.close()

	for range 2 {
		if rec, reply := postSwitch(t, h); rec.Code != http.StatusOK || reply.URL != "http://"+ports[0]+"/" {
			t.Fatalf("switch = %d %s", rec.Code, rec.Body)
		}
	}
	if !slices.Equal(started, ports[:1]) {
		t.Errorf("started = %v", started)
	}
}

func TestSwitchPeerCommand_QuotesTheProjectPath(t *testing.T) {
	got := switchPeer{root: "/Users/x/My Code/it's"}.command()
	if want := `agnostic-ai ui --dir '/Users/x/My Code/it'\''s'`; got != want {
		t.Errorf("command = %s, want %s", got, want)
	}
}

func TestServeSignals_KeepsAnIgnoredHangup(t *testing.T) {
	if got := serveSignals(func(os.Signal) bool { return true }); slices.Contains(got, os.Signal(syscall.SIGHUP)) {
		t.Errorf("ignored hangup is handled again: %v", got)
	}
	if got := serveSignals(func(os.Signal) bool { return false }); !slices.Contains(got, os.Signal(syscall.SIGHUP)) || !slices.Contains(got, os.Interrupt) {
		t.Errorf("signals = %v", got)
	}
}
