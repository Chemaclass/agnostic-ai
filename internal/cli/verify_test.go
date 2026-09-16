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

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
	writeFile(t, filepath.Join(dir, ".agnostic-ai/overlays/codex.config.toml"), "model = \"overlay-model\"\n")

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
	if got.ConfiguredModel != "overlay-model" {
		t.Errorf("configured_model = %q, want overlay-model", got.ConfiguredModel)
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

func TestVerify_SelectedTargetUsesAllConfiguredSharedEntryPointConsumers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	dir := testutil.TempCwd(t)
	silence(t)

	contextPath := filepath.Join(dir, "verify-context.json")
	verifierPath := filepath.Join(dir, "verify.sh")
	writeFile(t, verifierPath, "#!/bin/sh\ncat > \"$1\"\n")
	if err := os.Chmod(verifierPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf(`version: 1
targets: [codex, amp]
sync:
  target-overview: true
verify:
  command: [%q, %q]
`, verifierPath, contextPath))
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/review.md"), "Review changes.\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	root = NewRootCmd("test")
	root.SetArgs([]string{"verify", "-t", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatalf("verify selected codex target: %v", err)
	}

	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatal(err)
	}
	var got verifyContext
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := adapters.Resolve("codex")
	if err != nil {
		t.Fatal(err)
	}
	nativeFiles, err := captureAdapterFiles(adapters.NewSession(), adapter, bundle, cfg)
	if err != nil {
		t.Fatal(err)
	}
	agnosticBody, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		t.Fatal(err)
	}
	withEntryPoints := func(targets []string) []adapters.CapturedFile {
		files := append([]adapters.CapturedFile(nil), nativeFiles...)
		files = append(files, adapters.CapturedFile{Path: adapters.AgnosticEntryPointPath, Content: string(agnosticBody)})
		entryPoints, renderErr := renderEntryPointFiles(cfg, bundle, targets, header.Strip(string(agnosticBody)))
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		for _, file := range entryPoints {
			files = append(files, adapters.CapturedFile{Path: file.Path, Content: file.Content})
		}
		return files
	}
	want, err := fingerprintHarness(bundle.For("codex").All(), withEntryPoints(cfg.Targets))
	if err != nil {
		t.Fatal(err)
	}
	if got.HarnessFingerprint != want {
		t.Errorf("fingerprint = %q, want full shared-consumer fingerprint %q", got.HarnessFingerprint, want)
	}
	singleton, err := fingerprintHarness(bundle.For("codex").All(), withEntryPoints([]string{"codex"}))
	if err != nil {
		t.Fatal(err)
	}
	if got.HarnessFingerprint == singleton {
		t.Errorf("fingerprint used fictitious codex-only AGENTS.md bytes: %q", singleton)
	}
}

func TestVerify_DoesNotReportPortableModelForTargetsWithoutSettings(t *testing.T) {
	bundle := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindSettings,
		Name: "defaults",
		Meta: map[string]any{"model": "portable-model"},
	}})
	cfg := &config.Config{Outputs: map[string]config.Output{}}
	for _, target := range []string{"gemini", "aider"} {
		t.Run(target, func(t *testing.T) {
			adapter, err := adapters.Resolve(target)
			if err != nil {
				t.Fatal(err)
			}
			files, err := captureAdapterFiles(adapters.NewSession(), adapter, bundle, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got := configuredModel(cfg, target, files); got != "" {
				t.Errorf("configuredModel() = %q, want empty", got)
			}
		})
	}
}

func TestVerify_FingerprintNormalizesPathsAcrossOperatingSystems(t *testing.T) {
	unixEntries := []spec.Entry{{
		Kind:  spec.KindSettings,
		Name:  "defaults",
		Path:  "settings/team/defaults.yaml",
		Scope: "team/blue",
		Meta:  map[string]any{"model": "example"},
	}}
	windowsEntries := append([]spec.Entry(nil), unixEntries...)
	windowsEntries[0].Path = `settings\team\defaults.yaml`
	windowsEntries[0].Scope = `team\blue`

	unixFiles := []adapters.CapturedFile{
		{Path: ".codex/config.toml", Content: "model = \"example\"\n"},
		{Path: "AGENTS.md", Content: "instructions\n"},
	}
	windowsFiles := []adapters.CapturedFile{
		{Path: `AGENTS.md`, Content: "instructions\n"},
		{Path: `.codex\config.toml`, Content: "model = \"example\"\n"},
	}

	unixFingerprint, err := fingerprintHarness(unixEntries, unixFiles)
	if err != nil {
		t.Fatal(err)
	}
	windowsFingerprint, err := fingerprintHarness(windowsEntries, windowsFiles)
	if err != nil {
		t.Fatal(err)
	}
	if windowsFingerprint != unixFingerprint {
		t.Errorf("fingerprints differ by path separator: windows %q, unix %q", windowsFingerprint, unixFingerprint)
	}
	if got := normalizeFingerprintPath(`rules\nested/review.md`); got != "rules/nested/review.md" {
		t.Errorf("normalizeFingerprintPath() = %q", got)
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
