package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"

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
}

func companionUpdateStatusOf(info upgradeInfo) (companionUpdateStatus, error) {
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
	return companionUpdateStatus{Installed: info.Version, Latest: latest, Method: info.Method.String(), Command: info.Command, OneClick: oneClick}, nil
}

func writeCompanionUpdateStatus(out io.Writer, info upgradeInfo) error {
	status, err := companionUpdateStatusOf(info)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(status)
}

func companionUpdateStatusCommand(detect func(string) (upgradeInfo, error)) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "update-status",
		Short: "Show the installed and latest stable release, and how to upgrade.",
		Long: `Show the installed and latest stable release, and how to upgrade.

The companion installed from the terminal reads it with --json to offer an update.
It asks GitHub for the latest release each time.`,
		Example: `  agnostic-ai companion update-status --json`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info, err := detect(cmd.Root().Version)
			if err != nil {
				return err
			}
			if asJSON {
				return writeCompanionUpdateStatus(cmd.OutOrStdout(), info)
			}
			status, err := companionUpdateStatusOf(info)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed: %s\nLatest: %s\nInstalled with: %s\nUpgrade with: agnostic-ai upgrade\n", status.Installed, status.Latest, status.Method)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print one JSON object: installed, latest, method, command, oneClick")
	return cmd
}
