package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const verifyContractVersion = 1

type verifyContext struct {
	Version            int                `json:"version"`
	Target             string             `json:"target"`
	ConfiguredModel    string             `json:"configured_model,omitempty"`
	CLI                *verifyCLIIdentity `json:"cli,omitempty"`
	HarnessFingerprint string             `json:"harness_fingerprint"`
}

type verifyCLIIdentity struct {
	Command string `json:"command"`
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

type verifierExitError struct {
	target string
	err    *exec.ExitError
}

func (e *verifierExitError) Error() string {
	return fmt.Sprintf("verify %s: verifier exited with status %d", e.target, e.err.ExitCode())
}

func (e *verifierExitError) Unwrap() error { return e.err }

func (e *verifierExitError) ExitCode() int { return e.err.ExitCode() }

func newVerifyCmd() *cobra.Command {
	var targets []string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run an external behavior gate against the current AI harness.",
		Args:  cobra.NoArgs,
		Example: `  # Verify every configured target
  agnostic-ai verify

  # Verify one target after changing its model or CLI version
  agnostic-ai verify --target codex`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			if len(cfg.Verify.Command) == 0 || strings.TrimSpace(cfg.Verify.Command[0]) == "" {
				return fmt.Errorf("verify.command must name an executable and optional arguments")
			}

			effective := cfg.Targets
			if len(targets) > 0 {
				effective, err = filterTargets(cfg.Targets, targets, nil)
				if err != nil {
					return err
				}
			}
			reports, err := collectDrift(effective)
			if err != nil {
				return err
			}
			if err := reportCheckDrift(cmd, reports, checkFormatHuman, false); err != nil {
				return fmt.Errorf("verify: %w", err)
			}

			for _, target := range effective {
				verification, err := buildVerifyContext(cfg, b, target)
				if err != nil {
					return err
				}
				if err := runVerifier(cmd, cfg.Verify.Command, verification); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to verify (default: all in config)")
	registerTargetCompletion(cmd)
	return cmd
}

func buildVerifyContext(cfg *config.Config, b spec.Bundle, target string) (verifyContext, error) {
	fingerprint, err := harnessFingerprint(cfg, b, target)
	if err != nil {
		return verifyContext{}, err
	}
	return verifyContext{
		Version:            verifyContractVersion,
		Target:             target,
		ConfiguredModel:    configuredModel(cfg, b, target),
		CLI:                discoverCLIIdentity(target),
		HarnessFingerprint: fingerprint,
	}, nil
}

func configuredModel(cfg *config.Config, b spec.Bundle, target string) string {
	var model string
	for _, entry := range b.For(target).Settings {
		if value, _ := entry.Meta["model"].(string); value != "" {
			model = value
		}
	}
	output, ok := cfg.Outputs[target]
	if !ok {
		return model
	}
	if output.Model != "" {
		model = output.Model
	}
	if target == "claude" && output.Settings != nil && output.Settings.Model != "" {
		model = output.Settings.Model
	}
	if target == "codex" && output.Config != nil && output.Config.Model != "" {
		model = output.Config.Model
	}
	return model
}

func discoverCLIIdentity(target string) *verifyCLIIdentity {
	binary, ok := knownCLIBinaries[target]
	if !ok {
		return nil
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil
	}
	identity := &verifyCLIIdentity{Command: binary, Path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err == nil {
		identity.Version = strings.TrimSpace(string(output))
	}
	return identity
}

func harnessFingerprint(cfg *config.Config, b spec.Bundle, target string) (string, error) {
	adapter, err := adapters.Resolve(target)
	if err != nil {
		return "", fmt.Errorf("verify %s: %w", target, err)
	}
	sess := adapters.NewSession()
	files, err := captureAdapterFiles(sess, adapter, b, cfg)
	if err != nil {
		return "", fmt.Errorf("verify %s: %w", target, err)
	}

	agnosticBody, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		return "", fmt.Errorf("%s: %w", adapters.AgnosticEntryPointPath, err)
	}
	entryPoints, err := renderEntryPointFiles(cfg, b, []string{target}, header.Strip(string(agnosticBody)))
	if err != nil {
		return "", err
	}
	files = append(files, adapters.CapturedFile{Path: adapters.AgnosticEntryPointPath, Content: string(agnosticBody)})
	for _, file := range entryPoints {
		files = append(files, adapters.CapturedFile{Path: file.Path, Content: file.Content})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path == files[j].Path {
			return files[i].Content < files[j].Content
		}
		return files[i].Path < files[j].Path
	})

	hash := sha256.New()
	specs, err := json.Marshal(b.For(target).All())
	if err != nil {
		return "", fmt.Errorf("verify %s: fingerprint specs: %w", target, err)
	}
	writeFingerprintPart(hash, "specs", string(specs))
	for _, file := range files {
		writeFingerprintPart(hash, file.Path, file.Content)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func writeFingerprintPart(dst io.Writer, name, content string) {
	_, _ = fmt.Fprintf(dst, "%d:%s%d:%s", len(name), name, len(content), content)
}

func runVerifier(cmd *cobra.Command, argv []string, context verifyContext) error {
	payload, err := json.Marshal(context)
	if err != nil {
		return fmt.Errorf("verify %s: encode context: %w", context.Target, err)
	}
	child := exec.Command(argv[0], argv[1:]...)
	child.Stdin = strings.NewReader(string(payload) + "\n")
	child.Stdout = cmd.OutOrStdout()
	child.Stderr = cmd.ErrOrStderr()
	if err := child.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &verifierExitError{target: context.Target, err: exitErr}
		}
		return fmt.Errorf("verify %s: run %s: %w", context.Target, argv[0], err)
	}
	return nil
}
