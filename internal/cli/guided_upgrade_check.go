package cli

import (
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func stableRelease(version string) bool {
	_, ok := stableReleaseVersion(version)
	return ok
}

// stableReleaseVersion is version as X.Y.Z when it names a stable
// release, not a dev, snapshot, or pre-release build.
func stableReleaseVersion(version string) (string, bool) {
	r, err := config.ParseRequirement(version)
	if err != nil {
		return "", false
	}
	if allowed, stable := r.Allows(version); allowed && stable {
		return r.String(), true
	}
	return "", false
}

func newerStableRelease(latest, current string) bool {
	if !stableRelease(latest) || !stableRelease(current) {
		return false
	}
	r, err := config.ParseRequirement("<" + strings.TrimPrefix(latest, "v"))
	if err != nil {
		return false
	}
	allowed, _ := r.Allows(current)
	return allowed
}

func fetchUpgradeOffer(current string) (upgradeOffer, error) {
	latest, err := fetchLatestRelease(2 * time.Second)
	if err != nil {
		return upgradeOffer{}, err
	}
	return upgradeOffer{Latest: strings.TrimPrefix(latest, "v")}, nil
}
