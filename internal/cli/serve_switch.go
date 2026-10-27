package cli

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// switchPath finds or starts the server for the other configuration:
// the user configuration from a project, or the project a global server
// started in.
const switchPath = "/api/switch"

const (
	switchStartTimeout = 10 * time.Second
	switchPollInterval = 100 * time.Millisecond
)

// switchPeer is the configuration the page switches to. It is fixed when
// the server starts; no request names a path.
type switchPeer struct {
	root   string
	global bool
	// dir is where a started global server runs, so its health names the
	// project it came from.
	dir string
}

// peerProcess is a server this one started. done closes when it exits.
type peerProcess struct {
	done <-chan struct{}
	stop func()
}

type switchReply struct {
	URL     string `json:"url"`
	Root    string `json:"root"`
	Global  bool   `json:"global"`
	Started bool   `json:"started"`
}

type switchHandler struct {
	peer  *switchPeer
	ports []string
	probe func(string, time.Duration) *viewServer
	start func(switchPeer, string) (*peerProcess, error)

	mu      sync.Mutex
	started []*peerProcess
}

func newSwitchHandler(peer *switchPeer, ports []string, probe func(string, time.Duration) *viewServer, start func(switchPeer, string) (*peerProcess, error)) *switchHandler {
	return &switchHandler{peer: peer, ports: ports, probe: probe, start: start}
}

// switchPeerFor names the other configuration of a server reading root:
// the user configuration for a project, or for a global server the
// project that contains the directory it started in.
func switchPeerFor(root string, global bool) *switchPeer {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	if global {
		project, useGlobal, err := resolveViewSource(abs, false, false)
		if err != nil || useGlobal {
			return nil
		}
		return &switchPeer{root: project}
	}
	home, err := globalSourceRoot()
	if err != nil {
		return nil
	}
	return &switchPeer{root: home, global: true, dir: abs}
}

func (p switchPeer) command() string {
	if p.global {
		return "agnostic-ai ui --global"
	}
	return "agnostic-ai ui --dir " + p.root
}

func (h *switchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeEditJSON(w, http.StatusMethodNotAllowed, editError{Error: "use POST"})
		return
	}
	if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
		writeEditJSON(w, http.StatusUnsupportedMediaType, editError{Error: "use Content-Type application/json"})
		return
	}
	if h.peer == nil {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "this server did not start in a project; run agnostic-ai ui in the project"})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	peer := *h.peer
	reply := func(addr string, started bool) {
		writeEditJSON(w, http.StatusOK, switchReply{URL: "http://" + addr + "/", Root: peer.root, Global: peer.global, Started: started})
	}
	var free []string
	for _, addr := range h.ports {
		running := h.probe(addr, viewProbeTimeout)
		if running == nil {
			free = append(free, addr)
			continue
		}
		if h.serves(running) {
			reply(addr, false)
			return
		}
	}
	for _, addr := range free {
		process, err := h.start(peer, addr)
		if err != nil {
			writeEditJSON(w, http.StatusServiceUnavailable, editError{Error: fmt.Sprintf("could not start a server (%v); run %s", err, peer.command())})
			return
		}
		h.started = append(h.started, process)
		if h.awaitHealthy(addr, process) {
			reply(addr, true)
			return
		}
		process.stop()
	}
	writeEditJSON(w, http.StatusServiceUnavailable, editError{Error: "no free port from " + h.portRange() + " served the other configuration; run " + peer.command()})
}

// serves also requires a global server to link back to this project, so
// its page offers the right way back and it does not stop with another
// project's server.
func (h *switchHandler) serves(running *viewServer) bool {
	if !running.UI || running.Global != h.peer.global || !sameProjectRoot(running.Root, h.peer.root) {
		return false
	}
	return !h.peer.global || (running.ProjectRoot != "" && sameProjectRoot(running.ProjectRoot, h.peer.dir))
}

// awaitHealthy waits for a started server to answer, and gives up when
// it exits first, which means another program took the port.
func (h *switchHandler) awaitHealthy(addr string, process *peerProcess) bool {
	deadline := time.Now().Add(switchStartTimeout)
	for time.Now().Before(deadline) {
		if running := h.probe(addr, viewProbeTimeout); running != nil {
			return h.serves(running)
		}
		select {
		case <-process.done:
			return false
		case <-time.After(switchPollInterval):
		}
	}
	return false
}

func (h *switchHandler) portRange() string {
	if len(h.ports) == 0 {
		return "the scanned range"
	}
	return h.ports[0] + " to " + h.ports[len(h.ports)-1]
}

// close stops the servers this one started, so none outlives it.
func (h *switchHandler) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, process := range h.started {
		process.stop()
	}
	h.started = nil
}

// peerServerCommand returns the serve arguments and working directory
// for a peer. A global server runs in the project so its health names it.
func peerServerCommand(peer switchPeer, addr string) ([]string, string) {
	args := []string{"serve", "--addr", addr}
	if peer.global {
		return append(args, "--global"), peer.dir
	}
	return append(args, "--dir", peer.root), ""
}

// startPeerServer runs this binary's serve with fixed arguments. Its
// output is discarded; the health probe decides whether it started.
func startPeerServer(peer switchPeer, addr string) (*peerProcess, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find the agnostic-ai binary: %w", err)
	}
	args, dir := peerServerCommand(peer, addr)
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = peerSysProcAttr()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(exe), err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			if cmd.Process.Signal(os.Interrupt) != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
			}
		})
	}
	return &peerProcess{done: done, stop: stop}, nil
}
