package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// installMethod identifies how the running agnostic-ai binary was
// installed. The detector inspects os.Executable() and matches against
// well-known path prefixes per platform.
type installMethod int

const (
	installUnknown installMethod = iota
	installHomebrew
	installGoInstall
	installBinary
)

func (m installMethod) String() string {
	switch m {
	case installHomebrew:
		return "homebrew"
	case installGoInstall:
		return "go install"
	case installBinary:
		return "binary"
	}
	return "unknown"
}

// upgradeInfo summarizes what `upgrade` knows about the current install.
// Path is the resolved executable path; Method is the detected install
// channel; Command is the shell command that would refresh the binary;
// Shadows lists any other agnostic-ai binaries on PATH that would beat
// the current one (or be beaten by it), so the user can spot a stale
// shadow before running the upgrade.
type upgradeInfo struct {
	Path    string
	Method  installMethod
	Version string
	Latest  string
	Command string
	Shadows []string
	Notes   []string
}

func newUpgradeCmd() *cobra.Command {
	var (
		run       bool
		checkOnly bool
	)
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Report (or run) the right command to upgrade agnostic-ai.",
		Long: `upgrade detects how the running agnostic-ai binary was installed
(Homebrew, ` + "`go install`" + `, or a raw prebuilt binary) and prints the
matching upgrade command. It does not replace its own binary; package
managers are still the source of truth for installed versions.

Pass --run to exec the detected command. Pass --check to print the
detection result and exit without running anything. Both flags are
no-ops when the install method is unknown.`,
		Example: `  # Show the upgrade command for the current install
  agnostic-ai upgrade

  # Run the upgrade command directly
  agnostic-ai upgrade --run

  # Diagnose install location + PATH shadowing
  agnostic-ai upgrade --check`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpgrade(cmd.OutOrStdout(), run, checkOnly, cmd.Root().Version)
		},
	}
	cmd.Flags().BoolVar(&run, "run", false, "Execute the detected upgrade command")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Print detection details and exit without running")
	return cmd
}

func runUpgrade(out io.Writer, doRun, checkOnly bool, currentVersion string) error {
	info, err := detectUpgrade(currentVersion)
	if err != nil {
		return err
	}
	printUpgradeInfo(out, info)
	if checkOnly {
		return nil
	}
	if info.Latest != "" && info.Version != "" && versionsEqual(info.Version, info.Latest) {
		_, _ = fmt.Fprintf(out, "\nAlready on latest (%s). Nothing to do.\n", info.Latest)
		return nil
	}
	if !doRun {
		if info.Command != "" {
			_, _ = fmt.Fprintf(out, "\nRun: %s\n", info.Command)
			_, _ = fmt.Fprintf(out, "Or re-run with --run to execute.\n")
		}
		return nil
	}
	if info.Command == "" {
		return fmt.Errorf("upgrade: install method unknown; download a release from https://github.com/Chemaclass/agnostic-ai/releases")
	}
	_, _ = fmt.Fprintf(out, "\n→ %s\n", info.Command)
	return execShell(info.Command)
}

func detectUpgrade(currentVersion string) (upgradeInfo, error) {
	exe, err := os.Executable()
	if err != nil {
		return upgradeInfo{}, fmt.Errorf("locate executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = resolved
	}
	info := upgradeInfo{
		Path:    exe,
		Method:  detectInstallMethod(exe),
		Version: strings.TrimSpace(currentVersion),
	}
	info.Command = upgradeCommandFor(info.Method)
	info.Shadows = otherInstancesOnPATH(exe)
	if latest, err := fetchLatestRelease(2 * time.Second); err == nil {
		info.Latest = latest
	}
	if info.Method == installUnknown {
		info.Notes = append(info.Notes,
			"install location does not match Homebrew, $GOPATH/bin, or common binary dirs;",
			"download a release tarball: https://github.com/Chemaclass/agnostic-ai/releases")
	}
	if len(info.Shadows) > 0 {
		info.Notes = append(info.Notes,
			"another agnostic-ai is on PATH. The shadowing binary may be an older",
			"install (e.g. go install + brew, or a stale /usr/local/bin copy). Remove it",
			"so `agnostic-ai --version` resolves to the upgraded binary.")
	}
	return info, nil
}

// detectInstallMethod classifies an executable path. Homebrew layouts
// vary by platform: macOS Apple silicon under /opt/homebrew, Intel
// under /usr/local/Cellar, Linuxbrew under /home/linuxbrew/.linuxbrew.
// Casks land under Caskroom on macOS. Go installs land under $GOBIN
// (when set) or $GOPATH/bin (default $HOME/go/bin). Anything else is
// treated as a raw binary install.
func detectInstallMethod(exe string) installMethod {
	p := filepath.ToSlash(exe)
	for _, marker := range []string{
		"/Cellar/", "/Caskroom/", "/opt/homebrew/", "/linuxbrew/", "/homebrew/",
	} {
		if strings.Contains(p, marker) {
			return installHomebrew
		}
	}
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		if sameDir(filepath.Dir(exe), gobin) {
			return installGoInstall
		}
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		if sameDir(filepath.Dir(exe), filepath.Join(gopath, "bin")) {
			return installGoInstall
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if sameDir(filepath.Dir(exe), filepath.Join(home, "go", "bin")) {
			return installGoInstall
		}
	}
	if exe != "" {
		return installBinary
	}
	return installUnknown
}

func upgradeCommandFor(m installMethod) string {
	switch m {
	case installHomebrew:
		return "brew update && brew upgrade Chemaclass/tap/agnostic-ai"
	case installGoInstall:
		return "go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest"
	case installBinary:
		return ""
	}
	return ""
}

// otherInstancesOnPATH returns absolute paths of every agnostic-ai
// binary on PATH that is not the running one. A non-empty result hints
// at a shadowing problem: PATH lookup may pick an older copy even after
// the brew install is up-to-date. Paths are resolved through symlinks
// so a symlink + its target collapse to one entry.
func otherInstancesOnPATH(self string) []string {
	binName := "agnostic-ai"
	if runtime.GOOS == "windows" {
		binName = "agnostic-ai.exe"
	}
	selfResolved, _ := filepath.EvalSymlinks(self)
	if selfResolved == "" {
		selfResolved = self
	}
	seen := map[string]bool{selfResolved: true}
	var out []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, binName)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			resolved = candidate
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		out = append(out, candidate)
	}
	return out
}

func sameDir(a, b string) bool {
	ap, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bp, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return filepath.Clean(ap) == filepath.Clean(bp)
}

// fetchLatestRelease queries the GitHub releases API for the tag of the
// latest published release. Best-effort: a network failure returns an
// error and the caller proceeds without a "Latest" hint.
func fetchLatestRelease(timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/Chemaclass/agnostic-ai/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases api: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(body.TagName), "v"), nil
}

func versionsEqual(a, b string) bool {
	return strings.TrimPrefix(strings.TrimSpace(a), "v") == strings.TrimPrefix(strings.TrimSpace(b), "v")
}

func execShell(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func printUpgradeInfo(out io.Writer, info upgradeInfo) {
	_, _ = fmt.Fprintf(out, "Install method: %s\n", info.Method)
	_, _ = fmt.Fprintf(out, "Binary:         %s\n", info.Path)
	if info.Version != "" {
		_, _ = fmt.Fprintf(out, "Installed:      %s\n", info.Version)
	}
	if info.Latest != "" {
		_, _ = fmt.Fprintf(out, "Latest:         %s\n", info.Latest)
	}
	if len(info.Shadows) > 0 {
		_, _ = fmt.Fprintf(out, "PATH shadows:\n")
		for _, s := range info.Shadows {
			_, _ = fmt.Fprintf(out, "  - %s\n", s)
		}
	}
	for _, n := range info.Notes {
		_, _ = fmt.Fprintf(out, "! %s\n", n)
	}
}
