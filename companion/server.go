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

// maxStartBackoff bounds the pause between starts after failed ones. A
// failed start waits twice as long as the one before, from pollEvery.
const maxStartBackoff = time.Minute

// maxLateCalls is how many calls in a row may go unanswered in time
// before the server is taken for stuck, as after a sleep, and replaced.
const maxLateCalls = 3

// callTimeout bounds one call to the server. A first scan of a large
// history takes seconds.
const callTimeout = 30 * time.Second

// openTimeout bounds an open, which takes longer than any other call:
// the server may list Claude Code's background sessions, wait for an
// editor window and run a terminal script, each for up to 15 seconds.
const openTimeout = 60 * time.Second

// sendTimeout bounds a send: the server may wait for a scan of the whole
// history to find the chat, then bring it forward and type.
const sendTimeout = 60 * time.Second

// openLateTimeout is how long an open keeps being waited for after the
// page got its pending reply. It outlasts the 90 seconds agnostic-ai
// serve gives an open to answer, so a late answer is still read.
const openLateTimeout = 95 * time.Second

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
	// callWait, openWait and sendWait bound a call, an open and a send;
	// tests shorten them.
	callWait, openWait, sendWait time.Duration
	// startWait bounds how long a started server may take to print its
	// address; tests shorten it.
	startWait time.Duration
	// openLateWait bounds an open the page stopped waiting for.
	openLateWait time.Duration

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
	// failedStarts counts the starts that failed in a row; no start
	// begins before retryAt, and startProblem says why the last failed.
	failedStarts int
	retryAt      time.Time
	startProblem error
	// lateCalls counts the calls in a row the running server did not answer in time.
	lateCalls int
}

var errResting = errors.New("the companion is quitting and asks agnostic-ai nothing")

var errOlderAgnosticAI = errors.New("The installed agnostic-ai is older than this companion. Run: agnostic-ai upgrade")

func newServer() *server {
	// Each call sets its own time limit; see callWithin.
	return &server{client: &http.Client{}, callWait: callTimeout, openWait: openTimeout, sendWait: sendTimeout, openLateWait: openLateTimeout, startWait: serverStartWait}
}

// address returns the running server's address, starting one first when
// none runs. After starts that failed in a row it waits longer before
// the next, and answers why the last one failed meanwhile.
func (s *server) address() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resting.Load() {
		return "", errResting
	}
	if s.base != "" {
		return s.base, nil
	}
	if time.Now().Before(s.retryAt) {
		return "", s.startProblem
	}
	base, err := s.start()
	switch {
	case errors.Is(err, errResting):
	case err != nil:
		s.failedStarts++
		s.retryAt = time.Now().Add(startBackoff(s.failedStarts))
		s.startProblem = err
	default:
		s.failedStarts, s.retryAt, s.startProblem = 0, time.Time{}, nil
	}
	return base, err
}

// startBackoff is the pause after failed starts in a row: none after
// the first, then pollEvery doubled each time, up to maxStartBackoff.
func startBackoff(failed int) time.Duration {
	if failed < 2 {
		return 0
	}
	wait := pollEvery
	for range failed - 2 {
		wait *= 2
		if wait >= maxStartBackoff {
			return maxStartBackoff
		}
	}
	return wait
}

// start starts a server and waits for its address. The caller holds mu.
func (s *server) start() (string, error) {
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
			case <-time.After(s.startWait):
			}
			said := problems.String()
			if strings.Contains(said, "unknown flag") {
				log.Printf("agnostic-ai serve stopped before it listened: %s", said)
				return "", errOlderAgnosticAI
			}
			return "", fmt.Errorf("agnostic-ai serve stopped before it listened: %s", said)
		}
		if s.resting.Load() {
			// The companion quit while this server started.
			_ = input.Close()
			return "", errResting
		}
		s.base, s.process, s.input, s.lateCalls = address, process, input, 0
	case <-time.After(s.startWait):
		_ = input.Close()
		_ = process.Process.Kill()
		return "", fmt.Errorf("agnostic-ai serve did not listen within %s", s.startWait)
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
		return reply{}, s.unanswered(base, ctx, limited, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReplyBytes+1))
	if err != nil {
		return reply{}, s.unanswered(base, ctx, limited, path, err)
	}
	s.answered(base)
	if len(raw) > maxReplyBytes {
		return reply{}, errors.New("the answer of agnostic-ai is larger than the companion reads")
	}
	return reply{status: response.StatusCode, tag: response.Header.Get("ETag"), body: raw}, nil
}

// answered notes that the server at base answered a call.
func (s *server) answered(base string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base == base {
		s.lateCalls = 0
	}
}

// unanswered is unanswered for a call to the server at base. After
// maxLateCalls in a row that got no answer in time, it stops that
// server, so the next call starts another.
func (s *server) unanswered(base string, caller, limited context.Context, path string, cause error) error {
	err := unanswered(caller, limited, path, cause)
	if !timedOut(err) {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base != base {
		return err
	}
	s.lateCalls++
	if s.lateCalls < maxLateCalls {
		return err
	}
	log.Printf("agnostic-ai serve answered none of %d calls in a row in time; starting another", maxLateCalls)
	if s.input != nil {
		_ = s.input.Close()
	}
	if s.process != nil && s.process.Process != nil {
		_ = s.process.Process.Kill()
	}
	s.base, s.version, s.process, s.input, s.lateCalls = "", "", nil, nil, 0
	return err
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
