package cli

import (
	"context"
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
func downloadCompanion(ctx context.Context, out io.Writer, client *http.Client, baseURL, version, goarch, into string, verify provenanceCheck) (string, error) {
	asset := companionAsset(goarch)
	archive, err := downloadVerifiedCompanionAsset(ctx, out, client, baseURL, version, asset, verify)
	if err != nil {
		return "", missingAsNoPrebuilt("", err)
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
	if _, ok := errors.AsType[releaseAssetMissing](err); ok {
		err = fmt.Errorf("%w (%w)", errNoPrebuiltCompanion, err)
	}
	if what == "" {
		return err
	}
	return fmt.Errorf("%s: %w", what, err)
}
