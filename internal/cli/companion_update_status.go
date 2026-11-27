package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// companionUpdateStatus is what a terminal-installed companion reads to
// offer an update: the companion and this CLI share one version.
type companionUpdateStatus struct {
	Installed string `json:"installed"`
	Latest    string `json:"latest"`
	Method    string `json:"method"`
	Command   string `json:"command,omitempty"`
	// OneClick says agnostic-ai upgrade knows how to upgrade this install.
	OneClick bool `json:"oneClick"`
	// ConfirmsRelease says upgrade can refuse a release that differs from the one accepted in the companion.
	ConfirmsRelease bool `json:"confirmsRelease"`
	// Image says checksums.txt of a newer latest release lists this Mac's companion disk image.
	Image bool `json:"image"`
	// ImageError says why checksums.txt could not tell whether it lists the disk image.
	ImageError string `json:"imageError,omitempty"`
}

// companionImage is whether the latest release lists this Mac's disk image,
// or why that is unknown.
type companionImage struct {
	Listed bool
	Err    error
}

func companionUpdateStatusOf(info upgradeInfo, image companionImage) (companionUpdateStatus, error) {
	if info.Latest == "" {
		if info.LatestError != nil {
			return companionUpdateStatus{}, fmt.Errorf("check latest release: %w", info.LatestError)
		}
		return companionUpdateStatus{}, errors.New("check latest release: no published release")
	}
	latest, ok := stableReleaseVersion(info.Latest)
	if !ok {
		return companionUpdateStatus{}, fmt.Errorf("check latest release: %q is not a stable release", info.Latest)
	}
	oneClick := info.Method != installUnknown && (info.Method != installBinary || standalonePlatformError(runtime.GOOS) == nil)
	command := info.Command
	if info.Method == installNPM {
		oneClick = info.NPMPrefix != ""
		command = ""
		if oneClick {
			command = npmUpgradeStatusCommand(info.Path)
		}
	}
	status := companionUpdateStatus{Installed: info.Version, Latest: latest, Method: info.Method.String(), Command: command, OneClick: oneClick, ConfirmsRelease: true, Image: image.Listed}
	if image.Err != nil {
		status.ImageError = image.Err.Error()
	}
	return status, nil
}

func writeCompanionUpdateStatus(out io.Writer, info upgradeInfo, image companionImage) error {
	status, err := companionUpdateStatusOf(info, image)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(status)
}

// companionImageListed says checksums.txt of version lists this Mac's
// companion disk image, which the update needs.
func companionImageListed(version string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	checksums, err := downloadReleaseAssetContext(ctx, &http.Client{}, releaseAssetURL(releasesBaseURL, version, "checksums.txt"), maxChecksumsFile)
	if err != nil {
		return false, fmt.Errorf("checksums.txt: %w", err)
	}
	return companionImageInChecksums(checksums, machineArch())
}

func companionImageInChecksums(checksums []byte, goarch string) (bool, error) {
	_, _, err := companionReleaseChecksum(checksums, goarch, "dmg")
	if _, missing := errors.AsType[releaseAssetMissing](err); missing {
		return false, nil
	}
	return err == nil, err
}

// detectCompanionUpdate is what update-status reports: the install, and
// the latest release from the shared update cache.
func detectCompanionUpdate(current string, refresh bool) (upgradeInfo, companionImage, error) {
	info, err := detectUpgradeInstallation(current)
	if err != nil {
		return upgradeInfo{}, companionImage{}, err
	}
	path, err := upgradeCachePath()
	if err != nil {
		return upgradeInfo{}, companionImage{}, err
	}
	var image func(string) (bool, error)
	// A terminal companion only needs the latest release. Keep image
	// checks enabled for direct update-status calls.
	if runtime.GOOS == "darwin" && os.Getenv("AGNOSTIC_AI_COMPANION_SKIP_IMAGE") != "1" {
		image = companionImageListed
	}
	keepImageOffer := os.Getenv("AGNOSTIC_AI_COMPANION_KEEP_IMAGE_OFFER") == "1"
	latest, listed, err := companionUpdateCacheAt(path, info.Version, time.Now(), refresh, keepImageOffer, fetchUpgradeOffer, image)
	info.Latest = latest
	if latest == "" {
		info.LatestError = err
		return info, companionImage{}, nil
	}
	return info, companionImage{Listed: listed, Err: err}, nil
}

func companionUpdateStatusCommand(detect func(string, bool) (upgradeInfo, companionImage, error)) *cobra.Command {
	var asJSON, refresh bool
	cmd := &cobra.Command{
		Use:   "update-status",
		Short: "Show the installed and latest stable release, and how to upgrade.",
		Long: `Show the installed and latest stable release, and how to upgrade.

The companion reads it with --json to offer an update. The answer is kept
for up to a day in the update cache agnostic-ai upgrade shares, and a failed
check keeps the last one; --refresh asks GitHub now.`,
		Example: `  agnostic-ai companion update-status --json`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info, image, err := detect(cmd.Root().Version, refresh)
			if err != nil {
				return err
			}
			if asJSON {
				return writeCompanionUpdateStatus(cmd.OutOrStdout(), info, image)
			}
			status, err := companionUpdateStatusOf(info, image)
			if err != nil {
				return err
			}
			guidance := "Upgrade with: agnostic-ai upgrade"
			if info.Method == installNPM {
				guidance = npmUpgradeGuidance
				if status.Command != "" {
					guidance = "Upgrade with: " + status.Command
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed: %s\nLatest: %s\nInstalled with: %s\n%s\n", status.Installed, status.Latest, status.Method, guidance)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print one JSON object: installed, latest, method, command, oneClick, image")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Ask GitHub now instead of using an answer from the last 24 hours")
	return cmd
}
