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
	// GuidedFailureAt throttles guided checks without losing the last release.
	GuidedFailureAt      time.Time
	GuidedFailureCurrent string
	Offer                upgradeOffer
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

// savedLater reports whether stored was saved by a process that started
// while ours waited, after ours started, so ours must not replace it. That
// process saved a time at most waited after ours; a time further ahead
// comes from a clock that was wrong, and is replaced.
func savedLater(stored, ours time.Time, waited time.Duration) bool {
	return stored.After(ours) && stored.Sub(ours) <= waited
}

// wallSince is the wall clock time since started. The monotonic clock
// stops while a Mac sleeps, and the saved times it is compared with do not.
func wallSince(started time.Time) time.Duration {
	return time.Now().Round(0).Sub(started.Round(0))
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
// shares. A failed automatic release lookup uses the last answer. A forced
// refresh reports a failed lookup even when a matching image result is cached.
// The image is checked only for a newer release, and nil skips it.
// keepImageOffer lets the app restore an older verified image offer when
// the latest release's image cannot be checked.
// It writes only the fields it owns, over a fresh read of the file.
func companionUpdateCacheAt(path, current string, now time.Time, refresh, keepImageOffer bool, fetch func(string) (upgradeOffer, error), image func(string) (bool, error)) (string, bool, error) {
	started := time.Now()
	cache := readUpgradeCache(path)
	fresh := cache.Offer.Latest != "" && recentUpgradeCheck(cache.CheckedAt, now)
	changed, imageChanged := false, false
	if refresh || !fresh {
		offer, err := fetch(current)
		if err != nil {
			latest := cache.Offer.Latest
			matchingImage := cache.CompanionImageTag == latest
			if !refresh && latest != "" {
				if image == nil || !newerStableRelease(latest, current) || matchingImage {
					return latest, matchingImage && cache.CompanionImage, nil
				}
				if keepImageOffer && cache.CompanionImage && newerStableRelease(cache.CompanionImageTag, current) {
					return cache.CompanionImageTag, true, nil
				}
			}
			return "", false, err
		}
		cache.CheckedAt, cache.Offer, changed = now, offer, true
	}
	latest := cache.Offer.Latest
	var imageErr error
	if image != nil && newerStableRelease(latest, current) && (refresh || cache.CompanionImageTag != latest) {
		listed, err := image(latest)
		switch {
		case err == nil:
			cache.CompanionImageTag, cache.CompanionImage, imageChanged = latest, listed, true
		case refresh || cache.CompanionImageTag != latest:
			imageErr = err
		}
	}
	var later *upgradeCheckCache
	if changed || imageChanged {
		_ = updateUpgradeCache(path, func(stored *upgradeCheckCache) {
			late := changed && savedLater(stored.CheckedAt, cache.CheckedAt, wallSince(started))
			if changed && !late {
				if stored.Offer.Latest != cache.Offer.Latest {
					stored.OfferedAt = time.Time{}
				}
				stored.CheckedAt, stored.Offer = cache.CheckedAt, cache.Offer
			}
			if imageChanged && stored.Offer.Latest == latest {
				stored.CompanionImageTag, stored.CompanionImage = cache.CompanionImageTag, cache.CompanionImage
			}
			if late {
				kept := *stored
				later = &kept
			}
		})
	}
	if later != nil && (later.Offer.Latest != latest || imageErr != nil && later.CompanionImageTag == latest) {
		return later.Offer.Latest, later.CompanionImageTag == later.Offer.Latest && later.CompanionImage, nil
	}
	if imageErr != nil && !refresh && keepImageOffer && cache.CompanionImage && newerStableRelease(cache.CompanionImageTag, current) {
		return cache.CompanionImageTag, true, nil
	}
	return latest, cache.CompanionImageTag == latest && cache.CompanionImage, imageErr
}

func cachedUpgradeOffer(current string) (upgradeOffer, error) {
	path, err := upgradeCachePath()
	if err != nil {
		return upgradeOffer{}, err
	}
	return cachedUpgradeOfferAt(path, current, time.Now(), fetchUpgradeOffer)
}

func cachedUpgradeOfferAt(path, current string, now time.Time, fetch func(string) (upgradeOffer, error)) (upgradeOffer, error) {
	started := time.Now()
	cache := readUpgradeCache(path)
	recent := func(at time.Time) bool { return recentUpgradeCheck(at, now) }
	if cache.GuidedFailureCurrent == current && cache.GuidedFailureAt.After(cache.CheckedAt) && recent(cache.GuidedFailureAt) {
		return upgradeOffer{}, nil
	}
	fetched := !recent(cache.CheckedAt) || cache.Current != current
	if fetched {
		offer, err := fetch(current)
		if err != nil {
			_ = updateUpgradeCache(path, func(stored *upgradeCheckCache) {
				if stored.CheckedAt.Before(now) && stored.GuidedFailureAt.Before(now) {
					stored.GuidedFailureAt, stored.GuidedFailureCurrent = now, current
				}
			})
			return upgradeOffer{}, err
		}
		if offer.Latest != cache.Offer.Latest {
			cache.OfferedAt = time.Time{}
		}
		cache.CheckedAt, cache.Current, cache.Offer = now, current, offer
		cache.GuidedFailureAt, cache.GuidedFailureCurrent = time.Time{}, ""
	}
	// The offer and the time it was made are decided on a fresh read, so
	// OfferedAt always belongs to the release stored beside it.
	var offered upgradeOffer
	offer := func(c *upgradeCheckCache) {
		if newerStableRelease(c.Offer.Latest, current) && !recent(c.OfferedAt) && !savedLater(c.OfferedAt, now, wallSince(started)) {
			c.OfferedAt, offered = now, c.Offer
		}
	}
	read := false
	// The companion fields belong to companion update-status; keep what it wrote meanwhile.
	_ = updateUpgradeCache(path, func(stored *upgradeCheckCache) {
		read = true
		if fetched && !savedLater(stored.CheckedAt, cache.CheckedAt, wallSince(started)) {
			if stored.Offer.Latest != cache.Offer.Latest {
				stored.OfferedAt = time.Time{}
			}
			stored.CheckedAt, stored.Current, stored.Offer = cache.CheckedAt, cache.Current, cache.Offer
			stored.GuidedFailureAt, stored.GuidedFailureCurrent = cache.GuidedFailureAt, cache.GuidedFailureCurrent
		}
		offer(stored)
	})
	if !read {
		// The cache could not be locked, so the offer comes from what was read before.
		offer(&cache)
	}
	return offered, nil
}

// Writers retry the cache lock this long; the holder only reads and replaces one small file.
const (
	upgradeCacheLockTries = 50
	upgradeCacheLockPause = 20 * time.Millisecond
)

// updateUpgradeCache applies change to a fresh read of the cache while it
// holds a lock beside it, so CLI processes that run at once keep each
// other's fields.
func updateUpgradeCache(path string, change func(*upgradeCheckCache)) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = lock.Close() }()
	for try := 1; ; try++ {
		err := tryLockProjectFile(lock)
		if err == nil {
			break
		}
		if !projectLockContended(err) {
			// Without file locks the cache is still written, as before the lock existed.
			break
		}
		if try == upgradeCacheLockTries {
			return fmt.Errorf("lock %s: %w", path, err)
		}
		time.Sleep(upgradeCacheLockPause)
	}
	cache := readUpgradeCache(path)
	change(&cache)
	return writeUpgradeCache(path, cache)
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
