package integration

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func writeLifecycleNodeLauncher(t *testing.T, launcher, native string) {
	t.Helper()
	encoded, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, launcher, `const fs = require('node:fs');
function record(phase, fields = {}) {
  const file = process.env.AGNOSTIC_LIFECYCLE_LAUNCH_DIAGNOSTIC;
  if (!file) return;
  try {
    fs.appendFileSync(file, JSON.stringify({ phase, at: new Date().toISOString(), pid: process.pid, ...fields }) + '\n');
  } catch (error) {
    console.error('Cannot save launcher diagnostic:', error.code);
  }
}
record('node-entry');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const streams = [0, 1, 2].map(fd => {
  try {
    const stat = fs.fstatSync(fd);
    return { fd, character: stat.isCharacterDevice(), pipe: stat.isFIFO(), file: stat.isFile() };
  } catch (error) {
    return { fd, error: error.code };
  }
});
record('native-launch', { versionProbe: process.argv.slice(2).includes('--version'), streams });
const result = spawnSync(path.resolve(__dirname, `+string(encoded)+`), process.argv.slice(2), { stdio: 'inherit' });
record('native-return', {
  status: result.status, signal: result.signal,
  error: result.error && { code: result.error.code, errno: result.error.errno },
});
if (result.error) throw result.error;
process.exit(result.status === null ? 1 : result.status);
`)
}

func TestProjectLifecycle_LaunchDiagnosticSurvivesNativeTimeout(t *testing.T) {
	skipFlakyWindowsStartup(t)
	node := lifecycleDiagnosticNode(t)
	native, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	launcher, receipt := filepath.Join(dir, "launcher.js"), filepath.Join(dir, "launcher.jsonl")
	started, finished := filepath.Join(dir, "native-started"), filepath.Join(dir, "native-finished")
	release := filepath.Join(dir, "native-release")
	writeLifecycleNodeLauncher(t, launcher, native)
	t.Setenv("AGNOSTIC_LIFECYCLE_LAUNCH_DIAGNOSTIC", receipt)
	t.Setenv("AGNOSTIC_LIFECYCLE_DIAGNOSTIC_STARTED", started)
	t.Setenv("AGNOSTIC_LIFECYCLE_DIAGNOSTIC_FINISHED", finished)
	t.Setenv("AGNOSTIC_LIFECYCLE_DIAGNOSTIC_RELEASE", release)
	t.Cleanup(func() { mustWrite(t, release, "release\n") })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, launcher, "-test.run=^TestProjectLifecycleDiagnosticNativeProcess$")
	cmd.WaitDelay = 250 * time.Millisecond
	var output lifecycleProbeOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()
	waitLifecycleDiagnosticFile(t, started)
	cancel()
	if err := cmd.Wait(); err == nil {
		t.Fatal("delayed native process completed before launcher cancellation")
	}
	mustWrite(t, release, "release\n")
	if runtime.GOOS != "windows" {
		waitLifecycleDiagnosticFile(t, finished)
	}
	assertLifecycleDiagnosticPhases(t, receipt, "node-entry", "native-launch")
}

func TestProjectLifecycle_LaunchDiagnosticRecordsNativeReturn(t *testing.T) {
	skipFlakyWindowsStartup(t)
	node := lifecycleDiagnosticNode(t)
	native, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	launcher, receipt := filepath.Join(dir, "launcher.js"), filepath.Join(dir, "launcher.jsonl")
	writeLifecycleNodeLauncher(t, launcher, native)
	t.Setenv("AGNOSTIC_LIFECYCLE_LAUNCH_DIAGNOSTIC", receipt)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, launcher, "-test.run=^$")
	cmd.WaitDelay = 250 * time.Millisecond
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch native: %v\n%s", err, output)
	}
	assertLifecycleDiagnosticPhases(t, receipt, "node-entry", "native-launch", "native-return")
}

func lifecycleDiagnosticNode(t *testing.T) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("CI must provide Node for launcher diagnostics: %v", err)
		}
		t.Skipf("launcher diagnostics require Node: %v", err)
	}
	return node
}

func TestProjectLifecycleDiagnosticNativeProcess(t *testing.T) {
	started := os.Getenv("AGNOSTIC_LIFECYCLE_DIAGNOSTIC_STARTED")
	if started == "" {
		return
	}
	mustWrite(t, started, "started\n")
	waitLifecycleDiagnosticFile(t, os.Getenv("AGNOSTIC_LIFECYCLE_DIAGNOSTIC_RELEASE"))
	mustWrite(t, os.Getenv("AGNOSTIC_LIFECYCLE_DIAGNOSTIC_FINISHED"), "finished\n")
}

func waitLifecycleDiagnosticFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("native diagnostic not written: %s", path)
}

func assertLifecycleDiagnosticPhases(t *testing.T, path string, want ...string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	for _, phase := range want {
		var receipt struct {
			Phase string
			At    time.Time
			PID   int
		}
		if err := decoder.Decode(&receipt); err != nil {
			t.Fatalf("read %s receipt: %v", phase, err)
		}
		if receipt.Phase != phase || receipt.At.IsZero() || receipt.PID <= 0 {
			t.Errorf("launcher receipt = %+v, want phase %q with timestamp and PID", receipt, phase)
		}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Errorf("launcher recorded extra data: %v, receipt: %v", err, extra)
	}
}
