package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	releasesBaseURL = releasesHTMLURL + "/download"
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
	Path        string
	LinkPath    string
	Method      installMethod
	Version     string
	Latest      string
	LatestError error
	Command     string
	Shadows     []string
	Notes       []string
}

func newUpgradeCmd() *cobra.Command {
	return newUpgradeCmdWithDeps(defaultUpgradeDeps())
}

type upgradeDeps struct {
	detect  func(string) (upgradeInfo, error)
	run     func(io.Writer, string) error
	install func(string, string, string, *http.Client) error
}

func defaultUpgradeDeps() upgradeDeps {
	return upgradeDeps{detect: detectUpgrade, run: execShell, install: installReleaseBinary}
}

func newUpgradeCmdWithDeps(deps upgradeDeps) *cobra.Command {
	var (
		checkOnly     bool
		targetVersion string
	)
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update"},
		Short:   "Upgrade agnostic-ai to the latest release.",
		Long: `upgrade detects how the running agnostic-ai binary was installed
(Homebrew, ` + "`go install`" + `, Scoop, winget, npm, or a raw prebuilt
binary) and upgrades it to the latest release. Package-manager installs
use their package manager. Standalone binaries on macOS and Linux are
downloaded, verified against release checksums, and replaced in place.

Pass --version to install one specific release instead of the latest,
which is how a project pinned to an older version gets there in one
command. Downgrades are allowed, and standalone binary installs only: a
package-manager install pins through its own package manager.

Pass --check to show install details without changing anything. --run
is still accepted for compatibility.`,
		Example: `  # Upgrade to the latest release
  agnostic-ai upgrade

  # Install the release this project pins, even an older one
  agnostic-ai upgrade --version v0.56.1

  # Diagnose install location + PATH shadowing
  agnostic-ai upgrade --check`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			want := ""
			if cmd.Flags().Changed("version") {
				v, err := normalizeReleaseVersion(targetVersion)
				if err != nil {
					return err
				}
				want = v
			}
			return runUpgradeWithDeps(cmd.OutOrStdout(), checkOnly, want, cmd.Root().Version, deps)
		},
	}
	cmd.Flags().Bool("run", false, "Run upgrade (now the default)")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Print detection details and exit without running")
	cmd.Flags().StringVar(&targetVersion, "version", "", "Install this release instead of the latest (e.g. v0.56.1)")
	return cmd
}

// releaseVersionRE matches the release tags this repo publishes. The
// value is interpolated into a release asset URL, so anything else is
// refused here rather than downloaded as a 404 the user has to decode.
var releaseVersionRE = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// normalizeReleaseVersion validates a --version tag and returns it in the
// prefix-less form the release URL builder and version comparisons use.
func normalizeReleaseVersion(tag string) (string, error) {
	trimmed := strings.TrimSpace(tag)
	if !releaseVersionRE.MatchString(trimmed) {
		return "", fmt.Errorf("upgrade: %q is not a release version; pass one like v0.56.1 (see %s)", tag, releasesHTMLURL)
	}
	return strings.TrimPrefix(trimmed, "v"), nil
}

func runUpgrade(out io.Writer, checkOnly bool, currentVersion string) error {
	return runUpgradeWithDeps(out, checkOnly, "", currentVersion, defaultUpgradeDeps())
}

// runUpgradeWithDeps upgrades to the latest release, or to requestedVersion
// when the caller named one. A named version is already normalized and
// validated by normalizeReleaseVersion.
func runUpgradeWithDeps(out io.Writer, checkOnly bool, requestedVersion, currentVersion string, deps upgradeDeps) error {
	info, err := deps.detect(currentVersion)
	if err != nil {
		return err
	}
	printUpgradeInfo(out, info, requestedVersion)
	if checkOnly {
		return nil
	}
	if requestedVersion != "" {
		return installRequestedVersion(out, info, requestedVersion, deps)
	}
	if info.Latest != "" && info.Version != "" && versionsEqual(info.Version, info.Latest) {
		_, _ = fmt.Fprintf(out, "\nAlready on latest (%s). Nothing to do.\n", info.Latest)
		return nil
	}
	if info.Method == installBinary {
		if info.Latest == "" {
			if info.LatestError != nil {
				return fmt.Errorf("upgrade: check latest release: %w", info.LatestError)
			}
			return fmt.Errorf("upgrade: latest release tag unavailable")
		}
		if err := downloadRelease(out, info.Path, info.Latest, deps); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Upgraded %s to %s.\n", info.Path, info.Latest)
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
	if err := deps.run(out, info.Command); err != nil {
		return fmt.Errorf("upgrade: run %s: %w", info.Command, err)
	}
	return nil
}

// installRequestedVersion installs one named release. Unlike the latest
// path it allows a downgrade, which is the case that motivated the flag:
// a project pinned behind the installed CLI had no way back other than
// re-running the installer by hand.
func installRequestedVersion(out io.Writer, info upgradeInfo, requestedVersion string, deps upgradeDeps) error {
	if info.Version != "" && versionsEqual(info.Version, requestedVersion) {
		_, _ = fmt.Fprintf(out, "\nAlready on %s. Nothing to do.\n", requestedVersion)
		return nil
	}
	if info.Method != installBinary {
		return fmt.Errorf("upgrade: --version needs a standalone binary install, and this one is %s; pin the version through %s itself, or install a standalone binary from %s",
			info.Method, info.Method, releasesHTMLURL)
	}
	if err := downloadRelease(out, info.Path, requestedVersion, deps); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Installed %s at %s.\n", requestedVersion, info.Path)
	return nil
}

// downloadRelease fetches one release archive and replaces the binary at
// path with it. The caller reports the outcome, since upgrading to latest
// and installing a named version say different things about the same work.
func downloadRelease(out io.Writer, path, version string, deps upgradeDeps) error {
	_, _ = fmt.Fprintf(out, "\nDownloading and installing %s...\n", version)
	if err := deps.install(path, version, releasesBaseURL, &http.Client{Timeout: time.Minute}); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}
	return nil
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
	} else {
		info.LatestError = err
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
// binary that wins a PATH lookup ahead of the running one, so a
// non-empty result means `agnostic-ai` really does resolve to another
// copy. Paths are resolved through symlinks so a symlink and its
// target collapse to one entry.
//
// Order is the whole point. A copy sitting after the running binary
// never wins a lookup and shadows nothing, so it is not reported. This
// used to return every other copy: a Homebrew user with a stale
// ~/.local/bin file was told to delete it "so `agnostic-ai --version`
// resolves to the upgraded binary" when /opt/homebrew/bin came first
// and it already did.
//
// A running binary that is not on PATH at all stops nothing, so every
// copy found is reported: one of them is what the name resolves to.
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
		if resolved == selfResolved {
			// Reached the running binary; anything further down PATH
			// loses the lookup to it.
			break
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
	tag := strings.TrimPrefix(strings.TrimSpace(body.TagName), "v")
	if tag == "" {
		return "", fmt.Errorf("github releases api: latest release has no tag")
	}
	return tag, nil
}

func versionsEqual(a, b string) bool {
	return strings.TrimPrefix(strings.TrimSpace(a), "v") == strings.TrimPrefix(strings.TrimSpace(b), "v")
}

func execShell(out io.Writer, command string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// printUpgradeInfo renders the install summary. requested is the version
// named by --version, printed next to the detected ones so the three
// read together, and empty on the plain latest path.
func printUpgradeInfo(out io.Writer, info upgradeInfo, requested string) {
	_, _ = fmt.Fprintf(out, "Install method: %s\n", info.Method)
	_, _ = fmt.Fprintf(out, "Binary:         %s\n", info.Path)
	if info.Version != "" {
		_, _ = fmt.Fprintf(out, "Installed:      %s\n", info.Version)
	}
	if info.Latest != "" {
		_, _ = fmt.Fprintf(out, "Latest:         %s\n", info.Latest)
	}
	if requested != "" {
		_, _ = fmt.Fprintf(out, "Requested:      %s\n", requested)
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
