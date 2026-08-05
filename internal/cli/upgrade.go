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

// Repository + distribution coordinates. Kept as package-level constants
// so a fork or rename only touches this block.
const (
	repoOwner       = "Chemaclass"
	repoName        = "agnostic-ai"
	binaryName      = "agnostic-ai"
	tapPackage      = repoOwner + "/tap/" + repoName
	wingetPackage   = repoOwner + "." + repoName
	releasesAPIURL  = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
	releasesHTMLURL = "https://github.com/" + repoOwner + "/" + repoName + "/releases"
	userAgent       = repoName + "-upgrade"
)

// installMethod identifies how the running agnostic-ai binary was
// installed. The detector inspects os.Executable() and matches against
// well-known path prefixes per platform.
type installMethod int

const (
	installUnknown installMethod = iota
	installHomebrew
	installGoInstall
	installScoop
	installWinget
	installNPM
	installBinary
)

// methodSpec carries the human-readable name and shell command for a
// given install method. Keeping name + command in one table avoids
// two parallel switches drifting out of sync.
type methodSpec struct {
	name    string
	command string
}

var methodSpecs = map[installMethod]methodSpec{
	installHomebrew:  {"homebrew", "brew update && brew upgrade --cask " + tapPackage},
	installGoInstall: {"go install", "go install github.com/chemaclass/" + repoName + "/cmd/" + binaryName + "@latest"},
	installScoop:     {"scoop", "scoop update " + repoName},
	installWinget:    {"winget", "winget upgrade " + wingetPackage},
	installNPM:       {"npm", "npm install -g " + repoName + "@latest"},
	installBinary:    {"binary", ""},
	installUnknown:   {"unknown", ""},
}

func (m installMethod) String() string {
	if s, ok := methodSpecs[m]; ok {
		return s.name
	}
	return "unknown"
}

func upgradeCommandFor(m installMethod) string {
	return methodSpecs[m].command
}

// upgradeInfo summarizes what `upgrade` knows about the current install.
//
//   - Path:     resolved executable path (symlinks evaluated).
//   - LinkPath: original `os.Executable()` path before symlink eval; lets
//     callers inspect the binstub itself (e.g. brew's `bin/agnostic-ai`
//     symlink) rather than its Caskroom target.
//   - Method:   detected install channel.
//   - Version:  build-time version of the running binary.
//   - Latest:   tag of the latest GitHub release, when reachable.
//   - Command:  shell command that would refresh the binary.
//   - Shadows:  other agnostic-ai binaries on PATH that beat or are beaten
//     by the current one; surfaces stale installs the user may want to
//     remove.
//   - Notes:    additional diagnostics for unknown installs or shadows.
type upgradeInfo struct {
	Path     string
	LinkPath string
	Method   installMethod
	Version  string
	Latest   string
	Command  string
	Shadows  []string
	Notes    []string
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
(Homebrew, ` + "`go install`" + `, Scoop, winget, npm, or a raw prebuilt
binary) and prints the matching upgrade command. It does not replace its
own binary; package managers are still the source of truth for installed
versions.

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
		return fmt.Errorf("upgrade: install method unknown; download a release from %s", releasesHTMLURL)
	}
	if info.Method == installHomebrew {
		if ok, hint := homebrewBinaryOK(info.LinkPath); !ok {
			return fmt.Errorf("upgrade: homebrew cask state is inconsistent.\n%s", hint)
		}
	}
	_, _ = fmt.Fprintf(out, "\n→ %s\n", info.Command)
	return execShell(info.Command)
}

// homebrewBinaryOK verifies that the binstub path is a symlink owned by
// Homebrew. The cask post-install links `<brew>/bin/agnostic-ai` to a file
// inside `<brew>/Caskroom/agnostic-ai/<version>/`. If a regular file lives
// at the binstub path instead (e.g. a raw binary copied in before the cask
// existed), `brew upgrade --cask` refuses to overwrite it, reverts mid-run,
// and leaves the cask uninstalled while the orphan file remains. Catching
// that pre-flight gives the user a clear `rm + brew install --cask`
// remediation instead of a confusing brew error followed by a half-broken
// install.
func homebrewBinaryOK(linkPath string) (ok bool, hint string) {
	if linkPath == "" {
		return true, ""
	}
	info, err := os.Lstat(linkPath)
	if err != nil {
		return true, ""
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, brewCollisionHint(linkPath, fmt.Sprintf("%s is a regular file, not a brew-owned symlink", linkPath))
	}
	target, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		return false, brewCollisionHint(linkPath, fmt.Sprintf("%s is a dangling symlink: %v", linkPath, err))
	}
	t := filepath.ToSlash(target)
	if !strings.Contains(t, "/Caskroom/") && !strings.Contains(t, "/Cellar/") {
		return false, brewCollisionHint(linkPath, fmt.Sprintf("%s resolves to %s, outside the brew prefix", linkPath, target))
	}
	return true, ""
}

func brewCollisionHint(path, reason string) string {
	return fmt.Sprintf("%s.\n"+
		"`brew upgrade --cask` would fail and revert. Recover with:\n"+
		"  rm %s && brew install --cask %s",
		reason, path, tapPackage)
}

func detectUpgrade(currentVersion string) (upgradeInfo, error) {
	exe, err := os.Executable()
	if err != nil {
		return upgradeInfo{}, fmt.Errorf("locate executable: %w", err)
	}
	linkPath := exe
	resolved, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = resolved
	}
	info := upgradeInfo{
		Path:     exe,
		LinkPath: linkPath,
		Method:   detectInstallMethod(exe),
		Version:  strings.TrimSpace(currentVersion),
	}
	info.Command = upgradeCommandFor(info.Method)
	info.Shadows = otherInstancesOnPATH(exe)
	if latest, err := fetchLatestRelease(2 * time.Second); err == nil {
		info.Latest = latest
	}
	if info.Method == installUnknown {
		info.Notes = append(info.Notes,
			"install location does not match Homebrew, $GOPATH/bin, or common binary dirs;",
			"download a release tarball: "+releasesHTMLURL)
	}
	if len(info.Shadows) > 0 {
		info.Notes = append(info.Notes,
			"another "+binaryName+" is on PATH. The shadowing binary may be an older",
			"install (e.g. go install + brew, or a stale /usr/local/bin copy). Remove it",
			"so `"+binaryName+" --version` resolves to the upgraded binary.")
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
	for _, marker := range homebrewMarkers {
		if strings.Contains(p, marker) {
			return installHomebrew
		}
	}
	// Backslashes are replaced explicitly, not via ToSlash, which is a no-op
	// off Windows: a Windows path reaching this code from a test or a WSL
	// mount must classify the same way. Casing is folded too, since these
	// path segments carry whatever the user typed (C:\Users\Me\Scoop\shims).
	lower := strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
	for _, m := range pathMarkers {
		if strings.Contains(lower, m.marker) {
			return m.method
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

// homebrewMarkers are substrings that identify a brew-managed executable
// path across macOS Intel, macOS Apple silicon, and Linuxbrew layouts.
var homebrewMarkers = []string{
	"/Cellar/",
	"/Caskroom/",
	"/opt/homebrew/",
	"/linuxbrew/",
	"/homebrew/",
}

// pathMarkers map a lowercased path fragment to the channel that owns it.
// Scoop keeps apps under <root>/apps and shims under <root>/shims; winget
// portable packages land under Microsoft/WinGet with a shim in its Links
// dir; an npm global install always sits inside node_modules.
var pathMarkers = []struct {
	marker string
	method installMethod
}{
	{"/scoop/apps/", installScoop},
	{"/scoop/shims/", installScoop},
	{"/microsoft/winget/", installWinget},
	{"/node_modules/", installNPM},
}

// otherInstancesOnPATH returns absolute paths of every agnostic-ai
// binary on PATH that is not the running one. A non-empty result hints
// at a shadowing problem: PATH lookup may pick an older copy even after
// the brew install is up-to-date. Paths are resolved through symlinks
// so a symlink + its target collapse to one entry.
func otherInstancesOnPATH(self string) []string {
	binFile := binaryName
	if runtime.GOOS == "windows" {
		binFile = binaryName + ".exe"
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
		candidate := filepath.Join(dir, binFile)
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
	return fetchLatestReleaseFrom(releasesAPIURL, timeout)
}

// fetchLatestReleaseFrom is the testable form of fetchLatestRelease. It
// accepts an explicit URL so tests can point at an httptest.Server.
//
// A User-Agent header is set because the GitHub API rejects unidentified
// clients with HTTP 403 under load.
func fetchLatestReleaseFrom(url string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
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
