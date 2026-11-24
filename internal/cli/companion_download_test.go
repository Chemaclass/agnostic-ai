package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func testCompanionArchive(t *testing.T, program []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: companionName, Mode: 0o755, Size: int64(len(program))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(program); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// companionRelease serves release v1.2.3 with the given checksums.txt and
// the arm64 companion archive; a nil checksums answers 404.
func companionRelease(t *testing.T, checksums []byte, archive []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.2.3/checksums.txt":
			if checksums == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(checksums)
		case "/v1.2.3/agnostic-ai-companion_darwin_arm64.tar.gz":
			if archive == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func checksumLine(archive []byte, asset string) []byte {
	return fmt.Appendf(nil, "%x  %s\n", sha256.Sum256(archive), asset)
}

func TestDownloadCompanion_WritesTheVerifiedProgram(t *testing.T) {
	clearVerifyAttestation(t)
	archive := testCompanionArchive(t, []byte("the released companion"))
	server := companionRelease(t, checksumLine(archive, "agnostic-ai-companion_darwin_arm64.tar.gz"), archive)
	into := t.TempDir()
	var verified []string
	verify := func(path, version string) error {
		verified = append(verified, version)
		return nil
	}

	path, err := downloadCompanion(context.Background(), io.Discard, server.Client(), server.URL, "1.2.3", "arm64", into, verify)

	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(into, companionName) {
		t.Errorf("path = %q", path)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "the released companion" {
		t.Errorf("program = %q", raw)
	}
	if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the program cannot run: %v %v", info, err)
	}
	if !slices.Equal(verified, []string{"1.2.3"}) {
		t.Errorf("provenance checked for %v", verified)
	}
}

func TestDownloadCompanion_SaysWhenTheReleaseHasNoBuiltCompanion(t *testing.T) {
	clearVerifyAttestation(t)
	archive := testCompanionArchive(t, []byte("the released companion"))
	for name, tc := range map[string]struct {
		checksums []byte
		archive   []byte
	}{
		"no checksums.txt":           {nil, archive},
		"not in checksums.txt":       {checksumLine(archive, "agnostic-ai_darwin_arm64.tar.gz"), archive},
		"listed but not on the page": {checksumLine(archive, "agnostic-ai-companion_darwin_arm64.tar.gz"), nil},
	} {
		t.Run(name, func(t *testing.T) {
			server := companionRelease(t, tc.checksums, tc.archive)

			_, err := downloadCompanion(context.Background(), io.Discard, server.Client(), server.URL, "1.2.3", "arm64", t.TempDir(), skipProvenance)

			if !errors.Is(err, errNoPrebuiltCompanion) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestDownloadCompanion_RefusesAnArchiveThatDoesNotVerify(t *testing.T) {
	clearVerifyAttestation(t)
	archive := testCompanionArchive(t, []byte("the released companion"))
	asset := "agnostic-ai-companion_darwin_arm64.tar.gz"
	for name, tc := range map[string]struct {
		checksums []byte
		verify    provenanceCheck
		want      string
	}{
		"checksum mismatch": {fmt.Appendf(nil, "%064x  %s\n", 0, asset), skipProvenance, "checksum mismatch"},
		"provenance refused": {checksumLine(archive, asset), func(string, string) error {
			return errors.New("build provenance did not verify: no attestation")
		}, "build provenance did not verify"},
	} {
		t.Run(name, func(t *testing.T) {
			server := companionRelease(t, tc.checksums, archive)
			into := t.TempDir()

			_, err := downloadCompanion(context.Background(), io.Discard, server.Client(), server.URL, "1.2.3", "arm64", into, tc.verify)

			if err == nil || errors.Is(err, errNoPrebuiltCompanion) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
			if left, _ := os.ReadDir(into); len(left) != 0 {
				t.Errorf("wrote %v from an archive that did not verify", left)
			}
		})
	}
}

// releasedMacBuild is macBuild from an agnostic-ai built at its release tag.
func releasedMacBuild() companionBuild {
	build := macBuild()
	build.module = "v" + build.version
	return build
}

// downloadedTools are tools whose download writes a released companion,
// on a machine that has nothing a build needs.
func downloadedTools(t *testing.T) companionTools {
	t.Helper()
	tools := builtTools(t)
	tools.succeeds = func(_ context.Context, name string, _ ...string) bool {
		t.Errorf("checked for %s although the companion was downloaded", name)
		return false
	}
	tools.build = func(context.Context, string, []string, func(string), ...string) error {
		t.Error("built although the companion was downloaded")
		return errors.New("no build here")
	}
	tools.download = func(_ context.Context, _ io.Writer, version, into string) (string, error) {
		path := filepath.Join(into, companionName)
		return path, os.WriteFile(path, []byte("released for "+version), 0o700)
	}
	return tools
}

func TestInstallCompanion_InstallsTheReleasedCompanionOnMacOSWithoutBuilding(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, downloadedTools(t), dir, releasedMacBuild(), nil)

	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(companionFile(dir, "darwin")); string(raw) != "released for 1.2.3" {
		t.Errorf("installed program = %q", raw)
	}
	app := filepath.Join(dir, "companion", "agnostic-ai companion.app")
	want := "Downloading the companion for agnostic-ai 1.2.3.\n" +
		"✓ Downloaded and verified the companion\n" +
		"✓ Installed " + app + "\n" +
		"Start it with: agnostic-ai companion\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestInstallCompanion_BuildsWhenTheReleaseHasNoBuiltCompanion(t *testing.T) {
	dir := t.TempDir()
	tools := builtTools(t)
	tools.download = func(context.Context, io.Writer, string, string) (string, error) {
		return "", fmt.Errorf("checksums.txt: %w", errNoPrebuiltCompanion)
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, releasedMacBuild(), nil)

	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(companionFile(dir, "darwin")); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	if !strings.HasPrefix(out.String(), "Downloading the companion for agnostic-ai 1.2.3.\n"+
		"! This release ships no built companion for this Mac, so it is built here.\n"+
		"Building the companion for agnostic-ai 1.2.3 on this machine.\n") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestInstallCompanion_StopsWhenTheDownloadDoesNotVerify(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "darwin")
	tools := downloadedTools(t)
	tools.download = func(context.Context, io.Writer, string, string) (string, error) {
		return "", errors.New("agnostic-ai-companion_darwin_arm64.tar.gz: checksum mismatch")
	}

	err := installCompanion(context.Background(), io.Discard, false, tools, dir, releasedMacBuild(), nil)

	if err == nil || err.Error() != "download the companion: agnostic-ai-companion_darwin_arm64.tar.gz: checksum mismatch" {
		t.Errorf("err = %v", err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
}

func TestInstallCompanion_DownloadsOnlyAReleaseForMacOSFetchedFromItsCommit(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module companion\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, build := range map[string]companionBuild{
		"a checkout":             {goos: "darwin", version: "1.2.3", source: source, module: "v1.2.3"},
		"Linux":                  {goos: "linux", version: "1.2.3", revision: "abc123", module: "v1.2.3"},
		"Windows":                {goos: "windows", version: "1.2.3", revision: "abc123", module: "v1.2.3"},
		"a commit after the tag": {goos: "darwin", version: "1.2.3", revision: "abc123", module: "v1.2.4-0.20261116213956-abc123abc123"},
		"changes not committed":  {goos: "darwin", version: "1.2.3", revision: "abc123", module: "v1.2.3+dirty"},
		"no version stamped":     {goos: "darwin", version: "1.2.3", revision: "abc123", module: "(devel)"},
	} {
		t.Run(name, func(t *testing.T) {
			tools := builtTools(t)
			tools.download = func(context.Context, io.Writer, string, string) (string, error) {
				t.Error("downloaded a companion")
				return "", errNoPrebuiltCompanion
			}
			tools.build = func(_ context.Context, _ string, env []string, _ func(string), args ...string) error {
				into := ""
				for i, arg := range args {
					if arg == "-o" {
						into = filepath.Dir(args[i+1])
					}
				}
				// As in go build, -o wins over a GOBIN the environment may already hold.
				if into == "" {
					for _, entry := range env {
						if gobin, ok := strings.CutPrefix(entry, "GOBIN="); ok {
							into = gobin
						}
					}
				}
				return os.WriteFile(filepath.Join(into, companionProgramName(build.goos)), []byte("built"), 0o700)
			}

			if err := installCompanion(context.Background(), io.Discard, false, tools, t.TempDir(), build, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInstallCompanion_NamesHowToSkipAProvenanceCheckThatFails(t *testing.T) {
	t.Setenv(verifyAttestationEnv, "")
	tools := downloadedTools(t)
	tools.download = func(context.Context, io.Writer, string, string) (string, error) {
		return "", fmt.Errorf("agnostic-ai-companion_darwin_arm64.tar.gz: %w: gh crashed", errProvenanceFailed)
	}

	err := installCompanion(context.Background(), io.Discard, false, tools, t.TempDir(), releasedMacBuild(), nil)

	want := "download the companion: agnostic-ai-companion_darwin_arm64.tar.gz: build provenance did not verify: gh crashed. If gh failed rather than the archive, AGNOSTIC_AI_VERIFY_ATTESTATION=0 installs it on the checksum alone"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}
