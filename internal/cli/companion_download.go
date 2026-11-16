package cli

import (
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

// companionAsset is the release archive that holds the macOS companion for goarch.
func companionAsset(goarch string) string {
	return fmt.Sprintf("%s_darwin_%s.tar.gz", companionName, goarch)
}

// downloadCompanion fetches the released macOS companion for goarch,
// checks it against checksums.txt and the release's build provenance,
// and writes the program into the folder into. Nothing is written
// unless both checks pass.
func downloadCompanion(out io.Writer, client *http.Client, baseURL, version, goarch, into string, verify provenanceCheck) (string, error) {
	asset := companionAsset(goarch)
	checksums, err := downloadReleaseAsset(client, releaseAssetURL(baseURL, version, "checksums.txt"), maxChecksumsFile)
	if err != nil {
		return "", missingAsNoPrebuilt("checksums.txt", err)
	}
	want, err := releaseChecksum(checksums, asset)
	if err != nil {
		return "", missingAsNoPrebuilt("", err)
	}
	archive, err := downloadReleaseAsset(client, releaseAssetURL(baseURL, version, asset), maxReleaseArchive)
	if err != nil {
		return "", missingAsNoPrebuilt(asset, err)
	}
	if sha256.Sum256(archive) != want {
		return "", fmt.Errorf("%s: checksum mismatch", asset)
	}
	if err := checkReleaseProvenance(out, archive, asset, version, verify); err != nil {
		return "", err
	}
	path := filepath.Join(into, companionName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	err = extractReleaseFile(file, archive, companionName)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", asset, err)
	}
	return path, nil
}

// missingAsNoPrebuilt names what was fetched and marks a release that
// does not publish it as one without a built companion.
func missingAsNoPrebuilt(what string, err error) error {
	var missing releaseAssetMissing
	if errors.As(err, &missing) {
		err = fmt.Errorf("%w (%w)", errNoPrebuiltCompanion, err)
	}
	if what == "" {
		return err
	}
	return fmt.Errorf("%s: %w", what, err)
}
