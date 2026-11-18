package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
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

func TestServer_SaysAnOlderAgnosticAIIsInstalledAndWaitsBeforeTryingAgain(t *testing.T) {
	starts := countedCommand(t, "echo 'Error: unknown flag: --cloud-sessions' >&2\nexit 1")
	s := newServer()

	for range 3 {
		if _, err := s.address(); !errors.Is(err, errOlderAgnosticAI) {
			t.Fatalf("address = %v; want errOlderAgnosticAI", err)
		}
	}

	if got := startsIn(starts); got != 2 {
		t.Errorf("started %d servers; want a pause after the second failed start", got)
	}
	if errOlderAgnosticAI.Error() != "The installed agnostic-ai is older than this companion. Run: agnostic-ai upgrade" {
		t.Errorf("message = %q", errOlderAgnosticAI)
	}
}

func TestStartBackoff_DoublesUpToAMinute(t *testing.T) {
	for failed, want := range map[int]time.Duration{1: 0, 2: pollEvery, 3: 2 * pollEvery, 4: 4 * pollEvery, 5: 8 * pollEvery, 6: maxStartBackoff, 20: maxStartBackoff} {
		if got := startBackoff(failed); got != want {
			t.Errorf("startBackoff(%d) = %s; want %s", failed, got, want)
		}
	}
}

func TestServer_ReplacesAServerThatStopsAnswering(t *testing.T) {
	fake := &fakeAgnosticAI{hold: make(chan struct{})}
	in, _ := connected(t, fake)
	s := in.server
	s.callWait = 20 * time.Millisecond
	input := &closer{}
	s.input = input

	for i := range maxLateCalls {
		if _, err := s.call(context.Background(), http.MethodPost, serverMarkPath, []byte(`{}`), ""); !timedOut(err) {
			t.Fatalf("call %d = %v; want a timeout", i+1, err)
		}
		if i < maxLateCalls-1 && s.base == "" {
			t.Fatalf("the server was stopped after %d late calls", i+1)
		}
	}

	if s.base != "" || !input.closed {
		t.Errorf("base %q, input closed %v; want the stuck server stopped", s.base, input.closed)
	}
}

func TestServer_KeepsAServerThatAnswersBetweenLateCalls(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	s := in.server
	base := s.base
	s.lateCalls = maxLateCalls - 1

	if _, err := s.call(context.Background(), http.MethodGet, serverListPath, nil, ""); err != nil {
		t.Fatal(err)
	}

	if s.base != base || s.lateCalls != 0 {
		t.Errorf("base %q, late calls %d; want the server kept and the count reset", s.base, s.lateCalls)
	}
}
