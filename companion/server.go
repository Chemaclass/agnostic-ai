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
	"os"
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

// maxLateReads is how many list reads in a row may go unanswered in
// time before the server is asked whether it still runs.
const maxLateReads = 3

// healthTimeout bounds that question. The health endpoint takes no
// scan lock, so a server busy with a long scan still answers it, and
// only a stuck one, as after a sleep, is replaced.
const healthTimeout = 5 * time.Second

// stuckScanAfter is how long the server's running scan may last before
// a server that still answers its health is replaced too: each scan
// waits for the one before, so one that never ends holds every list.
const stuckScanAfter = 2 * time.Minute

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
	// address, and healthWait the question whether a server that missed
	// reads still runs; tests shorten them.
	startWait, healthWait time.Duration
	// openLateWait bounds an open the page stopped waiting for.
	openLateWait time.Duration

	mu sync.Mutex
	// resting is set once the companion quits: no call starts a server
	// then. It is outside mu so a quit never waits for a server that is
	// still starting.
	resting atomic.Bool
	// base is the running server's address, such as
	// http://127.0.0.1:52000; empty while none runs.
	base    string
	health  serverHealth
	process *exec.Cmd
	// input is the server's standard input. The server stops when it closes.
	input io.Closer
	// failedStarts counts the starts that failed in a row; no start
	// begins before retryAt while the binary is still failedBinary, and
	// startProblem says why the last failed.
	failedStarts int
	retryAt      time.Time
	failedBinary string
	startProblem error
	// lateReads counts the list reads in a row the running server did
	// not answer in time.
	lateReads int
}

var errResting = errors.New("the companion is quitting and asks agnostic-ai nothing")

var errMismatchedAgnosticAI = errors.New("The installed agnostic-ai does not match this companion. Run: agnostic-ai upgrade, then agnostic-ai companion install") //nolint:staticcheck // shown to the user as a full sentence; server_test.go pins the text

func newServer() *server {
	// Each call sets its own time limit; see callWithin.
	return &server{client: &http.Client{}, callWait: callTimeout, openWait: openTimeout, sendWait: sendTimeout, openLateWait: openLateTimeout, startWait: serverStartWait, healthWait: healthTimeout}
}

// address returns the running server's address, starting one first when
// none runs. After starts that failed in a row it waits longer before
// the next, and answers why the last one failed meanwhile, unless the
// binary changed since, such as after an upgrade.
func (s *server) address() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resting.Load() {
		return "", errResting
	}
	if s.base != "" {
		return s.base, nil
	}
	binary := binaryStamp()
	if time.Now().Before(s.retryAt) && binary == s.failedBinary {
		return "", s.startProblem
	}
	base, err := s.start()
	switch {
	case errors.Is(err, errResting):
	case err != nil:
		s.failedStarts++
		s.retryAt = time.Now().Add(startBackoff(s.failedStarts))
		s.failedBinary, s.startProblem = binary, err
	default:
		s.failedStarts, s.retryAt, s.startProblem = 0, time.Time{}, nil
	}
	return base, err
}

// binaryStamp names the agnostic-ai the companion would start, by its
// path and its last change; empty when none is found.
func binaryStamp() string {
	binary, err := findBinary()
	if err != nil {
		return ""
	}
	info, err := os.Stat(binary)
	if err != nil {
		return binary
	}
	return binary + "@" + info.ModTime().String()
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
	read := make(chan struct{})
	go func() {
		defer close(read)
		lines := bufio.NewScanner(output)
		for lines.Scan() {
			if address, ok := strings.CutPrefix(lines.Text(), servedLinePrefix); ok {
				select {
				case served <- strings.TrimSuffix(strings.TrimSpace(address), "/"):
				default:
				}
			}
		}
		close(served)
	}()
	ended := make(chan struct{})
	go func() {
		// Wait closes stdout, so it waits for the reader: a server that
		// says its address and ends at once would otherwise lose the line.
		<-read
		// Collect the process when it ends, and forget its address then.
		_ = process.Wait()
		close(ended)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.process == process {
			s.base, s.health, s.process, s.input = "", serverHealth{}, nil, nil
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
				return "", errMismatchedAgnosticAI
			}
			return "", fmt.Errorf("agnostic-ai serve stopped before it listened: %s", said)
		}
		if s.resting.Load() {
			// The companion quit while this server started.
			_ = input.Close()
			return "", errResting
		}
		s.base, s.process, s.input, s.lateReads = address, process, input, 0
	case <-time.After(s.startWait):
		_ = input.Close()
		_ = process.Process.Kill()
		return "", fmt.Errorf("agnostic-ai serve did not listen within %s", s.startWait)
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.callWait)
	defer cancel()
	s.health, _ = readServerHealth(ctx, s.client, s.base)
	return s.base, nil
}

type serverHealth struct {
	OK, Global, UI bool
	Root, Version  string
}

func readServerHealth(ctx context.Context, client *http.Client, base string) (serverHealth, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return serverHealth{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return serverHealth{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return serverHealth{}, fmt.Errorf("dashboard health: %s", response.Status)
	}
	var health serverHealth
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<10)).Decode(&health); err != nil {
		return serverHealth{}, err
	}
	return health, nil
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
	s.base, s.health, s.process, s.input = "", serverHealth{}, nil, nil
}

func (s *server) runningHealth(ctx context.Context) serverHealth {
	s.mu.Lock()
	health, base, process := s.health, s.base, s.process
	s.mu.Unlock()
	if health != (serverHealth{}) || base == "" {
		return health
	}
	limited, cancel := context.WithTimeout(ctx, s.healthWait)
	defer cancel()
	health, err := readServerHealth(limited, s.client, base)
	if err != nil {
		return serverHealth{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base == base && s.process == process {
		s.health = health
	}
	return s.health
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

// get sends one GET request to the server. ifNoneMatch is the tag of the
// list the caller already holds, or empty.
func (s *server) get(ctx context.Context, path, ifNoneMatch string) (reply, error) {
	return s.callWithin(ctx, s.callWait, http.MethodGet, path, nil, ifNoneMatch)
}

// callWithin sends one request and waits up to wait for the whole answer.
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
	listRead := method == http.MethodGet && strings.HasPrefix(path, serverListPath)
	response, err := s.client.Do(request)
	if err != nil {
		return reply{}, s.unanswered(base, listRead, ctx, limited, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReplyBytes+1))
	if err != nil {
		return reply{}, s.unanswered(base, listRead, ctx, limited, path, err)
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
		s.lateReads = 0
	}
}

// unanswered is unanswered for a call to the server at base. After
// maxLateReads list reads in a row got no answer in time, it asks the
// server for its health, and stops it when that gets no answer either,
// so the next call starts another.
func (s *server) unanswered(base string, listRead bool, caller, limited context.Context, path string, cause error) error {
	err := unanswered(caller, limited, path, cause)
	if !listRead || !timedOut(err) {
		return err
	}
	s.mu.Lock()
	if s.base != base {
		s.mu.Unlock()
		return err
	}
	s.lateReads++
	stuck := s.lateReads >= maxLateReads
	s.mu.Unlock()
	if !stuck {
		return err
	}
	healthy := s.servesLists(base)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base != base {
		return err
	}
	if healthy {
		s.lateReads = 0
		return err
	}
	log.Printf("agnostic-ai serve answered none of %d list reads in a row, and its health did not answer or named a scan running over %s; starting another", maxLateReads, stuckScanAfter)
	if s.input != nil {
		_ = s.input.Close()
	}
	if s.process != nil && s.process.Process != nil {
		_ = s.process.Process.Kill()
	}
	s.base, s.health, s.process, s.input, s.lateReads = "", serverHealth{}, nil, nil, 0
	return err
}

// servesLists reports whether the server at base answers its health
// within healthWait, with no scan running past stuckScanAfter.
func (s *server) servesLists(base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), s.healthWait)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return false
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		ScanningSince time.Time `json:"scanningSince"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<10)).Decode(&health); err != nil {
		return true
	}
	return health.ScanningSince.IsZero() || time.Since(health.ScanningSince) < stuckScanAfter
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
		if request, ok := errors.AsType[*url.Error](cause); ok {
			said = request.Err
		}
		if dial, ok := errors.AsType[*net.OpError](said); ok {
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
