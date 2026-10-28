package cli

import (
	"bytes"
	"crypto/sha256"
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
)

func skipProvenance(string, string) error { return nil }

// clearVerifyAttestation keeps a value exported in the developer's shell
// from changing what these tests check.
func clearVerifyAttestation(t *testing.T) {
	t.Helper()
	t.Setenv(verifyAttestationEnv, "")
}

func provenanceNotCalled(t *testing.T) provenanceCheck {
	return func(string, string) error {
		t.Error("checked build provenance where it must be skipped")
		return nil
	}
}

// serveRelease serves one release's checksums.txt and archive for this
// platform and returns the archive bytes.
func serveRelease(t *testing.T, version, binaryVersion string) (*httptest.Server, []byte) {
	t.Helper()
	clearVerifyAttestation(t)
	asset := fmt.Sprintf("%s_%s_%s.tar.gz", binaryName, runtime.GOOS, runtime.GOARCH)
	archive := testReleaseArchive(t, []byte("#!/bin/sh\nprintf 'agnostic-ai version "+binaryVersion+"\\n'\n"))
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v" + version + "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%x  %s\n", sum, asset)
		case "/v" + version + "/" + asset:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, archive
}

func installedBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("standalone binary updates are unsupported on Windows")
	}
	path := filepath.Join(t.TempDir(), binaryName)
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstallVerifiedRelease_ChecksTheArchiveProvenance(t *testing.T) {
	server, archive := serveRelease(t, "0.90.0", "0.90.0")
	path := installedBinary(t)
	var checked []string
	verify := func(archivePath, version string) error {
		got, err := os.ReadFile(archivePath)
		if err != nil || !bytes.Equal(got, archive) {
			t.Errorf("provenance check read %q, not the downloaded archive: %v", archivePath, err)
		}
		checked = append(checked, version)
		return nil
	}
	var out bytes.Buffer
	if err := installVerifiedRelease(&out, path, "0.90.0", server.URL, server.Client(), verify); err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(checked) != 1 || checked[0] != "0.90.0" {
		t.Errorf("provenance checked for %v, want [0.90.0]", checked)
	}
	if !strings.Contains(out.String(), "Build provenance verified") {
		t.Errorf("output does not say provenance was verified:\n%s", out.String())
	}
}

func TestInstallVerifiedRelease_KeepsTheOldBinaryWhenProvenanceFails(t *testing.T) {
	server, _ := serveRelease(t, "0.90.0", "0.90.0")
	path := installedBinary(t)
	verify := func(string, string) error {
		return errors.New("build provenance did not verify: no attestations found")
	}
	err := installVerifiedRelease(&bytes.Buffer{}, path, "0.90.0", server.URL, server.Client(), verify)
	if err == nil || !strings.Contains(err.Error(), "did not verify") {
		t.Fatalf("error = %v, want a provenance failure", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary replaced after a failed provenance check")
	}
}

func TestInstallVerifiedRelease_SaysSoWhenProvenanceCannotBeChecked(t *testing.T) {
	server, _ := serveRelease(t, "0.90.0", "0.90.0")
	path := installedBinary(t)
	verify := func(string, string) error {
		return provenanceUnchecked{"the GitHub CLI (gh) is not installed"}
	}
	var out bytes.Buffer
	if err := installVerifiedRelease(&out, path, "0.90.0", server.URL, server.Client(), verify); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "Build provenance not checked: the GitHub CLI (gh) is not installed") {
		t.Errorf("output does not say provenance went unchecked:\n%s", out.String())
	}
}

func TestInstallVerifiedRelease_SkipsReleasesBeforeProvenanceExisted(t *testing.T) {
	server, _ := serveRelease(t, "0.74.0", "0.74.0")
	path := installedBinary(t)
	var out bytes.Buffer
	if err := installVerifiedRelease(&out, path, "0.74.0", server.URL, server.Client(), provenanceNotCalled(t)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "v0.75.0") {
		t.Errorf("output does not explain why provenance was skipped:\n%s", out.String())
	}
}

func TestReleaseHasProvenance(t *testing.T) {
	for version, want := range map[string]bool{
		"0.74.0": false, "0.74.9": false, "0.75.0": true, "0.75.0-rc.1": true, "0.89.0": true, "1.0.0": true, "0.100.0": true,
	} {
		if got := releaseHasProvenance(version); got != want {
			t.Errorf("releaseHasProvenance(%q) = %v, want %v", version, got, want)
		}
	}
}

// fakeGH puts a gh on PATH that logs its arguments, prints stderr, and
// exits with code.
func fakeGH(t *testing.T, code int, stderr string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" > %q\necho %q >&2\nexit %d\n", log, stderr, code)
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func TestGHProvenance_PinsTheReleaseWorkflowAndTag(t *testing.T) {
	log := fakeGH(t, 0, "")
	if err := ghProvenance("/tmp/archive.tar.gz", "0.90.0"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	args, _ := os.ReadFile(log)
	want := "attestation verify /tmp/archive.tar.gz --hostname github.com --repo Chemaclass/agnostic-ai --signer-workflow Chemaclass/agnostic-ai/.github/workflows/release.yml --source-ref refs/tags/v0.90.0 --deny-self-hosted-runners\n"
	if string(args) != want {
		t.Errorf("gh args = %q, want %q", args, want)
	}
}

func TestGHProvenance_FailsOnARejectedAttestation(t *testing.T) {
	fakeGH(t, 1, "gh says no")
	err := ghProvenance("/tmp/archive.tar.gz", "0.90.0")
	if err == nil || errors.As(err, new(provenanceUnchecked)) || !strings.Contains(err.Error(), "gh says no") {
		t.Fatalf("error = %v, want a verification failure carrying gh's message", err)
	}
}

func TestGHProvenance_TreatsALoggedOutGHAsUnchecked(t *testing.T) {
	fakeGH(t, 4, "To get started with GitHub CLI, please run:  gh auth login")
	if err := ghProvenance("/tmp/archive.tar.gz", "0.90.0"); !errors.As(err, new(provenanceUnchecked)) {
		t.Fatalf("error = %v, want provenanceUnchecked", err)
	}
}

func TestGHProvenance_TreatsAMissingGHAsUnchecked(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := ghProvenance("/tmp/archive.tar.gz", "0.90.0"); !errors.As(err, new(provenanceUnchecked)) {
		t.Fatalf("error = %v, want provenanceUnchecked", err)
	}
}

func TestGHProvenance_TreatsAGHWithoutTheFlagsAsUnchecked(t *testing.T) {
	fakeGH(t, 1, "unknown flag: --source-ref")
	err := ghProvenance("/tmp/archive.tar.gz", "0.90.0")
	var unchecked provenanceUnchecked
	if !errors.As(err, &unchecked) || !strings.Contains(unchecked.reason, "update gh") {
		t.Fatalf("error = %v, want provenanceUnchecked asking to update gh", err)
	}
}

func TestInstallVerifiedRelease_SkipsTheCheckWhenTurnedOff(t *testing.T) {
	server, _ := serveRelease(t, "0.90.0", "0.90.0")
	t.Setenv(verifyAttestationEnv, "0")
	path := installedBinary(t)
	var out bytes.Buffer
	if err := installVerifiedRelease(&out, path, "0.90.0", server.URL, server.Client(), provenanceNotCalled(t)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), verifyAttestationEnv+"=0") {
		t.Errorf("output does not say the check was turned off:\n%s", out.String())
	}
}

func TestInstallVerifiedRelease_FailsAnUncheckedReleaseWhenRequired(t *testing.T) {
	server, _ := serveRelease(t, "0.90.0", "0.90.0")
	t.Setenv(verifyAttestationEnv, "1")
	path := installedBinary(t)
	verify := func(string, string) error { return provenanceUnchecked{"the GitHub CLI (gh) is not installed"} }
	err := installVerifiedRelease(&bytes.Buffer{}, path, "0.90.0", server.URL, server.Client(), verify)
	if err == nil || !strings.Contains(err.Error(), "gh) is not installed") {
		t.Fatalf("error = %v, want the unchecked reason as a failure", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary replaced although the required check could not run")
	}
}

// A release published as latest by whoever can write release assets must
// not skip the check by claiming a version from before it existed.
func TestUpgrade_RefusesALatestReleaseThatPredatesProvenance(t *testing.T) {
	clearVerifyAttestation(t)
	deps := upgradeDeps{
		detect: func(version string) (upgradeInfo, error) {
			return upgradeInfo{Path: "/tmp/agnostic-ai", Method: installBinary, Version: version, Latest: "0.74.9"}, nil
		},
		run: func(io.Writer, string) error { t.Error("ran a command"); return nil },
		install: func(io.Writer, string, string, string, *http.Client) error {
			t.Error("installed a latest release that predates build provenance")
			return nil
		},
	}
	err := runUpgradeWithDeps(&bytes.Buffer{}, false, "", "0.89.0", deps)
	if err == nil || !strings.Contains(err.Error(), "predates build provenance") {
		t.Fatalf("error = %v, want a refusal", err)
	}
}
