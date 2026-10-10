package cli

import (
	"context"
	"encoding/json"
	"fmt"
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
		Long: "Prefers node_modules/.bin/agnostic-ai to PATH and checks the project's requires. " +
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
	if err != nil {
		return fmt.Errorf("project root: %w", err)
	}
	cfg, _, err := config.LoadWithSources(root)
	if errs.CodeOf(err) == errs.CodeConfigMissing && len(args) > 1 && args[0] == "hook" {
		cfg, err = &config.Config{}, nil
	}
	if err != nil {
		return err
	}
	pkg, err := readProjectPackage(root)
	if err != nil {
		return err
	}
	binary, resolveErr := resolveProjectBinary(root, pkg, cfg.Requires)
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
	out, err := probe.Output()
	if err != nil {
		return projectBinary{}, fmt.Errorf("project binary %s cannot report its version; run `agnostic-ai project --bootstrap`: %w", selected, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return projectBinary{}, fmt.Errorf("project binary %s reports no version; run `agnostic-ai project --bootstrap`", selected)
	}
	version := fields[len(fields)-1]
	if requires != "" {
		requirement, err := config.ParseRequirement(requires)
		if err != nil {
			return projectBinary{}, err
		}
		if allowed, _ := requirement.Allows(version); !allowed {
			return projectBinary{}, fmt.Errorf("project binary %s is %s, but requires %s; run `agnostic-ai project --bootstrap` (the lockfile must satisfy requires)", selected, version, requires)
		}
	}
	return binary, nil
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
