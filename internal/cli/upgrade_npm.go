package cli

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const npmUpgradeGuidance = "Could not verify a global npm prefix for this CLI. Update it with the package manager that installed it."

var npmPowerShellQuotes = strings.NewReplacer(
	"'", "''",
	"\U00002018", "\U00002018\U00002018",
	"\U00002019", "\U00002019\U00002019",
	"\U0000201a", "\U0000201a\U0000201a",
	"\U0000201b", "\U0000201b\U0000201b",
)

func npmUpgradeStatusCommand(path string) string {
	if runtime.GOOS == "windows" {
		return "& '" + npmPowerShellQuotes.Replace(path) + "' upgrade"
	}
	return shellQuote(path) + " upgrade"
}

func npmPlatformPath() []string {
	npmOS, npmArch := runtime.GOOS, runtime.GOARCH
	name := binaryName
	if runtime.GOOS == "windows" {
		npmOS, name = "win32", name+".exe"
	}
	if npmArch == "amd64" {
		npmArch = "x64"
	}
	return []string{"node_modules", "@agnostic-ai", npmOS + "-" + npmArch, name}
}

func npmGlobalPrefix(exe string) string {
	platformPath := npmPlatformPath()
	prefix := trimNPMPathSuffix(exe, platformPath...)
	if prefix == "" {
		return ""
	}

	umbrella := prefix
	if base := trimNPMPathSuffix(umbrella, "node_modules", repoName); base != "" {
		prefix = base
	} else {
		umbrella = filepath.Join(prefix, "node_modules", repoName)
		// The launcher resolves a nested dependency before a hoisted one.
		if _, err := os.Stat(filepath.Join(umbrella, filepath.Join(platformPath...))); !errors.Is(err, os.ErrNotExist) {
			return ""
		}
	}
	if runtime.GOOS != "windows" {
		prefix = trimNPMPathSuffix(prefix, "lib")
	}
	if prefix == "" || !npmGlobalLauncher(prefix, umbrella) {
		return ""
	}
	return prefix
}

func npmGlobalPlatformBinary(prefix string) string {
	base := prefix
	if runtime.GOOS != "windows" {
		base = filepath.Join(base, "lib")
	}
	platformPath := filepath.Join(npmPlatformPath()...)
	nested := filepath.Join(base, "node_modules", repoName, platformPath)
	if _, err := os.Stat(nested); errors.Is(err, os.ErrNotExist) {
		return filepath.Join(base, platformPath)
	}
	return nested
}

func trimNPMPathSuffix(path string, parts ...string) string {
	for _, expected := range slices.Backward(parts) {
		part := filepath.Base(path)
		if part != expected && (runtime.GOOS != "windows" || !strings.EqualFold(part, expected)) {
			return ""
		}
		path = filepath.Dir(path)
	}
	return path
}

func npmGlobalLauncher(prefix, umbrella string) bool {
	targetPath := filepath.Join(umbrella, "bin", binaryName+".js")
	entryPoint, err := os.Lstat(targetPath)
	if err != nil || !entryPoint.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		shim, err := os.ReadFile(filepath.Join(prefix, binaryName+".cmd"))
		if err != nil {
			return false
		}
		target := `"%dp0%\node_modules\agnostic-ai\bin\agnostic-ai.js"`
		normalized := strings.ReplaceAll(strings.ToLower(string(shim)), `\\`, `\`)
		return strings.Contains(normalized, target)
	}
	shim, err := filepath.EvalSymlinks(filepath.Join(prefix, "bin", binaryName))
	return err == nil && shim == targetPath
}

func runNPMUpgrade(out io.Writer, prefix string) error {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", upgradeCommandFor(installNPM))
		cmd.Env = append(os.Environ(), "npm_config_prefix="+prefix)
		return runUpgradeCommand(out, cmd)
	}
	cmd := exec.Command("npm", "install", "-g", "--prefix", prefix, repoName+"@latest")
	return runUpgradeCommand(out, cmd)
}
