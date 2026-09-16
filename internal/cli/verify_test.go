package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestVerify_PassesStructuredContextToConfiguredCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	dir := testutil.TempCwd(t)
	silence(t)

	contextPath := filepath.Join(dir, "verify-context.json")
	verifierPath := filepath.Join(dir, "verify.sh")
	writeFile(t, verifierPath, "#!/bin/sh\ncat > \"$1\"\necho verifier-stdout\necho verifier-stderr >&2\n")
	if err := os.Chmod(verifierPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf(`version: 1
targets: [codex]
verify:
  command: [%q, %q]
outputs:
  codex:
    config:
      model: gpt-test
`, verifierPath, contextPath))
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/review.md"), "---\nname: review\n---\nReview changes.\n")

	binDir := filepath.Join(dir, "bin")
	writeFile(t, filepath.Join(binDir, "codex"), "#!/bin/sh\necho codex-cli 9.9.9\n")
	if err := os.Chmod(filepath.Join(binDir, "codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"verify", "-t", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(stdout.String(), "verifier-stdout") {
		t.Errorf("verifier stdout was not preserved: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "verifier-stderr") {
		t.Errorf("verifier stderr was not preserved: %q", stderr.String())
	}

	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version            int    `json:"version"`
		Target             string `json:"target"`
		ConfiguredModel    string `json:"configured_model"`
		RuntimeModel       string `json:"runtime_model"`
		HarnessFingerprint string `json:"harness_fingerprint"`
		CLI                *struct {
			Command string `json:"command"`
			Path    string `json:"path"`
			Version string `json:"version"`
		} `json:"cli"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode context: %v\n%s", err, data)
	}
	if got.Version != 1 || got.Target != "codex" {
		t.Errorf("unexpected contract identity: %+v", got)
	}
	if got.ConfiguredModel != "gpt-test" {
		t.Errorf("configured_model = %q, want gpt-test", got.ConfiguredModel)
	}
	if got.RuntimeModel != "" || bytes.Contains(data, []byte(`"runtime_model"`)) {
		t.Errorf("verify must not claim an actual runtime model: %s", data)
	}
	if !strings.HasPrefix(got.HarnessFingerprint, "sha256:") || len(got.HarnessFingerprint) != len("sha256:")+64 {
		t.Errorf("unexpected fingerprint %q", got.HarnessFingerprint)
	}
	if got.CLI == nil || got.CLI.Command != "codex" || got.CLI.Path != filepath.Join(binDir, "codex") || got.CLI.Version != "codex-cli 9.9.9" {
		t.Errorf("unexpected CLI identity: %+v", got.CLI)
	}
}

func TestVerify_FailsBeforeVerifierWhenSelectedTargetDrifts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	dir := testutil.TempCwd(t)
	silence(t)

	marker := filepath.Join(dir, "verifier-ran")
	verifierPath := filepath.Join(dir, "verify.sh")
	writeFile(t, verifierPath, "#!/bin/sh\ntouch \"$1\"\n")
	if err := os.Chmod(verifierPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf("version: 1\ntargets: [claude]\nverify:\n  command: [%q, %q]\n", verifierPath, marker))
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/review.md"), "Review changes.\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"verify", "-t", "claude"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("verify should fail on drift, got: %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Errorf("verifier ran before sync drift passed: %v", statErr)
	}
}

func TestVerify_WithoutTargetRunsEveryConfiguredTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	dir := testutil.TempCwd(t)
	silence(t)

	contextsPath := filepath.Join(dir, "verify-contexts.jsonl")
	verifierPath := filepath.Join(dir, "verify.sh")
	writeFile(t, verifierPath, "#!/bin/sh\ncat >> \"$1\"\n")
	if err := os.Chmod(verifierPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf("version: 1\ntargets: [claude, codex]\nverify:\n  command: [%q, %q]\n", verifierPath, contextsPath))
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/review.md"), "Review changes.\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude,codex"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	root = NewRootCmd("test")
	root.SetArgs([]string{"verify"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(contextsPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("verifier invocations = %d, want 2: %s", len(lines), data)
	}
	var targets []string
	for _, line := range lines {
		var context struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal([]byte(line), &context); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, context.Target)
	}
	if strings.Join(targets, ",") != "claude,codex" {
		t.Errorf("targets = %v, want [claude codex]", targets)
	}
}

func TestVerify_RequiresConfiguredCommand(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"verify", "-t", "claude"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "verify.command") {
		t.Fatalf("expected missing verifier command error, got: %v", err)
	}
}

func TestVerify_ReturnsVerifierExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	dir := testutil.TempCwd(t)
	silence(t)

	verifierPath := filepath.Join(dir, "verify.sh")
	writeFile(t, verifierPath, "#!/bin/sh\nexit 23\n")
	if err := os.Chmod(verifierPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf("version: 1\ntargets: [claude]\nverify:\n  command: [%q]\n", verifierPath))
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/review.md"), "Review changes.\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"verify", "-t", "claude"})
	err := root.Execute()
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit-coded verifier error, got: %v", err)
	}
	if exitErr.ExitCode() != 23 {
		t.Errorf("exit code = %d, want 23", exitErr.ExitCode())
	}
}
