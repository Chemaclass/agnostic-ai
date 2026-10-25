package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

type verifyRun struct {
	Version    int                  `json:"version"`
	StartedAt  time.Time            `json:"started_at"`
	Evaluation string               `json:"evaluation"`
	Verifier   string               `json:"verifier_identity"`
	Targets    []verifyTargetResult `json:"targets"`
}

type verifyTargetResult struct {
	verifyContext
	ElapsedNS int64  `json:"elapsed_ns"`
	Outcome   string `json:"outcome"`
	ExitCode  *int   `json:"exit_code,omitempty"`
}

func validateVerifyRecording(path, evaluation string, overwrite bool) error {
	if path == "" {
		if evaluation != "" || overwrite {
			return fmt.Errorf("--evaluation and --overwrite require --record")
		}
		return nil
	}
	if strings.TrimSpace(evaluation) == "" {
		return fmt.Errorf("--record requires --evaluation with a suite and dataset revision")
	}
	if preview.RedactText(evaluation) != evaluation {
		return fmt.Errorf("--evaluation must be an identifier without credentials")
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !overwrite {
			return fmt.Errorf("%s: result already exists; use --overwrite to replace it", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: result destination must be a regular file", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func verifierIdentity(argv []string) string {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return ""
	}
	for _, arg := range argv {
		writeFingerprintPart(hash, "arg", arg)
	}

	script, comparable := verifierEntryScript(path, argv[1:])
	if !comparable {
		return ""
	}
	if script != "" {
		file, err := os.Open(script)
		if err != nil {
			return ""
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			_ = file.Close()
			return ""
		}
		_, prefixErr := fmt.Fprintf(hash, "%d:%s%d:", len("script"), "script", info.Size())
		copied, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if prefixErr != nil || copyErr != nil || closeErr != nil || copied != info.Size() {
			return ""
		}
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func verifierEntryScript(executable string, args []string) (string, bool) {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe")
	interpreter := slices.Contains([]string{"sh", "bash", "dash", "ash", "ksh", "zsh", "node", "nodejs", "ruby", "perl", "lua", "luajit", "php", "pwsh", "powershell", "cmd", "cscript", "wscript"}, name)
	if version, python := strings.CutPrefix(name, "python"); python {
		interpreter = version == "" || version[0] >= '0' && version[0] <= '9' && strings.Trim(version, "0123456789.") == ""
	}
	if !interpreter {
		return "", true
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[0], "/") && (name == "cmd" || name == "cscript" || name == "wscript") {
		return "", false
	}
	return args[0], true
}

func writeVerifyRun(path string, run verifyRun, overwrite bool) error {
	data, err := json.MarshalIndent(redactVerifyRun(run), "", "  ")
	if err != nil {
		return fmt.Errorf("%s: encode result: %w", path, err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".verify-result-*")
	if err != nil {
		return fmt.Errorf("%s: create result: %w", path, err)
	}
	name := temp.Name()
	defer func() { _ = os.Remove(name) }()
	_, err = temp.Write(append(data, '\n'))
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		if overwrite {
			err = os.Rename(name, path)
		} else {
			err = os.Link(name, path)
		}
	}
	if err != nil {
		return fmt.Errorf("%s: save result: %w", path, err)
	}
	return nil
}

func redactVerifyRun(run verifyRun) verifyRun {
	run.Targets = slices.Clone(run.Targets)
	for i := range run.Targets {
		target := &run.Targets[i]
		target.ConfiguredModel = preview.RedactText(target.ConfiguredModel)
		if target.CLI != nil {
			identity := *target.CLI
			identity.Command = preview.RedactText(identity.Command)
			identity.Path = preview.RedactText(identity.Path)
			identity.Version = preview.RedactText(identity.Version)
			target.CLI = &identity
		}
	}
	return run
}

type verifyComparison struct {
	Version             int                      `json:"version"`
	Evaluation          string                   `json:"evaluation"`
	CandidateEvaluation string                   `json:"candidate_evaluation"`
	BaselineVerifier    string                   `json:"baseline_verifier_identity"`
	CandidateVerifier   string                   `json:"candidate_verifier_identity"`
	BaselineStartedAt   time.Time                `json:"baseline_started_at"`
	CandidateStartedAt  time.Time                `json:"candidate_started_at"`
	Targets             []verifyTargetComparison `json:"targets"`
}

type verifyTargetComparison struct {
	Target              string              `json:"target"`
	Status              string              `json:"status"`
	Reason              string              `json:"reason,omitempty"`
	Before              *verifyTargetResult `json:"before,omitempty"`
	After               *verifyTargetResult `json:"after,omitempty"`
	ElapsedDifferenceNS int64               `json:"elapsed_difference_ns"`
}

func newVerifyCompareCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{Use: "compare <baseline.json> <candidate.json>", Short: "Compare two explicitly recorded verification runs", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		baseline, err := readVerifyRun(args[0])
		if err != nil {
			return err
		}
		candidate, err := readVerifyRun(args[1])
		if err != nil {
			return err
		}
		result := compareVerifyRuns(baseline, candidate)
		if jsonOut {
			return writeIndentedJSON(cmd, result)
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Baseline: %s, evaluation %s, verifier %s\nCandidate: %s, evaluation %s, verifier %s\n", baseline.StartedAt.Format(time.RFC3339), baseline.Evaluation, baseline.Verifier, candidate.StartedAt.Format(time.RFC3339), candidate.Evaluation, candidate.Verifier); err != nil {
			return err
		}
		for _, target := range result.Targets {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (elapsed difference %s)%s\n", target.Target, target.Status, time.Duration(target.ElapsedDifferenceNS), verifyComparisonReason(target.Reason)); err != nil {
				return err
			}
			for _, item := range []struct {
				label  string
				result *verifyTargetResult
			}{{"before", target.Before}, {"after", target.After}} {
				if item.result == nil {
					continue
				}
				identity := "unknown"
				if item.result.CLI != nil {
					identity = item.result.CLI.Command + " " + item.result.CLI.Version + " (" + item.result.CLI.Path + ")"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s, elapsed %s, CLI %s, configured model %q, harness %s\n", item.label, item.result.Outcome, time.Duration(item.result.ElapsedNS), identity, item.result.ConfiguredModel, item.result.HarnessFingerprint); err != nil {
					return err
				}
			}
		}
		return nil
	}}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output a versioned JSON comparison")
	return cmd
}

func verifyComparisonReason(reason string) string {
	if reason == "" {
		return ""
	}
	return ": " + reason
}

func readVerifyRun(path string) (verifyRun, error) {
	var run verifyRun
	data, err := os.ReadFile(path)
	if err != nil {
		return run, fmt.Errorf("%s: %w", path, err)
	}
	if err := json.Unmarshal(data, &run); err != nil {
		return run, fmt.Errorf("parse %s: %w", path, err)
	}
	if run.Version != 1 || strings.TrimSpace(run.Evaluation) == "" || run.StartedAt.IsZero() || len(run.Targets) == 0 {
		return run, fmt.Errorf("%s: unsupported or incomplete verification result", path)
	}
	seen := map[string]bool{}
	for _, target := range run.Targets {
		if target.Version != verifyContractVersion || target.Target == "" || seen[target.Target] || target.ElapsedNS < 0 {
			return run, fmt.Errorf("%s: invalid or duplicate target", path)
		}
		seen[target.Target] = true
		switch target.Outcome {
		case "passed":
			if target.HarnessFingerprint == "" || target.ExitCode == nil || *target.ExitCode != 0 {
				return run, fmt.Errorf("%s: invalid passing outcome", path)
			}
		case "failed":
			if target.HarnessFingerprint == "" || target.ExitCode == nil || *target.ExitCode <= 0 {
				return run, fmt.Errorf("%s: invalid failing outcome", path)
			}
		case "execution_error", "skipped":
			if target.ExitCode != nil {
				return run, fmt.Errorf("%s: invalid non-evaluation outcome", path)
			}
		default:
			return run, fmt.Errorf("%s: unknown outcome %q", path, target.Outcome)
		}
	}
	return redactVerifyRun(run), nil
}

func compareVerifyRuns(before, after verifyRun) verifyComparison {
	out := verifyComparison{Version: 1, Evaluation: before.Evaluation, CandidateEvaluation: after.Evaluation, BaselineVerifier: before.Verifier, CandidateVerifier: after.Verifier, BaselineStartedAt: before.StartedAt, CandidateStartedAt: after.StartedAt, Targets: []verifyTargetComparison{}}
	old := map[string]*verifyTargetResult{}
	next := map[string]*verifyTargetResult{}
	names := []string{}
	for i := range before.Targets {
		target := &before.Targets[i]
		old[target.Target] = target
		names = append(names, target.Target)
	}
	for i := range after.Targets {
		target := &after.Targets[i]
		next[target.Target] = target
		if old[target.Target] == nil {
			names = append(names, target.Target)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		row := verifyTargetComparison{Target: name, Before: old[name], After: next[name]}
		switch {
		case row.Before == nil || row.After == nil:
			row.Status = "missing_target"
		case before.Evaluation != after.Evaluation:
			row.Status = "non_comparable"
			row.Reason = "evaluation identity changed"
		case before.Verifier == "" || after.Verifier == "":
			row.Status = "non_comparable"
			row.Reason = "verifier identity unknown"
		case before.Verifier != after.Verifier:
			row.Status = "non_comparable"
			row.Reason = "verifier identity changed"
		case row.Before.Outcome == "skipped" || row.After.Outcome == "skipped":
			row.Status = "non_comparable"
			row.Reason = "target was skipped"
		case row.Before.Outcome == "execution_error" || row.After.Outcome == "execution_error":
			row.Status = "execution_error"
		case row.Before.Outcome == "passed" && row.After.Outcome == "failed":
			row.Status = "regression"
		case row.Before.Outcome == "failed" && row.After.Outcome == "passed":
			row.Status = "recovery"
		default:
			row.Status = "unchanged"
		}
		if row.Before != nil && row.After != nil {
			row.ElapsedDifferenceNS = row.After.ElapsedNS - row.Before.ElapsedNS
		}
		out.Targets = append(out.Targets, row)
	}
	return out
}
