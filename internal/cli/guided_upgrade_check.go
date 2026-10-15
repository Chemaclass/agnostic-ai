package cli

import (
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func stableRelease(version string) bool {
	r, err := config.ParseRequirement(version)
	if err != nil {
		return false
	}
	allowed, stable := r.Allows(version)
	return allowed && stable
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
