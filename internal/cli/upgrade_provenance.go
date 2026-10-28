package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	releaseWorkflow = repoOwner + "/" + repoName + "/.github/workflows/release.yml"
	// verifyAttestationEnv set to 1 makes a provenance check that cannot run
	// fail, and set to 0 skips the check. The installers read it too.
	verifyAttestationEnv = "AGNOSTIC_AI_VERIFY_ATTESTATION"
	// gh exits 4 when a command needs a login it does not have.
	ghAuthRequiredExit = 4
	ghVerifyTimeout    = time.Minute
)

// firstAttestedRelease is the first release whose archives carry build
// provenance. Older releases still install, on the checksum alone, when
// the user names one.
var firstAttestedRelease = []int{0, 75, 0}

// ghTooOldRE matches gh rejecting the attestation command or one of its
// flags, which older releases of gh do not have.
var ghTooOldRE = regexp.MustCompile(`unknown (flag|shorthand flag|command)`)

// provenanceUnchecked is a provenance check that could not run, as opposed
// to one that ran and failed.
type provenanceUnchecked struct{ reason string }

func (e provenanceUnchecked) Error() string { return "build provenance not checked: " + e.reason }

// provenanceCheck verifies that this repository's release workflow built
// the archive at archivePath for the given version's tag.
type provenanceCheck func(archivePath, version string) error

// checkReleaseProvenance runs verify on the downloaded archive. The
// checksum already matched, but it comes from the same release page as
// the archive; the attestation is signed by the release workflow itself.
func checkReleaseProvenance(out io.Writer, archive []byte, asset, version string, verify provenanceCheck) error {
	required := os.Getenv(verifyAttestationEnv) == "1"
	if os.Getenv(verifyAttestationEnv) == "0" {
		_, _ = fmt.Fprintf(out, "Build provenance not checked: %s=0. The checksum matched.\n", verifyAttestationEnv)
		return nil
	}
	if !releaseHasProvenance(version) {
		if required {
			return fmt.Errorf("%s: v%s predates build provenance, which starts at %s", asset, version, firstAttestedTag())
		}
		_, _ = fmt.Fprintf(out, "Build provenance not checked: releases before %s do not publish it.\n", firstAttestedTag())
		return nil
	}
	file, err := os.CreateTemp("", "agnostic-ai-release-*.tar.gz")
	if err != nil {
		return fmt.Errorf("%s: stage for provenance check: %w", asset, err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, err = file.Write(archive)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("%s: stage for provenance check: %w", asset, err)
	}
	err = verify(file.Name(), version)
	var unchecked provenanceUnchecked
	switch {
	case errors.As(err, &unchecked) && !required:
		_, _ = fmt.Fprintf(out, "Build provenance not checked: %s. The checksum matched.\n", unchecked.reason)
		return nil
	case err != nil:
		return fmt.Errorf("%s: %w", asset, err)
	}
	_, _ = fmt.Fprintln(out, "Build provenance verified with gh attestation verify.")
	return nil
}

// requireLatestProvenance refuses a latest release that claims a version
// from before build provenance existed. Whoever can publish release assets
// could otherwise mark such a release latest and skip the check.
func requireLatestProvenance(latest string) error {
	if os.Getenv(verifyAttestationEnv) == "0" || releaseHasProvenance(latest) {
		return nil
	}
	if os.Getenv(verifyAttestationEnv) == "1" {
		return fmt.Errorf("upgrade: the latest release v%s predates build provenance, which starts at %s, and %s=1 requires it",
			strings.TrimPrefix(latest, "v"), firstAttestedTag(), verifyAttestationEnv)
	}
	return fmt.Errorf("upgrade: the latest release v%s predates build provenance, which starts at %s; pass --version to install it on the checksum alone",
		strings.TrimPrefix(latest, "v"), firstAttestedTag())
}

// releaseHasProvenance reports whether version is new enough to carry
// build provenance. A version it cannot read is checked.
func releaseHasProvenance(version string) bool {
	core, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), "-")
	parts := make([]int, 3)
	if _, err := fmt.Sscanf(core, "%d.%d.%d", &parts[0], &parts[1], &parts[2]); err != nil {
		return true
	}
	return slices.Compare(parts, firstAttestedRelease) >= 0
}

func firstAttestedTag() string {
	return fmt.Sprintf("v%d.%d.%d", firstAttestedRelease[0], firstAttestedRelease[1], firstAttestedRelease[2])
}

// ghProvenance checks the archive with the GitHub CLI. A native Sigstore
// check would avoid the dependency on gh but adds a large one to the binary.
// The host is pinned because a gh logged in to GitHub Enterprise only
// would otherwise ask that server, which does not hold these attestations.
func ghProvenance(archivePath, version string) error {
	gh, err := exec.LookPath("gh")
	if err != nil {
		return provenanceUnchecked{"the GitHub CLI (gh) is not installed"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), ghVerifyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gh, "attestation", "verify", archivePath,
		"--hostname", "github.com",
		"--repo", repoOwner+"/"+repoName,
		"--signer-workflow", releaseWorkflow,
		"--source-ref", "refs/tags/v"+version,
		"--deny-self-hosted-runners")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	detail := strings.TrimSpace(stderr.String())
	var exit *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("build provenance check timed out after %s", ghVerifyTimeout)
	case errors.As(err, &exit) && exit.ExitCode() == ghAuthRequiredExit:
		return provenanceUnchecked{"gh is not logged in (run gh auth login)"}
	case ghTooOldRE.MatchString(detail):
		return provenanceUnchecked{"this gh cannot verify build provenance; update gh"}
	case detail == "":
		detail = err.Error()
	}
	return fmt.Errorf("build provenance did not verify: %s", detail)
}
