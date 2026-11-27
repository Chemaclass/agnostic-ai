package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// errNoPrebuiltCompanion is a release that ships no built companion for
// this Mac. The install then builds one, as it did before releases had it.
var errNoPrebuiltCompanion = errors.New("this release ships no built companion for this Mac")

func companionAsset(program, goarch, extension string) string {
	return fmt.Sprintf("%s_darwin_%s.%s", program, goarch, extension)
}

func companionReleaseChecksum(checksums []byte, goarch, extension string) (program string, sum [sha256.Size]byte, err error) {
	sum, err = releaseChecksum(checksums, companionAsset(companionName, goarch, extension))
	if _, missing := errors.AsType[releaseAssetMissing](err); extension != "dmg" || !missing {
		return companionName, sum, err
	}
	// #4060: Accept the earlier disk image layout during the rename.
	sum, err = releaseChecksum(checksums, companionAsset(legacyCompanionName, goarch, extension))
	return legacyCompanionName, sum, err
}

func downloadVerifiedCompanionAsset(ctx context.Context, out io.Writer, client *http.Client, baseURL, version, goarch, extension string, verify provenanceCheck) (string, []byte, error) {
	checksums, err := downloadReleaseAssetContext(ctx, client, releaseAssetURL(baseURL, version, "checksums.txt"), maxChecksumsFile)
	if err != nil {
		return "", nil, fmt.Errorf("checksums.txt: %w", err)
	}
	program, want, err := companionReleaseChecksum(checksums, goarch, extension)
	if err != nil {
		return "", nil, err
	}
	asset := companionAsset(program, goarch, extension)
	data, err := downloadReleaseAssetContext(ctx, client, releaseAssetURL(baseURL, version, asset), 128<<20)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", asset, err)
	}
	if sha256.Sum256(data) != want {
		return "", nil, fmt.Errorf("%s: checksum mismatch", asset)
	}
	if err := checkReleaseProvenance(out, data, asset, version, verify); err != nil {
		return "", nil, err
	}
	return program, data, nil
}

// downloadCompanion fetches the released macOS companion for goarch,
// checks it against checksums.txt and the release's build provenance,
// and writes the program into the folder into. Nothing is written
// unless both checks pass.
func downloadCompanion(ctx context.Context, out io.Writer, client *http.Client, baseURL, version, goarch, into string, verify provenanceCheck) (string, error) {
	program, archive, err := downloadVerifiedCompanionAsset(ctx, out, client, baseURL, version, goarch, "tar.gz", verify)
	if _, missing := errors.AsType[releaseAssetMissing](err); missing {
		return "", fmt.Errorf("%w (%w)", errNoPrebuiltCompanion, err)
	}
	if err != nil {
		return "", err
	}
	path := filepath.Join(into, companionName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	err = extractReleaseFile(file, archive, program)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", companionAsset(program, goarch, "tar.gz"), err)
	}
	return path, nil
}
