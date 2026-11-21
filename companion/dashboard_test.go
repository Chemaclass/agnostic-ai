package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenDashboard_ReportsStartupFailureAndAllowsRetry(t *testing.T) {
	fakeCommand(t, "echo 'could not read the configuration' >&2; exit 1")
	view := func() *exec.Cmd { return exec.Command(os.Getenv("AGNOSTIC_AI_BIN"), "view", "--scope", "user") }
	opened := ""
	open := func(url string) error { opened = url; return nil }
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := openDashboard(ctx, view(), open); err == nil || !strings.Contains(err.Error(), "could not read the configuration") || opened != "" {
		t.Fatalf("startup failure = %v; opened %q", err, opened)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"global":true,"ui":true}`)
	}))
	defer srv.Close()
	fakeCommand(t, "echo '"+srv.URL+"/'")
	if err := openDashboard(ctx, view(), open); err != nil || opened != srv.URL+"/" {
		t.Errorf("retry = %v; opened %q", err, opened)
	}
	fakeCommand(t, "echo 'Using agnostic-ai at "+srv.URL+"/'")
	if err := openDashboard(ctx, view(), func(string) error { t.Error("opened a second browser for the older CLI"); return nil }); err != nil {
		t.Errorf("older CLI failed: %v", err)
	}
}

func TestOpenDashboard_LeavesAnUnrelatedServiceRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"ok":true,"global":false,"ui":true}`)
	}))
	defer srv.Close()
	fakeCommand(t, "echo '"+srv.URL+"/'")
	view := exec.Command(os.Getenv("AGNOSTIC_AI_BIN"))
	if err := openDashboard(t.Context(), view, func(string) error { t.Error("opened an unrelated service"); return nil }); err == nil {
		t.Error("accepted a different scope")
	}
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestOpenDashboard_TimesOutWhenTheCommandNeverAnswers(t *testing.T) {
	fakeCommand(t, "exec sleep 30")
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err := openDashboard(ctx, exec.Command(os.Getenv("AGNOSTIC_AI_BIN")), func(string) error { t.Error("opened before readiness"); return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("startup error = %v, want a timeout", err)
	}
}

func TestFindBinary_CLIManagedLaunchUsesRecordedCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("AGNOSTIC_AI_BIN", "")
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(dir, "agnostic-ai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(home, "previously-installed-command")
	if err := os.WriteFile(command, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, commandPathFile), []byte(command), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := findBinary(); err != nil || got != command {
		t.Errorf("command = %q, %v; want %q", got, err, command)
	}
	fakeBundledCommand(t)
	if got, err := findBinary(); err != nil || got == command {
		t.Errorf("packaged launch used the recorded command: %q, %v", got, err)
	}
}
