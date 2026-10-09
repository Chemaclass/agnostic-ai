package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestReleaseGuidance_CoversSkippedVersionsAndExcludesOtherReleases(t *testing.T) {
	doc := "## [Unreleased]\nfuture\n## v0.83.0 - 2026-10-11\nfuture version\n## v0.82.0 - 2026-10-10\nBreaking: manual task\n## [0.81.0] - 2026-10-07\nBuilt-in migration\n## v0.80.0 - 2026-10-01\nold task\n"
	guidance, err := releaseGuidance(doc, "0.80.0", "0.82.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Breaking: manual task", "Built-in migration"} {
		if !strings.Contains(guidance, want) {
			t.Errorf("missing %s in %s", want, guidance)
		}
	}
	for _, no := range []string{"future", "old task"} {
		if strings.Contains(guidance, no) {
			t.Errorf("unexpected %s in %s", no, guidance)
		}
	}
	if _, err := releaseGuidance(doc, "0.82.0", "0.84.0"); err == nil {
		t.Error("missing target guidance accepted")
	}
}

func TestCachedUpgradeOffer_DailyChecksAndPromptsIncludeOfflineFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	now := time.Now()
	calls := 0
	fetch := func(string) (upgradeOffer, error) {
		calls++
		return upgradeOffer{Latest: "0.82.0", Guidance: "notes"}, nil
	}
	offer, err := cachedUpgradeOfferAt(path, "0.81.0", now, fetch)
	if err != nil || offer.Latest != "0.82.0" {
		t.Fatalf("offer %v, error %v", offer, err)
	}
	offer, err = cachedUpgradeOfferAt(path, "0.81.0", now.Add(time.Hour), fetch)
	if err != nil || offer.Latest != "" || calls != 1 {
		t.Errorf("repeated offer %v, calls %d, error %v", offer, calls, err)
	}
	_, err = cachedUpgradeOfferAt(path, "0.81.0", now.Add(25*time.Hour), func(string) (upgradeOffer, error) { calls++; return upgradeOffer{}, errors.New("offline") })
	if err == nil {
		t.Error("offline check did not fail")
	}
	offer, err = cachedUpgradeOfferAt(path, "0.81.0", now.Add(26*time.Hour), fetch)
	if err != nil || offer.Latest != "" || calls != 2 {
		t.Errorf("offline retry %v, calls %d, error %v", offer, calls, err)
	}
}

func TestAutomaticUpgradeAllowed_OnlyInteractiveStableCommands(t *testing.T) {
	for _, name := range []string{envNoUpdateCheck, envUpgradeInProgress, "CI", "AGNOSTIC_AI_TARGET"} {
		t.Setenv(name, "")
	}
	for _, tc := range []struct {
		name, version, command, flag, env string
		interactive, want                 bool
	}{
		{name: "terminal release", version: "v0.81.0", command: "status", interactive: true, want: true},
		{name: "redirected", version: "0.81.0", command: "status"},
		{name: "development", version: "(devel)", command: "status", interactive: true},
		{name: "pseudo version", version: "v0.81.1-0.20261009123456-abcdef123456", command: "status", interactive: true},
		{name: "quiet", version: "0.81.0", command: "status", flag: "quiet", interactive: true},
		{name: "json", version: "0.81.0", command: "status", flag: "json", interactive: true},
		{name: "hook", version: "0.81.0", command: "hook", interactive: true},
		{name: "service", version: "0.81.0", command: "lsp", interactive: true},
		{name: "disabled", version: "0.81.0", command: "status", env: envNoUpdateCheck, interactive: true},
		{name: "recursive", version: "0.81.0", command: "status", env: envUpgradeInProgress, interactive: true},
		{name: "ci", version: "0.81.0", command: "status", env: "CI", interactive: true},
		{name: "global", version: "0.81.0", command: "sync", flag: "global", interactive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: tc.command}
			if tc.flag != "" {
				cmd.Flags().Bool(tc.flag, true, "")
			}
			if tc.env != "" {
				t.Setenv(tc.env, "1")
			}
			if got := automaticUpgradeAllowed(cmd, tc.version, tc.interactive); got != tc.want {
				t.Errorf("allowed %v, want %v", got, tc.want)
			}
		})
	}
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetOut(os.Stdout)
	if automaticUpgradeEligible(cmd, "0.81.0") {
		t.Error("redirected input accepted")
	}
}

func TestNewerStableRelease_RefusesDevelopmentAndDowngrades(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"0.82.0", "0.81.0", true}, {"0.81.0", "0.81.0", false}, {"0.81.0", "0.82.0", false}, {"0.82.0-rc1", "0.81.0", false}, {"0.82.0", "(devel)", false},
	} {
		if got := newerStableRelease(tc.latest, tc.current); got != tc.want {
			t.Errorf("%s > %s = %v", tc.latest, tc.current, got)
		}
	}
}
