package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	// Image says checksums.txt of a newer latest release lists this Mac's companion disk image.
	Image bool `json:"image"`
}

func companionUpdateStatusOf(info upgradeInfo, image bool) (companionUpdateStatus, error) {
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
	return companionUpdateStatus{Installed: info.Version, Latest: latest, Method: info.Method.String(), Command: info.Command, OneClick: oneClick, Image: image}, nil
}

func writeCompanionUpdateStatus(out io.Writer, info upgradeInfo, image bool) error {
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
	_, err = releaseChecksum(checksums, fmt.Sprintf("%s_darwin_%s.dmg", companionName, machineArch()))
	return err == nil, nil
}

// detectCompanionUpdate is what update-status reports: the install, and
// the latest release from the shared update cache.
func detectCompanionUpdate(current string, refresh bool) (upgradeInfo, bool, error) {
	info, err := detectUpgradeInstallation(current)
	if err != nil {
		return upgradeInfo{}, false, err
	}
	path, err := upgradeCachePath()
	if err != nil {
		return upgradeInfo{}, false, err
	}
	var image func(string) (bool, error)
	if runtime.GOOS == "darwin" {
		image = companionImageListed
	}
	latest, hasImage, err := companionUpdateCacheAt(path, info.Version, time.Now(), refresh, fetchUpgradeOffer, image)
	info.Latest, info.LatestError = latest, err
	return info, hasImage, nil
}

func companionUpdateStatusCommand(detect func(string, bool) (upgradeInfo, bool, error)) *cobra.Command {
	var asJSON, refresh bool
	cmd := &cobra.Command{
		Use:   "update-status",
		Short: "Show the installed and latest stable release, and how to upgrade.",
		Long: `Show the installed and latest stable release, and how to upgrade.

The companion reads it with --json to offer an update. The answer is kept
for 24 hours in the update cache agnostic-ai upgrade shares; --refresh asks
GitHub now.`,
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
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed: %s\nLatest: %s\nInstalled with: %s\nUpgrade with: agnostic-ai upgrade\n", status.Installed, status.Latest, status.Method)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print one JSON object: installed, latest, method, command, oneClick, image")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Ask GitHub now instead of using an answer from the last 24 hours")
	return cmd
}
