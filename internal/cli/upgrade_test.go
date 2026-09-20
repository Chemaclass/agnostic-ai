package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestDetectInstallMethod_Homebrew(t *testing.T) {
	cases := []string{
		"/opt/homebrew/bin/agnostic-ai",
		"/opt/homebrew/Caskroom/agnostic-ai/0.22.0/agnostic-ai",
		"/usr/local/Cellar/agnostic-ai/0.22.0/bin/agnostic-ai",
		"/home/linuxbrew/.linuxbrew/bin/agnostic-ai",
	}
	for _, p := range cases {
		if got := detectInstallMethod(p); got != installHomebrew {
			t.Errorf("detectInstallMethod(%q) = %v, want homebrew", p, got)
		}
	}
}

func TestDetectInstallMethod_GoInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path semantics differ on windows")
	}
	tmp := t.TempDir()
	gobin := filepath.Join(tmp, "gobin")
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOBIN", gobin)
	t.Setenv("GOPATH", "")

	exe := filepath.Join(gobin, "agnostic-ai")
	if got := detectInstallMethod(exe); got != installGoInstall {
		t.Errorf("GOBIN install: got %v, want go install", got)
	}

	t.Setenv("GOBIN", "")
	gopath := filepath.Join(tmp, "gopath")
	if err := os.MkdirAll(filepath.Join(gopath, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPATH", gopath)
	exe2 := filepath.Join(gopath, "bin", "agnostic-ai")
	if got := detectInstallMethod(exe2); got != installGoInstall {
		t.Errorf("GOPATH/bin install: got %v, want go install", got)
	}
}

func TestDetectInstallMethod_WindowsPackageManagers(t *testing.T) {
	cases := map[string]installMethod{
		`C:\Users\me\scoop\shims\agnostic-ai.exe`:                                          installScoop,
		`C:\Users\me\scoop\apps\agnostic-ai\0.45.0\agnostic-ai.exe`:                        installScoop,
		`C:\Users\me\Scoop\Shims\agnostic-ai.exe`:                                          installScoop,
		`C:\Users\me\AppData\Local\Microsoft\WinGet\Links\agnostic-ai.exe`:                 installWinget,
		`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\Chemaclass.agnostic-ai\x.exe`: installWinget,
		`/usr/local/lib/node_modules/agnostic-ai/bin/agnostic-ai`:                          installNPM,
	}
	for p, want := range cases {
		if got := detectInstallMethod(p); got != want {
			t.Errorf("detectInstallMethod(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestDetectInstallMethod_Binary(t *testing.T) {
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "/nowhere")
	// HOME guarded too: $HOME/go/bin should not match an unrelated path.
	t.Setenv("HOME", "/nowhere")
	if got := detectInstallMethod("/usr/local/bin/agnostic-ai"); got != installBinary {
		t.Errorf("raw binary: got %v, want binary", got)
	}
}

func TestUpgradeCommandFor(t *testing.T) {
	cases := map[installMethod]string{
		installHomebrew:  "brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai",
		installGoInstall: "go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest",
		installScoop:     "scoop update agnostic-ai",
		installWinget:    "winget upgrade Chemaclass.agnostic-ai",
		installNPM:       "npm install -g agnostic-ai@latest",
		installBinary:    "",
		installUnknown:   "",
	}
	for m, want := range cases {
		if got := upgradeCommandFor(m); got != want {
			t.Errorf("upgradeCommandFor(%v) = %q, want %q", m, got, want)
		}
	}
}

func TestInstallMethodString(t *testing.T) {
	cases := map[installMethod]string{
		installHomebrew:  "homebrew",
		installGoInstall: "go install",
		installScoop:     "scoop",
		installWinget:    "winget",
		installNPM:       "npm",
		installBinary:    "binary",
		installUnknown:   "unknown",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("installMethod(%d).String() = %q, want %q", m, got, want)
		}
	}
}

// redirectServer stands in for github.com/<owner>/<repo>/releases/latest:
// it answers with one 302 and never serves a body, exactly as GitHub does.
func redirectServer(t *testing.T, status int, location string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchLatestReleaseFrom_ReadsTheTagFromTheRedirect(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Location", "https://github.com/Chemaclass/agnostic-ai/releases/tag/v9.9.9")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	tag, err := fetchLatestReleaseFrom(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("fetchLatestReleaseFrom: %v", err)
	}
	if tag != "9.9.9" {
		t.Errorf("tag = %q, want %q", tag, "9.9.9")
	}
	if gotUA == "" {
		t.Errorf("User-Agent header was empty")
	}
}

// The redirect must not be followed: the tag lives in the Location header,
// and the page it points at is a few hundred kilobytes of HTML.
func TestFetchLatestReleaseFrom_DoesNotFollowTheRedirect(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/releases/tag/v1.2.3" {
			_, _ = w.Write([]byte("<html>release page</html>"))
			return
		}
		w.Header().Set("Location", "/releases/tag/v1.2.3")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	tag, err := fetchLatestReleaseFrom(srv.URL+"/releases/latest", time.Second)
	if err != nil {
		t.Fatalf("fetchLatestReleaseFrom: %v", err)
	}
	if tag != "1.2.3" {
		t.Errorf("tag = %q, want %q", tag, "1.2.3")
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1 (the redirect was followed)", hits)
	}
}

func TestFetchLatestReleaseFrom_NamesRateLimiting(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		srv := redirectServer(t, status, "")
		_, err := fetchLatestReleaseFrom(srv.URL, time.Second)
		if !errors.Is(err, errReleaseRateLimited) {
			t.Fatalf("status %d: err = %v, want errReleaseRateLimited", status, err)
		}
		if !strings.Contains(err.Error(), "rate") {
			t.Errorf("status %d: %q does not say it was rate limited", status, err)
		}
		if strings.Contains(err.Error(), "403 Forbidden") {
			t.Errorf("status %d: %q still leaks a bare HTTP status", status, err)
		}
	}
}

func TestFetchLatestReleaseFrom_NamesAnAbsentRelease(t *testing.T) {
	srv := redirectServer(t, http.StatusNotFound, "")
	_, err := fetchLatestReleaseFrom(srv.URL, time.Second)
	if !errors.Is(err, errNoPublishedRelease) {
		t.Fatalf("err = %v, want errNoPublishedRelease", err)
	}
	if strings.Contains(err.Error(), "rate") {
		t.Errorf("%q blames rate limiting for a missing release", err)
	}
}

// A 302 that lands anywhere but a tag page means there is nothing tagged
// yet, which reads the same to the user as an absent release.
func TestFetchLatestReleaseFrom_RejectsARedirectWithoutATag(t *testing.T) {
	srv := redirectServer(t, http.StatusFound, "https://github.com/Chemaclass/agnostic-ai/releases")
	if _, err := fetchLatestReleaseFrom(srv.URL, time.Second); !errors.Is(err, errNoPublishedRelease) {
		t.Fatalf("err = %v, want errNoPublishedRelease", err)
	}
}

func TestFetchLatestReleaseFrom_ReportsAnUnexpectedStatus(t *testing.T) {
	srv := redirectServer(t, http.StatusInternalServerError, "")
	_, err := fetchLatestReleaseFrom(srv.URL, time.Second)
	if err == nil {
		t.Fatal("expected an error for 500")
	}
	if errors.Is(err, errReleaseRateLimited) || errors.Is(err, errNoPublishedRelease) {
		t.Errorf("%q misclassifies a server error", err)
	}
	if !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("%q does not name the URL it failed on", err)
	}
}

// The API endpoint is the whole bug: 60 unauthenticated requests per hour
// per IP. Nothing may resolve the latest tag through it.
func TestLatestReleaseURLAvoidsTheRateLimitedAPI(t *testing.T) {
	if strings.Contains(releasesLatestURL, "api.github.com") {
		t.Errorf("releasesLatestURL = %q, want the github.com redirect", releasesLatestURL)
	}
	if want := "https://github.com/Chemaclass/agnostic-ai/releases/latest"; releasesLatestURL != want {
		t.Errorf("releasesLatestURL = %q, want %q", releasesLatestURL, want)
	}
}

func TestTagFromReleaseLocation(t *testing.T) {
	cases := map[string]string{
		"https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.62.0": "0.62.0",
		"/releases/tag/v1.0.0-rc.1":                                      "1.0.0-rc.1",
		"https://github.com/Chemaclass/agnostic-ai/releases":             "",
		"": "",
	}
	for location, want := range cases {
		if got := tagFromReleaseLocation(location); got != want {
			t.Errorf("tagFromReleaseLocation(%q) = %q, want %q", location, got, want)
		}
	}
}

// A failed resolution used to vanish: printUpgradeInfo simply omitted the
// "Latest" line, so `upgrade --check` looked normal on a rate-limited
// network. Say why instead.
func TestPrintUpgradeInfo_SaysWhyTheLatestReleaseIsUnknown(t *testing.T) {
	var buf bytes.Buffer
	printUpgradeInfo(&buf, upgradeInfo{
		Method:      installBinary,
		Path:        "/usr/local/bin/agnostic-ai",
		Version:     "0.62.0",
		LatestError: fmt.Errorf("%s: %w", releasesLatestURL, errReleaseRateLimited),
	}, "")

	out := buf.String()
	if !strings.Contains(out, "Latest:") {
		t.Fatalf("output has no Latest line:\n%s", out)
	}
	if !strings.Contains(out, "rate limiting") {
		t.Errorf("output does not name rate limiting:\n%s", out)
	}
}

func TestVersionsEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.22.0", "v0.22.0", true},
		{" 0.22.0 ", "0.22.0", true},
		{"0.22.0", "0.21.0", false},
		{"", "", true},
	}
	for _, c := range cases {
		if got := versionsEqual(c.a, c.b); got != c.want {
			t.Errorf("versionsEqual(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestOtherInstancesOnPATH_OnlyReportsEarlierCopies pins what a shadow
// is. A copy that sits AFTER the running binary on PATH never wins a
// lookup, so it shadows nothing and must not be reported.
//
// This used to report every other copy regardless of order, which told
// a Homebrew user on a healthy install to delete a stale ~/.local/bin
// file "so agnostic-ai --version resolves to the upgraded binary" when
// it already did. The advice was to remove a file that changed nothing.
func TestOtherInstancesOnPATH_OnlyReportsEarlierCopies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH separator + exe extension differ on windows")
	}
	tmp := t.TempDir()
	dirA := filepath.Join(tmp, "a")
	dirB := filepath.Join(tmp, "b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inA := filepath.Join(dirA, "agnostic-ai")
	inB := filepath.Join(dirB, "agnostic-ai")
	if err := os.WriteFile(inA, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inB, []byte("y"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dirA+string(os.PathListSeparator)+dirB)

	t.Run("a copy earlier on PATH is a shadow", func(t *testing.T) {
		got := otherInstancesOnPATH(inB)
		if len(got) != 1 || got[0] != inA {
			t.Errorf("shadows = %v, want [%s]", got, inA)
		}
	})

	t.Run("a copy later on PATH is not", func(t *testing.T) {
		if got := otherInstancesOnPATH(inA); len(got) != 0 {
			t.Errorf("shadows = %v, want none: %s wins the lookup already", got, inA)
		}
	})
}

// TestOtherInstancesOnPATH_SelfOffPATH covers the case where the
// running binary is not on PATH at all. Then whatever is on PATH does
// win the lookup, so it is a genuine shadow.
func TestOtherInstancesOnPATH_SelfOffPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH semantics differ on windows")
	}
	tmp := t.TempDir()
	onPath := filepath.Join(tmp, "agnostic-ai")
	if err := os.WriteFile(onPath, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(t.TempDir(), "agnostic-ai")
	if err := os.WriteFile(elsewhere, []byte("y"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	got := otherInstancesOnPATH(elsewhere)
	if len(got) != 1 || got[0] != onPath {
		t.Errorf("shadows = %v, want [%s]", got, onPath)
	}
}

func TestOtherInstancesOnPATH_NoShadow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH semantics differ on windows")
	}
	tmp := t.TempDir()
	self := filepath.Join(tmp, "agnostic-ai")
	if err := os.WriteFile(self, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	if got := otherInstancesOnPATH(self); len(got) != 0 {
		t.Errorf("expected no shadows, got %v", got)
	}
}

func TestPrintUpgradeInfo_RendersAllFields(t *testing.T) {
	var buf bytes.Buffer
	printUpgradeInfo(&buf, upgradeInfo{
		Path:    "/opt/homebrew/bin/agnostic-ai",
		Method:  installHomebrew,
		Version: "0.20.0",
		Latest:  "0.22.0",
		Shadows: []string{"/usr/local/bin/agnostic-ai"},
		Notes:   []string{"shadow detected"},
	}, "")
	out := buf.String()
	for _, want := range []string{
		"Install method: homebrew",
		"Binary:         /opt/homebrew/bin/agnostic-ai",
		"Installed:      0.20.0",
		"Latest:         0.22.0",
		"PATH shadows",
		"/usr/local/bin/agnostic-ai",
		"shadow detected",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestRunUpgrade_CheckOnlyPrintsAndReturns(t *testing.T) {
	var buf bytes.Buffer
	if err := runUpgrade(&buf, true, "0.22.0"); err != nil {
		t.Fatalf("check-only: %v", err)
	}
	if !strings.Contains(buf.String(), "Install method:") {
		t.Errorf("expected install method line, got:\n%s", buf.String())
	}
}

func TestUpgradeCommand_ExecutesAndChecksBothInstallPaths(t *testing.T) {
	for _, method := range []installMethod{installGoInstall, installBinary} {
		for _, tc := range []struct {
			name, command string
			wantRuns      int
		}{
			{"upgrade defaults to run", "upgrade", 1},
			{"update alias defaults to run", "update", 1},
			{"legacy run flag executes", "upgrade --run", 1},
			{"check suppresses update", "upgrade --check", 0},
			{"check overrides run", "update --run --check", 0},
		} {
			t.Run(method.String()+"/"+tc.name, func(t *testing.T) {
				var runs int
				deps := upgradeDeps{
					detect: func(version string) (upgradeInfo, error) {
						if version != "1.0.0" {
							t.Errorf("version = %q, want 1.0.0", version)
						}
						return upgradeInfo{Path: "/tmp/agnostic-ai", Method: method, Version: version, Latest: "2.0.0", Command: upgradeCommandFor(method)}, nil
					},
					run: func(_ io.Writer, command string) error {
						runs++
						if command != upgradeCommandFor(method) {
							t.Errorf("command = %q", command)
						}
						return nil
					},
					install: func(path, version, baseURL string, _ *http.Client) error {
						runs++
						if path != "/tmp/agnostic-ai" || version != "2.0.0" || baseURL != releasesBaseURL {
							t.Errorf("install arguments = %q, %q, %q", path, version, baseURL)
						}
						return nil
					},
				}
				root := &cobra.Command{Use: "agnostic-ai", Version: "1.0.0", SilenceUsage: true}
				root.AddCommand(newUpgradeCmdWithDeps(deps))
				root.SetArgs(strings.Fields(tc.command))
				root.SetOut(&bytes.Buffer{})
				if err := root.Execute(); err != nil {
					t.Fatalf("execute %q: %v", tc.command, err)
				}
				if runs != tc.wantRuns {
					t.Errorf("update calls = %d, want %d", runs, tc.wantRuns)
				}
			})
		}
	}
}

func TestStandalonePlatformError_WindowsInstallerGuidance(t *testing.T) {
	err := standalonePlatformError("windows")
	if err == nil || !strings.Contains(err.Error(), "irm "+windowsInstallerURL+" | iex") {
		t.Errorf("Windows guidance = %v, want runnable installer command", err)
	}
}

func TestHomebrewBinaryOK_MissingPathIsOK(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "agnostic-ai")
	ok, hint := homebrewBinaryOK(missing)
	if !ok || hint != "" {
		t.Errorf("missing path: ok=%v hint=%q, want ok=true hint=\"\"", ok, hint)
	}
}

func TestHomebrewBinaryOK_RegularFileCollides(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "agnostic-ai")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	ok, hint := homebrewBinaryOK(bin)
	if ok {
		t.Fatalf("regular file should not be OK")
	}
	for _, want := range []string{"regular file", "rm " + bin, "brew install --cask"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\nfull:\n%s", want, hint)
		}
	}
}

func TestHomebrewBinaryOK_SymlinkIntoCaskroom(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	tmp := t.TempDir()
	caskroom := filepath.Join(tmp, "Caskroom", "agnostic-ai", "0.26.0")
	if err := os.MkdirAll(caskroom, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(caskroom, "agnostic-ai")
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "bin", "agnostic-ai")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, bin); err != nil {
		t.Fatal(err)
	}
	ok, hint := homebrewBinaryOK(bin)
	if !ok {
		t.Errorf("caskroom symlink should be OK, got hint:\n%s", hint)
	}
}

func TestHomebrewBinaryOK_SymlinkOutsideBrew(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	tmp := t.TempDir()
	target := filepath.Join(tmp, "elsewhere", "agnostic-ai")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "bin", "agnostic-ai")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, bin); err != nil {
		t.Fatal(err)
	}
	ok, hint := homebrewBinaryOK(bin)
	if ok {
		t.Fatalf("symlink outside brew prefix should not be OK")
	}
	if !strings.Contains(hint, "outside the brew prefix") {
		t.Errorf("hint missing prefix reason:\n%s", hint)
	}
}

func TestHomebrewBinaryOK_DanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "agnostic-ai")
	if err := os.Symlink(filepath.Join(tmp, "Caskroom", "missing"), bin); err != nil {
		t.Fatal(err)
	}
	ok, hint := homebrewBinaryOK(bin)
	if ok {
		t.Fatalf("dangling symlink should not be OK")
	}
	if !strings.Contains(hint, "dangling symlink") {
		t.Errorf("hint missing dangling reason:\n%s", hint)
	}
}

func TestUpgradeCmd_RegistersOnRoot(t *testing.T) {
	root := NewRootCmd("test")
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "upgrade" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("upgrade command not registered on root")
	}
}

func TestUpdateAlias_ResolvesToUpgrade(t *testing.T) {
	root := NewRootCmd("test")
	cmd, _, err := root.Find([]string{"update"})
	if err != nil {
		t.Fatalf("find update command: %v", err)
	}
	if cmd.Name() != "upgrade" {
		t.Errorf("update resolves to %q, want upgrade", cmd.Name())
	}
	for _, name := range []string{"run", "check"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("update missing --%s flag", name)
		}
	}
}
