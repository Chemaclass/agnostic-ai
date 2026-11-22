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
	"runtime"
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
		switch r.URL.Path {
		case "/api/health":
			_, _ = fmt.Fprint(w, `{"ok":true,"global":true,"ui":true}`)
		case "/api/session/offer":
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	fakeCommand(t, "echo '"+srv.URL+"/'")
	if err := openDashboard(ctx, view(), open); err != nil || opened != srv.URL+"/" {
		t.Errorf("retry = %v; opened %q", err, opened)
	}
	for _, line := range []string{"Using agnostic-ai at ", "Serving agnostic-ai at "} {
		fakeCommand(t, "echo '"+line+srv.URL+"/'")
		if err := openDashboard(ctx, view(), func(string) error { t.Error("opened the dashboard of an older CLI"); return nil }); !errors.Is(err, errMismatchedAgnosticAI) {
			t.Errorf("older CLI printing %q: err = %v; want errMismatchedAgnosticAI", line, err)
		}
	}
}

func TestMatchesCompanion_RejectsAnAgnosticAIWithoutTheSecretFlag(t *testing.T) {
	fakeCommand(t, "echo 'Error: unknown flag: --require-companion-secret' >&2; exit 1")
	if err := matchesCompanion(t.Context(), os.Getenv("AGNOSTIC_AI_BIN")); !errors.Is(err, errMismatchedAgnosticAI) {
		t.Errorf("older CLI: err = %v; want errMismatchedAgnosticAI", err)
	}
	fakeCommand(t, `[ -z "$AGNOSTIC_AI_COMPANION_SECRET" ] || { echo 'unknown flag: the probe passed on a secret' >&2; exit 1; }
echo 'Error: --require-companion-secret needs a secret of at least 32 characters in AGNOSTIC_AI_COMPANION_SECRET' >&2; exit 1`)
	t.Setenv(secretEnv, "inherited")
	if err := matchesCompanion(t.Context(), os.Getenv("AGNOSTIC_AI_BIN")); err != nil {
		t.Errorf("matching CLI: err = %v", err)
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

// TestOpenDashboard_OpensTheUnlockFileTheServerOffers: the page the
// companion opens gets the session key. A missing file, one others can
// read, a link, or a file in a linked folder outside the unlock folder
// opens the plain address instead.
func TestOpenDashboard_OpensTheUnlockFileTheServerOffers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unlock files on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	offered := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			_, _ = fmt.Fprint(w, `{"ok":true,"global":true,"ui":true}`)
		case "/api/session/offer":
			_, _ = fmt.Fprintf(w, `{"file":%q}`, offered)
		}
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	folder := filepath.Join(config, "agnostic-ai", "unlock")
	outside := t.TempDir()
	place := func(dir, id string, mode os.FileMode) string {
		t.Helper()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "unlock-"+port+"-"+id+".html")
		if err := os.WriteFile(path, []byte("<!doctype html>"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ours := place(folder, "0123456789ab", 0o600)
	resolved, err := filepath.EvalSymlinks(ours)
	if err != nil {
		t.Fatal(err)
	}
	readable := place(folder, "aaaaaaaaaaaa", 0o644)
	linked := filepath.Join(folder, "unlock-"+port+"-bbbbbbbbbbbb.html")
	if err := os.Symlink(place(outside, "cccccccccccc", 0o600), linked); err != nil {
		t.Fatal(err)
	}
	linkedFolder := filepath.Join(config, "agnostic-ai", "linked")
	if err := os.Symlink(outside, linkedFolder); err != nil {
		t.Fatal(err)
	}
	locked := srv.URL + "/"
	for _, c := range []struct{ file, want string }{
		{ours, ours},
		{resolved, resolved},
		{"/Users/Shared/x.command", locked},
		{filepath.Join(folder, "unlock-1-0123456789ab.html"), locked},
		{filepath.Join(folder, "unlock-"+port+"-dddddddddddd.html"), locked},
		{readable, locked},
		{linked, locked},
		{filepath.Join(linkedFolder, "unlock-"+port+"-cccccccccccc.html"), locked},
	} {
		offered = c.file
		opened := ""
		fakeCommand(t, "echo '"+srv.URL+"/'")
		if err := openDashboard(t.Context(), exec.Command(os.Getenv("AGNOSTIC_AI_BIN")), func(target string) error { opened = target; return nil }); err != nil || opened != c.want {
			t.Errorf("offer %q: opened %q, %v; want %q", c.file, opened, err, c.want)
		}
	}
}

func TestDashboard_DoesNotRunViewWithAnOlderAgnosticAI(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "ran")
	fakeCommand(t, `echo "$1" >> "`+ran+`"
[ "$1" = view ] && exit 0
echo 'Error: unknown flag: --require-companion-secret' >&2; exit 1`)
	rec := httptest.NewRecorder()

	(&shell{}).dashboard(rec, httptest.NewRequest(http.MethodPost, "/api/dashboard", nil))

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "does not match this companion") {
		t.Errorf("dashboard = %d %s; want the mismatch", rec.Code, rec.Body.String())
	}
	if got, _ := os.ReadFile(ran); strings.TrimSpace(string(got)) != "serve" {
		t.Errorf("ran %q; want only the serve probe", got)
	}
}
