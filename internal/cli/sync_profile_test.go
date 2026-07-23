package cli

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestSync_ProfileFlagWritesCPUProfile verifies --profile writes a non-empty
// runtime/pprof CPU profile for the run. runtime/pprof frames the profile as
// gzip, so a valid file decodes back to a non-empty payload with the stdlib
// alone (no external pprof parser, per the profiling scope).
func TestSync_ProfileFlagWritesCPUProfile(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	prof := filepath.Join(t.TempDir(), "cpu.prof")
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--profile", prof})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --profile: %v", err)
	}

	assertValidCPUProfile(t, prof)
}

// TestSync_ProfileEnvWritesCPUProfile verifies AGNOSTIC_AI_PROFILE is the
// env-var fallback for --profile, so profiling can be turned on without
// touching the command line.
func TestSync_ProfileEnvWritesCPUProfile(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	prof := filepath.Join(t.TempDir(), "cpu.prof")
	t.Setenv("AGNOSTIC_AI_PROFILE", prof)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync with AGNOSTIC_AI_PROFILE: %v", err)
	}

	assertValidCPUProfile(t, prof)
}

// TestSync_VerboseShowsPerTargetTiming verifies the --verbose summary appends
// per-target wall time to each target line, so a slow sync can be attributed
// to a specific adapter.
func TestSync_VerboseShowsPerTargetTiming(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	prev := logOut
	buf := &bytes.Buffer{}
	logOut = buf
	t.Cleanup(func() { logOut = prev })

	root := NewRootCmd("test")
	root.SetArgs([]string{"-v", "sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync -v: %v", err)
	}

	out := buf.String()
	if !regexp.MustCompile(`→ claude:.*\bin \d+ms`).MatchString(out) {
		t.Errorf("expected a per-target ms token in verbose output, got:\n%s", out)
	}
}

// assertValidCPUProfile fails unless path holds a non-empty CPU profile.
// runtime/pprof writes a gzip-framed protobuf, so the check reads the file,
// confirms the gzip framing, and decodes it to a non-empty payload using the
// stdlib only.
func assertValidCPUProfile(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("profile file is empty")
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("profile is not a gzip-framed pprof: %v", err)
	}
	defer func() { _ = gr.Close() }()
	payload, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("profile payload is empty")
	}
}
