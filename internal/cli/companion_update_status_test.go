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

	if err := writeCompanionUpdateStatus(&out, info, false); err != nil {
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
	if err := writeCompanionUpdateStatus(&out, upgradeInfo{Method: installUnknown, Version: "0.96.0", Latest: "0.97.0"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"oneClick":false`) || !strings.Contains(out.String(), `"method":"unknown"`) {
		t.Errorf("output = %s", out.String())
	}
}

func TestCompanionUpdateStatus_FailsWhenTheLatestReleaseIsUnknown(t *testing.T) {
	var out bytes.Buffer
	err := writeCompanionUpdateStatus(&out, upgradeInfo{Method: installNPM, Version: "0.96.0", LatestError: errors.New("offline")}, false)
	if err == nil || !strings.Contains(err.Error(), "offline") || out.Len() != 0 {
		t.Errorf("err = %v, output %q", err, out.String())
	}
	err = writeCompanionUpdateStatus(&out, upgradeInfo{Method: installNPM, Version: "0.96.0", Latest: "0.97.0-rc.1"}, false)
	if err == nil || !strings.Contains(err.Error(), "not a stable release") {
		t.Errorf("prerelease err = %v", err)
	}
}

func TestCompanionUpdateStatus_OneClickFollowsWhatUpgradeCanRun(t *testing.T) {
	status, err := companionUpdateStatusOf(upgradeInfo{Method: installBinary, Version: "0.96.0", Latest: "0.97.0"}, false)
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

	for i, at := range []time.Time{now, now.Add(time.Hour), now.Add(23 * time.Hour)} {
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
	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now.Add(27*time.Hour), false, fetch, image); err != nil {
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

func TestCompanionUpdateCache_RemembersAFailedCheckForTheDay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	asked := 0
	offline := func(string) (upgradeOffer, error) { asked++; return upgradeOffer{}, errors.New("offline") }

	if _, _, err := companionUpdateCacheAt(path, "0.96.0", now, false, offline, nil); err == nil {
		t.Fatal("a failed check reported no error")
	}
	latest, _, err := companionUpdateCacheAt(path, "0.96.0", now.Add(time.Hour), false, offline, nil)
	if asked != 1 || latest != "" || err != nil {
		t.Errorf("an hour later: asked %d, latest %q, err %v; want no second request and no answer", asked, latest, err)
	}
}

func TestCompanionUpdateStatus_SaysWhetherTheLatestReleaseHasADiskImage(t *testing.T) {
	status, err := companionUpdateStatusOf(upgradeInfo{Method: installHomebrew, Version: "0.96.0", Latest: "0.97.0"}, true)
	if err != nil || !status.Image {
		t.Errorf("status = %+v, %v; want a disk image", status, err)
	}
}
