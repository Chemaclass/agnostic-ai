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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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
			entryPointTargets := configuredEntryPointConsumers(cfg, effective)
			reports, err := collectDriftWithEntryPointTargets(effective, entryPointTargets)
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
	adapter, err := adapters.Resolve(target)
	if err != nil {
		return verifyContext{}, fmt.Errorf("verify %s: %w", target, err)
	}
	files, err := captureVerifyFiles(cfg, b, target, adapter)
	if err != nil {
		return verifyContext{}, err
	}
	fingerprint, err := fingerprintHarness(b.For(target).All(), files)
	if err != nil {
		return verifyContext{}, fmt.Errorf("verify %s: fingerprint specs: %w", target, err)
	}
	return verifyContext{
		Version:            verifyContractVersion,
		Target:             target,
		ConfiguredModel:    configuredModel(cfg, target, files),
		CLI:                discoverCLIIdentity(target),
		HarnessFingerprint: fingerprint,
	}, nil
}

func configuredModel(cfg *config.Config, target string, files []adapters.CapturedFile) string {
	model, _ := emittedConfiguredModel(cfg, target, files)
	return model
}

// emittedConfiguredModel reads model identity from the exact config bytes the
// adapter captured. It describes configured emission only, never runtime
// identity. Parse failures are non-fatal because configured_model is optional.
func emittedConfiguredModel(cfg *config.Config, target string, files []adapters.CapturedFile) (string, bool) {
	path, format := emittedModelFile(cfg, target)
	if path == "" {
		return "", false
	}
	content, ok := capturedContent(files, path)
	if !ok {
		return "", false
	}
	switch format {
	case "toml":
		doc := map[string]any{}
		if _, err := toml.Decode(header.Strip(content), &doc); err == nil {
			model, found := doc["model"]
			value, _ := model.(string)
			return value, found
		}
	case "yaml":
		doc := map[string]any{}
		if err := yaml.Unmarshal([]byte(header.Strip(content)), &doc); err == nil {
			model, found := doc["model"]
			value, _ := model.(string)
			return value, found
		}
	case "json":
		doc := map[string]any{}
		strictJSON, _ := adapters.StripJSONC([]byte(content))
		if err := json.Unmarshal(strictJSON, &doc); err != nil {
			return "", false
		}
		model, found := doc["model"]
		switch model := model.(type) {
		case string:
			return model, true
		case map[string]any:
			name, _ := model["name"].(string)
			return name, true
		}
		return "", found
	}
	return "", false
}

func emittedModelFile(cfg *config.Config, target string) (string, string) {
	output := cfg.Outputs[target]
	switch target {
	case "aider":
		return output.ConfFile, "yaml"
	case "claude":
		dir := output.Dir
		if dir == "" {
			dir = ".claude"
		}
		return filepath.Join(dir, "settings.json"), "json"
	case "codex":
		path := output.MCPFile
		if path == "" {
			path = ".codex/config.toml"
		}
		return path, "toml"
	case "copilot":
		return ".github/copilot/settings.json", "json"
	case "junie":
		return ".junie/config.json", "json"
	case "kilo":
		path := output.MCPFile
		if path == "" {
			path = "kilo.jsonc"
		}
		return path, "json"
	case "opencode":
		path := output.MCPFile
		if path == "" {
			path = "opencode.json"
		}
		return path, "json"
	case "qoder":
		path := output.MCPFile
		if path == "" {
			path = ".qoder/settings.json"
		}
		return path, "json"
	default:
		return "", ""
	}
}

func capturedContent(files []adapters.CapturedFile, path string) (string, bool) {
	want := normalizeFingerprintPath(path)
	for i := len(files) - 1; i >= 0; i-- {
		if normalizeFingerprintPath(files[i].Path) == want {
			return files[i].Content, true
		}
	}
	return "", false
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

func captureVerifyFiles(cfg *config.Config, b spec.Bundle, target string, adapter adapters.Adapter) ([]adapters.CapturedFile, error) {
	sess := adapters.NewSession()
	files, err := captureAdapterFiles(sess, adapter, b, cfg)
	if err != nil {
		return nil, fmt.Errorf("verify %s: %w", target, err)
	}

	agnosticBody, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", adapters.AgnosticEntryPointPath, err)
	}
	entryPointTargets := configuredEntryPointConsumers(cfg, []string{target})
	entryPoints, err := renderEntryPointFiles(cfg, b, entryPointTargets, header.Strip(string(agnosticBody)))
	if err != nil {
		return nil, err
	}
	files = append(files, adapters.CapturedFile{Path: adapters.AgnosticEntryPointPath, Content: string(agnosticBody)})
	for _, file := range entryPoints {
		files = append(files, adapters.CapturedFile{Path: file.Path, Content: file.Content})
	}
	return files, nil
}

// configuredEntryPointConsumers expands selected targets only across paths
// they consume. This preserves target-scoped native verification while a
// shared AGENTS.md is rendered with every configured reader of that file.
func configuredEntryPointConsumers(cfg *config.Config, selected []string) []string {
	selectedPaths := map[string]bool{}
	for _, target := range selected {
		if adapters.LegacyRulesFileOwnsEntryPoint(cfg, target) {
			continue
		}
		if path := adapters.EntryPointPath(cfg, target); path != "" && path != adapters.AgnosticEntryPointPath {
			selectedPaths[normalizeFingerprintPath(path)] = true
		}
	}
	var consumers []string
	for _, target := range cfg.Targets {
		if adapters.LegacyRulesFileOwnsEntryPoint(cfg, target) {
			continue
		}
		if selectedPaths[normalizeFingerprintPath(adapters.EntryPointPath(cfg, target))] {
			consumers = append(consumers, target)
		}
	}
	return consumers
}

func fingerprintHarness(entries []spec.Entry, files []adapters.CapturedFile) (string, error) {
	normalizedFiles := make([]adapters.CapturedFile, len(files))
	for i, file := range files {
		normalizedFiles[i] = adapters.CapturedFile{
			Path:    normalizeFingerprintPath(file.Path),
			Content: file.Content,
		}
	}
	sort.Slice(normalizedFiles, func(i, j int) bool {
		if normalizedFiles[i].Path == normalizedFiles[j].Path {
			return normalizedFiles[i].Content < normalizedFiles[j].Content
		}
		return normalizedFiles[i].Path < normalizedFiles[j].Path
	})

	hash := sha256.New()
	specs, err := normalizedSpecFingerprint(entries)
	if err != nil {
		return "", err
	}
	writeFingerprintPart(hash, "specs", specs)
	for _, file := range normalizedFiles {
		writeFingerprintPart(hash, file.Path, file.Content)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

type fingerprintSpec struct {
	Kind       spec.Kind      `json:"kind"`
	Name       string         `json:"name"`
	Path       string         `json:"path"`
	Scope      string         `json:"scope"`
	Layer      string         `json:"layer"`
	Meta       map[string]any `json:"meta"`
	MetaKeys   []string       `json:"meta_keys"`
	MetaStyles map[string]int `json:"meta_styles"`
	Body       string         `json:"body"`
}

func normalizedSpecFingerprint(entries []spec.Entry) (string, error) {
	normalized := make([]fingerprintSpec, len(entries))
	for i, entry := range entries {
		styles := make(map[string]int, len(entry.MetaStyles))
		for key, style := range entry.MetaStyles {
			styles[key] = int(style)
		}
		normalized[i] = fingerprintSpec{
			Kind:       entry.Kind,
			Name:       entry.Name,
			Path:       normalizeFingerprintPath(entry.Path),
			Scope:      normalizeFingerprintPath(entry.Scope),
			Layer:      entry.Layer,
			Meta:       entry.Meta,
			MetaKeys:   entry.MetaKeys,
			MetaStyles: styles,
			Body:       entry.Body,
		}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizeFingerprintPath(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), `\`, "/")
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
