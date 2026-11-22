package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompanionUpdateStatus_ReportsTheLatestStableReleaseAndHowToUpgrade(t *testing.T) {
	info := upgradeInfo{Method: installHomebrew, Version: "0.96.0", Latest: "0.97.0", Command: upgradeCommandFor(installHomebrew)}
	var out bytes.Buffer

	if err := writeCompanionUpdateStatus(&out, info, companionImage{}); err != nil {
		t.Fatal(err)
	}

	var got companionUpdateStatus
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	want := companionUpdateStatus{Installed: "0.96.0", Latest: "0.97.0", Method: "homebrew", Command: upgradeCommandFor(installHomebrew), OneClick: true, ConfirmsRelease: true}
	if got != want {
		t.Errorf("status = %+v; want %+v", got, want)
	}
}

func TestCompanionUpdateStatus_OffersNoOneClickForAnUnknownInstall(t *testing.T) {
	var out bytes.Buffer
	if err := writeCompanionUpdateStatus(&out, upgradeInfo{Method: installUnknown, Version: "0.96.0", Latest: "0.97.0"}, companionImage{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"oneClick":false`) || !strings.Contains(out.String(), `"method":"unknown"`) {
		t.Errorf("output = %s", out.String())
	}
}

func TestCompanionUpdateStatus_FailsWhenTheLatestReleaseIsUnknown(t *testing.T) {
	var out bytes.Buffer
	err := writeCompanionUpdateStatus(&out, upgradeInfo{Method: installNPM, Version: "0.96.0", LatestError: errors.New("offline")}, companionImage{})
	if err == nil || !strings.Contains(err.Error(), "offline") || out.Len() != 0 {
		t.Errorf("err = %v, output %q", err, out.String())
	}
	err = writeCompanionUpdateStatus(&out, upgradeInfo{Method: installNPM, Version: "0.96.0", Latest: "0.97.0-rc.1"}, companionImage{})
	if err == nil || !strings.Contains(err.Error(), "not a stable release") {
		t.Errorf("prerelease err = %v", err)
	}
}

func TestCompanionUpdateStatus_OneClickFollowsWhatUpgradeCanRun(t *testing.T) {
	status, err := companionUpdateStatusOf(upgradeInfo{Method: installBinary, Version: "0.96.0", Latest: "0.97.0"}, companionImage{})
	if err != nil {
		t.Fatal(err)
	}
	if want := standalonePlatformError(runtime.GOOS) == nil; status.OneClick != want {
		t.Errorf("binary install on %s: oneClick = %v; want %v", runtime.GOOS, status.OneClick, want)
	}
}

func TestCompanionUpdateCache_AsksAtMostOnceADayUnlessRefreshed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	asked, imaged := 0, 0
	fetch := func(string) (upgradeOffer, error) { asked++; return upgradeOffer{Latest: "0.97.0"}, nil }
	image := func(latest string) (bool, error) {
		imaged++
		if latest != "0.97.0" {
			t.Errorf("image checked for %q", latest)
		}
		return true, nil
	}

	for i, at := range []time.Time{now, now.Add(time.Hour), now.Add(23*time.Hour + 59*time.Minute)} {
		latest, hasImage, err := companionUpdateCacheAt(path, "0.96.0", at, false, false, fetch, image)
		if err != nil || latest != "0.97.0" || !hasImage {
			t.Fatalf("check %d = %q, %v, %v", i, latest, hasImage, err)
		}
	}
	if asked != 1 || imaged != 1 {
		t.Errorf("within a day: asked %d times, checked the image %d times; want once each", asked, imaged)
	}
	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now.Add(2*time.Hour), true, false, fetch, image); err != nil {
		t.Fatal(err)
	}
	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now.Add(26*time.Hour), false, false, fetch, image); err != nil {
		t.Fatal(err)
	}
	if asked != 3 || imaged != 2 {
		t.Errorf("after a refresh and a day: asked %d, image %d; want 3 and 2", asked, imaged)
	}
}

func TestCompanionUpdateCache_LeavesTheGuidedOfferTimeAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	offered := now.Add(-time.Hour)
	if err := writeUpgradeCache(path, upgradeCheckCache{CheckedAt: now.Add(-48 * time.Hour), OfferedAt: offered, Current: "0.96.0", Offer: upgradeOffer{Latest: "0.97.0"}}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now, false, false, func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.97.0"}, nil }, nil); err != nil {
		t.Fatal(err)
	}

	var cache upgradeCheckCache
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &cache); err != nil || !cache.OfferedAt.Equal(offered) || !cache.CheckedAt.Equal(now) {
		t.Errorf("cache = %+v, %v; want the offer time kept and the check time moved", cache, err)
	}
}

func TestCompanionUpdateCache_AFailedCheckKeepsTheLastAnswer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	kept := upgradeCheckCache{CompanionImageTag: "0.97.0", CompanionImage: true, CheckedAt: now.Add(-48 * time.Hour), OfferedAt: now.Add(-time.Hour), Current: "0.95.0", Offer: upgradeOffer{Latest: "0.97.0"}}
	if err := writeUpgradeCache(path, kept); err != nil {
		t.Fatal(err)
	}
	offline := func(string) (upgradeOffer, error) { return upgradeOffer{}, errors.New("offline") }

	image := func(string) (bool, error) { t.Fatal("rechecked the cached image while offline"); return false, nil }
	if latest, listed, err := companionUpdateCacheAt(path, "0.96.0", now, false, false, offline, image); err != nil || latest != "0.97.0" || !listed {
		t.Errorf("offline start = %q, %v, %v; want the cached answer", latest, listed, err)
	}
	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now, true, false, offline, image); err == nil {
		t.Error("a failed manual check reported no error")
	}

	var cache upgradeCheckCache
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &cache); err != nil || cache.Offer != kept.Offer || !cache.OfferedAt.Equal(kept.OfferedAt) || cache.Current != "0.95.0" {
		t.Errorf("cache = %+v, %v; want it unchanged", cache, err)
	}
}

func TestCompanionUpdateCache_DoesNotAskAgainForAnotherInstalledVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	asked := 0
	fetch := func(string) (upgradeOffer, error) { asked++; return upgradeOffer{Latest: "0.97.0"}, nil }

	for _, installed := range []string{"0.96.0", "0.95.0", "0.96.0"} {
		if _, _, err := companionUpdateCacheAt(path, installed, now, false, false, fetch, nil); err != nil {
			t.Fatal(err)
		}
	}
	if asked != 1 {
		t.Errorf("asked %d times for two installed versions; want once", asked)
	}
}

func TestCompanionUpdateStatus_SaysWhetherTheLatestReleaseHasADiskImage(t *testing.T) {
	status, err := companionUpdateStatusOf(upgradeInfo{Method: installHomebrew, Version: "0.96.0", Latest: "0.97.0"}, companionImage{Listed: true})
	if err != nil || !status.Image {
		t.Errorf("status = %+v, %v; want a disk image", status, err)
	}
}

func TestCompanionUpdateCache_AsksAgainAfterAFailedGuidedCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if err := writeUpgradeCache(path, upgradeCheckCache{CheckedAt: now.Add(-time.Hour), Current: "0.96.0"}); err != nil {
		t.Fatal(err)
	}
	asked := 0
	fetch := func(string) (upgradeOffer, error) { asked++; return upgradeOffer{Latest: "0.97.0"}, nil }

	latest, _, err := companionUpdateCacheAt(path, "0.96.0", now, false, false, fetch, nil)
	if err != nil || latest != "0.97.0" || asked != 1 {
		t.Errorf("latest %q, err %v, asked %d; want 0.97.0 after one fetch", latest, err, asked)
	}
}

func TestCompanionUpdateCache_ReportsAnUnreadableImageListWhenNothingIsCached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	fetch := func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.97.0"}, nil }
	unreadable := func(string) (bool, error) { return false, errors.New("checksums.txt: 502") }

	if latest, _, err := companionUpdateCacheAt(path, "0.96.0", now, true, false, fetch, unreadable); latest != "0.97.0" || err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("first check = %q, %v; want the latest release and the image list error", latest, err)
	}

	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now, true, false, fetch, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	_, image, err := companionUpdateCacheAt(path, "0.96.0", now, true, false, fetch, unreadable)
	if err == nil || !image {
		t.Errorf("with a cached answer: image %v, err %v; want the cached image and the refresh failure", image, err)
	}
}

func TestCompanionUpdateStatus_KeepsTheLatestReleaseWhenTheImageListIsUnreadable(t *testing.T) {
	status, err := companionUpdateStatusOf(upgradeInfo{Method: installHomebrew, Version: "0.96.0", Latest: "0.97.0"}, companionImage{Err: errors.New("checksums.txt: 502")})
	if err != nil || status.Latest != "0.97.0" || status.Image || status.ImageError != "checksums.txt: 502" {
		t.Errorf("status = %+v, %v; want the latest release and the image error", status, err)
	}
}

func TestCompanionUpdateCache_KeepsVerifiedImageAfterNewReleaseManifestFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Now()
	previous := upgradeCheckCache{CheckedAt: now, Offer: upgradeOffer{Latest: "0.97.0"}, CompanionImageTag: "0.97.0", CompanionImage: true}
	if err := writeUpgradeCache(path, previous); err != nil {
		t.Fatal(err)
	}
	next := func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.98.0"}, nil }
	offline := errors.New("offline")
	unreadable := func(string) (bool, error) { return false, offline }
	if latest, image, err := companionUpdateCacheAt(path, "0.96.0", now, true, false, next, unreadable); latest != "0.98.0" || image || !errors.Is(err, offline) {
		t.Errorf("refresh = %q, %t, %v; want a failed check of 0.98.0", latest, image, err)
	}
	failedFetch := func(string) (upgradeOffer, error) { return upgradeOffer{}, offline }
	for _, at := range []time.Time{now.Add(time.Hour), now.Add(25 * time.Hour)} {
		if latest, image, err := companionUpdateCacheAt(path, "0.96.0", at, false, true, failedFetch, unreadable); latest != "0.97.0" || !image || err != nil {
			t.Errorf("offline start at %v = %q, %t, %v; want the verified 0.97.0 image", at, latest, image, err)
		}
	}
	if latest, image, err := companionUpdateCacheAt(path, "0.96.0", now.Add(time.Hour), false, false, failedFetch, unreadable); latest != "0.98.0" || image || !errors.Is(err, offline) {
		t.Errorf("direct status = %q, %t, %v; want latest release 0.98.0 and its image error", latest, image, err)
	}
	if latest, _, err := companionUpdateCacheAt(path, "0.96.0", now.Add(time.Hour), false, false, failedFetch, nil); latest != "0.98.0" || err != nil {
		t.Errorf("terminal start = %q, %v; want latest release 0.98.0", latest, err)
	}
	if latest, image, err := companionUpdateCacheAt(path, "0.96.0", now.Add(time.Hour), false, false, failedFetch, func(string) (bool, error) {
		return false, nil
	}); latest != "0.98.0" || image || err != nil {
		t.Errorf("successful manifest check = %q, %t, %v; want 0.98.0 without an image", latest, image, err)
	}
}

func TestCompanionUpdateStatus_DistinguishesMissingAndMalformedImageChecksums(t *testing.T) {
	const asset = "agnostic-ai-companion_darwin_arm64.dmg"
	for _, tc := range []struct {
		name, line      string
		listed, invalid bool
	}{
		{"absent", "invalid another-asset", false, false},
		{"invalid checksum", "invalid " + asset, false, true},
		{"missing checksum", asset, false, true},
		{"malformed row", "invalid extra " + asset, false, true},
		{"valid checksum", strings.Repeat("a", 64) + "  " + asset, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listed, err := companionImageInChecksums([]byte(tc.line), asset)
			if listed != tc.listed || (err != nil) != tc.invalid {
				t.Errorf("listed = %t, error = %v; want %t, invalid %t", listed, err, tc.listed, tc.invalid)
			}
		})
	}
}

func TestUpgradeCache_TheGuidedCheckKeepsTheImageAnotherProcessWrote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	fetch := func(string) (upgradeOffer, error) {
		// Another CLI process records the disk image while this one asks GitHub.
		if err := writeUpgradeCache(path, upgradeCheckCache{CheckedAt: now.Add(-time.Minute), Offer: upgradeOffer{Latest: "0.97.0"}, CompanionImageTag: "0.97.0", CompanionImage: true}); err != nil {
			t.Fatal(err)
		}
		return upgradeOffer{Latest: "0.97.0"}, nil
	}

	if _, err := cachedUpgradeOfferAt(path, "0.96.0", now, fetch); err != nil {
		t.Fatal(err)
	}

	cache := readUpgradeCache(path)
	if cache.CompanionImageTag != "0.97.0" || !cache.CompanionImage || !cache.CheckedAt.Equal(now) || !cache.OfferedAt.Equal(now) {
		t.Errorf("cache = %+v; want the image kept and the guided check recorded", cache)
	}
}

func TestUpgradeCache_WaitsForAnotherWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := tryLockProjectFile(lock); err != nil {
		t.Skipf("no file locks here: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		if err := writeUpgradeCache(path, upgradeCheckCache{CompanionImageTag: "0.97.0"}); err != nil {
			t.Error(err)
		}
		close(released)
		_ = lock.Close()
	}()

	err = updateUpgradeCache(path, func(cache *upgradeCheckCache) {
		select {
		case <-released:
		default:
			t.Error("wrote while another process held the cache")
		}
		cache.Current = "0.96.0"
	})

	if cache := readUpgradeCache(path); err != nil || cache.CompanionImageTag != "0.97.0" || cache.Current != "0.96.0" {
		t.Errorf("cache = %+v, %v; want both writes", cache, err)
	}
}

func TestCompanionUpdateCache_AFailedAutomaticCheckAsksAgainAtTheNextStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if err := writeUpgradeCache(path, upgradeCheckCache{CheckedAt: now.Add(-48 * time.Hour), Offer: upgradeOffer{Latest: "0.97.0"}}); err != nil {
		t.Fatal(err)
	}
	asked := 0
	offline := func(string) (upgradeOffer, error) { asked++; return upgradeOffer{}, errors.New("offline") }

	for _, at := range []time.Time{now, now.Add(time.Minute)} {
		if latest, _, err := companionUpdateCacheAt(path, "0.96.0", at, false, false, offline, nil); err != nil || latest != "0.97.0" {
			t.Fatalf("offline start = %q, %v; want the last answer", latest, err)
		}
	}
	if asked != 2 {
		t.Errorf("asked %d times; want each start to ask again after a failure", asked)
	}
}
