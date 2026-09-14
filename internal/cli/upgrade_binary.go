package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	maxReleaseArchive   = 64 << 20
	maxReleaseBinary    = 128 << 20
	maxChecksumsFile    = 1 << 20
	windowsInstallerURL = "https://raw.githubusercontent.com/" + repoOwner + "/" + repoName + "/main/scripts/install.ps1"
)

func standalonePlatformError(goos string) error {
	if goos == "windows" {
		return fmt.Errorf("standalone binary updates on Windows require the installer: irm %s | iex", windowsInstallerURL)
	}
	if goos != "darwin" && goos != "linux" {
		return fmt.Errorf("standalone binary updates are unsupported on %s", goos)
	}
	return nil
}

func installReleaseBinary(path, version, baseURL string, client *http.Client) error {
	if err := standalonePlatformError(runtime.GOOS); err != nil {
		return err
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return fmt.Errorf("no release binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	current, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !current.Mode().IsRegular() {
		return fmt.Errorf("%s: installed binary is not a regular file", path)
	}

	asset := fmt.Sprintf("%s_%s_%s.tar.gz", binaryName, runtime.GOOS, runtime.GOARCH)
	checksums, err := downloadReleaseAsset(client, releaseAssetURL(baseURL, version, "checksums.txt"), maxChecksumsFile)
	if err != nil {
		return fmt.Errorf("checksums.txt: %w", err)
	}
	want, err := releaseChecksum(checksums, asset)
	if err != nil {
		return err
	}
	archive, err := downloadReleaseAsset(client, releaseAssetURL(baseURL, version, asset), maxReleaseArchive)
	if err != nil {
		return fmt.Errorf("%s: %w", asset, err)
	}
	got := sha256.Sum256(archive)
	if got != want {
		return fmt.Errorf("%s: checksum mismatch", asset)
	}

	staged, err := os.CreateTemp(filepath.Dir(path), ".agnostic-ai-update-*")
	if err != nil {
		return fmt.Errorf("%s: stage update: %w", path, err)
	}
	defer func() { _ = os.Remove(staged.Name()) }()
	if err := extractReleaseBinary(staged, archive); err != nil {
		_ = staged.Close()
		return fmt.Errorf("%s: %w", asset, err)
	}
	if err := staged.Chmod(0o755); err != nil {
		_ = staged.Close()
		return fmt.Errorf("%s: set executable mode: %w", staged.Name(), err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("%s: close staged binary: %w", staged.Name(), err)
	}
	if err := verifyReleaseBinary(staged.Name(), version); err != nil {
		return err
	}
	if err := os.Rename(staged.Name(), path); err != nil {
		return fmt.Errorf("%s: replace binary: %w", path, err)
	}
	return nil
}

func releaseAssetURL(baseURL, version, asset string) string {
	return strings.TrimRight(baseURL, "/") + "/" + url.PathEscape("v"+version) + "/" + asset
}

func downloadReleaseAsset(client *http.Client, assetURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeds %d bytes", limit)
	}
	return data, nil
}

func releaseChecksum(data []byte, asset string) ([sha256.Size]byte, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != asset {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size {
			return [sha256.Size]byte{}, fmt.Errorf("checksums.txt: invalid checksum for %s", asset)
		}
		var sum [sha256.Size]byte
		copy(sum[:], decoded)
		return sum, nil
	}
	return [sha256.Size]byte{}, fmt.Errorf("checksums.txt: %s is missing", asset)
}

func extractReleaseBinary(dst io.Writer, archive []byte) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("archive has no %s binary", binaryName)
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if header.Name != binaryName && header.Name != "./"+binaryName {
			continue
		}
		// Both '0' and NUL mark regular files in tar archives.
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return fmt.Errorf("archive %s is not a regular file", binaryName)
		}
		if header.Size <= 0 || header.Size > maxReleaseBinary {
			return fmt.Errorf("archive %s has invalid size", binaryName)
		}
		if _, err := io.CopyN(dst, tr, header.Size); err != nil {
			return fmt.Errorf("extract %s: %w", binaryName, err)
		}
		return nil
	}
}

func verifyReleaseBinary(path, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return fmt.Errorf("%s: verify version: %w", path, err)
	}
	want := binaryName + " version " + version
	if strings.TrimSpace(string(output)) != want {
		return fmt.Errorf("%s: expected %q, got %q", path, want, strings.TrimSpace(string(output)))
	}
	return nil
}
