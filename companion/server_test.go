package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// countedCommand is fakeCommand that notes each start in the returned file.
func countedCommand(t *testing.T, script string) string {
	t.Helper()
	starts := filepath.Join(t.TempDir(), "starts")
	fakeCommand(t, "echo started >> '"+starts+"'\n"+script)
	return starts
}

func startsIn(path string) int {
	raw, _ := os.ReadFile(path)
	return strings.Count(string(raw), "started")
}

func TestServer_KillsAServerThatNeverSaysItsAddress(t *testing.T) {
	countedCommand(t, "exec sleep 30")
	s := newServer()
	s.startWait = 100 * time.Millisecond

	_, err := s.address()

	if err == nil || err.Error() != "agnostic-ai serve did not listen within 100ms" {
		t.Errorf("address = %v", err)
	}
}

func TestServer_StartsAnotherAfterTheRunningOneEnded(t *testing.T) {
	starts := countedCommand(t, "echo 'Serving agnostic-ai at http://127.0.0.1:1/'")
	s := newServer()

	first, err := s.address()
	if err != nil || first != "http://127.0.0.1:1" {
		t.Fatalf("address = %q, %v", first, err)
	}
	waitFor(t, "the ended server to be forgotten", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.base == ""
	})
	if _, err := s.address(); err != nil {
		t.Fatal(err)
	}

	if got := startsIn(starts); got != 2 {
		t.Errorf("started %d servers; want a second once the first ended", got)
	}
}

func TestServer_StopsAServerWhoseAddressArrivesAfterQuit(t *testing.T) {
	dir := t.TempDir()
	stopped := filepath.Join(dir, "stopped")
	starts := countedCommand(t, "sleep 0.3\necho 'Serving agnostic-ai at http://127.0.0.1:1/'\nread line\necho stopped > '"+stopped+"'")
	s := newServer()
	answered := make(chan error, 1)

	go func() {
		_, err := s.address()
		answered <- err
	}()
	waitFor(t, "the server to start", func() bool { return startsIn(starts) == 1 })
	s.resting.Store(true)

	if err := <-answered; !errors.Is(err, errResting) {
		t.Errorf("address = %v; want errResting", err)
	}
	waitFor(t, "the late server to stop", func() bool {
		_, err := os.Stat(stopped)
		return err == nil
	})
}

func TestServer_SaysTheInstalledAgnosticAIDoesNotMatchAndWaitsBeforeTryingAgain(t *testing.T) {
	starts := countedCommand(t, "echo 'Error: unknown flag: --cloud-sessions' >&2\nexit 1")
	s := newServer()

	for range 3 {
		if _, err := s.address(); !errors.Is(err, errMismatchedAgnosticAI) {
			t.Fatalf("address = %v; want errMismatchedAgnosticAI", err)
		}
	}

	if got := startsIn(starts); got != 2 {
		t.Errorf("started %d servers; want a pause after the second failed start", got)
	}
	if errMismatchedAgnosticAI.Error() != "The installed agnostic-ai does not match this companion. Run: agnostic-ai upgrade, then agnostic-ai companion install" {
		t.Errorf("message = %q", errMismatchedAgnosticAI)
	}
}

func TestServer_StartsAtOnceWhenTheBinaryChangedDuringAPause(t *testing.T) {
	starts := countedCommand(t, "echo 'Error: unknown flag: --cloud-sessions' >&2\nexit 1")
	s := newServer()
	for range 2 {
		_, _ = s.address()
	}
	binary := os.Getenv("AGNOSTIC_AI_BIN")
	upgraded := "#!/bin/sh\necho started >> '" + starts + "'\necho 'Serving agnostic-ai at http://127.0.0.1:1/'\ncat > /dev/null\n"
	if err := os.WriteFile(binary, []byte(upgraded), 0o700); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(binary, later, later); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.rest)

	address, err := s.address()

	if err != nil || address != "http://127.0.0.1:1" {
		t.Errorf("address after an upgrade = %q, %v; want the new server", address, err)
	}
}

func TestServer_SaysAMissingBinaryUntilItIsInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in command is a shell script")
	}
	missing := filepath.Join(t.TempDir(), "agnostic-ai")
	t.Setenv("AGNOSTIC_AI_BIN", missing)
	s := newServer()
	for range 2 {
		if _, err := s.address(); err == nil {
			t.Fatal("a missing binary started")
		}
	}
	if err := os.WriteFile(missing, []byte("#!/bin/sh\necho 'Serving agnostic-ai at http://127.0.0.1:1/'\ncat > /dev/null\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.rest)

	if address, err := s.address(); err != nil || address != "http://127.0.0.1:1" {
		t.Errorf("address once installed = %q, %v; want the new server", address, err)
	}
}

func TestStartBackoff_DoublesUpToAMinute(t *testing.T) {
	for failed, want := range map[int]time.Duration{1: 0, 2: pollEvery, 3: 2 * pollEvery, 4: 4 * pollEvery, 5: 8 * pollEvery, 6: maxStartBackoff, 20: maxStartBackoff} {
		if got := startBackoff(failed); got != want {
			t.Errorf("startBackoff(%d) = %s; want %s", failed, got, want)
		}
	}
}

// stalledLists is an agnostic-ai whose list reads never answer; its
// health answers while healthy is set.
func stalledLists(healthy bool) *fakeAgnosticAI {
	return &fakeAgnosticAI{hold: make(chan struct{}), list: func(w http.ResponseWriter, r *http.Request) {
		if healthy && r.URL.Path == "/api/health" {
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
			return
		}
		<-r.Context().Done()
	}}
}

func lateServer(t *testing.T, fake *fakeAgnosticAI) (*server, *closer) {
	t.Helper()
	in, _ := connected(t, fake)
	s := in.server
	s.callWait, s.healthWait = 20*time.Millisecond, time.Second
	input := &closer{}
	s.input = input
	return s, input
}

func TestServer_ReplacesAServerThatAnswersNeitherReadsNorHealth(t *testing.T) {
	s, input := lateServer(t, stalledLists(false))

	for i := range maxLateReads {
		if _, err := s.get(context.Background(), serverListPath, ""); !timedOut(err) {
			t.Fatalf("read %d = %v; want a timeout", i+1, err)
		}
		if i < maxLateReads-1 && s.base == "" {
			t.Fatalf("the server was stopped after %d late reads", i+1)
		}
	}

	if s.base != "" || !input.closed {
		t.Errorf("base %q, input closed %v; want the stuck server stopped", s.base, input.closed)
	}
}

func TestServer_KeepsAServerBusyWithALongScan(t *testing.T) {
	s, input := lateServer(t, stalledLists(true))
	base := s.base

	for range 2 * maxLateReads {
		_, _ = s.get(context.Background(), serverListPath+"?q=parser", "")
	}

	if s.base != base || input.closed {
		t.Errorf("base %q, input closed %v; want a server that answers its health kept", s.base, input.closed)
	}
}

// scanSince is an agnostic-ai whose list reads never answer while its
// health answers that the running scan began at since.
func scanSince(since time.Time) *fakeAgnosticAI {
	return &fakeAgnosticAI{hold: make(chan struct{}), list: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			_, _ = w.Write([]byte(`{"version":"1.2.3","scanningSince":"` + since.UTC().Format(time.RFC3339) + `"}`))
			return
		}
		<-r.Context().Done()
	}}
}

func TestServer_ReplacesAServerWhoseScanIsStuck(t *testing.T) {
	s, input := lateServer(t, scanSince(time.Now().Add(-stuckScanAfter-time.Minute)))

	for range maxLateReads {
		_, _ = s.get(context.Background(), serverListPath, "")
	}

	if s.base != "" || !input.closed {
		t.Errorf("base %q, input closed %v; want a server whose scan outlasted %s stopped", s.base, input.closed, stuckScanAfter)
	}
}

func TestServer_KeepsAServerWhoseScanIsRecent(t *testing.T) {
	s, input := lateServer(t, scanSince(time.Now().Add(-time.Minute)))
	base := s.base

	for range 2 * maxLateReads {
		_, _ = s.get(context.Background(), serverListPath, "")
	}

	if s.base != base || input.closed {
		t.Errorf("base %q, input closed %v; want a server whose scan began a minute ago kept", s.base, input.closed)
	}
}

func TestServer_CountsOnlyListReads(t *testing.T) {
	s, input := lateServer(t, stalledLists(false))
	base := s.base

	for range 2 * maxLateReads {
		_, _ = s.callWithin(context.Background(), 20*time.Millisecond, http.MethodPost, serverMarkPath, []byte(`{}`), "")
	}

	if s.base != base || input.closed {
		t.Errorf("base %q, input closed %v; want late marks to leave the server alone", s.base, input.closed)
	}
}

func TestServer_KeepsAServerThatAnswersBetweenLateReads(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	s := in.server
	base := s.base
	s.lateReads = maxLateReads - 1

	if _, err := s.get(context.Background(), serverListPath, ""); err != nil {
		t.Fatal(err)
	}

	if s.base != base || s.lateReads != 0 {
		t.Errorf("base %q, late reads %d; want the server kept and the count reset", s.base, s.lateReads)
	}
}

func TestServer_HandsItsSecretOverStdinAndSendsItOnEachCall(t *testing.T) {
	got := make(chan string, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/conversations" {
			got <- r.Header.Get(secretHeader)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(api.Close)
	seen := filepath.Join(t.TempDir(), "seen")
	fakeCommand(t, `echo "$@" > "`+seen+`.args"
env > "`+seen+`.env"
read -r line
echo "$line" > "`+seen+`.stdin"
echo "Serving agnostic-ai at `+api.URL+`/"
cat > /dev/null`)
	s := newServer()
	t.Cleanup(s.rest)

	if _, err := s.get(context.Background(), "/api/conversations", ""); err != nil {
		t.Fatal(err)
	}

	if len(s.secret) != 64 {
		t.Fatalf("secret has %d characters, want 64", len(s.secret))
	}
	if header := <-got; header != s.secret {
		t.Errorf("the call carried %q, want the launch secret", header)
	}
	stdin, _ := os.ReadFile(seen + ".stdin")
	env, _ := os.ReadFile(seen + ".env")
	args, _ := os.ReadFile(seen + ".args")
	if strings.TrimSpace(string(stdin)) != s.secret {
		t.Errorf("the first stdin line = %q, want the launch secret", stdin)
	}
	if strings.Contains(string(env), s.secret) || strings.Contains(string(args), s.secret) || !strings.Contains(string(args), secretFlag) {
		t.Errorf("the server got env %q and args %q; want the secret on stdin only, asked for with %s", env, args, secretFlag)
	}
}

func TestServer_WithoutASecretAsksForNone(t *testing.T) {
	seen := filepath.Join(t.TempDir(), "seen")
	fakeCommand(t, `echo "$@" > "`+seen+`.args"
echo "Serving agnostic-ai at http://127.0.0.1:1/"
cat > "`+seen+`.stdin"`)
	s := newServer()
	s.secret = ""

	if _, err := s.address(); err != nil {
		t.Fatal(err)
	}
	s.rest()

	waitFor(t, "the server to read its input to the end", func() bool {
		_, err := os.Stat(seen + ".stdin")
		return err == nil
	})
	args, _ := os.ReadFile(seen + ".args")
	if strings.Contains(string(args), secretFlag) {
		t.Errorf("started with %q; want no %s without a secret", args, secretFlag)
	}
	if stdin, _ := os.ReadFile(seen + ".stdin"); len(stdin) != 0 {
		t.Errorf("stdin = %q; want nothing written", stdin)
	}
}
