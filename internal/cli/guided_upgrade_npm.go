package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func guidedNPMUpgradeHint(root, latest string) string {
	exe, err := runningExecutable()
	if err != nil {
		return ""
	}
	if resolved, err := config.ResolveSourceAlias(exe); err == nil {
		exe = resolved
	}
	return npmUpgradeHint(exe, root, latest, func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "npm", "root", "-g").Output()
		return strings.TrimSpace(string(output)), err
	})
}

func npmUpgradeHint(exe, root, latest string, globalRoot func() (string, error)) string {
	if detectInstallMethod(exe) != installNPM {
		return ""
	}
	if root != "" {
		source, _, err := config.ResolveConfigPath(root)
		if err == nil {
			if pm, ok := projectPackageManager(source); ok {
				packageRoot := root
				if i := strings.Index(exe, string(filepath.Separator)+"node_modules"+string(filepath.Separator)); i >= 0 {
					packageRoot = exe[:i]
				}
				return fmt.Sprintf("This is a project package installation. From %s, run %s agnostic-ai@%s, then rerun this command and reconcile the project with its package manager's installed CLI.", packageRoot, pm.add, latest)
			}
		}
	}
	path := strings.ReplaceAll(filepath.ToSlash(exe), "\\", "/")
	if strings.Contains(path, "/_npx/") {
		return fmt.Sprintf("This executable belongs to the npx cache. Run npx agnostic-ai@%s with your requested command to use the new release.", latest)
	}
	if dir, err := globalRoot(); err == nil && dir != "" {
		if resolved, err := config.ResolveSourceAlias(dir); err == nil {
			dir = resolved
		}
		rel, err := filepath.Rel(dir, exe)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
	}
	return fmt.Sprintf("This npm executable at %s is not a verified global installation. Update the package that owns it with its package manager, then rerun this command; the global npm updater will not run.", exe)
}
