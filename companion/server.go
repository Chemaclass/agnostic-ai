package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
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

// openTimeout bounds an open, which takes longer than any other call:
// the server may list Claude Code's background sessions, wait for an
// editor window and run a terminal script, each for up to 15 seconds.
const openTimeout = 60 * time.Second

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
	// callWait and openWait bound a call and an open; tests shorten them.
	callWait, openWait time.Duration

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
	// Each call sets its own time limit; see callWithin.
	return &server{client: &http.Client{}, callWait: callTimeout, openWait: openTimeout}
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
	ctx, cancel := context.WithTimeout(context.Background(), s.callWait)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return ""
	}
	response, err := s.client.Do(request)
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

// callError is a call the server did not answer. Error is the sentence
// the person reads; cause keeps what the system said, for the log.
type callError struct {
	message string
	// timedOut: no answer came in time; the server may still be working on it.
	timedOut bool
	cause    error
}

func (e *callError) Error() string { return e.message }

func (e *callError) Unwrap() error { return e.cause }

// timedOut reports whether err is a call the server did not answer in
// time, which says nothing about whether it did what was asked.
func timedOut(err error) bool {
	var failed *callError
	return errors.As(err, &failed) && failed.timedOut
}

// call sends one request to the server. body is JSON or nil; ifNoneMatch
// is the tag of the list the caller already holds, or empty.
func (s *server) call(ctx context.Context, method, path string, body []byte, ifNoneMatch string) (reply, error) {
	return s.callWithin(ctx, s.callWait, method, path, body, ifNoneMatch)
}

// callWithin is call that waits up to wait for the whole answer.
func (s *server) callWithin(ctx context.Context, wait time.Duration, method, path string, body []byte, ifNoneMatch string) (reply, error) {
	base, err := s.address()
	if err != nil {
		return reply{}, err
	}
	limited, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	request, err := http.NewRequestWithContext(limited, method, base+path, bytes.NewReader(body))
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
		return reply{}, unanswered(ctx, limited, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReplyBytes+1))
	if err != nil {
		return reply{}, unanswered(ctx, limited, path, err)
	}
	if len(raw) > maxReplyBytes {
		return reply{}, errors.New("the answer of agnostic-ai is larger than the companion reads")
	}
	return reply{status: response.StatusCode, tag: response.Header.Get("ETag"), body: raw}, nil
}

// unanswered words a call that got no whole answer for the person, and
// logs what the system said. A caller that gave the call up, such as a
// search newer typing replaced, is not logged.
func unanswered(caller, limited context.Context, path string, cause error) error {
	late := caller.Err() == nil && errors.Is(limited.Err(), context.DeadlineExceeded)
	if caller.Err() == nil {
		// A failed request and its dial name the address, and a search's
		// holds the words typed: the log gets the path alone.
		said := cause
		var request *url.Error
		if errors.As(cause, &request) {
			said = request.Err
		}
		var dial *net.OpError
		if errors.As(said, &dial) {
			said = dial.Err
		}
		endpoint, _, _ := strings.Cut(path, "?")
		log.Printf("ask agnostic-ai %s: %v", endpoint, said)
	}
	if late {
		return &callError{message: "agnostic-ai took too long to answer. Try again.", timedOut: true, cause: cause}
	}
	return &callError{message: "The companion could not reach agnostic-ai. Try again.", cause: cause}
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
