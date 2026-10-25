package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupRecordedVerifier(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VERIFY_RESULTS_TEST_PROCESS", mode)
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [claude, codex]\nbuiltins: []\nverify:\n  command: [%q, '-test.run=^TestVerifyResultProcess$']\n", path))
	writeFile(t, ".agnostic-ai/rules/review.md", "Review changes.\n")
	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVerifyResultProcess(t *testing.T) {
	mode := os.Getenv("VERIFY_RESULTS_TEST_PROCESS")
	if mode == "" {
		return
	}
	var verification verifyContext
	if err := json.NewDecoder(os.Stdin).Decode(&verification); err != nil {
		os.Exit(90)
	}
	switch mode {
	case "pass":
		os.Exit(0)
	case "fail23":
		_, _ = fmt.Fprintln(os.Stdout, "private-output")
		os.Exit(23)
	case "wait":
		if err := os.WriteFile("verifier-started", []byte("started"), 0o600); err != nil {
			os.Exit(91)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "write-error", "write-success-error":
		if err := os.Mkdir("run.json", 0o700); err != nil {
			os.Exit(91)
		}
		if mode == "write-success-error" {
			os.Exit(0)
		}
		os.Exit(19)
	default:
		os.Exit(92)
	}
}

func executeRecordedVerify(args ...string) error {
	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"verify"}, args...))
	return root.Execute()
}

func TestVerify_RecordsFailureAndSkippedTargetsPrivately(t *testing.T) {
	setupRecordedVerifier(t, "fail23")
	err := executeRecordedVerify("--record", "run.json", "--evaluation", "suite-v1")
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) || coded.ExitCode() != 23 {
		t.Fatalf("exit = %v", err)
	}
	data, err := os.ReadFile("run.json")
	if err != nil {
		t.Fatal(err)
	}
	var run verifyRun
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	if run.Version != 1 || run.Evaluation != "suite-v1" || run.StartedAt.IsZero() || run.Verifier == "" {
		t.Errorf("run identity = %+v", run)
	}
	if len(run.Targets) != 2 || run.Targets[0].Outcome != "failed" || run.Targets[0].ExitCode == nil || *run.Targets[0].ExitCode != 23 || run.Targets[1].Outcome != "skipped" {
		t.Fatalf("targets = %+v", run.Targets)
	}
	if run.Targets[0].ElapsedNS <= 0 || run.Targets[0].HarnessFingerprint == "" {
		t.Errorf("missing measurements: %+v", run.Targets[0])
	}
	if bytes.Contains(data, []byte("private-output")) || bytes.Contains(data, []byte("-test.run=")) {
		t.Errorf("record leaked verifier output/arguments: %s", data)
	}
	info, err := os.Stat("run.json")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("permissions = %o", info.Mode().Perm())
	}
	before := string(data)
	if err := executeRecordedVerify("--record", "run.json", "--evaluation", "suite-v1"); err == nil {
		t.Fatal("overwrite allowed without flag")
	}
	after, _ := os.ReadFile("run.json")
	if string(after) != before {
		t.Error("existing result changed")
	}
	err = executeRecordedVerify("--record", "run.json", "--evaluation", "suite-v1", "--overwrite")
	if !errors.As(err, &coded) || coded.ExitCode() != 23 {
		t.Errorf("overwrite changed exit = %v", err)
	}
}

func TestVerify_RecordingStopsAtDriftAndRequiresEvaluation(t *testing.T) {
	setupRecordedVerifier(t, "pass")
	if err := executeRecordedVerify("--record", "run.json"); err == nil || !strings.Contains(err.Error(), "evaluation") {
		t.Fatalf("missing evaluation = %v", err)
	}
	writeFile(t, ".agnostic-ai/rules/review.md", "Changed.\n")
	if err := executeRecordedVerify("--record", "run.json", "--evaluation", "v1"); err == nil {
		t.Fatal("drift passed")
	}
	if _, err := os.Stat("run.json"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("drift wrote result: %v", err)
	}
}

func TestVerify_RecordsLaunchFailure(t *testing.T) {
	setupRecordedVerifier(t, "pass")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: []\nverify:\n  command: [./missing-verifier]\n")
	if err := executeRecordedVerify("--record", "launch.json", "--evaluation", "v1"); err == nil {
		t.Fatal("missing verifier succeeded")
	}
	data, err := os.ReadFile("launch.json")
	if err != nil {
		t.Fatal(err)
	}
	var run verifyRun
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	if run.Targets[0].Outcome != "execution_error" || run.Targets[0].ExitCode != nil {
		t.Errorf("launch outcome = %+v", run.Targets[0])
	}
}

func TestVerify_RecordingCancellation(t *testing.T) {
	setupRecordedVerifier(t, "wait")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"verify", "--record", "cancel.json", "--evaluation", "v1"})
	if err := root.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
}

func TestVerify_RecordingWriteFailurePreservesVerifierExit(t *testing.T) {
	setupRecordedVerifier(t, "write-error")
	err := executeRecordedVerify("--record", "run.json", "--evaluation", "v1")
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) || coded.ExitCode() != 19 || !strings.Contains(err.Error(), "save result") {
		t.Fatalf("write failure lost exit or error: %v", err)
	}
	temps, _ := filepath.Glob(".verify-result-*")
	if len(temps) != 0 {
		t.Errorf("left temporary files: %v", temps)
	}
}

func TestVerify_ComparisonOutcomesAndIdentity(t *testing.T) {
	zero, one := 0, 1
	before := verifyRun{Version: 1, Evaluation: "suite-v1", Verifier: "same", StartedAt: time.Now(), Targets: []verifyTargetResult{
		{verifyContext: verifyContext{Target: "regresses", HarnessFingerprint: "old"}, Outcome: "passed", ExitCode: &zero, ElapsedNS: 20},
		{verifyContext: verifyContext{Target: "recovers"}, Outcome: "failed", ExitCode: &one, ElapsedNS: 30},
		{verifyContext: verifyContext{Target: "unchanged"}, Outcome: "passed", ExitCode: &zero},
		{verifyContext: verifyContext{Target: "removed"}, Outcome: "passed", ExitCode: &zero},
	}}
	after := verifyRun{Version: 1, Evaluation: "suite-v1", Verifier: "same", StartedAt: time.Now(), Targets: []verifyTargetResult{
		{verifyContext: verifyContext{Target: "regresses", HarnessFingerprint: "new"}, Outcome: "failed", ExitCode: &one, ElapsedNS: 50},
		{verifyContext: verifyContext{Target: "recovers"}, Outcome: "passed", ExitCode: &zero, ElapsedNS: 10},
		{verifyContext: verifyContext{Target: "unchanged"}, Outcome: "passed", ExitCode: &zero},
		{verifyContext: verifyContext{Target: "added"}, Outcome: "skipped"},
	}}
	got := compareVerifyRuns(before, after)
	want := map[string]string{"regresses": "regression", "recovers": "recovery", "unchanged": "unchanged", "removed": "missing_target", "added": "missing_target"}
	for _, row := range got.Targets {
		if row.Status != want[row.Target] {
			t.Errorf("%s = %s", row.Target, row.Status)
		}
		if row.Target == "regresses" && row.ElapsedDifferenceNS != 30 {
			t.Errorf("elapsed = %d", row.ElapsedDifferenceNS)
		}
	}
	for _, tc := range []struct{ name, evaluation, verifier, reason string }{
		{"evaluation", "other", "same", "evaluation identity changed"},
		{"verifier", "suite-v1", "other", "verifier identity changed"},
		{"unknown", "suite-v1", "", "verifier identity unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := after
			candidate.Evaluation = tc.evaluation
			candidate.Verifier = tc.verifier
			for _, row := range compareVerifyRuns(before, candidate).Targets {
				if row.Before != nil && row.After != nil && (row.Status != "non_comparable" || row.Reason != tc.reason) {
					t.Errorf("identity = %+v", row)
				}
			}
		})
	}
}

func TestVerify_ComparisonValidatesFilesAndPrintsVersionedJSON(t *testing.T) {
	dir := t.TempDir()
	for _, body := range []string{`{`, `{"version":2}`, `{"version":1}`, `{"version":1,"evaluation":"v1","started_at":"2026-10-24T00:00:00Z","targets":[{"target":"claude","outcome":"passed"}]}`} {
		path := filepath.Join(dir, "invalid.json")
		writeFile(t, path, body)
		if _, err := readVerifyRun(path); err == nil {
			t.Errorf("accepted invalid record %s", body)
		}
	}
	zero := 0
	run := verifyRun{Version: 1, Evaluation: "suite", StartedAt: time.Now().UTC(), Verifier: "sha256:same", Targets: []verifyTargetResult{{verifyContext: verifyContext{Version: 1, Target: "claude", HarnessFingerprint: "hash"}, Outcome: "passed", ExitCode: &zero, ElapsedNS: 10}}}
	baseline := filepath.Join(dir, "before.json")
	candidate := filepath.Join(dir, "after.json")
	if err := writeVerifyRun(baseline, run, false); err != nil {
		t.Fatal(err)
	}
	if err := writeVerifyRun(candidate, run, false); err != nil {
		t.Fatal(err)
	}
	for _, jsonOut := range []bool{false, true} {
		root := NewRootCmd("test")
		var out bytes.Buffer
		root.SetOut(&out)
		args := []string{"verify", "compare", baseline, candidate}
		if jsonOut {
			args = append(args, "--json")
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if jsonOut {
			var comparison verifyComparison
			if err := json.Unmarshal(out.Bytes(), &comparison); err != nil || comparison.Version != 1 || comparison.Targets[0].Status != "unchanged" {
				t.Errorf("JSON: %v %s", err, out.String())
			}
		} else if !strings.Contains(out.String(), "claude: unchanged") {
			t.Errorf("human output: %s", out.String())
		}
	}
}

func TestVerify_VerifierIdentityTracksInterpreterScriptChanges(t *testing.T) {
	dir := t.TempDir()
	interpreter := filepath.Join(dir, "python3")
	if runtime.GOOS == "windows" {
		interpreter += ".exe"
	}
	writeFile(t, interpreter, "Local interpreter bytes.\n")
	if err := os.Chmod(interpreter, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "evaluate.py")
	writeFile(t, script, "exit(0)\n")
	argv := []string{interpreter, script, "secret-argument"}
	first := verifierIdentity(argv)
	if first == "" || strings.Contains(first, "secret") {
		t.Fatalf("identity = %q", first)
	}
	writeFile(t, script, "exit(1)\n")
	second := verifierIdentity(argv)
	if second == first || second == "" {
		t.Fatalf("script change not detected: %q", second)
	}
	before := verifyRun{Evaluation: "suite-v1", Verifier: first, Targets: []verifyTargetResult{{verifyContext: verifyContext{Target: "codex"}, Outcome: "passed"}}}
	after := before
	after.Verifier = second
	row := compareVerifyRuns(before, after).Targets[0]
	if row.Status != "non_comparable" || row.Reason != "verifier identity changed" {
		t.Errorf("script change compared: %+v", row)
	}
	if identity := verifierIdentity([]string{interpreter, "-m", "evaluation_module"}); identity != "" {
		t.Errorf("module invocation claims identity %q", identity)
	}
}

func TestVerify_RecordingRedactsDetectedMetadata(t *testing.T) {
	secret := "sk-" + strings.Repeat("a1", 24)
	path := filepath.Join(t.TempDir(), "result.json")
	zero := 0
	run := verifyRun{Version: 1, Evaluation: "suite-v1", StartedAt: time.Now().UTC(), Targets: []verifyTargetResult{{
		verifyContext: verifyContext{Version: 1, Target: "codex", ConfiguredModel: secret, CLI: &verifyCLIIdentity{Command: "codex", Path: "/tools/" + secret + "/codex", Version: "codex " + secret}}, Outcome: "passed", ExitCode: &zero,
	}}}
	if err := writeVerifyRun(path, run, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Error("saved detected metadata contains credentials")
	}
	if run.Targets[0].ConfiguredModel != secret || run.Targets[0].CLI.Version != "codex "+secret {
		t.Error("recording changed the verifier context")
	}
}

func TestVerify_RecordingSuccessDoesNotHideWriteFailure(t *testing.T) {
	setupRecordedVerifier(t, "write-success-error")
	err := executeRecordedVerify("--record", "run.json", "--evaluation", "v1")
	if err == nil || !strings.Contains(err.Error(), "save result") {
		t.Fatalf("recording failure = %v", err)
	}
}

func TestVerify_ResultInstallDoesNotOverwriteAConcurrentDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	if err := validateVerifyRecording(path, "v1", false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "Keep the concurrent file.\n")
	if err := writeVerifyRun(path, verifyRun{Version: 1}, false); err == nil {
		t.Fatal("concurrent destination overwritten")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Keep the concurrent file.\n" {
		t.Errorf("concurrent content changed: %q", got)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != "result.json" {
		t.Errorf("temporary result left behind: %v", files)
	}
}

func TestVerify_ResultOverwritePublishesPrivateCompleteJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	writeFile(t, path, "Original result.\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeVerifyRun(path, verifyRun{Version: 1, Evaluation: "replacement"}, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result verifyRun
	if err := json.Unmarshal(data, &result); err != nil || result.Evaluation != "replacement" {
		t.Fatalf("replacement = %s, error %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("overwrite permissions = %o", info.Mode().Perm())
	}
}

func TestVerify_RecordingRejectsCredentialsInEvaluationIdentifier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	if err := validateVerifyRecording(path, "sk-"+strings.Repeat("a1", 24), false); err == nil {
		t.Error("credential accepted as evaluation identity")
	}
}

func TestVerify_RecordsAllPassingTargetsAndConfiguredModel(t *testing.T) {
	setupRecordedVerifier(t, "pass")
	data, err := os.ReadFile("agnostic-ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, "agnostic-ai.yaml", string(data)+"outputs:\n  claude:\n    settings:\n      model: configured-only\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if err := executeRecordedVerify("--record", "passed.json", "--evaluation", "suite-v1"); err != nil {
		t.Fatal(err)
	}
	run, err := readVerifyRun("passed.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Targets) != 2 {
		t.Fatalf("passing targets = %v", run.Targets)
	}
	for _, target := range run.Targets {
		if target.Outcome != "passed" || target.ExitCode == nil || *target.ExitCode != 0 || target.ElapsedNS <= 0 || target.HarnessFingerprint == "" {
			t.Errorf("passing result = %+v", target)
		}
	}
	if run.Targets[0].ConfiguredModel != "configured-only" {
		t.Errorf("configured model = %q", run.Targets[0].ConfiguredModel)
	}
}

func TestVerify_CancellationAfterLaunchRecordsExecutionError(t *testing.T) {
	setupRecordedVerifier(t, "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if _, err := os.Stat("verifier-started"); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"verify", "--record", "cancelled.json", "--evaluation", "suite-v1"})
	err := root.ExecuteContext(ctx)
	cancel()
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("running cancellation = %v", err)
	}
	if _, err := os.Stat("verifier-started"); err != nil {
		t.Fatalf("verifier never launched: %v", err)
	}
	run, err := readVerifyRun("cancelled.json")
	if err != nil {
		t.Fatal(err)
	}
	if run.Targets[0].Outcome != "execution_error" || run.Targets[0].ExitCode != nil || run.Targets[1].Outcome != "skipped" {
		t.Errorf("canceled results = %+v", run.Targets)
	}
}

func TestVerify_DataArgumentsDoNotChangeVerifierIdentity(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	zero, failure := 0, 23
	for _, kind := range []string{"harness input", "output destination"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "argument.txt")
			writeFile(t, path, "Before.\n")
			argv := []string{executable, path}
			before := verifyRun{Version: 1, Evaluation: "suite-v1", Verifier: verifierIdentity(argv), Targets: []verifyTargetResult{{verifyContext: verifyContext{Target: "codex"}, Outcome: "passed", ExitCode: &zero}}}
			writeFile(t, path, "After.\n")
			after := before
			after.Verifier = verifierIdentity(argv)
			after.Targets = append([]verifyTargetResult(nil), before.Targets...)
			want := "unchanged"
			if kind == "harness input" {
				after.Targets[0].Outcome, after.Targets[0].ExitCode = "failed", &failure
				want = "regression"
			}
			rows := compareVerifyRuns(before, after).Targets
			if rows[0].Status != want {
				t.Errorf("changed data argument: got %s, want %s", rows[0].Status, want)
			}
		})
	}
}

func TestVerify_RecordingRunsDuplicateSelectionOnce(t *testing.T) {
	setupRecordedVerifier(t, "pass")
	if err := executeRecordedVerify("--target", "codex,claude,codex", "--record", "duplicate.json", "--evaluation", "suite-v1"); err != nil {
		t.Fatal(err)
	}
	run, err := readVerifyRun("duplicate.json")
	if err != nil {
		t.Fatalf("recording cannot read its own run: %v", err)
	}
	if len(run.Targets) != 2 || run.Targets[0].Target != "codex" || run.Targets[1].Target != "claude" {
		t.Errorf("selected targets = %+v", run.Targets)
	}
}
