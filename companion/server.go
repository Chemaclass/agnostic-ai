package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// servedLinePrefix starts the line `agnostic-ai serve` prints with its address.
const servedLinePrefix = "Serving agnostic-ai at "

// serverStartWait is how long a started server may take to print its address.
const serverStartWait = 15 * time.Second

// callTimeout bounds one call to the server. A first scan of a large
// history takes seconds.
const callTimeout = 30 * time.Second

// maxReplyBytes bounds a reply from the server: a list of conversations.
const maxReplyBytes = 32 << 20

// maxProblemBytes is how much of the server's error output is kept to explain a failed start.
const maxProblemBytes = 2 << 10

// server is the companion's own `agnostic-ai serve --global`: every
// conversation on this machine, read and marked by agnostic-ai itself.
// The companion starts it when it needs a list and stops it when it
// quits.
type server struct {
	client *http.Client

	mu sync.Mutex
	// resting is set once the companion quits: no call starts a server
	// then. It is outside mu so a quit never waits for a server that is
	// still starting.
	resting atomic.Bool
	// base is the running server's address, such as
	// http://127.0.0.1:52000; empty while none runs.
	base string
	// version is the agnostic-ai release the running server reports.
	version string
	process *exec.Cmd
	// input is the server's standard input. The server stops when it closes.
	input io.Closer
}

var errResting = errors.New("the companion is quitting and asks agnostic-ai nothing")

func newServer() *server {
	return &server{client: &http.Client{Timeout: callTimeout}}
}

// address returns the running server's address, starting one first when none runs.
func (s *server) address() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resting.Load() {
		return "", errResting
	}
	if s.base != "" {
		return s.base, nil
	}
	binary, err := findBinary()
	if err != nil {
		return "", err
	}
	// Port 0 lets the system pick a free port, away from the ports
	// `agnostic-ai view` uses. --stop-with-stdin ends the server with
	// this process, however this process ends. --cloud-sessions adds
	// the Claude Code sessions that run at claude.ai/code.
	process := exec.Command(binary, "serve", "--global", "--api-only", "--no-installed", "--addr", "127.0.0.1:0", "--stop-with-stdin", "--cloud-sessions")
	withoutConsoleWindow(process)
	input, err := process.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("start agnostic-ai serve: %w", err)
	}
	output, err := process.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("start agnostic-ai serve: %w", err)
	}
	problems := &tail{limit: maxProblemBytes}
	process.Stderr = problems
	if err := process.Start(); err != nil {
		return "", fmt.Errorf("start agnostic-ai serve: %w", err)
	}

	served := make(chan string, 1)
	go func() {
		lines := bufio.NewScanner(output)
		for lines.Scan() {
			if address, ok := strings.CutPrefix(lines.Text(), servedLinePrefix); ok {
				served <- strings.TrimSuffix(strings.TrimSpace(address), "/")
			}
		}
		close(served)
	}()
	ended := make(chan struct{})
	go func() {
		// Collect the process when it ends, and forget its address then.
		_ = process.Wait()
		close(ended)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.process == process {
			s.base, s.version, s.process, s.input = "", "", nil, nil
		}
	}()

	select {
	case address, ok := <-served:
		if !ok || address == "" {
			_ = input.Close()
			// Wait copies the last of stderr into problems, so read it only after the process ends.
			select {
			case <-ended:
			case <-time.After(serverStartWait):
			}
			return "", fmt.Errorf("agnostic-ai serve stopped before it listened: %s", problems.String())
		}
		if s.resting.Load() {
			// The companion quit while this server started.
			_ = input.Close()
			return "", errResting
		}
		s.base, s.process, s.input = address, process, input
	case <-time.After(serverStartWait):
		_ = input.Close()
		_ = process.Process.Kill()
		return "", fmt.Errorf("agnostic-ai serve did not listen within %s", serverStartWait)
	}
	s.version = s.readVersion(s.base)
	return s.base, nil
}

// readVersion asks the server which agnostic-ai release it is; empty when it does not say.
func (s *server) readVersion(base string) string {
	response, err := s.client.Get(base + "/api/health")
	if err != nil {
		return ""
	}
	defer func() { _ = response.Body.Close() }()
	var health struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<10)).Decode(&health); err != nil {
		return ""
	}
	return health.Version
}

// rest ends the server this companion started, if one runs, and lets no
// call start another.
func (s *server) rest() {
	s.resting.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.input != nil {
		_ = s.input.Close()
	}
	s.base, s.version, s.process, s.input = "", "", nil, nil
}

// runningVersion is the release of the server that runs now; empty when none runs.
func (s *server) runningVersion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// reply is one answer from the server.
type reply struct {
	status int
	// tag is the list's ETag; empty on other answers.
	tag  string
	body []byte
}

// call sends one request to the server. body is JSON or nil; ifNoneMatch
// is the tag of the list the caller already holds, or empty.
func (s *server) call(ctx context.Context, method, path string, body []byte, ifNoneMatch string) (reply, error) {
	base, err := s.address()
	if err != nil {
		return reply{}, err
	}
	request, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return reply{}, fmt.Errorf("ask agnostic-ai: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if ifNoneMatch != "" {
		request.Header.Set("If-None-Match", ifNoneMatch)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return reply{}, fmt.Errorf("ask agnostic-ai: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReplyBytes+1))
	if err != nil {
		return reply{}, fmt.Errorf("read the answer of agnostic-ai: %w", err)
	}
	if len(raw) > maxReplyBytes {
		return reply{}, errors.New("the answer of agnostic-ai is larger than the companion reads")
	}
	return reply{status: response.StatusCode, tag: response.Header.Get("ETag"), body: raw}, nil
}

// tail keeps the last limit bytes written to it.
type tail struct {
	limit int

	mu   sync.Mutex
	kept []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.kept = append(t.kept, p...)
	if len(t.kept) > t.limit {
		t.kept = t.kept[len(t.kept)-t.limit:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.kept))
}
