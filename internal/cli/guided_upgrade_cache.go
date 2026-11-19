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
}

func cachedUpgradeOffer(current string) (upgradeOffer, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return upgradeOffer{}, fmt.Errorf("locate update cache: %w", err)
	}
	return cachedUpgradeOfferAt(filepath.Join(dir, "agnostic-ai", "update-check.json"), current, time.Now(), fetchUpgradeOffer)
}

func cachedUpgradeOfferAt(path, current string, now time.Time, fetch func(string) (upgradeOffer, error)) (upgradeOffer, error) {
	var cache upgradeCheckCache
	if f, err := os.Open(path); err == nil {
		_ = json.NewDecoder(io.LimitReader(f, 256<<10)).Decode(&cache)
		_ = f.Close()
	}
	recent := func(at time.Time) bool { return !at.IsZero() && !at.After(now) && now.Sub(at) < 24*time.Hour }
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
