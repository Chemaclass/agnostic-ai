package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

const projectBootstrapEnv = "AGNOSTIC_AI_PROJECT_BOOTSTRAP"

type projectPackage struct {
	PackageManager  string            `json:"packageManager"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

type projectBinary struct {
	path   string
	prefix []string
}

func newProjectCmd() *cobra.Command {
	var check, bootstrap bool
	var against string
	cmd := &cobra.Command{
		Use:   "project [--check | --bootstrap] [-- <command>...]",
		Short: "Run the installed project binary with its configured version contract.",
		Long: "Prefers node_modules/.bin/agnostic-ai to PATH and checks exact stable package.json pins and the project's requires. " +
			"By default syncs with --keep-edits --quiet. --check checks output without installing " +
			"or writing files. --bootstrap installs missing or mismatched npm/pnpm dependencies " +
			"from the lockfile, then syncs. Package scripts run normally; recursive bootstrap " +
			"calls cannot start another install. Takes an optional command after -- for hooks.",
		Example: "  agnostic-ai project --check\n  agnostic-ai project --bootstrap\n  agnostic-ai project -- hook memory --target codex",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && (check || bootstrap || args[0] == "project") {
				return fmt.Errorf("project: command arguments cannot be combined with --check or --bootstrap, or recursively invoke project")
			}
			if against != "" && !check {
				return fmt.Errorf("project: --against requires --check")
			}
			return runProject(cmd, args, check, bootstrap, against)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Check the installed version and generated output without installing or writing.")
	cmd.Flags().BoolVar(&bootstrap, "bootstrap", false, "Install missing or mismatched npm/pnpm dependencies once from the lockfile, then sync preserving manual edits.")
	cmd.Flags().StringVar(&against, "against", "", "With --check, compare index or HEAD using the existing sync gate.")
	cmd.MarkFlagsMutuallyExclusive("check", "bootstrap")
	return cmd
}

func runProject(cmd *cobra.Command, args []string, check, bootstrap bool, against string) error {
	root, err := projectCommandRoot(args)
	if against != "" {
		root, err = os.Getwd()
	}
	if err != nil {
		return fmt.Errorf("project root: %w", err)
	}
	root, cfg, pkg, err := readProjectContract(root, against)
	if errs.CodeOf(err) == errs.CodeConfigMissing && len(args) > 1 && args[0] == "hook" {
		cfg, err = &config.Config{}, nil
	}
	if err != nil {
		return err
	}
	if _, err := projectExactPackagePin(pkg, cfg.Requires); err != nil {
		return err
	}
	binary, resolveErr := resolveProjectBinary(root, pkg, cfg.Requires)
	var capabilityErr *projectCapabilityError
	if errors.As(resolveErr, &capabilityErr) {
		return resolveErr
	}
	if resolveErr != nil && bootstrap && os.Getenv(projectBootstrapEnv) == "" {
		install, err := projectInstallCommand(root, pkg)
		if err != nil {
			return fmt.Errorf("project bootstrap: %w", err)
		}
		install = exec.CommandContext(cmd.Context(), install.Path, install.Args[1:]...)
		install.Dir, install.Stdin, install.Stdout, install.Stderr = root, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
		install.Env = append(os.Environ(), projectBootstrapEnv+"=1", envNoUpdateCheck+"=1")
		if err := install.Run(); err != nil {
			return fmt.Errorf("project bootstrap install: %w", err)
		}
		binary, resolveErr = resolveProjectBinary(root, pkg, cfg.Requires)
	}
	if resolveErr != nil {
		return resolveErr
	}
	if len(args) == 0 {
		args = []string{"sync", "--keep-edits", "--quiet"}
		if check {
			args = []string{"sync", "--check"}
			if against != "" {
				args = append(args, "--against", against)
			}
		}
	}
	child := exec.CommandContext(cmd.Context(), binary.path, append(binary.prefix, args...)...)
	child.Dir, child.Stdin, child.Stdout, child.Stderr = root, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	child.Env = append(os.Environ(), envNoUpdateCheck+"=1")
	if err := child.Run(); err != nil {
		return fmt.Errorf("project binary %s: %w", binary.path, err)
	}
	return nil
}

func readProjectContract(root, against string) (string, *config.Config, projectPackage, error) {
	contractRoot := root
	if against != "" {
		if err := validateAgainst(against, true, false, false, false); err != nil {
			return root, nil, projectPackage{}, err
		}
		origin, err := os.Getwd()
		if err != nil {
			return root, nil, projectPackage{}, err
		}
		top, err := gitOutput(root, nil, "rev-parse", "--show-toplevel")
		if err != nil {
			return root, nil, projectPackage{}, fmt.Errorf("project --against %s: %w", against, err)
		}
		top = canonicalDir(strings.TrimSpace(top))
		root = canonicalDir(root)
		relative, err := filepath.Rel(top, root)
		if err != nil {
			return root, nil, projectPackage{}, err
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return root, nil, projectPackage{}, fmt.Errorf("project contract %s is outside Git root %s", root, top)
		}
		if err := os.Chdir(top); err != nil {
			return root, nil, projectPackage{}, fmt.Errorf("project contract %s: %w", root, err)
		}
		defer func() { _ = os.Chdir(origin) }()
		tree, err := enterAgainstTree(against)
		if err != nil {
			return root, nil, projectPackage{}, err
		}
		defer tree.leave()
		contractRoot = filepath.Join(tree.root, relative)
		for {
			if _, _, err := config.ResolveConfigPath(contractRoot); err == nil || contractRoot == tree.root {
				break
			}
			contractRoot = filepath.Dir(contractRoot)
		}
		relative, err = filepath.Rel(tree.root, contractRoot)
		if err != nil {
			return root, nil, projectPackage{}, err
		}
		manifest := filepath.ToSlash(filepath.Join(relative, "package.json"))
		if err := tree.export(tree.trackedOf([]string{manifest})); err != nil {
			return root, nil, projectPackage{}, fmt.Errorf("project --against %s package.json: %w", against, err)
		}
		root = filepath.Join(top, relative)
	}
	cfg, _, configErr := config.LoadWithSources(contractRoot)
	if configErr != nil && errs.CodeOf(configErr) != errs.CodeConfigMissing {
		return root, cfg, projectPackage{}, configErr
	}
	pkg, err := readProjectPackage(contractRoot)
	if err == nil {
		err = configErr
	}
	return root, cfg, pkg, err
}

func projectCommandRoot(args []string) (string, error) {
	if len(args) >= 2 && args[0] == "hook" && args[1] == "memory" {
		hook := newHookMemoryCmd()
		if err := hook.ParseFlags(args[2:]); err != nil {
			return "", fmt.Errorf("project memory arguments: %w", err)
		}
		target, err := hook.Flags().GetString("target")
		if err != nil {
			return "", fmt.Errorf("project memory target: %w", err)
		}
		if target == "" {
			target = os.Getenv(adapters.HookTargetEnv)
		}
		if root := memoryProjectRoot(target); root != "" {
			return root, nil
		}
	}
	return projectRoot()
}

func projectRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("project working directory: %w", err)
	}
	if project := os.Getenv(hookProjectDirEnv[os.Getenv(adapters.HookTargetEnv)]); project != "" {
		root, err = filepath.Abs(project)
		if err != nil {
			return "", fmt.Errorf("hook project directory: %w", err)
		}
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		if _, _, err := config.ResolveConfigPath(dir); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return root, nil
		}
	}
}

func readProjectPackage(root string) (projectPackage, error) {
	path := filepath.Join(root, "package.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return projectPackage{}, nil
	}
	if err != nil {
		return projectPackage{}, fmt.Errorf("%s: %w", path, err)
	}
	var pkg projectPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return projectPackage{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return pkg, nil
}

func resolveProjectBinary(root string, pkg projectPackage, requires string) (projectBinary, error) {
	pin, err := projectExactPackagePin(pkg, requires)
	if err != nil {
		return projectBinary{}, err
	}
	local := filepath.Join(root, "node_modules", ".bin", "agnostic-ai")
	binary := projectBinary{path: local}
	if runtime.GOOS == "windows" {
		local = filepath.Join(root, "node_modules", "agnostic-ai", "bin", "agnostic-ai.js")
		binary = projectBinary{path: "node", prefix: []string{local}}
	}
	if _, err := os.Stat(local); err != nil {
		if !os.IsNotExist(err) {
			return projectBinary{}, fmt.Errorf("%s: %w", local, err)
		}
		if pkg.Dependencies["agnostic-ai"] != "" || pkg.DevDependencies["agnostic-ai"] != "" {
			return projectBinary{}, fmt.Errorf("project binary %s is missing; run `agnostic-ai project --bootstrap` from the project root", local)
		}
		path, err := exec.LookPath("agnostic-ai")
		if err != nil {
			return projectBinary{}, fmt.Errorf("no project or PATH binary; install agnostic-ai first: %w", err)
		}
		binary = projectBinary{path: path}
		if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(path), ".cmd") {
			shim := filepath.Join(filepath.Dir(path), "node_modules", "agnostic-ai", "bin", "agnostic-ai.js")
			if _, err := os.Stat(shim); err != nil {
				return projectBinary{}, fmt.Errorf("global launcher %s: use a native executable or install the project package: %w", shim, err)
			}
			binary = projectBinary{path: "node", prefix: []string{shim}}
		}
	}
	selected := binary.path
	if len(binary.prefix) > 0 {
		selected = binary.prefix[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, binary.path, append(binary.prefix, "--version")...)
	probe.Dir = root
	probe.Env = append(os.Environ(), envNoUpdateCheck+"=1")
	probe.WaitDelay = 250 * time.Millisecond
	var output projectProbeOutput
	probe.Stdout, probe.Stderr = &output, io.Discard
	err = probe.Run()
	if err != nil {
		return projectBinary{}, fmt.Errorf("project binary %s cannot report its version; run `agnostic-ai project --bootstrap`: %w", selected, err)
	}
	if output.overflow {
		return projectBinary{}, fmt.Errorf("project binary %s version output exceeds 64 KiB", selected)
	}
	fields := strings.Fields(output.buffer.String())
	if len(fields) == 0 {
		return projectBinary{}, fmt.Errorf("project binary %s reports no version; run `agnostic-ai project --bootstrap`", selected)
	}
	version := fields[len(fields)-1]
	if pin != "" && !versionsEqual(pin, version) {
		return projectBinary{}, fmt.Errorf("project binary %s is %s, but package.json pins agnostic-ai to %s; run `agnostic-ai project --bootstrap` (the lockfile must match the declared pin)", selected, version, pin)
	}
	if requires != "" {
		requirement, err := config.ParseRequirement(requires)
		if err != nil {
			return projectBinary{}, err
		}
		if allowed, _ := requirement.Allows(version); !allowed {
			return projectBinary{}, fmt.Errorf("project binary %s is %s, but requires %s; run `agnostic-ai project --bootstrap` (the lockfile must satisfy requires)", selected, version, requires)
		}
	}
	capability := exec.CommandContext(ctx, binary.path, append(binary.prefix, "project", "--help")...)
	capability.Dir, capability.Stdout, capability.Stderr = root, io.Discard, io.Discard
	capability.Env = append(os.Environ(), envNoUpdateCheck+"=1")
	capability.WaitDelay = 250 * time.Millisecond
	if err := capability.Run(); err != nil {
		return projectBinary{}, &projectCapabilityError{selected: selected, err: err}
	}
	return binary, nil
}

type projectCapabilityError struct {
	selected string
	err      error
}

func (e *projectCapabilityError) Error() string {
	return fmt.Sprintf("project binary %s does not support the project command; upgrade its owning package to a supporting release and update package.json and requires explicitly, then rerun (no global fallback): %v", e.selected, e.err)
}

func (e *projectCapabilityError) Unwrap() error { return e.err }

type projectProbeOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (o *projectProbeOutput) Write(p []byte) (int, error) {
	remaining := (64 << 10) - o.buffer.Len()
	if len(p) > remaining {
		o.overflow = true
		_, _ = o.buffer.Write(p[:remaining])
	} else {
		_, _ = o.buffer.Write(p)
	}
	return len(p), nil
}

func projectExactPackagePin(pkg projectPackage, requires string) (string, error) {
	var pin string
	for _, declaration := range []string{pkg.Dependencies["agnostic-ai"], pkg.DevDependencies["agnostic-ai"]} {
		declaration = strings.TrimSpace(declaration)
		if !stableRelease(declaration) {
			continue
		}
		if pin != "" && !versionsEqual(pin, declaration) {
			return "", fmt.Errorf("package.json declares conflicting exact agnostic-ai versions %s and %s; update the declarations explicitly", pin, declaration)
		}
		pin = declaration
	}
	if pin != "" && requires != "" {
		requirement, err := config.ParseRequirement(requires)
		if err != nil {
			return "", err
		}
		if allowed, _ := requirement.Allows(pin); !allowed {
			return "", fmt.Errorf("package.json pins agnostic-ai to %s, which conflicts with requires %s; update the declared pin, lockfile, or requires explicitly before bootstrap", pin, requires)
		}
	}
	return pin, nil
}

func projectInstallCommand(root string, pkg projectPackage) (*exec.Cmd, error) {
	if pkg.Dependencies["agnostic-ai"] == "" && pkg.DevDependencies["agnostic-ai"] == "" {
		return nil, fmt.Errorf("package.json must declare agnostic-ai before bootstrap can install it")
	}
	manager, _, _ := strings.Cut(pkg.PackageManager, "@")
	pnpm := fileExists(filepath.Join(root, "pnpm-lock.yaml"))
	npm := fileExists(filepath.Join(root, "package-lock.json"))
	if manager == "" {
		if pnpm == npm {
			return nil, fmt.Errorf("declare packageManager as npm or pnpm, with its committed lockfile")
		}
		if pnpm {
			manager = "pnpm"
		} else {
			manager = "npm"
		}
	}
	switch manager {
	case "pnpm":
		if !pnpm {
			return nil, fmt.Errorf("pnpm bootstrap requires pnpm-lock.yaml")
		}
		if runtime.GOOS == "windows" {
			return exec.Command("cmd.exe", "/d", "/s", "/c", "pnpm install --frozen-lockfile"), nil
		}
		return exec.Command("pnpm", "install", "--frozen-lockfile"), nil
	case "npm":
		if !npm {
			return nil, fmt.Errorf("npm bootstrap requires package-lock.json")
		}
		if runtime.GOOS == "windows" {
			return exec.Command("cmd.exe", "/d", "/s", "/c", "npm ci"), nil
		}
		return exec.Command("npm", "ci"), nil
	default:
		return nil, fmt.Errorf("bootstrap supports npm and pnpm; %s installs must run explicitly before `agnostic-ai project`", manager)
	}
}
