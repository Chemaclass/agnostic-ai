package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
)

type upgradeCheckCache struct {
	CheckedAt time.Time
	OfferedAt time.Time
	Current   string
	Offer     upgradeOffer
	// CompanionImageTag is the release whose checksums.txt was read for
	// this Mac's companion disk image; CompanionImage says it was listed.
	CompanionImageTag string
	CompanionImage    bool
}

func upgradeCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate update cache: %w", err)
	}
	return filepath.Join(dir, "agnostic-ai", "update-check.json"), nil
}

func recentUpgradeCheck(at, now time.Time) bool {
	return !at.IsZero() && !at.After(now) && now.Sub(at) < 24*time.Hour
}

func readUpgradeCache(path string) upgradeCheckCache {
	var cache upgradeCheckCache
	if f, err := os.Open(path); err == nil {
		_ = json.NewDecoder(io.LimitReader(f, 256<<10)).Decode(&cache)
		_ = f.Close()
	}
	return cache
}

// companionUpdateCacheAt answers the latest stable release, and whether
// it has a companion disk image, from the update cache the guided upgrade
// shares. It asks again after 24 hours, for another installed version, or
// when refresh is set; a failed check is remembered for the day. image is
// checked only for a newer release, and nil skips it.
func companionUpdateCacheAt(path, current string, now time.Time, refresh bool, fetch func(string) (upgradeOffer, error), image func(string) (bool, error)) (string, bool, error) {
	cache := readUpgradeCache(path)
	if refresh || !recentUpgradeCheck(cache.CheckedAt, now) || cache.Current != current {
		offer, err := fetch(current)
		if err != nil {
			offer = upgradeOffer{}
		}
		if offer.Latest != cache.Offer.Latest {
			cache.OfferedAt, cache.CompanionImageTag, cache.CompanionImage = time.Time{}, "", false
		}
		cache.CheckedAt, cache.Current, cache.Offer = now, current, offer
		if err != nil {
			_ = writeUpgradeCache(path, cache)
			return "", false, err
		}
	}
	latest := cache.Offer.Latest
	if image != nil && newerStableRelease(latest, current) && (refresh || cache.CompanionImageTag != latest) {
		if listed, err := image(latest); err == nil {
			cache.CompanionImageTag, cache.CompanionImage = latest, listed
		}
	}
	_ = writeUpgradeCache(path, cache)
	return latest, cache.CompanionImageTag == latest && cache.CompanionImage, nil
}

func cachedUpgradeOffer(current string) (upgradeOffer, error) {
	path, err := upgradeCachePath()
	if err != nil {
		return upgradeOffer{}, err
	}
	return cachedUpgradeOfferAt(path, current, time.Now(), fetchUpgradeOffer)
}

func cachedUpgradeOfferAt(path, current string, now time.Time, fetch func(string) (upgradeOffer, error)) (upgradeOffer, error) {
	cache := readUpgradeCache(path)
	recent := func(at time.Time) bool { return recentUpgradeCheck(at, now) }
	if !recent(cache.CheckedAt) || cache.Current != current {
		offer, err := fetch(current)
		if offer.Latest != cache.Offer.Latest {
			cache.OfferedAt = time.Time{}
		}
		cache.CheckedAt, cache.Current, cache.Offer = now, current, offer
		if err != nil {
			cache.Offer = upgradeOffer{}
			_ = writeUpgradeCache(path, cache)
			return upgradeOffer{}, err
		}
	}
	if !newerStableRelease(cache.Offer.Latest, current) || recent(cache.OfferedAt) {
		_ = writeUpgradeCache(path, cache)
		return upgradeOffer{}, nil
	}
	cache.OfferedAt = now
	_ = writeUpgradeCache(path, cache)
	return cache.Offer, nil
}

func writeUpgradeCache(path string, cache upgradeCheckCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return atomicfile.Replace(path, append(data, '\n'), 0o600)
}
