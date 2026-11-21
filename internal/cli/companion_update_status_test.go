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
	want := companionUpdateStatus{Installed: "0.96.0", Latest: "0.97.0", Method: "homebrew", Command: upgradeCommandFor(installHomebrew), OneClick: true}
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

	for i, at := range []time.Time{now, now.Add(time.Hour), now.Add(22 * time.Hour)} {
		latest, hasImage, err := companionUpdateCacheAt(path, "0.96.0", at, false, fetch, image)
		if err != nil || latest != "0.97.0" || !hasImage {
			t.Fatalf("check %d = %q, %v, %v", i, latest, hasImage, err)
		}
	}
	if asked != 1 || imaged != 1 {
		t.Errorf("within a day: asked %d times, checked the image %d times; want once each", asked, imaged)
	}
	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now.Add(2*time.Hour), true, fetch, image); err != nil {
		t.Fatal(err)
	}
	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now.Add(26*time.Hour), false, fetch, image); err != nil {
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

	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now, false, func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.97.0"}, nil }, nil); err != nil {
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
	kept := upgradeCheckCache{CheckedAt: now.Add(-48 * time.Hour), OfferedAt: now.Add(-time.Hour), Current: "0.95.0", Offer: upgradeOffer{Latest: "0.97.0"}}
	if err := writeUpgradeCache(path, kept); err != nil {
		t.Fatal(err)
	}
	offline := func(string) (upgradeOffer, error) { return upgradeOffer{}, errors.New("offline") }

	for _, refresh := range []bool{false, true} {
		if _, _, err := companionUpdateCacheAt(path, "0.96.0", now, refresh, offline, nil); err == nil {
			t.Fatalf("refresh %v: a failed check reported no error", refresh)
		}
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
		if _, _, err := companionUpdateCacheAt(path, installed, now, false, fetch, nil); err != nil {
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

	latest, _, err := companionUpdateCacheAt(path, "0.96.0", now, false, fetch, nil)
	if err != nil || latest != "0.97.0" || asked != 1 {
		t.Errorf("latest %q, err %v, asked %d; want 0.97.0 after one fetch", latest, err, asked)
	}
}

func TestCompanionUpdateCache_ReportsAnUnreadableImageListWhenNothingIsCached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	fetch := func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.97.0"}, nil }
	unreadable := func(string) (bool, error) { return false, errors.New("checksums.txt: 502") }

	if latest, _, err := companionUpdateCacheAt(path, "0.96.0", now, true, fetch, unreadable); latest != "0.97.0" || err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("first check = %q, %v; want the latest release and the image list error", latest, err)
	}

	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now, true, fetch, func(string) (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	_, image, err := companionUpdateCacheAt(path, "0.96.0", now, true, fetch, unreadable)
	if err != nil || !image {
		t.Errorf("with a cached answer: image %v, err %v; want the cached image and no error", image, err)
	}
}

func TestCompanionUpdateStatus_KeepsTheLatestReleaseWhenTheImageListIsUnreadable(t *testing.T) {
	status, err := companionUpdateStatusOf(upgradeInfo{Method: installHomebrew, Version: "0.96.0", Latest: "0.97.0"}, companionImage{Err: errors.New("checksums.txt: 502")})
	if err != nil || status.Latest != "0.97.0" || status.Image || status.ImageError != "checksums.txt: 502" {
		t.Errorf("status = %+v, %v; want the latest release and the image error", status, err)
	}
}
